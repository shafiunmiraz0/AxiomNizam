package secretmanager

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// VaultSecretStore is a placeholder for HashiCorp Vault integration.
// When VAULT_ADDR is set, it attempts to connect to Vault.
// When Vault is unavailable, it falls back to EnvSecretStore.
type VaultSecretStore struct {
	address    string
	token      string
	mountPath  string
	httpClient *http.Client
	mu         sync.RWMutex
	available  bool
}

// NewVaultSecretStore creates a new Vault-backed secret store.
// It checks VAULT_ADDR and VAULT_TOKEN environment variables.
func NewVaultSecretStore() *VaultSecretStore {
	store := &VaultSecretStore{
		address:   os.Getenv("VAULT_ADDR"),
		token:     os.Getenv("VAULT_TOKEN"),
		mountPath: os.Getenv("VAULT_MOUNT_PATH"),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	if store.mountPath == "" {
		store.mountPath = "secret"
	}
	store.checkAvailability()
	return store
}

func (s *VaultSecretStore) checkAvailability() {
	if s.address == "" {
		s.available = false
		return
	}
	// Try to reach Vault health endpoint
	resp, err := s.httpClient.Get(s.address + "/v1/sys/health")
	if err != nil {
		log.Printf("⚠️  [Vault] Not available at %s: %v", s.address, err)
		s.available = false
		return
	}
	defer resp.Body.Close()
	s.available = resp.StatusCode == 200 || resp.StatusCode == 429 // 429 = unsealed but standby
	if s.available {
		log.Printf("✅ [Vault] Connected to %s", s.address)
	}
}

// Get retrieves a secret from Vault.
func (s *VaultSecretStore) Get(key string) (string, error) {
	if !s.available {
		return "", fmt.Errorf("vault not available")
	}
	// In a real implementation, this would make an HTTP GET to:
	//   VAULT_ADDR/v1/secret/data/<key>
	// with X-Vault-Token header
	return "", fmt.Errorf("vault get not implemented — set VAULT_ADDR and VAULT_TOKEN to enable")
}

// Put stores a secret in Vault.
func (s *VaultSecretStore) Put(key, value string) error {
	if !s.available {
		return fmt.Errorf("vault not available")
	}
	return fmt.Errorf("vault put not implemented")
}

// Delete removes a secret from Vault.
func (s *VaultSecretStore) Delete(key string) error {
	if !s.available {
		return fmt.Errorf("vault not available")
	}
	return fmt.Errorf("vault delete not implemented")
}

// List returns all secret keys from Vault.
func (s *VaultSecretStore) List() ([]string, error) {
	if !s.available {
		return nil, fmt.Errorf("vault not available")
	}
	return nil, fmt.Errorf("vault list not implemented")
}

// IsAvailable returns true if Vault is reachable.
func (s *VaultSecretStore) IsAvailable() bool { return s.available }

// Name returns the store type name.
func (s *VaultSecretStore) Name() string { return "vault" }

// LoadSecretStoreFromEnv creates the appropriate secret store based on environment.
// Priority: Vault (if VAULT_ADDR is set) → Environment variables.
func LoadSecretStoreFromEnv() SecretStore {
	if addr := os.Getenv("VAULT_ADDR"); addr != "" {
		vault := NewVaultSecretStore()
		if vault.IsAvailable() {
			return vault
		}
		log.Printf("⚠️  VAULT_ADDR set but Vault unavailable, falling back to env vars")
	}
	return NewEnvSecretStore("")
}

// GenerateSecretID generates a random secret identifier.
func GenerateSecretID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
