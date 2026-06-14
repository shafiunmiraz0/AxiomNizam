package featurestore

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

// FeatureMaterializer abstracts the feature materialization pipeline.
type FeatureMaterializer interface {
	Materialize(ctx context.Context, group *FeatureGroupResource) (*MaterializeResult, error)
}

type MaterializeResult struct {
	EntityCount int64         `json:"entityCount"`
	Duration    time.Duration `json:"duration"`
}

type FeatureGroupReconciler struct {
	store        store.ResourceStore[*FeatureGroupResource]
	materializer FeatureMaterializer
}

func NewFeatureGroupReconciler(s store.ResourceStore[*FeatureGroupResource], m FeatureMaterializer) *FeatureGroupReconciler {
	return &FeatureGroupReconciler{store: s, materializer: m}
}

func (r *FeatureGroupReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	fg, ok := obj.(*FeatureGroupResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("featurestore: received non-FeatureGroupResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", fg.GetKey()), zap.String("kind", fg.GetTypeMeta().Kind))

	now := time.Now()
	status := fg.Status

	if !fg.Spec.Enabled {
		status.Phase = "Disabled"
		status.ObservedGeneration = fg.Generation
		status.LastTransitionTime = now
		fg.Status = status
		storeutil.Update(ctx, r.store, fg) //nolint:errcheck
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 1 * time.Hour}
	}

	status.FeatureCount = len(fg.Spec.Features)

	// Check if materialization is due.
	interval := parseDuration(fg.Spec.Schedule)
	if interval == 0 {
		interval = 1 * time.Hour
	}

	needsMaterialization := status.LastMaterializedAt == nil ||
		now.Sub(*status.LastMaterializedAt) >= interval ||
		status.ObservedGeneration < fg.Generation

	if !needsMaterialization {
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: interval}
	}

	// Run materialization.
	if r.materializer != nil {
		result, err := r.materializer.Materialize(ctx, fg)
		if err != nil {
			status.Phase = "Error"
			status.Conditions = upsertCondition(status.Conditions, resources.Condition{
				Type: "Ready", Status: "False",
				Reason: "MaterializationFailed", Message: err.Error(),
				LastTransitionTime: now,
			})
			status.ObservedGeneration = fg.Generation
			status.LastTransitionTime = now
			fg.Status = status
			storeutil.Update(ctx, r.store, fg) //nolint:errcheck
			logging.Z().Warn("reconciliation error", zap.String("name", fg.GetKey()), zap.Error(err))
			return reconciler.ReconcileResult{Requeue: true, RequeueAfter: resilience.ReconcileBackoff(1)}
		}

		status.EntityCount = result.EntityCount
		status.MaterializationDuration = result.Duration.Truncate(time.Millisecond).String()
		status.LastMaterializedAt = &now
		status.OnlineStoreStatus = "ready"
		status.OfflineStoreStatus = "ready"
		status.FreshnessStatus = "fresh"
	} else {
		status.LastMaterializedAt = &now
		status.OnlineStoreStatus = "ready"
		status.FreshnessStatus = "fresh"
	}

	status.Phase = "Ready"
	status.Conditions = upsertCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: "True",
		Reason: "Materialized",
		Message: fmt.Sprintf("materialized %d features for %d entities", status.FeatureCount, status.EntityCount),
		LastTransitionTime: now,
	})

	status.ObservedGeneration = fg.Generation
	status.LastTransitionTime = now
	fg.Status = status
	storeutil.Update(ctx, r.store, fg) //nolint:errcheck

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
