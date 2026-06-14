package apibanks

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/apibanks/audit"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

// System holds the APIBanks module's dependencies and provides
// a standard bootstrap interface (NewSystem, RegisterRoutes, Start, Stop, SetKVStore).
type System struct {
	manager     *APIBankManager
	handler     *Handler
	auditLogger *audit.Logger
}

// NewSystem creates a new APIBanks System.
func NewSystem() *System {
	mgr := NewAPIBankManager()
	auditLog := audit.NewLogger()
	return &System{
		manager:     mgr,
		handler:     NewHandler(mgr, auditLog),
		auditLogger: auditLog,
	}
}

// Name returns the module identifier.
func (s *System) Name() string { return "apibanks" }

// Start initializes the APIBanks module.
func (s *System) Start(ctx context.Context) error {
	logging.Z().Info("apibanks: module started")
	return nil
}

// Stop gracefully shuts down the APIBanks module.
func (s *System) Stop() error {
	logging.Z().Info("apibanks: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into audit log.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	if s.auditLogger != nil {
		s.auditLogger.ConfigureKVPersistence(kv)
	}
	logging.Z().Info("apibanks: KVStore persistence configured")
}

// Handler returns the HTTP handler.
func (s *System) Handler() *Handler {
	return s.handler
}

// Manager returns the APIBank manager.
func (s *System) Manager() *APIBankManager {
	return s.manager
}
