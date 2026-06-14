package users

import (
	"context"
	"errors"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
)

// UserReconciler brings observed state of a UserResource in line with
// its declared spec.  For now it only keeps Status.Phase consistent and
// stamps ObservedGeneration so the workqueue stops requeueing; external
// identity-provider sync hangs off the `act` step.
type UserReconciler struct {
	store store.ResourceStore[*UserResource]
}

// NewUserReconciler constructs a reconciler backed by the given store.
func NewUserReconciler(s store.ResourceStore[*UserResource]) *UserReconciler {
	return &UserReconciler{store: s}
}

// Reconcile implements reconciler.Reconciler.
func (r *UserReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	u, ok := obj.(*UserResource)
	if !ok {
		return reconciler.ReconcileResult{Error: errors.New("users: reconcile received wrong resource type")}
	}

	phase := "Active"
	if u.Spec.Suspended {
		phase = "Suspended"
	}
	if u.DeletedAt != nil {
		phase = "Deleting"
	}

	u.Status.Phase = phase
	u.Status.ObservedGeneration = u.Generation
	u.Status.LastTransitionTime = time.Now()

	if r.store != nil {
		if err := r.store.Update(ctx, u); err != nil {
			return reconciler.ReconcileResult{Error: err}
		}
	}
	return reconciler.ReconcileResult{}
}
