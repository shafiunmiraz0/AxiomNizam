package export

// Reconciler for ExportJobResource.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// ExportJobReconciler reconciles ExportJobResource objects.
type ExportJobReconciler struct {
	store   store.ResourceStore[*ExportJobResource]
	manager ExportManager
}

// NewExportJobReconciler builds a reconciler.
func NewExportJobReconciler(rs store.ResourceStore[*ExportJobResource], mgr ExportManager) *ExportJobReconciler {
	return &ExportJobReconciler{store: rs, manager: mgr}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *ExportJobReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*ExportJobResource)
	if !ok {
		return reconciler.ReconcileResult{Error: exportErr("export: reconciler received non-ExportJobResource")}
	}

	now := time.Now()
	status := res.Status

	// Handle cancellation.
	if res.Spec.Cancel && status.ExportStatus != ExportCancelled && status.ExportStatus != ExportCompleted {
		if r.manager != nil {
			_ = r.manager.CancelExport(res.Name)
		}
		status.ExportStatus = ExportCancelled
		status.Phase = string(ExportCancelled)
		status.CompletedAt = &now
		status.Conditions = upsertExportCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason: "Cancelled", Message: "export cancelled by user",
			LastTransitionTime: now,
		})
		status.ObservedGeneration = res.Generation
		status.LastTransitionTime = now
		res.Status = status
		storeutil.Update(ctx, r.store, res) //nolint:errcheck
		return reconciler.ReconcileResult{}
	}

	// Drive lifecycle.
	switch status.ExportStatus {
	case "", ExportPending:
		if r.manager != nil {
			job := &ExportJob{
				ID:          res.Name,
				Name:        res.Name,
				Description: res.Spec.Description,
				Format:      res.Spec.Format,
				Source:      res.Spec.Source,
				Query:       res.Spec.Query,
				Filters:     res.Spec.Filters,
				Columns:     res.Spec.Columns,
				Compression: res.Spec.Compression,
				Encryption:  res.Spec.Encryption,
				Destination: res.Spec.Destination,
				Schedule:    res.Spec.Schedule,
			}
			_, err := r.manager.SubmitExport(job)
			if err != nil {
				status.ExportStatus = ExportFailed
				status.Phase = string(ExportFailed)
				status.FailureReason = err.Error()
			} else {
				status.ExportStatus = ExportRunning
				status.Phase = string(ExportRunning)
				startedAt := now
				status.StartedAt = &startedAt
			}
		}

	case ExportRunning, ExportProcessing, ExportQueued:
		if r.manager != nil {
			job, err := r.manager.GetExport(res.Name)
			if err == nil && job != nil {
				status.Progress = job.Progress
				status.RecordCount = job.RecordCount
				status.FileSize = job.FileSize
				status.ProcessedRows = job.ProcessedRows
				status.SkippedRows = job.SkippedRows
				status.ErrorRows = job.ErrorRows

				if job.Status == ExportCompleted || job.Status == ExportFailed {
					status.ExportStatus = job.Status
					status.Phase = string(job.Status)
					status.FailureReason = job.FailureReason
					completedAt := now
					status.CompletedAt = &completedAt
				} else {
					status.ExportStatus = job.Status
					status.Phase = string(job.Status)
				}
			}
		}

		// Requeue while still running.
		if status.ExportStatus == ExportRunning || status.ExportStatus == ExportProcessing || status.ExportStatus == ExportQueued {
			status.ObservedGeneration = res.Generation
			status.LastTransitionTime = now
			res.Status = status
			storeutil.Update(ctx, r.store, res) //nolint:errcheck
			return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 2 * time.Second}
		}
	}

	readyStatus := "True"
	if status.ExportStatus == ExportFailed {
		readyStatus = "False"
	}
	status.Conditions = upsertExportCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: readyStatus,
		Reason: string(status.ExportStatus), Message: "export reconciled",
		LastTransitionTime: now,
	})
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

func upsertExportCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}

type exportErr string

func (e exportErr) Error() string { return string(e) }
