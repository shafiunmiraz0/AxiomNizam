package cache

import (
	cacheconfig "axiomnizam.bitbd.net/axiomnizam/internal/cache/config"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"fmt"
)

// Manager handles cache initialization and management
type Manager struct {
	cache   Cache
	config  *CacheConfig
	backend string
}

// NewCacheManager creates a new cache manager
func NewCacheManager(config *CacheConfig) (*Manager, error) {
	if config == nil {
		cfg := cacheconfig.DefaultConfig()
		config = &CacheConfig{
			Type:       cfg.Type,
			DefaultTTL: cfg.DefaultTTL,
			MaxSize:    cfg.MaxSize,
		}
	}


	manager := &Manager{
		config: config,	}

	// Initialize cache backend
	if err := manager.initializeCache(); err != nil {
		return nil, err
	}

	return manager, nil
}

// initializeCache initializes the cache backend based on configuration
func (m *Manager) initializeCache() error {
	switch m.config.Type {
	case "redis":
		cache, err := NewRedisCache(m.config)
		if err != nil {
			return fmt.Errorf("failed to initialize Redis cache: %w", err)
		}
		m.cache = cache
		m.backend = "Redis"
		logging.Z().Info(fmt.Sprintf("Redis cache initialized: %s:%d", m.config.Host, m.config.Port))

	case "memory":
		m.cache = NewMemoryCache(m.config.MaxSize)
		m.backend = "Memory"
		logging.Z().Info(fmt.Sprintf("Memory cache initialized (max size: %d)", m.config.MaxSize))

	default:
		return fmt.Errorf("unknown cache type: %s", m.config.Type)
	}

	return nil
}

// GetCache returns the underlying cache instance
func (m *Manager) GetCache() Cache {
	return m.cache
}

// GetBackend returns the cache backend type
func (m *Manager) GetBackend() string {
	return m.backend
}

// Health checks the health of the cache backend
func (m *Manager) Health() error {
	if m.cache == nil {
		return fmt.Errorf("cache not initialized")
	}

	return m.cache.Health(nil)
}

// Close closes the cache connection
func (m *Manager) Close() error {
	if m.cache != nil {
		return m.cache.Close()
	}
	return nil
}

// SwitchBackend switches to a different cache backend
func (m *Manager) SwitchBackend(cacheType string) error {
	oldBackend := m.backend

	m.config.Type = cacheType
	if err := m.initializeCache(); err != nil {
		logging.Z().Info(fmt.Sprintf("Error switching to %s cache, keeping %s", cacheType, oldBackend))
		return err
	}

	logging.Z().Info(fmt.Sprintf("Switched from %s to %s cache", oldBackend, m.backend))
	return nil
}

// GetConfig returns the current cache configuration
func (m *Manager) GetConfig() *CacheConfig {
	return m.config
}

// DefaultCacheConfig returns a sensible default cache configuration
func DefaultCacheConfig() *CacheConfig {
	cfg := cacheconfig.DefaultConfig()
	return &CacheConfig{
		Type:       cfg.Type,
		DefaultTTL: cfg.DefaultTTL,
		MaxSize:    cfg.MaxSize,
	}
}

// RedisCacheConfig returns a Redis cache configuration with sensible defaults
func RedisCacheConfig(host string, port int, password string) *CacheConfig {
	cfg := cacheconfig.DefaultConfig()
	if host == "" {
		host = cfg.Host
	}
	if port == 0 {
		port = cfg.Port
	}

	return &CacheConfig{
		Type:       "redis",
		Host:       host,
		Port:       port,
		Password:   password,
		DB:         cfg.DB,
		DefaultTTL: cfg.DefaultTTL,
		PoolSize:   cfg.PoolSize,
	}
}

// MemoryCacheConfig returns a memory cache configuration
func MemoryCacheConfig(maxSize int) *CacheConfig {
	cfg := cacheconfig.DefaultConfig()
	if maxSize <= 0 {
		maxSize = cfg.MaxSize
	}

	return &CacheConfig{
		Type:       "memory",
		DefaultTTL: cfg.DefaultTTL,
		MaxSize:    maxSize,
	}
}

// BuildCacheConfig creates a cache config from environment variables or defaults
// This is a helper function that can be expanded based on your config system
func BuildCacheConfig(cacheType string) *CacheConfig {
	switch cacheType {
	case "redis":
		return RedisCacheConfig("localhost", 6379, "")
	case "memory":
		return MemoryCacheConfig(1000)
	default:
		return DefaultCacheConfig()
	}
}
