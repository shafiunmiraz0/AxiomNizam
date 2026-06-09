// Package secretmanager provides centralized secret management for AxiomNizam (Phase 14).
//
// Components:
//   - SecretStore: abstract interface for secret storage (Vault, env, file)
//   - EnvSecretStore: reads secrets from environment variables (local fallback)
//   - SecretManager: orchestrates secret lifecycle (versioning, rotation, grace period)
//   - SecretAccessLog: audit trail for secret access
//   - SecretRotator: scheduled rotation of database credentials, API keys, encryption keys
package secretmanager

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// SecretStore abstracts secret storage backends.
// Implementations: EnvSecretStore (env vars), VaultSecretStore (HashiCorp Vault).
type SecretStore interface {
	// Get retrieves a secret by key. Returns the secret value or an error.
	Get(key string) (string, error)
	// Put stores a secret with the given key and value.
	Put(key, value string) error
	// Delete removes a secret by key.
	Delete(key string) error
	// List returns all secret keys (not values).
	List() ([]string, error)
	// IsAvailable returns true if the store is reachable.
	IsAvailable() bool
	// Name returns the store type name for logging.
	Name() string
}

// SecretVersion represents a versioned secret.
type SecretVersion struct {
	Value     string    `json:"-"` // never serialize the value
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	IsActive  bool      `json:"is_active"`
	RotatedBy string    `json:"rotated_by,omitempty"`
}

// SecretEntry holds all versions of a secret.
type SecretEntry struct {
	Key        string           `json:"key"`
	Versions   []*SecretVersion `json:"versions"`
	MaxVersion int              `json:"max_version"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

// AccessEvent records a secret access for the audit trail.
type AccessEvent struct {
	Key       string    `json:"key"`
	Action    string    `json:"action"` // "read", "write", "rotate", "delete"
	UserID    string    `json:"user_id,omitempty"`
	IPAddress string    `json:"ip_address,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// SecretManager orchestrates secret lifecycle with versioning and grace periods.
type SecretManager struct {
	mu          sync.RWMutex
	store       SecretStore
	secrets     map[string]*SecretEntry
	accessLog   []AccessEvent
	maxVersions int
	gracePeriod time.Duration
	stopCh      chan struct{}
}

// NewSecretManager creates a new secret manager.
func NewSecretManager(store SecretStore, maxVersions int, gracePeriod time.Duration) *SecretManager {
	return &SecretManager{
		store:       store,
		secrets:     make(map[string]*SecretEntry),
		accessLog:   make([]AccessEvent, 0, 1000),
		maxVersions: maxVersions,
		gracePeriod: gracePeriod,
		stopCh:      make(chan struct{}),
	}
}

// Get retrieves the active version of a secret.
func (m *SecretManager) Get(key string) (string, error) {
	m.mu.RLock()
	entry, exists := m.secrets[key]
	m.mu.RUnlock()

	if !exists {
		// Try the backing store directly
		val, err := m.store.Get(key)
		if err != nil {
			m.recordAccess(key, "read", "", "", false, err.Error())
			return "", err
		}
		m.recordAccess(key, "read", "", "", true, "")
		return val, nil
	}

	// Find the active version
	for _, v := range entry.Versions {
		if v.IsActive {
			m.recordAccess(key, "read", "", "", true, "")
			return v.Value, nil
		}
	}

	// Fallback to backing store
	val, err := m.store.Get(key)
	if err != nil {
		m.recordAccess(key, "read", "", "", false, err.Error())
		return "", err
	}
	m.recordAccess(key, "read", "", "", true, "")
	return val, nil
}

// Put stores a new version of a secret. Previous versions are kept for grace period.
func (m *SecretManager) Put(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.secrets[key]
	if !exists {
		entry = &SecretEntry{
			Key:       key,
			Versions:  make([]*SecretVersion, 0),
			CreatedAt: time.Now().UTC(),
		}
		m.secrets[key] = entry
	}

	// Deactivate current active version
	for _, v := range entry.Versions {
		v.IsActive = false
	}

	// Create new version
	entry.MaxVersion++
	now := time.Now().UTC()
	newVersion := &SecretVersion{
		Value:     value,
		Version:   entry.MaxVersion,
		CreatedAt: now,
		ExpiresAt: now.Add(m.gracePeriod),
		IsActive:  true,
	}
	entry.Versions = append(entry.Versions, newVersion)
	entry.UpdatedAt = now

	// Prune old versions beyond maxVersions
	if len(entry.Versions) > m.maxVersions {
		entry.Versions = entry.Versions[len(entry.Versions)-m.maxVersions:]
	}

	// Write to backing store
	if err := m.store.Put(key, value); err != nil {
		m.recordAccess(key, "write", "", "", false, err.Error())
		return err
	}

	m.recordAccess(key, "write", "", "", true, "")
	log.Printf("✅ [SecretManager] Secret %s updated to version %d", key, entry.MaxVersion)
	return nil
}

// Rotate creates a new version of a secret, keeping the old one active during the grace period.
func (m *SecretManager) Rotate(key, newValue string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.secrets[key]
	if !exists {
		// Just create it
		m.mu.Unlock()
		return m.Put(key, newValue)
	}

	// Keep old active version as "grace period" version (still active)
	// The new version becomes active, old version expires after grace period
	entry.MaxVersion++
	now := time.Now().UTC()
	newVersion := &SecretVersion{
		Value:     newValue,
		Version:   entry.MaxVersion,
		CreatedAt: now,
		IsActive:  true,
		RotatedBy: "system",
	}

	// Deactivate old versions
	for _, v := range entry.Versions {
		v.IsActive = false
	}

	entry.Versions = append(entry.Versions, newVersion)
	entry.UpdatedAt = now

	// Write to backing store
	if err := m.store.Put(key, newValue); err != nil {
		m.recordAccess(key, "rotate", "", "", false, err.Error())
		return err
	}

	m.recordAccess(key, "rotate", "", "", true, "")
	log.Printf("🔄 [SecretManager] Secret %s rotated to version %d (grace period: %s)",
		key, entry.MaxVersion, m.gracePeriod)
	return nil
}

// GetByVersion retrieves a specific version of a secret (for grace period access).
func (m *SecretManager) GetByVersion(key string, version int) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, exists := m.secrets[key]
	if !exists {
		return "", fmt.Errorf("secret %s not found", key)
	}

	for _, v := range entry.Versions {
		if v.Version == version {
			m.recordAccess(key, "read", "", "", true, "")
			return v.Value, nil
		}
	}

	return "", fmt.Errorf("secret %s version %d not found", key, version)
}

// GetAccessLog returns the recent access events.
func (m *SecretManager) GetAccessLog(limit int) []AccessEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > len(m.accessLog) {
		limit = len(m.accessLog)
	}
	start := len(m.accessLog) - limit
	if start < 0 {
		start = 0
	}
	result := make([]AccessEvent, limit)
	copy(result, m.accessLog[start:])
	return result
}

// CleanupExpiredVersions removes expired versions from all secrets.
func (m *SecretManager) CleanupExpiredVersions() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	now := time.Now().UTC()
	for _, entry := range m.secrets {
		var kept []*SecretVersion
		for _, v := range entry.Versions {
			if !v.IsActive && !v.ExpiresAt.IsZero() && now.After(v.ExpiresAt) {
				removed++
				continue
			}
			kept = append(kept, v)
		}
		entry.Versions = kept
	}
	if removed > 0 {
		log.Printf("🧹 [SecretManager] Cleaned up %d expired secret versions", removed)
	}
	return removed
}

// StartCleanupLoop starts a background goroutine that cleans up expired versions.
func (m *SecretManager) StartCleanupLoop(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.CleanupExpiredVersions()
			case <-m.stopCh:
				return
			}
		}
	}()
}

// Stop halts the cleanup loop.
func (m *SecretManager) Stop() {
	close(m.stopCh)
}

func (m *SecretManager) recordAccess(key, action, userID, ip string, success bool, errMsg string) {
	evt := AccessEvent{
		Key:       key,
		Action:    action,
		UserID:    userID,
		IPAddress: ip,
		Timestamp: time.Now().UTC(),
		Success:   success,
		Error:     errMsg,
	}
	m.accessLog = append(m.accessLog, evt)
	if len(m.accessLog) > 10000 {
		m.accessLog = m.accessLog[1:]
	}
}
