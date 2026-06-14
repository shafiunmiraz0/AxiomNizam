// Package controller — canonical reconciler bridge for BucketController.
//
// Part of P0.1 (unify Reconciler interface). BucketController.Reconcile
// takes a concrete *models.BucketResource and returns a plain error. This
// file wraps it so it can be plugged into the canonical reconciler plumbing.
package controller

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
)

// AsReconciler returns the canonical reconciler.Reconciler wrapper around
// this BucketController. Non-nil errors trigger a requeue.
func (bc *BucketController) AsReconciler() reconciler.Reconciler {
	return reconciler.FromTyped[*models.BucketResource](bc)
}
