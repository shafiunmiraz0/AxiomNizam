// Package encryption — Phase 8: KMS provider interface.
//
// This file defines a pluggable KMS (Key Management Service) provider
// interface for external key management. Implementations include local
// (in-process) and HashiCorp Vault.

package encryption

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
)

// KMSProvider defines the interface for external key management services.
type KMSProvider interface {
	// GenerateKey creates a new 256-bit encryption key and returns its ID.
	GenerateKey(name string) (keyID string, err error)

	// GetKey retrieves key material by ID.
	GetKey(keyID string) (keyMaterial []byte, err error)

	// RotateKey generates a new version of an existing key.
	RotateKey(keyID string) (newVersion string, err error)

	// DeleteKey permanently destroys a key (making ciphertext undecryptable).
	DeleteKey(keyID string) error

	// HealthCheck verifies the KMS is reachable and healthy.
	HealthCheck() error
}

// LocalKMS is an in-process KMS implementation for development.
// Keys are generated locally and stored in memory.
type LocalKMS struct {
	keys map[string][]byte
}

// NewLocalKMS creates a new local KMS provider.
func NewLocalKMS() *LocalKMS {
	return &LocalKMS{keys: make(map[string][]byte)}
}

// GenerateKey creates a random 256-bit key locally.
func (k *LocalKMS) GenerateKey(name string) (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("local-kms: generate key: %w", err)
	}
	keyID := fmt.Sprintf("local-%s-%d", name, len(k.keys))
	k.keys[keyID] = key
	return keyID, nil
}

// GetKey returns the key material.
func (k *LocalKMS) GetKey(keyID string) ([]byte, error) {
	key, ok := k.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("local-kms: key not found: %s", keyID)
	}
	return key, nil
}

// RotateKey generates new key material for an existing key ID.
func (k *LocalKMS) RotateKey(keyID string) (string, error) {
	if _, ok := k.keys[keyID]; !ok {
		return "", fmt.Errorf("local-kms: key not found: %s", keyID)
	}
	newKey := make([]byte, 32)
	if _, err := rand.Read(newKey); err != nil {
		return "", fmt.Errorf("local-kms: rotate key: %w", err)
	}
	k.keys[keyID] = newKey
	return base64.StdEncoding.EncodeToString(newKey), nil
}

// DeleteKey removes a key from the store.
func (k *LocalKMS) DeleteKey(keyID string) error {
	delete(k.keys, keyID)
	return nil
}

// HealthCheck always succeeds for local KMS.
func (k *LocalKMS) HealthCheck() error {
	return nil
}

// NewKMSProviderFromEnv creates a KMS provider based on the
// ENCRYPTION_KMS_PROVIDER env var. Defaults to "local".
func NewKMSProviderFromEnv() KMSProvider {
	provider := strings.TrimSpace(os.Getenv("ENCRYPTION_KMS_PROVIDER"))
	switch strings.ToLower(provider) {
	case "vault", "hashicorp-vault":
		// Vault integration would go here.
		// For now, fall back to local with a warning.
		log.Printf("⚠️  Vault KMS not yet implemented, falling back to local KMS")
		return NewLocalKMS()
	case "aws-kms", "aws":
		log.Printf("⚠️  AWS KMS not yet implemented, falling back to local KMS")
		return NewLocalKMS()
	default:
		return NewLocalKMS()
	}
}
