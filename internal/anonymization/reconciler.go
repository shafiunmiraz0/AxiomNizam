package anonymization

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/resilience"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"

	"go.uber.org/zap"
)

// AnonymizationExecutor abstracts the masking execution engine.
type AnonymizationExecutor interface {
	Execute(ctx context.Context, policy *AnonymizationPolicyResource) (*ExecutionResult, error)
}

type ExecutionResult struct {
	AssetsProcessed   int           `json:"assetsProcessed"`
	ColumnsAnonymized int           `json:"columnsAnonymized"`
	RowsProcessed     int64         `json:"rowsProcessed"`
	Duration          time.Duration `json:"duration"`
	Errors            []string      `json:"errors,omitempty"`
}

type AnonymizationReconciler struct {
	store    store.ResourceStore[*AnonymizationPolicyResource]
	executor AnonymizationExecutor
}

func NewAnonymizationReconciler(s store.ResourceStore[*AnonymizationPolicyResource], exec AnonymizationExecutor) *AnonymizationReconciler {
	return &AnonymizationReconciler{store: s, executor: exec}
}

func (r *AnonymizationReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	policy, ok := obj.(*AnonymizationPolicyResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("anonymization: received non-AnonymizationPolicyResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", policy.GetKey()), zap.String("kind", policy.GetTypeMeta().Kind))

	now := time.Now()
	status := policy.Status

	if !policy.Spec.Enabled {
		status.Phase = "Disabled"
		status.ObservedGeneration = policy.Generation
		status.LastTransitionTime = now
		policy.Status = status
		storeutil.Update(ctx, r.store, policy) //nolint:errcheck
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 1 * time.Hour}
	}

	// Check if run is due.
	interval := parseDuration(policy.Spec.Schedule)
	if interval == 0 {
		interval = 24 * time.Hour
	}
	if status.LastRunAt != nil && now.Sub(*status.LastRunAt) < interval && status.ObservedGeneration >= policy.Generation {
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: interval - now.Sub(*status.LastRunAt)}
	}

	// Execute anonymization.
	if r.executor != nil {
		result, err := r.executor.Execute(ctx, policy)
		if err != nil {
			status.Phase = "Error"
			status.Errors = append(status.Errors, err.Error())
			status.Conditions = upsertCondition(status.Conditions, resources.Condition{
				Type: "Ready", Status: "False",
				Reason: "ExecutionFailed", Message: err.Error(),
				LastTransitionTime: now,
			})
			status.ObservedGeneration = policy.Generation
			status.LastTransitionTime = now
			policy.Status = status
			storeutil.Update(ctx, r.store, policy) //nolint:errcheck
			logging.Z().Warn("reconciliation error", zap.String("name", policy.GetKey()), zap.Error(err))
			return reconciler.ReconcileResult{Requeue: true, RequeueAfter: resilience.ReconcileBackoff(1)}
		}

		status.AssetsProcessed = result.AssetsProcessed
		status.ColumnsAnonymized = result.ColumnsAnonymized
		status.RowsProcessed = result.RowsProcessed
		status.LastRunDuration = result.Duration.Truncate(time.Millisecond).String()
		status.Errors = result.Errors
	}

	status.LastRunAt = &now
	nextRun := now.Add(interval)
	status.NextRunAt = &nextRun
	status.Phase = "Completed"
	status.Conditions = upsertCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: "True",
		Reason: "ExecutionComplete",
		Message: fmt.Sprintf("anonymized %d columns across %d assets (%d rows)", status.ColumnsAnonymized, status.AssetsProcessed, status.RowsProcessed),
		LastTransitionTime: now,
	})

	status.ObservedGeneration = policy.Generation
	status.LastTransitionTime = now
	policy.Status = status
	storeutil.Update(ctx, r.store, policy) //nolint:errcheck

	return reconciler.ReconcileResult{Requeue: true, RequeueAfter: interval}
}

func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

func upsertCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}
