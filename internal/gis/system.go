package gis

import (
	"context"

	gaudit "axiomnizam.bitbd.net/axiomnizam/internal/gis/audit"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

// System holds the GIS module's dependencies and provides
// a standard bootstrap interface.
type System struct {
	handler       *GISSpecializedHandler
	auditLogger   *gaudit.Logger
}

// NewSystem creates a new GIS System.
func NewSystem() *System {
	return &System{
		handler:     NewGISSpecializedHandler(),
		auditLogger: gaudit.NewLogger(),
	}
}

// Name returns the module identifier.
func (s *System) Name() string { return "gis" }

// Start initializes the GIS module.
func (s *System) Start(ctx context.Context) error {
	logging.Z().Info("gis: module started")
	return nil
}

// Stop gracefully shuts down the GIS module.
func (s *System) Stop() error {
	logging.Z().Info("gis: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into the audit log.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	if s.auditLogger != nil {
		s.auditLogger.ConfigureKVPersistence(kv)
	}
	logging.Z().Info("gis: KVStore persistence configured")
}

// Handler returns the specialized GIS handler.
func (s *System) Handler() *GISSpecializedHandler {
	return s.handler
}

// AuditLogger returns the audit logger.
func (s *System) AuditLogger() *gaudit.Logger {
	return s.auditLogger
}
