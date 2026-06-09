package secretmanager

import (
	"crypto/rand"
	"fmt"
	"log"
	"time"
)

// SecretRotator performs scheduled rotation of database credentials, API keys,
// and encryption keys. It integrates with the SecretManager for versioned storage.
type SecretRotator struct {
	manager  *SecretManager
	configs  []RotationConfig
	stopCh   chan struct{}
}

// RotationConfig defines what to rotate and how often.
type RotationConfig struct {
	Key        string        // Secret key name (e.g., "POSTGRES_PASSWORD")
	Interval   time.Duration // How often to rotate
	Generator  func() (string, error) // Function to generate new value
	LastRotate time.Time     // Last rotation time
}

// NewSecretRotator creates a new secret rotator.
func NewSecretRotator(manager *SecretManager) *SecretRotator {
	return &SecretRotator{
		manager: manager,
		configs: make([]RotationConfig, 0),
		stopCh:  make(chan struct{}),
	}
}

// AddConfig adds a rotation configuration.
func (r *SecretRotator) AddConfig(cfg RotationConfig) {
	r.configs = append(r.configs, cfg)
}

// Start begins the rotation loop.
func (r *SecretRotator) Start() {
	go r.run()
	log.Printf("✅ [SecretRotator] Started with %d rotation configs", len(r.configs))
}

// Stop halts the rotation loop.
func (r *SecretRotator) Stop() {
	close(r.stopCh)
}

func (r *SecretRotator) run() {
	ticker := time.NewTicker(1 * time.Hour) // check every hour
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.checkAndRotate()
		case <-r.stopCh:
			return
		}
	}
}

func (r *SecretRotator) checkAndRotate() {
	now := time.Now().UTC()
	for i, cfg := range r.configs {
		if cfg.Generator == nil {
			continue
		}
		if now.Sub(cfg.LastRotate) >= cfg.Interval {
			newValue, err := cfg.Generator()
			if err != nil {
				log.Printf("⚠️  [SecretRotator] Failed to generate new value for %s: %v", cfg.Key, err)
				continue
			}
			if err := r.manager.Rotate(cfg.Key, newValue); err != nil {
				log.Printf("⚠️  [SecretRotator] Failed to rotate %s: %v", cfg.Key, err)
				continue
			}
			r.configs[i].LastRotate = now
			log.Printf("🔄 [SecretRotator] Rotated %s", cfg.Key)
		}
	}
}

// GenerateRandomPassword generates a cryptographically random password.
func GenerateRandomPassword(length int) (string, error) {
	if length < 16 {
		length = 16
	}
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random password: %w", err)
	}
	for i, v := range b {
		b[i] = charset[int(v)%len(charset)]
	}
	return string(b), nil
}
