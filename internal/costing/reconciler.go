package costing

// =====================================================
// WS-4.4 — Cost Reconciler
//
// Aggregates usage records into billing periods, checks quotas,
// and triggers cost alerts when thresholds are exceeded.
//
// Behavior:
//   1. Observe: Read models.CostPolicyResource from etcd
//   2. Diff: Check if aggregation is due
//   3. Act: Sum usage records for current period, check quotas
//   4. Update Status: Record totals, fire alerts if over threshold
// =====================================================

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/costing/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"

	"go.uber.org/zap"
)

// CostReconciler reconciles cost policies.
type CostReconciler struct {
	policyStore store.ResourceStore[*models.CostPolicyResource]
	usageStore  store.ResourceStore[*models.UsageRecordResource]
}

// NewCostReconciler creates a new reconciler.
func NewCostReconciler(
	policyStore store.ResourceStore[*models.CostPolicyResource],
	usageStore store.ResourceStore[*models.UsageRecordResource],
) *CostReconciler {
	return &CostReconciler{
		policyStore: policyStore,
		usageStore:  usageStore,
	}
}

// Reconcile implements reconciler.Reconciler.
func (r *CostReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	policy, ok := obj.(*models.CostPolicyResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("costing: reconciler received non-models.CostPolicyResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", policy.GetKey()), zap.String("kind", policy.GetTypeMeta().Kind))

	now := time.Now()
	status := policy.Status

	if !policy.Spec.Enabled {
		status.Phase = "Disabled"
		status.ObservedGeneration = policy.Generation
		status.LastTransitionTime = now
		policy.Status = status
		storeutil.Update(ctx, r.policyStore, policy) //nolint:errcheck
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 1 * time.Hour}
	}

	// Determine current billing period.
	periodStart, periodEnd := r.currentPeriod(now, policy.Spec.BillingPeriod)
	status.CurrentPeriodStart = &periodStart
	status.CurrentPeriodEnd = &periodEnd

	// Aggregate usage records for this tenant and period.
	usageByDimension := make(map[string]float64)
	var totalCredits float64

	if r.usageStore != nil {
		records, err := r.usageStore.List(ctx, "")
		if err == nil {
			for _, record := range records {
				if record.Spec.TenantID != policy.Spec.TenantID {
					continue
				}
				if record.Spec.Timestamp.Before(periodStart) || record.Spec.Timestamp.After(periodEnd) {
					continue
				}
				dim := string(record.Spec.Dimension)
				usageByDimension[dim] += record.Spec.Credits
				totalCredits += record.Spec.Credits
			}
		}
	}

	status.UsageByDimension = usageByDimension
	status.TotalCreditsUsed = totalCredits

	// Calculate total limit from quotas.
	var totalLimit float64
	for _, quota := range policy.Spec.Quotas {
		totalLimit += quota.Limit
	}
	status.TotalCreditsLimit = totalLimit

	// Check quota breaches.
	var breaches []string
	for dim, quota := range policy.Spec.Quotas {
		usage := usageByDimension[dim]
		if quota.Limit > 0 && usage >= quota.Limit {
			breaches = append(breaches, dim)
		}
	}
	status.QuotaBreaches = breaches

	// Update phase.
	if len(breaches) > 0 {
		status.Phase = "OverQuota"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "WithinBudget", Status: "False",
			Reason: "QuotaExceeded",
			Message: fmt.Sprintf("%d dimension(s) over quota: %v", len(breaches), breaches),
			LastTransitionTime: now,
		})
	} else if totalLimit > 0 && totalCredits/totalLimit > 0.8 {
		status.Phase = "Warning"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "WithinBudget", Status: "True",
			Reason: "ApproachingLimit",
			Message: fmt.Sprintf("usage at %.1f%% of total limit", totalCredits/totalLimit*100),
			LastTransitionTime: now,
		})
	} else {
		status.Phase = "Healthy"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "WithinBudget", Status: "True",
			Reason: "WithinLimits",
			Message: fmt.Sprintf("total usage: %.2f credits", totalCredits),
			LastTransitionTime: now,
		})
	}

	status.LastAggregatedAt = &now
	status.ObservedGeneration = policy.Generation
	status.LastTransitionTime = now
	policy.Status = status

	if r.policyStore != nil {
		storeutil.Update(ctx, r.policyStore, policy) //nolint:errcheck
	}

	return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 5 * time.Minute}
}

// currentPeriod calculates the start and end of the current billing period.
func (r *CostReconciler) currentPeriod(now time.Time, period string) (time.Time, time.Time) {
	switch period {
	case "daily":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return start, start.Add(24 * time.Hour)
	case "weekly":
		weekday := int(now.Weekday())
		start := time.Date(now.Year(), now.Month(), now.Day()-weekday, 0, 0, 0, 0, now.Location())
		return start, start.Add(7 * 24 * time.Hour)
	default: // monthly
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 1, 0)
		return start, end
	}
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
