package apiresource

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	apiv1 "axiomnizam.bitbd.net/axiomnizam/internal/resources/apiresource/v1"
)

// APIResourceReconciler implements the Reconciler interface for APIResource
type APIResourceReconciler struct {
	storeBackend StoreBackend
	runtime      Runtime
}

// StoreBackend defines the persistence interface
type StoreBackend interface {
	Get(ctx context.Context, namespace, name string) (*apiv1.APIResource, error)
	Update(ctx context.Context, resource *apiv1.APIResource) error
	UpdateStatus(ctx context.Context, namespace, name string, status apiv1.APIResourceStatus) error
}

// Runtime defines the actual runtime state interface
type Runtime interface {
	Exists(ctx context.Context, namespace, name string) (bool, error)
	Create(ctx context.Context, namespace, name string, spec apiv1.APIResourceSpec) error
	Delete(ctx context.Context, namespace, name string) error
	GetStatus(ctx context.Context, namespace, name string) (map[string]interface{}, error)
}

// New creates a new APIResourceReconciler
func NewReconciler(store StoreBackend, runtime Runtime) *APIResourceReconciler {
	return &APIResourceReconciler{
		storeBackend: store,
		runtime:      runtime,
	}
}

// Reconcile implements the standard Observe → Diff → Act → Update Status pattern
//
// The reconciliation process follows:
// 1. OBSERVE: Fetch desired state (from storage) and actual state (from runtime)
// 2. DIFF: Compare desired vs actual to find what changed
// 3. ACT: Take action to move toward desired state (idempotent)
// 4. UPDATE STATUS: Record the result in storage with proper phase and conditions
func (r *APIResourceReconciler) Reconcile(ctx context.Context, key string) reconciler.ReconcileResult {
	// Extract namespace and name from key (format: namespace/name)
	namespace, name, err := parseKey(key)
	if err != nil {
		return reconciler.ReconcileResult{
			Requeue:      false,
			RequeueAfter: 0,
			Error:        fmt.Errorf("invalid key format: %v", err),
		}
	}

	// PHASE 1: OBSERVE
	// Fetch desired state from storage
	desired, err := r.storeBackend.Get(ctx, namespace, name)
	if err != nil {
		return reconciler.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
			Error:        fmt.Errorf("failed to observe desired state: %v", err),
		}
	}

	// Fetch actual state from runtime
	runtimeExists, err := r.runtime.Exists(ctx, namespace, name)
	if err != nil {
		desired.MarkNotReady("RuntimeError", fmt.Sprintf("Failed to query runtime: %v", err))
		r.storeBackend.UpdateStatus(ctx, namespace, name, desired.Status)
		return reconciler.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 10 * time.Second,
			Error:        fmt.Errorf("failed to observe actual state: %v", err),
		}
	}

	// Increment reconciliation counter
	desired.IncrementReconcileCount()

	// PHASE 2: DIFF
	// Identify differences between desired and actual state
	var actions []string

	switch desired.GetPhase() {
	case apiv1.PhasePending:
		// Resource hasn't been validated yet
		actions = append(actions, "validate-spec")
		desired.MarkValidated("SpecValid", "API specification is valid")

	case apiv1.PhaseValidated:
		// Resource has been validated but not yet applied
		if !runtimeExists {
			actions = append(actions, "create-api")
		} else {
			actions = append(actions, "update-api")
		}

	case apiv1.PhaseApplied:
		// Resource should be operational - verify status
		if !runtimeExists {
			// API was deleted externally, mark for recreation
			actions = append(actions, "recreate-api")
		} else {
			// Verify runtime state matches desired
			runtimeStatus, err := r.runtime.GetStatus(ctx, namespace, name)
			if err != nil {
				desired.MarkNotReady("StatusCheckFailed", fmt.Sprintf("Cannot verify runtime status: %v", err))
				r.storeBackend.UpdateStatus(ctx, namespace, name, desired.Status)
				return reconciler.ReconcileResult{
					Requeue:      true,
					RequeueAfter: 10 * time.Second,
					Error:        err,
				}
			}

			// Check if runtime state is healthy
			if healthy := r.isHealthy(runtimeStatus); !healthy {
				desired.MarkNotReady("RuntimeUnhealthy", "API runtime is not responding")
				r.storeBackend.UpdateStatus(ctx, namespace, name, desired.Status)
				return reconciler.ReconcileResult{
					Requeue:      true,
					RequeueAfter: 15 * time.Second,
					Error:        fmt.Errorf("runtime is unhealthy"),
				}
			}

			// Mark as synced (no actions needed)
			desired.MarkSynced("InSync", "API is synchronized with desired state")
		}

	case apiv1.PhaseFailed:
		// Resource is in failed state - no automatic recovery
		return reconciler.ReconcileResult{
			Requeue: false,
			Error:   fmt.Errorf("resource in failed state: %s", desired.Status.Message),
		}

	case apiv1.PhaseTerminated:
		// Resource is being deleted
		if runtimeExists {
			actions = append(actions, "delete-api")
		}
		return reconciler.ReconcileResult{
			Requeue: false,
			Error:   nil,
		}
	}

	// PHASE 3: ACT
	// Execute identified actions (idempotent)
	if len(actions) > 0 {
		desired.MarkReconciling("Reconciling", fmt.Sprintf("Taking actions: %v", actions))

		for _, action := range actions {
			switch action {
			case "validate-spec":
				// Validation happens here
				if err := r.validateSpec(desired.Spec); err != nil {
					desired.MarkFailed("ValidationFailed", err.Error())
					r.storeBackend.Update(ctx, desired)
					return reconciler.ReconcileResult{
						Requeue:      false,
						RequeueAfter: 0,
						Error:        err,
					}
				}
				// Move to validated phase
				desired.SetPhase(apiv1.PhaseValidated)

			case "create-api":
				// Create the API in runtime (idempotent - check first)
				if !runtimeExists {
					err := r.runtime.Create(ctx, namespace, name, desired.Spec)
					if err != nil {
						desired.MarkFailed("CreationFailed", fmt.Sprintf("Failed to create API: %v", err))
						r.storeBackend.Update(ctx, desired)
						return reconciler.ReconcileResult{
							Requeue:      true,
							RequeueAfter: 10 * time.Second,
							Error:        err,
						}
					}
					runtimeExists = true
				}

			case "update-api":
				// Update is treated like create in this implementation
				err := r.runtime.Create(ctx, namespace, name, desired.Spec)
				if err != nil {
					desired.MarkFailed("UpdateFailed", fmt.Sprintf("Failed to update API: %v", err))
					r.storeBackend.Update(ctx, desired)
					return reconciler.ReconcileResult{
						Requeue:      true,
						RequeueAfter: 10 * time.Second,
						Error:        err,
					}
				}

			case "recreate-api":
				// API was deleted, recreate it
				err := r.runtime.Create(ctx, namespace, name, desired.Spec)
				if err != nil {
					desired.MarkFailed("RecreationFailed", fmt.Sprintf("Failed to recreate API: %v", err))
					r.storeBackend.Update(ctx, desired)
					return reconciler.ReconcileResult{
						Requeue:      true,
						RequeueAfter: 10 * time.Second,
						Error:        err,
					}
				}

			case "delete-api":
				// Delete the API from runtime
				err := r.runtime.Delete(ctx, namespace, name)
				if err != nil {
					desired.MarkFailed("DeletionFailed", fmt.Sprintf("Failed to delete API: %v", err))
					r.storeBackend.Update(ctx, desired)
					return reconciler.ReconcileResult{
						Requeue:      true,
						RequeueAfter: 10 * time.Second,
						Error:        err,
					}
				}
			}
		}
	}

	// PHASE 4: UPDATE STATUS
	// If we got here, reconciliation was successful
	if !desired.IsReady() && desired.GetPhase() == apiv1.PhaseValidated {
		// Transition to Applied and mark ready
		desired.MarkReady("Applied", "API resource is applied and ready")
	}

	// Update status in storage
	err = r.storeBackend.Update(ctx, desired)
	if err != nil {
		return reconciler.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
			Error:        fmt.Errorf("failed to update status: %v", err),
		}
	}

	// Success - no requeue needed
	return reconciler.ReconcileResult{
		Requeue: false,
		Error:   nil,
	}
}

// validateSpec performs basic validation on the spec
func (r *APIResourceReconciler) validateSpec(spec apiv1.APIResourceSpec) error {
	if spec.BasePath == "" {
		return fmt.Errorf("basePath is required")
	}
	if spec.Title == "" {
		return fmt.Errorf("title is required")
	}
	if spec.Version == "" {
		return fmt.Errorf("version is required")
	}
	return nil
}

// isHealthy checks if the runtime state indicates a healthy resource
func (r *APIResourceReconciler) isHealthy(status map[string]interface{}) bool {
	// Simple check: if status exists and is not empty, consider it healthy
	return len(status) > 0
}

// parseKey splits a key into namespace and name
func parseKey(key string) (string, string, error) {
	// Key format: namespace/name
	var namespace, name string
	_, err := fmt.Sscanf(key, "%[^/]/%s", &namespace, &name)
	if err != nil {
		return "", "", fmt.Errorf("invalid key format (expected namespace/name): %s", key)
	}
	return namespace, name, nil
}
