package controllers

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources/apiresource"
)

// APIResourceController reconciles APIResource lifecycle
type APIResourceController struct {
	name      string
	namespace string
	store     *apiresource.Store
	queue     *apiresource.WorkQueue
	runtime   *FakeRuntime
	stopCh    chan struct{}
	done      chan struct{}
	wg        sync.WaitGroup
	mu        sync.RWMutex
	running   bool
	workers   int
	metrics   *ReconcileMetrics
}

// ReconcileMetrics tracks reconciliation statistics
type ReconcileMetrics struct {
	mu                   sync.RWMutex
	TotalReconciles      int64
	SuccessfulReconciles int64
	FailedReconciles     int64
	RequeuedReconciles   int64
	AverageTimeMs        float64
	LastReconcileTime    time.Time
	resourcesReady       int
	resourcesCreating    int
	resourcesFailed      int
}

// FakeRuntime simulates actual API server runtime
type FakeRuntime struct {
	mu sync.RWMutex
	// In real system, this would communicate with actual services
	// For now, it simulates work
}

// NewAPIResourceController creates controller for APIResource
func NewAPIResourceController(store *apiresource.Store, workers int) *APIResourceController {
	if workers == 0 {
		workers = 3
	}

	return &APIResourceController{
		name:      "apiresource-controller",
		namespace: "default",
		store:     store,
		queue:     apiresource.NewWorkQueue(5),
		runtime:   &FakeRuntime{},
		stopCh:    make(chan struct{}),
		done:      make(chan struct{}),
		workers:   workers,
		metrics: &ReconcileMetrics{
			resourcesReady:    0,
			resourcesCreating: 0,
			resourcesFailed:   0,
		},
	}
}

// Start starts the controller with worker goroutines
func (c *APIResourceController) Start(ctx context.Context) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.mu.Unlock()

	fmt.Printf("[%s] Starting with %d workers\n", c.name, c.workers)

	// Start worker goroutines
	for i := 0; i < c.workers; i++ {
		c.wg.Add(1)
		go c.worker(ctx, i)
	}
}

// Stop gracefully stops the controller
func (c *APIResourceController) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	c.mu.Unlock()

	fmt.Printf("[%s] Stopping\n", c.name)
	close(c.stopCh)
	c.wg.Wait()
	close(c.done)
}

// Enqueue adds resource to reconciliation queue
func (c *APIResourceController) Enqueue(namespace, name string) {
	key := namespace + "/" + name
	c.queue.Add(key)
	fmt.Printf("[%s] Enqueued: %s (queue len: %d)\n", c.name, key, c.queue.Len())
}

// worker processes items from the queue
func (c *APIResourceController) worker(ctx context.Context, id int) {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			fmt.Printf("[%s] Worker %d stopped\n", c.name, id)
			return
		default:
		}

		// Get next item from queue
		key, err := c.queue.Get(ctx)
		if err != nil {
			continue
		}

		startTime := time.Now()

		// Reconcile the resource
		reconcileErr := c.reconcileResource(ctx, key)

		duration := time.Since(startTime)
		c.metrics.mu.Lock()
		c.metrics.TotalReconciles++
		c.metrics.LastReconcileTime = time.Now()
		c.metrics.mu.Unlock()

		if reconcileErr != nil {
			fmt.Printf("[%s] Worker %d RECONCILE FAILED %s: %v (took %v)\n",
				c.name, id, key, reconcileErr, duration)

			c.metrics.mu.Lock()
			c.metrics.FailedReconciles++
			c.metrics.mu.Unlock()

			// Requeue with exponential backoff
			backoff := time.Duration(rand.Intn(5)+1) * time.Second
			c.queue.AddAfter(key, backoff)
			fmt.Printf("[%s] Requeuing %s after %v\n", c.name, key, backoff)
		} else {
			fmt.Printf("[%s] Worker %d RECONCILE SUCCESS %s (took %v)\n",
				c.name, id, key, duration)

			c.metrics.mu.Lock()
			c.metrics.SuccessfulReconciles++
			c.metrics.mu.Unlock()

			// Mark as done
			c.queue.Done(key)
		}
	}
}

// reconcileResource implements the core reconciliation logic
func (c *APIResourceController) reconcileResource(ctx context.Context, key string) error {
	// Parse key (namespace/name)
	parts := parseKey(key)
	if len(parts) != 2 {
		return fmt.Errorf("invalid key format: %s", key)
	}

	namespace, name := parts[0], parts[1]

	// Step 1: Fetch desired state from store
	desired, err := c.store.Get(ctx, namespace, name)
	if err != nil {
		return fmt.Errorf("failed to fetch resource: %w", err)
	}

	fmt.Printf("  → Reconciling %s (current phase: %s)\n", key, desired.Status.Phase)

	// Step 2: Compare with actual state
	// In fake runtime, actual state is simulated
	actual := c.runtime.GetActualState(ctx, namespace, name)

	// Step 3: Take action based on current phase
	switch desired.Status.Phase {
	case "Pending":
		return c.handlePending(ctx, desired, actual)
	case "Creating":
		return c.handleCreating(ctx, desired, actual)
	case "Ready":
		return c.handleReady(ctx, desired)
	case "Failed":
		return c.handleFailed(ctx, desired)
	default:
		return fmt.Errorf("unknown phase: %s", desired.Status.Phase)
	}
}

// handlePending transitions Pending -> Creating
func (c *APIResourceController) handlePending(ctx context.Context, desired *apiresource.APIResource, actual *ActualState) error {
	fmt.Printf("  → Phase: Pending → transitioning to Creating\n")

	// Mark as creating
	desired.MarkCreating("Initializing API resource")

	// Update in store
	_, err := c.store.Update(ctx, desired)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue immediately to move to next phase
	return fmt.Errorf("requeue-for-creating") // Trigger requeue
}

// handleCreating transitions Creating -> Ready
func (c *APIResourceController) handleCreating(ctx context.Context, desired *apiresource.APIResource, actual *ActualState) error {
	fmt.Printf("  → Phase: Creating → simulating work\n")

	// Simulate API creation work (1-2 seconds)
	sleepTime := time.Duration(1+rand.Intn(2)) * time.Second
	fmt.Printf("  → Simulating API creation work for %v\n", sleepTime)
	time.Sleep(sleepTime)

	// Check if work succeeded (fake: always succeeds)
	success := true
	if success {
		desired.SetReady(true)
		desired.SetPhase("Ready")
		desired.SetMessage("API resource is ready and operational")
		desired.AddCondition("Ready", "True", "Resource successfully created and running")

		// Update in store
		_, err := c.store.Update(ctx, desired)
		if err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}

		c.metrics.mu.Lock()
		c.metrics.resourcesReady++
		c.metrics.mu.Unlock()

		fmt.Printf("  → SUCCESS: Resource is now Ready\n")
		return nil
	}

	// Error case (rarely in fake runtime)
	desired.MarkFailed("Failed to create API resource")
	c.store.Update(ctx, desired)
	return fmt.Errorf("api creation failed")
}

// handleReady maintains Ready state
func (c *APIResourceController) handleReady(ctx context.Context, desired *apiresource.APIResource) error {
	fmt.Printf("  → Phase: Ready → monitoring\n")

	// In real system, would check health
	// For now, just verify it's still good
	if !desired.Status.Ready {
		desired.SetMessage("Resource health check passed")
		c.store.Update(ctx, desired)
	}

	return nil
}

// handleFailed handles failed resources
func (c *APIResourceController) handleFailed(ctx context.Context, desired *apiresource.APIResource) error {
	fmt.Printf("  → Phase: Failed → no automatic recovery\n")
	return nil
}

// GetMetrics returns controller metrics
func (c *APIResourceController) GetMetrics() map[string]interface{} {
	c.metrics.mu.RLock()
	defer c.metrics.mu.RUnlock()

	return map[string]interface{}{
		"total_reconciles":      c.metrics.TotalReconciles,
		"successful_reconciles": c.metrics.SuccessfulReconciles,
		"failed_reconciles":     c.metrics.FailedReconciles,
		"requeued_reconciles":   c.metrics.RequeuedReconciles,
		"queue_length":          c.queue.Len(),
		"processing_length":     c.queue.ProcessingLen(),
		"last_reconcile":        c.metrics.LastReconcileTime,
		"resources_ready":       c.metrics.resourcesReady,
		"resources_creating":    c.metrics.resourcesCreating,
		"resources_failed":      c.metrics.resourcesFailed,
	}
}

// ActualState represents the current runtime state
type ActualState struct {
	Exists bool
	Ready  bool
	Data   map[string]interface{}
}

// GetActualState returns the actual runtime state (fake implementation)
func (fr *FakeRuntime) GetActualState(ctx context.Context, namespace, name string) *ActualState {
	return &ActualState{
		Exists: false, // Initially doesn't exist, will be created
		Ready:  false,
		Data:   make(map[string]interface{}),
	}
}

// Helper function to parse namespace/name from key
func parseKey(key string) []string {
	parts := make([]string, 0)
	part := ""
	for _, c := range key {
		if c == '/' {
			if part != "" {
				parts = append(parts, part)
				part = ""
			}
		} else {
			part += string(c)
		}
	}
	if part != "" {
		parts = append(parts, part)
	}
	return parts
}
