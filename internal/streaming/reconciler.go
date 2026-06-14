package streaming

// Reconciler for StreamResource.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// StreamReconciler reconciles StreamResource objects.
type StreamReconciler struct {
	store store.ResourceStore[*StreamResource]
}

// NewStreamReconciler builds a reconciler.
func NewStreamReconciler(rs store.ResourceStore[*StreamResource]) *StreamReconciler {
	return &StreamReconciler{store: rs}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *StreamReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*StreamResource)
	if !ok {
		return reconciler.ReconcileResult{Error: streamErr("streaming: reconciler received non-StreamResource")}
	}

	now := time.Now()
	status := res.Status

	phase := "Inactive"
	if res.Spec.Active {
		phase = "Active"
	}

	status.StreamActive = res.Spec.Active
	status.Phase = phase
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertStreamCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: streamBoolStatus(res.Spec.Active),
		Reason: phase, Message: "stream reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

func upsertStreamCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}

func streamBoolStatus(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

type streamErr string

func (e streamErr) Error() string { return string(e) }
