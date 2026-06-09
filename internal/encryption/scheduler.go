// Package encryption — Phase 8: Scheduled key rotation.
//
// This file provides a background goroutine that automatically rotates
// encryption keys on a configurable interval.

package encryption

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// KeyRotationScheduler manages automatic key rotation on a schedule.
type KeyRotationScheduler struct {
	manager  SecretsManager
	interval time.Duration
	stopCh   chan struct{}
}

// NewKeyRotationScheduler creates a new scheduler.
// Rotation interval is read from ENCRYPTION_KEY_ROTATION_DAYS env var (default: 30).
func NewKeyRotationScheduler(manager SecretsManager) *KeyRotationScheduler {
	days := 30
	if v := strings.TrimSpace(os.Getenv("ENCRYPTION_KEY_ROTATION_DAYS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	return &KeyRotationScheduler{
		manager:  manager,
		interval: time.Duration(days) * 24 * time.Hour,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the background rotation loop.
func (s *KeyRotationScheduler) Start(ctx context.Context) {
	go func() {
		log.Printf("🔒 Key rotation scheduler started (interval: %s)", s.interval)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("🔒 Key rotation scheduler stopped (context cancelled)")
				return
			case <-s.stopCh:
				log.Println("🔒 Key rotation scheduler stopped")
				return
			case <-ticker.C:
				s.rotateAllKeys()
			}
		}
	}()
}

// Stop stops the scheduler.
func (s *KeyRotationScheduler) Stop() {
	close(s.stopCh)
}

// rotateAllKeys rotates all active keys that are due for rotation.
func (s *KeyRotationScheduler) rotateAllKeys() {
	keys, err := s.manager.ListKeys("")
	if err != nil {
		log.Printf("⚠️  Key rotation: failed to list keys: %v", err)
		return
	}

	now := time.Now()
	rotated := 0
	for _, key := range keys {
		if key.Status != "Active" {
			continue
		}
		// Check if rotation is due.
		if !key.NextRotation.IsZero() && now.After(key.NextRotation) {
			if _, err := s.manager.RotateKey(key.ID); err != nil {
				log.Printf("⚠️  Key rotation failed for %s: %v", key.ID, err)
				continue
			}
			rotated++
			log.Printf("🔒 Auto-rotated key %s (was version %d)", key.ID, key.Version)
		}
	}

	if rotated > 0 {
		log.Printf("🔒 Key rotation complete: %d keys rotated", rotated)
	}
}
