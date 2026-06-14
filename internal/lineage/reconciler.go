package lineage

// Reconciler for LineageNodeResource.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// LineageNodeReconciler reconciles LineageNodeResource objects.
type LineageNodeReconciler struct {
	store   store.ResourceStore[*LineageNodeResource]
	manager LineageManager
}

// NewLineageNodeReconciler builds a reconciler.
func NewLineageNodeReconciler(rs store.ResourceStore[*LineageNodeResource], mgr LineageManager) *LineageNodeReconciler {
	return &LineageNodeReconciler{store: rs, manager: mgr}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *LineageNodeReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*LineageNodeResource)
	if !ok {
		return reconciler.ReconcileResult{Error: lineageErr("lineage: reconciler received wrong type")}
	}

	now := time.Now()
	status := res.Status

	phase := "Inactive"
	if res.Spec.Active {
		phase = "Active"
	}

	status.NodeActive = res.Spec.Active
	status.Phase = phase
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertLineageCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: lineageBoolStatus(res.Spec.Active),
		Reason: phase, Message: "lineage node reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

func upsertLineageCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}

func lineageBoolStatus(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

type lineageErr string

func (e lineageErr) Error() string { return string(e) }
