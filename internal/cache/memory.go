package cache

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// cacheEntry represents a cached value with expiration
type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

// MemoryCache implements Cache interface using in-memory storage
// Safe for concurrent use with mutex protection
type MemoryCache struct {
	mu      sync.RWMutex
	data    map[string]*cacheEntry
	maxSize int
}

// NewMemoryCache creates a new in-memory cache instance
func NewMemoryCache(maxSize int) *MemoryCache {
	if maxSize <= 0 {
		maxSize = 1000
	}

	cache := &MemoryCache{
		data:    make(map[string]*cacheEntry),
		maxSize: maxSize,
	}

	// Start cleanup goroutine for expired entries
	go cache.cleanupExpired()

	return cache
}

// Get retrieves a value from cache
func (m *MemoryCache) Get(ctx context.Context, key string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, exists := m.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}

	if time.Now().After(entry.expiresAt) {
		// Entry has expired, but we'll let cleanup handle deletion
		return nil, ErrKeyNotFound
	}

	return entry.value, nil
}

// Set stores a value in cache with TTL
func (m *MemoryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidDuration
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check size limit
	if len(m.data) >= m.maxSize && !exists(m.data, key) {
		// Evict oldest entry if at capacity
		m.evictOldest()
	}

	m.data[key] = &cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}

	return nil
}

// Delete removes a key from cache
func (m *MemoryCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return nil
}

// Clear removes all keys from cache
func (m *MemoryCache) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = make(map[string]*cacheEntry)
	return nil
}

// Exists checks if a key exists in cache
func (m *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, exists := m.data[key]
	if !exists {
		return false, nil
	}

	if time.Now().After(entry.expiresAt) {
		return false, nil
	}

	return true, nil
}

// GetString retrieves a string value from cache
func (m *MemoryCache) GetString(ctx context.Context, key string) (string, error) {
	val, err := m.Get(ctx, key)
	if err != nil {
		return "", err
	}

	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("value is not a string")
	}

	return str, nil
}

// SetString stores a string value in cache
func (m *MemoryCache) SetString(ctx context.Context, key string, value string, ttl time.Duration) error {
	return m.Set(ctx, key, value, ttl)
}

// GetJSON retrieves and unmarshals a JSON value from cache
func (m *MemoryCache) GetJSON(ctx context.Context, key string, target interface{}) error {
	val, err := m.Get(ctx, key)
	if err != nil {
		return err
	}

	// If value is already unmarshaled, try direct assignment
	if jsonData, ok := val.(string); ok {
		if err := json.Unmarshal([]byte(jsonData), target); err != nil {
			logging.Z().Info(fmt.Sprintf("Error unmarshaling JSON for key %s: %v", key, err))
			return fmt.Errorf("failed to unmarshal JSON: %w", err)
		}
		return nil
	}

	// If value is already an object, try to marshal and unmarshal
	data, err := json.Marshal(val)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("Error marshaling value for key %s: %v", key, err))
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		logging.Z().Info(fmt.Sprintf("Error unmarshaling JSON for key %s: %v", key, err))
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return nil
}

// SetJSON marshals and stores a JSON value in cache
func (m *MemoryCache) SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("Error marshaling JSON for key %s: %v", key, err))
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return m.SetString(ctx, key, string(data), ttl)
}

// IncrementCounter increments a counter by given amount
func (m *MemoryCache) IncrementCounter(ctx context.Context, key string, amount int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.data[key]
	if !exists {
		m.data[key] = &cacheEntry{
			value:     amount,
			expiresAt: time.Now().Add(24 * time.Hour),
		}
		return nil
	}

	if time.Now().After(entry.expiresAt) {
		m.data[key] = &cacheEntry{
			value:     amount,
			expiresAt: time.Now().Add(24 * time.Hour),
		}
		return nil
	}

	// Increment existing value
	current, ok := entry.value.(int64)
	if !ok {
		return fmt.Errorf("value is not an integer")
	}

	entry.value = current + amount
	return nil
}

// DecrementCounter decrements a counter by given amount
func (m *MemoryCache) DecrementCounter(ctx context.Context, key string, amount int64) error {
	return m.IncrementCounter(ctx, key, -amount)
}

// GetCounter gets a counter value
func (m *MemoryCache) GetCounter(ctx context.Context, key string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, exists := m.data[key]
	if !exists {
		return 0, ErrKeyNotFound
	}

	if time.Now().After(entry.expiresAt) {
		return 0, ErrKeyNotFound
	}

	val, ok := entry.value.(int64)
	if !ok {
		return 0, fmt.Errorf("value is not an integer")
	}

	return val, nil
}

// SetCounter sets a counter value
func (m *MemoryCache) SetCounter(ctx context.Context, key string, value int64, ttl time.Duration) error {
	return m.Set(ctx, key, value, ttl)
}

// Health checks if the cache is healthy
func (m *MemoryCache) Health(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Simple health check - if we can access the data map, we're healthy
	if m.data == nil {
		return fmt.Errorf("cache data is nil")
	}

	return nil
}

// Close closes the cache (no-op for memory cache)
func (m *MemoryCache) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = nil
	return nil
}

// cleanupExpired periodically removes expired entries
func (m *MemoryCache) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()

		now := time.Now()
		for key, entry := range m.data {
			if now.After(entry.expiresAt) {
				delete(m.data, key)
			}
		}

		m.mu.Unlock()
	}
}

// evictOldest removes the entry with the earliest expiration time
func (m *MemoryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range m.data {
		if oldestTime.IsZero() || entry.expiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.expiresAt
		}
	}

	if oldestKey != "" {
		delete(m.data, oldestKey)
		logging.Z().Info(fmt.Sprintf("Evicted oldest entry: %s", oldestKey))
	}
}

// exists helper function
func exists(data map[string]*cacheEntry, key string) bool {
	_, ok := data[key]
	return ok
}
