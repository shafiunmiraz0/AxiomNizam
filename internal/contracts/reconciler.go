package contracts

// =====================================================
// WS-2.2 — Data Contract Reconciler
//
// Validates data contracts against actual schemas from the catalog.
// Detects breaking changes, schema drift, and SLA violations.
//
// Behavior:
//   1. Observe: Read DataContractResource from etcd
//   2. Diff: Compare contract schema against actual catalog asset schema
//   3. Act: Detect violations (schema drift, breaking changes)
//   4. Update Status: Record compliance state, violations, notify on break
// =====================================================

import (
	"context"
	"fmt"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/resilience"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"

	"go.uber.org/zap"
)

// CatalogAssetLookup abstracts looking up catalog asset metadata.
type CatalogAssetLookup interface {
	// GetAssetColumns returns the current columns for a catalog asset.
	GetAssetColumns(ctx context.Context, assetRef string) ([]ActualColumn, error)

	// GetAssetQualityScore returns the current quality score for an asset.
	GetAssetQualityScore(ctx context.Context, assetRef string) (float64, error)

	// GetAssetFreshness returns the last modified time for an asset.
	GetAssetFreshness(ctx context.Context, assetRef string) (*time.Time, error)
}

// ActualColumn represents a column as it exists in the real datasource.
type ActualColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// DataContractReconciler reconciles data contracts.
type DataContractReconciler struct {
	store   store.ResourceStore[*DataContractResource]
	catalog CatalogAssetLookup
}

// NewDataContractReconciler creates a new reconciler.
func NewDataContractReconciler(
	s store.ResourceStore[*DataContractResource],
	catalog CatalogAssetLookup,
) *DataContractReconciler {
	return &DataContractReconciler{
		store:   s,
		catalog: catalog,
	}
}

// Reconcile implements reconciler.Reconciler.
func (r *DataContractReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	contract, ok := obj.(*DataContractResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("contracts: reconciler received non-DataContractResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", contract.GetKey()), zap.String("kind", contract.GetTypeMeta().Kind))

	now := time.Now()
	status := contract.Status

	// Skip if disabled.
	if !contract.Spec.Enabled {
		status.Phase = "Disabled"
		status.ObservedGeneration = contract.Generation
		status.LastTransitionTime = now
		contract.Status = status
		storeutil.Update(ctx, r.store, contract) //nolint:errcheck
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 1 * time.Hour}
	}

	// Skip if already validated recently (within 5 minutes) and generation unchanged.
	if status.ObservedGeneration >= contract.Generation && status.LastValidatedAt != nil {
		if now.Sub(*status.LastValidatedAt) < 5*time.Minute {
			return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 5 * time.Minute}
		}
	}

	if r.catalog == nil {
		status.Phase = "Error"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason: "NoCatalog", Message: "catalog lookup not available",
			LastTransitionTime: now,
		})
		status.ObservedGeneration = contract.Generation
		status.LastTransitionTime = now
		contract.Status = status
		storeutil.Update(ctx, r.store, contract) //nolint:errcheck
		logging.Z().Warn("reconciliation error", zap.String("name", contract.GetKey()), zap.Error(fmt.Errorf("catalog lookup not available")))
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: resilience.ReconcileBackoff(1)}
	}

	// Validate schema.
	var violations []ContractViolation

	actualColumns, err := r.catalog.GetAssetColumns(ctx, contract.Spec.AssetRef)
	if err != nil {
		status.Phase = "Error"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason: "CatalogLookupFailed", Message: fmt.Sprintf("failed to get asset columns: %v", err),
			LastTransitionTime: now,
		})
		status.ObservedGeneration = contract.Generation
		status.LastTransitionTime = now
		contract.Status = status
		storeutil.Update(ctx, r.store, contract) //nolint:errcheck
		logging.Z().Warn("reconciliation error", zap.String("name", contract.GetKey()), zap.Error(err))
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: resilience.ReconcileBackoff(1)}
	}

	// Check schema violations.
	schemaViolations := r.validateSchema(contract, actualColumns, now)
	violations = append(violations, schemaViolations...)

	// Check quality violations.
	qualityScore, err := r.catalog.GetAssetQualityScore(ctx, contract.Spec.AssetRef)
	if err == nil && contract.Spec.Quality.MinQualityScore > 0 {
		if qualityScore < contract.Spec.Quality.MinQualityScore {
			violations = append(violations, ContractViolation{
				Type:       "quality_drop",
				Severity:   "warning",
				Message:    fmt.Sprintf("quality score %.1f below minimum %.1f", qualityScore, contract.Spec.Quality.MinQualityScore),
				Expected:   fmt.Sprintf(">=%.1f", contract.Spec.Quality.MinQualityScore),
				Actual:     fmt.Sprintf("%.1f", qualityScore),
				DetectedAt: now,
			})
		}
	}

	// Check freshness SLA.
	if contract.Spec.SLA.MaxFreshnessAge != "" {
		lastModified, err := r.catalog.GetAssetFreshness(ctx, contract.Spec.AssetRef)
		if err == nil && lastModified != nil {
			maxAge, parseErr := time.ParseDuration(contract.Spec.SLA.MaxFreshnessAge)
			if parseErr == nil {
				age := now.Sub(*lastModified)
				if age > maxAge {
					violations = append(violations, ContractViolation{
						Type:       "sla_breach",
						Severity:   "critical",
						Message:    fmt.Sprintf("data freshness %s exceeds SLA %s", age.Truncate(time.Second), maxAge),
						Expected:   fmt.Sprintf("<=%s", maxAge),
						Actual:     age.Truncate(time.Second).String(),
						DetectedAt: now,
					})
				}
			}
		}
	}

	// Update status.
	status.Violations = violations
	status.Compliant = len(violations) == 0
	status.LastValidatedAt = &now

	// Calculate schema match percentage.
	if len(contract.Spec.Schema.Columns) > 0 {
		matchCount := 0
		for _, expected := range contract.Spec.Schema.Columns {
			for _, actual := range actualColumns {
				if strings.EqualFold(expected.Name, actual.Name) {
					matchCount++
					break
				}
			}
		}
		status.SchemaMatchPercent = float64(matchCount) / float64(len(contract.Spec.Schema.Columns)) * 100.0
	}

	// Update SLA/quality status.
	status.SLAStatus = "met"
	status.QualityStatus = "met"
	for _, v := range violations {
		if v.Type == "sla_breach" {
			status.SLAStatus = "breached"
		}
		if v.Type == "quality_drop" {
			status.QualityStatus = "at_risk"
		}
	}

	if status.Compliant {
		status.ConsecutiveViolations = 0
		status.LastCompliantAt = &now
		status.Phase = "Compliant"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Compliant", Status: "True",
			Reason: "ContractMet", Message: "all contract terms are satisfied",
			LastTransitionTime: now,
		})
	} else {
		status.ConsecutiveViolations++
		status.Phase = "Violated"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Compliant", Status: "False",
			Reason: "ContractViolated",
			Message: fmt.Sprintf("%d violations detected", len(violations)),
			LastTransitionTime: now,
		})
	}

	status.Conditions = upsertCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: "True",
		Reason: "Validated", Message: "contract validation completed",
		LastTransitionTime: now,
	})

	status.ObservedGeneration = contract.Generation
	status.LastTransitionTime = now
	contract.Status = status

	storeutil.Update(ctx, r.store, contract) //nolint:errcheck

	return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 5 * time.Minute}
}

// validateSchema checks the contract schema against actual columns.
func (r *DataContractReconciler) validateSchema(contract *DataContractResource, actual []ActualColumn, now time.Time) []ContractViolation {
	var violations []ContractViolation

	actualMap := make(map[string]ActualColumn)
	for _, col := range actual {
		actualMap[strings.ToLower(col.Name)] = col
	}

	forbiddenSet := make(map[string]bool)
	for _, f := range contract.Spec.Schema.ForbiddenChanges {
		forbiddenSet[f] = true
	}

	// Check each expected column.
	for _, expected := range contract.Spec.Schema.Columns {
		actualCol, exists := actualMap[strings.ToLower(expected.Name)]

		if !exists {
			// Column missing.
			if expected.Required {
				severity := "critical"
				if forbiddenSet["drop_column"] {
					severity = "critical"
				}
				violations = append(violations, ContractViolation{
					Type:       "schema_drift",
					Severity:   severity,
					Message:    fmt.Sprintf("required column '%s' is missing", expected.Name),
					Field:      expected.Name,
					Expected:   "present",
					Actual:     "missing",
					DetectedAt: now,
				})
			}
			continue
		}

		// Check type mismatch.
		if expected.Type != "" && !typesCompatible(expected.Type, actualCol.Type) {
			severity := "warning"
			if forbiddenSet["change_type"] {
				severity = "critical"
			}
			violations = append(violations, ContractViolation{
				Type:       "schema_drift",
				Severity:   severity,
				Message:    fmt.Sprintf("column '%s' type changed", expected.Name),
				Field:      expected.Name,
				Expected:   expected.Type,
				Actual:     actualCol.Type,
				DetectedAt: now,
			})
		}

		// Check nullability change (nullable -> not nullable is breaking for backward compat).
		if expected.Nullable && !actualCol.Nullable {
			if contract.Spec.Compatibility == CompatBackward || contract.Spec.Compatibility == CompatFull {
				violations = append(violations, ContractViolation{
					Type:       "schema_drift",
					Severity:   "warning",
					Message:    fmt.Sprintf("column '%s' nullability changed (was nullable, now required)", expected.Name),
					Field:      expected.Name,
					Expected:   "nullable",
					Actual:     "not nullable",
					DetectedAt: now,
				})
			}
		}
	}

	// Check for required columns from the explicit list.
	for _, reqCol := range contract.Spec.Schema.RequiredColumns {
		if _, exists := actualMap[strings.ToLower(reqCol)]; !exists {
			violations = append(violations, ContractViolation{
				Type:       "schema_drift",
				Severity:   "critical",
				Message:    fmt.Sprintf("required column '%s' is missing from asset", reqCol),
				Field:      reqCol,
				Expected:   "present",
				Actual:     "missing",
				DetectedAt: now,
			})
		}
	}

	return violations
}

// typesCompatible checks if two SQL types are compatible (case-insensitive, alias-aware).
func typesCompatible(expected, actual string) bool {
	e := strings.ToLower(strings.TrimSpace(expected))
	a := strings.ToLower(strings.TrimSpace(actual))

	if e == a {
		return true
	}

	// Common aliases.
	aliases := map[string][]string{
		"int":       {"integer", "int4", "int32"},
		"bigint":    {"int8", "int64", "long"},
		"varchar":   {"text", "string", "character varying"},
		"float":     {"float4", "real"},
		"double":    {"float8", "double precision"},
		"bool":      {"boolean"},
		"timestamp": {"timestamptz", "datetime", "timestamp with time zone"},
	}

	for canonical, alts := range aliases {
		group := append(alts, canonical)
		eInGroup := false
		aInGroup := false
		for _, g := range group {
			if e == g || strings.HasPrefix(e, g+"(") {
				eInGroup = true
			}
			if a == g || strings.HasPrefix(a, g+"(") {
				aInGroup = true
			}
		}
		if eInGroup && aInGroup {
			return true
		}
	}

	return false
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
