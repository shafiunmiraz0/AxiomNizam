package rules

// QualityRuleReconciler evaluates data quality rules on schedule.
//
// Behavior:
//   1. Observe: Read QualityRuleResource, check schedule against lastCheckAt
//   2. Diff: If check is due (interval elapsed or manual trigger)
//   3. Act: Connect to datasource, execute check, evaluate result
//   4. Update Status: Record pass/fail, update consecutive fails, trigger alert

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

// QualityChecker executes quality checks against datasources.
type QualityChecker interface {
	// CheckFreshness verifies data is not older than maxAge.
	CheckFreshness(ctx context.Context, dsRef, database, schema, table, timestampCol string, maxAge time.Duration) (*CheckOutput, error)

	// CheckVolume verifies row count is within expected range.
	CheckVolume(ctx context.Context, dsRef, database, schema, table string, minRows, maxRows int64) (*CheckOutput, error)

	// CheckNotNull verifies a column has no nulls (or below threshold).
	CheckNotNull(ctx context.Context, dsRef, database, schema, table, column string, threshold float64) (*CheckOutput, error)

	// CheckUnique verifies column values are unique.
	CheckUnique(ctx context.Context, dsRef, database, schema, table, column string, threshold float64) (*CheckOutput, error)

	// CheckCustomSQL executes a custom SQL query and checks row count.
	CheckCustomSQL(ctx context.Context, dsRef, query string, threshold int64) (*CheckOutput, error)

	// CheckRange verifies numeric column is within bounds.
	CheckRange(ctx context.Context, dsRef, database, schema, table, column string, min, max *float64) (*CheckOutput, error)

	// CheckCompleteness verifies column completeness ratio.
	CheckCompleteness(ctx context.Context, dsRef, database, schema, table, column string, threshold float64) (*CheckOutput, error)
}

// CheckOutput holds the result of a quality check execution.
type CheckOutput struct {
	Passed      bool   `json:"passed"`
	Message     string `json:"message"`
	FailCount   int64  `json:"failCount"`
	TotalRows   int64  `json:"totalRows"`
	ActualValue string `json:"actualValue,omitempty"`
}

// AlertDispatcher sends alerts when quality checks fail.
type AlertDispatcher interface {
	// DispatchQualityAlert sends an alert for a failed quality check.
	DispatchQualityAlert(ctx context.Context, rule *QualityRuleResource, message string) error
}

// QualityRuleReconciler reconciles quality rules.
type QualityRuleReconciler struct {
	store      store.ResourceStore[*QualityRuleResource]
	checker    QualityChecker
	alerter    AlertDispatcher
}

// NewQualityRuleReconciler creates a new reconciler.
func NewQualityRuleReconciler(
	s store.ResourceStore[*QualityRuleResource],
	checker QualityChecker,
	alerter AlertDispatcher,
) *QualityRuleReconciler {
	return &QualityRuleReconciler{
		store:   s,
		checker: checker,
		alerter: alerter,
	}
}

// Reconcile implements reconciler.Reconciler.
func (r *QualityRuleReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	rule, ok := obj.(*QualityRuleResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("quality: reconciler received non-QualityRuleResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", rule.GetKey()), zap.String("kind", rule.GetTypeMeta().Kind))

	now := time.Now()
	status := rule.Status

	// Skip if rule is disabled.
	if !rule.Spec.Enabled {
		status.Phase = "Disabled"
		status.ObservedGeneration = rule.Generation
		status.LastTransitionTime = now
		rule.Status = status
		storeutil.Update(ctx, r.store, rule) //nolint:errcheck
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 1 * time.Hour}
	}

	// Determine if check is due.
	if !r.isCheckDue(rule, now) {
		return reconciler.ReconcileResult{
			Requeue:      true,
			RequeueAfter: r.nextCheckInterval(rule),
		}
	}

	// Execute the quality check.
	if r.checker == nil {
		status.LastResult = CheckResultError
		status.LastFailureMessage = "no quality checker configured"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason: "NoChecker", Message: "quality checker not available",
			LastTransitionTime: now,
		})
		status.ObservedGeneration = rule.Generation
		status.LastTransitionTime = now
		rule.Status = status
		storeutil.Update(ctx, r.store, rule) //nolint:errcheck
		logging.Z().Warn("reconciliation error", zap.String("name", rule.GetKey()), zap.Error(fmt.Errorf("quality checker not available")))
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: resilience.ReconcileBackoff(1)}
	}

	start := time.Now()
	output, err := r.executeCheck(ctx, rule)
	duration := time.Since(start)

	status.LastCheckAt = &now
	status.LastCheckDuration = duration.String()
	status.TotalChecks++

	if err != nil {
		// Check execution error (not a data quality failure).
		status.LastResult = CheckResultError
		status.LastFailureMessage = err.Error()
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason: "CheckError", Message: fmt.Sprintf("check execution failed: %v", err),
			LastTransitionTime: now,
		})
		logging.Z().Warn("reconciliation error", zap.String("name", rule.GetKey()), zap.Error(err))
	} else if output.Passed {
		// Check passed.
		status.LastResult = CheckResultPass
		status.LastFailureMessage = ""
		status.ConsecutiveFails = 0
		status.TotalPasses++
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "True",
			Reason: "CheckPassed", Message: output.Message,
			LastTransitionTime: now,
		})
	} else {
		// Check failed — data quality issue.
		status.LastResult = CheckResultFail
		status.LastFailureMessage = output.Message
		status.ConsecutiveFails++
		status.TotalFailures++
		status.FailingRows = output.FailCount
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Ready", Status: "False",
			Reason: "CheckFailed", Message: output.Message,
			LastTransitionTime: now,
		})

		// Dispatch alert if configured.
		if rule.Spec.AlertOnFailure && r.alerter != nil {
			_ = r.alerter.DispatchQualityAlert(ctx, rule, output.Message)
		}
	}

	// Update pass rate.
	if status.TotalChecks > 0 {
		status.PassRate = float64(status.TotalPasses) / float64(status.TotalChecks)
	}

	// Update SLA status.
	status.SLAStatus = r.evaluateSLA(rule, &status)

	// Calculate next check time.
	nextCheck := now.Add(r.nextCheckInterval(rule))
	status.NextCheckAt = &nextCheck

	status.Phase = "Active"
	status.ObservedGeneration = rule.Generation
	status.LastTransitionTime = now
	rule.Status = status

	storeutil.Update(ctx, r.store, rule) //nolint:errcheck

	return reconciler.ReconcileResult{
		Requeue:      true,
		RequeueAfter: r.nextCheckInterval(rule),
	}
}

// executeCheck runs the appropriate check based on rule type.
func (r *QualityRuleReconciler) executeCheck(ctx context.Context, rule *QualityRuleResource) (*CheckOutput, error) {
	dsRef := rule.Spec.DataSourceRef

	switch rule.Spec.RuleType {
	case RuleTypeFreshness:
		if rule.Spec.Freshness == nil {
			return nil, fmt.Errorf("freshness rule config is nil")
		}
		maxAge, err := time.ParseDuration(rule.Spec.Freshness.MaxAge)
		if err != nil {
			return nil, fmt.Errorf("invalid maxAge duration: %w", err)
		}
		return r.checker.CheckFreshness(ctx, dsRef, "", "", rule.Spec.AssetRef, rule.Spec.Freshness.TimestampColumn, maxAge)

	case RuleTypeVolume:
		if rule.Spec.Volume == nil {
			return nil, fmt.Errorf("volume rule config is nil")
		}
		return r.checker.CheckVolume(ctx, dsRef, "", "", rule.Spec.AssetRef, rule.Spec.Volume.MinRows, rule.Spec.Volume.MaxRows)

	case RuleTypeNotNull:
		if rule.Spec.NotNull == nil {
			return nil, fmt.Errorf("not_null rule config is nil")
		}
		threshold := rule.Spec.NotNull.Threshold
		if threshold == 0 {
			threshold = 0.0 // Zero tolerance by default
		}
		return r.checker.CheckNotNull(ctx, dsRef, "", "", rule.Spec.AssetRef, rule.Spec.NotNull.Column, threshold)

	case RuleTypeUnique:
		if rule.Spec.Unique == nil {
			return nil, fmt.Errorf("unique rule config is nil")
		}
		threshold := rule.Spec.Unique.Threshold
		if threshold == 0 {
			threshold = 1.0 // 100% unique by default
		}
		return r.checker.CheckUnique(ctx, dsRef, "", "", rule.Spec.AssetRef, rule.Spec.Unique.Column, threshold)

	case RuleTypeCustomSQL:
		if rule.Spec.CustomSQL == nil {
			return nil, fmt.Errorf("custom_sql rule config is nil")
		}
		return r.checker.CheckCustomSQL(ctx, dsRef, rule.Spec.CustomSQL.Query, rule.Spec.CustomSQL.Threshold)

	case RuleTypeRange:
		if rule.Spec.Range == nil {
			return nil, fmt.Errorf("range rule config is nil")
		}
		return r.checker.CheckRange(ctx, dsRef, "", "", rule.Spec.AssetRef, rule.Spec.Range.Column, rule.Spec.Range.MinValue, rule.Spec.Range.MaxValue)

	case RuleTypeCompleteness:
		if rule.Spec.Completeness == nil {
			return nil, fmt.Errorf("completeness rule config is nil")
		}
		return r.checker.CheckCompleteness(ctx, dsRef, "", "", rule.Spec.AssetRef, rule.Spec.Completeness.Column, rule.Spec.Completeness.Threshold)

	default:
		return nil, fmt.Errorf("unsupported rule type: %s", rule.Spec.RuleType)
	}
}

// isCheckDue determines if the quality check should run now.
func (r *QualityRuleReconciler) isCheckDue(rule *QualityRuleResource, now time.Time) bool {
	// Always run if never checked.
	if rule.Status.LastCheckAt == nil {
		return true
	}

	// Run if generation changed.
	if rule.Status.ObservedGeneration < rule.Generation {
		return true
	}

	// Check interval.
	interval := r.nextCheckInterval(rule)
	return now.Sub(*rule.Status.LastCheckAt) >= interval
}

// nextCheckInterval returns the duration between checks.
func (r *QualityRuleReconciler) nextCheckInterval(rule *QualityRuleResource) time.Duration {
	if rule.Spec.Interval != "" {
		d, err := time.ParseDuration(rule.Spec.Interval)
		if err == nil {
			return d
		}
	}
	// Default: 1 hour
	return 1 * time.Hour
}

// evaluateSLA checks if the quality SLA is being met.
func (r *QualityRuleReconciler) evaluateSLA(rule *QualityRuleResource, status *QualityRuleResourceStatus) string {
	if rule.Spec.SLA == nil {
		return ""
	}

	sla := rule.Spec.SLA

	// Check consecutive failures.
	if sla.MaxConsecutiveFails > 0 && status.ConsecutiveFails >= sla.MaxConsecutiveFails {
		return "breached"
	}

	// Check pass rate.
	if sla.MinPassRate > 0 && status.TotalChecks > 10 {
		if status.PassRate < sla.MinPassRate {
			return "breached"
		}
		// At risk if within 5% of threshold.
		if status.PassRate < sla.MinPassRate+0.05 {
			return "at_risk"
		}
	}

	return "met"
}

// upsertCondition adds or updates a condition.
func upsertCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}
