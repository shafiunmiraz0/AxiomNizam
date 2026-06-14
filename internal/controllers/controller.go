package controllers

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apiserver"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/timing"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
	"axiomnizam.bitbd.net/axiomnizam/internal/workqueue"
)

// ResourceWatcher is an alias to apiserver.ResourceWatcher for watching resource changes
type ResourceWatcher = apiserver.ResourceWatcher

// Reconciler performs reconciliation for a resource
type Reconciler interface {
	// Reconcile makes the actual state match desired state
	Reconcile(ctx context.Context, req ReconcileRequest) (ReconcileResult, error)

	// Finalize cleanup before deletion
	Finalize(ctx context.Context, resource resources.Resource) error
}

// ReconcileRequest is the request to reconcile a resource
type ReconcileRequest struct {
	// Namespace the resource is in
	Namespace string

	// Name of the resource
	Name string

	// Generation the resource was at when queued
	Generation int64

	// Reconciler to execute reconciliation
	Reconciler Reconciler

	// Resource the actual resource object
	Resource resources.Resource
}

// ReconcileResult is the result of reconciliation
type ReconcileResult struct {
	// Requeue indicates if item should be requeued
	Requeue bool

	// RequeueAfter requeue after duration
	RequeueAfter time.Duration
}

// ResourceController watches a resource type and reconciles changes
type ResourceController struct {
	name           string
	workQueue      workqueue.WorkQueue
	store          ResourceStore
	reconciler     Reconciler
	maxConcurrent  int
	finalizerName  string
	resyncPeriod   time.Duration
	lastResyncTime time.Time
}

// ResourceStore interface for accessing resources
type ResourceStore interface {
	Get(namespace, name string) (resources.Resource, error)
	Update(resource resources.Resource) error
	Watch(namespace string, watcher ResourceWatcher)
}

// NewResourceController creates a new resource controller
func NewResourceController(
	name string,
	workQueue workqueue.WorkQueue,
	store ResourceStore,
	reconciler Reconciler,
	maxConcurrent int,
) *ResourceController {
	return &ResourceController{
		name:          name,
		workQueue:     workQueue,
		store:         store,
		reconciler:    reconciler,
		maxConcurrent: maxConcurrent,
		finalizerName: fmt.Sprintf("%s.axiom.dev/controller", name),
		resyncPeriod:  timing.DefaultResyncPeriod,
	}
}

// Start starts the controller
func (rc *ResourceController) Start(ctx context.Context) error {
	logging.Z().Info(fmt.Sprintf("Starting controller %s", rc.name))

	// Start workers
	for i := 0; i < rc.maxConcurrent; i++ {
		go rc.runWorker(ctx, i)
	}

	// Start resync ticker
	go rc.resyncPeriodically(ctx)

	<-ctx.Done()
	logging.Z().Info(fmt.Sprintf("Stopping controller %s", rc.name))
	return rc.workQueue.Shutdown()
}

// runWorker processes items from the work queue
func (rc *ResourceController) runWorker(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		item, err := rc.workQueue.Get()
		if err != nil {
			return
		}

		rc.processItem(ctx, workerID, item)
	}
}

// processItem handles reconciliation for one item
func (rc *ResourceController) processItem(ctx context.Context, workerID int, item *workqueue.Item) {
	defer rc.workQueue.Done(item.Key)

	// Parse the item key (format: namespace/name)
	parts := parseKey(item.Key)
	if len(parts) != 2 {
		logging.Z().Info(fmt.Sprintf("Invalid key: %s", item.Key))
		return
	}

	namespace, name := parts[0], parts[1]

	// Get the resource
	resource, err := rc.store.Get(namespace, name)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("Worker %d: Failed to get resource %s/%s: %v", workerID, namespace, name, err))
		rc.workQueue.AddRateLimited(item.Key)
		return
	}

	// Handle deletion if marked
	if resource.GetObjectMeta().DeletedAt != nil {
		if err := rc.handleDeletion(ctx, resource); err != nil {
			logging.Z().Info(fmt.Sprintf("Worker %d: Failed to finalize resource: %v", workerID, err))
			rc.workQueue.AddRateLimited(item.Key)
		}
		return
	}

	// Run reconciliation
	result, err := rc.reconciler.Reconcile(ctx, ReconcileRequest{
		Namespace:  namespace,
		Name:       name,
		Generation: resource.GetObjectMeta().Generation,
	})

	if err != nil {
		logging.Z().Info(fmt.Sprintf("Worker %d: Reconciliation failed for %s/%s: %v", workerID, namespace, name, err))
		rc.workQueue.AddRateLimited(item.Key)
		return
	}

	if result.Requeue {
		if result.RequeueAfter > 0 {
			rc.workQueue.AddAfter(item.Key, result.RequeueAfter)
		} else {
			rc.workQueue.AddRateLimited(item.Key)
		}
	}
}

// handleDeletion handles resource deletion
func (rc *ResourceController) handleDeletion(ctx context.Context, resource resources.Resource) error {
	meta := resource.GetObjectMeta()

	// If finalizer exists, run finalization
	if meta.HasFinalizer(rc.finalizerName) {
		if err := rc.reconciler.Finalize(ctx, resource); err != nil {
			logging.Z().Info(fmt.Sprintf("Error finalizing resource: %v", err))
			return err
		}

		// Remove finalizer
		meta.RemoveFinalizer(rc.finalizerName)
		if err := rc.store.Update(resource); err != nil {
			logging.Z().Info(fmt.Sprintf("Error removing finalizer: %v", err))
			return err
		}
	}

	return nil
}

// resyncPeriodically resyncs all resources
func (rc *ResourceController) resyncPeriodically(ctx context.Context) {
	ticker := time.NewTicker(rc.resyncPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logging.Z().Info(fmt.Sprintf("Controller %s: Resyncing all resources", rc.name))
			rc.lastResyncTime = time.Now()
			// In a real implementation, would resync all resources
		}
	}
}

// Enqueue adds a resource to the work queue
func (rc *ResourceController) Enqueue(namespace, name string) {
	key := fmt.Sprintf("%s/%s", namespace, name)
	rc.workQueue.Add(key)
}

// WorkloadReconciler reconciles WorkloadResource
type WorkloadReconciler struct {
	store ResourceStore
}

// NewWorkloadReconciler creates a workload reconciler
func NewWorkloadReconciler(store ResourceStore) *WorkloadReconciler {
	return &WorkloadReconciler{store: store}
}

// Reconcile ensures workload is running
func (wr *WorkloadReconciler) Reconcile(ctx context.Context, req ReconcileRequest) (ReconcileResult, error) {
	// Get the workload
	resource, err := wr.store.Get(req.Namespace, req.Name)
	if err != nil {
		return ReconcileResult{}, err
	}

	workload := resource.(*resources.WorkloadResource)
	status := workload.GetStatus()

	// Check if already running
	if status.Phase == "Running" {
		return ReconcileResult{Requeue: false}, nil
	}

	// Transition to Running
	status.Phase = "Running"
	status.Conditions = append(status.Conditions, resources.Condition{
		Type:               "Ready",
		Status:             "True",
		Reason:             "WorkloadStarted",
		Message:            "Workload has been started",
		LastTransitionTime: time.Now(),
	})
	status.ObservedGeneration = req.Generation

	workload.SetStatus(status)

	if err := wr.store.Update(workload); err != nil {
		return ReconcileResult{Requeue: true}, err
	}

	logging.Z().Info(fmt.Sprintf("Workload %s/%s is now running", req.Namespace, req.Name))
	return ReconcileResult{Requeue: false}, nil
}

// Finalize cleans up workload resources
func (wr *WorkloadReconciler) Finalize(ctx context.Context, resource resources.Resource) error {
	workload := resource.(*resources.WorkloadResource)
	logging.Z().Info(fmt.Sprintf("Finalizing workload %s/%s", workload.ObjectMeta.Namespace, workload.ObjectMeta.Name))
	return nil
}

// PipelineReconciler reconciles PipelineResource
type PipelineReconciler struct {
	store ResourceStore
}

// NewPipelineReconciler creates a pipeline reconciler
func NewPipelineReconciler(store ResourceStore) *PipelineReconciler {
	return &PipelineReconciler{store: store}
}

// Reconcile executes pipeline stages
func (pr *PipelineReconciler) Reconcile(ctx context.Context, req ReconcileRequest) (ReconcileResult, error) {
	resource, err := pr.store.Get(req.Namespace, req.Name)
	if err != nil {
		return ReconcileResult{}, err
	}

	pipeline := resource.(*resources.PipelineResource)
	status := pipeline.GetStatus()

	// Check if already completed
	if status.Phase == "Succeeded" || status.Phase == "Failed" {
		return ReconcileResult{Requeue: false}, nil
	}

	// Execute first stage
	if status.Phase == "Pending" {
		status.Phase = "Running"
		status.ObservedGeneration = req.Generation
		pipeline.SetStatus(status)

		if err := pr.store.Update(pipeline); err != nil {
			return ReconcileResult{Requeue: true}, err
		}

		logging.Z().Info(fmt.Sprintf("Pipeline %s/%s executing", req.Namespace, req.Name))
		return ReconcileResult{Requeue: true, RequeueAfter: 10 * time.Second}, nil
	}

	return ReconcileResult{Requeue: false}, nil
}

// Finalize cleans up pipeline resources
func (pr *PipelineReconciler) Finalize(ctx context.Context, resource resources.Resource) error {
	pipeline := resource.(*resources.PipelineResource)
	logging.Z().Info(fmt.Sprintf("Finalizing pipeline %s/%s", pipeline.ObjectMeta.Namespace, pipeline.ObjectMeta.Name))
	return nil
}

// ScheduleReconciler reconciles ScheduleResource
type ScheduleReconciler struct {
	store ResourceStore
}

// NewScheduleReconciler creates a schedule reconciler
func NewScheduleReconciler(store ResourceStore) *ScheduleReconciler {
	return &ScheduleReconciler{store: store}
}

// Reconcile ensures schedule is active
func (sr *ScheduleReconciler) Reconcile(ctx context.Context, req ReconcileRequest) (ReconcileResult, error) {
	resource, err := sr.store.Get(req.Namespace, req.Name)
	if err != nil {
		return ReconcileResult{}, err
	}

	schedule := resource.(*resources.ScheduleResource)
	status := schedule.GetStatus()

	// Check if suspend is set
	if schedule.Spec.Suspend {
		status.Phase = "Suspended"
	} else {
		status.Phase = "Active"
	}

	status.ObservedGeneration = req.Generation
	schedule.SetStatus(status)

	if err := sr.store.Update(schedule); err != nil {
		return ReconcileResult{Requeue: true}, err
	}

	logging.Z().Info(fmt.Sprintf("Schedule %s/%s is %s", req.Namespace, req.Name, status.Phase))
	return ReconcileResult{Requeue: true, RequeueAfter: timing.DefaultRequeueAfter}, nil
}

// Finalize cleans up schedule resources
func (sr *ScheduleReconciler) Finalize(ctx context.Context, resource resources.Resource) error {
	schedule := resource.(*resources.ScheduleResource)
	logging.Z().Info(fmt.Sprintf("Finalizing schedule %s/%s", schedule.ObjectMeta.Namespace, schedule.ObjectMeta.Name))
	return nil
}
