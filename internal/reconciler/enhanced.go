package reconciler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/events"
)

// ========== ENHANCED RESOURCE INTERFACE ==========

// EnhancedResource extends Resource with additional fields needed for production reconciliation
type EnhancedResource interface {
	Resource

	// GetDeletionTimestamp returns when resource was marked for deletion (nil if not deleting)
	GetDeletionTimestamp() *time.Time

	// GetFinalizers returns list of finalizer strings that must be cleared before deletion
	GetFinalizers() []string

	// SetFinalizers sets the finalizers list
	SetFinalizers([]string)

	// AddFinalizer adds a finalizer
	AddFinalizer(finalizer string)

	// RemoveFinalizer removes a finalizer
	RemoveFinalizer(finalizer string)

	// GetResourceVersion returns optimistic concurrency control version
	GetResourceVersion() string

	// SetResourceVersion sets the resource version
	SetResourceVersion(version string)

	// GetLabels returns resource labels
	GetLabels() map[string]string

	// GetAnnotations returns resource annotations
	GetAnnotations() map[string]string
}

// ========== ENHANCED RECONCILE RESULT ==========

// ReconcileError represents a typed error from reconciliation
type ReconcileError struct {
	Phase   string // "observe", "validate", "diff", "act", "status"
	Err     error
	Retry   bool
	Retries int
}

func (e *ReconcileError) Error() string {
	return fmt.Sprintf("reconcile failed in %s phase: %v", e.Phase, e.Err)
}

// EnhancedReconcileResult extends ReconcileResult with additional fields
type EnhancedReconcileResult struct {
	ReconcileResult
	RateLimited  bool
	BackoffDelay time.Duration
	Duration     time.Duration
	ChangesCount int
	PhaseErrors  map[string]error // Map of phase -> error
	ReconcileID  string           // Trace ID
}

// ========== VALIDATION INTERFACE ==========

// Validator validates resource before reconciliation
type Validator interface {
	// Validate checks if resource is valid
	// Returns nil if valid, []error if validation failed
	Validate(ctx context.Context, resource Resource) []error
}

// ========== RATE LIMITER INTERFACE ==========

// RateLimiter implements token bucket or exponential backoff rate limiting
type RateLimiter interface {
	// Next returns the duration to wait before next reconciliation
	// Returns 0 if not rate limited
	Next(key string) time.Duration

	// Forget clears rate limit state for key
	Forget(key string)

	// Reset resets the rate limiter
	Reset()
}

// ExponentialBackoffLimiter implements exponential backoff
type ExponentialBackoffLimiter struct {
	mu       sync.RWMutex
	attempts map[string]int
	maxWait  time.Duration
	base     time.Duration
}

// NewExponentialBackoffLimiter creates limiter
func NewExponentialBackoffLimiter(base, max time.Duration) *ExponentialBackoffLimiter {
	return &ExponentialBackoffLimiter{
		attempts: make(map[string]int),
		base:     base,
		maxWait:  max,
	}
}

// Next calculates next backoff duration
func (ebl *ExponentialBackoffLimiter) Next(key string) time.Duration {
	ebl.mu.Lock()
	defer ebl.mu.Unlock()

	attempts := ebl.attempts[key]
	if attempts == 0 {
		return 0 // Not rate limited
	}

	// 2^n * base, capped at maxWait
	wait := ebl.base
	for i := 1; i < attempts && wait < ebl.maxWait; i++ {
		wait *= 2
	}

	if wait > ebl.maxWait {
		wait = ebl.maxWait
	}

	return wait
}

// Record records an attempt
func (ebl *ExponentialBackoffLimiter) Record(key string, success bool) {
	ebl.mu.Lock()
	defer ebl.mu.Unlock()

	if success {
		delete(ebl.attempts, key)
	} else {
		ebl.attempts[key]++
	}
}

// Forget clears rate limit
func (ebl *ExponentialBackoffLimiter) Forget(key string) {
	ebl.mu.Lock()
	defer ebl.mu.Unlock()
	delete(ebl.attempts, key)
}

// Reset resets all
func (ebl *ExponentialBackoffLimiter) Reset() {
	ebl.mu.Lock()
	defer ebl.mu.Unlock()
	ebl.attempts = make(map[string]int)
}

// ========== CONCURRENCY CONTROLLER INTERFACE ==========

// ConcurrencyController limits concurrent reconciliation of same resource
type ConcurrencyController interface {
	// Acquire tries to acquire lock for key
	// Returns true if acquired, false if already locked
	Acquire(key string) bool

	// Release releases lock for key
	Release(key string)

	// IsLocked checks if key is locked
	IsLocked(key string) bool
}

// SimpleConcurrencyController uses mutexes for concurrency control
type SimpleConcurrencyController struct {
	mu    sync.RWMutex
	locks map[string]*sync.Mutex
}

// NewSimpleConcurrencyController creates controller
func NewSimpleConcurrencyController() *SimpleConcurrencyController {
	return &SimpleConcurrencyController{
		locks: make(map[string]*sync.Mutex),
	}
}

// Acquire acquires lock
func (scc *SimpleConcurrencyController) Acquire(key string) bool {
	scc.mu.Lock()
	if scc.locks[key] == nil {
		scc.locks[key] = &sync.Mutex{}
	}
	mu := scc.locks[key]
	scc.mu.Unlock()

	// Try non-blocking lock
	return mu.TryLock()
}

// Release releases lock
func (scc *SimpleConcurrencyController) Release(key string) {
	scc.mu.RLock()
	mu := scc.locks[key]
	scc.mu.RUnlock()

	if mu != nil {
		mu.Unlock()
	}
}

// IsLocked checks if locked
func (scc *SimpleConcurrencyController) IsLocked(key string) bool {
	scc.mu.RLock()
	defer scc.mu.RUnlock()

	mu := scc.locks[key]
	if mu == nil {
		return false
	}

	locked := !mu.TryLock()
	if !locked {
		mu.Unlock()
	}
	return locked
}

// ========== EVENT RECORDER INTERFACE ==========

// ReconciliationEventRecorder records reconciliation events
type ReconciliationEventRecorder interface {
	// RecordReconcileStart records start of reconciliation
	RecordReconcileStart(resource Resource)

	// RecordReconcileSuccess records successful reconciliation
	RecordReconcileSuccess(resource Resource, changes int)

	// RecordReconcileError records reconciliation error
	RecordReconcileError(resource Resource, phase string, err error)

	// RecordChangesApplied records when changes are applied
	RecordChangesApplied(resource Resource, changes []string)

	// RecordValidationError records validation error
	RecordValidationError(resource Resource, errors []error)
}

// StandardEventRecorder records events
type StandardEventRecorder struct {
	recorder events.EventRecorder
}

// NewStandardEventRecorder creates recorder
func NewStandardEventRecorder(recorder events.EventRecorder) *StandardEventRecorder {
	return &StandardEventRecorder{
		recorder: recorder,
	}
}

// RecordReconcileStart records start
func (ser *StandardEventRecorder) RecordReconcileStart(resource Resource) {
	if ser.recorder != nil {
		// Would call ser.recorder.RecordEvent(resource, ...) if available
	}
}

// RecordReconcileSuccess records success
func (ser *StandardEventRecorder) RecordReconcileSuccess(resource Resource, changes int) {
	if ser.recorder != nil {
		_ = fmt.Sprintf("Reconciliation succeeded, %d changes applied", changes)
		// Would call ser.recorder.RecordEvent(resource, events.EventTypeReconcileSuccess, msg)
	}
}

// RecordReconcileError records error
func (ser *StandardEventRecorder) RecordReconcileError(resource Resource, phase string, err error) {
	if ser.recorder != nil {
		_ = fmt.Sprintf("Reconciliation failed in %s phase: %v", phase, err)
		// Would call ser.recorder.RecordEvent(resource, events.EventTypeReconcileFailed, msg)
	}
}

// RecordChangesApplied records changes
func (ser *StandardEventRecorder) RecordChangesApplied(resource Resource, changes []string) {
	if ser.recorder != nil {
		_ = fmt.Sprintf("Applied changes: %v", changes)
		// Would call ser.recorder.RecordEvent(resource, "ChangesApplied", msg)
	}
}

// RecordValidationError records validation error
func (ser *StandardEventRecorder) RecordValidationError(resource Resource, errs []error) {
	if ser.recorder != nil {
		_ = fmt.Sprintf("Validation errors: %v", errs)
		// Would call ser.recorder.RecordEvent(resource, "ValidationError", msg)
	}
}

// ========== FINALIZER SUPPORT ==========

// FinalizerHandler handles resource deletion
type FinalizerHandler interface {
	// Finalize performs cleanup before deletion
	Finalize(ctx context.Context, resource Resource) error
}

// ========== METRICS RECORDER ==========

// ReconciliationMetrics records reconciliation metrics
type ReconciliationMetrics interface {
	// RecordDuration records reconciliation duration
	RecordDuration(duration time.Duration, success bool, phase string)

	// RecordError records error
	RecordError(errorType string)

	// RecordChangesApplied records changes count
	RecordChangesApplied(count int)

	// RecordRetry records retry attempt
	RecordRetry(key string)
}

// ========== RECONCILIATION CONTEXT ==========

// ReconciliationContext captures full context of a reconciliation
type ReconciliationContext struct {
	StartTime        time.Time
	DesiredResource  Resource
	ActualState      map[string]interface{}
	Changes          []string
	Error            error
	Phase            string
	ID               string // Trace ID
	Duration         time.Duration
	ValidationErrors []error
	Retries          int
	LastError        *ReconcileError
}

// ========== ENHANCED STANDARD RECONCILER ==========

// EnhancedStandardReconciler extends StandardReconciler with production features
type EnhancedStandardReconciler struct {
	// Core components
	Observer      Observer
	Differ        Differ
	Actor         Actor
	StatusUpdater StatusUpdater

	// Production features
	Validator             Validator
	RateLimiter           RateLimiter
	ConcurrencyController ConcurrencyController
	EventRecorder         ReconciliationEventRecorder
	FinalizerHandler      FinalizerHandler
	MetricsRecorder       ReconciliationMetrics
	Logger                Logger // Structured logging interface

	// State tracking
	reconciliationCtx map[string]*ReconciliationContext
	ctxMu             sync.RWMutex
	retryAttempts     map[string]int
	retryMu           sync.RWMutex
}

// Logger interface for structured logging
type Logger interface {
	Debug(msg string, keysAndValues ...interface{})
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}

// NewEnhancedStandardReconciler creates enhanced reconciler
func NewEnhancedStandardReconciler(
	observer Observer,
	differ Differ,
	actor Actor,
	updater StatusUpdater,
) *EnhancedStandardReconciler {
	return &EnhancedStandardReconciler{
		Observer:          observer,
		Differ:            differ,
		Actor:             actor,
		StatusUpdater:     updater,
		reconciliationCtx: make(map[string]*ReconciliationContext),
		retryAttempts:     make(map[string]int),
	}
}

// SetValidator sets validator
func (r *EnhancedStandardReconciler) SetValidator(v Validator) *EnhancedStandardReconciler {
	r.Validator = v
	return r
}

// SetRateLimiter sets rate limiter
func (r *EnhancedStandardReconciler) SetRateLimiter(rl RateLimiter) *EnhancedStandardReconciler {
	r.RateLimiter = rl
	return r
}

// SetConcurrencyController sets concurrency controller
func (r *EnhancedStandardReconciler) SetConcurrencyController(cc ConcurrencyController) *EnhancedStandardReconciler {
	r.ConcurrencyController = cc
	return r
}

// SetEventRecorder sets event recorder
func (r *EnhancedStandardReconciler) SetEventRecorder(er ReconciliationEventRecorder) *EnhancedStandardReconciler {
	r.EventRecorder = er
	return r
}

// SetFinalizerHandler sets finalizer handler
func (r *EnhancedStandardReconciler) SetFinalizerHandler(fh FinalizerHandler) *EnhancedStandardReconciler {
	r.FinalizerHandler = fh
	return r
}

// SetMetricsRecorder sets metrics recorder
func (r *EnhancedStandardReconciler) SetMetricsRecorder(mr ReconciliationMetrics) *EnhancedStandardReconciler {
	r.MetricsRecorder = mr
	return r
}

// SetLogger sets logger
func (r *EnhancedStandardReconciler) SetLogger(logger Logger) *EnhancedStandardReconciler {
	r.Logger = logger
	return r
}

// Reconcile implements enhanced reconciliation
func (r *EnhancedStandardReconciler) Reconcile(ctx context.Context, obj Resource) ReconcileResult {
	key := obj.GetKey()
	startTime := time.Now()
	reconcileID := fmt.Sprintf("%s-%d", key, startTime.UnixNano())

	// Create reconciliation context
	recCtx := &ReconciliationContext{
		StartTime: startTime,
		ID:        reconcileID,
	}
	r.ctxMu.Lock()
	r.reconciliationCtx[key] = recCtx
	r.ctxMu.Unlock()

	defer func() {
		duration := time.Since(startTime)
		recCtx.Duration = duration
		if r.MetricsRecorder != nil {
			r.MetricsRecorder.RecordDuration(duration, recCtx.Error == nil, recCtx.Phase)
		}
	}()

	// Phase 0: Check rate limiting
	if r.RateLimiter != nil {
		backoff := r.RateLimiter.Next(key)
		if backoff > 0 {
			if r.Logger != nil {
				r.Logger.Warn("Rate limited", "key", key, "backoff", backoff)
			}
			return ReconcileResult{
				Requeue:      true,
				RequeueAfter: backoff,
				Error:        fmt.Errorf("rate limited: %v", backoff),
			}
		}
	}

	// Phase 1: Check concurrency control
	if r.ConcurrencyController != nil {
		if !r.ConcurrencyController.Acquire(key) {
			if r.Logger != nil {
				r.Logger.Debug("Waiting for concurrent reconciliation", "key", key)
			}
			return ReconcileResult{
				Requeue:      true,
				RequeueAfter: 100 * time.Millisecond,
			}
		}
		defer r.ConcurrencyController.Release(key)
	}

	// Phase 2: Handle finalization
	if enhanced, ok := obj.(EnhancedResource); ok {
		if enhanced.GetDeletionTimestamp() != nil {
			if r.FinalizerHandler != nil {
				recCtx.Phase = "finalize"
				if err := r.FinalizerHandler.Finalize(ctx, obj); err != nil {
					if r.EventRecorder != nil {
						r.EventRecorder.RecordReconcileError(obj, "finalize", err)
					}
					if r.Logger != nil {
						r.Logger.Error("Finalization failed", "key", key, "error", err)
					}
					return ReconcileResult{
						Requeue:      true,
						RequeueAfter: 5 * time.Second,
						Error:        err,
					}
				}
			}
			return ReconcileResult{Requeue: false}
		}
	}

	// Phase 3: Validate
	if r.Validator != nil {
		recCtx.Phase = "validate"
		validationErrors := r.Validator.Validate(ctx, obj)
		if len(validationErrors) > 0 {
			recCtx.ValidationErrors = validationErrors
			if r.EventRecorder != nil {
				r.EventRecorder.RecordValidationError(obj, validationErrors)
			}
			if r.Logger != nil {
				r.Logger.Error("Validation failed", "key", key, "errors", validationErrors)
			}
			if r.MetricsRecorder != nil {
				r.MetricsRecorder.RecordError("validation_error")
			}
			// Don't requeue invalid resources
			return ReconcileResult{
				Requeue: false,
				Error:   fmt.Errorf("validation failed: %v", validationErrors),
			}
		}
	}

	// Phase 4: OBSERVE
	if r.EventRecorder != nil {
		r.EventRecorder.RecordReconcileStart(obj)
	}

	recCtx.Phase = "observe"
	desired, err := r.Observer.ObserveDesiredState(ctx, key)
	if err != nil {
		recCtx.Error = err
		if r.EventRecorder != nil {
			r.EventRecorder.RecordReconcileError(obj, "observe-desired", err)
		}
		if r.Logger != nil {
			r.Logger.Error("Failed to observe desired state", "key", key, "error", err)
		}
		if r.MetricsRecorder != nil {
			r.MetricsRecorder.RecordError("observe_desired_error")
		}
		r.recordRetry(key)
		return ReconcileResult{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
			Error:        &ReconcileError{Phase: "observe-desired", Err: err, Retry: true},
		}
	}
	recCtx.DesiredResource = desired

	actual, err := r.Observer.ObserveActualState(ctx, key)
	if err != nil {
		recCtx.Error = err
		if r.EventRecorder != nil {
			r.EventRecorder.RecordReconcileError(obj, "observe-actual", err)
		}
		if r.Logger != nil {
			r.Logger.Error("Failed to observe actual state", "key", key, "error", err)
		}
		if r.MetricsRecorder != nil {
			r.MetricsRecorder.RecordError("observe_actual_error")
		}
		r.recordRetry(key)
		return ReconcileResult{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
			Error:        &ReconcileError{Phase: "observe-actual", Err: err, Retry: true},
		}
	}
	recCtx.ActualState = actual

	// Phase 5: DIFF
	recCtx.Phase = "diff"
	changes := r.Differ.Diff(ctx, desired, actual)
	recCtx.Changes = changes

	if r.Logger != nil && len(changes) > 0 {
		r.Logger.Info("Changes detected", "key", key, "changes", len(changes))
	}

	// Phase 6: ACT
	if len(changes) > 0 {
		recCtx.Phase = "act"
		err := r.Actor.Act(ctx, desired, changes)
		if err != nil {
			recCtx.Error = err
			if r.EventRecorder != nil {
				r.EventRecorder.RecordReconcileError(obj, "act", err)
			}
			if r.Logger != nil {
				r.Logger.Error("Failed to apply changes", "key", key, "error", err)
			}
			if r.MetricsRecorder != nil {
				r.MetricsRecorder.RecordError("act_error")
			}
			r.recordRetry(key)
			return ReconcileResult{
				Requeue:      true,
				RequeueAfter: 10 * time.Second,
				Error:        &ReconcileError{Phase: "act", Err: err, Retry: true},
			}
		}

		if r.EventRecorder != nil {
			r.EventRecorder.RecordChangesApplied(desired, changes)
		}
		if r.MetricsRecorder != nil {
			r.MetricsRecorder.RecordChangesApplied(len(changes))
		}
		recCtx.Changes = changes
	}

	// Phase 7: UPDATE STATUS
	recCtx.Phase = "status"
	err = r.StatusUpdater.UpdateStatus(ctx, desired)
	if err != nil {
		recCtx.Error = err
		if r.EventRecorder != nil {
			r.EventRecorder.RecordReconcileError(obj, "status", err)
		}
		if r.Logger != nil {
			r.Logger.Error("Failed to update status", "key", key, "error", err)
		}
		if r.MetricsRecorder != nil {
			r.MetricsRecorder.RecordError("status_update_error")
		}
		r.recordRetry(key)
		return ReconcileResult{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
			Error:        &ReconcileError{Phase: "status", Err: err, Retry: true},
		}
	}

	// Success
	recCtx.Phase = "succeeded"
	if r.EventRecorder != nil {
		r.EventRecorder.RecordReconcileSuccess(desired, len(changes))
	}
	if r.RateLimiter != nil {
		r.RateLimiter.Forget(key)
	}
	r.clearRetry(key)

	if r.Logger != nil {
		r.Logger.Info("Reconciliation succeeded", "key", key, "changes", len(changes), "duration", time.Since(startTime))
	}

	return ReconcileResult{
		Requeue: false,
		Error:   nil,
	}
}

// Helper functions
func (r *EnhancedStandardReconciler) recordRetry(key string) {
	r.retryMu.Lock()
	defer r.retryMu.Unlock()
	r.retryAttempts[key]++
	if r.MetricsRecorder != nil {
		r.MetricsRecorder.RecordRetry(key)
	}
}

func (r *EnhancedStandardReconciler) clearRetry(key string) {
	r.retryMu.Lock()
	defer r.retryMu.Unlock()
	delete(r.retryAttempts, key)
}

// GetReconciliationContext returns context of last reconciliation
func (r *EnhancedStandardReconciler) GetReconciliationContext(key string) *ReconciliationContext {
	r.ctxMu.RLock()
	defer r.ctxMu.RUnlock()
	return r.reconciliationCtx[key]
}

// ========== RESULT HELPERS ==========

// SuccessResult returns successful reconciliation result
func SuccessResult() ReconcileResult {
	return ReconcileResult{Requeue: false}
}

// RequeueResult returns requeue result
func RequeueResult(after time.Duration) ReconcileResult {
	return ReconcileResult{
		Requeue:      true,
		RequeueAfter: after,
	}
}

// ErrorResult returns error result with requeue
func ErrorResult(err error, requeueAfter time.Duration) ReconcileResult {
	return ReconcileResult{
		Requeue:      true,
		RequeueAfter: requeueAfter,
		Error:        err,
	}
}

// PhaseErrorResult returns error result for specific phase
func PhaseErrorResult(phase string, err error, retry bool, requeueAfter time.Duration) ReconcileResult {
	return ReconcileResult{
		Requeue:      retry,
		RequeueAfter: requeueAfter,
		Error: &ReconcileError{
			Phase: phase,
			Err:   err,
			Retry: retry,
		},
	}
}
