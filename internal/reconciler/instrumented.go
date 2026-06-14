package reconciler

// InstrumentedReconciler wraps any Reconciler with structured logging
// and metrics recording. This is Phase 0.3 of the migration plan.
//
// Every Reconcile() call is logged with:
//   module, key, phase, duration, result, error
//
// Every call is recorded in ReconcilerMetrics for the /health/reconcilers
// endpoint and Prometheus-style counters.

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"time"
)

// MetricsRecorder is the interface the instrumented reconciler uses to
// record per-call metrics. Matches metrics.ReconcilerMetrics.RecordReconcile.
type MetricsRecorder interface {
	RecordReconcile(module string, duration time.Duration, success bool, requeued bool, errMsg string)
}

// InstrumentedReconciler wraps a Reconciler with logging and metrics.
type InstrumentedReconciler struct {
	// Module is the human-readable name (e.g. "bulk", "rbac-role").
	Module string

	// Inner is the actual reconciler that does the work.
	Inner Reconciler

	// Metrics records per-call stats. Nil-safe — skipped when nil.
	Metrics MetricsRecorder
}

// NewInstrumented wraps inner with structured logging and metrics.
func NewInstrumented(module string, inner Reconciler, metrics MetricsRecorder) *InstrumentedReconciler {
	return &InstrumentedReconciler{
		Module:  module,
		Inner:   inner,
		Metrics: metrics,
	}
}

// Reconcile delegates to Inner.Reconcile and logs the result.
func (ir *InstrumentedReconciler) Reconcile(ctx context.Context, obj Resource) ReconcileResult {
	key := obj.GetKey()
	gen := obj.GetGeneration()
	observedGen := obj.GetObservedGeneration()

	start := time.Now()
	result := ir.Inner.Reconcile(ctx, obj)
	duration := time.Since(start)

	success := result.Error == nil
	requeued := result.Requeue

	// Structured log line.
	errMsg := ""
	if result.Error != nil {
		errMsg = result.Error.Error()
	}

	if success {
		logging.Z().Info(fmt.Sprintf("reconcile module=%s key=%s gen=%d observed=%d duration=%s result=success requeue=%v",
			ir.Module, key, gen, observedGen, duration.Round(time.Millisecond), requeued))
	} else {
		logging.Z().Info(fmt.Sprintf("reconcile module=%s key=%s gen=%d observed=%d duration=%s result=error requeue=%v err=%q",
			ir.Module, key, gen, observedGen, duration.Round(time.Millisecond), requeued, errMsg))
	}

	// Record metrics.
	if ir.Metrics != nil {
		ir.Metrics.RecordReconcile(ir.Module, duration, success, requeued, errMsg)
	}

	return result
}

// String returns a human-readable description.
func (ir *InstrumentedReconciler) String() string {
	return fmt.Sprintf("InstrumentedReconciler[%s]", ir.Module)
}
