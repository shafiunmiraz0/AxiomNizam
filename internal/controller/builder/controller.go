// Package builder — Controller is the runnable produced by
// Builder.Complete.  It owns a workqueue, a pool of worker
// goroutines, and the list of watches it consumes.
//
// Controller.Start blocks until the stop channel is closed.  Each
// worker pulls a reconcile.Request off the queue, invokes the
// Reconciler, and either drops the request on success or re-enqueues
// it with rate-limiting on failure.
package builder

import (
	"context"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/controller/predicate"
	"axiomnizam.bitbd.net/axiomnizam/internal/controller/reconcile"
)

// Queue is the narrow subset of workqueue.RateLimitingInterface the
// Controller relies on.  Implementations are expected to provide
// exponential backoff via AddRateLimited.
type Queue interface {
	Add(item interface{})
	AddRateLimited(item interface{})
	Get() (item interface{}, shutdown bool)
	Done(item interface{})
	Forget(item interface{})
	ShutDown()
}

// QueueFactory is the hook used by tests to substitute a fake queue.
// Production callers set it in main(); if nil, a default in-memory
// queue is created.
var QueueFactory func(name string) Queue

// Controller is the runtime object the Manager owns.
type Controller struct {
	Name        string
	Reconciler  reconcile.Reconciler
	Watches     []watchSpec
	GlobalPreds []predicate.Predicate
	WorkerCount int

	queue   Queue
	started bool
	mu      sync.Mutex
}

// Start implements Runnable.  It creates the queue, wires up watches,
// and spins WorkerCount goroutines before blocking on stopCh.
func (c *Controller) Start(stopCh <-chan struct{}) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return fmt.Errorf("controller %q already started", c.Name)
	}
	c.started = true
	if QueueFactory != nil {
		c.queue = QueueFactory(c.Name)
	} else {
		c.queue = newSimpleQueue()
	}
	c.mu.Unlock()

	ctx, cancel := contextFromStop(stopCh)
	defer cancel()

	// Wire each Source.  Failures here are fatal — the controller
	// can't function with broken feeds.
	for i, w := range c.Watches {
		preds := w.preds
		if len(preds) == 0 {
			preds = c.GlobalPreds
		}
		if err := w.src.Start(ctx, w.handler, c.queue, preds...); err != nil {
			return fmt.Errorf("controller %q watch %d: %w", c.Name, i, err)
		}
	}

	// Spin workers.
	var wg sync.WaitGroup
	for i := 0; i < c.WorkerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.runWorker(ctx)
		}()
	}

	<-stopCh
	c.queue.ShutDown()
	wg.Wait()
	return nil
}

// runWorker is the pull loop every worker goroutine runs.  It exits
// when the queue is shut down.
func (c *Controller) runWorker(ctx context.Context) {
	for {
		item, shutdown := c.queue.Get()
		if shutdown {
			return
		}
		c.processOne(ctx, item)
	}
}

// processOne invokes the reconciler for a single queue item and
// handles the result: success → Forget; non-zero Result → re-enqueue
// (with or without backoff); error → AddRateLimited.
func (c *Controller) processOne(ctx context.Context, item interface{}) {
	defer c.queue.Done(item)
	req, ok := item.(reconcile.Request)
	if !ok {
		// Unexpected queue entry — forget so we don't loop forever.
		c.queue.Forget(item)
		return
	}
	result, err := c.Reconciler.Reconcile(ctx, req)
	switch {
	case err != nil:
		c.queue.AddRateLimited(req)
	case result.RequeueAfter > 0:
		// Sleep in a goroutine so the worker returns to the pool.
		go func(d time.Duration) {
			select {
			case <-ctx.Done():
			case <-time.After(d):
				c.queue.Add(req)
			}
		}(result.RequeueAfter)
		c.queue.Forget(req)
	case result.Requeue:
		c.queue.AddRateLimited(req)
	default:
		c.queue.Forget(req)
	}
}

// contextFromStop adapts the legacy stop-channel pattern to context.Context.
func contextFromStop(stopCh <-chan struct{}) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-stopCh
		cancel()
	}()
	return ctx, cancel
}
