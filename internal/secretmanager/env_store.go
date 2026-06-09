package secretmanager

import (
	"fmt"
	"os"
	"strings"
)

// EnvSecretStore reads secrets from environment variables.
// This is the local fallback when Vault is not configured.
// Secrets are stored as env vars with a configurable prefix.
type EnvSecretStore struct {
	prefix string
}

// NewEnvSecretStore creates a new environment variable-based secret store.
// prefix is prepended to all keys (e.g., "SECRET_" → SECRET_DB_PASSWORD).
func NewEnvSecretStore(prefix string) *EnvSecretStore {
	return &EnvSecretStore{prefix: prefix}
}

// Get retrieves a secret from the environment.
func (s *EnvSecretStore) Get(key string) (string, error) {
	envKey := s.prefix + key
	val := os.Getenv(envKey)
	if val == "" {
		return "", fmt.Errorf("secret %s not found in environment (looked for %s)", key, envKey)
	}
	return val, nil
}

// Put stores a secret in the environment (runtime only, not persisted).
func (s *EnvSecretStore) Put(key, value string) error {
	envKey := s.prefix + key
	return os.Setenv(envKey, value)
}

// Delete removes a secret from the environment.
func (s *EnvSecretStore) Delete(key string) error {
	envKey := s.prefix + key
	return os.Unsetenv(envKey)
}

// List returns all environment variable keys with the configured prefix.
func (s *EnvSecretStore) List() ([]string, error) {
	var keys []string
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], s.prefix) {
			keys = append(keys, strings.TrimPrefix(parts[0], s.prefix))
		}
	}
	return keys, nil
}

// IsAvailable always returns true (env vars are always available).
func (s *EnvSecretStore) IsAvailable() bool { return true }

// Name returns the store type name.
func (s *EnvSecretStore) Name() string { return "env" }
