package netintel

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/netintel/audit"
	nmetrics "axiomnizam.bitbd.net/axiomnizam/internal/netintel/metrics"
	"axiomnizam.bitbd.net/axiomnizam/internal/netintel/modes"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"

	"github.com/gin-gonic/gin"
)

// System holds the netintel module's dependencies and provides
// a standard bootstrap interface (NewSystem, RegisterRoutes, Start, Stop, SetKVStore).
type System struct {
	handler     *Handler
	auditLogger *audit.Logger
	modesMgr    *modes.Manager
}

// NewSystem creates a new netintel System.
func NewSystem() *System {
	auditLog := audit.NewLogger()
	modesMgr := modes.NewManager()
	parser := NewParserEngine()
	analytics := NewAnalyticsEngine(parser)
	topo := NewTopologyEngine()
	h := NewHandlerWithDeps(parser, analytics, topo, nmetrics.Collector, auditLog)
	return &System{
		handler:     h,
		auditLogger: auditLog,
		modesMgr:    modesMgr,
	}
}

// Name returns the module identifier.
func (s *System) Name() string { return "netintel" }

// Start initializes the netintel module.
func (s *System) Start(ctx context.Context) error {
	if s.auditLogger != nil {
		s.auditLogger.Log(audit.SeverityInfo, audit.CategoryLifecycle, audit.ActionModuleStarted, "netintel module started")
	}
	logging.Z().Info("netintel: module started")
	return nil
}

// Stop gracefully shuts down the netintel module.
func (s *System) Stop() error {
	if s.auditLogger != nil {
		s.auditLogger.Log(audit.SeverityInfo, audit.CategoryLifecycle, audit.ActionModuleStopped, "netintel module stopped")
	}
	logging.Z().Info("netintel: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into the audit log, metrics, and modes manager.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	if s.auditLogger != nil {
		s.auditLogger.ConfigureKVPersistence(kv)
	}
	if s.modesMgr != nil {
		s.modesMgr.ConfigureKVPersistence(kv)
	}
	nmetrics.Collector.ConfigureKVPersistence(kv)
	logging.Z().Info("netintel: KVStore persistence configured")
}

// RegisterRoutes registers all netintel API routes.
func (s *System) RegisterRoutes(group *gin.RouterGroup) {
	s.handler.RegisterRoutes(group)
}

// Handler returns the HTTP handler.
func (s *System) Handler() *Handler {
	return s.handler
}

// ModesManager returns the modes manager for inline route wiring.
func (s *System) ModesManager() *modes.Manager {
	return s.modesMgr
}
