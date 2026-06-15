package audit

// Reconciler for AuditPolicyResource.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// AuditPolicyReconciler reconciles AuditPolicyResource objects.
type AuditPolicyReconciler struct {
	store store.ResourceStore[*AuditPolicyResource]
}

// NewAuditPolicyReconciler builds a reconciler.
func NewAuditPolicyReconciler(rs store.ResourceStore[*AuditPolicyResource]) *AuditPolicyReconciler {
	return &AuditPolicyReconciler{store: rs}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *AuditPolicyReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*AuditPolicyResource)
	if !ok {
		return reconciler.ReconcileResult{Error: auditErr("audit: reconciler received wrong type")}
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
	status.Conditions = upsertAuditCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: auditBoolStatus(res.Spec.Enabled),
		Reason: phase, Message: "audit policy reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

func upsertAuditCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}

func auditBoolStatus(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

type auditErr string

func (e auditErr) Error() string { return string(e) }
