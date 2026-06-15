// Package controller provides GenericController[T] — a reusable
// controller that watches an EtcdStore for changes, enqueues resource
// keys into a work queue, and drives a reconciler in worker goroutines.
//
// This is Phase 1 of the migration plan. When RECONCILER_SHADOW_MODE
// is true (default), reconcilers run the full cycle but the controller
// logs that it's operating in shadow mode. When shadow mode is off,
// reconcilers drive the actual managers.
package controller

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"sync"

	"axiomnizam.bitbd.net/axiomnizam/internal/metrics"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/workqueue"
)

// GenericController watches a store and reconciles resources.
type GenericController[T store.Resource] struct {
	name       string
	store      store.ResourceStore[T]
	reconciler reconciler.Reconciler
	queue      *workqueue.SimpleQueue
	workers    int
	shadowMode bool
	metrics    *metrics.ReconcilerMetrics

	mu      sync.Mutex
	running bool
}

// NewGenericController creates a controller for a single resource kind.
//
//   - name:       human-readable module name (e.g. "bulk", "rbac-role")
//   - store:      EtcdStore[T] that holds the resources
//   - reconciler: the reconciler to call (should be InstrumentedReconciler)
//   - workers:    number of concurrent worker goroutines
//   - shadowMode: when true, controller runs but logs shadow-mode status
//   - m:          per-module metrics recorder
func NewGenericController[T store.Resource](
	name string,
	s store.ResourceStore[T],
	r reconciler.Reconciler,
	workers int,
	shadowMode bool,
	m *metrics.ReconcilerMetrics,
) *GenericController[T] {
	if workers < 1 {
		workers = 1
	}
	return &GenericController[T]{
		name:       name,
		store:      s,
		reconciler: r,
		queue:      workqueue.NewSimpleQueue(workqueue.DefaultControllerRateLimiter()),
		workers:    workers,
		shadowMode: shadowMode,
		metrics:    m,
	}
}

// Start launches the watch loop and worker goroutines. Blocks until
// ctx is cancelled. Safe to call from a goroutine.
func (gc *GenericController[T]) Start(ctx context.Context) error {
	gc.mu.Lock()
	if gc.running {
		gc.mu.Unlock()
		return fmt.Errorf("controller %s: already running", gc.name)
	}
	gc.running = true
	gc.mu.Unlock()

	if gc.metrics != nil {
		gc.metrics.SetRunning(gc.name, true)
		gc.metrics.SetShadowMode(gc.name, gc.shadowMode)
	}

	mode := "live"
	if gc.shadowMode {
		mode = "shadow"
	}
	logging.Z().Info(fmt.Sprintf("controller %s: starting (%d workers, mode=%s)", gc.name, gc.workers, mode))

	// Start the watch loop that feeds the queue.
	go gc.watchLoop(ctx)

	// Also do an initial list to seed the queue with existing resources.
	go gc.initialSync(ctx)

	// Start worker goroutines.
	var wg sync.WaitGroup
	for i := 0; i < gc.workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			gc.worker(ctx, workerID)
		}(i)
	}

	// Block until context is cancelled.
	<-ctx.Done()

	// Shutdown queue so workers unblock.
	_ = gc.queue.Shutdown()
	wg.Wait()

	gc.mu.Lock()
	gc.running = false
	gc.mu.Unlock()

	if gc.metrics != nil {
		gc.metrics.SetRunning(gc.name, false)
	}

	logging.Z().Info(fmt.Sprintf("controller %s: stopped", gc.name))
	return nil
}

// watchLoop subscribes to store watch events and enqueues keys.
func (gc *GenericController[T]) watchLoop(ctx context.Context) {
	ch, err := gc.store.Watch(ctx)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("controller %s: watch failed: %v", gc.name, err))
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			key := event.Object.GetKey()
			if key != "" {
				_ = gc.queue.Add(key)
			}
		}
	}
}

// initialSync lists all existing resources and enqueues them so the
// reconciler processes the full state on startup.
func (gc *GenericController[T]) initialSync(ctx context.Context) {
	resources, err := gc.store.List(ctx, "")
	if err != nil {
		logging.Z().Info(fmt.Sprintf("controller %s: initial sync failed: %v", gc.name, err))
		return
	}
	for _, res := range resources {
		key := res.GetKey()
		if key != "" {
			_ = gc.queue.Add(key)
		}
	}
	if len(resources) > 0 {
		logging.Z().Info(fmt.Sprintf("controller %s: initial sync enqueued %d resources", gc.name, len(resources)))
	}
}

// worker is the main processing loop for a single worker goroutine.
func (gc *GenericController[T]) worker(ctx context.Context, id int) {
	for {
		item, err := gc.queue.Get()
		if err != nil {
			// Queue shut down.
			return
		}

		gc.processItem(ctx, item.Key)
		_ = gc.queue.Done(item.Key)
	}
}

// processItem fetches the resource from the store and calls Reconcile.
func (gc *GenericController[T]) processItem(ctx context.Context, key string) {
	// Panic recovery — a crashing reconciler must not kill the worker.
	defer func() {
		if r := recover(); r != nil {
			logging.Z().Info(fmt.Sprintf("controller %s: PANIC in reconciler for key=%s: %v", gc.name, key, r))
			if gc.metrics != nil {
				gc.metrics.RecordReconcile(gc.name, 0, false, false, fmt.Sprintf("panic: %v", r))
			}
		}
	}()

	// Fetch the resource from the store.
	resource, err := gc.store.Get(ctx, key)
	if err != nil {
		// Resource may have been deleted between enqueue and dequeue.
		// This is normal — just forget the key.
		_ = gc.queue.Forget(key)
		return
	}

	// Call the reconciler.
	result := gc.reconciler.Reconcile(ctx, resource)

	// Handle requeue.
	if result.Requeue {
		if result.RequeueAfter > 0 {
			_ = gc.queue.AddAfter(key, result.RequeueAfter)
		} else {
			_ = gc.queue.AddRateLimited(key)
		}
	} else {
		_ = gc.queue.Forget(key)
	}
}

// Name returns the controller name.
func (gc *GenericController[T]) Name() string { return gc.name }

// IsRunning reports whether the controller is active.
func (gc *GenericController[T]) IsRunning() bool {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	return gc.running
}

// IsShadowMode reports whether the controller is in shadow mode.
func (gc *GenericController[T]) IsShadowMode() bool { return gc.shadowMode }

// QueueDepth returns the current work queue length.
func (gc *GenericController[T]) QueueDepth() int { return gc.queue.Len() }
