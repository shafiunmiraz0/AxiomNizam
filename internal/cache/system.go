package cache

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

// System holds the cache module's components and provides
// a standard bootstrap interface (NewSystem, Start, SetKVStore).
type System struct {
	manager *Manager
}

// NewSystem creates a new cache System with the given manager.
func NewSystem(manager *Manager) *System {
	return &System{manager: manager}
}

// Name returns the module identifier.
func (s *System) Name() string { return "cache" }

// Start initializes the cache module.
func (s *System) Start(ctx context.Context) error {
	logging.Z().Info("✅ Cache: module started")
	return nil
}

// Stop gracefully shuts down the cache module.
func (s *System) Stop() error {
	logging.Z().Info("Cache: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into the cache module.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	// Cache uses Redis/in-memory; no KV persistence needed.
	logging.Z().Info("✅ Cache: KVStore persistence configured (no-op)")
}

// Manager returns the cache manager.
func (s *System) Manager() *Manager {
	return s.manager
}
