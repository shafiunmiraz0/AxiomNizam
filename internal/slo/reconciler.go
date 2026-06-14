package slo

// =====================================================
// WS-4.3 — SLO Reconciler
//
// Evaluates SLOs on a schedule, calculates error budgets,
// burn rates, and triggers burn-rate alerts when thresholds
// are exceeded.
//
// Behavior:
//   1. Observe: Read SLOResource from etcd
//   2. Diff: Check if evaluation is due (based on evalInterval)
//   3. Act: Query metrics, calculate SLI, budget, burn rate
//   4. Update Status: Record current state, fire alerts if needed
// =====================================================

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

// MetricsQuerier abstracts querying metrics for SLI calculation.
type MetricsQuerier interface {
	// QueryCounter returns the count for a metric query over a time window.
	QueryCounter(ctx context.Context, query string, window time.Duration) (int64, error)

	// QueryGauge returns the current gauge value for a metric query.
	QueryGauge(ctx context.Context, query string) (float64, error)
}

// SLOReconciler reconciles SLO resources.
type SLOReconciler struct {
	store   store.ResourceStore[*SLOResource]
	metrics MetricsQuerier
}

// NewSLOReconciler creates a new reconciler.
func NewSLOReconciler(
	s store.ResourceStore[*SLOResource],
	metrics MetricsQuerier,
) *SLOReconciler {
	return &SLOReconciler{
		store:   s,
		metrics: metrics,
	}
}

// Reconcile implements reconciler.Reconciler.
func (r *SLOReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	slo, ok := obj.(*SLOResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("slo: reconciler received non-SLOResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", slo.GetKey()), zap.String("kind", slo.GetTypeMeta().Kind))

	now := time.Now()
	status := slo.Status

	// Determine eval interval.
	evalInterval := r.parseInterval(slo.Spec.EvalInterval)
	if evalInterval == 0 {
		evalInterval = 1 * time.Minute
	}

	// Check if evaluation is due.
	if status.LastEvaluatedAt != nil && now.Sub(*status.LastEvaluatedAt) < evalInterval {
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: evalInterval}
	}

	// Parse window duration.
	window := r.parseInterval(slo.Spec.Window)
	if window == 0 {
		window = 30 * 24 * time.Hour // Default: 30 days
	}

	// Calculate window boundaries.
	windowEnd := now
	windowStart := now.Add(-window)
	status.WindowStart = &windowStart
	status.WindowEnd = &windowEnd

	// Query metrics for SLI calculation.
	var goodEvents, totalEvents int64
	var err error

	if r.metrics != nil {
		goodEvents, err = r.metrics.QueryCounter(ctx, slo.Spec.Indicator.GoodQuery, window)
		if err != nil {
			status.Conditions = upsertCondition(status.Conditions, resources.Condition{
				Type: "Ready", Status: "False",
				Reason: "MetricsQueryFailed", Message: fmt.Sprintf("good query failed: %v", err),
				LastTransitionTime: now,
			})
			status.ObservedGeneration = slo.Generation
			status.LastTransitionTime = now
			slo.Status = status
			storeutil.Update(ctx, r.store, slo) //nolint:errcheck
			logging.Z().Warn("reconciliation error", zap.String("name", slo.GetKey()), zap.Error(err))
			return reconciler.ReconcileResult{Requeue: true, RequeueAfter: resilience.ReconcileBackoff(1)}
		}

		totalEvents, err = r.metrics.QueryCounter(ctx, slo.Spec.Indicator.TotalQuery, window)
		if err != nil {
			status.Conditions = upsertCondition(status.Conditions, resources.Condition{
				Type: "Ready", Status: "False",
				Reason: "MetricsQueryFailed", Message: fmt.Sprintf("total query failed: %v", err),
				LastTransitionTime: now,
			})
			status.ObservedGeneration = slo.Generation
			status.LastTransitionTime = now
			slo.Status = status
			storeutil.Update(ctx, r.store, slo) //nolint:errcheck
			logging.Z().Warn("reconciliation error", zap.String("name", slo.GetKey()), zap.Error(err))
			return reconciler.ReconcileResult{Requeue: true, RequeueAfter: resilience.ReconcileBackoff(1)}
		}
	}

	status.GoodEvents = goodEvents
	status.TotalEvents = totalEvents

	// Calculate SLI.
	if totalEvents > 0 {
		status.CurrentSLI = float64(goodEvents) / float64(totalEvents)
	} else {
		status.CurrentSLI = 1.0 // No events = no errors = 100%
	}

	// Calculate error budget.
	target := slo.Spec.Target
	if target <= 0 || target >= 1 {
		target = 0.999 // Default: 99.9%
	}

	allowedErrorRate := 1.0 - target
	actualErrorRate := 1.0 - status.CurrentSLI

	if allowedErrorRate > 0 {
		status.BudgetConsumed = actualErrorRate / allowedErrorRate
		status.ErrorBudget = 1.0 - status.BudgetConsumed
		if status.ErrorBudget < 0 {
			status.ErrorBudget = 0
		}
	} else {
		status.BudgetConsumed = 0
		status.ErrorBudget = 1.0
	}

	// Calculate burn rate.
	// Burn rate = how fast we're consuming budget relative to the window.
	// A burn rate of 1.0 means we'll exactly exhaust the budget at window end.
	windowElapsed := now.Sub(windowStart)
	windowTotal := window
	if windowTotal > 0 && windowElapsed > 0 {
		expectedConsumption := float64(windowElapsed) / float64(windowTotal)
		if expectedConsumption > 0 {
			status.BurnRate = status.BudgetConsumed / expectedConsumption
		}
	}

	// Calculate time to exhaust.
	if status.BurnRate > 0 && status.ErrorBudget > 0 {
		remainingBudgetFraction := status.ErrorBudget
		timeToExhaust := time.Duration(float64(window) * remainingBudgetFraction / status.BurnRate)
		status.TimeToExhaust = timeToExhaust.Truncate(time.Minute).String()
	} else if status.ErrorBudget <= 0 {
		status.TimeToExhaust = "exhausted"
	} else {
		status.TimeToExhaust = "infinite"
	}

	// Determine if breaching.
	status.IsBreaching = status.CurrentSLI < target

	// Update phase and conditions.
	if status.IsBreaching {
		status.Phase = "Breaching"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Met", Status: "False",
			Reason: "SLOBreached",
			Message: fmt.Sprintf("SLI %.4f below target %.4f (budget consumed: %.1f%%)", status.CurrentSLI, target, status.BudgetConsumed*100),
			LastTransitionTime: now,
		})
	} else if status.BudgetConsumed > 0.8 {
		status.Phase = "AtRisk"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Met", Status: "True",
			Reason: "BudgetAtRisk",
			Message: fmt.Sprintf("SLI %.4f meets target but budget %.1f%% consumed", status.CurrentSLI, status.BudgetConsumed*100),
			LastTransitionTime: now,
		})
	} else {
		status.Phase = "Healthy"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Met", Status: "True",
			Reason: "SLOMet",
			Message: fmt.Sprintf("SLI %.4f meets target %.4f (budget remaining: %.1f%%)", status.CurrentSLI, target, status.ErrorBudget*100),
			LastTransitionTime: now,
		})
	}

	status.Conditions = upsertCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: "True",
		Reason: "Evaluated", Message: "SLO evaluation completed",
		LastTransitionTime: now,
	})

	status.LastEvaluatedAt = &now
	status.ObservedGeneration = slo.Generation
	status.LastTransitionTime = now
	slo.Status = status

	storeutil.Update(ctx, r.store, slo) //nolint:errcheck

	return reconciler.ReconcileResult{Requeue: true, RequeueAfter: evalInterval}
}

// parseInterval parses a duration string.
func (r *SLOReconciler) parseInterval(s string) time.Duration {
	if s == "" {
		return 0
	}
	// Handle day notation (e.g. "30d", "7d").
	if len(s) > 1 && s[len(s)-1] == 'd' {
		days := 0
		for _, c := range s[:len(s)-1] {
			if c >= '0' && c <= '9' {
				days = days*10 + int(c-'0')
			}
		}
		if days > 0 {
			return time.Duration(days) * 24 * time.Hour
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
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
