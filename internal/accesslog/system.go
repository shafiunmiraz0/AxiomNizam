package accesslog

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"github.com/gin-gonic/gin"
)

// System is the access log module's top-level bootstrap.
type System struct {
	store *Store
}

// NewSystem creates a new access log system.
func NewSystem() *System {
	return &System{
		store: NewStore(),
	}
}

// Store returns the underlying access log store.
func (s *System) Store() *Store {
	return s.store
}

// SetKVStore wires the KV store for blocklist persistence.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	s.store.SetKVStore(kv)
	logging.Z().Info("✅ Access log: KVStore persistence configured")
}

// RegisterRoutes registers access log API routes.
func (s *System) RegisterRoutes(rg *gin.RouterGroup, sysadminMiddleware ...gin.HandlerFunc) {
	handler := NewHandler(s.store)
	handler.RegisterRoutes(rg, sysadminMiddleware...)
	logging.Z().Info("✅ Access log routes registered")
}
