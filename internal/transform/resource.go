package transform

// Phase 6 P2 — Transformation resource-ification.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/transform/models"
)

// --- Constants (aliases) ---

const (
	RuleKind       = models.RuleKind
	RuleAPIVersion = models.RuleAPIVersion
)

// --- Type aliases ---

type RuleSpec = models.RuleSpec
type RuleResourceStatus = models.RuleResourceStatus
type RuleResource = models.RuleResource

// --- Reconciler (business logic stays in parent package) ---

type RuleReconciler struct {
	store store.ResourceStore[*RuleResource]
}

func NewRuleReconciler(rs store.ResourceStore[*RuleResource]) *RuleReconciler {
	return &RuleReconciler{store: rs}
}

func (rec *RuleReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*RuleResource)
	if !ok {
		return reconciler.ReconcileResult{Error: ruleErr("transform: wrong type")}
	}
	now := time.Now()
	phase := "Disabled"
	if res.Spec.Enabled {
		phase = "Active"
	}
	res.Status.Phase = phase
	res.Status.RuleActive = res.Spec.Enabled
	res.Status.ObservedGeneration = res.Generation
	res.Status.LastTransitionTime = now
	if rec.store != nil {
		_ = rec.store.Update(ctx, res)
	}
	return reconciler.ReconcileResult{}
}

type ruleErr string

func (e ruleErr) Error() string { return string(e) }
