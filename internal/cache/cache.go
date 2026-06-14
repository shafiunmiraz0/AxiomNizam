package cache

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/cache/config"
	"context"
	"errors"
	"time"
)

// ErrKeyNotFound is returned when a key is not found in cache
var ErrKeyNotFound = errors.New("key not found in cache")

// ErrInvalidDuration is returned when duration is invalid
var ErrInvalidDuration = errors.New("invalid cache duration")

// Cache is the interface for all cache implementations
type Cache interface {
	// Get retrieves a value from cache by key
	Get(ctx context.Context, key string) (interface{}, error)

	// Set stores a value in cache with TTL
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error

	// Delete removes a key from cache
	Delete(ctx context.Context, key string) error

	// Clear removes all keys from cache
	Clear(ctx context.Context) error

	// Exists checks if a key exists in cache
	Exists(ctx context.Context, key string) (bool, error)

	// GetString retrieves a string value from cache
	GetString(ctx context.Context, key string) (string, error)

	// SetString stores a string value in cache
	SetString(ctx context.Context, key string, value string, ttl time.Duration) error

	// GetJSON retrieves and unmarshals a JSON value from cache
	GetJSON(ctx context.Context, key string, target interface{}) error

	// SetJSON marshals and stores a JSON value in cache
	SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error

	// IncrementCounter increments a counter by given amount
	IncrementCounter(ctx context.Context, key string, amount int64) error

	// DecrementCounter decrements a counter by given amount
	DecrementCounter(ctx context.Context, key string, amount int64) error

	// GetCounter gets a counter value
	GetCounter(ctx context.Context, key string) (int64, error)

	// SetCounter sets a counter value
	SetCounter(ctx context.Context, key string, value int64, ttl time.Duration) error

	// Health checks if the cache backend is healthy
	Health(ctx context.Context) error

	// Close closes the cache connection (if applicable)
	Close() error
}

// CacheConfig re-exports the config type from the config subpackage.
type CacheConfig = config.Config

// CacheKeyBuilder provides consistent key naming
type CacheKeyBuilder struct {
	prefix string
}

// NewCacheKeyBuilder creates a new cache key builder
func NewCacheKeyBuilder(prefix string) *CacheKeyBuilder {
	return &CacheKeyBuilder{
		prefix: prefix,
	}
}

// Build creates a cache key from parts
func (b *CacheKeyBuilder) Build(parts ...string) string {
	key := b.prefix
	for _, part := range parts {
		if part != "" {
			key += ":" + part
		}
	}
	return key
}

// UserKey builds a user cache key
func (b *CacheKeyBuilder) UserKey(userID string) string {
	return b.Build("user", userID)
}

// UserEmailKey builds a user email cache key
func (b *CacheKeyBuilder) UserEmailKey(email string) string {
	return b.Build("user:email", email)
}

// UserUsernameKey builds a user username cache key
func (b *CacheKeyBuilder) UserUsernameKey(username string) string {
	return b.Build("user:username", username)
}

// UserListKey builds a user list cache key with pagination
func (b *CacheKeyBuilder) UserListKey(page, pageSize int) string {
	return b.Build("users:list", "page", string(rune(page)), "size", string(rune(pageSize)))
}

// UserCountKey builds a user count cache key
func (b *CacheKeyBuilder) UserCountKey() string {
	return b.Build("users:count")
}

// SessionKey builds a session cache key
func (b *CacheKeyBuilder) SessionKey(sessionID string) string {
	return b.Build("session", sessionID)
}

// TokenKey builds a token cache key
func (b *CacheKeyBuilder) TokenKey(token string) string {
	return b.Build("token", token)
}

// RateLimitKey builds a rate limit cache key
func (b *CacheKeyBuilder) RateLimitKey(identifier string) string {
	return b.Build("ratelimit", identifier)
}

// InvalidateUserKeys invalidates all user-related cache keys
// This is called when a user is modified
type InvalidationPattern struct {
	Pattern string
}

// InvalidationPatterns for different entities
var (
	InvalidateUserPattern     = &InvalidationPattern{Pattern: "user:*"}
	InvalidateSessionPattern  = &InvalidationPattern{Pattern: "session:*"}
	InvalidateTokenPattern    = &InvalidationPattern{Pattern: "token:*"}
	InvalidateUserListPattern = &InvalidationPattern{Pattern: "users:list:*"}
)
