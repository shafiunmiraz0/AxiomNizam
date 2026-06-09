package bootstrap

import (
	"database/sql"

	"example.com/axiomnizam/internal/gatekeeper/audit"
	"example.com/axiomnizam/internal/gatekeeper/backupcodes"
	"example.com/axiomnizam/internal/gatekeeper/cache"
	"example.com/axiomnizam/internal/gatekeeper/challenge"
	"example.com/axiomnizam/internal/gatekeeper/config"
	"example.com/axiomnizam/internal/gatekeeper/enrollment"
	"example.com/axiomnizam/internal/gatekeeper/handlers"
	"example.com/axiomnizam/internal/gatekeeper/metrics"
	"example.com/axiomnizam/internal/gatekeeper/pgstore"
	"example.com/axiomnizam/internal/gatekeeper/policy"
	"example.com/axiomnizam/internal/gatekeeper/repositories"
	"example.com/axiomnizam/internal/gatekeeper/risk"
	"example.com/axiomnizam/internal/gatekeeper/totp"
	"example.com/axiomnizam/internal/gatekeeper/trusteddevices"
	"github.com/gin-gonic/gin"
)

// Module bootstraps the Gatekeeper 2FA module.
type Module struct {
	cfg *config.Config
	db  *sql.DB

	// Repositories
	factorRepo     repositories.FactorRepository
	challengeRepo  repositories.ChallengeRepository
	backupCodeRepo repositories.BackupCodeRepository
	trustedDevRepo repositories.TrustedDeviceRepository

	// Services
	totpSvc       *totp.Service
	challengeSvc  *challenge.Service
	enrollmentSvc *enrollment.Service
	backupCodeSvc *backupcodes.Service
	deviceSvc     *trusteddevices.Service
	policySvc     *policy.Engine
	riskSvc       *risk.Engine

	// Infrastructure
	auditLog *audit.Logger
	coll     *metrics.Collector
	cacheSvc cache.Store

	// HTTP
	httpHandler *handlers.HTTPHandler
}

// NewModule creates a new Gatekeeper module.
func NewModule(cfg *config.Config, db *sql.DB) (*Module, error) {
	m := &Module{
		cfg: cfg,
		db:  db,
	}

	if err := m.initialize(); err != nil {
		return nil, err
	}

	return m, nil
}

// initialize wires all dependencies (K8s-style reconciliation pattern).
func (m *Module) initialize() error {
	// 1. Initialize repositories (data layer)
	m.factorRepo = pgstore.NewFactorRepository(m.db)
	m.challengeRepo = pgstore.NewChallengeRepository(m.db)
	m.backupCodeRepo = pgstore.NewBackupCodeRepository(m.db)
	m.trustedDevRepo = pgstore.NewTrustedDeviceRepository(m.db)

	// 2. Initialize cache
	m.cacheSvc = cache.NewInMemoryStore()

	// 3. Initialize TOTP service
	m.totpSvc = totp.NewService(
		totp.NewSecretGenerator(),
		totp.NewValidator(),
		totp.NewIssuerProvider(),
		totp.NewRealClock(),
	)

	// 4. Initialize challenge service
	m.challengeSvc = challenge.NewService(
		m.challengeRepo,
		m.factorRepo,
		m.totpSvc,
		challenge.NewRealClock(),
		m.cfg.EncryptionKey,
	)

	// 5. Initialize enrollment service
	m.enrollmentSvc = enrollment.NewService(
		m.factorRepo,
		m.backupCodeRepo,
		m.totpSvc,
		m.cfg.EncryptionKey,
	)

	// 6. Initialize backup code service
	m.backupCodeSvc = backupcodes.NewService(m.backupCodeRepo)

	// 7. Initialize trusted device service
	m.deviceSvc = trusteddevices.NewService(
		m.trustedDevRepo,
		trusteddevices.NewRealClock(),
	)

	// 8. Initialize policy engine with default rules
	m.policySvc = policy.NewEngine(
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

	// 9. Initialize risk engine
	m.riskSvc = risk.NewEngine(&risk.DefaultScorer{})

	// 10. Initialize audit logging
	auditBackend := audit.NewInMemoryBackend() // TODO: PostgreSQL backend
	m.auditLog = audit.NewLogger(auditBackend)

	// 11. Initialize metrics
	m.coll = metrics.NewCollector()

	// 12. Initialize HTTP handler (with adapter wrappers to match contracts interfaces)
	m.httpHandler = handlers.NewHTTPHandler(
		wrapEnrollmentService(m.enrollmentSvc),
		wrapChallengeService(m.challengeSvc),
		wrapFactorService(m.factorRepo),
		wrapPolicyService(m.policySvc),
		wrapRiskService(m.riskSvc),
		wrapTrustedDeviceService(m.deviceSvc),
		wrapBackupCodeService(m.backupCodeSvc),
		nil, // WebAuthn service (initialized in main gatekeeper system)
	)

	return nil
}

// RegisterRoutes registers all HTTP routes on the given router group.
func (m *Module) RegisterRoutes(api *gin.RouterGroup) {
	m.httpHandler.RegisterRoutes(api)
}

// TotpService returns the TOTP service.
func (m *Module) TotpService() *totp.Service {
	return m.totpSvc
}

// ChallengeService returns the challenge service.
func (m *Module) ChallengeService() *challenge.Service {
	return m.challengeSvc
}

// EnrollmentService returns the enrollment service.
func (m *Module) EnrollmentService() *enrollment.Service {
	return m.enrollmentSvc
}

// BackupCodeService returns the backup code service.
func (m *Module) BackupCodeService() *backupcodes.Service {
	return m.backupCodeSvc
}

// TrustedDeviceService returns the trusted device service.
func (m *Module) TrustedDeviceService() *trusteddevices.Service {
	return m.deviceSvc
}

// PolicyEngine returns the policy engine.
func (m *Module) PolicyEngine() *policy.Engine {
	return m.policySvc
}

// RiskEngine returns the risk engine.
func (m *Module) RiskEngine() *risk.Engine {
	return m.riskSvc
}

// AuditLogger returns the audit logger.
func (m *Module) AuditLogger() *audit.Logger {
	return m.auditLog
}

// MetricsCollector returns the metrics collector.
func (m *Module) MetricsCollector() *metrics.Collector {
	return m.coll
}
