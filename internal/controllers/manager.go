package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/cache"
	"axiomnizam.bitbd.net/axiomnizam/internal/utils/logger"
	"go.uber.org/zap"
)

// ControllerManager manages multiple controllers and their lifecycle
type ControllerManager struct {
	logger                  *logger.Logger
	controllers             map[string]*ManagedController
	informerFactory         *cache.InformerFactory
	reconciliationFramework *ReconciliationFramework
	mu                      sync.RWMutex
	running                 bool
	stopCh                  chan struct{}
	syncedCh                chan struct{}
}

// ManagedController wraps a controller with lifecycle management
type ManagedController struct {
	Name                string
	Controller          ResourceController
	Informer            cache.Informer
	Reconciler          Reconciler
	Status              ControllerStatus
	LastSyncTime        time.Time
	ReconciliationCount int64
	FailureCount        int64
	mu                  sync.RWMutex
}

// ControllerStatus represents the status of a controller
type ControllerStatus string

const (
	StatusInitializing ControllerStatus = "initializing"
	StatusRunning      ControllerStatus = "running"
	StatusSynced       ControllerStatus = "synced"
	StatusError        ControllerStatus = "error"
	StatusStopped      ControllerStatus = "stopped"
)

// NewControllerManager creates a new controller manager
func NewControllerManager(informerFactory *cache.InformerFactory) *ControllerManager {
	log, _ := logger.New("development")
	return &ControllerManager{
		logger:                  log,
		controllers:             make(map[string]*ManagedController),
		informerFactory:         informerFactory,
		reconciliationFramework: NewReconciliationFramework(4),
		stopCh:                  make(chan struct{}),
		syncedCh:                make(chan struct{}),
	}
}

// Start starts the controller manager
func (cm *ControllerManager) Start(ctx context.Context) error {
	cm.mu.Lock()
	if cm.running {
		cm.mu.Unlock()
		return errors.New("controller manager already running")
	}
	cm.running = true
	cm.mu.Unlock()

	// Start reconciliation framework
	if err := cm.reconciliationFramework.Start(ctx); err != nil {
		return fmt.Errorf("failed to start reconciliation framework: %w", err)
	}

	// Start all controllers
	cm.mu.RLock()
	controllerNames := make([]string, 0, len(cm.controllers))
	for name := range cm.controllers {
		controllerNames = append(controllerNames, name)
	}
	cm.mu.RUnlock()

	for _, name := range controllerNames {
		cm.mu.RLock()
		managed := cm.controllers[name]
		cm.mu.RUnlock()

		if err := cm.startController(ctx, managed); err != nil {
			cm.logger.Error("failed to start controller", zap.String("name", name), zap.Error(err))
			return err
		}
	}

	// Wait for all informers to sync
	go cm.waitForSync(ctx)

	cm.logger.Info("controller manager started", zap.Int("num_controllers", len(controllerNames)))
	return nil
}

// startController starts a single controller
func (cm *ControllerManager) startController(ctx context.Context, managed *ManagedController) error {
	managed.mu.Lock()
	managed.Status = StatusInitializing
	managed.mu.Unlock()

	cm.logger.Debug("starting controller", zap.String("name", managed.Name))

	// Start the informer
	if err := cm.startInformer(ctx, managed); err != nil {
		managed.mu.Lock()
		managed.Status = StatusError
		managed.mu.Unlock()
		return fmt.Errorf("failed to start informer: %w", err)
	}

	// Start the controller
	go cm.runController(ctx, managed)

	return nil
}

// startInformer starts an informer with event handlers
func (cm *ControllerManager) startInformer(ctx context.Context, managed *ManagedController) error {
	informer := managed.Informer

	// Add event handler to queue reconciliation requests
	handler := &cache.HandlerFuncs{
		AddFunc: func(obj interface{}, isInitialList bool) error {
			cm.enqueueReconciliation(managed, obj, "Add")
			return nil
		},
		UpdateFunc: func(oldObj, newObj interface{}) error {
			cm.enqueueReconciliation(managed, newObj, "Update")
			return nil
		},
		DeleteFunc: func(obj interface{}) error {
			cm.enqueueReconciliation(managed, obj, "Delete")
			return nil
		},
	}

	informer.AddEventHandler(handler)

	// Start the informer
	go informer.Start(ctx)

	// Wait for initial sync
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	synced := make(chan struct{})
	go func() {
		for !informer.HasSynced() {
			time.Sleep(100 * time.Millisecond)
		}
		close(synced)
	}()

	select {
	case <-synced:
		managed.mu.Lock()
		managed.Status = StatusSynced
		managed.LastSyncTime = time.Now()
		managed.mu.Unlock()
		cm.logger.Debug("informer synced for controller", zap.String("name", managed.Name))
		return nil
	case <-timeout.C:
		return fmt.Errorf("informer sync timeout for controller: %s", managed.Name)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// enqueueReconciliation enqueues a reconciliation request
func (cm *ControllerManager) enqueueReconciliation(managed *ManagedController, obj interface{}, action string) {
	key := cache.ExtractKey(obj)
	parts := splitKey(key)
	req := ReconcileRequest{
		Namespace:  parts[0],
		Name:       parts[1],
		Generation: 0,
	}
	cm.reconciliationFramework.Enqueue(req)
}

func splitKey(key string) [2]string {
	parts := strings.Split(key, "/")
	if len(parts) == 2 {
		return [2]string{parts[0], parts[1]}
	}
	return [2]string{"", ""}
}

// runController runs a controller's reconciliation loop
func (cm *ControllerManager) runController(ctx context.Context, managed *ManagedController) {
	managed.mu.Lock()
	managed.Status = StatusRunning
	managed.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopCh:
			return
		}
	}
}

// waitForSync waits for all controllers to sync
func (cm *ControllerManager) waitForSync(ctx context.Context) {
	cm.mu.RLock()
	controllers := make([]*ManagedController, 0, len(cm.controllers))
	for _, managed := range cm.controllers {
		controllers = append(controllers, managed)
	}
	cm.mu.RUnlock()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopCh:
			close(cm.syncedCh)
			return
		case <-ticker.C:
			allSynced := true
			for _, managed := range controllers {
				managed.mu.RLock()
				status := managed.Status
				managed.mu.RUnlock()

				if status != StatusSynced {
					allSynced = false
					break
				}
			}

			if allSynced {
				close(cm.syncedCh)
				cm.logger.Info("all controllers synced")
				return
			}
		}
	}
}

// Stop stops the controller manager
func (cm *ControllerManager) Stop(ctx context.Context) error {
	cm.mu.Lock()
	if !cm.running {
		cm.mu.Unlock()
		return errors.New("controller manager not running")
	}
	cm.running = false
	cm.mu.Unlock()

	close(cm.stopCh)

	// Stop reconciliation framework
	if err := cm.reconciliationFramework.Stop(); err != nil {
		cm.logger.Error("failed to stop reconciliation framework", zap.Error(err))
	}

	// Stop all informers
	cm.mu.RLock()
	for _, managed := range cm.controllers {
		managed.Informer.Stop()
		managed.mu.Lock()
		managed.Status = StatusStopped
		managed.mu.Unlock()
	}
	cm.mu.RUnlock()

	cm.logger.Info("controller manager stopped")
	return nil
}

// RegisterController registers a controller
func (cm *ControllerManager) RegisterController(name string, controller ResourceController, informer cache.Informer, reconciler Reconciler) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.controllers[name]; exists {
		return fmt.Errorf("controller %s already registered", name)
	}

	cm.controllers[name] = &ManagedController{
		Name:       name,
		Controller: controller,
		Informer:   informer,
		Reconciler: reconciler,
		Status:     StatusInitializing,
	}

	return nil
}

// GetControllerStatus gets the status of a controller
func (cm *ControllerManager) GetControllerStatus(name string) (ControllerStatus, error) {
	cm.mu.RLock()
	managed, exists := cm.controllers[name]
	cm.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("controller %s not found", name)
	}

	managed.mu.RLock()
	defer managed.mu.RUnlock()
	return managed.Status, nil
}

// GetControllerMetrics gets metrics for a controller
func (cm *ControllerManager) GetControllerMetrics(name string) (map[string]interface{}, error) {
	cm.mu.RLock()
	managed, exists := cm.controllers[name]
	cm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("controller %s not found", name)
	}

	managed.mu.RLock()
	defer managed.mu.RUnlock()

	return map[string]interface{}{
		"name":                 managed.Name,
		"status":               managed.Status,
		"last_sync_time":       managed.LastSyncTime,
		"reconciliation_count": managed.ReconciliationCount,
		"failure_count":        managed.FailureCount,
	}, nil
}

// WaitForSync waits until all controllers are synced
func (cm *ControllerManager) WaitForSync(ctx context.Context) error {
	select {
	case <-cm.syncedCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetAllControllerStatus returns status of all controllers
func (cm *ControllerManager) GetAllControllerStatus() map[string]ControllerStatus {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	statuses := make(map[string]ControllerStatus)
	for name, managed := range cm.controllers {
		managed.mu.RLock()
		statuses[name] = managed.Status
		managed.mu.RUnlock()
	}
	return statuses
}
