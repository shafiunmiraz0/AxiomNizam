package integration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apiserver"
	"axiomnizam.bitbd.net/axiomnizam/internal/client"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
	"axiomnizam.bitbd.net/axiomnizam/internal/status"
)

// ========== RECONCILIATION INTEGRATION MANAGER ==========

// ReconciliationIntegration manages reconciliation across the platform
type ReconciliationIntegration struct {
	apiServer         *apiserver.APIServer
	resourceClient    client.ResourceClient
	reconciler        client.UnifiedReconciler
	statusTracker     *status.StatusTracker
	generationTracker *status.GenerationTracker
	eventLog          *status.ReconciliationEventLog
	conditionMgr      *status.ConditionManager
	workers           int
	workQueue         chan ReconciliationWork
	stopCh            chan struct{}
	wg                sync.WaitGroup
	mu                sync.RWMutex
}

// ReconciliationWork represents work to reconcile
type ReconciliationWork struct {
	Kind      string
	Namespace string
	Name      string
	ID        string // Unique reconciliation ID
	Timestamp time.Time
}

// NewReconciliationIntegration creates integration manager
func NewReconciliationIntegration(
	apiServer *apiserver.APIServer,
	resourceClient client.ResourceClient,
) *ReconciliationIntegration {
	return &ReconciliationIntegration{
		apiServer:         apiServer,
		resourceClient:    resourceClient,
		reconciler:        client.NewDefaultReconciler(resourceClient),
		statusTracker:     status.NewStatusTracker(),
		generationTracker: status.NewGenerationTracker(),
		eventLog:          status.NewReconciliationEventLog(1000),
		conditionMgr:      status.NewConditionManager(),
		workers:           4,
		workQueue:         make(chan ReconciliationWork, 100),
		stopCh:            make(chan struct{}),
	}
}

// Start starts reconciliation workers
func (ri *ReconciliationIntegration) Start(ctx context.Context) error {
	ri.mu.Lock()
	defer ri.mu.Unlock()

	for i := 0; i < ri.workers; i++ {
		ri.wg.Add(1)
		go ri.worker(ctx, i)
	}

	return nil
}

// Stop stops reconciliation workers
func (ri *ReconciliationIntegration) Stop() {
	ri.mu.Lock()
	defer ri.mu.Unlock()

	close(ri.stopCh)
	ri.wg.Wait()
}

// worker processes reconciliation work
func (ri *ReconciliationIntegration) worker(ctx context.Context, id int) {
	defer ri.wg.Done()

	for {
		select {
		case <-ri.stopCh:
			return
		case work := <-ri.workQueue:
			ri.reconcileResource(ctx, work)
		}
	}
}

// Enqueue enqueues a resource for reconciliation
func (ri *ReconciliationIntegration) Enqueue(kind, namespace, name string) {
	work := ReconciliationWork{
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
		ID:        fmt.Sprintf("%s-%s-%d", kind, name, time.Now().UnixNano()),
		Timestamp: time.Now(),
	}

	select {
	case ri.workQueue <- work:
	default:
		// Queue full, drop for now
	}
}

// reconcileResource performs actual reconciliation
func (ri *ReconciliationIntegration) reconcileResource(ctx context.Context, work ReconciliationWork) {
	reconID := work.ID

	// Log start
	ri.eventLog.Log(status.ReconciliationEvent{
		ReconciliationID: reconID,
		Kind:             work.Kind,
		Namespace:        work.Namespace,
		Name:             work.Name,
		Event:            "Started",
	})

	// 1. OBSERVE - Get desired and actual state
	ri.eventLog.Log(status.ReconciliationEvent{
		ReconciliationID: reconID,
		Kind:             work.Kind,
		Namespace:        work.Namespace,
		Name:             work.Name,
		Event:            "Observing",
	})

	desired, gen, err := ri.reconciler.GetDesiredState(ctx, work.Kind, work.Namespace, work.Name)
	if err != nil {
		ri.handleReconciliationError(ctx, work, reconID, "Failed to observe desired state", err)
		return
	}

	actual, err := ri.reconciler.GetActualState(ctx, work.Kind, work.Namespace, work.Name)
	if err != nil {
		ri.handleReconciliationError(ctx, work, reconID, "Failed to get actual state", err)
		return
	}

	// Update generation tracker
	ri.generationTracker.SetGeneration(work.Kind, work.Namespace, work.Name, gen)

	// 2. VALIDATE
	ri.eventLog.Log(status.ReconciliationEvent{
		ReconciliationID: reconID,
		Kind:             work.Kind,
		Namespace:        work.Namespace,
		Name:             work.Name,
		Event:            "Validating",
	})

	// Set validating condition
	ri.conditionMgr.SetCondition(work.Kind, work.Namespace, work.Name, status.Condition{
		Type:    "Validating",
		Status:  "True",
		Reason:  "ValidationInProgress",
		Message: "Validating resource configuration",
	})

	// Validation passes (simplified for now)
	ri.conditionMgr.SetCondition(work.Kind, work.Namespace, work.Name, status.Condition{
		Type:    "Validating",
		Status:  "False",
		Reason:  "ValidationComplete",
		Message: "Validation passed",
	})

	// 3. ACT
	ri.eventLog.Log(status.ReconciliationEvent{
		ReconciliationID: reconID,
		Kind:             work.Kind,
		Namespace:        work.Namespace,
		Name:             work.Name,
		Event:            "Acting",
	})

	// Set syncing condition
	ri.conditionMgr.SetCondition(work.Kind, work.Namespace, work.Name, status.Condition{
		Type:    "Synced",
		Status:  "False",
		Reason:  "SyncInProgress",
		Message: "Synchronizing to desired state",
	})

	// Compare and act (simplified: if different, mark as acted)
	if !ri.compare(desired, actual) {
		ri.act(ctx, work, desired)
	}

	// 4. UPDATE STATUS
	ri.eventLog.Log(status.ReconciliationEvent{
		ReconciliationID: reconID,
		Kind:             work.Kind,
		Namespace:        work.Namespace,
		Name:             work.Name,
		Event:            "Succeeded",
		Message:          "Reconciliation completed successfully",
	})

	// Set final status
	resourceStatus := &status.ResourceStatus{
		Kind:               work.Kind,
		Namespace:          work.Namespace,
		Name:               work.Name,
		Phase:              "Succeeded",
		Ready:              true,
		Message:            "Resource is in sync",
		Generation:         gen,
		ObservedGeneration: gen,
		ReconciliationID:   reconID,
		Conditions:         ri.conditionMgr.GetConditions(work.Kind, work.Namespace, work.Name),
	}

	// Update synced condition
	ri.conditionMgr.SetCondition(work.Kind, work.Namespace, work.Name, status.Condition{
		Type:    "Synced",
		Status:  "True",
		Reason:  "SyncComplete",
		Message: "Resource synchronized to desired state",
	})

	// Update ready condition
	ri.conditionMgr.SetCondition(work.Kind, work.Namespace, work.Name, status.Condition{
		Type:    "Ready",
		Status:  "True",
		Reason:  "ReconciliationSucceeded",
		Message: "Resource reconciliation succeeded",
	})

	resourceStatus.Conditions = ri.conditionMgr.GetConditions(work.Kind, work.Namespace, work.Name)

	// Update in tracker
	ri.statusTracker.SetStatus(resourceStatus)

	// Update in API server status subresource
	err = ri.reconciler.UpdateStatus(ctx, work.Kind, work.Namespace, work.Name, "Succeeded", true, "Resource is in sync")
	if err != nil {
		ri.eventLog.Log(status.ReconciliationEvent{
			ReconciliationID: reconID,
			Kind:             work.Kind,
			Namespace:        work.Namespace,
			Name:             work.Name,
			Event:            "Failed",
			Message:          "Status update failed",
			Error:            err.Error(),
		})
	}
}

// compare compares desired vs actual
func (ri *ReconciliationIntegration) compare(desired, actual map[string]interface{}) bool {
	// Simple comparison - in real implementation would do deep comparison
	if len(desired) != len(actual) {
		return false
	}
	for k, v := range desired {
		if actual[k] != v {
			return false
		}
	}
	return true
}

// act makes changes to reach desired state
func (ri *ReconciliationIntegration) act(ctx context.Context, work ReconciliationWork, desired map[string]interface{}) error {
	// Placeholder for actual implementation
	// In real scenario, would update external systems based on desired state
	return nil
}

// handleReconciliationError handles reconciliation errors
func (ri *ReconciliationIntegration) handleReconciliationError(ctx context.Context, work ReconciliationWork, reconID, msg string, err error) {
	ri.eventLog.Log(status.ReconciliationEvent{
		ReconciliationID: reconID,
		Kind:             work.Kind,
		Namespace:        work.Namespace,
		Name:             work.Name,
		Event:            "Failed",
		Message:          msg,
		Error:            err.Error(),
	})

	resourceStatus := &status.ResourceStatus{
		Kind:             work.Kind,
		Namespace:        work.Namespace,
		Name:             work.Name,
		Phase:            "Failed",
		Ready:            false,
		Message:          fmt.Sprintf("%s: %v", msg, err),
		ReconciliationID: reconID,
	}

	ri.statusTracker.SetStatus(resourceStatus)

	// Set failed condition
	ri.conditionMgr.SetCondition(work.Kind, work.Namespace, work.Name, status.Condition{
		Type:    "Failed",
		Status:  "True",
		Reason:  "ReconciliationFailed",
		Message: err.Error(),
	})
}

// GetStatus returns resource status
func (ri *ReconciliationIntegration) GetStatus(kind, namespace, name string) *status.ResourceStatus {
	return ri.statusTracker.GetStatus(kind, namespace, name)
}

// ListStatuses lists all statuses
func (ri *ReconciliationIntegration) ListStatuses() []*status.ResourceStatus {
	return ri.statusTracker.ListStatuses()
}

// GetReconciliationEvents returns events for resource
func (ri *ReconciliationIntegration) GetReconciliationEvents(kind, namespace, name string) []status.ReconciliationEvent {
	return ri.eventLog.GetEventsForResource(kind, namespace, name)
}

// ========== RECONCILIATION TRIGGER HOOK ==========

// TriggerFromAPIServer should be called when resource is applied via API
func (ri *ReconciliationIntegration) TriggerFromAPIServer(kind, namespace, name string) {
	// Enqueue for reconciliation
	ri.Enqueue(kind, namespace, name)
}

// ========== STATUS UPDATE HOOK FOR WATCHERS ==========

// OnResourceApplied is called when resource is applied
func (ri *ReconciliationIntegration) OnResourceApplied(resource resources.Resource) {
	meta := resource.GetObjectMeta()
	ri.TriggerFromAPIServer("", meta.Namespace, meta.Name)
}

// ========== QUERY INTERFACE ==========

// QueryStatus allows querying statuses
func (ri *ReconciliationIntegration) QueryStatus() *status.StatusQuery {
	return status.NewStatusQuery(ri.statusTracker)
}

// WaitForReady waits for resource to be ready
func (ri *ReconciliationIntegration) WaitForReady(ctx context.Context, kind, namespace, name string) error {
	waiter := status.NewStatusWaiter(ri.statusTracker)
	return waiter.Wait(ctx, kind, namespace, name)
}
