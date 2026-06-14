package waitx

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/waitx/audit"
)

// System holds the waitx module's dependencies and provides
// a standard bootstrap interface (NewSystem, RegisterRoutes, Start, Stop, SetKVStore).
type System struct {
	handler     *Handler
	auditLogger *audit.Logger
}

// NewSystem creates a new waitx System.
func NewSystem() *System {
	auditLog := audit.NewLogger()
	return &System{
		handler:     NewHandler(auditLog),
		auditLogger: auditLog,
	}
}

// Name returns the module identifier.
func (s *System) Name() string { return "waitx" }

// Start initializes the waitx module.
func (s *System) Start(ctx context.Context) error {
	logging.Z().Info("waitx: module started")
	return nil
}

// Stop gracefully shuts down the waitx module.
func (s *System) Stop() error {
	logging.Z().Info("waitx: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into the audit log.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	if s.auditLogger != nil {
		s.auditLogger.ConfigureKVPersistence(kv)
	}
	logging.Z().Info("waitx: KVStore persistence configured")
}

// Handler returns the HTTP handler.
func (s *System) Handler() *Handler {
	return s.handler
}
