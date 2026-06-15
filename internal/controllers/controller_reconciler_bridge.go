// Package controllers — canonical reconciler bridge.
//
// Part of P0.1 (unify Reconciler interface). This file provides a thin adapter
// so that the legacy ControllerReconciler (which takes a string key and
// returns (time.Duration, error)) can be used anywhere the canonical
// reconciler.Reconciler is expected.
package controllers

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
)

// keyedShim adapts ControllerReconciler.Reconcile(ctx, key) to the
// reconciler.KeyedReconciler interface.
type keyedShim struct {
	cr *ControllerReconciler
}

func (k keyedShim) Reconcile(ctx context.Context, key string) (time.Duration, error) {
	return k.cr.Reconcile(ctx, key)
}

// AsReconciler returns the canonical reconciler.Reconciler wrapper around
// this legacy ControllerReconciler.
//
// The wrapper extracts the key from obj.GetKey() and forwards the call. Any
// non-zero requeueAfter becomes ReconcileResult.RequeueAfter; errors are
// propagated unchanged.
func (cr *ControllerReconciler) AsReconciler() reconciler.Reconciler {
	return reconciler.FromKeyed(keyedShim{cr: cr})
}
