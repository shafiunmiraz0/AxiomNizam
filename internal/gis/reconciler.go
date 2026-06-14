package gis

// Phase 6 P2 — GIS reconciler.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// GISReconciler reconciles GISResource objects.
type GISReconciler struct {
	store store.ResourceStore[*GISResource]
}

// NewGISReconciler builds a reconciler.
func NewGISReconciler(rs store.ResourceStore[*GISResource]) *GISReconciler {
	return &GISReconciler{store: rs}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *GISReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*GISResource)
	if !ok {
		return reconciler.ReconcileResult{Error: gisReconcilerErr("gis: reconciler received wrong type")}
	}

	now := time.Now()
	status := res.Status

	phase := "Active"
	if res.Spec.GISKind == "" {
		phase = "Invalid"
	}

	status.Phase = phase
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertGISCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: gisBoolStatus(phase == "Active"),
		Reason: phase, Message: res.Spec.GISKind + " resource reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	if r.store != nil {
		_ = r.store.Update(ctx, res)
	}
	return reconciler.ReconcileResult{}
}

func upsertGISCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}

func gisBoolStatus(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

type gisReconcilerErr string

func (e gisReconcilerErr) Error() string { return string(e) }
