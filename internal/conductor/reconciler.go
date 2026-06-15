package conductor

// Reconciler for models.ProducerResource and models.ConsumerResource.

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/conductor/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// ProducerReconciler reconciles models.ProducerResource objects.
type ProducerReconciler struct {
	store   store.ResourceStore[*models.ProducerResource]
	manager *Manager
}

// NewProducerReconciler builds a reconciler.
func NewProducerReconciler(rs store.ResourceStore[*models.ProducerResource], mgr *Manager) *ProducerReconciler {
	return &ProducerReconciler{store: rs, manager: mgr}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *ProducerReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*models.ProducerResource)
	if !ok {
		return reconciler.ReconcileResult{Error: conductorErr("conductor: producer reconciler received wrong type")}
	}

	now := time.Now()
	status := res.Status

	target := StatusStopped
	if res.Spec.Active {
		target = StatusActive
	}

	// Sync producer to manager.
	if r.manager != nil && res.Spec.Active {
		existing, _ := r.manager.GetProducer(res.Name)
		if existing == nil {
			req := &CreateProducerRequest{
				Name:        res.Name,
				Backend:     res.Spec.Backend,
				Exchange:    res.Spec.Exchange,
				RoutingKey:  res.Spec.RoutingKey,
				Topic:       res.Spec.Topic,
				ContentType: res.Spec.ContentType,
				Headers:     res.Spec.Headers,
				Config:      res.Spec.Config,
			}
			if _, err := r.manager.CreateProducer(req); err != nil {
				logging.Z().Info(fmt.Sprintf("conductor: create producer %s error: %v", res.Name, err))
			}
		}
	}

	status.ProducerStatus = target
	status.Phase = target
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertConductorCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: conductorBoolStatus(res.Spec.Active),
		Reason: target, Message: "producer reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

// ConsumerReconciler reconciles models.ConsumerResource objects.
type ConsumerReconciler struct {
	store   store.ResourceStore[*models.ConsumerResource]
	manager *Manager
}

// NewConsumerReconciler builds a reconciler.
func NewConsumerReconciler(rs store.ResourceStore[*models.ConsumerResource], mgr *Manager) *ConsumerReconciler {
	return &ConsumerReconciler{store: rs, manager: mgr}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *ConsumerReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*models.ConsumerResource)
	if !ok {
		return reconciler.ReconcileResult{Error: conductorErr("conductor: consumer reconciler received wrong type")}
	}

	now := time.Now()
	status := res.Status

	target := StatusStopped
	if res.Spec.Active {
		target = StatusActive
	}

	// Sync consumer to manager.
	if r.manager != nil && res.Spec.Active {
		existing, _ := r.manager.GetConsumer(res.Name)
		if existing == nil {
			req := &CreateConsumerRequest{
				Name:          res.Name,
				Backend:       res.Spec.Backend,
				Queue:         res.Spec.Queue,
				Exchange:      res.Spec.Exchange,
				RoutingKey:    res.Spec.RoutingKey,
				Topic:         res.Spec.Topic,
				ConsumerGroup: res.Spec.ConsumerGroup,
				Config:        res.Spec.Config,
			}
			if _, err := r.manager.CreateConsumer(req); err != nil {
				logging.Z().Info(fmt.Sprintf("conductor: create consumer %s error: %v", res.Name, err))
			}
		}
	}

	status.ConsumerStatus = target
	status.Phase = target
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertConductorCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: conductorBoolStatus(res.Spec.Active),
		Reason: target, Message: "consumer reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

func upsertConductorCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}

func conductorBoolStatus(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

type conductorErr string

func (e conductorErr) Error() string { return string(e) }
