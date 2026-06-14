package versioning

// Reconciler for VersionPolicyResource.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// VersionPolicyReconciler reconciles VersionPolicyResource objects.
type VersionPolicyReconciler struct {
	store store.ResourceStore[*VersionPolicyResource]
}

// NewVersionPolicyReconciler builds a reconciler.
func NewVersionPolicyReconciler(rs store.ResourceStore[*VersionPolicyResource]) *VersionPolicyReconciler {
	return &VersionPolicyReconciler{store: rs}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *VersionPolicyReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*VersionPolicyResource)
	if !ok {
		return reconciler.ReconcileResult{Error: versionErr("versioning: reconciler received wrong type")}
	}

	now := time.Now()
	status := res.Status

	phase := "Disabled"
	if res.Spec.Enabled {
		phase = "Active"
	}

	status.PolicyActive = res.Spec.Enabled
	status.Phase = phase
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertVersionCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: versionBoolStatus(res.Spec.Enabled),
		Reason: phase, Message: "version policy reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

func upsertVersionCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}

func versionBoolStatus(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

type versionErr string

func (e versionErr) Error() string { return string(e) }
