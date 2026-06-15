package jobs

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemoryQueue implements Queue interface using in-memory storage
type MemoryQueue struct {
	mu      sync.RWMutex
	jobs    map[string]*Job
	pending []*Job
	maxSize int
	closed  bool
}

// NewMemoryQueue creates a new in-memory queue
func NewMemoryQueue(maxSize int) *MemoryQueue {
	if maxSize <= 0 {
		maxSize = 10000
	}

	return &MemoryQueue{
		jobs:    make(map[string]*Job),
		pending: make([]*Job, 0),
		maxSize: maxSize,
	}
}

// Submit adds a job to the queue
func (mq *MemoryQueue) Submit(ctx context.Context, job *Job) error {
	if job == nil {
		return ErrInvalidJob
	}

	if err := JobValidator(job); err != nil {
		return err
	}

	mq.mu.Lock()
	defer mq.mu.Unlock()

	if len(mq.jobs) >= mq.maxSize {
		return ErrQueueFull
	}

	job.CreatedAt = time.Now()
	job.Status = JobStatusPending
	mq.jobs[job.ID] = job
	mq.pending = append(mq.pending, job)

	// Sort by priority (higher first)
	sort.Slice(mq.pending, func(i, j int) bool {
		return mq.pending[i].Priority > mq.pending[j].Priority
	})

	logging.Z().Info(fmt.Sprintf("Job submitted: %s (id: %s, priority: %d)", job.Type, job.ID, job.Priority))
	return nil
}

// Dequeue retrieves the next job from the queue
func (mq *MemoryQueue) Dequeue(ctx context.Context) (*Job, error) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if len(mq.pending) == 0 {
		return nil, ErrJobNotFound
	}

	// Pop first job (highest priority)
	job := mq.pending[0]
	mq.pending = mq.pending[1:]

	// Check if job is expired
	if job.IsExpired() {
		job.Status = JobStatusCancelled
		logging.Z().Info(fmt.Sprintf("Job expired: %s (id: %s)", job.Type, job.ID))
		return nil, ErrJobCancelled
	}

	return job, nil
}

// Get retrieves a job by ID
func (mq *MemoryQueue) Get(ctx context.Context, jobID string) (*Job, error) {
	mq.mu.RLock()
	defer mq.mu.RUnlock()

	job, exists := mq.jobs[jobID]
	if !exists {
		return nil, ErrJobNotFound
	}

	return job, nil
}

// GetByStatus retrieves jobs by status
func (mq *MemoryQueue) GetByStatus(ctx context.Context, status JobStatus, limit int) ([]*Job, error) {
	mq.mu.RLock()
	defer mq.mu.RUnlock()

	var results []*Job
	for _, job := range mq.jobs {
		if job.Status == status {
			results = append(results, job)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// Update updates a job
func (mq *MemoryQueue) Update(ctx context.Context, job *Job) error {
	if job == nil {
		return ErrInvalidJob
	}

	mq.mu.Lock()
	defer mq.mu.Unlock()

	if _, exists := mq.jobs[job.ID]; !exists {
		return ErrJobNotFound
	}

	mq.jobs[job.ID] = job
	return nil
}

// Delete removes a job
func (mq *MemoryQueue) Delete(ctx context.Context, jobID string) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if _, exists := mq.jobs[jobID]; !exists {
		return ErrJobNotFound
	}

	delete(mq.jobs, jobID)

	// Remove from pending queue
	for i, job := range mq.pending {
		if job.ID == jobID {
			mq.pending = append(mq.pending[:i], mq.pending[i+1:]...)
			break
		}
	}

	return nil
}

// Clear removes all jobs
func (mq *MemoryQueue) Clear(ctx context.Context) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	mq.jobs = make(map[string]*Job)
	mq.pending = make([]*Job, 0)

	logging.Z().Info(fmt.Sprintf("Queue cleared"))
	return nil
}

// GetStats returns queue statistics
func (mq *MemoryQueue) GetStats(ctx context.Context) (*QueueStats, error) {
	mq.mu.RLock()
	defer mq.mu.RUnlock()

	stats := &QueueStats{
		Total: int64(len(mq.jobs)),
	}

	var oldestTime time.Time
	for _, job := range mq.jobs {
		switch job.Status {
		case JobStatusPending:
			stats.Pending++
		case JobStatusRunning:
			stats.Running++
		case JobStatusCompleted:
			stats.Completed++
		case JobStatusFailed:
			stats.Failed++
		case JobStatusCancelled:
			stats.Cancelled++
		}

		if oldestTime.IsZero() || job.CreatedAt.Before(oldestTime) {
			oldestTime = job.CreatedAt
			stats.OldestJob = job
		}
	}

	if stats.Total > 0 {
		stats.AverageTime = time.Duration(int64(time.Since(oldestTime)) / stats.Total)
	}

	return stats, nil
}

// Processor processes jobs from the queue
type MemoryProcessor struct {
	queue      Queue
	handlers   map[JobType]JobHandler
	workers    int
	running    bool
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	stats      *ProcessorStats
	jobLogger  *JobLogger
	resultChan chan *JobResult
}

// NewMemoryProcessor creates a new job processor
func NewMemoryProcessor(queue Queue) *MemoryProcessor {
	return &MemoryProcessor{
		queue:      queue,
		handlers:   make(map[JobType]JobHandler),
		running:    false,
		stats:      &ProcessorStats{},
		jobLogger:  NewJobLogger(),
		resultChan: make(chan *JobResult, 100),
	}
}

// Register registers a job handler
func (mp *MemoryProcessor) Register(jobType JobType, handler JobHandler) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	mp.handlers[jobType] = handler
	logging.Z().Info(fmt.Sprintf("Handler registered for job type: %s", jobType))
}

// Start starts processing jobs
func (mp *MemoryProcessor) Start(ctx context.Context, numWorkers int) error {
	mp.mu.Lock()
	if mp.running {
		mp.mu.Unlock()
		return fmt.Errorf("processor already running")
	}

	mp.running = true
	mp.workers = numWorkers
	mp.stats.WorkersTotal = numWorkers
	mp.ctx, mp.cancel = context.WithCancel(ctx)
	mp.mu.Unlock()

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		mp.wg.Add(1)
		go mp.worker(i)
	}

	logging.Z().Info(fmt.Sprintf("Processor started with %d workers", numWorkers))
	return nil
}

// worker processes jobs from the queue
func (mp *MemoryProcessor) worker(id int) {
	defer mp.wg.Done()

	for {
		select {
		case <-mp.ctx.Done():
			logging.Z().Info(fmt.Sprintf("Worker %d stopping", id))
			return
		default:
		}

		// Get next job
		mq := mp.queue.(*MemoryQueue)
		job, err := mq.Dequeue(mp.ctx)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Process job
		mp.processJob(job)
	}
}

// processJob processes a single job
func (mp *MemoryProcessor) processJob(job *Job) {
	startTime := time.Now()
	mp.jobLogger.LogJobStart(job)

	mp.mu.Lock()
	mp.stats.WorkersActive++
	mp.mu.Unlock()

	job.Status = JobStatusRunning
	job.StartedAt = time.Now()
	mp.queue.Update(mp.ctx, job)

	// Get handler
	mp.mu.RLock()
	handler, exists := mp.handlers[job.Type]
	mp.mu.RUnlock()

	if !exists {
		job.Status = JobStatusFailed
		job.Error = fmt.Sprintf("no handler for job type: %s", job.Type)
		job.CompletedAt = time.Now()
		mp.queue.Update(mp.ctx, job)

		mp.mu.Lock()
		mp.stats.JobsFailed++
		mp.stats.WorkersActive--
		mp.mu.Unlock()

		mp.jobLogger.LogJobFailed(job, fmt.Errorf(job.Error))
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(mp.ctx, job.Timeout)
	defer cancel()

	// Execute handler
	err := handler(ctx, job)
	duration := time.Since(startTime)

	if err != nil {
		job.Retries++
		if job.Retries < job.MaxRetries {
			job.Status = JobStatusRetrying
			mp.jobLogger.LogJobRetry(job)
			mp.queue.Update(mp.ctx, job)

			// Re-queue for retry
			mp.queue.Submit(mp.ctx, job)
		} else {
			job.Status = JobStatusFailed
			job.Error = err.Error()
			job.CompletedAt = time.Now()
			mp.queue.Update(mp.ctx, job)

			mp.mu.Lock()
			mp.stats.JobsFailed++
			mp.mu.Unlock()

			mp.jobLogger.LogJobFailed(job, err)
		}
	} else {
		job.Status = JobStatusCompleted
		job.CompletedAt = time.Now()
		mp.queue.Update(mp.ctx, job)

		mp.mu.Lock()
		mp.stats.JobsSucceeded++
		mp.mu.Unlock()

		mp.jobLogger.LogJobComplete(job, duration)
	}

	mp.mu.Lock()
	mp.stats.JobsProcessed++
	mp.stats.WorkersActive--
	if mp.stats.JobsProcessed > 0 {
		mp.stats.AverageJobTime = time.Duration(
			int64(mp.stats.AverageJobTime)*(int64(mp.stats.JobsProcessed)-1)+int64(duration),
		) / time.Duration(mp.stats.JobsProcessed)
	} else {
		mp.stats.AverageJobTime = duration
	}

	if mp.stats.JobsProcessed > 0 {
		mp.stats.SuccessRate = float64(mp.stats.JobsSucceeded) / float64(mp.stats.JobsProcessed)
	}
	mp.mu.Unlock()

	// Send result
	select {
	case mp.resultChan <- &JobResult{
		JobID:      job.ID,
		Status:     job.Status,
		Result:     job.Result,
		Error:      job.Error,
		Duration:   duration,
		RetryCount: job.Retries,
		Timestamp:  time.Now(),
	}:
	default:
		// Channel full, ignore
	}
}

// Stop stops processing
func (mp *MemoryProcessor) Stop() error {
	mp.mu.Lock()
	if !mp.running {
		mp.mu.Unlock()
		return fmt.Errorf("processor not running")
	}

	mp.running = false
	mp.mu.Unlock()

	mp.cancel()
	mp.wg.Wait()

	logging.Z().Info(fmt.Sprintf("Processor stopped"))
	return nil
}

// IsRunning returns if processor is running
func (mp *MemoryProcessor) IsRunning() bool {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	return mp.running
}

// Process handles job execution directly
func (mp *MemoryProcessor) Process(ctx context.Context, job *Job) error {
	mp.mu.RLock()
	handler, exists := mp.handlers[job.Type]
	mp.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no handler for job type: %s", job.Type)
	}

	return handler(ctx, job)
}

// GetStats returns processor statistics
func (mp *MemoryProcessor) GetStats() *ProcessorStats {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	statsCopy := *mp.stats
	return &statsCopy
}

// GetResults returns a channel for job results
func (mp *MemoryProcessor) GetResults() <-chan *JobResult {
	return mp.resultChan
}
