package storage

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/cache"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/entropy"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/hashdb"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/heuristic"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/matcher"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/sigdb"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/yara"
	iamMiddleware "axiomnizam.bitbd.net/axiomnizam/internal/iam/middleware"
	iamStorage "axiomnizam.bitbd.net/axiomnizam/internal/iam/storage"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/token"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/archivescan"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/macro"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/metadata"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/mimetype"
	nativeav "axiomnizam.bitbd.net/axiomnizam/internal/scanner/native"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/svg"
	storageConfig "axiomnizam.bitbd.net/axiomnizam/internal/storage/config"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/access"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/admin"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/controller"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/audit"
	storageMetrics "axiomnizam.bitbd.net/axiomnizam/internal/storage/metrics"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/native"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/policy"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/tenant"
	"github.com/gin-gonic/gin"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// System holds the fully initialised object storage system and exposes
// the router registration. Follows the IAM System struct pattern.
type System struct {
	Backend    models.Backend
	Store      *store.BucketStore
	Controller *controller.BucketController
	Tenant     *tenant.Manager
	Policy     *policy.Controller
	Access     *access.Controller
	Handler    *admin.Handler
	Metrics    *storageMetrics.Collector
	AuditLog   *audit.AuditLog
	Config     Config

	// Antivirus engine for malware scanning on uploads.
	AVEngine   *antivirus.Engine
	AVSigDB    *sigdb.Database
	AVCache    *cache.Cache

	// SafeGate scanner orchestrator — full pipeline scanning on uploads.
	ScanOrch   *scanner.Orchestrator

	// IAM references (nil when IAM is not available).
	// These may be set after construction via SetIAM() when IAM init
	// is deferred (Raft mode).
	iamIssuer       *token.Issuer
	iamRevokedStore *iamStorage.EtcdRevokedTokenStore
}

// SetIAM sets the IAM issuer and revoked token store after construction.
// Used when IAM initialization is deferred (e.g., STORAGE_BACKEND=raft).
func (s *System) SetIAM(issuer *token.Issuer, revokedStore *iamStorage.EtcdRevokedTokenStore) {
	s.iamIssuer = issuer
	s.iamRevokedStore = revokedStore
}

// SetKVStore wires the KVStore-backed persistence into the BucketStore and
// Access Controller. Called in Raft mode after the Raft KV becomes available.
// This loads previously persisted buckets from the KVStore.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	s.Store.ConfigureKVPersistence(kv)
	s.Access.ConfigureKVPersistence(kv)
	if s.Metrics != nil {
		s.Metrics.ConfigureKVPersistence(kv)
	}
	if s.ScanOrch != nil && s.ScanOrch.Metrics() != nil {
		s.ScanOrch.Metrics().ConfigureKVPersistence(kv)
	}
	if s.AuditLog != nil {
		s.AuditLog.ConfigureKVPersistence(kv)
	}
	logging.Z().Info("✅ Storage: KVStore persistence configured (Raft mode)")
}

// Config re-exports the storage config type from the config sub-package.
type Config = storageConfig.Config

// DefaultConfig re-exports the default config constructor.
var DefaultConfig = storageConfig.DefaultConfig

// NewSystem initialises the complete object storage system.
// Uses the built-in native filesystem backend — no external service required.
// IAM issuer and revokedStore are optional; when provided, routes are protected
// by IAM JWT middleware so that the access controller can extract user identity.
func NewSystem(cfg Config, issuer *token.Issuer, revokedStore *iamStorage.EtcdRevokedTokenStore, etcdClient *clientv3.Client) (*System, error) {
	backend, err := native.New(cfg.DataDir, cfg.PresignSecret)
	if err != nil {
		return nil, err
	}

	endpoint := backend.Endpoint()
	bucketStore := store.NewBucketStore()
	bucketStore.ConfigurePersistence(etcdClient)
	tenantMgr := tenant.NewManager(cfg.BucketPrefix, bucketStore)
	bucketCtrl := controller.NewBucketController(bucketStore, backend, endpoint, cfg.ControllerResyncInterval, cfg.ControllerDebug)
	policyCtrl := policy.NewController()
	metricsCollector := storageMetrics.NewCollector()
	auditLog := audit.NewAuditLog(cfg.MaxAuditEvents)
	accessCtrl := access.NewController(auditLog, access.ControllerConfig{
		DefaultReadRateLimit:  cfg.ObjectReadRateLimit,
		DefaultWriteRateLimit: cfg.ObjectWriteRateLimit,
		EtcdTimeout:           cfg.EtcdTimeout,
	})
	accessCtrl.SetBucketStore(bucketStore)
	accessCtrl.ConfigurePersistence(etcdClient)
	// ── Antivirus engine ────────────────────────────────────────────
	avCfg := antivirus.LoadConfig()
	avEngine := antivirus.NewEngine(avCfg)

	// Create scan layers.
	hashDB := hashdb.New(0, 0)
	matcherBuilder := matcher.NewBuilder()
	matcher.RegisterBuiltinPatterns(matcherBuilder)
	matcherAutomaton := matcherBuilder.Build()
	matcherLayer := matcher.NewLayer(matcherAutomaton)
	heuristicLayer := heuristic.New()
	entropyLayer := entropy.New()
	yaraRuleSet := yara.NewRuleSet()
	yara.RegisterBuiltinRules(yaraRuleSet)
	yaraLayer := yara.NewLayer(yaraRuleSet)

	// Register layers in execution order (fastest → slowest).
	if avCfg.HashDBEnabled {
		avEngine.RegisterLayer(hashDB)
	}
	if avCfg.PatternEnabled {
		avEngine.RegisterLayer(matcherLayer)
	}
	if avCfg.HeuristicEnabled {
		avEngine.RegisterLayer(heuristicLayer)
	}
	if avCfg.EntropyEnabled {
		avEngine.RegisterLayer(entropyLayer)
	}
	if avCfg.YARAEnabled {
		avEngine.RegisterLayer(yaraLayer)
	}

	// Signature database.
	avSigDB := sigdb.New(avCfg.SigDir)
	avSigDB.SetLayers(hashDB, matcherLayer, yaraLayer)

	// Scan cache.
	avCache := cache.New(avCfg.CacheSize, avCfg.CacheTTL)

	// ── SafeGate scanner orchestrator ────────────────────────────────
	// Full pipeline: metadata → MIME → SVG → macro → archive → native AV.
	// Metrics from this orchestrator power the Object Storage SafeGate dashboard.
	scannerCfg := scanner.LoadConfigFromEnv()
	scanOrch := scanner.NewOrchestratorWithConfig(scannerCfg,
		metadata.NewScannerWithConfig(scannerCfg.MaxFileSize, scannerCfg.NullByteSampleSize, scannerCfg.MaxFilenameLength),
		mimetype.NewScanner(scannerCfg.AllowedMIMETypes),
		svg.NewScanner(),
		macro.NewScanner(),
		archivescan.NewScannerWithLimits(scannerCfg.ArchiveMaxDepth, scannerCfg.ArchiveMaxDecompressedSize, scannerCfg.ArchiveCompressionRatioLimit, scannerCfg.ArchiveMaxFiles),
		nativeav.NewScanner(avEngine),
	)

	handler := admin.NewHandler(bucketStore, backend, tenantMgr, bucketCtrl, accessCtrl, sigV4PresignSigner{}, metricsCollector, auditLog, endpoint, avEngine, avCache, scanOrch)

	return &System{
		Backend:         backend,
		Store:           bucketStore,
		Controller:      bucketCtrl,
		Tenant:          tenantMgr,
		Policy:          policyCtrl,
		Access:          accessCtrl,
		Handler:         handler,
		Metrics:         metricsCollector,
		AuditLog:        auditLog,
		Config:          cfg,
		AVEngine:        avEngine,
		AVSigDB:         avSigDB,
		AVCache:         avCache,
		ScanOrch:        scanOrch,
		iamIssuer:       issuer,
		iamRevokedStore: revokedStore,
	}, nil
}

// RegisterRoutes mounts all object storage endpoints on the provided router group.
// When an IAM issuer is configured, the storage route group is wrapped with JWTAuth
// middleware so that downstream handlers can extract iam_claims for access control.
func (s *System) RegisterRoutes(rg *gin.RouterGroup) error {
	ConfigurePresignedMiddleware(s.Access.ResolveAccessKey, s.Config.PresignRateLimitPerMinute)

	// JWT auth is resolved at request time (not route-registration time)
	// so that the IAM issuer can be set after routes are registered
	// (deferred IAM init in Raft mode).
	rg.Use(func(c *gin.Context) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Request = r

			if info, ok := getPresignedRequestInfo(r.Context()); ok {
				c.Set("storage_presigned_request", true)
				c.Set("storage_presigned_access_key", info.AccessKeyID)
				c.Set("storage_presigned_bucket", info.Bucket)
				c.Set("storage_presigned_key", info.ObjectKey)
				c.Set("storage_presigned_tenant", info.TenantID)
				c.Set("storage_presigned_user", info.UserID)
			}

			if !c.GetBool("storage_presigned_request") && s.iamIssuer != nil {
				jwtAuth := iamMiddleware.JWTAuth(s.iamIssuer, s.iamRevokedStore)
				jwtAuth(c)
				return
			}

			c.Next()
		})

		PresignedOrIAMMiddleware(next).ServeHTTP(c.Writer, c.Request)
	})

	if s.iamIssuer != nil {
		logging.Z().Info("✅ Storage: secure presigned/IAM middleware attached to storage routes")
	}
	s.Handler.RegisterRoutes(rg)
	return nil
}

type sigV4PresignSigner struct{}

func (sigV4PresignSigner) Generate(method, bucket, objectKey string, expiry time.Duration, accessKey, secretKey, host string) (string, error) {
	return GeneratePresignedURLWithHost(method, bucket, objectKey, expiry, accessKey, secretKey, host)
}

// Name returns the module identifier.
func (s *System) Name() string { return "storage" }

// Start begins the reconciliation controller.
func (s *System) Start(ctx context.Context) error {
	// Initialize antivirus signature database.
	if s.AVSigDB != nil {
		if _, err := s.AVSigDB.Init(); err != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  Storage: antivirus sigdb init error: %v", err))
		}
	}

	// Start antivirus engine.
	if s.AVEngine != nil {
		s.AVEngine.Start()
	}

	s.Controller.Start(ctx)
	logging.Z().Info(fmt.Sprint("✅ Storage: module started (native backend, IAM-integrated, antivirus-enabled)"))
	return nil
}

// Stop gracefully shuts down the storage module.
func (s *System) Stop() error {
	// Shutdown antivirus engine.
	if s.AVEngine != nil {
		s.AVEngine.Shutdown(context.Background())
	}

	s.Controller.Stop()
	logging.Z().Info("✅ Storage: module stopped")
	return nil
}
