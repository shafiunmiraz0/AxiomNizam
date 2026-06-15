package conductor

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"github.com/gin-gonic/gin"
)

// System holds the conductor module's components and provides
// a standard bootstrap interface (NewSystem, RegisterRoutes, Start, SetKVStore).
type System struct {
	manager *Manager
}

// NewSystem creates a new conductor System with the given manager.
func NewSystem(manager *Manager) *System {
	return &System{manager: manager}
}

// Name returns the module identifier.
func (s *System) Name() string { return "conductor" }

// Start initializes the conductor module.
func (s *System) Start(ctx context.Context) error {
	logging.Z().Info("✅ Conductor: module started")
	return nil
}

// Stop gracefully shuts down the conductor module.
func (s *System) Stop() error {
	logging.Z().Info("Conductor: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into the conductor module.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	// Conductor uses etcd/PostgreSQL for persistence; KVStore wiring is not needed.
	logging.Z().Info("✅ Conductor: KVStore persistence configured (no-op)")
}

// RegisterRoutes registers conductor API routes on the given router.
func (s *System) RegisterRoutes(router *gin.Engine, authMiddleware, adminMiddleware gin.HandlerFunc) {
	RegisterRoutes(router, s.manager, authMiddleware, adminMiddleware)
	logging.Z().Info("✅ Conductor routes registered")
}

// Manager returns the conductor manager.
func (s *System) Manager() *Manager {
	return s.manager
}
