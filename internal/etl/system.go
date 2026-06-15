package etl

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/etl/audit"
	"axiomnizam.bitbd.net/axiomnizam/internal/etl/metrics"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

// System holds the ETL module's dependencies and provides
// a standard bootstrap interface (NewSystem, RegisterRoutes, Start, Stop, SetKVStore).
type System struct {
	engine      *Engine
	handler     *Handler
	controller  *PipelineController
	auditLogger *audit.Logger
}

// NewSystem creates a new ETL System.
func NewSystem(engine *Engine) *System {
	auditLog := audit.NewLogger()
	return &System{
		engine:      engine,
		handler:     NewHandler(engine, auditLog),
		controller:  NewPipelineController(engine, nil),
		auditLogger: auditLog,
	}
}

// Name returns the module identifier.
func (s *System) Name() string { return "etl" }

// Start initializes the ETL module.
func (s *System) Start(ctx context.Context) error {
	logging.Z().Info("etl: module started")
	return nil
}

// Stop gracefully shuts down the ETL module.
func (s *System) Stop() error {
	logging.Z().Info("etl: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into audit log and metrics.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	if s.auditLogger != nil {
		s.auditLogger.ConfigureKVPersistence(kv)
	}
	metrics.Collector.ConfigureKVPersistence(kv)
	logging.Z().Info("etl: KVStore persistence configured")
}

// Handler returns the HTTP handler.
func (s *System) Handler() *Handler {
	return s.handler
}

// Controller returns the pipeline reconciler.
func (s *System) Controller() *PipelineController {
	return s.controller
}

// Engine returns the ETL engine.
func (s *System) Engine() *Engine {
	return s.engine
}
