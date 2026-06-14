package antivirus

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"github.com/gin-gonic/gin"
)

// System holds the antivirus module's engine and provides
// a standard bootstrap interface (NewSystem, RegisterRoutes, Start, SetKVStore).
type System struct {
	engine *Engine
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
	// Antivirus uses in-memory threat log; no KV persistence needed currently.
	logging.Z().Info("✅ Antivirus: KVStore persistence configured (no-op)")
}

// RegisterRoutes registers antivirus API routes on the given router group.
func (s *System) RegisterRoutes(rg *gin.RouterGroup) {
	handler := NewAPIHandler(s.engine)
	handler.RegisterRoutes(rg)
	logging.Z().Info("✅ Antivirus routes registered")
}

// Engine returns the antivirus engine.
func (s *System) Engine() *Engine {
	return s.engine
}
