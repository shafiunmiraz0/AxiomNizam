package cdc

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/cdc/audit"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// System holds the CDC module's dependencies and provides
// a standard bootstrap interface.
type System struct {
	handler     *Handler
	streamH     *StreamHandler
	auditLogger *audit.Logger
}

// NewSystem creates a new CDC System.
func NewSystem(etcd ...*clientv3.Client) *System {
	auditLog := audit.NewLogger()
	return &System{
		handler:     NewHandler(etcd...),
		streamH:     NewStreamHandler(nil),
		auditLogger: auditLog,
	}
}

// Name returns the module identifier.
func (s *System) Name() string { return "cdc" }

// Start initializes the CDC module.
func (s *System) Start(ctx context.Context) error {
	logging.Z().Info("cdc: module started")
	return nil
}

// Stop gracefully shuts down the CDC module.
func (s *System) Stop() error {
	logging.Z().Info("cdc: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into the audit log.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	if s.auditLogger != nil {
		s.auditLogger.ConfigureKVPersistence(kv)
	}
	logging.Z().Info("cdc: KVStore persistence configured")
}

// Handler returns the CDC/ETL handler.
func (s *System) Handler() *Handler {
	return s.handler
}

// StreamHandler returns the stream handler.
func (s *System) StreamHandler() *StreamHandler {
	return s.streamH
}

// AuditLogger returns the audit logger.
func (s *System) AuditLogger() *audit.Logger {
	return s.auditLogger
}
