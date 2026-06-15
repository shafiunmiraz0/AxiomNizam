package tracing

// Reconciler for TracingConfigResource.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// TracingConfigReconciler reconciles TracingConfigResource objects.
type TracingConfigReconciler struct {
	store store.ResourceStore[*TracingConfigResource]
}

// NewTracingConfigReconciler builds a reconciler.
func NewTracingConfigReconciler(rs store.ResourceStore[*TracingConfigResource]) *TracingConfigReconciler {
	return &TracingConfigReconciler{store: rs}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *TracingConfigReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*TracingConfigResource)
	if !ok {
		return reconciler.ReconcileResult{Error: tracingErr("tracing: reconciler received wrong type")}
	}

	now := time.Now()
	status := res.Status

	phase := "Disabled"
	if res.Spec.Enabled {
		phase = "Active"
	}

	status.ConfigActive = res.Spec.Enabled
	status.Phase = phase
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertTracingCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: tracingBoolStatus(res.Spec.Enabled),
		Reason: phase, Message: "tracing config reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

func upsertTracingCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}

func tracingBoolStatus(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

type tracingErr string

func (e tracingErr) Error() string { return string(e) }
