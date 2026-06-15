package controllers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/cache"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
	"axiomnizam.bitbd.net/axiomnizam/internal/utils/logger"
	"go.uber.org/zap"
)

// ReconcilationStatus represents the status of a reconciliation
type ReconcilationStatus string

// ReconcileEvent represents an event during reconciliation
type ReconcileEvent struct {
	Type       string    `json:"type"`
	ObjectName string    `json:"objectName"`
	Namespace  string    `json:"namespace"`
	Reason     string    `json:"reason"`
	Message    string    `json:"message"`
	Timestamp  time.Time `json:"timestamp"`
	Generation int64     `json:"generation"`
}

// ReconcileContext holds context for a reconciliation
type ReconcileContext struct {
	Ctx          context.Context
	Resource     resources.Resource
	Informer     cache.Informer
	Generation   int64
	Status       ReconcilationStatus
	Events       []ReconcileEvent
	RetryCount   int
	MaxRetries   int
	LastError    error
	LastSyncTime time.Time
	FinalizeFunc func(context.Context, resources.Resource) error
	mu           sync.RWMutex
}

// AddEvent adds a reconciliation event
func (rc *ReconcileContext) AddEvent(eventType, reason, message string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	meta := rc.Resource.GetObjectMeta()
	event := ReconcileEvent{
		Type:       eventType,
		ObjectName: meta.Name,
		Namespace:  meta.Namespace,
		Reason:     reason,
		Message:    message,
		Timestamp:  time.Now(),
		Generation: rc.Generation,
	}
	rc.Events = append(rc.Events, event)
}

// GetEvents returns all events
func (rc *ReconcileContext) GetEvents() []ReconcileEvent {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	events := make([]ReconcileEvent, len(rc.Events))
	copy(events, rc.Events)
	return events
}

// ReconciliationFramework coordinates the reconciliation process
type ReconciliationFramework struct {
	logger  *logger.Logger
	queue   chan ReconcileRequest
	workers int
	stopCh  chan struct{}
	mu      sync.RWMutex
	running bool
}

// NewReconciliationFramework creates a new reconciliation framework
func NewReconciliationFramework(workerCount int) *ReconciliationFramework {
	log, _ := logger.New("development")
	return &ReconciliationFramework{
		logger:  log,
		queue:   make(chan ReconcileRequest, 100),
		workers: workerCount,
		stopCh:  make(chan struct{}),
	}
}

// Start starts the reconciliation framework
func (rf *ReconciliationFramework) Start(ctx context.Context) error {
	rf.mu.Lock()
	if rf.running {
		rf.mu.Unlock()
		return errors.New("framework already running")
	}
	rf.running = true
	rf.mu.Unlock()

	for i := 0; i < rf.workers; i++ {
		go rf.worker(ctx)
	}

	return nil
}

// Stop stops the reconciliation framework
func (rf *ReconciliationFramework) Stop() error {
	rf.mu.Lock()
	if !rf.running {
		rf.mu.Unlock()
		return errors.New("framework not running")
	}
	rf.running = false
	rf.mu.Unlock()

	close(rf.stopCh)
	return nil
}

// Enqueue enqueues a reconcile request
func (rf *ReconciliationFramework) Enqueue(req ReconcileRequest) {
	select {
	case rf.queue <- req:
	case <-rf.stopCh:
		// Framework is stopping
	}
}

// worker processes reconciliation requests
func (rf *ReconciliationFramework) worker(ctx context.Context) {
	for {
		select {
		case <-rf.stopCh:
			return
		case req := <-rf.queue:
			rf.handleReconcileRequest(ctx, req)
		}
	}
}

// handleReconcileRequest handles a single reconcile request following:
// Observe → Compare → Act → Update Status
func (rf *ReconciliationFramework) handleReconcileRequest(ctx context.Context, req ReconcileRequest) {
	// Phase 1: OBSERVE - Get current state
	rf.logger.Warn("reconciling resource", zap.String("namespace", req.Namespace), zap.String("name", req.Name))

	// Get the current resource from informer or store
	// For now, skip the store lookup since ReconciliationFramework doesn't have it
	// The resource should come through the queue from the informer

	resource, ok := req.Resource.(resources.Resource)
	if !ok {
		rf.logger.Error("object is not a Resource", zap.Any("obj", req.Resource))
		return
	}

	meta := resource.GetObjectMeta()
	generation := meta.Generation

	// Create reconciliation context
	reconcileCtx := &ReconcileContext{
		Ctx:        ctx,
		Resource:   resource,
		Generation: generation,
		MaxRetries: 3,
	}

	reconcileCtx.AddEvent("Reconciling", "Started", fmt.Sprintf("reconciliation started for generation %d", generation))

	// Phase 2: COMPARE - Compare desired state (spec) vs actual state (status)
	// This is where the reconciler implements business logic
	rf.logger.Debug("Starting reconciliation", zap.String("namespace", meta.Namespace), zap.String("name", meta.Name), zap.Int64("generation", generation))

	// Phase 3: ACT - Execute changes
	// Phase 4: Update Status
	result, err := req.Reconciler.Reconcile(ctx, req)
	if err != nil {
		reconcileCtx.AddEvent("ReconciliationFailed", "Error", err.Error())
		reconcileCtx.LastError = err
		reconcileCtx.RetryCount++

		if reconcileCtx.RetryCount < reconcileCtx.MaxRetries {
			reconcileCtx.Status = ReconcilationStatus(StatusRequeue)
			rf.logger.Warn("reconciliation failed, requeuing", zap.Error(err))
			time.Sleep(time.Second * time.Duration(reconcileCtx.RetryCount))
			rf.Enqueue(req)
		} else {
			reconcileCtx.Status = ReconcilationStatus(StatusFailed)
			reconcileCtx.AddEvent("ReconciliationFailed", "MaxRetriesExceeded", "max retries exceeded")
			rf.logger.Error("reconciliation failed after retries", zap.Error(err))
		}
		return
	}

	reconcileCtx.LastSyncTime = time.Now()
	reconcileCtx.Status = ReconcilationStatus(StatusSuccess)
	reconcileCtx.AddEvent("Reconciled", "Success", "reconciliation completed successfully")

	rf.logger.Debug("reconciliation completed", zap.String("namespace", meta.Namespace), zap.String("name", meta.Name))

	// Handle requeue if needed
	if result.Requeue {
		requeueAfter := result.RequeueAfter
		if requeueAfter == 0 {
			requeueAfter = 5 * time.Second
		}
		time.Sleep(requeueAfter)
		rf.Enqueue(req)
	}
}

// FinalizerManager handles finalizers for graceful deletion
type FinalizerManager struct {
	finalizers map[string]func(context.Context, resources.Resource) error
	mu         sync.RWMutex
}

// NewFinalizerManager creates a new finalizer manager
func NewFinalizerManager() *FinalizerManager {
	return &FinalizerManager{
		finalizers: make(map[string]func(context.Context, resources.Resource) error),
	}
}

// Register registers a finalizer
func (fm *FinalizerManager) Register(name string, fn func(context.Context, resources.Resource) error) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.finalizers[name] = fn
}

// Execute executes all registered finalizers
func (fm *FinalizerManager) Execute(ctx context.Context, resource resources.Resource) error {
	fm.mu.RLock()
	finalizers := make([]string, 0, len(fm.finalizers))
	for name := range fm.finalizers {
		finalizers = append(finalizers, name)
	}
	fm.mu.RUnlock()

	for _, name := range finalizers {
		fm.mu.RLock()
		fn := fm.finalizers[name]
		fm.mu.RUnlock()

		if err := fn(ctx, resource); err != nil {
			return fmt.Errorf("finalizer %s failed: %w", name, err)
		}
	}

	return nil
}

// RemoveFinalizer removes a finalizer from a resource
func (fm *FinalizerManager) RemoveFinalizer(resource resources.Resource, finalizerName string) {
	meta := resource.GetObjectMeta()
	finalizers := make([]string, 0)
	for _, f := range meta.Finalizers {
		if f != finalizerName {
			finalizers = append(finalizers, f)
		}
	}
	meta.Finalizers = finalizers
}

// AddFinalizer adds a finalizer to a resource
func (fm *FinalizerManager) AddFinalizer(resource resources.Resource, finalizerName string) bool {
	meta := resource.GetObjectMeta()
	for _, f := range meta.Finalizers {
		if f == finalizerName {
			return false // Already exists
		}
	}
	meta.Finalizers = append(meta.Finalizers, finalizerName)
	return true
}

// HasFinalizer checks if resource has a finalizer
func (fm *FinalizerManager) HasFinalizer(resource resources.Resource, finalizerName string) bool {
	meta := resource.GetObjectMeta()
	for _, f := range meta.Finalizers {
		if f == finalizerName {
			return true
		}
	}
	return false
}

// RecordSuccess records a successful reconciliation
func (rm *ReconciliationMetrics) RecordSuccess(duration time.Duration) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.TotalReconciliations++
	rm.SuccessfulCount++
	rm.LastReconcileTime = time.Now()
	// Update average duration
	if rm.AverageDuration == 0 {
		rm.AverageDuration = duration
	} else {
		rm.AverageDuration = (rm.AverageDuration + duration) / 2
	}
}

// RecordFailure records a failed reconciliation
func (rm *ReconciliationMetrics) RecordFailure(duration time.Duration) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.TotalReconciliations++
	rm.FailedCount++
	rm.LastReconcileTime = time.Now()
}

// GetMetrics returns current metrics
func (rm *ReconciliationMetrics) GetMetrics() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return map[string]interface{}{
		"total_reconciliations": rm.TotalReconciliations,
		"successful_count":      rm.SuccessfulCount,
		"failed_count":          rm.FailedCount,
		"average_duration":      rm.AverageDuration.String(),
		"last_reconcile_time":   rm.LastReconcileTime,
	}
}
