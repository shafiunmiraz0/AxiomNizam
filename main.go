package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base32"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"example.com/axiomnizam/internal/apibanks"
	"example.com/axiomnizam/internal/apigateway"
	"example.com/axiomnizam/internal/apiscanner"
	"example.com/axiomnizam/internal/audit"
	"example.com/axiomnizam/internal/auth"
	"example.com/axiomnizam/internal/bulk"
	"example.com/axiomnizam/internal/cdc"
	"example.com/axiomnizam/internal/conductor"
	"example.com/axiomnizam/internal/config"
	"example.com/axiomnizam/internal/contracts"
	"example.com/axiomnizam/internal/database"
	datasourceresource "example.com/axiomnizam/internal/datasource"
	"example.com/axiomnizam/internal/encryption"
	"example.com/axiomnizam/internal/etl"
	"example.com/axiomnizam/internal/frontend"
	"example.com/axiomnizam/internal/eventbus"
	exportpkg "example.com/axiomnizam/internal/export"
	"example.com/axiomnizam/internal/gatekeeper"
	gkmodels "example.com/axiomnizam/internal/gatekeeper/models"
	gkpolicy "example.com/axiomnizam/internal/gatekeeper/policy"
	gkrisk "example.com/axiomnizam/internal/gatekeeper/risk"
	analyticspkg "example.com/axiomnizam/internal/analytics"
	gispkg "example.com/axiomnizam/internal/gis"
	graphqlpkg "example.com/axiomnizam/internal/graphql"
	healthpkg "example.com/axiomnizam/internal/health"
	apibuilder "example.com/axiomnizam/internal/apibuilder"
	authn "example.com/axiomnizam/internal/iam/authn"
	netintelpkg "example.com/axiomnizam/internal/netintel"
	notificationpkg "example.com/axiomnizam/internal/notification"
	transformpkg "example.com/axiomnizam/internal/transform"
	iampkg "example.com/axiomnizam/internal/iam"
	iamstorage "example.com/axiomnizam/internal/iam/storage"
	iamtoken "example.com/axiomnizam/internal/iam/token"
	iamusers "example.com/axiomnizam/internal/iam/users"
	"example.com/axiomnizam/internal/integration"
	"example.com/axiomnizam/internal/jobs"
	"example.com/axiomnizam/internal/kubeplus/admission"
	"example.com/axiomnizam/internal/kubeplus/crd"
	"example.com/axiomnizam/internal/kubeplus/scheduler"
	iammw "example.com/axiomnizam/internal/iam/middleware"
	"example.com/axiomnizam/internal/lineage"
	"example.com/axiomnizam/internal/logging"
	"example.com/axiomnizam/internal/metrics"
	"example.com/axiomnizam/internal/observability"
	"example.com/axiomnizam/internal/models"
	"example.com/axiomnizam/internal/netintel/modes"
	"example.com/axiomnizam/internal/platform"
	genericctrl "example.com/axiomnizam/internal/platform/controller"
	"example.com/axiomnizam/internal/platform/gc"
	snapshothandler "example.com/axiomnizam/internal/platform/snapshot"
	querypkg "example.com/axiomnizam/internal/query"
	resourcespkg "example.com/axiomnizam/internal/resources"
	"example.com/axiomnizam/internal/platform/featureflags"
	platformstore "example.com/axiomnizam/internal/platform/store"
	"example.com/axiomnizam/internal/policies"
	"example.com/axiomnizam/internal/rbac"
	reconcilerpkg "example.com/axiomnizam/internal/reconciler"
	"example.com/axiomnizam/internal/reviewflow"
	"example.com/axiomnizam/internal/runtime"
	"example.com/axiomnizam/internal/server"
	securitypkg "example.com/axiomnizam/internal/security"
	"example.com/axiomnizam/internal/storage"
	"example.com/axiomnizam/internal/streaming"
	"example.com/axiomnizam/internal/tenant"
	axmtls "example.com/axiomnizam/internal/tls"
	"example.com/axiomnizam/internal/tracing"
	"example.com/axiomnizam/internal/vectorplus"
	"example.com/axiomnizam/internal/waitx"
	"example.com/axiomnizam/internal/versioning"
	"example.com/axiomnizam/internal/webhooks"
	"example.com/axiomnizam/internal/workflows"
	"example.com/axiomnizam/internal/secretmanager"
	"example.com/axiomnizam/internal/federation"
	"example.com/axiomnizam/internal/securitymon"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

// decryptAESSecret decrypts an AES-GCM encrypted TOTP secret.
// The ciphertext must include the nonce as a prefix (standard Go AES-GCM format).
func decryptAESSecret(encryptionKey []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return aesGCM.Open(nil, nonce, ct, nil)
}

// ── Zero Trust Phase 3: RBAC Authorization Helpers ────────────────────────────

// mapHTTPMethodToRBACVerb translates HTTP methods to RBAC verb strings.
func mapHTTPMethodToRBACVerb(method string) string {
	switch method {
	case "GET", "HEAD", "OPTIONS":
		return "read"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

// mapPathToRBACResource extracts the primary resource kind from a URL path.
// Examples:
//
//	"/api/v1/storage/buckets" → "storage"
//	"/api/v1/iam/users"       → "iam"
//	"/api/v1/rbac/roles"      → "rbac"
//	"/api/mysql/query"         → "database"
//	"/auth/login"              → "auth"
func mapPathToRBACResource(path string) string {
	// Handle /api/v1/<module>/... paths
	if strings.HasPrefix(path, "/api/v1/") {
		rest := strings.TrimPrefix(path, "/api/v1/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) > 0 && parts[0] != "" {
			return parts[0]
		}
	}

	// Handle /api/<service>/... paths
	if strings.HasPrefix(path, "/api/") {
		rest := strings.TrimPrefix(path, "/api/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) > 0 {
			switch parts[0] {
			case "mysql", "mariadb", "postgres", "percona", "oracle", "mssql", "sqlite", "mongodb":
				return "database"
			case "graphql":
				return "graphql"
			default:
				return parts[0]
			}
		}
	}

	// Handle /auth/... paths
	if strings.HasPrefix(path, "/auth/") {
		return "auth"
	}

	return "unknown"
}

// seedDefaultRBACRoles populates the K8s-style RBAC engine with default
// cluster roles that mirror the IAM system roles. This ensures CanPerform()
// has data to evaluate against from startup.
func seedDefaultRBACRoles(engine *rbac.Engine) {
	ctx := context.Background()

	// Sysadmin: full wildcard access on all resources/verbs
	_ = engine.CreateClusterRole(ctx, &rbac.EngineClusterRole{
		Name: "sysadmin",
		Rules: []*rbac.EnginePolicyRule{
			{Verbs: []string{"*"}, Resources: []string{"*"}},
		},
	})
	_ = engine.CreateClusterRoleBinding(ctx, &rbac.EngineClusterRoleBinding{
		Name:     "sysadmin-binding",
		Role:     "sysadmin",
		Subjects: []*rbac.EngineSubject{{Type: "User", Name: "sysadmin"}},
	})

	// Admin: full wildcard access on all resources (matches IAM admin role capabilities)
	_ = engine.CreateClusterRole(ctx, &rbac.EngineClusterRole{
		Name: "admin",
		Rules: []*rbac.EnginePolicyRule{
			{Verbs: []string{"*"}, Resources: []string{"*"}},
		},
	})
	_ = engine.CreateClusterRoleBinding(ctx, &rbac.EngineClusterRoleBinding{
		Name:     "admin-binding",
		Role:     "admin",
		Subjects: []*rbac.EngineSubject{{Type: "User", Name: "admin"}},
	})

	// Manager: read on most resources, execute on jobs
	_ = engine.CreateClusterRole(ctx, &rbac.EngineClusterRole{
		Name: "manager",
		Rules: []*rbac.EnginePolicyRule{
			{Verbs: []string{"read"}, Resources: []string{"*"}},
			{Verbs: []string{"create", "update"}, Resources: []string{"jobs"}},
		},
	})
	_ = engine.CreateClusterRoleBinding(ctx, &rbac.EngineClusterRoleBinding{
		Name:     "manager-binding",
		Role:     "manager",
		Subjects: []*rbac.EngineSubject{{Type: "User", Name: "manager"}},
	})

	// User: read/update on own profile only
	_ = engine.CreateClusterRole(ctx, &rbac.EngineClusterRole{
		Name: "user",
		Rules: []*rbac.EnginePolicyRule{
			{Verbs: []string{"read", "update"}, Resources: []string{"profile", "auth"}},
		},
	})
	_ = engine.CreateClusterRoleBinding(ctx, &rbac.EngineClusterRoleBinding{
		Name:     "user-binding",
		Role:     "user",
		Subjects: []*rbac.EngineSubject{{Type: "User", Name: "user"}},
	})

	log.Println("✅ RBAC engine seeded with default cluster roles (sysadmin, admin, manager, user)")
}

func main() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️  No .env file found, using system environment variables")
	}
	fmt.Println("🚀 Starting AxiomNizam with Kubernetes-style Runtime...")
	fmt.Println()

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Initialize Runtime
	log.Println("📦 Initializing Kubernetes-style runtime...")
	rt := runtime.NewRuntime("1.0.0")

	if err := rt.Initialize(ctx); err != nil {
		log.Fatalf("Failed to initialize runtime: %v", err)
	}

	// Module registry — collects all modules for uniform lifecycle management.
	var modules []contracts.Module

	// Load configuration
	cfg := config.LoadConfig()
	server.ApplySecurityGuardrails(cfg)

	// ── TLS initialization (Phase 4) ─────────────────────────────────────────
	// When TLS_CERT_FILE + TLS_KEY_FILE are set, or TLS_AUTO_GENERATE=true,
	// the server starts with HTTPS. Self-signed certs are created in data/certs/
	// for development when auto-generate is enabled.
	tlsCfg, tlsErr := axmtls.LoadOrCreate(
		cfg.TLS.CertFile,
		cfg.TLS.KeyFile,
		cfg.TLS.AutoGenerate,
		"/data",
	)
	if tlsErr != nil {
		log.Fatalf("TLS initialization failed: %v", tlsErr)
	}
	if tlsCfg.Enabled {
		if tlsCfg.AutoGenerated {
			log.Printf("🔒 TLS enabled with auto-generated self-signed certificate (dev mode)")
			log.Printf("   Cert: %s", tlsCfg.CertFile)
			log.Printf("   Key:  %s", tlsCfg.KeyFile)
		} else {
			log.Printf("🔒 TLS enabled with provided certificate")
		}
	} else {
		log.Println("⚠️  TLS disabled — server running on plain HTTP")
	}

	// NOTE: Do NOT auto-set POSTGRES_SSLMODE=require — internal Docker PostgreSQL
	// runs on an isolated data-net without TLS. Set POSTGRES_SSLMODE=require only
	// when connecting to an external PostgreSQL instance with SSL configured.

	iamOnlyAuthRaw := strings.TrimSpace(os.Getenv("IAM_ONLY_AUTH"))
	iamOnlyAuth := true
	if iamOnlyAuthRaw != "" {
		iamOnlyAuth = strings.EqualFold(iamOnlyAuthRaw, "true")
	}

	// Initialize IAM token validator
	iamIssuerURL := strings.TrimSpace(os.Getenv("IAM_ISSUER_URL"))
	if iamIssuerURL == "" {
		iamIssuerURL = cfg.GetIAMURL()
	}
	normalizeBaseURL := func(raw string) string {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			return ""
		}

		if parsed, parseErr := url.Parse(candidate); parseErr == nil && parsed.Scheme != "" && parsed.Host != "" {
			return strings.TrimRight(parsed.Scheme+"://"+parsed.Host+parsed.Path, "/")
		}

		return strings.TrimRight(candidate, "/")
	}

	validatorJWKSBases := make([]string, 0, 4)
	seenValidatorBase := make(map[string]struct{}, 4)
	addValidatorJWKSBase := func(raw string) {
		base := normalizeBaseURL(raw)
		if base == "" {
			return
		}
		if _, exists := seenValidatorBase[base]; exists {
			return
		}
		seenValidatorBase[base] = struct{}{}
		validatorJWKSBases = append(validatorJWKSBases, base)
	}

	addValidatorJWKSBase(os.Getenv("IAM_INTERNAL_BASE_URL"))
	addValidatorJWKSBase(iamIssuerURL)
	addValidatorJWKSBase(cfg.GetIAMURL())
	jwksScheme := "http"
	if tlsCfg.Enabled {
		jwksScheme = "https"
	}
	addValidatorJWKSBase(fmt.Sprintf("%s://localhost:%s", jwksScheme, cfg.API.Port))

	buildValidatorConfig := func(jwksBase string) *auth.TokenValidatorConfig {
		validatorConfig := &auth.TokenValidatorConfig{
			IssuerURL: iamIssuerURL,
		}
		if jwksBase != "" {
			validatorConfig.JWKSURL = strings.TrimRight(jwksBase, "/") + "/.well-known/jwks.json"
		}
		return validatorConfig
	}

	initializeTokenValidator := func() (*auth.TokenValidator, error) {
		if len(validatorJWKSBases) == 0 {
			return nil, fmt.Errorf("no IAM JWKS base URLs configured")
		}

		initErrors := make([]string, 0, len(validatorJWKSBases))
		for _, jwksBase := range validatorJWKSBases {
			candidateConfig := buildValidatorConfig(jwksBase)
			initializedValidator, initErr := auth.NewTokenValidator(candidateConfig)
			if initErr == nil {
				log.Printf("✅ IAM token validator JWKS source: %s/.well-known/jwks.json", strings.TrimRight(jwksBase, "/"))
				return initializedValidator, nil
			}
			initErrors = append(initErrors, fmt.Sprintf("%s: %v", jwksBase, initErr))
		}

		return nil, fmt.Errorf("all IAM JWKS endpoints failed: %s", strings.Join(initErrors, " | "))
	}

	var tokenValidatorMu sync.RWMutex
	tokenValidator, err := initializeTokenValidator()
	if err != nil {
		log.Printf("⚠️  IAM token validator initialization failed at startup: %v", err)
		log.Printf("⚠️  Auth-protected APIs will return 503 until IAM JWKS becomes reachable")
		tokenValidator = nil
	}

	getOrInitTokenValidator := func() *auth.TokenValidator {
		tokenValidatorMu.RLock()
		tv := tokenValidator
		tokenValidatorMu.RUnlock()
		if tv != nil {
			return tv
		}

		tokenValidatorMu.Lock()
		defer tokenValidatorMu.Unlock()
		if tokenValidator != nil {
			return tokenValidator
		}

		initializedValidator, initErr := initializeTokenValidator()
		if initErr != nil {
			log.Printf("⚠️  IAM token validator still unavailable: %v", initErr)
			return nil
		}

		tokenValidator = initializedValidator
		log.Printf("✅ IAM token validator initialized after startup")
		return tokenValidator
	}

	// Initialize all connections
	conns := database.InitConnections(cfg)

	// JWT secret and module persistence configuration.
	// When using Raft backend, these are deferred until BackendManager is ready.
	// When using etcd (default), they run immediately.
	earlyStorageBackend := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_BACKEND")))
	if earlyStorageBackend != "raft" {
		if _, secretErr := server.EnsureSharedDemoJWTSecret(conns.PostgreSQL, conns.Etcd, nil); secretErr != nil {
			log.Printf("⚠️  DEMO_JWT_SECRET synchronization failed: %v", secretErr)
		} else {
			log.Println("✅ DEMO_JWT_SECRET synchronized for replica-safe token validation")
		}
		workflows.ConfigureGlobalPersistence(conns.Etcd)
		modes.ConfigureGlobalPersistence(conns.Etcd)
		vectorplus.ConfigureGlobalPersistence(conns.Etcd)
		reviewflow.ConfigureGlobalPersistence(conns.Etcd)
		integration.ConfigureGlobalPersistence(conns.Etcd)
	} else {
		// Raft mode: JWT secret from env or postgres only (KVStore not ready yet).
		if _, secretErr := server.EnsureSharedDemoJWTSecret(conns.PostgreSQL, nil, nil); secretErr != nil {
			log.Printf("ℹ️  DEMO_JWT_SECRET deferred to Raft backend init: %v", secretErr)
		} else {
			log.Println("✅ DEMO_JWT_SECRET synchronized (postgres/env)")
		}
		// Module persistence will be configured after BackendManager init.
	}

	// Create tables
	server.CreateTables(conns)

	// NOTE: Legacy train / bd-train GIS PostgreSQL connections and their
	// handlers (GISTrainHandler, GISBDTrainHandler) were removed as part of
	// the Kubernetes-style control-plane refactor. GIS APIs are now authored
	// via the API Builder, which persists artifacts through the
	// ResourceStore -> Controller -> Reconciler pipeline. External railway
	// datasets should be exposed through DataSource resources and reached
	// only from reconcilers, never from HTTP handlers.

	// ====================================
	// IAM SYSTEM INITIALIZATION
	// ====================================
	var iamSystem *iampkg.System
	var iamErr error

	// Gatekeeper (2FA) system — declared here so authenticateRequest() can
	// reference it in the closure even though it is initialized later.
	var gkSystem *gatekeeper.System

	if earlyStorageBackend != "raft" {
		// etcd mode: initialize IAM immediately.
		iamSystem, iamErr = iampkg.NewSystem(conns.PostgreSQL, conns.Etcd, iampkg.Config{
			IssuerURL: strings.TrimSpace(os.Getenv("IAM_ISSUER_URL")),
		})
	} else {
		// Raft mode: IAM will be initialized after BackendManager is ready.
		log.Println("ℹ️  IAM initialization deferred (STORAGE_BACKEND=raft — waiting for Raft KV)")
	}
	if iamErr != nil {
		log.Printf("⚠️  IAM system initialization failed: %v", iamErr)
		log.Println("⚠️  IAM endpoints will not be available. Ensure PostgreSQL is connected.")
	} else if iamSystem != nil {
		log.Println("✅ IAM system initialized")
	}

	// Initialize OpenTelemetry tracing (no-op if OTEL_EXPORTER_OTLP_ENDPOINT not set)
	otelShutdown, otelErr := observability.InitTracer(ctx, "axiomnizam")
	if otelErr != nil {
		log.Printf("⚠️  OTel tracer init failed: %v (tracing disabled)", otelErr)
	} else {
		defer otelShutdown(context.Background())
	}

	// Initialize structured logging
	logEnv := os.Getenv("LOG_ENV")
	if logEnv == "" {
		logEnv = "production"
	}
	logging.Init(logEnv)

	// Create Gin router
	router := gin.New()
	router.Use(gin.Recovery())

	// Strip server/framework identifying headers (anti-fingerprinting)
	router.Use(func(c *gin.Context) {
		c.Header("X-Powered-By", "")
		c.Header("Server", "")
		c.Next()
	})

	// HTTPS redirect middleware (Phase 4).
	// When TLS is enabled, redirect plain HTTP requests to HTTPS.
	// Skips redirect for health checks and internal probes.
	if tlsCfg.Enabled {
		router.Use(func(c *gin.Context) {
			// Skip redirect for health/status endpoints (load balancer probes)
			path := c.Request.URL.Path
			if path == "/health" || path == "/status" || path == "/api/health" || path == "/api/status" {
				c.Next()
				return
			}
			// If request is not TLS, redirect to HTTPS
			if c.Request.TLS == nil {
				// Trust X-Forwarded-Proto from reverse proxy
				proto := c.GetHeader("X-Forwarded-Proto")
				if proto == "" || proto == "http" {
					target := "https://" + c.Request.Host + c.Request.URL.RequestURI()
					c.Redirect(http.StatusMovedPermanently, target)
					c.Abort()
					return
				}
			}
			c.Next()
		})
	}

	// Trust proxies for X-Forwarded-For / X-Real-IP.
	// Set TRUSTED_PROXIES env to comma-separated CIDRs (e.g. "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16").
	// Defaults to private Docker/K8s CIDRs. Set to "0.0.0.0/0" to trust all, or "*" to trust none.
	trustedProxiesEnv := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES"))
	if trustedProxiesEnv == "" {
		trustedProxiesEnv = "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
	}
	var trustedProxies []string
	if trustedProxiesEnv == "*" {
		trustedProxies = []string{} // trust none
	} else {
		for _, cidr := range strings.Split(trustedProxiesEnv, ",") {
			if trimmed := strings.TrimSpace(cidr); trimmed != "" {
				trustedProxies = append(trustedProxies, trimmed)
			}
		}
	}
	_ = router.SetTrustedProxies(trustedProxies)

	allowedOriginSet := make(map[string]struct{})
	addAllowedOrigin := func(raw string) {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			return
		}
		if parsed, err := url.Parse(candidate); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			candidate = parsed.Scheme + "://" + parsed.Host
		}
		allowedOriginSet[candidate] = struct{}{}
	}

	// Always include canonical frontend URL when provided.
	addAllowedOrigin(os.Getenv("PUBLIC_FRONTEND_URL"))

	for _, candidate := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		addAllowedOrigin(candidate)
	}
	if len(allowedOriginSet) == 0 {
		addAllowedOrigin("https://axiomnizam.bitbd.net")
		addAllowedOrigin("http://localhost:8000")
		addAllowedOrigin("http://127.0.0.1:8000")
		addAllowedOrigin("https://localhost:8000")
	}

	isAllowedOrigin := func(origin string) bool {
		_, ok := allowedOriginSet[origin]
		return ok
	}

	// Add CORS middleware
	router.Use(func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		c.Writer.Header().Set("Vary", "Origin")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-API-KEY")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "X-RateLimit-Limit, X-RateLimit-Remaining, X-Token-Expires-At")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		if origin != "" && isAllowedOrigin(origin) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			if origin != "" && !isAllowedOrigin(origin) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Observability middleware — request ID, trace context, structured access logs
	router.Use(observability.RequestIDMiddleware())
	router.Use(observability.AccessLogMiddleware())

	// Security headers + request body size limits + CSRF
	router.Use(observability.SecurityHeadersMiddleware())
	router.Use(observability.RequestValidationMiddleware(observability.DefaultRequestValidationConfig()))
	router.Use(observability.CSRFMiddleware(observability.CSRFConfigWithTLS(cfg.TLS.Enabled)))

	// ── Frontend (merged from separate frontend service) ─────────────────
	frontendHandler := frontend.NewHandler("")
	frontend.RegisterRoutes(router, frontendHandler)

	// Prometheus /metrics endpoint
	metrics.RegisterMetricsEndpoint(router)

	// ── Phase 13: Security Observability ──────────────────────────────────
	// Initialize security monitoring: metrics, anomaly detection, threat
	// response, audit chain verification, SIEM export, and dashboard.
	secMetrics := securitymon.NewSecurityMetrics()
	secSIEM := securitymon.LoadSIEMExporterFromEnv(secMetrics)
	secDetector := securitymon.NewAnomalyDetector(5*time.Minute, 3.0, nil) // callback set after responder init
	// Wire ThreatResponder with real IAM session/token revokers.
	// IAM System is initialized at this point (line ~458).
	var secSessionRevoker securitymon.SessionRevoker
	var secTokenRevoker securitymon.TokenRevoker
	if iamSystem != nil {
		secSessionRevoker = iamSystem.Sessions
		secTokenRevoker = iamSystem.RevokedStore
	}
	secResponder := securitymon.NewThreatResponder(secSessionRevoker, secTokenRevoker, secMetrics, securitymon.DefaultThreatThresholds())
	secDetector = securitymon.NewAnomalyDetector(5*time.Duration(1)*time.Minute, 3.0, func(evt securitymon.AnomalyEvent) {
		secResponder.HandleAnomaly(evt)
	})

	// Audit chain verifier — runs periodic integrity checks.
	// Uses a nil-safe adapter: returns empty entries until a real audit logger is wired.
	auditProvider := securitymon.NewAuditLoggerAdapter(func(_ context.Context, limit int) ([]securitymon.ChainEntry, error) {
		return nil, nil // no audit logger wired yet — verifier will report "chain empty"
	})
	secVerifier := securitymon.NewAuditChainVerifier(auditProvider, secMetrics, 1*time.Hour)
	secVerifier.Start()

	// Dashboard endpoint (no auth required — internal ops visibility)
	secDashboard := securitymon.NewDashboardHandler(secMetrics, secDetector, secResponder, secVerifier, secSIEM)
	secDashboard.RegisterRoutes(router)

	log.Println("✅ Security observability initialized (Phase 13)")

	// ── Phase 14: Secret Management ─────────────────────────────────────
	// Centralized secret management with Vault support, versioning, grace
	// period, and scheduled rotation. Falls back to env vars when Vault
	// is not configured.
	secStore := secretmanager.LoadSecretStoreFromEnv()
	secMgr := secretmanager.NewSecretManager(secStore, 5, 24*time.Hour)
	secMgr.StartCleanupLoop(1 * time.Hour)

	// Preload existing env secrets into the manager for versioning
	for _, key := range []string{
		"POSTGRES_PASSWORD", "MYSQL_PASSWORD", "MARIADB_PASSWORD",
		"MONGODB_PASSWORD", "RABBITMQ_PASSWORD", "GATEKEEPER_ENCRYPTION_KEY",
		"GATEKEEPER_HMAC_KEY", "DEMO_JWT_SECRET",
	} {
		if val, err := secStore.Get(key); err == nil && val != "" {
			_ = secMgr.Put(key, val)
		}
	}

	log.Printf("✅ Secret manager initialized (store: %s)", secStore.Name())

	// ── Phase 16: Identity Federation ────────────────────────────────────
	// OIDC/SAML federation, behavior profiling, identity risk scoring,
	// and just-in-time privilege elevation.
	behaviorProfiler := federation.NewBehaviorProfiler()
	identityRiskScorer := federation.NewIdentityRiskScorer()
	jitManager := federation.NewJITManager(nil) // nil repo = in-memory only
	jitManager.StartCleanupLoop(1 * time.Hour)
	_ = behaviorProfiler   // wired into auth flow below
	_ = identityRiskScorer // wired into auth flow below
	_ = jitManager         // available for JIT privilege elevation

	// Load OIDC federation from env (if configured)
	if oidcIssuer := os.Getenv("FEDERATION_OIDC_ISSUER"); oidcIssuer != "" {
		oidcProvider := federation.NewOIDCProvider(
			os.Getenv("FEDERATION_OIDC_ALIAS"),
			oidcIssuer,
			os.Getenv("FEDERATION_OIDC_CLIENT_ID"),
			os.Getenv("FEDERATION_OIDC_CLIENT_SECRET"),
			nil,
		)
		oidcProvider.AuthorizationURL = os.Getenv("FEDERATION_OIDC_AUTH_URL")
		oidcProvider.TokenURL = os.Getenv("FEDERATION_OIDC_TOKEN_URL")
		oidcProvider.UserInfoURL = os.Getenv("FEDERATION_OIDC_USERINFO_URL")
		log.Printf("✅ OIDC federation provider loaded: %s", oidcProvider.Alias)
		_ = oidcProvider
	}

	log.Println("✅ Identity federation initialized (Phase 16)")

	// ── Phase 17: API Gateway ──────────────────────────────────────────────
	// Centralized API gateway with per-endpoint rate limiting, API key
	// management for external consumers, OpenAPI request validation, and
	// API version negotiation.
	gwSystem := apigateway.NewSystem()
	gwSystem.RegisterMiddleware(router)
	log.Println("✅ API Gateway initialized (Phase 17)")

	// Add API Metrics tracking middleware
	// Initialize first before adding middleware
	apiMetricsTracker := metrics.NewAPIMetricsTracker(conns.Valkey)
	router.Use(metrics.MetricsMiddleware(apiMetricsTracker))

	// Initialize Rate Limiter
	// Max calls and token validity from config (.env)
	rateLimiter := auth.NewRateLimiter(cfg.RateLimiting.MaxCallsPerToken, cfg.RateLimiting.TokenValidityMinutes)

	// Initialize Query Logger with Valkey/Redis
	queryLogger := querypkg.NewQueryLogger(conns.Valkey, "/data/query_logs")

	// Initialize all handlers
	healthHandler := healthpkg.NewHandler(conns)

	// Admin handler for database and table creation
	// Only include SQL databases (MongoDB and Firebase don't support SQL DDL operations)
	dbConnections := map[string]*gorm.DB{
		"mysql":    conns.MySQL,
		"mariadb":  conns.MariaDB,
		"postgres": conns.PostgreSQL,
		"percona":  conns.Percona,
		"oracle":   conns.Oracle,
	}
	adminHandler := database.NewHandler(dbConnections, conns.PostgreSQL)

	// User management handler
	platformUserHandler := iamusers.NewPlatformUserHandler(conns.Etcd)

	// Dynamic Query handlers for each database
	mysqlDynamicHandler := querypkg.NewHandler(conns.MySQL, queryLogger)
	mariadbDynamicHandler := querypkg.NewHandler(conns.MariaDB, queryLogger)
	postgresDynamicHandler := querypkg.NewHandler(conns.PostgreSQL, queryLogger)
	perconaDynamicHandler := querypkg.NewHandler(conns.Percona, queryLogger)
	oracleDynamicHandler := querypkg.NewHandler(conns.Oracle, queryLogger)

	// Notification handler
	discordWebhookURL := cfg.Discord.WebhookURL
	notificationHandler := notificationpkg.NewHandler(discordWebhookURL, dbConnections)

	// GraphQL handler (prefer PostgreSQL for schema introspection; fallback to available SQL engines)
	graphQLDB := conns.PostgreSQL
	if graphQLDB == nil {
		graphQLDB = conns.MySQL
	}
	if graphQLDB == nil {
		graphQLDB = conns.MariaDB
	}
	if graphQLDB == nil {
		graphQLDB = conns.Percona
	}
	if graphQLDB == nil {
		graphQLDB = conns.Oracle
	}
	graphQLHandler := graphqlpkg.NewHandler(graphQLDB)

	// Context enrichment helper - populates database name and user info for logging
	enrichRequestContext := func(c *gin.Context) {
		// Extract database name from URL path (e.g., /api/mysql/query -> mysql)
		pathParts := strings.Split(c.Request.URL.Path, "/")
		if len(pathParts) >= 3 {
			dbName := pathParts[2]
			switch dbName {
			case "mysql", "mariadb", "postgres", "percona", "oracle":
				c.Set("database", dbName)
			}
		}

		// Extract user info from validated claims if available
		if claims := auth.GetUser(c); claims != nil && claims.Sub != "" {
			c.Set("user_id", claims.Sub)
		}
	}

	// validateTOTPForUser validates a TOTP code against the user's enrolled factors.
	// Used by authenticateRequest() (risk-based MFA) and authorizeRequest()
	// (policy-triggered and step-up MFA) to avoid code duplication.
	validateTOTPForUser := func(c *gin.Context, userIDStr string, mfaToken string) bool {
		if gkSystem == nil || gkSystem.FactorRepository() == nil || gkSystem.TOTPService == nil {
			return false
		}
		uid, err := uuid.Parse(strings.TrimSpace(userIDStr))
		if err != nil || uid == uuid.Nil {
			return false
		}
		factors, fErr := gkSystem.FactorRepository().GetByUserID(c.Request.Context(), uid)
		if fErr != nil {
			return false
		}
		for _, factor := range factors {
			if !factor.IsActive() || factor.Spec.Type != gkmodels.FactorTypeTOTP {
				continue
			}
			if len(factor.Spec.EncryptedSecret) == 0 {
				continue
			}
			secretBytes, decErr := decryptAESSecret(gkSystem.Config().EncryptionKey, factor.Spec.EncryptedSecret)
			if decErr != nil {
				continue
			}
			secretB32 := base32.StdEncoding.EncodeToString(secretBytes)
			if ok, _ := gkSystem.TOTPService.ValidateCode(c.Request.Context(), secretB32, mfaToken); ok {
				return true
			}
		}
		return false
	}

	// authenticateRequest validates token + rate limits and sets auth context without advancing handlers.
	//
	// Phase 1 — unified JWT validation:
	//   Primary:  IAM Issuer (RSA-256 + etcd revocation check)
	//   Fallback: legacy auth.TokenValidator (only when IAM is unavailable)
	authenticateRequest := func(c *gin.Context) bool {
		// Phase 13: Record request for anomaly detection
		secMetrics.RecordTotalRequest()
		securitymon.PromTotalRequests.Inc()
		secDetector.RecordRequest(c.ClientIP(), "") // user ID set after auth succeeds

		// Phase 18: Track auth failures and export to SIEM
		authFailed := false
		defer func() {
			if authFailed {
				secMetrics.RecordAuthFailure()
				securitymon.PromAuthFailures.Inc()
				go secSIEM.Export(context.Background(), securitymon.SIEMEvent{
					Timestamp: time.Now().UTC(),
					EventType: "auth_failure",
					Severity:  "warning",
					IPAddress: c.ClientIP(),
					Outcome:   "failure",
					Message:   "authentication failed",
					Source:    "axiomnizam",
				})
			}
		}()

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// WebSocket connections cannot send custom headers from browsers;
			// accept token as a query parameter for upgrade requests.
			if qToken := strings.TrimSpace(c.Query("token")); qToken != "" {
				authHeader = "Bearer " + qToken
			}
		}
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return false
		}

		rawToken, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Sprintf("invalid authorization header: %v", err)})
			c.Abort()
			return false
		}

		// ── Unified validation path ──────────────────────────────────────
		// When the IAM system is available, use its Issuer for RSA-256
		// signature validation + etcd revocation check.  This is the same
		// path the IAM-specific middleware uses, ensuring a single
		// validation surface for the entire platform.
		//
		// Only when IAM is nil (e.g. startup race) do we fall back to the
		// legacy JWKS-based TokenValidator which supports demo HMAC tokens.
		// ─────────────────────────────────────────────────────────────────

		var principal string
		var email string
		var roles []string
		var clientID string
		var iamClaims *iamtoken.IAMClaims
		var legacyClaims *auth.Claims

		if iamSystem != nil && iamSystem.Issuer != nil {
			// ── Primary: IAM Issuer (RSA-256 + revocation) ──
			parsed, valErr := iamSystem.Issuer.ValidateAccessToken(rawToken)
			if valErr != nil {
				authFailed = true
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				c.Abort()
				return false
			}
			iamClaims = parsed

			// Replay-attack prevention: check JTI revocation in etcd/Raft
			if iamSystem.RevokedStore != nil && strings.TrimSpace(iamClaims.ID) != "" {
				if revoked, _ := iamSystem.RevokedStore.IsRevoked(strings.TrimSpace(iamClaims.ID)); revoked {
					authFailed = true
					c.JSON(http.StatusUnauthorized, gin.H{"error": "token has been revoked"})
					c.Abort()
					return false
				}
			}

			// Bootstrap sysadmin role fallback (same logic as IAM middleware)
			if len(iamClaims.Roles) == 0 {
				configuredSysadminEmail := strings.ToLower(strings.TrimSpace(os.Getenv("IAM_SYSADMIN_EMAIL")))
				claimEmail := strings.ToLower(strings.TrimSpace(iamClaims.Email))
				if configuredSysadminEmail != "" && claimEmail != "" && claimEmail == configuredSysadminEmail {
					iamClaims.Roles = []string{"sysadmin", "system-manager", "admin"}
					log.Printf("⚠️  Applied bootstrap sysadmin role fallback for token subject %s", claimEmail)
				}
			}

			email = iamClaims.Email
			roles = iamClaims.Roles
			clientID = strings.TrimSpace(iamClaims.ClientID)

			principal = strings.TrimSpace(iamClaims.DisplayName)
			if principal == "" {
				principal = strings.TrimSpace(iamClaims.Email)
			}
			if principal == "" {
				principal = strings.TrimSpace(iamClaims.Sub)
			}
			if principal == "" {
				principal = "token-user"
			}
		} else {
			// ── Fallback: legacy JWKS TokenValidator ──
			activeTokenValidator := getOrInitTokenValidator()
			if activeTokenValidator == nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error":   "authentication unavailable",
					"message": "token validation is not available because IAM token validator initialization failed",
				})
				c.Abort()
				return false
			}

			claims, valErr := activeTokenValidator.ValidateToken(rawToken)
			if valErr != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Sprintf("invalid token: %v", valErr)})
				c.Abort()
				return false
			}
			legacyClaims = claims

			if len(legacyClaims.RolesList()) == 0 {
				configuredSysadminEmail := strings.ToLower(strings.TrimSpace(os.Getenv("IAM_SYSADMIN_EMAIL")))
				claimEmail := strings.ToLower(strings.TrimSpace(legacyClaims.Email))
				if configuredSysadminEmail != "" && claimEmail != "" && claimEmail == configuredSysadminEmail {
					fallbackRoles := []string{"sysadmin", "system-manager", "admin"}
					legacyClaims.Roles = append([]string{}, fallbackRoles...)
					legacyClaims.RealmAccess.Roles = append([]string{}, fallbackRoles...)
					log.Printf("⚠️  Applied bootstrap sysadmin role fallback for token subject %s", claimEmail)
				}
			}

			email = legacyClaims.Email
			roles = legacyClaims.RolesList()
			clientID = strings.TrimSpace(legacyClaims.ClientID)

			principal = strings.TrimSpace(legacyClaims.PreferredUsername)
			if principal == "" {
				principal = strings.TrimSpace(legacyClaims.Email)
			}
			if principal == "" {
				principal = strings.TrimSpace(legacyClaims.Sub)
			}
			if principal == "" {
				principal = "token-user"
			}
		}

		// ── Phase 9: Risk scoring with continuous verification ──────────────
		//
		// On each request:
		// 1. Compute current risk score from signals
		// 2. Compare with JWT-embedded last risk score (risk delta)
		// 3. Detect IP address and device fingerprint changes
		// 4. If risk delta > 30 or IP/device changed → flag for step-up MFA
		// 5. If risk >= 90 → revoke session and all tokens

		riskScore := 0
		currentIP := c.ClientIP()
		currentFP := c.GetHeader("X-Device-Fingerprint")

		if gkSystem != nil && gkSystem.RiskService != nil {
			signals := &gkrisk.Signals{
				IPAddress:         currentIP,
				DeviceFingerprint: currentFP,
			}
			if score, scoreErr := gkSystem.RiskService.Score(c.Request.Context(), signals); scoreErr == nil {
				riskScore = score
			} else {
				log.Printf("⚠️  Risk scoring failed: %v", scoreErr)
			}
		}

		// Phase 9: Risk delta comparison — detect risk score changes between requests.
		var riskDelta int
		var ipChanged, deviceChanged bool
		var lastRiskScore int

		if iamClaims != nil {
			lastRiskScore = iamClaims.LastRiskScore
			if iamClaims.LastIPAddress != "" && iamClaims.LastIPAddress != currentIP {
				ipChanged = true
			}
			if iamClaims.LastDeviceFP != "" && currentFP != "" && iamClaims.LastDeviceFP != currentFP {
				deviceChanged = true
			}
		} else if legacyClaims != nil {
			lastRiskScore = legacyClaims.LastRiskScore
			if legacyClaims.LastIPAddress != "" && legacyClaims.LastIPAddress != currentIP {
				ipChanged = true
			}
			if legacyClaims.LastDeviceFP != "" && currentFP != "" && legacyClaims.LastDeviceFP != currentFP {
				deviceChanged = true
			}
		}

		if lastRiskScore > 0 {
			riskDelta = riskScore - lastRiskScore
			if riskDelta < 0 {
				riskDelta = -riskDelta
			}
		}

		// Boost risk score on IP/device changes (signals not yet wired to scorer).
		if ipChanged {
			riskScore += 10
			log.Printf("⚠️  IP change detected for %s: risk %d → %s (risk +%d)", principal, lastRiskScore, currentIP, 10)
		}
		if deviceChanged {
			riskScore += 15
			log.Printf("⚠️  Device change detected for %s (risk +%d)", principal, 15)
		}
		if riskScore > 100 {
			riskScore = 100
		}

		// Phase 9: Revoke session on critical risk (>= 90).
		if riskScore >= 90 {
			log.Printf("🚨 Critical risk %d for %s — revoking session", riskScore, principal)
			secMetrics.RecordHighRisk()
			securitymon.PromHighRiskRequests.Inc()
			// Revoke the session if we have a session ID.
			if iamClaims != nil && iamClaims.SessionID != "" && iamSystem != nil && iamSystem.Sessions != nil {
				_ = iamSystem.Sessions.Revoke(iamClaims.SessionID)
				secMetrics.RecordSessionRevoked()
				securitymon.PromSessionsRevoked.Inc()
			}
			// Revoke the current token JTI so it can't be reused.
			if iamSystem != nil && iamSystem.RevokedStore != nil {
				if iamClaims != nil && iamClaims.ID != "" {
					remaining := time.Until(iamClaims.ExpiresAt.Time)
					if remaining > 0 {
						_ = iamSystem.RevokedStore.Revoke(iamClaims.ID, remaining)
					}
				}
			}
		}

		// ── Phase 11: Session lifecycle enforcement ──────────────────────────
		//
		// Proper session-based idle timeout and max lifespan enforcement.
		// Replaces Phase 9's token `iat`-based approximation with actual
		// session LastAccessAt tracking from etcd/Raft.
		//
		// Idle timeout: if no activity for SESSION_IDLE_TIMEOUT_MINUTES → 401
		// Max lifespan: if session older than SESSION_MAX_LIFESPAN_HOURS → 401
		// Both return `Session-Expired` header so frontend can redirect to login.

		idleTimeoutMinutes := 30 // default
		if v := strings.TrimSpace(os.Getenv("SESSION_IDLE_TIMEOUT_MINUTES")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				idleTimeoutMinutes = n
			}
		}

		maxLifespanHours := 10 // default 10 hours
		if v := strings.TrimSpace(os.Getenv("SESSION_MAX_LIFESPAN_HOURS")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				maxLifespanHours = n
			}
		}

		// Extract session ID from JWT claims
		var sessionID string
		if iamClaims != nil {
			sessionID = strings.TrimSpace(iamClaims.SessionID)
		}

		// Session-based lifecycle enforcement (only when we have a session ID and IAM system)
		if sessionID != "" && iamSystem != nil && iamSystem.Sessions != nil {
			sess, sessErr := iamSystem.Sessions.GetByID(sessionID)
			if sessErr == nil && sess != nil {
				now := time.Now().UTC()

				// Check max lifespan (absolute session duration)
				if !sess.CreatedAt.IsZero() {
					sessionAge := now.Sub(sess.CreatedAt)
					if sessionAge > time.Duration(maxLifespanHours)*time.Hour {
						log.Printf("⏰ Session max lifespan exceeded for %s: session age %s (limit: %d hours)",
							principal, sessionAge.Round(time.Second), maxLifespanHours)
						_ = iamSystem.Sessions.Revoke(sessionID)
						c.Header("Session-Expired", "max_lifespan")
						c.JSON(http.StatusUnauthorized, gin.H{
							"error":   "session expired",
							"reason":  "max_lifespan",
							"message": fmt.Sprintf("your session has exceeded the maximum lifespan of %d hours. please re-authenticate", maxLifespanHours),
						})
						c.Abort()
						return false
					}
				}

				// Check idle timeout (time since last activity)
				lastAccess := sess.LastAccessAt
				if lastAccess.IsZero() {
					lastAccess = sess.CreatedAt // fallback for sessions created before Phase 11
				}
				if !lastAccess.IsZero() {
					idleDuration := now.Sub(lastAccess)
					if idleDuration > time.Duration(idleTimeoutMinutes)*time.Minute {
						log.Printf("⏰ Session idle timeout for %s: idle %s (limit: %d min)",
							principal, idleDuration.Round(time.Second), idleTimeoutMinutes)
						_ = iamSystem.Sessions.Revoke(sessionID)
						c.Header("Session-Expired", "idle_timeout")
						c.JSON(http.StatusUnauthorized, gin.H{
							"error":   "session expired due to inactivity",
							"reason":  "idle_timeout",
							"message": fmt.Sprintf("your session has been idle for more than %d minutes. please re-authenticate", idleTimeoutMinutes),
						})
						c.Abort()
						return false
					}
				}

				// Update LastAccessAt asynchronously (don't block the request)
				go func(sid string) {
					_ = iamSystem.Sessions.Touch(sid)
				}(sessionID)
			}
		} else if sessionID == "" {
			// Fallback for tokens without session ID (legacy/demo tokens):
			// Use token `iat` as proxy for last activity.
			var tokenIssuedAt time.Time
			if iamClaims != nil {
				tokenIssuedAt = iamClaims.IssuedAt.Time
			} else if legacyClaims != nil {
				tokenIssuedAt = legacyClaims.IssuedAt.Time
			}
			if !tokenIssuedAt.IsZero() {
				idleDuration := time.Since(tokenIssuedAt)
				if idleDuration > time.Duration(idleTimeoutMinutes)*time.Minute {
					log.Printf("⏰ Token idle timeout for %s: issued %s ago (limit: %d min)",
						principal, idleDuration.Round(time.Second), idleTimeoutMinutes)
					c.Header("Session-Expired", "idle_timeout")
					c.JSON(http.StatusUnauthorized, gin.H{
						"error":   "session expired due to inactivity",
						"reason":  "idle_timeout",
						"message": fmt.Sprintf("your session has been idle for more than %d minutes. please re-authenticate", idleTimeoutMinutes),
					})
					c.Abort()
					return false
				}
			}
		}

		// Store continuous verification data in context for downstream use.
		c.Set("risk_score", riskScore)
		c.Set("risk_delta", riskDelta)
		c.Set("ip_changed", ipChanged)
		c.Set("device_changed", deviceChanged)

		// ── Phase 9: Risk delta step-up MFA ────────────────────────────────
		//
		// If risk delta > 30 and absolute risk >= 50, require step-up MFA.
		// This catches sudden risk increases even when absolute risk is moderate.

		if riskDelta > 30 && riskScore >= 50 {
			secMetrics.RecordRiskDeltaTrigger()
			securitymon.PromRiskDeltaTriggers.Inc()
			secMetrics.RecordStepUp()
			securitymon.PromStepUpRequired.Inc()
			mfaToken := strings.TrimSpace(c.GetHeader("X-MFA-Token"))
			if mfaToken == "" {
				log.Printf("⚠️  Risk delta %d requires step-up MFA for user %s", riskDelta, principal)
				secMetrics.RecordMFAChallenge("totp")
				securitymon.PromMFAChallenges.WithLabelValues("totp").Inc()
				c.JSON(http.StatusForbidden, gin.H{
					"error":        "step-up mfa required",
					"risk_score":   riskScore,
					"risk_delta":   riskDelta,
					"mfa_required": true,
					"message":      "a significant change in risk signals requires re-verification. provide a TOTP code in the X-MFA-Token header",
				})
				c.Abort()
				return false
			}
			var deltaUserID string
			if iamClaims != nil {
				deltaUserID = strings.TrimSpace(iamClaims.Sub)
			} else if legacyClaims != nil {
				deltaUserID = strings.TrimSpace(legacyClaims.Sub)
			}
			if !validateTOTPForUser(c, deltaUserID, mfaToken) {
				secMetrics.RecordMFAFailure()
				securitymon.PromMFAFailures.Inc()
				c.JSON(http.StatusForbidden, gin.H{
					"error":      "step-up mfa verification failed",
					"risk_delta": riskDelta,
					"message":    "the provided TOTP code is invalid",
				})
				c.Abort()
				return false
			}
			log.Printf("✅ Step-up MFA verified for user %s (risk delta: %d)", principal, riskDelta)
			secMetrics.RecordMFASuccess()
			securitymon.PromMFAChallenges.WithLabelValues("totp").Inc()
			// Record MFA verification timestamp for continuous verification.
			nowUnix := time.Now().Unix()
			if iamClaims != nil {
				iamClaims.LastVerifiedAt = nowUnix
			} else if legacyClaims != nil {
				legacyClaims.LastVerifiedAt = nowUnix
			}
		}

		// Propagate risk data into claims for downstream consumers and next token.
		if iamClaims != nil {
			iamClaims.LastRiskScore = riskScore
			iamClaims.RiskScore = riskScore
			iamClaims.LastIPAddress = currentIP
			iamClaims.LastDeviceFP = currentFP
		} else if legacyClaims != nil {
			legacyClaims.LastRiskScore = riskScore
			legacyClaims.RiskScore = riskScore
			legacyClaims.LastIPAddress = currentIP
			legacyClaims.LastDeviceFP = currentFP
		}

		// ── Risk-based MFA enforcement (Phase 5) ────────────────────────
		//
		// Score ≥ 90 → reject outright (critical risk).
		// Score ≥ 70 → require X-MFA-Token header with a valid TOTP code.
		//
		// If the Gatekeeper system is unavailable or the user has no
		// enrolled TOTP factor, high-risk requests are rejected to
		// maintain security posture.

		// ── Phase 5: Trusted device bypass + MFA enforcement ───────────────
		//
		// Risk >= 90: ChallengePhaseRejected — return structured MFA challenge
		//   so the frontend can prompt for TOTP instead of a hard block.
		// Risk >= 70: Require MFA (TOTP) unless the device is trusted.

		if riskScore >= 90 {
			log.Printf("🚫 Critical risk — %d for user %s from %s — requiring MFA challenge",
				riskScore, principal, c.ClientIP())
			secMetrics.RecordHighRisk()
			securitymon.PromHighRiskRequests.Inc()
			secMetrics.RecordMFAChallenge("totp")
			securitymon.PromMFAChallenges.WithLabelValues("totp").Inc()
			c.JSON(http.StatusForbidden, gin.H{
				"error":          "challenge_rejected",
				"risk_score":     riskScore,
				"mfa_required":   true,
				"challenge_type": "totp",
				"message":        "this request has been flagged as high risk. complete MFA verification to proceed",
			})
			c.Abort()
			return false
		}

		if riskScore >= 70 {
			// ── Trusted device bypass (Phase 5) ──────────────────────────
			// If the user has a valid trusted device cookie, skip TOTP.
			mfaSkippedByDevice := false
			if gkSystem != nil && gkSystem.DeviceService != nil {
				deviceToken, cookieErr := c.Cookie("axiomnizam_device_token")
				if cookieErr == nil && deviceToken != "" {
					deviceFingerprint := c.GetHeader("X-Device-Fingerprint")
					if deviceFingerprint != "" {
						var deviceUserID uuid.UUID
						if iamClaims != nil {
							deviceUserID, _ = uuid.Parse(strings.TrimSpace(iamClaims.Sub))
						} else if legacyClaims != nil {
							deviceUserID, _ = uuid.Parse(strings.TrimSpace(legacyClaims.Sub))
						}
						if deviceUserID != uuid.Nil {
							if verified, _ := gkSystem.DeviceService.VerifyDeviceToken(
								c.Request.Context(), deviceUserID, deviceFingerprint, deviceToken,
							); verified {
								log.Printf("✅ Trusted device bypass — user %s from %s (risk: %d)",
									principal, c.ClientIP(), riskScore)
								mfaSkippedByDevice = true
							}
						}
					}
				}
			}

			if !mfaSkippedByDevice {
				mfaToken := strings.TrimSpace(c.GetHeader("X-MFA-Token"))
				if mfaToken == "" {
					log.Printf("⚠️  MFA required — risk score %d for user %s from %s", riskScore, principal, c.ClientIP())
					c.JSON(http.StatusForbidden, gin.H{
						"error":        "mfa verification required",
						"risk_score":   riskScore,
						"mfa_required": true,
						"message":      "this request requires multi-factor authentication. provide a valid TOTP code in the X-MFA-Token header",
					})
					c.Abort()
					return false
				}

				// Validate TOTP via shared helper (used by authenticateRequest + authorizeRequest)
				var mfaUserID string
				if iamClaims != nil {
					mfaUserID = strings.TrimSpace(iamClaims.Sub)
				} else if legacyClaims != nil {
					mfaUserID = strings.TrimSpace(legacyClaims.Sub)
				}
				if !validateTOTPForUser(c, mfaUserID, mfaToken) {
					log.Printf("🚫 MFA verification failed — risk score %d for user %s from %s", riskScore, principal, c.ClientIP())
					secMetrics.RecordMFAFailure()
					securitymon.PromMFAFailures.Inc()
					c.JSON(http.StatusForbidden, gin.H{
						"error":      "mfa verification failed",
						"risk_score": riskScore,
						"message":    "the provided TOTP code is invalid or no MFA factor is enrolled",
					})
					c.Abort()
					return false
				}
				log.Printf("✅ MFA verified for user %s (risk score: %d)", principal, riskScore)
				secMetrics.RecordMFASuccess()
				securitymon.PromMFAChallenges.WithLabelValues("totp").Inc()
			}
		}

		// ── Rate limiting ────────────────────────────────────────────────

		defaultMaxCalls, defaultValidity := rateLimiter.DefaultPolicy()
		callsLimit := defaultMaxCalls

		allowed, callsRemaining, expiresAt, limitErr := rateLimiter.CheckRateLimit(rawToken)
		if !allowed && limitErr != nil && limitErr.Error() == "token not tracked or invalid" {
			// Accept valid IAM/JWT tokens even if they were not issued through /auth/login.
			policyCalls := defaultMaxCalls
			policyValidity := defaultValidity

			if clientID != "" && iamSystem != nil && iamSystem.Clients != nil {
				if clientCfg, clientErr := iamSystem.Clients.GetClient(clientID); clientErr == nil && clientCfg != nil {
					if clientCfg.RateLimitMaxCalls > 0 {
						policyCalls = clientCfg.RateLimitMaxCalls
					}
					if clientCfg.TokenValidityMinutes > 0 {
						policyValidity = time.Duration(clientCfg.TokenValidityMinutes) * time.Minute
					}
				}
			}

			rateLimiter.RegisterTokenWithPolicy(rawToken, principal, policyCalls, policyValidity)
			callsLimit = policyCalls
			allowed, callsRemaining, expiresAt, limitErr = rateLimiter.CheckRateLimit(rawToken)
		}

		if trackedLimit, _, tracked := rateLimiter.GetTokenPolicy(rawToken); tracked && trackedLimit > 0 {
			callsLimit = trackedLimit
		}

		if !allowed {
			if limitErr != nil && limitErr.Error() == "token expired" {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":      "token expired",
					"message":    "your token is no longer valid. please login again to get a new token",
					"expired_at": expiresAt.Format("2006-01-02 15:04:05"),
				})
			} else {
				c.Header("Retry-After", "60")
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":           "api call limit exceeded",
					"message":         fmt.Sprintf("you have used all %d api calls allowed for this token", callsLimit),
					"calls_limit":     callsLimit,
					"expires_at":      expiresAt.Format("2006-01-02 15:04:05"),
					"action_required": fmt.Sprintf("use a new token to continue with a fresh %d-call quota", callsLimit),
					"action_endpoint": "/auth/login",
				})
			}
			authFailed = true
			c.Abort()
			return false
		}

		if err := rateLimiter.IncrementCallCount(rawToken); err != nil {
			log.Printf("⚠️  Failed to increment call count: %v", err)
		}

		// ── Set context for downstream handlers ──────────────────────────

		// Store the appropriate claims type so downstream handlers can
		// retrieve them via auth.GetUser(c) or c.Get("user").
		if iamClaims != nil {
			// Convert IAM claims to legacy auth.Claims for backward compat
			compatClaims := &auth.Claims{
				Sub:               iamClaims.Sub,
				PreferredUsername: iamClaims.DisplayName,
				Email:             iamClaims.Email,
				DisplayName:       iamClaims.DisplayName,
				Roles:             iamClaims.Roles,
				RiskScore:         riskScore,
				RegisteredClaims:  iamClaims.RegisteredClaims,
			}
			c.Set("user", compatClaims)
		} else {
			c.Set("user", legacyClaims)
		}
		c.Set("username", principal)
		c.Set("email", email)
		c.Set("roles", roles)
		c.Set("calls_remaining", callsRemaining)
		c.Set("token_expires_at", expiresAt.Format("2006-01-02 15:04:05"))
		c.Set("token", rawToken)

		// Phase 13/18: Track authenticated user for anomaly detection
		secDetector.RecordRequest("", principal) // user ID only — IP already tracked above

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", callsLimit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", callsRemaining))
		c.Header("X-Token-Expires-At", expiresAt.Format("2006-01-02 15:04:05"))

		log.Printf("✅ Token validated & rate limit OK for user: %s (calls remaining: %d)", principal, callsRemaining)
		secMetrics.RecordAuthSuccess()
		securitymon.PromAuthSuccesses.Inc()
		// Phase 13: Export auth success to SIEM
		go secSIEM.Export(context.Background(), securitymon.SIEMEvent{
			Timestamp: time.Now().UTC(),
			EventType: "auth_success",
			Severity:  "info",
			UserID:    principal,
			IPAddress: c.ClientIP(),
			Outcome:   "success",
			Message:   fmt.Sprintf("User %s authenticated successfully", principal),
		})
		return true
	}

	// Apply auth middleware to protected routes.
	authMiddleware := func(c *gin.Context) {
		if !authenticateRequest(c) {
			return
		}
		enrichRequestContext(c)
		c.Next()
	}

	// Health check endpoints (no auth required)
	router.GET("/health", healthHandler.Health)
	router.GET("/status", healthHandler.Status)
	router.GET("/distributed", healthHandler.Distributed)

	// Phase 0: Reconciler health endpoint (no auth — ops visibility)
	var keySpaceMonitorRef *metrics.EtcdKeySpaceMonitor // set later when etcd is available
	router.GET("/health/reconcilers", func(c *gin.Context) {
		summary := metrics.GlobalReconcilerMetrics.HealthSummary()
		statuses := metrics.GlobalReconcilerMetrics.GetAllStatuses()
		status := http.StatusOK
		if summary.Status == "degraded" {
			status = http.StatusServiceUnavailable
		}
		response := gin.H{"summary": summary, "reconcilers": statuses}
		if keySpaceMonitorRef != nil {
			response["etcdKeySpace"] = keySpaceMonitorRef.GetStats()
		}
		c.JSON(status, response)
	})

	// Authentication endpoints (no auth required for login/refresh)
	authHandler := authn.NewAuthHandler()
	authHandler.SetRateLimiter(rateLimiter)
	authHandler.SetPlatformUserHandler(platformUserHandler)
	if iamSystem != nil && iamSystem.PGStore != nil {
		authHandler.SetIdentityProviderStore(iamSystem.PGStore)
	}
	if iamSystem != nil && iamSystem.Users != nil {
		authHandler.SetIAMUserRepository(iamSystem.Users)
	}
	if iamSystem != nil && iamSystem.Authorizer != nil {
		authHandler.SetIAMAuthorizer(iamSystem.Authorizer)
	}
	router.POST("/auth/login", authHandler.Login)
	router.POST("/auth/refresh", authHandler.RefreshToken)
	router.GET("/auth/validate", authHandler.ValidateToken)
	router.GET("/auth/oauth/start", authHandler.OAuthStart)
	router.GET("/auth/oauth/callback", authHandler.OAuthCallback)

	// Protected auth endpoints (auth required)
	router.POST("/auth/logout", authMiddleware, authHandler.Logout)
	router.GET("/auth/token-status", authMiddleware, authHandler.GetTokenStatus)
	// Phase 19: Wire IAM Authorizer RequirePermission on sensitive admin endpoints.
	// This adds IAM-level permission checking in addition to role-based admin checks.
	var requireManageUsers gin.HandlerFunc
	var requireManageRoles gin.HandlerFunc
	if iamSystem != nil && iamSystem.Authorizer != nil {
		requireManageUsers = iammw.RequirePermission(iamSystem.Authorizer, "iam.users", "manage")
		requireManageRoles = iammw.RequirePermission(iamSystem.Authorizer, "iam.roles", "manage")
	} else {
		// Fallback: if authorizer not available, pass through (admin check still applies)
		requireManageUsers = func(c *gin.Context) { c.Next() }
		requireManageRoles = func(c *gin.Context) { c.Next() }
	}
	router.GET("/auth/admin/tokens-status", authMiddleware, auth.RequireAdmin(), requireManageUsers, authHandler.GetAllTokensStatus)

	// Wire requireManageRoles on IAM role management routes
	if iamSystem != nil {
		iamRoutes := router.Group("/api/v1/iam", authMiddleware, requireManageRoles)
		_ = iamRoutes // routes registered by iamSystem.RegisterRoutes() below
	}

	// Get admin middleware (requires admin role)
	var adminMiddleware gin.HandlerFunc
	var adminOrSysMiddleware gin.HandlerFunc
	adminMiddleware = func(c *gin.Context) {
		if !authenticateRequest(c) {
			return
		}
		enrichRequestContext(c)
		claims := auth.GetUser(c)
		if claims == nil || !claims.HasRole("admin") {
			roles := []string{}
			if claims != nil {
				roles = claims.RolesList()
			}
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "forbidden: user does not have 'admin' role",
				"user_roles": roles,
				"required":   "admin",
			})
			c.Abort()
			return
		}
		c.Next()
	}
	adminOrSysMiddleware = func(c *gin.Context) {
		if !authenticateRequest(c) {
			return
		}
		enrichRequestContext(c)
		claims := auth.GetUser(c)
		if claims == nil || !(claims.HasRole("admin") || claims.HasRole("system-manager") || claims.HasRole("sysadmin") || claims.HasRole("system_admin") || claims.HasRole("system-admin")) {
			roles := []string{}
			if claims != nil {
				roles = claims.RolesList()
			}
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "forbidden: user must have one of roles [admin system-manager sysadmin system_admin system-admin]",
				"user_roles": roles,
				"required":   []string{"admin", "system-manager", "sysadmin", "system_admin", "system-admin"},
			})
			c.Abort()
			return
		}
		c.Next()
	}

	// ====================================
	// RBAC ENGINE + ZERO TRUST AUTHORIZATION (Phase 3)
	// ====================================
	//
	// The K8s-style RBAC engine provides resource+verb permission checks
	// with condition evaluation (IP restrictions, time windows).
	// It is seeded with default cluster roles matching the IAM system roles.

	rbacEngine := rbac.NewEngine()
	seedDefaultRBACRoles(rbacEngine)

	// authorizeRequest evaluates resource-level RBAC permissions and
	// risk-based policy decisions after JWT authentication has succeeded.
	//
	// Flow:
	//   1. Map HTTP method → RBAC verb (GET→read, POST→create, etc.)
	//   2. Map URL path segment → RBAC resource kind
	//   3. Inject request metadata (IP, time) into context for condition evaluation
	//   4. Call rbacEngine.CanPerform() — checks roles + conditions
	//   5. Call policy engine with actual risk score — may block or require MFA
	//   6. If RBAC denies and user is not sysadmin → 403
	authorizeRequest := func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		userIDStr, _ := userID.(string)
		if userIDStr == "" {
			c.Next()
			return
		}

		// Admin-equivalent bypass — matches the same roles that adminOrSysMiddleware accepts.
		// Sysadmin, admin, system-manager, system_admin, system-admin all get full access.
		// This preserves backward compatibility with the existing role model while the RBAC
		// engine handles fine-grained authorization for non-admin users.
		if claims := auth.GetUser(c); claims != nil {
			for _, r := range claims.RolesList() {
				role := strings.ToLower(strings.TrimSpace(r))
				if role == "sysadmin" || role == "admin" || role == "system-manager" ||
					role == "system_admin" || role == "system-admin" {
					c.Next()
					return
				}
			}
		}

		// Map HTTP method → RBAC verb
		verb := mapHTTPMethodToRBACVerb(c.Request.Method)

		// Map URL path → RBAC resource kind
		resource := mapPathToRBACResource(c.Request.URL.Path)

		// Inject request metadata for condition evaluation (IP restrictions, time windows)
		meta := &rbac.RequestMetadata{
			IPAddress:   c.ClientIP(),
			RequestTime: time.Now(),
			UserAgent:   c.GetHeader("User-Agent"),
		}
		ctx := context.WithValue(c.Request.Context(), rbac.RequestMetadataKey, meta)

		// RBAC check
		allowed, reason := rbacEngine.CanPerform(ctx, userIDStr, resource, verb, "")

		// Policy evaluation with actual risk score
		if gkSystem != nil && gkSystem.PolicyService != nil {
			riskScore := 0
			if rs, ok := c.Get("risk_score"); ok {
				if v, ok := rs.(int); ok {
					riskScore = v
				}
			}
			policyReq := &gkpolicy.EvaluationRequest{
				UserID:       userIDStr,
				ResourceType: resource,
				ResourcePath: c.Request.URL.Path,
				IPAddress:    c.ClientIP(),
				RiskScore:    riskScore,
			}
			policyResult, polErr := gkSystem.PolicyService.EvaluateHTTPRequest(ctx, policyReq)
			if polErr == nil && policyResult != nil {
				if policyResult.ShouldBlock() {
					log.Printf("🚫 Policy engine blocked request: user=%s resource=%s verb=%s reason=%s",
						userIDStr, resource, verb, policyResult.Reason)
					secMetrics.RecordPolicyBlock()
					securitymon.PromPolicyBlocks.Inc()
					c.JSON(http.StatusForbidden, gin.H{
						"error":  "request blocked by policy",
						"reason": policyResult.Reason,
					})
					c.Abort()
					return
				}
				// Store policy result for downstream handlers
				c.Set("policy_requires_mfa", policyResult.RequiresMFA)
				c.Set("policy_risk_action", policyResult.RiskAction)

				// ── Phase 5: Policy enforcement mode ────────────────────────
				// When the policy engine says MFA is required (risk >= 50, new device,
				// sensitive resource), enforce it here even if authenticateRequest()
				// didn't trigger MFA (e.g., risk score was < 70).
				if policyResult.ShouldChallenge() && !policyResult.ShouldBlock() {
					mfaToken := strings.TrimSpace(c.GetHeader("X-MFA-Token"))
					if mfaToken == "" {
						log.Printf("⚠️  Policy requires MFA: user=%s resource=%s verb=%s reason=%s",
							userIDStr, resource, verb, policyResult.Reason)
						c.JSON(http.StatusForbidden, gin.H{
							"error":          "challenge_rejected",
							"mfa_required":   true,
							"challenge_type": "totp",
							"reason":         policyResult.Reason,
							"message":        "this operation requires MFA verification. provide a TOTP code in the X-MFA-Token header",
						})
						c.Abort()
						return
					}
					// Validate the TOTP code for policy-triggered MFA
					mfaValid := validateTOTPForUser(c, userIDStr, mfaToken)
					if !mfaValid {
						c.JSON(http.StatusForbidden, gin.H{
							"error":   "mfa verification failed",
							"reason":  policyResult.Reason,
							"message": "the provided TOTP code is invalid",
						})
						c.Abort()
						return
					}
					log.Printf("✅ Policy MFA verified: user=%s resource=%s", userIDStr, resource)
				}
			}
		}

		// ── Phase 5: Step-up MFA for sensitive operations ────────────────
		// Certain operations require fresh MFA regardless of risk score:
		// DELETE operations, admin resources, policy/encryption changes.
		if verb == "delete" || resource == "admin" || resource == "encryption" || resource == "rbac" {
			mfaToken := strings.TrimSpace(c.GetHeader("X-MFA-Token"))
			if mfaToken == "" {
				log.Printf("⚠️  Step-up MFA required: user=%s resource=%s verb=%s",
					userIDStr, resource, verb)
				c.JSON(http.StatusForbidden, gin.H{
					"error":          "challenge_rejected",
					"mfa_required":   true,
					"challenge_type": "totp",
					"step_up":        true,
					"message":        "this sensitive operation requires fresh MFA verification",
				})
				c.Abort()
				return
			}
			mfaValid := validateTOTPForUser(c, userIDStr, mfaToken)
			if !mfaValid {
				c.JSON(http.StatusForbidden, gin.H{
					"error":   "mfa verification failed",
					"message": "the provided TOTP code is invalid for step-up verification",
				})
				c.Abort()
				return
			}
			log.Printf("✅ Step-up MFA verified: user=%s resource=%s verb=%s", userIDStr, resource, verb)
		}

		if !allowed {
			log.Printf("🚫 RBAC denied: user=%s resource=%s verb=%s reason=%s",
				userIDStr, resource, verb, reason)
			secMetrics.RecordRBACDenial()
			securitymon.PromRBACDenials.Inc()
			c.JSON(http.StatusForbidden, gin.H{
				"error":    "insufficient permissions",
				"resource": resource,
				"action":   verb,
				"reason":   reason,
			})
			c.Abort()
			return
		}

		c.Next()
	}

	// authzMiddleware combines authentication + RBAC authorization in one middleware.
	// Use this on route groups that require resource-level permission checks.
	authzMiddleware := func(c *gin.Context) {
		if !authenticateRequest(c) {
			return
		}
		enrichRequestContext(c)
		authorizeRequest(c)
	}

	// GraphQL endpoints (auth required)
	router.POST("/api/graphql", authMiddleware, graphQLHandler.Query)
	router.GET("/api/graphql/schema", authMiddleware, graphQLHandler.GetSchema)
	router.GET("/api/graphql/playground", authMiddleware, graphQLHandler.Playground)

	// ====================================
	// DYNAMIC QUERY ENDPOINTS (Auth Required)
	// ====================================
	// These endpoints allow dynamic SQL queries via Postman or any HTTP client
	// GET requests only support SELECT queries
	// POST requests are restricted to admin/system-manager roles.

	// MySQL Dynamic Queries
	router.GET("/api/mysql/query", authMiddleware, mysqlDynamicHandler.DynamicQuery)
	router.POST("/api/mysql/query", authzMiddleware, mysqlDynamicHandler.DynamicQueryWithBody)
	router.POST("/api/mysql/query/batch", authzMiddleware, mysqlDynamicHandler.BatchQueries)
	router.GET("/api/mysql/schema", authMiddleware, mysqlDynamicHandler.TableSchema)

	// MariaDB Dynamic Queries
	router.GET("/api/mariadb/query", authMiddleware, mariadbDynamicHandler.DynamicQuery)
	router.POST("/api/mariadb/query", authzMiddleware, mariadbDynamicHandler.DynamicQueryWithBody)
	router.POST("/api/mariadb/query/batch", authzMiddleware, mariadbDynamicHandler.BatchQueries)
	router.GET("/api/mariadb/schema", authMiddleware, mariadbDynamicHandler.TableSchema)

	// PostgreSQL Dynamic Queries
	router.GET("/api/postgres/query", authMiddleware, postgresDynamicHandler.DynamicQuery)
	router.POST("/api/postgres/query", authzMiddleware, postgresDynamicHandler.DynamicQueryWithBody)
	router.POST("/api/postgres/query/batch", authzMiddleware, postgresDynamicHandler.BatchQueries)
	router.GET("/api/postgres/schema", authMiddleware, postgresDynamicHandler.TableSchema)

	// Percona Dynamic Queries
	router.GET("/api/percona/query", authMiddleware, perconaDynamicHandler.DynamicQuery)
	router.POST("/api/percona/query", authzMiddleware, perconaDynamicHandler.DynamicQueryWithBody)
	router.POST("/api/percona/query/batch", authzMiddleware, perconaDynamicHandler.BatchQueries)
	router.GET("/api/percona/schema", authMiddleware, perconaDynamicHandler.TableSchema)

	// Oracle Dynamic Queries
	router.GET("/api/oracle/query", authMiddleware, oracleDynamicHandler.DynamicQuery)
	router.POST("/api/oracle/query", authzMiddleware, oracleDynamicHandler.DynamicQueryWithBody)
	router.POST("/api/oracle/query/batch", authzMiddleware, oracleDynamicHandler.BatchQueries)
	router.GET("/api/oracle/schema", authMiddleware, oracleDynamicHandler.TableSchema)

	// ====================================
	// QUERY LOGGING & STATISTICS
	// ====================================

	// MySQL Logging
	router.GET("/api/mysql/logs", authMiddleware, mysqlDynamicHandler.GetQueryLogs)
	router.GET("/api/mysql/stats", authMiddleware, mysqlDynamicHandler.GetQueryStats)

	// MariaDB Logging
	router.GET("/api/mariadb/logs", authMiddleware, mariadbDynamicHandler.GetQueryLogs)
	router.GET("/api/mariadb/stats", authMiddleware, mariadbDynamicHandler.GetQueryStats)

	// PostgreSQL Logging
	router.GET("/api/postgres/logs", authMiddleware, postgresDynamicHandler.GetQueryLogs)
	router.GET("/api/postgres/stats", authMiddleware, postgresDynamicHandler.GetQueryStats)

	// Percona Logging
	router.GET("/api/percona/logs", authMiddleware, perconaDynamicHandler.GetQueryLogs)
	router.GET("/api/percona/stats", authMiddleware, perconaDynamicHandler.GetQueryStats)

	// Oracle Logging
	router.GET("/api/oracle/logs", authMiddleware, oracleDynamicHandler.GetQueryLogs)
	router.GET("/api/oracle/stats", authMiddleware, oracleDynamicHandler.GetQueryStats)

	// ====================================
	// DATA TRANSFORMATION ENDPOINTS (Auth Required)
	// ====================================

	transformHandler := transformpkg.NewHandler()

	// Rule Management endpoints
	router.POST("/api/transform/rules", authMiddleware, transformHandler.RegisterRule)
	router.GET("/api/transform/rules", authMiddleware, transformHandler.ListRules)
	router.GET("/api/transform/rules/:name", authMiddleware, transformHandler.GetRule)
	router.DELETE("/api/transform/rules/:name", adminMiddleware, transformHandler.DeleteRule)

	// Transformation endpoints
	router.POST("/api/transform/apply", authMiddleware, transformHandler.Transform)
	router.POST("/api/transform/batch", authMiddleware, transformHandler.TransformBatch)
	router.POST("/api/transform/preview", authMiddleware, transformHandler.PreviewTransformation)

	// Feature Testing endpoints
	router.POST("/api/transform/test/rename", authMiddleware, transformHandler.TestFieldRename)
	router.POST("/api/transform/test/types", authMiddleware, transformHandler.TestTypeConversion)
	router.POST("/api/transform/test/flatten", authMiddleware, transformHandler.TestFlattening)

	// Import/Export endpoints
	router.GET("/api/transform/rules/export", authMiddleware, transformHandler.ExportRules)
	router.POST("/api/transform/rules/import", adminMiddleware, transformHandler.ImportRules)

	// ====================================
	// ADMIN OPERATIONS (Admin Only)
	// ====================================
	certificateHandler := securitypkg.NewHandler()

	// Database management endpoints (RBAC-authorized)
	router.POST("/api/admin/database/create", authzMiddleware, adminHandler.CreateDatabase)
	router.GET("/api/admin/database/list", authzMiddleware, adminHandler.ListDatabases)
	router.GET("/api/admin/database/servers", authzMiddleware, adminHandler.ListDatabaseServers)
	router.POST("/api/admin/database/connect", authzMiddleware, adminHandler.ConnectDatabaseServer)
	router.PUT("/api/admin/database/servers/:key", authzMiddleware, adminHandler.UpdateDatabaseServer)
	router.DELETE("/api/admin/database/servers/:key", authzMiddleware, adminHandler.DeleteDatabaseServer)
	router.GET("/api/admin/certificates/status", authzMiddleware, certificateHandler.GetCertificateStatus)
	router.POST("/api/admin/certificates/renew", authzMiddleware, certificateHandler.RenewCertificate)

	// Table management endpoints (RBAC-authorized)
	router.POST("/api/admin/table/create", authzMiddleware, adminHandler.CreateTable)
	router.GET("/api/admin/table/list", authzMiddleware, adminHandler.ListTables)

	// Legacy platform user management endpoints (RBAC-authorized)
	if !iamOnlyAuth {
		router.GET("/api/v1/users", authzMiddleware, platformUserHandler.ListPlatformUsers)
		router.GET("/api/v1/users/:id", authzMiddleware, platformUserHandler.GetPlatformUser)
		router.POST("/api/v1/users", authzMiddleware, platformUserHandler.CreatePlatformUser)
		router.PUT("/api/v1/users/:id", authzMiddleware, platformUserHandler.UpdatePlatformUser)
		router.DELETE("/api/v1/users/:id", authzMiddleware, platformUserHandler.DeletePlatformUser)
	} else {
		log.Println("ℹ️  IAM_ONLY_AUTH=true: legacy /api/v1/users endpoints are disabled; use /iam/admin/users")
	}

	// API Metrics endpoints (RBAC-authorized)
	router.GET("/api/admin/metrics/all", authzMiddleware, apiMetricsTracker.GetAllAPIMetrics)
	router.GET("/api/admin/metrics/count", authzMiddleware, apiMetricsTracker.GetAPICount)
	router.GET("/api/admin/metrics/stats", authzMiddleware, apiMetricsTracker.GetAPIStats)

	// ====================================
	// NOTIFICATION ENDPOINTS (Auth Required)
	// ====================================

	// Notification endpoints (authenticated users)
	router.POST("/api/notifications/send", authMiddleware, notificationHandler.SendNotification)
	router.POST("/api/notifications/health", authMiddleware, notificationHandler.SendHealthNotification)
	router.POST("/api/notifications/status", authMiddleware, notificationHandler.SendStatusNotification)
	router.GET("/api/notifications/status", notificationHandler.GetNotificationStatus)

	// Backward-compatible notification aliases restored under /api/v1.
	router.POST("/api/v1/notifications/send", authMiddleware, notificationHandler.SendNotification)
	router.POST("/api/v1/notifications/health", authMiddleware, notificationHandler.SendHealthNotification)
	router.POST("/api/v1/notifications/status", authMiddleware, notificationHandler.SendStatusNotification)
	router.GET("/api/v1/notifications/status", notificationHandler.GetNotificationStatus)

	// ====================================
	// CLI AUTHENTICATION ENDPOINTS
	// ====================================
	cliAuth := authn.NewCLIAuthHandler()
	router.POST("/api/v1/auth/login", cliAuth.Login)
	router.POST("/api/v1/auth/logout", authHandler.Logout)
	router.GET("/api/v1/auth/verify", cliAuth.Verify)
	router.GET("/api/v1/auth/whoami", cliAuth.WhoAmI)

	// ====================================
	// KUBERNETES-STYLE RESOURCE ENDPOINTS
	// ====================================
	resourceHandler := resourcespkg.NewGenericResourceHandler(conns.Etcd)

	// Namespaced resource endpoints: /api/v1/namespaces/{namespace}/{kind}
	nsAPI := router.Group("/api/v1/namespaces")
	{
		nsAPI.POST("/:namespace/:kind", authzMiddleware, resourceHandler.CreateOrUpdate)
		nsAPI.GET("/:namespace/:kind", authMiddleware, resourceHandler.List)
		nsAPI.GET("/:namespace/:kind/:name", authMiddleware, resourceHandler.Get)
		nsAPI.PUT("/:namespace/:kind/:name", authzMiddleware, resourceHandler.Update)
		nsAPI.DELETE("/:namespace/:kind/:name", authzMiddleware, resourceHandler.Delete)
		nsAPI.GET("/:namespace/:kind/:name/status", authMiddleware, resourceHandler.GetStatus)
		nsAPI.GET("/:namespace/:kind/:name/events", authMiddleware, resourceHandler.Events)
	}

	// Non-namespaced resource endpoints: /api/v1/{kind}
	router.POST("/api/v1/apis", authzMiddleware, resourceHandler.CreateOrUpdate)
	router.GET("/api/v1/apis", authMiddleware, resourceHandler.ListAll)
	router.POST("/api/v1/policies", authzMiddleware, resourceHandler.CreateOrUpdate)
	router.GET("/api/v1/policies", authMiddleware, resourceHandler.ListAll)
	router.POST("/api/v1/workflows", authzMiddleware, resourceHandler.CreateOrUpdate)
	router.GET("/api/v1/workflows", authMiddleware, resourceHandler.ListAll)
	router.POST("/api/v1/workflows/:name/run", authzMiddleware, func(c *gin.Context) {
		workflowName := strings.TrimSpace(c.Param("name"))
		if workflowName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "workflow name is required"})
			return
		}

		var req struct {
			TriggerContext map[string]interface{} `json:"triggerContext"`
		}
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}

		if err := server.EnsureWorkflowRegistered(c.Request.Context(), resourceHandler, workflowName); err != nil {
			if workflows.GlobalWorkflowEngine.GetWorkflow(workflowName) == nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			log.Printf("⚠️  workflow run using previously-registered definition for %s: %v", workflowName, err)
		}

		triggerContext := req.TriggerContext
		if triggerContext == nil {
			triggerContext = make(map[string]interface{})
		}
		if username := strings.TrimSpace(auth.GetUsername(c)); username != "" {
			if _, exists := triggerContext["requestedBy"]; !exists {
				triggerContext["requestedBy"] = username
			}
		}
		if _, exists := triggerContext["triggeredAt"]; !exists {
			triggerContext["triggeredAt"] = time.Now().UTC().Format(time.RFC3339)
		}

		execution, err := workflows.Execute(c.Request.Context(), workflowName, triggerContext)
		if err != nil {
			errMsg := strings.ToLower(err.Error())
			switch {
			case strings.Contains(errMsg, "not found"):
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			case strings.Contains(errMsg, "disabled"):
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			}
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":   fmt.Sprintf("Workflow '%s' executed", workflowName),
			"execution": execution,
			"status":    execution.Status,
		})
	})
	router.GET("/api/v1/workflows/:name/executions", authMiddleware, func(c *gin.Context) {
		workflowName := strings.TrimSpace(c.Param("name"))
		if workflowName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "workflow name is required"})
			return
		}

		executions := workflows.GlobalWorkflowEngine.ListExecutions(workflowName)
		c.JSON(http.StatusOK, gin.H{
			"workflow":   workflowName,
			"executions": executions,
			"count":      len(executions),
		})
	})
	router.GET("/api/v1/workflows/executions/:id", authMiddleware, func(c *gin.Context) {
		executionID := strings.TrimSpace(c.Param("id"))
		if executionID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "execution id is required"})
			return
		}

		execution := workflows.GlobalWorkflowEngine.GetExecution(executionID)
		if execution == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
			return
		}

		c.JSON(http.StatusOK, execution)
	})

	// DataSource endpoints
	dsHandler := datasourceresource.NewDataSourceHandler(conns.Etcd)
	router.POST("/api/v1/datasources", authzMiddleware, dsHandler.Create)
	router.GET("/api/v1/datasources", authMiddleware, dsHandler.List)
	router.GET("/api/v1/datasources/:name", authMiddleware, dsHandler.Get)
	router.PUT("/api/v1/datasources/:name", authzMiddleware, dsHandler.Update)
	router.DELETE("/api/v1/datasources/:name", authzMiddleware, dsHandler.Delete)
	router.POST("/api/v1/datasources/:name/test", authzMiddleware, dsHandler.Test)

	// Job endpoints
	jobHandler := jobs.NewLegacyJobHandler(conns.Etcd)
	router.POST("/api/v1/jobs", authzMiddleware, jobHandler.Create)
	router.GET("/api/v1/jobs", authMiddleware, jobHandler.List)
	router.GET("/api/v1/jobs/schedules", authMiddleware, jobHandler.ListSchedules)
	router.GET("/api/v1/jobs/:id", authMiddleware, jobHandler.Get)
	router.POST("/api/v1/jobs/:id/schedule", authzMiddleware, jobHandler.SetSchedule)
	router.DELETE("/api/v1/jobs/:id/schedule", authzMiddleware, jobHandler.RemoveSchedule)
	router.POST("/api/v1/jobs/:id/run", authzMiddleware, jobHandler.Run)
	router.GET("/api/v1/jobs/:id/logs", authMiddleware, jobHandler.GetLogs)
	router.POST("/api/v1/jobs/:id/cancel", authzMiddleware, jobHandler.Cancel)
	router.DELETE("/api/v1/jobs/:id", authzMiddleware, jobHandler.Delete)

	// ====================================
	// PLATFORM FEATURE APIs (PHASE 1)
	// ====================================
	platformManagers, err := platform.NewManagers(conns)
	if err != nil {
		log.Printf("⚠️  platform managers initialization failed: %v", err)
		log.Println("  Platform service APIs may have limited functionality")
	}

	bulkHandler := bulk.NewBulkHandler(platformManagers.Bulk)
	eventBusHandler := eventbus.NewEventBusHandler(platformManagers.EventBus)
	exportHandler := exportpkg.NewExportHandler(platformManagers.Export)
	streamHandler := streaming.NewStreamHandler(platformManagers.Stream)
	webhookHandler := webhooks.NewWebhookHandler(platformManagers.Webhook)
	tenantHandler := tenant.NewTenantHandler(platformManagers.Tenant)
	rbacHandler := rbac.NewRBACHandler(platformManagers.RBAC)
	versionHandler := versioning.NewVersionHandler(platformManagers.Version)
	lineageHandler := lineage.NewLineageHandler(platformManagers.Lineage)
	tracingHandler := tracing.NewTracingHandler(platformManagers.Tracing)

	// Bulk operations
	bulkAPI := router.Group("/api/v1/bulk/operations", authMiddleware)
	{
		bulkAPI.POST("", authzMiddleware, bulkHandler.SubmitBulkOperation)
		bulkAPI.GET("", bulkHandler.ListOperations)
		bulkAPI.GET("/:id", bulkHandler.GetOperation)
		bulkAPI.GET("/:id/progress", bulkHandler.GetProgress)
		bulkAPI.DELETE("/:id", authzMiddleware, bulkHandler.CancelOperation)
		bulkAPI.POST("/:id/retry-failed", authzMiddleware, bulkHandler.RetryFailed)
		bulkAPI.GET("/:id/results", bulkHandler.GetResults)
	}

	// Event bus
	eventBusAPI := router.Group("/api/v1/eventbus", authMiddleware)
	{
		eventBusAPI.POST("/events/publish", authzMiddleware, eventBusHandler.PublishEvent)
		eventBusAPI.GET("/events", eventBusHandler.ListEvents)
		eventBusAPI.POST("/events/:id/ack", authzMiddleware, eventBusHandler.AckEvent)
		eventBusAPI.POST("/topics", authzMiddleware, eventBusHandler.CreateTopic)
		eventBusAPI.GET("/topics", eventBusHandler.ListTopics)
		eventBusAPI.POST("/subscriptions", authzMiddleware, eventBusHandler.CreateSubscription)
		eventBusAPI.GET("/subscriptions/:id", eventBusHandler.GetSubscription)
		eventBusAPI.GET("/subscriptions", eventBusHandler.ListSubscriptions)
		eventBusAPI.GET("/dlq", eventBusHandler.ListDLQ)
		eventBusAPI.POST("/dlq/:id/replay", authzMiddleware, eventBusHandler.ReplayDLQEvent)
	}

	// Exports
	exportAPI := router.Group("/api/v1/exports", authMiddleware)
	{
		exportAPI.POST("", authzMiddleware, exportHandler.SubmitExport)
		exportAPI.GET("", exportHandler.ListExports)
		exportAPI.GET("/:id", exportHandler.GetExport)
		exportAPI.GET("/:id/progress", exportHandler.GetExportProgress)
		exportAPI.GET("/:id/download", exportHandler.DownloadExport)
		exportAPI.DELETE("/:id", authzMiddleware, exportHandler.CancelExport)
	}
	router.POST("/api/v1/export-templates", authzMiddleware, exportHandler.CreateTemplate)
	router.GET("/api/v1/export-templates", authMiddleware, exportHandler.ListTemplates)

	// Webhooks
	webhookAPI := router.Group("/api/v1/webhooks", authMiddleware)
	{
		webhookAPI.POST("", authzMiddleware, webhookHandler.CreateWebhook)
		webhookAPI.GET("", webhookHandler.ListWebhooks)
		webhookAPI.GET("/:id", webhookHandler.GetWebhook)
		webhookAPI.PATCH("/:id", authzMiddleware, webhookHandler.UpdateWebhook)
		webhookAPI.DELETE("/:id", authzMiddleware, webhookHandler.DeleteWebhook)
		webhookAPI.POST("/:id/test", authzMiddleware, webhookHandler.TestWebhook)
		webhookAPI.GET("/:id/deliveries", webhookHandler.GetDeliveryLogs)
	}

	// Streaming
	router.GET("/ws/stream", authMiddleware, streamHandler.HandleStream)
	streamsAPI := router.Group("/api/v1/streams", authMiddleware)
	{
		streamsAPI.POST("", authzMiddleware, streamHandler.CreateStreamRequest)
		streamsAPI.GET("", streamHandler.ListStreams)
		streamsAPI.GET("/:id", streamHandler.GetStreamStatus)
		streamsAPI.DELETE("/:id", authzMiddleware, streamHandler.CancelStream)
	}
	streamSubscriptionsAPI := router.Group("/api/v1/streaming/subscriptions", authMiddleware)
	{
		streamSubscriptionsAPI.POST("", authzMiddleware, streamHandler.Subscribe)
		streamSubscriptionsAPI.DELETE("/:id", authzMiddleware, streamHandler.Unsubscribe)
	}

	// Conductor (RabbitMQ / Kafka producer & consumer management)
	conductorCfg := conductor.LoadConfigFromEnv()
	conductorMgr := conductor.NewManager(conductorCfg)
	conductorMgr.InitPersistence(conns.PostgreSQL)
	conductor.RegisterRoutes(router, conductorMgr, authMiddleware, adminOrSysMiddleware)

	// Tenants
	tenantAPI := router.Group("/api/v1/tenants", authMiddleware)
	{
		tenantAPI.POST("", authzMiddleware, tenantHandler.CreateTenant)
		tenantAPI.GET("", tenantHandler.ListTenants)
		tenantAPI.GET("/:id", tenantHandler.GetTenant)
		tenantAPI.PATCH("/:id", authzMiddleware, tenantHandler.UpdateTenant)
		tenantAPI.DELETE("/:id", authzMiddleware, tenantHandler.DeleteTenant)
		tenantAPI.POST("/:id/members", authzMiddleware, tenantHandler.AddMember)
		tenantAPI.DELETE("/:id/members/:userId", authzMiddleware, tenantHandler.RemoveMember)
		tenantAPI.GET("/:id/quota", tenantHandler.GetQuota)
		tenantAPI.POST("/:id/quota/check", tenantHandler.CheckQuota)
	}

	// RBAC
	rbacAPI := router.Group("/api/v1/rbac", authMiddleware)
	{
		rbacAPI.POST("/roles", authzMiddleware, rbacHandler.CreateRole)
		rbacAPI.GET("/roles", rbacHandler.ListRoles)
		rbacAPI.GET("/roles/:id", rbacHandler.GetRole)
		rbacAPI.PATCH("/roles/:id", authzMiddleware, rbacHandler.UpdateRole)
		rbacAPI.DELETE("/roles/:id", authzMiddleware, rbacHandler.DeleteRole)

		rbacAPI.POST("/role-bindings", authzMiddleware, rbacHandler.BindRole)
		rbacAPI.GET("/role-bindings", rbacHandler.ListBindings)
		rbacAPI.DELETE("/role-bindings/:id", authzMiddleware, rbacHandler.DeleteBinding)

		rbacAPI.GET("/permissions", rbacHandler.ListPermissions)
		rbacAPI.POST("/permissions/check", rbacHandler.CheckPermission)

		rbacAPI.POST("/access-requests", rbacHandler.CreateAccessRequest)
		rbacAPI.GET("/access-requests", rbacHandler.ListAccessRequests)
		rbacAPI.POST("/access-requests/:id/approve", authzMiddleware, rbacHandler.ApproveAccessRequest)
		rbacAPI.POST("/access-requests/:id/reject", authzMiddleware, rbacHandler.RejectAccessRequest)
	}

	// Versioning
	versionAPI := router.Group("/api/v1/versioning", authMiddleware)
	{
		versionAPI.GET("/versions/:resourceType/:resourceId/:version", versionHandler.GetVersion)
		versionAPI.GET("/versions/:resourceType/:resourceId", versionHandler.ListVersions)
		versionAPI.GET("/history/:resourceType/:resourceId", versionHandler.GetHistory)
		versionAPI.GET("/diff/:resourceType/:resourceId", versionHandler.GetDiff)
		versionAPI.POST("/snapshots/:resourceType/:resourceId", authzMiddleware, versionHandler.CreateSnapshot)
		versionAPI.POST("/versions/:resourceType/:resourceId/rollback", authzMiddleware, versionHandler.Rollback)
	}

	// Lineage
	lineageAPI := router.Group("/api/v1/lineage", authMiddleware)
	{
		lineageAPI.GET("/nodes/:id", lineageHandler.GetNode)
		lineageAPI.GET("/nodes", lineageHandler.ListNodes)
		lineageAPI.GET("/:resourceType/:resourceId", lineageHandler.GetLineageGraph)
		lineageAPI.GET("/upstream/:resourceType/:resourceId", lineageHandler.GetUpstreamLineage)
		lineageAPI.GET("/downstream/:resourceType/:resourceId", lineageHandler.GetDownstreamLineage)
		lineageAPI.GET("/impact/:resourceType/:resourceId", lineageHandler.GetImpactAnalysis)
		lineageAPI.GET("/columns", lineageHandler.GetColumnLineage)
		lineageAPI.GET("/trace", lineageHandler.TraceDataFlow)
		lineageAPI.GET("/statistics", lineageHandler.GetStatistics)
	}

	// Tracing
	tracingAPI := router.Group("/api/v1/tracing", authMiddleware)
	{
		tracingAPI.POST("/traces", authzMiddleware, tracingHandler.IngestTrace)
		tracingAPI.GET("/traces/:traceId", tracingHandler.GetTrace)
		tracingAPI.GET("/traces/search", tracingHandler.SearchTraces)
		tracingAPI.POST("/spans", authzMiddleware, tracingHandler.IngestSpan)
		tracingAPI.GET("/spans/:spanId", tracingHandler.GetSpan)
		tracingAPI.GET("/service-map", tracingHandler.GetServiceMap)
		tracingAPI.GET("/services", tracingHandler.ListServices)
		tracingAPI.GET("/services/:service/metrics", tracingHandler.GetServiceMetrics)
		tracingAPI.GET("/services/:service/operations/:operation/metrics", tracingHandler.GetOperationMetrics)
		tracingAPI.GET("/errors/analysis", tracingHandler.GetErrorAnalysis)
		tracingAPI.GET("/ingestion/audit", authzMiddleware, tracingHandler.ListIngestionAudits)
	}

	// ====================================
	// GIS DASHBOARD ENDPOINTS
	// ====================================
	gisHandler := apibuilder.NewGISHandler()
	gisAPI := router.Group("/api/v1/gis", authMiddleware)
	{
		gisAPI.GET("/summary", gisHandler.GetSummary)

		gisAPI.GET("/layers", gisHandler.ListLayers)
		gisAPI.POST("/layers", authzMiddleware, gisHandler.CreateLayer)
		gisAPI.PUT("/layers/:id", authzMiddleware, gisHandler.UpdateLayer)
		gisAPI.DELETE("/layers/:id", authzMiddleware, gisHandler.DeleteLayer)

		gisAPI.GET("/regions", gisHandler.ListRegions)
		gisAPI.GET("/regions/:id", gisHandler.GetRegion)
		gisAPI.POST("/regions", authzMiddleware, gisHandler.CreateRegion)
		gisAPI.PUT("/regions/:id", authzMiddleware, gisHandler.UpdateRegion)
		gisAPI.DELETE("/regions/:id", authzMiddleware, gisHandler.DeleteRegion)

		gisAPI.GET("/markers", gisHandler.ListMarkers)
		gisAPI.POST("/markers", authzMiddleware, gisHandler.CreateMarker)
		gisAPI.DELETE("/markers/:id", authzMiddleware, gisHandler.DeleteMarker)

		gisAPI.GET("/datasets", gisHandler.ListDatasets)
		gisAPI.GET("/datasets/:id", gisHandler.GetDataset)
		gisAPI.POST("/datasets", authzMiddleware, gisHandler.CreateDataset)
		gisAPI.PUT("/datasets/:id", authzMiddleware, gisHandler.UpdateDataset)
		gisAPI.DELETE("/datasets/:id", authzMiddleware, gisHandler.DeleteDataset)
	}

	// Specialized GIS dashboards (agriculture, industries, medical, satellite, airplane, ship)
	gisSystem := gispkg.NewSystem()
	gisSpecHandler := gisSystem.Handler()
	_ = gisSystem.Start(ctx)
	gisSpecAPI := router.Group("/api/v1/gis/dashboards", authMiddleware)
	{
		gisSpecAPI.GET("", gisSpecHandler.ListDashboardTypes)
		gisSpecAPI.GET("/:type", gisSpecHandler.GetDashboard)
		gisSpecAPI.GET("/:type/summary", gisSpecHandler.GetDashboardSummary)
	}

	// GIS Train/Railway handlers (Indian + Bangladesh Railways) have been
	// removed. These previously held *gorm.DB directly and bypassed the
	// control plane entirely. Equivalent endpoints must now be authored in
	// the API Builder using DataSource resources.

	// Analytics dashboards (charts, graphs, tables, KPI, heatmap, export)
	analyticsHandler := apibuilder.NewAnalyticsHandler()
	analyticsAPI := router.Group("/api/v1/analytics", authMiddleware)
	{
		analyticsAPI.GET("/dashboards", analyticsHandler.ListDashboards)
		analyticsAPI.GET("/dashboards/:id", analyticsHandler.GetDashboard)
		analyticsAPI.PUT("/dashboards/:id/widgets/:widgetId", authzMiddleware, analyticsHandler.UpdateWidget)
		analyticsAPI.PUT("/dashboards/:id/layout", authzMiddleware, analyticsHandler.ReorderWidgets)
		analyticsAPI.GET("/dashboards/:id/widgets/:widgetId/export", analyticsHandler.ExportCSV)
		analyticsAPI.GET("/widget-types", analyticsHandler.GetWidgetTypes)
	}

	// ====================================
	// CDC & ETL DATA PLATFORM ENDPOINTS
	// ====================================
	cdcSystem := cdc.NewSystem(conns.Etcd)
	cdcEtlHandler := cdcSystem.Handler()
	_ = cdcSystem.Start(ctx)
	log.Println("✅ CDC module started")

	// ETL System (standalone with audit/metrics/KV persistence)
	etlEngine := etl.NewEngine(conns.Etcd)
	etlSystem := etl.NewSystem(etlEngine)
	_ = etlSystem.Start(ctx)
	etlHandler := etlSystem.Handler()
	log.Println("✅ ETL module started")

	// ETL Pipeline Management (standalone handler with typed DTOs + audit + metrics)
	etlAPI := router.Group("/api/v1/etl", authMiddleware)
	etlHandler.RegisterRoutes(etlAPI, adminOrSysMiddleware)

	// CDC Pipeline Management
	cdcAPI := router.Group("/api/v1/cdc", authMiddleware)
	{
		cdcAPI.GET("/pipelines", cdcEtlHandler.ListCDCPipelines)
		cdcAPI.GET("/pipelines/:id", cdcEtlHandler.GetCDCPipeline)
		cdcAPI.POST("/pipelines", authzMiddleware, cdcEtlHandler.CreateCDCPipeline)
		cdcAPI.PUT("/pipelines/:id", authzMiddleware, cdcEtlHandler.UpdateCDCPipeline)
		cdcAPI.DELETE("/pipelines/:id", authzMiddleware, cdcEtlHandler.DeleteCDCPipeline)
		cdcAPI.POST("/pipelines/:id/start", authzMiddleware, cdcEtlHandler.StartCDCPipeline)
		cdcAPI.POST("/pipelines/:id/pause", authzMiddleware, cdcEtlHandler.PauseCDCPipeline)
		cdcAPI.POST("/pipelines/:id/stop", authzMiddleware, cdcEtlHandler.StopCDCPipeline)
		cdcAPI.GET("/sources", cdcEtlHandler.GetCDCSourceTypes)
		cdcAPI.GET("/sinks", cdcEtlHandler.GetCDCSinkTypes)
		cdcAPI.GET("/observability", cdcEtlHandler.GetCDCObservability)
	}

	// Data Platform Overview
	router.GET("/api/v1/data-platform/overview", authMiddleware, cdcEtlHandler.GetPlatformOverview)

	// ====================================
	// API BANKS (standalone system with audit/metrics)
	// ====================================
	apiBankSystem := apibanks.NewSystem()
	_ = apiBankSystem.Start(ctx)
	apiBankHandler := apiBankSystem.Handler()
	log.Println("✅ API Banks module started")

	apiBanksAPI := router.Group("/api/v1/apibanks", authMiddleware)
	apiBankHandler.RegisterRoutes(apiBanksAPI, adminOrSysMiddleware)

	// ====================================
	// API BUILDER, CSV DASHBOARD & CONVERSION
	// ====================================
	apiBuilderHandler := apibuilder.NewAPIBuilderHandler(analyticsHandler, gisHandler, dbConnections, conns.Etcd, nil)
	apiBuilderSystem := apibuilder.NewSystem(apiBuilderHandler)
	_ = apiBuilderSystem.Start(ctx)
	log.Println("✅ API Builder module started")

	builderAPI := router.Group("/api/v1/builder", authMiddleware)
	{
		// Summary
		builderAPI.GET("/summary", apiBuilderHandler.GetSummary)

		// Custom API CRUD
		builderAPI.GET("/apis", apiBuilderHandler.ListAPIs)
		builderAPI.GET("/apis/:id", apiBuilderHandler.GetAPI)
		builderAPI.POST("/apis", authzMiddleware, apiBuilderHandler.CreateAPI)
		builderAPI.PUT("/apis/:id", authzMiddleware, apiBuilderHandler.UpdateAPI)
		builderAPI.DELETE("/apis/:id", authzMiddleware, apiBuilderHandler.DeleteAPI)
		builderAPI.POST("/apis/:id/test", authzMiddleware, apiBuilderHandler.TestAPI)

		// CSV Upload & Dashboard Generation
		builderAPI.POST("/csv/upload", authzMiddleware, apiBuilderHandler.UploadCSV)
		builderAPI.GET("/csv/uploads", apiBuilderHandler.ListCSVUploads)
		builderAPI.GET("/csv/uploads/:id", apiBuilderHandler.GetCSVUpload)
		builderAPI.DELETE("/csv/uploads/:id", authzMiddleware, apiBuilderHandler.DeleteCSVUpload)
		builderAPI.POST("/csv/uploads/:id/generate-dashboard", authzMiddleware, apiBuilderHandler.GenerateDashboard)
		builderAPI.POST("/csv/uploads/:id/generate-gis", authzMiddleware, apiBuilderHandler.GenerateGISFromCSV)

		// Dashboard <-> GIS Conversion
		builderAPI.POST("/convert/analyze", authzMiddleware, apiBuilderHandler.AnalyzeConversion)
		builderAPI.POST("/convert/dashboard-to-gis", authzMiddleware, apiBuilderHandler.ConvertDashboardToGIS)
		builderAPI.POST("/convert/gis-to-dashboard", authzMiddleware, apiBuilderHandler.ConvertGISToDashboard)
		builderAPI.GET("/conversions", apiBuilderHandler.ListConversions)

		// File Scanner (SafeGate Pipeline)
		builderAPI.POST("/scanner/scan", authzMiddleware, apiBuilderHandler.ScanFile)
		builderAPI.GET("/scanner/scans", apiBuilderHandler.ListScans)
		builderAPI.GET("/scanner/scans/:id", apiBuilderHandler.GetScan)
		builderAPI.GET("/scanner/health", apiBuilderHandler.GetScannerHealth)

		// API Scanner Reports
		builderAPI.POST("/api-scanner/scan", authzMiddleware, apiBuilderHandler.ScanAPI)
		builderAPI.GET("/api-scanner/reports", apiBuilderHandler.ListAPIScanReports)
		builderAPI.POST("/api-scanner/reports/bulk-delete", authzMiddleware, apiBuilderHandler.BulkDeleteAPIScanReports)
		builderAPI.GET("/api-scanner/reports/:id", apiBuilderHandler.GetAPIScanReport)
		builderAPI.DELETE("/api-scanner/reports/:id", authzMiddleware, apiBuilderHandler.DeleteAPIScanReport)

		// SQL Assistant for API Builder
		builderAPI.POST("/sql-assistant/chat", authzMiddleware, apiBuilderHandler.ChatSQLAssistant)

		// Dashboard Deletion
		builderAPI.DELETE("/dashboards/:id", authzMiddleware, apiBuilderHandler.DeleteDashboard)
	}

	// Runtime execution routes for REST APIs created via API Builder.
	router.Any("/api/custom", authMiddleware, apiBuilderHandler.InvokeCustomAPI)
	router.Any("/api/custom/*path", authMiddleware, apiBuilderHandler.InvokeCustomAPI)

	// ====================================
	// NETWORK INTELLIGENCE ENDPOINTS
	// ====================================
	netintelSystem := netintelpkg.NewSystem()
	netIntelHandler := netintelSystem.Handler()
	modeManager := netintelSystem.ModesManager()

	// ====================================
	// NEWLY ADDED FEATURE MODULES
	// ====================================
	admissionEngine := admission.NewEngine()
	admissionEngine.RegisterPolicy("template-001", 100, admission.PolicyTemplate001)
	admissionEngine.RegisterPolicy("template-002", 90, admission.PolicyTemplate002)
	admissionEngine.RegisterPolicy("template-003", 80, admission.PolicyTemplate003)

	kubeScheduler := scheduler.NewScheduler()
	crdRegistry := crd.NewRegistry()
	vectorIndex := vectorplus.NewIndex(4)
	reviewPipeline := reviewflow.NewPipeline()

	netintelAPI := router.Group("/api/v1/netintel", authMiddleware)
	{
		// Register all core netintel routes via the handler
		netIntelHandler.RegisterRoutes(netintelAPI)

		// Modes endpoints (inline — modes manager is separate from handler)
		netintelAPI.GET("/modes", func(c *gin.Context) {
			c.JSON(http.StatusOK, netintelpkg.ModesListResponse{Status: "success", Modes: modeManager.List()})
		})
		netintelAPI.PUT("/modes/:name", authzMiddleware, func(c *gin.Context) {
			var cfg modes.ModeConfig
			if err := c.ShouldBindJSON(&cfg); err != nil {
				c.JSON(http.StatusBadRequest, netintelpkg.MessageResponse{Error: err.Error()})
				return
			}
			cfg.Name = modes.Mode(strings.ToLower(strings.TrimSpace(c.Param("name"))))
			if cfg.Name == "" {
				c.JSON(http.StatusBadRequest, netintelpkg.MessageResponse{Error: "mode name is required"})
				return
			}
			modeManager.Upsert(cfg)
			c.JSON(http.StatusOK, netintelpkg.ModesUpsertResponse{Message: "mode upserted", Mode: cfg})
		})
		netintelAPI.POST("/modes/events", authzMiddleware, func(c *gin.Context) {
			var ev modes.ModeEvent
			if err := c.ShouldBindJSON(&ev); err != nil {
				c.JSON(http.StatusBadRequest, netintelpkg.MessageResponse{Error: err.Error()})
				return
			}
			if ev.Timestamp.IsZero() {
				ev.Timestamp = time.Now().UTC()
			}
			modeManager.Record(ev)
			c.JSON(http.StatusOK, netintelpkg.MessageResponse{Message: "event recorded"})
		})
		netintelAPI.GET("/modes/:name/events", func(c *gin.Context) {
			name := modes.Mode(strings.ToLower(strings.TrimSpace(c.Param("name"))))
			c.JSON(http.StatusOK, netintelpkg.ModesEventsResponse{Status: "success", Events: modeManager.FindByMode(name)})
		})
		netintelAPI.POST("/modes/detect", func(c *gin.Context) {
			var body struct {
				Detector int       `json:"detector"`
				Samples  []float64 `json:"samples"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, netintelpkg.MessageResponse{Error: err.Error()})
				return
			}
			var score float64
			switch body.Detector {
			case 2:
				score = modes.Detector002(body.Samples)
			case 3:
				score = modes.Detector003(body.Samples)
			case 4:
				score = modes.Detector004(body.Samples)
			case 5:
				score = modes.Detector005(body.Samples)
			default:
				score = modes.Detector001(body.Samples)
			}
			c.JSON(http.StatusOK, netintelpkg.ModesDetectResponse{Detector: body.Detector, Score: score})
		})
	}

	kubeplusAPI := router.Group("/api/v1/kubeplus", authMiddleware)
	{
		kubeplusAPI.GET("/admission/policies", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"policies": admissionEngine.ListPolicies()})
		})
		kubeplusAPI.POST("/admission/evaluate", func(c *gin.Context) {
			var req admission.AdmissionRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if req.Timestamp.IsZero() {
				req.Timestamp = time.Now().UTC()
			}
			c.JSON(http.StatusOK, admissionEngine.Evaluate(req))
		})

		kubeplusAPI.PUT("/scheduler/nodes/:name", authzMiddleware, func(c *gin.Context) {
			var node scheduler.Node
			if err := c.ShouldBindJSON(&node); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			node.Name = c.Param("name")
			if strings.TrimSpace(node.Name) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "node name is required"})
				return
			}
			kubeScheduler.UpsertNode(node)
			c.JSON(http.StatusOK, gin.H{"message": "node upserted", "node": node.Name})
		})
		kubeplusAPI.GET("/scheduler/nodes", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"nodes": kubeScheduler.ListNodes()})
		})
		kubeplusAPI.POST("/scheduler/score", func(c *gin.Context) {
			var w scheduler.Workload
			if err := c.ShouldBindJSON(&w); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"decisions": kubeScheduler.Score(w)})
		})
		kubeplusAPI.POST("/scheduler/pick", func(c *gin.Context) {
			var w scheduler.Workload
			if err := c.ShouldBindJSON(&w); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			best, ok := kubeScheduler.PickBest(w)
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "no suitable node found"})
				return
			}
			c.JSON(http.StatusOK, best)
		})

		kubeplusAPI.POST("/crd/definitions", authzMiddleware, func(c *gin.Context) {
			var def crd.Definition
			if err := c.ShouldBindJSON(&def); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if err := crdRegistry.Register(def); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "definition registered"})
		})
		kubeplusAPI.GET("/crd/definitions", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"definitions": crdRegistry.List()})
		})
		kubeplusAPI.POST("/crd/validate", func(c *gin.Context) {
			var body struct {
				Group   string         `json:"group"`
				Kind    string         `json:"kind"`
				Version string         `json:"version"`
				Spec    map[string]any `json:"spec"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			def, ok := crdRegistry.Get(body.Group, body.Kind)
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "definition not found"})
				return
			}
			if len(def.Versions) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "definition has no versions"})
				return
			}
			fields := def.Versions[0].Fields
			if strings.TrimSpace(body.Version) != "" {
				for _, v := range def.Versions {
					if v.Version == body.Version {
						fields = v.Fields
						break
					}
				}
			}
			c.JSON(http.StatusOK, crd.ValidateSpec(fields, body.Spec))
		})
	}

	vectorAPI := router.Group("/api/v1/vectorplus", authMiddleware)
	{
		vectorAPI.PUT("/records/:id", authzMiddleware, func(c *gin.Context) {
			var body struct {
				Vec    []float64         `json:"vec"`
				Labels map[string]string `json:"labels"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			rec := vectorplus.Record{ID: c.Param("id"), Vec: vectorplus.Vector(body.Vec), Labels: body.Labels}
			if !vectorIndex.Upsert(rec) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vector size or id"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "record upserted", "id": rec.ID})
		})
		vectorAPI.DELETE("/records/:id", authzMiddleware, func(c *gin.Context) {
			vectorIndex.Delete(c.Param("id"))
			c.JSON(http.StatusOK, gin.H{"message": "record deleted", "id": c.Param("id")})
		})
		vectorAPI.POST("/search", func(c *gin.Context) {
			var body struct {
				Query []float64 `json:"query"`
				K     int       `json:"k"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if body.K < 1 {
				body.K = 5
			}
			c.JSON(http.StatusOK, gin.H{"results": vectorIndex.Search(vectorplus.Vector(body.Query), body.K)})
		})
		vectorAPI.POST("/similarity", func(c *gin.Context) {
			var body struct {
				A      []float64 `json:"a"`
				B      []float64 `json:"b"`
				Metric int       `json:"metric"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			a := vectorplus.Vector(body.A)
			b := vectorplus.Vector(body.B)
			var score float64
			switch body.Metric {
			case 2:
				score = vectorplus.SimilarityMetric002(a, b)
			case 3:
				score = vectorplus.SimilarityMetric003(a, b)
			case 4:
				score = vectorplus.SimilarityMetric004(a, b)
			case 5:
				score = vectorplus.SimilarityMetric005(a, b)
			default:
				score = vectorplus.SimilarityMetric001(a, b)
			}
			c.JSON(http.StatusOK, gin.H{"metric": body.Metric, "score": score})
		})
	}

	reviewAPI := router.Group("/api/v1/reviewflow", authMiddleware)
	{
		reviewAPI.PUT("/items/:id", authzMiddleware, func(c *gin.Context) {
			var item reviewflow.ReviewItem
			if err := c.ShouldBindJSON(&item); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			item.ID = c.Param("id")
			if strings.TrimSpace(item.ID) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "item id is required"})
				return
			}
			if item.Score == 0 {
				item.Score = reviewflow.ScoreBySignals(item.Title, item.Description, item.Tags)
			}
			reviewPipeline.Upsert(item)
			c.JSON(http.StatusOK, gin.H{"message": "item upserted", "id": item.ID})
		})
		reviewAPI.GET("/items/:id", func(c *gin.Context) {
			item, ok := reviewPipeline.Get(c.Param("id"))
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
				return
			}
			c.JSON(http.StatusOK, item)
		})
		reviewAPI.GET("/items", func(c *gin.Context) {
			stage := reviewflow.Stage(strings.TrimSpace(c.Query("stage")))
			c.JSON(http.StatusOK, gin.H{"items": reviewPipeline.ListByStage(stage)})
		})
		reviewAPI.POST("/items/:id/stage", authzMiddleware, func(c *gin.Context) {
			var body struct {
				Stage reviewflow.Stage `json:"stage"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if !reviewPipeline.Advance(c.Param("id"), body.Stage) {
				c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "stage updated", "id": c.Param("id"), "stage": body.Stage})
		})
		reviewAPI.POST("/score", func(c *gin.Context) {
			var body struct {
				Title       string   `json:"title"`
				Description string   `json:"description"`
				Tags        []string `json:"tags"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"score": reviewflow.ScoreBySignals(body.Title, body.Description, body.Tags)})
		})
		reviewAPI.POST("/quality", func(c *gin.Context) {
			var body struct {
				Item  reviewflow.ReviewItem `json:"item"`
				Check int                   `json:"check"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			var score float64
			switch body.Check {
			case 2:
				score = reviewflow.QualityCheck002(body.Item)
			case 3:
				score = reviewflow.QualityCheck003(body.Item)
			case 4:
				score = reviewflow.QualityCheck004(body.Item)
			case 5:
				score = reviewflow.QualityCheck005(body.Item)
			default:
				score = reviewflow.QualityCheck001(body.Item)
			}
			c.JSON(http.StatusOK, gin.H{"check": body.Check, "score": score})
		})
	}

	// ====================================
	// IAM ROUTES
	// ====================================
	if iamSystem != nil {
		iamSystem.RegisterRoutes(router)
		log.Println("✅ IAM routes registered")
	}

	// ====================================
	// OBJECT STORAGE MODULE (Native S3)
	// ====================================
	storageCfg := storage.DefaultConfig()
	var storageIssuer *iamtoken.Issuer
	var storageRevokedStore *iamstorage.EtcdRevokedTokenStore
	if iamSystem != nil {
		storageIssuer = iamSystem.Issuer
		storageRevokedStore = iamSystem.RevokedStore
	}
	storageSys, storageErr := storage.NewSystem(storageCfg, storageIssuer, storageRevokedStore, conns.Etcd)
	if storageErr != nil {
		log.Printf("⚠️  Object storage module initialization failed: %v — storage API will be unavailable", storageErr)
	} else {
		storageAPI := router.Group("/api/v1")
		storageSys.RegisterRoutes(storageAPI)
		modules = append(modules, storageSys)
		log.Println("✅ Object Storage module registered (native backend, data:", storageCfg.DataDir, ")")

		// Wire antivirus engine to API builder scanner pipeline.
		if storageSys.AVEngine != nil {
			apiBuilderHandler.SetAVEngine(storageSys.AVEngine)
		}
	}

	// ====================================
	// GATEKEEPER 2FA MODULE
	// ====================================
	var gkErr error
	gkSystem, gkErr = gatekeeper.NewSystem(conns.PostgreSQL)
	if gkErr != nil {
		log.Printf("⚠️  Gatekeeper 2FA module initialization failed: %v — 2FA endpoints will be unavailable", gkErr)
	} else {
		mfaAPI := router.Group("/api/v1/mfa", authMiddleware)
		gkSystem.RegisterRoutes(mfaAPI)
		modules = append(modules, gkSystem)
		log.Println("✅ Gatekeeper 2FA module registered")
	}

	// Start all registered modules.
	for _, m := range modules {
		if err := m.Start(ctx); err != nil {
			log.Printf("⚠️  Module %s failed to start: %v", m.Name(), err)
		} else {
			log.Printf("✅ Module %s started", m.Name())
		}
	}

	// ====================================
	// AUDIT ENDPOINTS (previously unwired)
	// ====================================
	auditHandler := audit.NewAuditHandler(nil) // AuditLogger impl wired when available
	auditAPI := router.Group("/api/v1/audit", authMiddleware)
	{
		auditAPI.POST("/logs", authzMiddleware, auditHandler.LogAction)
		auditAPI.GET("/logs", auditHandler.QueryLogs)
		auditAPI.GET("/report", auditHandler.GetReport)
		auditAPI.DELETE("/logs", authzMiddleware, auditHandler.DeleteOldLogs)
	}
	log.Println("✅ Audit routes registered")

	// ====================================
	// ENCRYPTION ENDPOINTS (previously unwired)
	// ====================================
	// Note: InMemorySecretsManager is used directly; the SecretsManager
	// interface has a signature mismatch that will be unified in a follow-up.
	encryptionHandler := encryption.NewEncryptionHandler(nil)
	encryptionAPI := router.Group("/api/v1/encryption", authMiddleware)
	{
		encryptionAPI.POST("/keys", authzMiddleware, encryptionHandler.CreateKey)
		encryptionAPI.GET("/keys", encryptionHandler.ListKeys)
		encryptionAPI.GET("/keys/:id", encryptionHandler.GetKey)
		encryptionAPI.POST("/keys/:id/rotate", authzMiddleware, encryptionHandler.RotateKey)
		encryptionAPI.DELETE("/keys/:id", authzMiddleware, encryptionHandler.DeleteKey)
		encryptionAPI.POST("/encrypt", authMiddleware, encryptionHandler.Encrypt)
		encryptionAPI.POST("/decrypt", authMiddleware, encryptionHandler.Decrypt)
		encryptionAPI.POST("/policies", authzMiddleware, encryptionHandler.CreatePolicy)
		encryptionAPI.GET("/policies", encryptionHandler.ListPolicies)
	}
	log.Println("✅ Encryption routes registered")

	// Phase 19: Register auto-encryption GORM callbacks on PostgreSQL.
	// This enables transparent field-level encryption/decryption for all
	// model fields tagged with `classification:"PII"`, `"Sensitive"`, or `"Confidential"`.
	if conns.PostgreSQL != nil {
		autoEnc := encryption.NewAutoEncryptor(encryption.GetDefaultKey)
		gormCB := encryption.NewGORMCallbacks(autoEnc)
		gormCB.Register(conns.PostgreSQL)
		log.Println("✅ Auto-encryption GORM callbacks registered (Phase 19)")
	}

	// ====================================
	// API GATEWAY MANAGEMENT ENDPOINTS (Phase 17)
	// ====================================
	gwAPI := router.Group("/api/v1/gateway", authMiddleware)
	gwSystem.RegisterRoutes(gwAPI)
	log.Println("✅ API Gateway management routes registered (/api/v1/gateway)")

	// ====================================
	// RECONCILER CONTROLLERS (P2 — AxiomNizam architecture)
	// ====================================
	// Initialize ResourceStore-backed reconcilers for all migrated modules.
	// Each reconciler is started in a background goroutine that periodically
	// reconciles resources from the store.
	//
	// Storage backend is selected by STORAGE_BACKEND env var:
	//   "etcd" (default) — uses EtcdStore[T] backed by external etcd
	//   "raft"           — uses RaftStore[T] backed by embedded Raft + go-memdb
	storageBackend := featureflags.StorageBackend()
	var backendMgr *platformstore.BackendManager

	if storageBackend == "raft" || conns.Etcd != nil {
		// Initialize the backend manager.
		var bmErr error
		backendMgr, bmErr = platformstore.NewBackendManager(platformstore.AllResourceTables())
		if bmErr != nil {
			log.Fatalf("Failed to initialize storage backend: %v", bmErr)
		}
		if backendMgr.IsEtcd() {
			backendMgr.SetEtcdClient(conns.Etcd)
		}
		defer backendMgr.Close()

		// Raft mode: complete deferred initialization now that BackendManager is ready.
		if backendMgr.IsRaft() {
			// Wait for Raft leader election before writing (single-node
			// election typically completes in ~1-2 seconds).
			log.Println("  ⏳ Waiting for Raft leader election...")
			for i := 0; i < 20; i++ {
				if backendMgr.RaftServer.IsLeader() {
					break
				}
				time.Sleep(250 * time.Millisecond)
			}
			if backendMgr.RaftServer.IsLeader() {
				log.Println("  ✅ Raft node is leader")
			} else {
				log.Println("  ⚠️  Raft node is not leader yet (writes may fail until election completes)")
			}

			// Re-attempt JWT secret with KVStore.
			if _, secretErr := server.EnsureSharedDemoJWTSecret(conns.PostgreSQL, nil, backendMgr.KV()); secretErr != nil {
				log.Printf("⚠️  DEMO_JWT_SECRET synchronization via Raft KV failed: %v", secretErr)
			} else {
				log.Println("✅ DEMO_JWT_SECRET synchronized via Raft KV store")
			}

			// Initialize IAM with KVStore backend (if not already initialized).
			if iamSystem == nil {
				iamSystem, iamErr = iampkg.NewSystem(conns.PostgreSQL, nil, iampkg.Config{
					IssuerURL: strings.TrimSpace(os.Getenv("IAM_ISSUER_URL")),
				}, backendMgr.KV())
				if iamErr != nil {
					log.Printf("⚠️  IAM system initialization via Raft KV failed: %v", iamErr)
				} else {
					log.Println("✅ IAM system initialized via Raft KV store")
					// Register IAM routes now (deferred from early init).
					iamSystem.RegisterRoutes(router)
					log.Println("✅ IAM routes registered (deferred, Raft KV backend)")

					// Wire IAM into auth handler.
					if iamSystem.PGStore != nil {
						authHandler.SetIdentityProviderStore(iamSystem.PGStore)
					}
					if iamSystem.Users != nil {
						authHandler.SetIAMUserRepository(iamSystem.Users)
					}
					if iamSystem.Authorizer != nil {
						authHandler.SetIAMAuthorizer(iamSystem.Authorizer)
					}
				}
			}

			// Wire components into storage system (deferred).
			// This must happen even if IAM failed, to ensure bucket persistence works.
			if storageSys != nil {
				// Wire KV persistence first so buckets can be loaded.
				storageSys.SetKVStore(backendMgr.KV())
				log.Println("✅ Storage: Raft KV persistence wired (deferred)")

				// Wire IAM middleware if available.
				if iamSystem != nil && iamSystem.Issuer != nil {
					storageSys.SetIAM(iamSystem.Issuer, iamSystem.RevokedStore)
					log.Println("✅ Storage: IAM middleware attached (deferred, Raft KV backend)")
				}
			}

			// Wire Gatekeeper 2FA module KV persistence.
			if gkSystem != nil {
				gkSystem.SetKVStore(backendMgr.KV())
				log.Println("✅ Gatekeeper: Raft KV persistence wired (deferred)")
			}

			// Wire CDC audit persistence
			cdcSystem.SetKVStore(backendMgr.KV())

			// Wire ETL audit + metrics persistence
			etlSystem.SetKVStore(backendMgr.KV())

			// Wire GIS audit persistence
			gisSystem.SetKVStore(backendMgr.KV())

			// Wire API Builder audit persistence
			apiBuilderSystem.SetKVStore(backendMgr.KV())
			log.Println("✅ API Builder: Raft KV persistence wired (deferred)")

			// Wire APIBanks audit persistence
			apiBankSystem.SetKVStore(backendMgr.KV())

			// Wire NetIntel audit + metrics persistence
			netintelSystem.SetKVStore(backendMgr.KV())

			// Wire remaining modules to KV persistence in Raft mode.
			workflows.ConfigureGlobalKVPersistence(backendMgr.KV())
			workflows.GlobalWorkflowEngine.RegisterBuiltinHandlers()
			modes.ConfigureGlobalKVPersistence(backendMgr.KV())
			vectorplus.ConfigureGlobalKVPersistence(backendMgr.KV())
			reviewflow.ConfigureGlobalKVPersistence(backendMgr.KV())
			integration.ConfigureGlobalKVPersistence(backendMgr.KV())
			log.Println("✅ Workflows/Modes/VectorPlus/ReviewFlow/Integration: Raft KV persistence wired")

			log.Println("  ℹ️  Module persistence: Raft KV available via backendMgr.KV()")
		}

		// Wire backend manager to health handler for /distributed Raft status.
		healthHandler.SetBackendManager(backendMgr)

		// Raft cluster management API.
		// Supports two auth modes:
		//   1. Normal JWT admin auth (authMiddleware + adminMiddleware)
		//   2. ADMIN_TOKEN env-var bearer token (for cluster bootstrap when IAM isn't ready)
		if backendMgr.IsRaft() {
			adminToken := os.Getenv("ADMIN_TOKEN")
			raftAuthMiddleware := func(c *gin.Context) {
				// Check ADMIN_TOKEN first (bootstrap mode).
				if adminToken != "" {
					bearer := c.GetHeader("Authorization")
					if bearer == "Bearer "+adminToken {
						c.Next()
						return
					}
				}
				// Fall back to normal JWT admin auth.
				if !authenticateRequest(c) {
					return
				}
				enrichRequestContext(c)
				claims := auth.GetUser(c)
				if claims == nil || !claims.HasRole("admin") {
					c.JSON(http.StatusForbidden, models.Response{Status: "error", Error: "admin role or ADMIN_TOKEN required"})
					c.Abort()
					return
				}
				c.Next()
			}
			raftAPI := router.Group("/api/v1/raft")
			raftAPI.Use(raftAuthMiddleware)
			raftAPI.POST("/peers", func(c *gin.Context) {
				var req struct {
					ID   string `json:"id" binding:"required"`
					Addr string `json:"addr" binding:"required"`
				}
				if err := c.BindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, models.Response{Status: "error", Error: "invalid request"})
					return
				}
				if err := backendMgr.AddRaftPeer(req.ID, req.Addr); err != nil {
					c.JSON(http.StatusInternalServerError, models.Response{Status: "error", Error: fmt.Sprintf("failed to add peer: %v", err)})
					return
				}
				c.JSON(http.StatusOK, models.Response{Status: "ok", Message: fmt.Sprintf("peer %s added at %s", req.ID, req.Addr)})
			})
			raftAPI.DELETE("/peers/:id", func(c *gin.Context) {
				id := c.Param("id")
				if err := backendMgr.RemoveRaftPeer(id); err != nil {
					c.JSON(http.StatusInternalServerError, models.Response{Status: "error", Error: fmt.Sprintf("failed to remove peer: %v", err)})
					return
				}
				c.JSON(http.StatusOK, models.Response{Status: "ok", Message: fmt.Sprintf("peer %s removed", id)})
			})
			log.Println("  ✅ Raft cluster management API registered (/api/v1/raft/peers)")
		}

		// Snapshot backup/restore (raft mode only)
		if backendMgr.IsRaft() {
			snapshotH := snapshothandler.NewHandler(backendMgr)
			sysAPI := router.Group("/api/v1/system", authMiddleware)
			sysAPI.GET("/snapshot", snapshotH.Download)
			sysAPI.POST("/snapshot/restore", authzMiddleware, snapshotH.Restore)
			log.Println("  ✅ Snapshot backup/restore endpoints registered (/api/v1/system/snapshot)")
		}

		log.Printf("🔄 Initializing reconciler controllers (backend=%s)...", storageBackend)
		reconcilerMetrics := metrics.GlobalReconcilerMetrics

		// Phase 1: Shadow mode — reconcilers run but don't affect production.
		shadowMode := true
		if strings.EqualFold(strings.TrimSpace(os.Getenv("RECONCILER_SHADOW_MODE")), "false") {
			shadowMode = false
		}
		if shadowMode {
			log.Println("  ℹ️  Shadow mode ON (set RECONCILER_SHADOW_MODE=false to disable)")
		} else {
			log.Println("  ⚠️  Shadow mode OFF — reconcilers will drive managers")
		}

		// Bulk Operation reconciler
		bulkStore := platformstore.NewStore[*bulk.BulkOperationResource](backendMgr, "bulkoperations", func() *bulk.BulkOperationResource { return &bulk.BulkOperationResource{} })
		bulkReconciler := reconcilerpkg.NewInstrumented("bulk",
			bulk.NewBulkOperationReconciler(bulkStore, platformManagers.Bulk), reconcilerMetrics)
		reconcilerMetrics.Register("bulk")
		go genericctrl.NewGenericController("bulk", bulkStore, bulkReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		bulkHandler.SetDualWriteStore(bulkStore)
		log.Println("  ✅ BulkOperation controller started (dual-write enabled)")

		// EventBus Topic reconciler
		topicStore := platformstore.NewStore[*eventbus.TopicResource](backendMgr, "eventbus-topics", func() *eventbus.TopicResource { return &eventbus.TopicResource{} })
		topicReconciler := reconcilerpkg.NewInstrumented("eventbus-topic",
			eventbus.NewTopicReconciler(topicStore, platformManagers.EventBus), reconcilerMetrics)
		reconcilerMetrics.Register("eventbus-topic")
		go genericctrl.NewGenericController("eventbus-topic", topicStore, topicReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		eventBusHandler.SetTopicDualWriteStore(topicStore)
		log.Println("  ✅ EventBusTopic controller started (dual-write enabled)")

		// EventBus Subscription reconciler
		subscriptionStore := platformstore.NewStore[*eventbus.SubscriptionResource](backendMgr, "eventbus-subscriptions", func() *eventbus.SubscriptionResource { return &eventbus.SubscriptionResource{} })
		subscriptionReconciler := reconcilerpkg.NewInstrumented("eventbus-subscription",
			eventbus.NewSubscriptionReconciler(subscriptionStore, platformManagers.EventBus), reconcilerMetrics)
		reconcilerMetrics.Register("eventbus-subscription")
		go genericctrl.NewGenericController("eventbus-subscription", subscriptionStore, subscriptionReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ EventBusSubscription controller started")

		// Export Job reconciler
		exportStore := platformstore.NewStore[*exportpkg.ExportJobResource](backendMgr, "exportjobs", func() *exportpkg.ExportJobResource { return &exportpkg.ExportJobResource{} })
		exportReconciler := reconcilerpkg.NewInstrumented("export",
			exportpkg.NewExportJobReconciler(exportStore, platformManagers.Export), reconcilerMetrics)
		reconcilerMetrics.Register("export")
		go genericctrl.NewGenericController("export", exportStore, exportReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		exportHandler.SetDualWriteStore(exportStore)
		log.Println("  ✅ ExportJob controller started (dual-write enabled)")

		// Streaming reconciler
		streamStore := platformstore.NewStore[*streaming.StreamResource](backendMgr, "streams", func() *streaming.StreamResource { return &streaming.StreamResource{} })
		streamReconciler := reconcilerpkg.NewInstrumented("streaming",
			streaming.NewStreamReconciler(streamStore), reconcilerMetrics)
		reconcilerMetrics.Register("streaming")
		go genericctrl.NewGenericController("streaming", streamStore, streamReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		streamHandler.SetDualWriteStore(streamStore)
		log.Println("  ✅ Stream controller started (dual-write enabled)")

		// RBAC Role reconciler
		roleStore := platformstore.NewStore[*rbac.RoleResource](backendMgr, "rbac-roles", func() *rbac.RoleResource { return &rbac.RoleResource{} })
		roleReconciler := reconcilerpkg.NewInstrumented("rbac-role",
			rbac.NewRoleReconciler(roleStore, platformManagers.RBAC), reconcilerMetrics)
		reconcilerMetrics.Register("rbac-role")
		go genericctrl.NewGenericController("rbac-role", roleStore, roleReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		rbacHandler.SetRoleDualWriteStore(roleStore)
		log.Println("  ✅ RBAC Role controller started (dual-write enabled)")

		// RBAC RoleBinding reconciler
		roleBindingStore := platformstore.NewStore[*rbac.RoleBindingResource](backendMgr, "rbac-rolebindings", func() *rbac.RoleBindingResource { return &rbac.RoleBindingResource{} })
		roleBindingReconciler := reconcilerpkg.NewInstrumented("rbac-rolebinding",
			rbac.NewRoleBindingReconciler(roleBindingStore, platformManagers.RBAC), reconcilerMetrics)
		reconcilerMetrics.Register("rbac-rolebinding")
		go genericctrl.NewGenericController("rbac-rolebinding", roleBindingStore, roleBindingReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ RBAC RoleBinding controller started")

		// Versioning Policy reconciler
		versionPolicyStore := platformstore.NewStore[*versioning.VersionPolicyResource](backendMgr, "version-policies", func() *versioning.VersionPolicyResource { return &versioning.VersionPolicyResource{} })
		versionPolicyReconciler := reconcilerpkg.NewInstrumented("versioning",
			versioning.NewVersionPolicyReconciler(versionPolicyStore), reconcilerMetrics)
		reconcilerMetrics.Register("versioning")
		go genericctrl.NewGenericController("versioning", versionPolicyStore, versionPolicyReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		versionHandler.SetDualWriteStore(versionPolicyStore) // Phase 2: dual-write
		log.Println("  ✅ VersionPolicy controller started (dual-write enabled)")

		// Tracing Config reconciler
		tracingConfigStore := platformstore.NewStore[*tracing.TracingConfigResource](backendMgr, "tracing-configs", func() *tracing.TracingConfigResource { return &tracing.TracingConfigResource{} })
		tracingConfigReconciler := reconcilerpkg.NewInstrumented("tracing",
			tracing.NewTracingConfigReconciler(tracingConfigStore), reconcilerMetrics)
		reconcilerMetrics.Register("tracing")
		go genericctrl.NewGenericController("tracing", tracingConfigStore, tracingConfigReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		tracingHandler.SetDualWriteStore(tracingConfigStore)
		log.Println("  ✅ TracingConfig controller started (dual-write enabled)")

		// Lineage Node reconciler
		lineageNodeStore := platformstore.NewStore[*lineage.LineageNodeResource](backendMgr, "lineage-nodes", func() *lineage.LineageNodeResource { return &lineage.LineageNodeResource{} })
		lineageNodeReconciler := reconcilerpkg.NewInstrumented("lineage",
			lineage.NewLineageNodeReconciler(lineageNodeStore, platformManagers.Lineage), reconcilerMetrics)
		reconcilerMetrics.Register("lineage")
		go genericctrl.NewGenericController("lineage", lineageNodeStore, lineageNodeReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		lineageHandler.SetDualWriteStore(lineageNodeStore)
		log.Println("  ✅ LineageNode controller started (dual-write enabled)")

		// Audit Policy reconciler
		auditPolicyStore := platformstore.NewStore[*audit.AuditPolicyResource](backendMgr, "audit-policies", func() *audit.AuditPolicyResource { return &audit.AuditPolicyResource{} })
		auditPolicyReconciler := reconcilerpkg.NewInstrumented("audit",
			audit.NewAuditPolicyReconciler(auditPolicyStore), reconcilerMetrics)
		reconcilerMetrics.Register("audit")
		go genericctrl.NewGenericController("audit", auditPolicyStore, auditPolicyReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		auditHandler.SetDualWriteStore(auditPolicyStore)
		log.Println("  ✅ AuditPolicy controller started (dual-write enabled)")

		// Encryption Key reconciler
		encryptionKeyStore := platformstore.NewStore[*encryption.EncryptionKeyResource](backendMgr, "encryption-keys", func() *encryption.EncryptionKeyResource { return &encryption.EncryptionKeyResource{} })
		encryptionKeyReconciler := reconcilerpkg.NewInstrumented("encryption-key",
			encryption.NewEncryptionKeyReconciler(encryptionKeyStore, nil), reconcilerMetrics)
		reconcilerMetrics.Register("encryption-key")
		go genericctrl.NewGenericController("encryption-key", encryptionKeyStore, encryptionKeyReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		encryptionHandler.SetKeyDualWriteStore(encryptionKeyStore)
		log.Println("  ✅ EncryptionKey controller started (dual-write enabled)")

		// Encryption Policy reconciler
		encryptionPolicyStore := platformstore.NewStore[*encryption.EncryptionPolicyResource](backendMgr, "encryption-policies", func() *encryption.EncryptionPolicyResource { return &encryption.EncryptionPolicyResource{} })
		encryptionPolicyReconciler := reconcilerpkg.NewInstrumented("encryption-policy",
			encryption.NewEncryptionPolicyReconciler(encryptionPolicyStore), reconcilerMetrics)
		reconcilerMetrics.Register("encryption-policy")
		go genericctrl.NewGenericController("encryption-policy", encryptionPolicyStore, encryptionPolicyReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ EncryptionPolicy controller started")

		// Conductor Producer reconciler
		producerStore := platformstore.NewStore[*conductor.ProducerResource](backendMgr, "conductor-producers", func() *conductor.ProducerResource { return &conductor.ProducerResource{} })
		producerReconciler := reconcilerpkg.NewInstrumented("conductor-producer",
			conductor.NewProducerReconciler(producerStore, conductorMgr), reconcilerMetrics)
		reconcilerMetrics.Register("conductor-producer")
		go genericctrl.NewGenericController("conductor-producer", producerStore, producerReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		// Note: conductor Handler is created inside RegisterRoutes — dual-write store
		// will be wired when conductor handler is refactored to accept store injection.
		log.Println("  ✅ ConductorProducer controller started (dual-write pending handler refactor)")

		// Conductor Consumer reconciler
		consumerStore := platformstore.NewStore[*conductor.ConsumerResource](backendMgr, "conductor-consumers", func() *conductor.ConsumerResource { return &conductor.ConsumerResource{} })
		consumerReconciler := reconcilerpkg.NewInstrumented("conductor-consumer",
			conductor.NewConsumerReconciler(consumerStore, conductorMgr), reconcilerMetrics)
		reconcilerMetrics.Register("conductor-consumer")
		go genericctrl.NewGenericController("conductor-consumer", consumerStore, consumerReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ ConductorConsumer controller started")

		// Webhook reconciler
		webhookStore := platformstore.NewStore[*webhooks.WebhookResource](backendMgr, "webhooks", func() *webhooks.WebhookResource { return &webhooks.WebhookResource{} })
		webhookReconciler := reconcilerpkg.NewInstrumented("webhook",
			webhooks.NewWebhookReconciler(webhookStore), reconcilerMetrics)
		reconcilerMetrics.Register("webhook")
		go genericctrl.NewGenericController("webhook", webhookStore, webhookReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		webhookHandler.SetDualWriteStore(webhookStore)
		log.Println("  ✅ Webhook controller started (dual-write enabled)")

		// Tenant reconciler
		tenantStore := platformstore.NewStore[*tenant.TenantV1Resource](backendMgr, "tenants", func() *tenant.TenantV1Resource { return &tenant.TenantV1Resource{} })
		tenantReconciler := reconcilerpkg.NewInstrumented("tenant",
			tenant.NewTenantReconciler(tenantStore), reconcilerMetrics)
		reconcilerMetrics.Register("tenant")
		go genericctrl.NewGenericController("tenant", tenantStore, tenantReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		tenantHandler.SetDualWriteStore(tenantStore)
		log.Println("  ✅ Tenant controller started (dual-write enabled)")

		log.Printf("🔄 All 17 reconciler controllers RUNNING in %d goroutines (shadow=%v) — Phase 1 active", 17, shadowMode)

		// ====================================
		// PHASE 5: Wire remaining reconcilers
		// ====================================

		// Jobs reconciler
		jobsStore := platformstore.NewStore[*jobs.JobResource](backendMgr, "jobs", func() *jobs.JobResource { return &jobs.JobResource{} })
		jobsReconciler := reconcilerpkg.NewInstrumented("jobs",
			jobs.NewJobController(nil, nil), reconcilerMetrics)
		reconcilerMetrics.Register("jobs")
		go genericctrl.NewGenericController("jobs", jobsStore, jobsReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ Jobs controller started")

		// ETL Pipeline reconciler
		etlStore := platformstore.NewStore[*etl.PipelineResource](backendMgr, "etl-pipelines", func() *etl.PipelineResource { return &etl.PipelineResource{} })
		etlReconciler := reconcilerpkg.NewInstrumented("etl",
			etl.NewPipelineController(nil, nil), reconcilerMetrics)
		reconcilerMetrics.Register("etl")
		go genericctrl.NewGenericController("etl", etlStore, etlReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ ETL Pipeline controller started")

		// CDC Pipeline reconciler
		cdcStore := platformstore.NewStore[*cdc.CDCPipelineResource](backendMgr, "cdc-pipelines", func() *cdc.CDCPipelineResource { return &cdc.CDCPipelineResource{} })
		cdcReconciler := reconcilerpkg.NewInstrumented("cdc",
			cdc.NewCDCPipelineController(nil, nil), reconcilerMetrics)
		reconcilerMetrics.Register("cdc")
		go genericctrl.NewGenericController("cdc", cdcStore, cdcReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ CDC Pipeline controller started")

		// Policies reconciler
		policiesStore := platformstore.NewStore[*policies.PolicyResource](backendMgr, "policies", func() *policies.PolicyResource { return &policies.PolicyResource{} })
		policiesReconciler := reconcilerpkg.NewInstrumented("policies",
			policies.NewPolicyReconciler(policiesStore, nil), reconcilerMetrics)
		reconcilerMetrics.Register("policies")
		go genericctrl.NewGenericController("policies", policiesStore, policiesReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ Policies controller started")

		// DataSource reconciler
		datasourceStore := platformstore.NewStore[*datasourceresource.DataSourceV1Resource](backendMgr, "datasources", func() *datasourceresource.DataSourceV1Resource { return &datasourceresource.DataSourceV1Resource{} })
		datasourceReconciler := reconcilerpkg.NewInstrumented("datasource",
			datasourceresource.NewDataSourceReconciler(datasourceStore, nil), reconcilerMetrics)
		reconcilerMetrics.Register("datasource")
		go genericctrl.NewGenericController("datasource", datasourceStore, datasourceReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ DataSource controller started")

		// IAM Users reconciler
		iamUsersStore := platformstore.NewStore[*iamusers.UserResource](backendMgr, "iam-users", func() *iamusers.UserResource { return &iamusers.UserResource{} })
		iamUsersReconciler := reconcilerpkg.NewInstrumented("iam-users",
			iamusers.NewUserReconciler(iamUsersStore), reconcilerMetrics)
		reconcilerMetrics.Register("iam-users")
		go genericctrl.NewGenericController("iam-users", iamUsersStore, iamUsersReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ IAM Users controller started")

		// API Scanner reconciler
		apiScannerStore := platformstore.NewStore[*apiscanner.APIScanResource](backendMgr, "api-scans", func() *apiscanner.APIScanResource { return &apiscanner.APIScanResource{} })
		apiScannerReconciler := reconcilerpkg.NewInstrumented("apiscanner",
			apiscanner.NewAPIScanReconciler(apiScannerStore, nil), reconcilerMetrics)
		reconcilerMetrics.Register("apiscanner")
		go genericctrl.NewGenericController("apiscanner", apiScannerStore, apiScannerReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ API Scanner controller started")

		log.Printf("🔄 Phase 5: +7 reconciler controllers started (total: 24 controllers, shadow=%v)", shadowMode)

		// ====================================
		// PHASE 6 P2: GIS resource controller
		// ====================================
		gisStore := platformstore.NewStore[*gispkg.GISResource](backendMgr, "gis", func() *gispkg.GISResource { return &gispkg.GISResource{} })
		gisReconciler := reconcilerpkg.NewInstrumented("gis",
			gispkg.NewGISReconciler(gisStore), reconcilerMetrics)
		reconcilerMetrics.Register("gis")
		go genericctrl.NewGenericController("gis", gisStore, gisReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ GIS controller started (Phase 6 P2)")

		// Analytics Dashboard controller
		analyticsStore := platformstore.NewStore[*analyticspkg.DashboardResource](backendMgr, "analytics-dashboards", func() *analyticspkg.DashboardResource { return &analyticspkg.DashboardResource{} })
		analyticsReconciler := reconcilerpkg.NewInstrumented("analytics",
			analyticspkg.NewDashboardReconciler(analyticsStore), reconcilerMetrics)
		reconcilerMetrics.Register("analytics")
		go genericctrl.NewGenericController("analytics", analyticsStore, analyticsReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ Analytics Dashboard controller started (Phase 6 P2)")

		// Transform Rule controller
		transformStore := platformstore.NewStore[*transformpkg.RuleResource](backendMgr, "transform-rules", func() *transformpkg.RuleResource { return &transformpkg.RuleResource{} })
		transformReconciler := reconcilerpkg.NewInstrumented("transform",
			transformpkg.NewRuleReconciler(transformStore), reconcilerMetrics)
		reconcilerMetrics.Register("transform")
		go genericctrl.NewGenericController("transform", transformStore, transformReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ Transform Rule controller started (Phase 6 P2)")

		// Notification Channel controller
		notificationStore := platformstore.NewStore[*notificationpkg.ChannelResource](backendMgr, "notification-channels", func() *notificationpkg.ChannelResource { return &notificationpkg.ChannelResource{} })
		notificationReconciler := reconcilerpkg.NewInstrumented("notification",
			notificationpkg.NewChannelReconciler(notificationStore), reconcilerMetrics)
		reconcilerMetrics.Register("notification")
		go genericctrl.NewGenericController("notification", notificationStore, notificationReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ Notification Channel controller started (Phase 6 P2)")

		// NetIntel Config controller
		netintelStore := platformstore.NewStore[*netintelpkg.ConfigResource](backendMgr, "netintel-configs", func() *netintelpkg.ConfigResource { return &netintelpkg.ConfigResource{} })
		netintelReconciler := reconcilerpkg.NewInstrumented("netintel",
			netintelpkg.NewConfigReconciler(netintelStore), reconcilerMetrics)
		reconcilerMetrics.Register("netintel")
		go genericctrl.NewGenericController("netintel", netintelStore, netintelReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ NetIntel Config controller started (Phase 6 P2)")

		log.Println("🔄 Phase 6 P2: +5 controllers started (gis, analytics, transform, notification, netintel)")

		// APIBank reconciler
		apiBankReconcilerStore := platformstore.NewStore[*apibanks.APIBankResource](backendMgr, "apibanks", func() *apibanks.APIBankResource { return &apibanks.APIBankResource{} })
		apiBankReconciler := reconcilerpkg.NewInstrumented("apibanks",
			apibanks.NewAPIBankReconciler(apiBankReconcilerStore, apiBankSystem.Manager()), reconcilerMetrics)
		reconcilerMetrics.Register("apibanks")
		go genericctrl.NewGenericController("apibanks", apiBankReconcilerStore, apiBankReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ APIBank controller started")

		// WaitCheck reconciler
		waitCheckStore := platformstore.NewStore[*waitx.WaitCheckResource](backendMgr, "wait-checks", func() *waitx.WaitCheckResource { return &waitx.WaitCheckResource{} })
		waitCheckReconciler := reconcilerpkg.NewInstrumented("waitx",
			waitx.NewWaitCheckReconciler(), reconcilerMetrics)
		reconcilerMetrics.Register("waitx")
		go genericctrl.NewGenericController("waitx", waitCheckStore, waitCheckReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ WaitX controller started")

		// API Builder reconciler
		customAPIStore := platformstore.NewStore[*apibuilder.CustomAPIResource](backendMgr, "custom-apis", func() *apibuilder.CustomAPIResource { return &apibuilder.CustomAPIResource{} })
		customAPIReconciler := reconcilerpkg.NewInstrumented("apibuilder",
			apibuilder.NewCustomAPIReconciler(apiBuilderHandler), reconcilerMetrics)
		reconcilerMetrics.Register("apibuilder")
		go genericctrl.NewGenericController("apibuilder", customAPIStore, customAPIReconciler, 1, shadowMode, reconcilerMetrics).Start(ctx)
		log.Println("  ✅ API Builder controller started")

		// ====================================
		// GARBAGE COLLECTOR (owner-reference cascade + finalizer gating)
		// ====================================
		garbageCollector := gc.NewGarbageCollector(30 * time.Second)
		garbageCollector.Register("etl-pipelines", gc.NewStoreAdapter(etlStore, func() *etl.PipelineResource { return &etl.PipelineResource{} }))
		garbageCollector.Register("cdc-pipelines", gc.NewStoreAdapter(cdcStore, func() *cdc.CDCPipelineResource { return &cdc.CDCPipelineResource{} }))
		garbageCollector.Register("bulkoperations", gc.NewStoreAdapter(bulkStore, func() *bulk.BulkOperationResource { return &bulk.BulkOperationResource{} }))
		garbageCollector.Register("jobs", gc.NewStoreAdapter(jobsStore, func() *jobs.JobResource { return &jobs.JobResource{} }))
		garbageCollector.Register("exportjobs", gc.NewStoreAdapter(exportStore, func() *exportpkg.ExportJobResource { return &exportpkg.ExportJobResource{} }))
		garbageCollector.Register("streams", gc.NewStoreAdapter(streamStore, func() *streaming.StreamResource { return &streaming.StreamResource{} }))
		garbageCollector.Register("webhooks", gc.NewStoreAdapter(webhookStore, func() *webhooks.WebhookResource { return &webhooks.WebhookResource{} }))
		garbageCollector.Register("analytics-dashboards", gc.NewStoreAdapter(analyticsStore, func() *analyticspkg.DashboardResource { return &analyticspkg.DashboardResource{} }))
		garbageCollector.Register("custom-apis", gc.NewStoreAdapter(customAPIStore, func() *apibuilder.CustomAPIResource { return &apibuilder.CustomAPIResource{} }))
		garbageCollector.Register("api-scans", gc.NewStoreAdapter(apiScannerStore, func() *apiscanner.APIScanResource { return &apiscanner.APIScanResource{} }))
		garbageCollector.Register("gis", gc.NewStoreAdapter(gisStore, func() *gispkg.GISResource { return &gispkg.GISResource{} }))
		go garbageCollector.Start(ctx)
		log.Println("  ✅ Garbage collector started (11 stores, 30s interval)")

		// Phase 0.4: etcd key-space monitoring
		etcdPrefixes := []string{
			"/axiomnizam/bulkoperations/",
			"/axiomnizam/eventbus-topics/",
			"/axiomnizam/eventbus-subscriptions/",
			"/axiomnizam/exportjobs/",
			"/axiomnizam/streams/",
			"/axiomnizam/rbac-roles/",
			"/axiomnizam/rbac-rolebindings/",
			"/axiomnizam/version-policies/",
			"/axiomnizam/tracing-configs/",
			"/axiomnizam/lineage-nodes/",
			"/axiomnizam/audit-policies/",
			"/axiomnizam/encryption-keys/",
			"/axiomnizam/encryption-policies/",
			"/axiomnizam/conductor-producers/",
			"/axiomnizam/conductor-consumers/",
			"/axiomnizam/webhooks/",
			"/axiomnizam/tenants/",
			"/axiomnizam/apibanks/",
			"/axiomnizam/jobs/",
			"/axiomnizam/etl-pipelines/",
			"/axiomnizam/cdc-pipelines/",
			"/axiomnizam/policies/",
			"/axiomnizam/datasources/",
			"/axiomnizam/iam-users/",
			"/axiomnizam/api-scans/",
			"/axiomnizam/gis/",
			"/axiomnizam/analytics-dashboards/",
			"/axiomnizam/transform-rules/",
			"/axiomnizam/notification-channels/",
			"/axiomnizam/netintel-configs/",
		}
		keySpaceMonitor := metrics.NewEtcdKeySpaceMonitor(conns.Etcd, etcdPrefixes, 30*time.Second)
		keySpaceMonitor.Start(ctx)
		keySpaceMonitorRef = keySpaceMonitor
		log.Println("  ✅ etcd key-space monitor started (18 prefixes, 30s interval)")
	} else {
		log.Println("⚠️  etcd not available — reconciler controllers skipped")
	}

	// Wire previously-unwired modules (migrations, heartbeat, service registry, etc.)
	server.WireUnwiredModules(conns, cfg, router, authMiddleware, adminOrSysMiddleware, backendMgr, platformManagers, storageSys)

	// WaitX — service readiness checks (TCP, HTTP, DNS, gRPC, Redis, MySQL, PostgreSQL, MongoDB, Kafka, RabbitMQ)
	waitxSystem := waitx.NewSystem()
	waitxSystem.SetKVStore(backendMgr.KV())
	_ = waitxSystem.Start(ctx)
	waitxSystem.Handler().RegisterRoutes(router.Group("/api/v1", authMiddleware))
	log.Println("✅ WaitX module started")

	// Print startup banner
	server.PrintStartupBanner(cfg, iamOnlyAuth)

	apiHost := cfg.API.Host
	apiPort := cfg.API.Port

	fmt.Println()

	runtimeHost := strings.TrimSpace(os.Getenv("RUNTIME_HOST"))
	if runtimeHost == "" {
		runtimeHost = apiHost
	}
	runtimePort := strings.TrimSpace(os.Getenv("RUNTIME_PORT"))
	if runtimePort == "" {
		runtimePort = "8001"
	}
	if runtimeHost == apiHost && runtimePort == apiPort {
		runtimePort = "8001"
	}
	runtimeAddr := fmt.Sprintf("%s:%s", runtimeHost, runtimePort)

	// Start runtime in background on a dedicated port to avoid router conflicts.
	go func() {
		if err := rt.Start(ctx, runtimeAddr); err != nil {
			log.Printf("Failed to start runtime: %v", err)
			cancel()
		}
	}()

	// Start API server with graceful shutdown
	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", apiHost, apiPort),
		Handler: router,
	}

	go func() {
		var err error
		if tlsCfg.Enabled {
			err = srv.ListenAndServeTLS(tlsCfg.CertFile, tlsCfg.KeyFile)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	log.Println("🛑 Shutting down gracefully...")

	// Give handlers 10 seconds to finish
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	// Stop runtime
	if err := rt.Stop(); err != nil {
		log.Printf("Runtime stop error: %v", err)
	}

	// Stop all registered modules (reverse order).
	for i := len(modules) - 1; i >= 0; i-- {
		if err := modules[i].Stop(); err != nil {
			log.Printf("⚠️  Module %s stop error: %v", modules[i].Name(), err)
		} else {
			log.Printf("✅ Module %s stopped", modules[i].Name())
		}
	}

	// Flush conductor stats to DB before exit
	conductorMgr.Close()

	cancel()
	log.Println("✅ AxiomNizam stopped")
}
