package etl

// =====================================================
// P1.1 — ETL Pipeline controller
//
// Reconciles `PipelineResource`s against the imperative `Engine` by
// upserting / removing the underlying `*Pipeline`.  Uses the shared
// rate-limited workqueue so retries are exponentially backed-off and
// writes are never done inside fire-and-forget goroutines.
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

// PipelineStore is the minimal lookup used by the controller to fetch a
// `PipelineResource` by key.  It is kept small so callers can back it by
// any persistent store (etcd, memory, postgres).
type PipelineStore interface {
	GetPipeline(ctx context.Context, key string) (*PipelineResource, error)
	UpdatePipelineStatus(ctx context.Context, key string, status PipelineResourceStatus) error
}

// PipelineController reconciles PipelineResource -> *Pipeline inside
// Engine.  One controller per Engine.
type PipelineController struct {
	engine *Engine
	store  PipelineStore
	queue  workqueue.WorkQueue

	mu       sync.Mutex
	workers  int
	running  bool
	cancel   context.CancelFunc
	finished chan struct{}
}

// NewPipelineController constructs a controller bound to the given
// Engine.  `store` may be nil for pure in-memory operation; in that
// case `Reconcile` becomes a no-op on status persistence.
func NewPipelineController(engine *Engine, store PipelineStore) *PipelineController {
	return &PipelineController{
		engine:  engine,
		store:   store,
		queue:   workqueue.NewSimpleQueue(nil),
		workers: 2,
	}
}

// Enqueue schedules a reconcile for the given key.
func (c *PipelineController) Enqueue(key string) {
	_ = c.queue.Add(key)
}

// EnqueueAfter schedules a delayed reconcile for the given key.
func (c *PipelineController) EnqueueAfter(key string, d time.Duration) {
	_ = c.queue.AddAfter(key, d)
}

// Reconcile implements `reconciler.Reconciler`.
func (c *PipelineController) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	pr, ok := obj.(*PipelineResource)
	if !ok {
		return reconciler.ReconcileResult{
			Error: fmt.Errorf("PipelineController: unexpected resource type %T", obj),
		}
	}
	if c.engine == nil {
		return reconciler.ReconcileResult{Error: fmt.Errorf("PipelineController: engine is nil")}
	}

	desired := ToPipeline(pr)

	// Observe actual state inside the engine.
	c.engine.mu.Lock()
	existing, exists := c.engine.pipelines[desired.ID]
	if !exists {
		desired.CreatedAt = time.Now()
		c.engine.pipelines[desired.ID] = desired
	} else {
		// Act: update the mutable parts of the pipeline in place so
		// any in-flight runs continue to observe the latest spec.
		existing.Description = desired.Description
		existing.Steps = desired.Steps
		existing.Schedule = desired.Schedule
		existing.Orchestration = desired.Orchestration
		existing.Config = desired.Config
		existing.Tags = desired.Tags
		existing.UpdatedAt = time.Now()
		if pr.Spec.Paused && existing.Status == PipelineRunning {
			existing.Status = PipelinePaused
		}
	}
	snapshot := *c.engine.pipelines[desired.ID]
	c.engine.mu.Unlock()

	// Update status.
	newStatus := pr.Status
	newStatus.ObservedGeneration = pr.Generation
	newStatus.PipelineStatus = snapshot.Status
	newStatus.RunCount = snapshot.RunCount
	newStatus.LastRunAt = snapshot.LastRunAt
	newStatus.Phase = string(snapshot.Status)
	newStatus.LastTransitionTime = time.Now()
	newStatus.Conditions = upsertCondition(newStatus.Conditions, resources.Condition{
		Type:               "Ready",
		Status:             "True",
		Reason:             "Reconciled",
		Message:            "pipeline spec applied to engine",
		LastTransitionTime: time.Now(),
	})

	if c.store != nil {
		if err := c.store.UpdatePipelineStatus(ctx, pr.GetKey(), newStatus); err != nil {
			return reconciler.ReconcileResult{
				Requeue:      true,
				RequeueAfter: 5 * time.Second,
				Error:        fmt.Errorf("UpdatePipelineStatus: %w", err),
			}
		}
	}
	pr.Status = newStatus
	return reconciler.ReconcileResult{}
}

// Start launches the configured number of worker loops.  Start is
// idempotent.
func (c *PipelineController) Start(ctx context.Context) {
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
		pr, err := c.store.GetPipeline(ctx, item.Key)
		if err != nil {
			return err
		}
		if pr == nil {
			return nil
		}
		res := c.Reconcile(ctx, pr)
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
			w := workqueue.NewWorker(c.queue, process, 10)
			if err := w.Run(wctx); err != nil {
				logging.Z().Info(fmt.Sprintf("[etl] pipeline worker exited: %v", err))
			}
		}()
	}
	go func() {
		wg.Wait()
		close(c.finished)
	}()
}

// Stop signals the controller to drain and exit.
func (c *PipelineController) Stop() {
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

// upsertCondition inserts-or-updates a condition by its Type.
func upsertCondition(cs []resources.Condition, c resources.Condition) []resources.Condition {
	for i := range cs {
		if cs[i].Type == c.Type {
			cs[i] = c
			return cs
		}
	}
	return append(cs, c)
}
