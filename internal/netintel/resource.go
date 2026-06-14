package netintel

// Domain types moved to models/resource.go.
// Type aliases in types.go re-export them for backward compatibility.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
)

type ConfigReconciler struct {
	store store.ResourceStore[*ConfigResource]
}

func NewConfigReconciler(rs store.ResourceStore[*ConfigResource]) *ConfigReconciler {
	return &ConfigReconciler{store: rs}
}

func (rec *ConfigReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*ConfigResource)
	if !ok {
		return reconciler.ReconcileResult{Error: configErr("netintel: wrong type")}
	}
	now := time.Now()
	phase := "Disabled"
	if res.Spec.Enabled {
		phase = "Active"
	}
	res.Status.Phase = phase
	res.Status.ConfigActive = res.Spec.Enabled
	res.Status.ObservedGeneration = res.Generation
	res.Status.LastTransitionTime = now
	if rec.store != nil {
		_ = rec.store.Update(ctx, res)
	}
	return reconciler.ReconcileResult{}
}

type configErr string

func (e configErr) Error() string { return string(e) }
