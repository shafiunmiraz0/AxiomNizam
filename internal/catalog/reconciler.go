package catalog

// CatalogAssetReconciler reconciles CatalogAssetResource objects.
//
// Behavior:
//   1. Observe: Read CatalogAssetResource from etcd
//   2. Diff: Compare spec.refreshPolicy against status.lastScannedAt
//   3. Act: If stale, connect to datasource, introspect schema, update metadata
//   4. Update Status: Write back discovered metadata, quality score, freshness

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

// DataSourceConnector abstracts connecting to a datasource and introspecting schema.
type DataSourceConnector interface {
	// IntrospectTable returns column metadata, row count, and size for a table.
	IntrospectTable(ctx context.Context, dsRef, database, schema, table string) (*IntrospectionResult, error)

	// TestConnection verifies the datasource is reachable.
	TestConnection(ctx context.Context, dsRef string) error
}

// IntrospectionResult holds the result of a datasource introspection.
type IntrospectionResult struct {
	Columns    []CatalogColumn
	RowCount   int64
	SizeBytes  int64
	IndexCount int
	ModifiedAt *time.Time
}

// CatalogAssetReconciler reconciles catalog assets.
type CatalogAssetReconciler struct {
	store     store.ResourceStore[*CatalogAssetResource]
	connector DataSourceConnector
}

// NewCatalogAssetReconciler creates a new reconciler.
func NewCatalogAssetReconciler(
	s store.ResourceStore[*CatalogAssetResource],
	connector DataSourceConnector,
) *CatalogAssetReconciler {
	return &CatalogAssetReconciler{
		store:     s,
		connector: connector,
	}
}

// Reconcile implements reconciler.Reconciler.
func (r *CatalogAssetReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	asset, ok := obj.(*CatalogAssetResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("catalog: reconciler received non-CatalogAssetResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", asset.GetKey()), zap.String("kind", asset.GetTypeMeta().Kind))

	now := time.Now()
	status := asset.Status

	// Check if asset is being deleted (has deletedAt set).
	if asset.DeletedAt != nil {
		// Handle finalizer cleanup if needed.
		status.Phase = "Terminating"
		status.ObservedGeneration = asset.Generation
		status.LastTransitionTime = now
		asset.Status = status
		storeutil.Update(ctx, r.store, asset) //nolint:errcheck
		return reconciler.ReconcileResult{}
	}

	// Determine if a scan is needed.
	needsScan := r.needsScan(asset, now)

	if !needsScan {
		// No scan needed — just ensure status is up to date.
		if status.ObservedGeneration < asset.Generation {
			status.ObservedGeneration = asset.Generation
			status.LastTransitionTime = now
			asset.Status = status
			storeutil.Update(ctx, r.store, asset) //nolint:errcheck
		}
		// Requeue for next scan interval.
		return reconciler.ReconcileResult{
			Requeue:      true,
			RequeueAfter: r.nextScanInterval(asset),
		}
	}

	// Perform introspection scan.
	if r.connector == nil {
		status.ScanError = "no datasource connector configured"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason: "NoConnector", Message: "datasource connector not available",
			LastTransitionTime: now,
		})
		status.ObservedGeneration = asset.Generation
		status.LastTransitionTime = now
		asset.Status = status
		storeutil.Update(ctx, r.store, asset) //nolint:errcheck
		logging.Z().Warn("reconciliation error", zap.String("name", asset.GetKey()), zap.Error(fmt.Errorf("datasource connector not available")))
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: resilience.ReconcileBackoff(1)}
	}

	// Execute introspection.
	result, err := r.connector.IntrospectTable(
		ctx,
		asset.Spec.DataSourceRef,
		asset.Spec.Database,
		asset.Spec.Schema,
		asset.Spec.TableName,
	)

	if err != nil {
		status.ScanError = err.Error()
		status.FreshnessStatus = "unknown"
		status.ConsecutiveScanFailures++
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason: "ScanFailed", Message: fmt.Sprintf("introspection failed: %v", err),
			LastTransitionTime: now,
		})
		status.ObservedGeneration = asset.Generation
		status.LastTransitionTime = now
		asset.Status = status
		storeutil.Update(ctx, r.store, asset) //nolint:errcheck
		logging.Z().Warn("reconciliation error", zap.String("name", asset.GetKey()), zap.Error(err))
		// Retry with exponential backoff based on consecutive failures.
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: resilience.ReconcileBackoff(status.ConsecutiveScanFailures)}
	}

	// Update status with introspection results.
	status.RowCount = result.RowCount
	status.SizeBytes = result.SizeBytes
	status.ColumnCount = len(result.Columns)
	status.IndexCount = result.IndexCount
	status.LastScannedAt = &now
	status.LastModifiedAt = result.ModifiedAt
	status.ScanError = ""
	status.ConsecutiveScanFailures = 0
	status.Phase = "Active"

	// Determine freshness.
	status.FreshnessStatus = r.calculateFreshness(result.ModifiedAt, now)

	// Update conditions.
	status.Conditions = upsertCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: "True",
		Reason: "ScanComplete", Message: fmt.Sprintf("scanned %d columns, %d rows", len(result.Columns), result.RowCount),
		LastTransitionTime: now,
	})
	status.Conditions = upsertCondition(status.Conditions, resources.Condition{
		Type: "Scanned", Status: "True",
		Reason: "IntrospectionComplete", Message: "metadata refreshed from source",
		LastTransitionTime: now,
	})

	status.ObservedGeneration = asset.Generation
	status.LastTransitionTime = now
	asset.Status = status

	// Also update columns in spec if auto-discovery is enabled.
	if len(result.Columns) > 0 && len(asset.Spec.Columns) == 0 {
		asset.Spec.Columns = result.Columns
	}

	storeutil.Update(ctx, r.store, asset) //nolint:errcheck

	// Requeue for next scan.
	return reconciler.ReconcileResult{
		Requeue:      true,
		RequeueAfter: r.nextScanInterval(asset),
	}
}

// needsScan determines if the asset needs a metadata refresh.
func (r *CatalogAssetReconciler) needsScan(asset *CatalogAssetResource, now time.Time) bool {
	// Always scan if never scanned.
	if asset.Status.LastScannedAt == nil {
		return true
	}

	// Scan if generation changed (spec updated).
	if asset.Status.ObservedGeneration < asset.Generation {
		return true
	}

	// Scan if refresh policy interval has elapsed.
	if !asset.Spec.RefreshPolicy.Enabled {
		return false
	}

	interval := r.parseInterval(asset.Spec.RefreshPolicy.Interval)
	if interval == 0 {
		interval = 1 * time.Hour // Default: 1 hour
	}

	return now.Sub(*asset.Status.LastScannedAt) >= interval
}

// nextScanInterval returns the duration until the next scan should occur.
func (r *CatalogAssetReconciler) nextScanInterval(asset *CatalogAssetResource) time.Duration {
	if !asset.Spec.RefreshPolicy.Enabled {
		return 24 * time.Hour // Check daily even if disabled
	}

	interval := r.parseInterval(asset.Spec.RefreshPolicy.Interval)
	if interval == 0 {
		return 1 * time.Hour
	}
	return interval
}

// parseInterval parses a duration string like "1h", "6h", "24h".
func (r *CatalogAssetReconciler) parseInterval(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// calculateFreshness determines if data is fresh, stale, or unknown.
func (r *CatalogAssetReconciler) calculateFreshness(modifiedAt *time.Time, now time.Time) string {
	if modifiedAt == nil {
		return "unknown"
	}
	age := now.Sub(*modifiedAt)
	if age < 1*time.Hour {
		return "fresh"
	}
	if age < 24*time.Hour {
		return "recent"
	}
	if age < 7*24*time.Hour {
		return "aging"
	}
	return "stale"
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
