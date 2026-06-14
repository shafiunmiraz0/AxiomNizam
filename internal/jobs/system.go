package jobs

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"github.com/gin-gonic/gin"
)

// System holds the jobs module's components and provides
// a standard bootstrap interface (NewSystem, RegisterRoutes, Start, SetKVStore).
type System struct {
	manager *JobManagerImpl
	handler *V1Handler
}

// NewSystem creates a new jobs System.
func NewSystem(manager *JobManagerImpl, handler *V1Handler) *System {
	return &System{
		manager: manager,
		handler: handler,
	}
}

// Name returns the module identifier.
func (s *System) Name() string { return "jobs" }

// Start initializes the jobs module.
func (s *System) Start(ctx context.Context) error {
	logging.Z().Info("✅ Jobs: module started")
	return nil
}

// Stop gracefully shuts down the jobs module.
func (s *System) Stop() error {
	logging.Z().Info("Jobs: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into the jobs module.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	// Jobs uses PostgreSQL for persistence; KVStore wiring is not needed.
	logging.Z().Info("✅ Jobs: KVStore persistence configured (no-op)")
}

// RegisterRoutes registers jobs API routes on the given router group.
func (s *System) RegisterRoutes(rg *gin.RouterGroup) {
	if s.handler != nil {
		s.handler.RegisterRoutes(rg)
	}
	logging.Z().Info("✅ Jobs routes registered")
}

// Manager returns the job manager.
func (s *System) Manager() *JobManagerImpl {
	return s.manager
}
