package apibuilder

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/apibuilder/audit"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

// System holds the apibuilder module's dependencies and provides
// a standard bootstrap interface.
type System struct {
	handler     *APIBuilderHandler
	auditLogger *audit.Logger
}

// NewSystem creates a new apibuilder System.
// The handler is expected to be initialized by the caller (main.go)
// since it requires complex dependencies (etcd, db, scanner, analytics, gis).
func NewSystem(handler *APIBuilderHandler) *System {
	auditLog := audit.NewLogger()
	return &System{
		handler:     handler,
		auditLogger: auditLog,
	}
}

// Name returns the module identifier.
func (s *System) Name() string { return "apibuilder" }

// Start initializes the apibuilder module.
func (s *System) Start(ctx context.Context) error {
	logging.Z().Info("apibuilder: module started")
	return nil
}

// Stop gracefully shuts down the apibuilder module.
func (s *System) Stop() error {
	logging.Z().Info("apibuilder: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into the handler and audit log.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	if s.handler != nil {
		s.handler.SetKVStore(kv)
	}
	if s.auditLogger != nil {
		s.auditLogger.ConfigureKVPersistence(kv)
	}
	logging.Z().Info("apibuilder: KVStore persistence configured")
}

// Handler returns the API builder handler.
func (s *System) Handler() *APIBuilderHandler {
	return s.handler
}

// AuditLogger returns the audit logger.
func (s *System) AuditLogger() *audit.Logger {
	return s.auditLogger
}
