package alerting

// AlertRuleReconciler evaluates alert rules on schedule and manages incidents.
//
// Lifecycle:
//   Rule created -> Reconciler evaluates condition on evalInterval
//     -> Condition TRUE for forDuration -> AlertIncident created (firing)
//       -> Notifier dispatches to channels
//       -> Escalation if not acknowledged
//     -> Condition FALSE -> AlertIncident resolved

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/alerting/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/resilience"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"

	"go.uber.org/zap"
)

// ConditionEvaluator evaluates alert conditions.
type ConditionEvaluator interface {
	// Evaluate returns true if the condition is currently met.
	Evaluate(ctx context.Context, condition models.AlertCondition) (bool, string, error)
}

// Notifier dispatches notifications to channels.
type Notifier interface {
	// Notify sends a notification to the specified channel.
	Notify(ctx context.Context, channel *models.NotificationChannelResource, incident *models.AlertIncidentResource) error
}

// AlertRuleReconciler reconciles alert rules.
type AlertRuleReconciler struct {
	ruleStore     store.ResourceStore[*models.AlertRuleResource]
	incidentStore store.ResourceStore[*models.AlertIncidentResource]
	channelStore  store.ResourceStore[*models.NotificationChannelResource]
	evaluator     ConditionEvaluator
	notifier      Notifier
}

// NewAlertRuleReconciler creates a new reconciler.
func NewAlertRuleReconciler(
	ruleStore store.ResourceStore[*models.AlertRuleResource],
	incidentStore store.ResourceStore[*models.AlertIncidentResource],
	channelStore store.ResourceStore[*models.NotificationChannelResource],
	evaluator ConditionEvaluator,
	notifier Notifier,
) *AlertRuleReconciler {
	return &AlertRuleReconciler{
		ruleStore:     ruleStore,
		incidentStore: incidentStore,
		channelStore:  channelStore,
		evaluator:     evaluator,
		notifier:      notifier,
	}
}

// Reconcile implements reconciler.Reconciler.
func (r *AlertRuleReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	rule, ok := obj.(*models.AlertRuleResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("alerting: received non-AlertRuleResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", rule.GetKey()), zap.String("kind", rule.GetTypeMeta().Kind))

	now := time.Now()
	status := rule.Status

	// Skip if disabled or silenced.
	if !rule.Spec.Enabled {
		status.RuleState = "inactive"
		status.Phase = "Disabled"
		status.ObservedGeneration = rule.Generation
		status.LastTransitionTime = now
		rule.Status = status
		storeutil.Update(ctx, r.ruleStore, rule) //nolint:errcheck
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 1 * time.Hour}
	}

	if rule.Spec.Silenced {
		if rule.Spec.SilenceUntil != nil && now.After(*rule.Spec.SilenceUntil) {
			// Silence expired — continue evaluation.
			rule.Spec.Silenced = false
		} else {
			status.RuleState = "silenced"
			status.Phase = "Silenced"
			status.ObservedGeneration = rule.Generation
			status.LastTransitionTime = now
			rule.Status = status
			storeutil.Update(ctx, r.ruleStore, rule) //nolint:errcheck
			return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 5 * time.Minute}
		}
	}

	// Check if evaluation is due.
	evalInterval := r.parseInterval(rule.Spec.EvalInterval)
	if evalInterval == 0 {
		evalInterval = 1 * time.Minute
	}

	if status.LastEvalAt != nil && now.Sub(*status.LastEvalAt) < evalInterval {
		remaining := evalInterval - now.Sub(*status.LastEvalAt)
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: remaining}
	}

	// Evaluate condition.
	status.LastEvalAt = &now
	status.EvalCount++

	conditionMet := false
	var conditionMsg string

	if r.evaluator != nil {
		met, msg, err := r.evaluator.Evaluate(ctx, rule.Spec.Condition)
		if err != nil {
			status.Conditions = upsertCondition(status.Conditions, resources.Condition{
				Type: "Evaluated", Status: "False",
				Reason: "EvalError", Message: err.Error(),
				LastTransitionTime: now,
			})
			status.ObservedGeneration = rule.Generation
			status.LastTransitionTime = now
			rule.Status = status
			storeutil.Update(ctx, r.ruleStore, rule) //nolint:errcheck
			logging.Z().Warn("reconciliation error", zap.String("name", rule.GetKey()), zap.Error(err))
			return reconciler.ReconcileResult{Requeue: true, RequeueAfter: resilience.ReconcileBackoff(1)}
		}
		conditionMet = met
		conditionMsg = msg
	}

	// Handle state transitions.
	forDuration := r.parseInterval(rule.Spec.ForDuration)

	if conditionMet {
		switch status.RuleState {
		case "", "inactive", "resolved":
			// Condition just became true — start pending period.
			if forDuration > 0 {
				status.RuleState = "pending"
				status.PendingSince = &now
			} else {
				// No forDuration — fire immediately.
				r.fireAlert(ctx, rule, &status, now, conditionMsg)
			}

		case "pending":
			// Check if forDuration has elapsed.
			if status.PendingSince != nil && now.Sub(*status.PendingSince) >= forDuration {
				r.fireAlert(ctx, rule, &status, now, conditionMsg)
			}

		case "firing":
			// Already firing — check escalation.
			r.checkEscalation(ctx, rule, &status, now)
		}
	} else {
		// Condition is false.
		switch status.RuleState {
		case "pending":
			// Reset pending.
			status.RuleState = "inactive"
			status.PendingSince = nil

		case "firing":
			// Resolve the incident.
			r.resolveAlert(ctx, rule, &status, now)
		}
	}

	status.ObservedGeneration = rule.Generation
	status.LastTransitionTime = now
	status.Phase = "Active"
	rule.Status = status

	storeutil.Update(ctx, r.ruleStore, rule) //nolint:errcheck

	return reconciler.ReconcileResult{Requeue: true, RequeueAfter: evalInterval}
}

// fireAlert transitions the rule to firing state and creates an incident.
func (r *AlertRuleReconciler) fireAlert(ctx context.Context, rule *models.AlertRuleResource, status *models.AlertRuleResourceStatus, now time.Time, message string) {
	status.RuleState = "firing"
	status.LastFiredAt = &now
	status.TotalFirings++
	status.PendingSince = nil

	// Create incident resource.
	if r.incidentStore != nil {
		incident := &models.AlertIncidentResource{
			TypeMeta: resources.TypeMeta{
				APIVersion: models.AlertIncidentAPIVersion,
				Kind:       models.AlertIncidentKind,
			},
			ObjectMeta: resources.ObjectMeta{
				Name:       fmt.Sprintf("%s-%d", rule.Name, now.Unix()),
				UID:        fmt.Sprintf("incident-%s-%d", rule.Name, now.UnixNano()),
				Generation: 1,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
			Spec: models.AlertIncidentSpec{
				RuleRef:     rule.Name,
				Severity:    rule.Spec.Severity,
				Summary:     rule.Spec.DisplayName,
				Description: message,
				Labels:      rule.Spec.Labels,
			},
			Status: models.AlertIncidentResourceStatus{
				ObjectStatus: resources.ObjectStatus{
					Phase:              "Firing",
					LastTransitionTime: now,
					ObservedGeneration: 1,
				},
				IncidentStatus: models.IncidentFiring,
				FiredAt:        &now,
			},
		}

		storeutil.Create(ctx, r.incidentStore, incident) //nolint:errcheck
		status.ActiveIncident = incident.Name

		// Dispatch notifications.
		r.dispatchNotifications(ctx, rule, incident)
	}
}

// resolveAlert resolves the active incident.
func (r *AlertRuleReconciler) resolveAlert(ctx context.Context, rule *models.AlertRuleResource, status *models.AlertRuleResourceStatus, now time.Time) {
	status.RuleState = "resolved"
	status.LastResolvedAt = &now

	if status.ActiveIncident != "" && r.incidentStore != nil {
		incident, err := r.incidentStore.Get(ctx, status.ActiveIncident)
		if err == nil {
			incident.Status.IncidentStatus = models.IncidentResolved
			incident.Status.ResolvedAt = &now
			incident.Status.Phase = "Resolved"
			incident.Status.LastTransitionTime = now
			storeutil.Update(ctx, r.incidentStore, incident) //nolint:errcheck
		}
		status.ActiveIncident = ""
	}
}

// checkEscalation checks if the alert needs to be escalated.
func (r *AlertRuleReconciler) checkEscalation(ctx context.Context, rule *models.AlertRuleResource, status *models.AlertRuleResourceStatus, now time.Time) {
	if rule.Spec.Escalation == nil || len(rule.Spec.Escalation.Levels) == 0 {
		return
	}

	if status.ActiveIncident == "" {
		return
	}

	incident, err := r.incidentStore.Get(ctx, status.ActiveIncident)
	if err != nil {
		return
	}

	// Check if acknowledged — no escalation needed.
	if incident.Status.IncidentStatus == models.IncidentAcknowledged {
		return
	}

	// Check escalation levels.
	currentLevel := incident.Status.EscalationLevel
	if currentLevel >= len(rule.Spec.Escalation.Levels) {
		return
	}

	level := rule.Spec.Escalation.Levels[currentLevel]
	escalateAfter := r.parseInterval(level.After)

	if incident.Status.FiredAt != nil && now.Sub(*incident.Status.FiredAt) >= escalateAfter {
		// Escalate.
		incident.Status.EscalationLevel++
		incident.Status.LastTransitionTime = now
		storeutil.Update(ctx, r.incidentStore, incident) //nolint:errcheck

		// Notify escalation channels.
		for _, chRef := range level.Channels {
			if r.channelStore != nil && r.notifier != nil {
				ch, err := r.channelStore.Get(ctx, chRef.Name)
				if err == nil {
					if err := r.notifier.Notify(ctx, ch, incident); err != nil { logging.Z().Warn("notification dispatch failed", zap.String("channel", ch.Name), zap.Error(err)) }
				}
			}
		}
	}
}

// dispatchNotifications sends notifications to all configured channels.
func (r *AlertRuleReconciler) dispatchNotifications(ctx context.Context, rule *models.AlertRuleResource, incident *models.AlertIncidentResource) {
	if r.notifier == nil || r.channelStore == nil {
		return
	}

	for _, chRef := range rule.Spec.Channels {
		ch, err := r.channelStore.Get(ctx, chRef.Name)
		if err != nil {
			continue
		}
		if !ch.Spec.Enabled {
			continue
		}
		if err := r.notifier.Notify(ctx, ch, incident); err != nil { logging.Z().Warn("notification dispatch failed", zap.String("channel", ch.Name), zap.Error(err)) }
	}
}

// parseInterval parses a duration string.
func (r *AlertRuleReconciler) parseInterval(s string) time.Duration {
	if s == "" {
		return 0
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
