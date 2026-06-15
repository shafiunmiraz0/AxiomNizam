package controller

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"sync"
	"time"
)

// Manager coordinates the MFA reconciliation loops.
// Follows the K8s controller manager pattern.
type Manager struct {
	mu            sync.Mutex
	running       bool
	stopCh        chan struct{}
	factorCtrl    *FactorReconciler
	resyncInterval time.Duration
}

// NewManager creates a new Gatekeeper controller manager.
func NewManager(factorCtrl *FactorReconciler) *Manager {
	return &Manager{
		factorCtrl:    factorCtrl,
		resyncInterval: 5 * time.Minute,
	}
}

// Start begins all reconciliation loops.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.stopCh = make(chan struct{})
	m.mu.Unlock()

	logging.Z().Info("✅ Gatekeeper: Controller manager started")

	// Start periodic reconciliation
	go m.runReconcileLoop(ctx)
}

// Stop halts all reconciliation loops.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	close(m.stopCh)
	m.running = false
	logging.Z().Info("✅ Gatekeeper: Controller manager stopped")
}

func (m *Manager) runReconcileLoop(ctx context.Context) {
	ticker := time.NewTicker(m.resyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			// Periodic reconciliation of expired challenges
			if m.factorCtrl != nil {
				if err := m.factorCtrl.ReconcileExpiredChallenges(ctx); err != nil {
					logging.Z().Info(fmt.Sprintf("⚠️  Gatekeeper: expired challenge cleanup failed: %v", err))
				}
			}
		}
	}
}