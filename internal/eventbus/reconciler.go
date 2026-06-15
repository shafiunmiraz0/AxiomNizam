package eventbus

// Reconciler for TopicResource and SubscriptionResource.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// TopicReconciler reconciles TopicResource objects.
type TopicReconciler struct {
	store   store.ResourceStore[*TopicResource]
	manager EventBusManager
}

// NewTopicReconciler builds a reconciler.
func NewTopicReconciler(rs store.ResourceStore[*TopicResource], mgr EventBusManager) *TopicReconciler {
	return &TopicReconciler{store: rs, manager: mgr}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *TopicReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*TopicResource)
	if !ok {
		return reconciler.ReconcileResult{Error: eventbusErr("eventbus: topic reconciler received wrong type")}
	}

	now := time.Now()
	status := res.Status

	phase := "Inactive"
	if res.Spec.Active {
		phase = "Active"
		// Ensure topic exists in manager.
		if r.manager != nil {
			topic := &EventTopic{
				Name:              res.Name,
				Description:       res.Spec.Description,
				Schema:            res.Spec.Schema,
				Partitions:        res.Spec.Partitions,
				ReplicationFactor: res.Spec.ReplicationFactor,
				Retention:         res.Spec.Retention,
				Config:            res.Spec.Config,
				IsActive:          true,
			}
			_, _ = r.manager.CreateTopic(topic)
		}
	}

	status.Phase = phase
	status.TopicActive = res.Spec.Active
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertEventbusCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: boolToStatus(res.Spec.Active),
		Reason: phase, Message: "topic reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

// SubscriptionReconciler reconciles SubscriptionResource objects.
type SubscriptionReconciler struct {
	store   store.ResourceStore[*SubscriptionResource]
	manager EventBusManager
}

// NewSubscriptionReconciler builds a reconciler.
func NewSubscriptionReconciler(rs store.ResourceStore[*SubscriptionResource], mgr EventBusManager) *SubscriptionReconciler {
	return &SubscriptionReconciler{store: rs, manager: mgr}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *SubscriptionReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*SubscriptionResource)
	if !ok {
		return reconciler.ReconcileResult{Error: eventbusErr("eventbus: subscription reconciler received wrong type")}
	}

	now := time.Now()
	status := res.Status

	target := "active"
	if res.Spec.Paused {
		target = "paused"
	}

	status.SubscriptionStatus = target
	status.Phase = target
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertEventbusCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: boolToStatus(!res.Spec.Paused),
		Reason: target, Message: "subscription reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

func upsertEventbusCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}

func boolToStatus(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

type eventbusErr string

func (e eventbusErr) Error() string { return string(e) }
