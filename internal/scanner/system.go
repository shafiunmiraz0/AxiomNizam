package scanner

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

// System holds the scanner module's orchestrator and provides
// a standard bootstrap interface (NewSystem, RegisterRoutes, Start, SetKVStore).
type System struct {
	orchestrator *Orchestrator
	metrics      *Metrics
}

// NewSystem creates a new scanner System with the given orchestrator.
func NewSystem(orch *Orchestrator) *System {
	return &System{
		orchestrator: orch,
		metrics:      orch.Metrics(),
	}
}

// Name returns the module identifier.
func (s *System) Name() string { return "scanner" }

// Start initializes the scanner module.
func (s *System) Start(ctx context.Context) error {
	logging.Z().Info("✅ Scanner: module started")
	return nil
}

// Stop gracefully shuts down the scanner module.
func (s *System) Stop() error {
	logging.Z().Info("Scanner: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into the scanner metrics.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	if s.metrics != nil {
		s.metrics.ConfigureKVPersistence(kv)
	}
	logging.Z().Info("✅ Scanner: KVStore persistence configured (Raft mode)")
}

// Orchestrator returns the scanner orchestrator.
func (s *System) Orchestrator() *Orchestrator {
	return s.orchestrator
}

// Metrics returns the scanner metrics.
func (s *System) Metrics() *Metrics {
	return s.metrics
}
