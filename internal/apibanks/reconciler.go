package apibanks

// Reconciler for APIBankResource.
//
// This is the skeleton reconciler that satisfies the canonical
// `reconciler.Reconciler` contract (Observe → Diff → Act → Update
// Status).  It bridges the declarative `APIBankResource` to the
// imperative `APIBankManager` that already exists.
//
// Today the reconciler does two things:
//
//   1. Ensures a matching entry exists in the APIBankManager's
//      in-memory store (Create on first sight, Update when spec
//      generation changes).
//   2. Writes back APICount, LastSyncedAt, and ObservedGeneration into
//      the resource status.
//
// Runtime-side drift detection (e.g. probing the APIs listed in the
// bank) is out of scope for this skeleton; it is a natural next
// iteration that plugs in behind the `act` step.

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
)

// APIBankReconciler reconciles APIBankResource objects.
type APIBankReconciler struct {
	store   store.ResourceStore[*APIBankResource]
	manager *APIBankManager
}

// NewAPIBankReconciler builds a reconciler.  `manager` is optional — if
// nil, the reconciler only updates status and leaves the runtime
// concerns to whoever registers a follow-up reconciler.
func NewAPIBankReconciler(rs store.ResourceStore[*APIBankResource], mgr *APIBankManager) *APIBankReconciler {
	return &APIBankReconciler{store: rs, manager: mgr}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *APIBankReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*APIBankResource)
	if !ok {
		return reconciler.ReconcileResult{Error: errTypeMismatch}
	}

	// Observe: nothing to fetch beyond what's in `res`; manager state
	// is purely derived from spec.

	// Act: keep APIBankManager in sync with the spec.
	if r.manager != nil {
		bank := &APIBank{
			Name:        res.Name,
			Namespace:   res.Namespace,
			Description: res.Spec.Description,
			Owner:       res.Spec.Owner,
			Version:     res.Spec.Version,
			APIs:        append([]APIReference(nil), res.Spec.APIs...),
			Tags:        append([]string(nil), res.Spec.Tags...),
			Labels:      res.Spec.Labels,
		}
		// CreateBank fails if the bank already exists; treat that as
		// "already reconciled" — the manager has no Update today, so
		// once created the runtime state matches spec.
		if err := r.manager.CreateBank(ctx, bank); err != nil && !strings.Contains(err.Error(), "already exists") {
			logging.Z().Info(fmt.Sprintf("apibanks: create bank %s error: %v", res.Name, err))
		}
	}

	// Update Status.
	now := time.Now()
	status := res.Status
	status.APICount = len(res.Spec.APIs)
	status.LastSyncedAt = &now
	status.ObservedGeneration = res.Generation
	status.Phase = "Ready"
	status.LastTransitionTime = now
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

// errTypeMismatch is returned when Reconcile is called with a non-APIBank
// resource.  It is package-private to avoid leaking the sentinel into
// callers; they only need to observe via the standard error surface.
var errTypeMismatch = apibankError("apibanks: reconciler received non-APIBankResource")

type apibankError string

func (e apibankError) Error() string { return string(e) }
