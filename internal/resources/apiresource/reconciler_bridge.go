// Package apiresource — canonical reconciler bridge.
//
// Part of P0.1 (unify Reconciler interface). APIResourceReconciler already
// returns reconciler.ReconcileResult, but it still takes a string key rather
// than a typed reconciler.Resource. This file wraps it so it can be plugged
// directly into any place that expects the canonical reconciler.Reconciler.
package apiresource

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
)

// AsReconciler returns the canonical reconciler.Reconciler wrapper around
// this APIResourceReconciler.
func (r *APIResourceReconciler) AsReconciler() reconciler.Reconciler {
	return reconciler.ReconcilerFunc(func(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
		return r.Reconcile(ctx, obj.GetKey())
	})
}
