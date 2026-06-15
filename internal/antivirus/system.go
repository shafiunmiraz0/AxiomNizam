package antivirus

import (
	"context"
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"github.com/gin-gonic/gin"
)

// System holds the antivirus module's engine and provides
// a standard bootstrap interface (NewSystem, RegisterRoutes, Start, SetKVStore).
type System struct {
	engine  *Engine
	kvStore platformstore.KVStore
}

// NewSystem creates a new antivirus System with the given engine.
func NewSystem(engine *Engine) *System {
	return &System{engine: engine}
}

// Name returns the module identifier.
func (s *System) Name() string { return "antivirus" }

// Start initializes the antivirus engine.
func (s *System) Start(ctx context.Context) error {
	s.engine.Start()
	logging.Z().Info("✅ Antivirus: module started")
	return nil
}

// Stop gracefully shuts down the antivirus engine.
func (s *System) Stop() error {
	s.engine.Shutdown(context.Background())
	logging.Z().Info("Antivirus: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into the antivirus module.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	s.kvStore = kv

	// Load persisted config if available, falling back to env-var config.
	if kv != nil {
		if persisted := LoadConfigFromKV(context.Background(), kv); persisted != nil {
			if _, err := s.engine.UpdateConfig(persisted); err != nil {
				logging.Z().Warn(fmt.Sprintf("antivirus: failed to load persisted config, using env defaults: %v", err))
			} else {
				logging.Z().Info("✅ Antivirus: loaded config from KV store")
			}
		}
	}

	logging.Z().Info("✅ Antivirus: KVStore persistence configured")
}

// RegisterRoutes registers antivirus API routes on the given router group.
// sysadminMiddleware is applied to write endpoints (PUT /config).
func (s *System) RegisterRoutes(rg *gin.RouterGroup, sysadminMiddleware ...gin.HandlerFunc) {
	handler := NewAPIHandler(s.engine)
	handler.SetKVStore(s.kvStore)
	handler.RegisterRoutes(rg, sysadminMiddleware...)
	logging.Z().Info("✅ Antivirus routes registered")
}

// Engine returns the antivirus engine.
func (s *System) Engine() *Engine {
	return s.engine
}
