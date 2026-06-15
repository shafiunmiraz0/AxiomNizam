package streamanalytics

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/resilience"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"

	"go.uber.org/zap"
)

// StreamJobRunner abstracts the streaming job execution engine.
type StreamJobRunner interface {
	Start(ctx context.Context, job *StreamJobResource) error
	Stop(ctx context.Context, jobName string) error
	Status(ctx context.Context, jobName string) (*JobRunStatus, error)
}

type JobRunStatus struct {
	Running         bool   `json:"running"`
	EventsProcessed int64  `json:"eventsProcessed"`
	CurrentLag      int64  `json:"currentLag"`
	AvgProcessingMs float64 `json:"avgProcessingMs"`
	LastError       string `json:"lastError,omitempty"`
}

type StreamJobReconciler struct {
	store  store.ResourceStore[*StreamJobResource]
	runner StreamJobRunner
}

func NewStreamJobReconciler(s store.ResourceStore[*StreamJobResource], runner StreamJobRunner) *StreamJobReconciler {
	return &StreamJobReconciler{store: s, runner: runner}
}

func (r *StreamJobReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	job, ok := obj.(*StreamJobResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("streamanalytics: received non-StreamJobResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", job.GetKey()), zap.String("kind", job.GetTypeMeta().Kind))

	now := time.Now()
	status := job.Status

	// Handle disabled jobs — stop if running.
	if !job.Spec.Enabled {
		if status.JobStatus == "running" && r.runner != nil {
			_ = r.runner.Stop(ctx, job.Name)
		}
		status.Phase = "Disabled"
		status.JobStatus = "stopped"
		status.ObservedGeneration = job.Generation
		status.LastTransitionTime = now
		job.Status = status
		storeutil.Update(ctx, r.store, job) //nolint:errcheck
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 1 * time.Minute}
	}

	// Start job if not running.
	if status.JobStatus != "running" {
		if r.runner != nil {
			if err := r.runner.Start(ctx, job); err != nil {
				status.Phase = "Error"
				status.JobStatus = "failed"
				status.LastErrorMessage = err.Error()
				status.Conditions = upsertCondition(status.Conditions, resources.Condition{
					Type: "Ready", Status: "False",
					Reason: "StartFailed", Message: err.Error(),
					LastTransitionTime: now,
				})
				status.ObservedGeneration = job.Generation
				status.LastTransitionTime = now
				job.Status = status
				storeutil.Update(ctx, r.store, job) //nolint:errcheck
				logging.Z().Warn("reconciliation error", zap.String("name", job.GetKey()), zap.Error(err))
				return reconciler.ReconcileResult{Requeue: true, RequeueAfter: resilience.ReconcileBackoff(1)}
			}
		}
		status.JobStatus = "running"
		status.StartedAt = &now
	}

	// Poll job status.
	if r.runner != nil {
		runStatus, err := r.runner.Status(ctx, job.Name)
		if err == nil && runStatus != nil {
			status.EventsProcessed = runStatus.EventsProcessed
			status.CurrentLag = runStatus.CurrentLag
			status.AvgProcessingMs = runStatus.AvgProcessingMs
			if runStatus.LastError != "" {
				status.LastErrorMessage = runStatus.LastError
			}
			if !runStatus.Running {
				status.JobStatus = "stopped"
				status.Phase = "Stopped"
			}
		}
	}

	if status.JobStatus == "running" {
		status.Phase = "Running"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "True",
			Reason: "JobRunning",
			Message: fmt.Sprintf("processed %d events, lag: %d", status.EventsProcessed, status.CurrentLag),
			LastTransitionTime: now,
		})
	}

	status.LastCheckpointAt = &now
	status.ObservedGeneration = job.Generation
	status.LastTransitionTime = now
	job.Status = status
	storeutil.Update(ctx, r.store, job) //nolint:errcheck

	return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 30 * time.Second}
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
