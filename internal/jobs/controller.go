package jobs

// =====================================================
// P1.3 — JobController
//
// Reconciles `JobResource` through the shared rate-limited workqueue
// into the existing `JobManager`.  This is the declarative replacement
// for callers that previously spawned raw `go func()` submissions.
//
// The controller is intentionally thin: it relies on `JobManager` for
// the actual execution / persistence and only owns the Spec -> Job
// projection plus status updates.
// =====================================================

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
	"axiomnizam.bitbd.net/axiomnizam/internal/workqueue"
)

// JobResourceStore is a minimal persistence boundary for JobResource.
type JobResourceStore interface {
	GetJobResource(ctx context.Context, key string) (*JobResource, error)
	UpdateJobResourceStatus(ctx context.Context, key string, status JobResourceStatus) error
}

// JobController reconciles JobResource -> JobManager.Submit.
type JobController struct {
	manager *JobManagerImpl
	store   JobResourceStore
	queue   workqueue.WorkQueue

	mu       sync.Mutex
	workers  int
	running  bool
	cancel   context.CancelFunc
	finished chan struct{}
}

// NewJobController builds a controller bound to the given manager.
func NewJobController(manager *JobManagerImpl, store JobResourceStore) *JobController {
	return &JobController{
		manager: manager,
		store:   store,
		queue:   workqueue.NewSimpleQueue(nil),
		workers: 4,
	}
}

// Enqueue triggers a reconcile for the given key.
func (c *JobController) Enqueue(key string) { _ = c.queue.Add(key) }

// EnqueueAfter triggers a delayed reconcile for the given key.
func (c *JobController) EnqueueAfter(key string, d time.Duration) {
	_ = c.queue.AddAfter(key, d)
}

// Reconcile implements `reconciler.Reconciler`.
func (c *JobController) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	jr, ok := obj.(*JobResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("JobController: unexpected type %T", obj)}
	}
	if c.manager == nil {
		return reconciler.ReconcileResult{Error: fmt.Errorf("JobController: manager is nil")}
	}

	// Short-circuit if the spec hasn't moved since we last observed it
	// and the underlying job has reached a terminal state.
	if jr.Status.ObservedGeneration == jr.Generation && isTerminal(jr.Status.JobStatus) {
		return reconciler.ReconcileResult{}
	}

	// Suspended scheduled jobs must not be dispatched.
	if jr.Spec.Suspend {
		jr.Status.JobStatus = JobStatusCancelled
		jr.Status.Phase = string(JobStatusCancelled)
		jr.Status.ObservedGeneration = jr.Generation
		jr.Status.LastTransitionTime = time.Now()
		jr.Status.Conditions = upsertCondition(jr.Status.Conditions, resources.Condition{
			Type: "Scheduled", Status: "False",
			Reason: "Suspended", Message: "job spec is suspended",
			LastTransitionTime: time.Now(),
		})
		return c.persistStatus(ctx, jr)
	}

	// For one-shot jobs we call Submit exactly once per Generation.  If
	// we've already observed this generation we are past the dispatch
	// boundary and just keep status synced.
	dispatch := jr.Status.ObservedGeneration < jr.Generation

	if dispatch {
		job := ToJob(jr)
		if err := c.manager.Submit(ctx, job); err != nil {
			jr.Status.Error = err.Error()
			jr.Status.JobStatus = JobStatusFailed
			jr.Status.Phase = string(JobStatusFailed)
			_ = c.persistStatus(ctx, jr)
			return reconciler.ReconcileResult{
				Requeue:      true,
				RequeueAfter: 10 * time.Second,
				Error:        fmt.Errorf("Submit: %w", err),
			}
		}
		jr.Status.JobStatus = JobStatusPending
		jr.Status.Phase = string(JobStatusPending)
		jr.Status.ObservedGeneration = jr.Generation
		jr.Status.LastTransitionTime = time.Now()
		jr.Status.Conditions = upsertCondition(jr.Status.Conditions, resources.Condition{
			Type: "Queued", Status: "True",
			Reason: "Submitted", Message: "job handed to manager queue",
			LastTransitionTime: time.Now(),
		})
		return c.persistStatus(ctx, jr)
	}

	// Observe current job state from the manager's queue.
	current, err := c.manager.GetJob(ctx, ToJob(jr).ID)
	if err == nil && current != nil {
		jr.Status.JobStatus = current.Status
		jr.Status.Phase = string(current.Status)
		jr.Status.Retries = current.Retries
		jr.Status.Error = current.Error
		jr.Status.Result = current.Result
		if !current.StartedAt.IsZero() {
			jr.Status.StartedAt = current.StartedAt
		}
		if !current.CompletedAt.IsZero() {
			jr.Status.CompletedAt = current.CompletedAt
		}
	}
	result := c.persistStatus(ctx, jr)

	// Until the job reaches a terminal state, keep polling via the
	// rate-limited queue.
	if !isTerminal(jr.Status.JobStatus) {
		result.Requeue = true
		if result.RequeueAfter == 0 {
			result.RequeueAfter = 3 * time.Second
		}
	}
	return result
}

// persistStatus writes the current resource status through the store if
// one is configured.
func (c *JobController) persistStatus(ctx context.Context, jr *JobResource) reconciler.ReconcileResult {
	if c.store == nil {
		return reconciler.ReconcileResult{}
	}
	if err := c.store.UpdateJobResourceStatus(ctx, jr.GetKey(), jr.Status); err != nil {
		return reconciler.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
			Error:        fmt.Errorf("UpdateJobResourceStatus: %w", err),
		}
	}
	return reconciler.ReconcileResult{}
}

// Start launches reconciler workers against the shared workqueue.
func (c *JobController) Start(ctx context.Context) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	wctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.finished = make(chan struct{})
	workers := c.workers
	c.mu.Unlock()

	process := func(ctx context.Context, item *workqueue.Item) error {
		if c.store == nil {
			return nil
		}
		jr, err := c.store.GetJobResource(ctx, item.Key)
		if err != nil {
			return err
		}
		if jr == nil {
			return nil
		}
		res := c.Reconcile(ctx, jr)
		if res.Requeue && res.RequeueAfter > 0 {
			_ = c.queue.AddAfter(item.Key, res.RequeueAfter)
		}
		return res.Error
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := workqueue.NewWorker(c.queue, process, 20)
			if err := w.Run(wctx); err != nil {
				logging.Z().Info(fmt.Sprintf("[jobs] reconciler worker exited: %v", err))
			}
		}()
	}
	go func() { wg.Wait(); close(c.finished) }()
}

// Stop drains the controller.
func (c *JobController) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	cancel := c.cancel
	finished := c.finished
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if finished != nil {
		<-finished
	}
}

func isTerminal(s JobStatus) bool {
	switch s {
	case JobStatusCompleted, JobStatusFailed, JobStatusCancelled:
		return true
	}
	return false
}

// upsertCondition inserts-or-updates a Condition by Type.
func upsertCondition(cs []resources.Condition, c resources.Condition) []resources.Condition {
	for i := range cs {
		if cs[i].Type == c.Type {
			cs[i] = c
			return cs
		}
	}
	return append(cs, c)
}
