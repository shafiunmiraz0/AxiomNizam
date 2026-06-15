package mlpipeline

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"

	"go.uber.org/zap"
)

// StepExecutor abstracts executing individual ML pipeline steps.
type StepExecutor interface {
	ExecuteStep(ctx context.Context, pipeline *MLPipelineResource, step MLStep) (*StepStatus, error)
}

type MLPipelineReconciler struct {
	store    store.ResourceStore[*MLPipelineResource]
	executor StepExecutor
}

func NewMLPipelineReconciler(s store.ResourceStore[*MLPipelineResource], exec StepExecutor) *MLPipelineReconciler {
	return &MLPipelineReconciler{store: s, executor: exec}
}

func (r *MLPipelineReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	pipeline, ok := obj.(*MLPipelineResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("mlpipeline: received non-MLPipelineResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", pipeline.GetKey()), zap.String("kind", pipeline.GetTypeMeta().Kind))

	now := time.Now()
	status := pipeline.Status

	if !pipeline.Spec.Enabled {
		status.Phase = "Disabled"
		status.PipelineStatus = "disabled"
		status.ObservedGeneration = pipeline.Generation
		status.LastTransitionTime = now
		pipeline.Status = status
		storeutil.Update(ctx, r.store, pipeline) //nolint:errcheck
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 1 * time.Hour}
	}

	// Check if a run is due.
	interval := parseDuration(pipeline.Spec.Schedule)
	if interval == 0 {
		interval = 24 * time.Hour
	}
	if status.LastRunAt != nil && now.Sub(*status.LastRunAt) < interval && status.ObservedGeneration >= pipeline.Generation {
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: interval - now.Sub(*status.LastRunAt)}
	}

	// Initialize step statuses if needed.
	if len(status.StepStatuses) != len(pipeline.Spec.Steps) {
		status.StepStatuses = make([]StepStatus, len(pipeline.Spec.Steps))
		for i, step := range pipeline.Spec.Steps {
			status.StepStatuses[i] = StepStatus{Name: step.Name, Status: "pending"}
		}
	}

	// Execute steps in order (respecting dependencies).
	status.PipelineStatus = "running"
	status.Phase = "Running"
	allCompleted := true
	pipelineFailed := false

	for i, step := range pipeline.Spec.Steps {
		ss := &status.StepStatuses[i]

		// Skip already completed or failed steps.
		if ss.Status == "completed" || ss.Status == "failed" {
			if ss.Status == "failed" {
				pipelineFailed = true
			}
			continue
		}

		// Check dependencies.
		depsReady := true
		for _, dep := range step.DependsOn {
			for _, ds := range status.StepStatuses {
				if ds.Name == dep && ds.Status != "completed" {
					depsReady = false
					break
				}
			}
		}
		if !depsReady {
			allCompleted = false
			continue
		}

		// Execute step.
		allCompleted = false
		status.CurrentStep = step.Name
		ss.Status = "running"
		ss.StartedAt = &now

		if r.executor != nil {
			result, err := r.executor.ExecuteStep(ctx, pipeline, step)
			if err != nil {
				ss.Status = "failed"
				ss.Error = err.Error()
				endedAt := time.Now()
				ss.EndedAt = &endedAt
				pipelineFailed = true
				logging.Z().Warn("reconciliation error", zap.String("name", pipeline.GetKey()), zap.Error(err))
			} else if result != nil {
				*ss = *result
			}
		} else {
			// No executor — mark as completed for reconciler testing.
			ss.Status = "completed"
			endedAt := time.Now()
			ss.EndedAt = &endedAt
			ss.Duration = endedAt.Sub(now).Truncate(time.Millisecond).String()
		}

		// Only execute one step per reconcile cycle.
		break
	}

	if pipelineFailed {
		status.PipelineStatus = "failed"
		status.Phase = "Failed"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason: "StepFailed", Message: fmt.Sprintf("step '%s' failed", status.CurrentStep),
			LastTransitionTime: now,
		})
	} else if allCompleted {
		status.PipelineStatus = "completed"
		status.Phase = "Completed"
		status.RunCount++
		status.LastRunAt = &now
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "True",
			Reason: "PipelineCompleted",
			Message: fmt.Sprintf("all %d steps completed (run #%d)", len(pipeline.Spec.Steps), status.RunCount),
			LastTransitionTime: now,
		})
	} else {
		// Still running — requeue quickly.
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "True",
			Reason: "PipelineRunning", Message: fmt.Sprintf("executing step '%s'", status.CurrentStep),
			LastTransitionTime: now,
		})
	}

	status.ObservedGeneration = pipeline.Generation
	status.LastTransitionTime = now
	pipeline.Status = status
	storeutil.Update(ctx, r.store, pipeline) //nolint:errcheck

	if status.PipelineStatus == "running" {
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 5 * time.Second}
	}
	return reconciler.ReconcileResult{Requeue: true, RequeueAfter: interval}
}

func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
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
