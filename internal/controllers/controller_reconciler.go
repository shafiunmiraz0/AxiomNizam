package controllers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/cache"
	"axiomnizam.bitbd.net/axiomnizam/internal/events"
	"axiomnizam.bitbd.net/axiomnizam/internal/jobs"
	"axiomnizam.bitbd.net/axiomnizam/internal/rbac"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
	"axiomnizam.bitbd.net/axiomnizam/internal/utils/logger"
	"go.uber.org/zap"
)

// ControllerReconciler handles the full reconciliation lifecycle
//
// Deprecated (Phase 1 checklist): this type implements a legacy phased
// reconciliation loop (initialise → admit → observe → reconcile →
// finalize) that has been superseded by `reconciler.Reconciler` +
// resource-specific reconcilers under `internal/resources/*`.  No
// external caller constructs this struct any more; it remains for the
// in-package bridge (`controller_reconciler_bridge.go`) and will be
// removed once the phased-middleware port is complete.
type ControllerReconciler struct {
	Name                  string
	Namespace             string
	ResourceKind          string
	ResourceVersion       int64
	Logger                *logger.Logger
	EventBus              *events.EventBusWithLifecycle
	AdmissionController   *AdmissionController
	RBACEngine            *rbac.Engine
	ResourceManager       *ResourceManager
	Informer              cache.Informer
	Queue                 jobs.Queue
	RetryPolicy           *RetryPolicy
	FinalizerName         string
	StatusCollector       *StatusCollector
	ConditionManager      *ConditionManager
	ObserverManager       *ObserverManager
	mu                    sync.RWMutex
	activeReconciliations map[string]*ReconciliationState
	maxConcurrent         int
}

// ReconciliationState tracks state during reconciliation
type ReconciliationState struct {
	Key                string
	Resource           resources.Resource
	StartTime          time.Time
	LastStatusUpdate   time.Time
	RetryCount         int
	MaxRetries         int
	Status             ReconciliationStatus
	Conditions         []*Condition
	Events             []*events.ResourceEvent
	Finalizers         []string
	ObservedGeneration int64
	ProcessingTime     time.Duration
	Error              error
	Mutations          []interface{}
	Phase              ReconciliationPhase
}

// ReconciliationPhase represents stages of reconciliation
type ReconciliationPhase string

const (
	PhaseInitialize ReconciliationPhase = "initialize"
	PhaseAdmit      ReconciliationPhase = "admit"
	PhaseObserve    ReconciliationPhase = "observe"
	PhaseReconcile  ReconciliationPhase = "reconcile"
	PhaseFinalize   ReconciliationPhase = "finalize"
	PhaseComplete   ReconciliationPhase = "complete"
)

// ReconciliationStatus represents reconciliation outcome
type ReconciliationStatus string

const (
	StatusPending    ReconciliationStatus = "pending"
	StatusProcessing ReconciliationStatus = "processing"
	StatusSuccess    ReconciliationStatus = "success"
	StatusRequeue    ReconciliationStatus = "requeue"
	StatusFailed     ReconciliationStatus = "failed"
)

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxRetries        int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
	JitterFraction    float64
}

// StatusCollector collects and manages status
type StatusCollector struct {
	mu       sync.RWMutex
	statuses map[string]map[string]interface{}
}

// ConditionManager manages resource conditions
type ConditionManager struct {
	mu         sync.RWMutex
	conditions map[string][]*Condition
}

// Condition represents a resource condition
type Condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"` // True, False, Unknown
	LastUpdateTime     time.Time `json:"last_update_time"`
	LastTransitionTime time.Time `json:"last_transition_time"`
	Reason             string    `json:"reason"`
	Message            string    `json:"message"`
	Generation         int64     `json:"generation"`
}

// ObserverManager manages resource observers
type ObserverManager struct {
	mu        sync.RWMutex
	observers map[string][]ResourceObserver
}

// ResourceObserver observes resource state
type ResourceObserver interface {
	Name() string
	Observe(ctx context.Context, res resources.Resource) (map[string]interface{}, error)
	GetInterval() time.Duration
}

// ReconciliationMetrics tracks reconciliation metrics
type ReconciliationMetrics struct {
	mu                   sync.RWMutex
	TotalReconciliations int64
	SuccessfulCount      int64
	FailedCount          int64
	RequeuedCount        int64
	AverageProcessTime   float64
	CurrentActive        int
	ErrorsByKind         map[string]int64
	LastReconcileTime    time.Time
	AverageDuration      time.Duration
}

// NewControllerReconciler creates a new controller reconciler
func NewControllerReconciler(
	name string,
	kind string,
	eventBus *events.EventBusWithLifecycle,
	admissionCtrl *AdmissionController,
	rbacEngine *rbac.Engine,
	resourceMgr *ResourceManager,
	queue jobs.Queue,
) *ControllerReconciler {
	log, _ := logger.New("development")

	return &ControllerReconciler{
		Name:                  name,
		ResourceKind:          kind,
		Logger:                log,
		EventBus:              eventBus,
		AdmissionController:   admissionCtrl,
		RBACEngine:            rbacEngine,
		ResourceManager:       resourceMgr,
		Queue:                 queue,
		FinalizerName:         fmt.Sprintf("finalizer.%s.io/cleanup", name),
		RetryPolicy:           &RetryPolicy{MaxRetries: 5, InitialBackoff: 1 * time.Second, MaxBackoff: 1 * time.Minute, BackoffMultiplier: 2.0},
		StatusCollector:       &StatusCollector{statuses: make(map[string]map[string]interface{})},
		ConditionManager:      &ConditionManager{conditions: make(map[string][]*Condition)},
		ObserverManager:       &ObserverManager{observers: make(map[string][]ResourceObserver)},
		activeReconciliations: make(map[string]*ReconciliationState),
		maxConcurrent:         10,
	}
}

// Reconcile is the main reconciliation loop
func (cr *ControllerReconciler) Reconcile(ctx context.Context, key string) (time.Duration, error) {
	startTime := time.Now()

	// Initialize reconciliation state
	state := &ReconciliationState{
		Key:        key,
		StartTime:  startTime,
		Status:     StatusProcessing,
		Conditions: make([]*Condition, 0),
		Events:     make([]*events.ResourceEvent, 0),
		MaxRetries: cr.RetryPolicy.MaxRetries,
	}

	// Track active reconciliation
	cr.mu.Lock()
	cr.activeReconciliations[key] = state
	cr.mu.Unlock()

	defer func() {
		cr.mu.Lock()
		delete(cr.activeReconciliations, key)
		cr.mu.Unlock()
	}()

	// Phase 1: Initialize - fetch resource
	if err := cr.phaseInitialize(ctx, state); err != nil {
		return cr.handleError(ctx, state, err)
	}

	// Phase 2: Admit - check admission policies
	if err := cr.phaseAdmit(ctx, state); err != nil {
		return cr.handleError(ctx, state, err)
	}

	// Phase 3: Observe - gather current state
	if err := cr.phaseObserve(ctx, state); err != nil {
		return cr.handleError(ctx, state, err)
	}

	// Phase 4: Reconcile - perform reconciliation
	if err := cr.phaseReconcile(ctx, state); err != nil {
		if errors.Is(err, ErrRequeue) {
			state.Status = StatusRequeue
			return 30 * time.Second, nil
		}
		return cr.handleError(ctx, state, err)
	}

	// Phase 5: Finalize - handle deletion
	if err := cr.phaseFinalize(ctx, state); err != nil {
		return cr.handleError(ctx, state, err)
	}

	// Phase 6: Complete - success
	state.Phase = PhaseComplete
	state.Status = StatusSuccess
	state.ProcessingTime = time.Since(startTime)

	cr.publishReconciliationEvent(ctx, state, true)
	return 0, nil
}

// phaseInitialize fetches and initializes the resource
func (cr *ControllerReconciler) phaseInitialize(ctx context.Context, state *ReconciliationState) error {
	state.Phase = PhaseInitialize

	// Parse key (namespace/name)
	// For cluster-scoped resources, key is just name
	res, err := cr.ResourceManager.Get(ctx, state.Key)
	if err != nil {
		return fmt.Errorf("failed to get resource: %w", err)
	}

	if res == nil {
		// Resource was deleted, skip reconciliation
		return nil
	}

	state.Resource = res
	state.ObservedGeneration = res.GetObjectMeta().Generation

	// Emit resource found event
	cr.emitEvent(ctx, state, events.EventResourceReconciled, "Reconciliation started", "")

	return nil
}

// phaseAdmit checks if resource passes admission
func (cr *ControllerReconciler) phaseAdmit(ctx context.Context, state *ReconciliationState) error {
	state.Phase = PhaseAdmit

	if cr.AdmissionController == nil {
		return nil
	}

	// Build admission request
	req := &AdmissionRequest{
		ID:        fmt.Sprintf("req-%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Kind:      cr.ResourceKind,
		Operation: "update",
		Resource:  cr.resourceToMap(state.Resource),
	}

	// Run admission
	resp, err := cr.AdmissionController.Admit(ctx, req)
	if err != nil {
		return fmt.Errorf("admission error: %w", err)
	}

	if !resp.Allowed {
		cr.emitEvent(ctx, state, events.EventAdmissionRejected, resp.Reason, "warning")
		return fmt.Errorf("admission denied: %s", resp.Reason)
	}

	if len(resp.Warnings) > 0 {
		cr.emitEvent(ctx, state, events.EventResourceWarning, fmt.Sprintf("%d warnings: %v", len(resp.Warnings), resp.Warnings), "warning")
	}

	return nil
}

// phaseObserve gathers current state via observers
func (cr *ControllerReconciler) phaseObserve(ctx context.Context, state *ReconciliationState) error {
	state.Phase = PhaseObserve

	cr.ObserverManager.mu.RLock()
	observers := cr.ObserverManager.observers[cr.ResourceKind]
	cr.ObserverManager.mu.RUnlock()

	for _, observer := range observers {
		obs, err := observer.Observe(ctx, state.Resource)
		if err != nil {
			cr.Logger.Error("Observer error", zap.String("observer", observer.Name()), zap.Error(err))
			continue
		}

		// Record observation
		cr.StatusCollector.mu.Lock()
		if cr.StatusCollector.statuses[state.Key] == nil {
			cr.StatusCollector.statuses[state.Key] = make(map[string]interface{})
		}
		cr.StatusCollector.statuses[state.Key][observer.Name()] = obs
		cr.StatusCollector.mu.Unlock()
	}

	return nil
}

// phaseReconcile performs the actual reconciliation work
func (cr *ControllerReconciler) phaseReconcile(ctx context.Context, state *ReconciliationState) error {
	state.Phase = PhaseReconcile

	// This is where controllers implement domain-specific logic
	// Call user-provided reconciliation function
	// Example: ensure deployment replicas match desired state

	// Add condition tracking
	cr.addCondition(state, &Condition{
		Type:               "ReconcileStarted",
		Status:             "True",
		LastUpdateTime:     time.Now(),
		LastTransitionTime: time.Now(),
		Reason:             "ReconciliationInProgress",
		Message:            "Reconciliation started",
	})

	// Perform reconciliation (stub - should be implemented per controller)
	if err := cr.performReconciliationLogic(ctx, state); err != nil {
		cr.addCondition(state, &Condition{
			Type:           "ReconcileFailed",
			Status:         "True",
			LastUpdateTime: time.Now(),
			Reason:         "ReconcileError",
			Message:        err.Error(),
		})
		return err
	}

	cr.addCondition(state, &Condition{
		Type:           "ReconcileSucceeded",
		Status:         "True",
		LastUpdateTime: time.Now(),
		Reason:         "ReconcileComplete",
		Message:        "Reconciliation completed",
	})

	return nil
}

// phaseFinalize handles deletion via finalizers
func (cr *ControllerReconciler) phaseFinalize(ctx context.Context, state *ReconciliationState) error {
	state.Phase = PhaseFinalize

	objMeta := state.Resource.GetObjectMeta()

	// Check if resource is being deleted
	if objMeta.DeletedAt != nil {
		// Run finalizers
		if !cr.containsFinalizer(state, cr.FinalizerName) {
			return nil // Already cleaned up
		}

		// Perform cleanup
		if err := cr.cleanup(ctx, state); err != nil {
			cr.emitEvent(ctx, state, events.EventResourceError, fmt.Sprintf("Cleanup failed: %v", err), "error")
			return fmt.Errorf("cleanup failed: %w", err)
		}

		// Remove finalizer
		cr.removeFinalizer(state, cr.FinalizerName)
		if err := cr.ResourceManager.Update(ctx, state.Resource); err != nil {
			return fmt.Errorf("failed to update resource after finalizer removal: %w", err)
		}

		cr.emitEvent(ctx, state, events.EventResourceDeleted, "Resource deleted successfully", "")
		return nil
	}

	// Add finalizer if not present
	if !cr.containsFinalizer(state, cr.FinalizerName) {
		cr.addFinalizer(state, cr.FinalizerName)
		if err := cr.ResourceManager.Update(ctx, state.Resource); err != nil {
			return fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	return nil
}

// performReconciliationLogic is the core reconciliation - override per controller
func (cr *ControllerReconciler) performReconciliationLogic(ctx context.Context, state *ReconciliationState) error {
	// Stub - implement in controller subclass or via callback
	return nil
}

// cleanup performs cleanup during finalization
func (cr *ControllerReconciler) cleanup(ctx context.Context, state *ReconciliationState) error {
	// Stub - implement in controller subclass
	return nil
}

// Helper functions

func (cr *ControllerReconciler) addCondition(state *ReconciliationState, cond *Condition) {
	cr.ConditionManager.mu.Lock()
	defer cr.ConditionManager.mu.Unlock()

	state.Conditions = append(state.Conditions, cond)
	cr.ConditionManager.conditions[state.Key] = append(cr.ConditionManager.conditions[state.Key], cond)
}

func (cr *ControllerReconciler) addFinalizer(state *ReconciliationState, finalizer string) {
	for _, f := range state.Finalizers {
		if f == finalizer {
			return
		}
	}
	state.Finalizers = append(state.Finalizers, finalizer)
}

func (cr *ControllerReconciler) removeFinalizer(state *ReconciliationState, finalizer string) {
	filtered := make([]string, 0)
	for _, f := range state.Finalizers {
		if f != finalizer {
			filtered = append(filtered, f)
		}
	}
	state.Finalizers = filtered
}

func (cr *ControllerReconciler) containsFinalizer(state *ReconciliationState, finalizer string) bool {
	for _, f := range state.Finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

func (cr *ControllerReconciler) emitEvent(ctx context.Context, state *ReconciliationState, eventType events.ResourceEventType, message string, severity string) {
	if severity == "" {
		severity = "info"
	}

	event := &events.ResourceEvent{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Type:      eventType,
		Kind:      cr.ResourceKind,
		Timestamp: time.Now(),
		Reason:    string(state.Phase),
		Message:   message,
		Severity:  severity,
		Source:    cr.Name,
	}

	if state.Resource != nil {
		objMeta := state.Resource.GetObjectMeta()
		event.Name = objMeta.Name
		event.Namespace = objMeta.Namespace
		event.Generation = objMeta.Generation
	}

	if cr.EventBus != nil {
		if err := cr.EventBus.PublishResourceEvent(ctx, event); err != nil {
			cr.Logger.Error("Failed to emit event", zap.Error(err))
		}
	}

	state.Events = append(state.Events, event)
}

func (cr *ControllerReconciler) publishReconciliationEvent(ctx context.Context, state *ReconciliationState, success bool) {
	eventType := events.EventResourceReconciled
	severity := "info"

	if !success {
		eventType = events.EventResourceError
		severity = "error"
	}

	cr.emitEvent(ctx, state, eventType, fmt.Sprintf("Reconciliation %s in %v", map[bool]string{true: "completed", false: "failed"}[success], state.ProcessingTime), severity)
}

func (cr *ControllerReconciler) handleError(ctx context.Context, state *ReconciliationState, err error) (time.Duration, error) {
	state.Error = err
	state.RetryCount++

	if state.RetryCount >= state.MaxRetries {
		state.Status = StatusFailed
		cr.emitEvent(ctx, state, events.EventResourceError, fmt.Sprintf("Max retries exceeded: %v", err), "error")
		return 0, err
	}

	// Calculate backoff
	backoff := time.Duration(cr.RetryPolicy.InitialBackoff.Nanoseconds()) * time.Duration(int64(cr.RetryPolicy.BackoffMultiplier*float64(state.RetryCount)))
	if backoff > cr.RetryPolicy.MaxBackoff {
		backoff = cr.RetryPolicy.MaxBackoff
	}

	state.Status = StatusRequeue
	cr.emitEvent(ctx, state, events.EventResourceWarning, fmt.Sprintf("Requeuing after error (attempt %d/%d): %v", state.RetryCount, state.MaxRetries, err), "warning")

	return backoff, nil
}

func (cr *ControllerReconciler) resourceToMap(res resources.Resource) map[string]interface{} {
	// Convert resource to map for admission
	return map[string]interface{}{
		"kind": cr.ResourceKind,
		"metadata": map[string]interface{}{
			"name":      res.GetObjectMeta().Name,
			"namespace": res.GetObjectMeta().Namespace,
		},
	}
}

// GetActiveReconciliations returns currently active reconciliations
func (cr *ControllerReconciler) GetActiveReconciliations() []*ReconciliationState {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	result := make([]*ReconciliationState, 0, len(cr.activeReconciliations))
	for _, state := range cr.activeReconciliations {
		result = append(result, state)
	}

	return result
}

// RegisterObserver registers an observer for this resource kind
func (cr *ControllerReconciler) RegisterObserver(observer ResourceObserver) {
	cr.ObserverManager.mu.Lock()
	defer cr.ObserverManager.mu.Unlock()

	cr.ObserverManager.observers[cr.ResourceKind] = append(
		cr.ObserverManager.observers[cr.ResourceKind],
		observer,
	)
}

// ErrRequeue signals that reconciliation should be requeued
var ErrRequeue = errors.New("requeue reconciliation")

// ResourceManager defines interface for resource CRUD
type ResourceManager struct {
	mu        sync.RWMutex
	resources map[string]resources.Resource
}

// Get retrieves a resource
func (rm *ResourceManager) Get(ctx context.Context, key string) (resources.Resource, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if res, exists := rm.resources[key]; exists {
		return res, nil
	}

	return nil, nil
}

// Update updates a resource
func (rm *ResourceManager) Update(ctx context.Context, res resources.Resource) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	key := fmt.Sprintf("%s/%s", res.GetObjectMeta().Namespace, res.GetObjectMeta().Name)
	rm.resources[key] = res

	return nil
}
