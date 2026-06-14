package bulk

// Reconciler for BulkOperationResource.
//
// Drives bulk operation lifecycle through the existing BulkManager.
// The handler writes a BulkOperationResource; the reconciler observes
// the spec and drives the manager, recording progress on the status.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// BulkOperationReconciler reconciles BulkOperationResource objects.
type BulkOperationReconciler struct {
	store   store.ResourceStore[*BulkOperationResource]
	manager BulkManager
}

// NewBulkOperationReconciler builds a reconciler.
func NewBulkOperationReconciler(rs store.ResourceStore[*BulkOperationResource], mgr BulkManager) *BulkOperationReconciler {
	return &BulkOperationReconciler{store: rs, manager: mgr}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *BulkOperationReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*BulkOperationResource)
	if !ok {
		return reconciler.ReconcileResult{Error: bulkErr("bulk: reconciler received non-BulkOperationResource")}
	}

	now := time.Now()
	status := res.Status

	// Handle cancellation request.
	if res.Spec.Cancel && status.OperationStatus != BulkOpCancelled && status.OperationStatus != BulkOpCompleted {
		if r.manager != nil {
			_ = r.manager.CancelOperation(res.Name)
		}
		status.OperationStatus = BulkOpCancelled
		status.Phase = string(BulkOpCancelled)
		status.CompletedAt = &now
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason: "Cancelled", Message: "operation cancelled by user",
			LastTransitionTime: now,
		})
		status.ObservedGeneration = res.Generation
		status.LastTransitionTime = now
		res.Status = status
		storeutil.Update(ctx, r.store, res) //nolint:errcheck
		return reconciler.ReconcileResult{}
	}

	// Handle retry-failed request.
	if res.Spec.RetryFailed && (status.OperationStatus == BulkOpFailed || status.OperationStatus == BulkOpPartial) {
		if r.manager != nil {
			_, _ = r.manager.RetryFailed(res.Name)
		}
		status.OperationStatus = BulkOpRunning
		status.Phase = string(BulkOpRunning)
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason: "Retrying", Message: "retrying failed items",
			LastTransitionTime: now,
		})
	}

	// Drive lifecycle: Pending → Running → Completed/Failed.
	switch status.OperationStatus {
	case "", BulkOpPending:
		// Submit to manager.
		if r.manager != nil {
			op := &BulkOperation{
				ID:              res.Name,
				TenantID:        res.Spec.TenantID,
				Type:            res.Spec.Type,
				Items:           res.Spec.Items,
				Options:         res.Spec.Options,
				Timeout:         res.Spec.Timeout,
				Atomic:          res.Spec.Atomic,
				RollbackOnError: res.Spec.RollbackOnError,
			}
			submitted, err := r.manager.SubmitOperation(op)
			if err != nil {
				status.OperationStatus = BulkOpFailed
				status.Phase = string(BulkOpFailed)
				status.Conditions = upsertCondition(status.Conditions, resources.Condition{
					Type: "Ready", Status: "False",
					Reason: "SubmitFailed", Message: err.Error(),
					LastTransitionTime: now,
				})
			} else {
				status.OperationStatus = BulkOpRunning
				status.Phase = string(BulkOpRunning)
				status.TotalItems = submitted.TotalItems
				startedAt := now
				status.StartedAt = &startedAt
				status.Conditions = upsertCondition(status.Conditions, resources.Condition{
					Type: "Ready", Status: "False",
					Reason: "Running", Message: "operation submitted to manager",
					LastTransitionTime: now,
				})
			}
		}

	case BulkOpRunning:
		// Poll manager for progress.
		if r.manager != nil {
			op, err := r.manager.GetOperation(res.Name)
			if err == nil && op != nil {
				status.TotalItems = op.TotalItems
				status.SuccessCount = op.SuccessCount
				status.FailureCount = op.FailureCount
				status.SkippedCount = op.SkippedCount
				if op.TotalItems > 0 {
					status.Progress = int(float64(op.SuccessCount+op.FailureCount+op.SkippedCount) / float64(op.TotalItems) * 100)
				}
				status.ErrorSummary = op.ErrorSummary

				if op.Status == BulkOpCompleted || op.Status == BulkOpFailed || op.Status == BulkOpPartial {
					status.OperationStatus = op.Status
					status.Phase = string(op.Status)
					status.CompletedAt = op.CompletedAt
					readyStatus := "True"
					if op.Status == BulkOpFailed {
						readyStatus = "False"
					}
					status.Conditions = upsertCondition(status.Conditions, resources.Condition{
						Type: "Ready", Status: readyStatus,
						Reason: string(op.Status), Message: "operation completed",
						LastTransitionTime: now,
					})
				}
			}
		}

		// Requeue while still running to poll progress.
		if status.OperationStatus == BulkOpRunning {
			status.ObservedGeneration = res.Generation
			status.LastTransitionTime = now
			res.Status = status
			storeutil.Update(ctx, r.store, res) //nolint:errcheck
			return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 2 * time.Second}
		}
	}

	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

func upsertCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}

type bulkErr string

func (e bulkErr) Error() string { return string(e) }
