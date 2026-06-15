package cache

import (
	"context"
	"encoding/json"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// Store defines the interface for session/cache storage.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// ChallengeCache provides caching for MFA challenges.
type ChallengeCache struct {
	store Store
}

// NewChallengeCache creates a new challenge cache.
func NewChallengeCache(s Store) *ChallengeCache {
	return &ChallengeCache{store: s}
}

// Set caches a challenge with TTL.
func (c *ChallengeCache) Set(ctx context.Context, challenge *models.Challenge) error {
	data, err := json.Marshal(challenge)
	if err != nil {
		return err
	}

	key := "challenge:" + challenge.ID.String()
	ttl := time.Until(challenge.ExpiresAt)
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	return c.store.Set(ctx, key, data, ttl)
}

// Get retrieves a cached challenge.
func (c *ChallengeCache) Get(ctx context.Context, id models.ChallengeID) (*models.Challenge, error) {
	key := "challenge:" + id.String()
	data, err := c.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if data == nil {
		return nil, nil
	}

	challenge := &models.Challenge{}
	if err := json.Unmarshal(data, challenge); err != nil {
		return nil, err
	}

	return challenge, nil
}

// Delete removes a cached challenge.
func (c *ChallengeCache) Delete(ctx context.Context, id models.ChallengeID) error {
	key := "challenge:" + id.String()
	return c.store.Delete(ctx, key)
}

// SessionCache provides caching for user sessions during MFA enrollment.
type SessionCache struct {
	store Store
}

// NewSessionCache creates a new session cache.
func NewSessionCache(s Store) *SessionCache {
	return &SessionCache{store: s}
}

// SessionData represents MFA setup session state.
type SessionData struct {
	UserID    models.UserID
	FactorID  models.FactorID
	Secret    string
	Nonce     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Set stores session data with TTL.
func (s *SessionCache) Set(ctx context.Context, sessionID string, data *SessionData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	key := "session:" + sessionID
	ttl := time.Until(data.ExpiresAt)
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	return s.store.Set(ctx, key, jsonData, ttl)
}

// Get retrieves session data.
func (s *SessionCache) Get(ctx context.Context, sessionID string) (*SessionData, error) {
	key := "session:" + sessionID
	data, err := s.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if data == nil {
		return nil, nil
	}

	session := &SessionData{}
	if err := json.Unmarshal(data, session); err != nil {
		return nil, err
	}

	return session, nil
}

// Delete removes session data.
func (s *SessionCache) Delete(ctx context.Context, sessionID string) error {
	key := "session:" + sessionID
	return s.store.Delete(ctx, key)
}

// RateLimitStore provides rate limiting for authentication attempts.
type RateLimitStore struct {
	store Store
}

// NewRateLimitStore creates a new rate limit store.
func NewRateLimitStore(s Store) *RateLimitStore {
	return &RateLimitStore{store: s}
}

// CheckLimit checks if a user has exceeded rate limits.
func (r *RateLimitStore) CheckLimit(ctx context.Context, userID models.UserID, limit int, window time.Duration) (bool, error) {
	key := "ratelimit:" + userID.String()

	var count int
	data, err := r.store.Get(ctx, key)
	if err != nil {
		return false, err
	}

	if data != nil {
		_ = json.Unmarshal(data, &count)
	}

	if count >= limit {
		return false, nil // Rate limit exceeded
	}

	// Increment counter
	count++
	jsonData, _ := json.Marshal(count)
	_ = r.store.Set(ctx, key, jsonData, window)

	return true, nil
}

// Reset clears rate limit for a user.
func (r *RateLimitStore) Reset(ctx context.Context, userID models.UserID) error {
	key := "ratelimit:" + userID.String()
	return r.store.Delete(ctx, key)
}

// InMemoryStore provides a simple in-memory implementation for testing.
type InMemoryStore struct {
	data map[string][]byte
	ttl  map[string]time.Time
}

// NewInMemoryStore creates a new in-memory store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		data: make(map[string][]byte),
		ttl:  make(map[string]time.Time),
	}
}

// Get retrieves a value from memory.
func (s *InMemoryStore) Get(ctx context.Context, key string) ([]byte, error) {
	// Check if key has expired
	if ttl, exists := s.ttl[key]; exists && time.Now().After(ttl) {
		delete(s.data, key)
		delete(s.ttl, key)
		return nil, nil
	}

	return s.data[key], nil
}

// Set stores a value in memory with TTL.
func (s *InMemoryStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	s.data[key] = value
	s.ttl[key] = time.Now().Add(ttl)
	return nil
}

// Delete removes a value from memory.
func (s *InMemoryStore) Delete(ctx context.Context, key string) error {
	delete(s.data, key)
	delete(s.ttl, key)
	return nil
}

// Exists checks if a key exists in memory.
func (s *InMemoryStore) Exists(ctx context.Context, key string) (bool, error) {
	_, exists := s.data[key]
	return exists, nil
}
