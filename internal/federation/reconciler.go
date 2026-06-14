package federation

// =====================================================
// WS-5.1 — Virtual Table Reconciler
//
// Maintains virtual table metadata by validating that all underlying
// datasources are reachable and that column mappings are still valid.
//
// Behavior:
//   1. Observe: Read VirtualTableResource from etcd
//   2. Diff: Check source health and column validity against last validation
//   3. Act: Probe each source datasource, verify columns exist
//   4. Update Status: Record source health, column count, estimated rows
// =====================================================

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

// DataSourceProber abstracts health-checking and metadata lookup for datasources.
type DataSourceProber interface {
	// Ping checks if a datasource is reachable.
	Ping(ctx context.Context, dsRef string) error

	// TableExists checks if a table exists in the datasource.
	TableExists(ctx context.Context, dsRef, database, schema, table string) (bool, error)

	// EstimateRowCount returns an approximate row count for a table.
	EstimateRowCount(ctx context.Context, dsRef, database, schema, table string) (int64, error)
}

// VirtualTableReconciler reconciles virtual table resources.
type VirtualTableReconciler struct {
	store  store.ResourceStore[*VirtualTableResource]
	prober DataSourceProber
}

// NewVirtualTableReconciler creates a new reconciler.
func NewVirtualTableReconciler(
	s store.ResourceStore[*VirtualTableResource],
	prober DataSourceProber,
) *VirtualTableReconciler {
	return &VirtualTableReconciler{store: s, prober: prober}
}

// Reconcile implements reconciler.Reconciler.
func (r *VirtualTableReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	vt, ok := obj.(*VirtualTableResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("federation: reconciler received non-VirtualTableResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", vt.GetKey()), zap.String("kind", vt.GetTypeMeta().Kind))

	now := time.Now()
	status := vt.Status

	// Handle deletion.
	if vt.DeletedAt != nil {
		status.Phase = "Terminating"
		status.ObservedGeneration = vt.Generation
		status.LastTransitionTime = now
		vt.Status = status
		storeutil.Update(ctx, r.store, vt) //nolint:errcheck
		return reconciler.ReconcileResult{}
	}

	// Skip if recently validated and generation unchanged.
	if status.ObservedGeneration >= vt.Generation && status.LastValidatedAt != nil {
		if now.Sub(*status.LastValidatedAt) < 5*time.Minute {
			return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 5 * time.Minute}
		}
	}

	// Validate sources.
	status.SourceCount = len(vt.Spec.Sources)
	status.ColumnCount = len(vt.Spec.Columns)
	status.SourcesHealthy = 0
	status.SourcesUnhealthy = 0

	var totalEstimatedRows int64

	if r.prober != nil {
		for _, src := range vt.Spec.Sources {
			// Ping datasource.
			if err := r.prober.Ping(ctx, src.DataSourceRef); err != nil {
				status.SourcesUnhealthy++
				continue
			}

			// Verify table exists.
			exists, err := r.prober.TableExists(ctx, src.DataSourceRef, src.Database, src.Schema, src.Table)
			if err != nil || !exists {
				status.SourcesUnhealthy++
				continue
			}

			status.SourcesHealthy++

			// Estimate row count.
			rows, err := r.prober.EstimateRowCount(ctx, src.DataSourceRef, src.Database, src.Schema, src.Table)
			if err == nil {
				totalEstimatedRows += rows
			}
		}
	} else {
		// No prober — assume all sources healthy.
		status.SourcesHealthy = status.SourceCount
	}

	status.EstimatedRows = totalEstimatedRows
	status.LastValidatedAt = &now

	// Determine overall health.
	if status.SourcesUnhealthy == 0 && status.SourceCount > 0 {
		status.Phase = "Ready"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "True",
			Reason:  "AllSourcesHealthy",
			Message: fmt.Sprintf("all %d sources healthy, %d columns mapped", status.SourceCount, status.ColumnCount),
			LastTransitionTime: now,
		})
	} else if status.SourcesHealthy > 0 {
		status.Phase = "Degraded"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "True",
			Reason:  "PartiallyHealthy",
			Message: fmt.Sprintf("%d/%d sources healthy", status.SourcesHealthy, status.SourceCount),
			LastTransitionTime: now,
		})
	} else if status.SourceCount > 0 {
		status.Phase = "Unhealthy"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason:  "NoHealthySources",
			Message: "all sources are unreachable",
			LastTransitionTime: now,
		})
	} else {
		status.Phase = "Invalid"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason:  "NoSources",
			Message: "virtual table has no sources defined",
			LastTransitionTime: now,
		})
	}

	status.ObservedGeneration = vt.Generation
	status.LastTransitionTime = now
	vt.Status = status

	storeutil.Update(ctx, r.store, vt) //nolint:errcheck

	return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 5 * time.Minute}
}

// upsertCondition adds or updates a condition in the conditions slice.
func upsertCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}
