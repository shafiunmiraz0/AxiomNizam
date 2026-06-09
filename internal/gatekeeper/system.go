package gatekeeper

import (
	"example.com/axiomnizam/internal/logging"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"example.com/axiomnizam/internal/gatekeeper/audit"
	"example.com/axiomnizam/internal/gatekeeper/backupcodes"
	gkcache "example.com/axiomnizam/internal/gatekeeper/cache"
	"example.com/axiomnizam/internal/gatekeeper/challenge"
	"example.com/axiomnizam/internal/gatekeeper/config"
	gkcontroller "example.com/axiomnizam/internal/gatekeeper/controller"
	"example.com/axiomnizam/internal/gatekeeper/enrollment"
	"example.com/axiomnizam/internal/gatekeeper/handlers"
	"example.com/axiomnizam/internal/gatekeeper/metrics"
	"example.com/axiomnizam/internal/gatekeeper/models"
	"example.com/axiomnizam/internal/gatekeeper/pgstore"
	"example.com/axiomnizam/internal/gatekeeper/policy"
	"example.com/axiomnizam/internal/gatekeeper/repositories"
	"example.com/axiomnizam/internal/gatekeeper/risk"
	"example.com/axiomnizam/internal/gatekeeper/totp"
	"example.com/axiomnizam/internal/gatekeeper/trusteddevices"
	"example.com/axiomnizam/internal/gatekeeper/webauthn"
	platformstore "example.com/axiomnizam/internal/platform/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// System holds the fully initialized Gatekeeper 2FA module.
// Follows the storage.System pattern with KVStore persistence support.
type System struct {
	cfg *config.Config
	db  *sql.DB

	// Raft/etcd KV store for distributed state persistence
	kvStore platformstore.KVStore

	// Repositories (PostgreSQL-backed)
	factorRepo     repositories.FactorRepository
	challengeRepo  repositories.ChallengeRepository
	backupCodeRepo repositories.BackupCodeRepository
	trustedDevRepo repositories.TrustedDeviceRepository

	// Services
	TOTPService       *totp.Service
	ChallengeService  *challenge.Service
	EnrollmentService *enrollment.Service
	BackupCodeService *backupcodes.Service
	DeviceService     *trusteddevices.Service
	PolicyService     *policy.Engine
	RiskService       *risk.Engine
	WebAuthnService   *webauthn.Service

	// Controllers/Reconcilers (K8s-style)
	FactorController *gkcontroller.FactorReconciler
	ctrlMgr          *gkcontroller.Manager

	// Infrastructure
	auditLog  *audit.Logger
	collector *metrics.Collector
	cache     gkcache.Store

	// HTTP Handler
	httpHandler *handlers.HTTPHandler
}

// NewSystem initializes the Gatekeeper 2FA module.
func NewSystem(gormDB *gorm.DB) (*System, error) {
	cfg := config.DefaultConfig()
	cfg.LoadFromEnv()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("gatekeeper config validation: %w", err)
	}

	// Guard against nil DB — PostgreSQL may be unavailable (e.g., TLS mismatch, connection refused).
	if gormDB == nil {
		logging.Z().Warn("⚠️  Gatekeeper: PostgreSQL unavailable — running in degraded mode (no persistence)")
		s := &System{cfg: cfg, db: nil}
		if err := s.initialize(); err != nil {
			return nil, err
		}
		logging.Z().Info("⚠️  Gatekeeper 2FA module initialized in degraded mode")
		return s, nil
	}

	// Auto-migrate Gatekeeper tables (same pattern as IAM pgstore.New)
	if err := pgstore.MigrateGatekeeperTables(gormDB); err != nil {
		return nil, fmt.Errorf("gatekeeper migration: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}

	s := &System{
		cfg: cfg,
		db:  sqlDB,
	}

	if err := s.initialize(); err != nil {
		return nil, err
	}

	logging.Z().Info("✅ Gatekeeper 2FA module initialized")
	return s, nil
}

// NewSystemWithConfig initializes Gatekeeper with custom config.
func NewSystemWithConfig(gormDB *gorm.DB, cfg *config.Config) (*System, error) {
	cfg.LoadFromEnv()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("gatekeeper config validation: %w", err)
	}

	if err := pgstore.MigrateGatekeeperTables(gormDB); err != nil {
		return nil, fmt.Errorf("gatekeeper migration: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}

	s := &System{
		cfg: cfg,
		db:  sqlDB,
	}

	if err := s.initialize(); err != nil {
		return nil, err
	}

	logging.Z().Info("✅ Gatekeeper 2FA module initialized with custom config")
	return s, nil
}

// NewSystemWithKVStore initializes Gatekeeper with both PostgreSQL and Raft KV store.
// Used when running in Raft mode for distributed state persistence.
func NewSystemWithKVStore(db *sql.DB, kvStore platformstore.KVStore) (*System, error) {
	cfg := config.DefaultConfig()
	cfg.LoadFromEnv()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("gatekeeper config validation: %w", err)
	}

	s := &System{
		cfg:     cfg,
		db:      db,
		kvStore: kvStore,
	}

	if err := s.initialize(); err != nil {
		return nil, err
	}

	// Load persisted state from KV store
	if kvStore != nil {
		s.loadFromKVStore()
	}

	logging.Z().Info("✅ Gatekeeper 2FA module initialized with KV store")
	return s, nil
}

// SetKVStore wires the KVStore-backed persistence into Gatekeeper.
// Called in Raft mode after the Raft KV becomes available.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	s.kvStore = kv

	// Configure persistence on metrics collector
	if s.collector != nil {
		s.collector.ConfigureKVPersistence(kv)
	}

	// Configure persistence on audit logger
	if s.auditLog != nil {
		s.auditLog.ConfigureKVPersistence(kv)
	}

	logging.Z().Info("✅ Gatekeeper: KVStore persistence configured (Raft mode)")
}

// loadFromKVStore loads persisted state from Raft KV on startup.
func (s *System) loadFromKVStore() {
	if s.kvStore == nil {
		return
	}

	// Load factors from KV store
	if s.factorRepo != nil {
		factors, err := s.loadFactorsFromKV()
		if err != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  Gatekeeper: failed to load factors from KV: %v", err))
		} else if len(factors) > 0 {
			logging.Z().Info(fmt.Sprintf("✅ Gatekeeper: loaded %d factors from KV store", len(factors)))
		}
	}

	// Load policies from KV store
	if s.PolicyService != nil {
		policies, err := s.loadPoliciesFromKV()
		if err != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  Gatekeeper: failed to load policies from KV: %v", err))
		} else if len(policies) > 0 {
			logging.Z().Info(fmt.Sprintf("✅ Gatekeeper: loaded %d policies from KV store", len(policies)))
		}
	}
}

func (s *System) loadFactorsFromKV() ([]*models.Factor, error) {
	if s.kvStore == nil {
		return nil, nil
	}

	ctx := context.Background()
	entries, err := s.kvStore.List(ctx, "gatekeeper:factors:")
	if err != nil {
		return nil, err
	}

	// Parse and restore factors
	var factors []*models.Factor
	for _, value := range entries {
		var factor models.Factor
		if err := json.Unmarshal([]byte(value), &factor); err != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  Gatekeeper: skipping malformed factor KV entry: %v", err))
			continue
		}
		factors = append(factors, &factor)
	}

	return factors, nil
}

func (s *System) loadPoliciesFromKV() ([]*models.MFAPolicy, error) {
	if s.kvStore == nil {
		return nil, nil
	}

	ctx := context.Background()
	entries, err := s.kvStore.List(ctx, "gatekeeper:policies:")
	if err != nil {
		return nil, err
	}

	var policies []*models.MFAPolicy
	for _, value := range entries {
		var policy models.MFAPolicy
		if err := json.Unmarshal([]byte(value), &policy); err != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  Gatekeeper: skipping malformed policy KV entry: %v", err))
			continue
		}
		policies = append(policies, &policy)
	}

	return policies, nil
}

// initialize wires all dependencies.
func (s *System) initialize() error {
	// 1. Initialize repositories
	s.factorRepo = pgstore.NewFactorRepository(s.db)
	s.challengeRepo = pgstore.NewChallengeRepository(s.db)
	s.backupCodeRepo = pgstore.NewBackupCodeRepository(s.db)
	s.trustedDevRepo = pgstore.NewTrustedDeviceRepository(s.db)

	// 2. Initialize cache
	s.cache = gkcache.NewInMemoryStore()

	// 4. Initialize TOTP service
	s.TOTPService = totp.NewService(
		totp.NewSecretGenerator(),
		totp.NewValidator(),
		totp.NewIssuerProvider(),
		totp.NewRealClock(),
	)

	// 5. Initialize challenge service
	s.ChallengeService = challenge.NewService(
		s.challengeRepo,
		s.factorRepo,
		s.TOTPService,
		challenge.NewRealClock(),
		s.cfg.EncryptionKey,
	)

	// 6. Initialize enrollment service
	s.EnrollmentService = enrollment.NewService(
		s.factorRepo,
		s.backupCodeRepo,
		s.TOTPService,
		s.cfg.EncryptionKey,
	)

	// 7. Initialize backup code service
	s.BackupCodeService = backupcodes.NewService(s.backupCodeRepo)

	// 8. Initialize trusted device service
	s.DeviceService = trusteddevices.NewService(
		s.trustedDevRepo,
		trusteddevices.NewRealClock(),
	)

	// 9. Initialize policy engine with default rules
	s.PolicyService = policy.NewEngine(
		&policy.DefaultEvaluator{},
		[]policy.Rule{
			&policy.SensitiveResourceRule{
				ResourceTypes: []string{"sensitive-operation", "admin", "billing"},
			},
			&policy.HighRiskBlockRule{
				Threshold: 90,
			},
		},
	)

	// 10. Initialize risk engine
	s.RiskService = risk.NewEngine(&risk.DefaultScorer{})

	// 10b. Initialize WebAuthn service (FIDO2 / security keys)
	rpID := os.Getenv("WEBAUTHN_RP_ID")
	if rpID == "" {
		rpID = "localhost"
	}
	rpOrigin := os.Getenv("WEBAUTHN_RP_ORIGIN")
	if rpOrigin == "" {
		rpOrigin = "http://localhost:8000"
	}
	credStore := pgstore.NewWebAuthnCredentialRepository(s.db)
	s.WebAuthnService = webauthn.NewService(rpID, rpOrigin, credStore)

	// 11. Initialize audit logging
	var auditBackend audit.AuditBackend = audit.NewInMemoryBackend()
	if s.db != nil {
		auditBackend = pgstore.NewAuditRepository(s.db)
	}
	s.auditLog = audit.NewLogger(auditBackend)

	// 12. Initialize metrics
	s.collector = metrics.NewCollector()

	// 13. Initialize Factor Controller (K8s-style reconciler)
	s.FactorController = gkcontroller.NewFactorReconciler(
		s.factorRepo,
		s.challengeRepo,
	)
	s.ctrlMgr = gkcontroller.NewManager(s.FactorController)

	// 14. Initialize HTTP handler with service wrappers
	s.httpHandler = handlers.NewHTTPHandler(
		wrapEnrollmentService(s.EnrollmentService),
		wrapChallengeService(s.ChallengeService),
		wrapFactorService(s.factorRepo),
		wrapPolicyService(s.PolicyService),
		wrapRiskService(s.RiskService),
		wrapTrustedDeviceService(s.DeviceService),
		wrapBackupCodeService(s.BackupCodeService),
		wrapWebAuthnService(s.WebAuthnService),
	)

	return nil
}

// RegisterRoutes registers all HTTP routes for the 2FA module on the given router group.
// The caller should apply auth middleware to the group before passing it.
func (s *System) RegisterRoutes(api *gin.RouterGroup) error {
	s.httpHandler.RegisterRoutes(api)
	logging.Z().Info("✅ Gatekeeper routes registered at /api/v1/mfa")
	return nil
}

// StartControllers starts the K8s-style reconciliation loops.
func (s *System) StartControllers(ctx context.Context) {
	if s.db == nil {
		logging.Z().Info("⚠️  Gatekeeper: skipping controller manager (no PostgreSQL)")
		return
	}
	if s.ctrlMgr != nil {
		s.ctrlMgr.Start(ctx)
		logging.Z().Info("✅ Gatekeeper: Controller manager started")
	}
}

// Name returns the module identifier.
func (s *System) Name() string { return "gatekeeper" }

// Start initializes the module — starts controllers, schedulers, background workers.
func (s *System) Start(ctx context.Context) error {
	s.StartControllers(ctx)
	return nil
}

// Stop gracefully shuts down the module.
func (s *System) Stop() error {
	logging.Z().Info("Gatekeeper: stopping")
	return nil
}

// Config returns the module configuration.
func (s *System) Config() *config.Config {
	return s.cfg
}

// AuditLogger returns the audit logger.
func (s *System) AuditLogger() *audit.Logger {
	return s.auditLog
}

// MetricsCollector returns the metrics collector.
func (s *System) MetricsCollector() *metrics.Collector {
	return s.collector
}

// FactorRepository returns the factor repository for direct access.
func (s *System) FactorRepository() repositories.FactorRepository {
	return s.factorRepo
}

// ChallengeRepository returns the challenge repository for direct access.
func (s *System) ChallengeRepository() repositories.ChallengeRepository {
	return s.challengeRepo
}

// KVStore returns the KV store if configured.
func (s *System) KVStore() platformstore.KVStore {
	return s.kvStore
}
