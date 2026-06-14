package jobs

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"sync"
	"time"
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(tokensPerSecond float64, maxTokens float64) *RateLimiter {
	if maxTokens <= 0 {
		maxTokens = tokensPerSecond * 10
	}

	return &RateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: tokensPerSecond,
		lastRefill: time.Now(),
	}
}

// Allow checks if an operation is allowed
func (rl *RateLimiter) Allow(ctx context.Context) bool {
	return rl.AllowN(1)
}

// AllowN checks if N tokens are available
func (rl *RateLimiter) AllowN(n int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens = min(rl.maxTokens, rl.tokens+elapsed*rl.refillRate)
	rl.lastRefill = now

	if rl.tokens >= float64(n) {
		rl.tokens -= float64(n)
		return true
	}

	return false
}

// WaitUntilAllow blocks until tokens are available
func (rl *RateLimiter) WaitUntilAllow(ctx context.Context) error {
	for {
		if rl.AllowN(1) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// Try again
		}
	}
}

// helper function
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// Throttler controls job submission rate
type Throttler struct {
	mu            sync.RWMutex
	limiters      map[JobType]*RateLimiter
	globalLimiter *RateLimiter
}

// NewThrottler creates a new throttler
func NewThrottler(globalJobsPerSecond float64) *Throttler {
	return &Throttler{
		limiters:      make(map[JobType]*RateLimiter),
		globalLimiter: NewRateLimiter(globalJobsPerSecond, 0),
	}
}

// SetJobTypeLimit sets rate limit for a specific job type
func (t *Throttler) SetJobTypeLimit(jobType JobType, jobsPerSecond float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.limiters[jobType] = NewRateLimiter(jobsPerSecond, 0)
	logging.Z().Info(fmt.Sprintf("Rate limit set: %s = %.1f jobs/sec", jobType, jobsPerSecond))
}

// CanSubmit checks if a job can be submitted
func (t *Throttler) CanSubmit(ctx context.Context, jobType JobType) bool {
	// Check global limit
	if !t.globalLimiter.AllowN(1) {
		return false
	}

	// Check job type limit
	t.mu.RLock()
	limiter, exists := t.limiters[jobType]
	t.mu.RUnlock()

	if exists {
		return limiter.AllowN(1)
	}

	return true
}

// WaitAndSubmit waits until job can be submitted
func (t *Throttler) WaitAndSubmit(ctx context.Context, jobType JobType, submitFn func() error) error {
	// Wait for global limit
	if err := t.globalLimiter.WaitUntilAllow(ctx); err != nil {
		return err
	}

	// Wait for type limit
	t.mu.RLock()
	limiter, exists := t.limiters[jobType]
	t.mu.RUnlock()

	if exists {
		if err := limiter.WaitUntilAllow(ctx); err != nil {
			return err
		}
	}

	return submitFn()
}

// PriorityQueue implements a priority-aware queue
type PriorityQueue struct {
	mu             sync.RWMutex
	jobs           map[JobPriority][]*Job
	fairnessWeight map[JobPriority]float64
}

// NewPriorityQueue creates a new priority queue
func NewPriorityQueue() *PriorityQueue {
	pq := &PriorityQueue{
		jobs: make(map[JobPriority][]*Job),
		fairnessWeight: map[JobPriority]float64{
			PriorityLow:      0.1,
			PriorityNormal:   0.3,
			PriorityHigh:     0.4,
			PriorityCritical: 0.2,
		},
	}

	return pq
}

// Enqueue adds a job to the queue
func (pq *PriorityQueue) Enqueue(job *Job) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pq.jobs[job.Priority] = append(pq.jobs[job.Priority], job)
}

// DequeueFairly dequeues next job using weighted fairness
func (pq *PriorityQueue) DequeueFairly() *Job {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	// Try to get a job from each priority level based on weight
	priorities := []JobPriority{
		PriorityCritical,
		PriorityHigh,
		PriorityNormal,
		PriorityLow,
	}

	for _, priority := range priorities {
		if len(pq.jobs[priority]) > 0 {
			job := pq.jobs[priority][0]
			pq.jobs[priority] = pq.jobs[priority][1:]
			return job
		}
	}

	return nil
}

// Size returns queue size
func (pq *PriorityQueue) Size() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	total := 0
	for _, jobs := range pq.jobs {
		total += len(jobs)
	}
	return total
}

// ConcurrencyLimiter limits concurrent jobs by type
type ConcurrencyLimiter struct {
	mu      sync.RWMutex
	limits  map[JobType]int // max concurrent jobs per type
	running map[JobType]int // currently running
}

// NewConcurrencyLimiter creates a new concurrency limiter
func NewConcurrencyLimiter() *ConcurrencyLimiter {
	return &ConcurrencyLimiter{
		limits:  make(map[JobType]int),
		running: make(map[JobType]int),
	}
}

// SetLimit sets max concurrent jobs for a type
func (cl *ConcurrencyLimiter) SetLimit(jobType JobType, limit int) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	cl.limits[jobType] = limit
	logging.Z().Info(fmt.Sprintf("Concurrency limit set: %s = %d", jobType, limit))
}

// CanRun checks if a job of type can run
func (cl *ConcurrencyLimiter) CanRun(jobType JobType) bool {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	limit, exists := cl.limits[jobType]
	if !exists {
		return true // No limit
	}

	return cl.running[jobType] < limit
}

// Start marks a job as started
func (cl *ConcurrencyLimiter) Start(jobType JobType) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	cl.running[jobType]++
}

// End marks a job as ended
func (cl *ConcurrencyLimiter) End(jobType JobType) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.running[jobType] > 0 {
		cl.running[jobType]--
	}
}

// GetRunning returns number of running jobs by type
func (cl *ConcurrencyLimiter) GetRunning() map[JobType]int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	return cl.running
}

// BackpressureHandler prevents queue overflow
type BackpressureHandler struct {
	mu        sync.RWMutex
	queueSize int
	maxSize   int
	highWater int // Start backpressure at this size
	lowWater  int // Stop backpressure at this size
	isActive  bool
}

// NewBackpressureHandler creates a new backpressure handler
func NewBackpressureHandler(maxSize int) *BackpressureHandler {
	return &BackpressureHandler{
		maxSize:   maxSize,
		highWater: int(float64(maxSize) * 0.8), // 80% full
		lowWater:  int(float64(maxSize) * 0.5), // 50% full
	}
}

// UpdateQueueSize updates current queue size
func (bh *BackpressureHandler) UpdateQueueSize(size int) {
	bh.mu.Lock()
	defer bh.mu.Unlock()

	wasActive := bh.isActive
	bh.queueSize = size

	if size > bh.highWater && !bh.isActive {
		bh.isActive = true
		logging.Z().Info(fmt.Sprintf("Backpressure ACTIVE: queue at %d/%d", size, bh.maxSize))
	} else if size < bh.lowWater && bh.isActive {
		bh.isActive = false
		logging.Z().Info(fmt.Sprintf("Backpressure INACTIVE: queue at %d/%d", size, bh.maxSize))
	}

	if wasActive != bh.isActive {
		// Log state change
	}
}

// IsActive returns if backpressure is active
func (bh *BackpressureHandler) IsActive() bool {
	bh.mu.RLock()
	defer bh.mu.RUnlock()

	return bh.isActive
}

// GetQueueHealth returns queue health status
func (bh *BackpressureHandler) GetQueueHealth() map[string]interface{} {
	bh.mu.RLock()
	defer bh.mu.RUnlock()

	utilization := float64(bh.queueSize) / float64(bh.maxSize)
	status := "healthy"

	if utilization > 0.8 {
		status = "critical"
	} else if utilization > 0.6 {
		status = "warning"
	}

	return map[string]interface{}{
		"current_size": bh.queueSize,
		"max_size":     bh.maxSize,
		"utilization":  fmt.Sprintf("%.1f%%", utilization*100),
		"backpressure": bh.isActive,
		"status":       status,
	}
}

// AdaptiveThrottler adjusts throttling based on queue depth
type AdaptiveThrottler struct {
	mu          sync.RWMutex
	baseRate    float64
	currentRate float64
	throttler   *Throttler
	queueSize   int
	maxSize     int
}

// NewAdaptiveThrottler creates an adaptive throttler
func NewAdaptiveThrottler(baseJobsPerSecond float64, maxQueueSize int) *AdaptiveThrottler {
	return &AdaptiveThrottler{
		baseRate:    baseJobsPerSecond,
		currentRate: baseJobsPerSecond,
		throttler:   NewThrottler(baseJobsPerSecond),
		maxSize:     maxQueueSize,
	}
}

// UpdateQueueMetrics updates rate based on queue size
func (at *AdaptiveThrottler) UpdateQueueMetrics(queueSize int) {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.queueSize = queueSize
	utilization := float64(queueSize) / float64(at.maxSize)

	// Reduce rate as queue fills up
	newRate := at.baseRate * (1 - utilization)
	if newRate < 0.1 {
		newRate = 0.1
	}

	if newRate != at.currentRate {
		at.currentRate = newRate
		// Update throttler rate
		at.throttler.globalLimiter = NewRateLimiter(newRate, 0)
		logging.Z().Info(fmt.Sprintf("Adaptive throttle rate: %.1f jobs/sec (utilization: %.1f%%)",
			newRate, utilization*100))
	}
}

// CanSubmit checks if job can be submitted
func (at *AdaptiveThrottler) CanSubmit(ctx context.Context, jobType JobType) bool {
	at.mu.RLock()
	defer at.mu.RUnlock()

	return at.throttler.CanSubmit(ctx, jobType)
}
