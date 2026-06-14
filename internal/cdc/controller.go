package cdc

// =====================================================
// P1.2 — CDC Pipeline controller
//
// Reconciles `CDCPipelineResource` -> `*CDCPipeline` inside `PipelineEngine`.
// Uses the shared workqueue for rate-limited retries.
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

// CDCPipelineStore is a minimal persistence boundary.
type CDCPipelineStore interface {
	GetCDCPipeline(ctx context.Context, key string) (*CDCPipelineResource, error)
	UpdateCDCPipelineStatus(ctx context.Context, key string, status CDCPipelineResourceStatus) error
}

// CDCPipelineController reconciles CDCPipelineResource into PipelineEngine.
type CDCPipelineController struct {
	engine *PipelineEngine
	store  CDCPipelineStore
	queue  workqueue.WorkQueue

	mu       sync.Mutex
	workers  int
	running  bool
	cancel   context.CancelFunc
	finished chan struct{}
}

// NewCDCPipelineController builds a controller for the given engine.
func NewCDCPipelineController(engine *PipelineEngine, store CDCPipelineStore) *CDCPipelineController {
	return &CDCPipelineController{
		engine:  engine,
		store:   store,
		queue:   workqueue.NewSimpleQueue(nil),
		workers: 2,
	}
}

// Enqueue triggers a reconcile for the given key.
func (c *CDCPipelineController) Enqueue(key string) { _ = c.queue.Add(key) }

// Reconcile implements `reconciler.Reconciler`.
func (c *CDCPipelineController) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	pr, ok := obj.(*CDCPipelineResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("CDCPipelineController: unexpected type %T", obj)}
	}
	if c.engine == nil {
		return reconciler.ReconcileResult{Error: fmt.Errorf("CDCPipelineController: engine is nil")}
	}

	desired := ToCDCPipeline(pr)

	c.engine.mu.Lock()
	existing, exists := c.engine.pipelines[desired.ID]
	if !exists {
		desired.CreatedAt = time.Now()
		c.engine.pipelines[desired.ID] = desired
	} else {
		existing.Description = desired.Description
		existing.Source = desired.Source
		existing.Sink = desired.Sink
		existing.Filters = desired.Filters
		existing.Config = desired.Config
		existing.Tags = desired.Tags
		existing.UpdatedAt = time.Now()
		if pr.Spec.Paused && existing.Status == CDCActive {
			existing.Status = CDCPaused
		}
	}
	snapshot := *c.engine.pipelines[desired.ID]
	c.engine.mu.Unlock()

	status := pr.Status
	status.ObservedGeneration = pr.Generation
	status.CDCStatus = snapshot.Status
	status.EventCount = snapshot.EventCount
	status.ErrorCount = snapshot.ErrorCount
	status.LastEventAt = snapshot.LastEventAt
	status.Lag = snapshot.Lag
	status.Phase = string(snapshot.Status)
	status.LastTransitionTime = time.Now()
	status.Conditions = upsertCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: "True",
		Reason: "Reconciled", Message: "cdc pipeline spec applied",
		LastTransitionTime: time.Now(),
	})

	if c.store != nil {
		if err := c.store.UpdateCDCPipelineStatus(ctx, pr.GetKey(), status); err != nil {
			return reconciler.ReconcileResult{
				Requeue: true, RequeueAfter: 5 * time.Second,
				Error: fmt.Errorf("UpdateCDCPipelineStatus: %w", err),
			}
		}
	}
	pr.Status = status
	return reconciler.ReconcileResult{}
}

// Start launches worker loops.
func (c *CDCPipelineController) Start(ctx context.Context) {
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
		pr, err := c.store.GetCDCPipeline(ctx, item.Key)
		if err != nil {
			return err
		}
		if pr == nil {
			return nil
		}
		res := c.Reconcile(ctx, pr)
		return res.Error
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := workqueue.NewWorker(c.queue, process, 10)
			if err := w.Run(wctx); err != nil {
				logging.Z().Info(fmt.Sprintf("[cdc] pipeline worker exited: %v", err))
			}
		}()
	}
	go func() { wg.Wait(); close(c.finished) }()
}

// Stop drains the controller.
func (c *CDCPipelineController) Stop() {
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
