package jobs

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"sync"
	"time"
)

// DeadLetterJob represents a job that failed and moved to DLQ
type DeadLetterJob struct {
	ID            string                 `json:"id"`
	OriginalJobID string                 `json:"original_job_id"`
	OriginalType  JobType                `json:"original_type"`
	Data          map[string]interface{} `json:"data"`
	Error         string                 `json:"error"`
	FailureCount  int                    `json:"failure_count"`
	LastFailedAt  time.Time              `json:"last_failed_at"`
	MovedToDLQAt  time.Time              `json:"moved_to_dlq_at"`
	ExpiresAt     time.Time              `json:"expires_at"`
	Retryable     bool                   `json:"retryable"`
	ManualReview  bool                   `json:"manual_review"`
	Notes         string                 `json:"notes,omitempty"`
}

// DeadLetterQueue stores failed jobs for analysis and recovery
type DeadLetterQueue struct {
	mu         sync.RWMutex
	jobs       map[string]*DeadLetterJob
	maxSize    int
	retention  time.Duration
	repository JobRepository
}

// NewDeadLetterQueue creates a new dead letter queue
func NewDeadLetterQueue(maxSize int, retention time.Duration) *DeadLetterQueue {
	if maxSize <= 0 {
		maxSize = 10000
	}
	if retention == 0 {
		retention = 30 * 24 * time.Hour // 30 days
	}

	dlq := &DeadLetterQueue{
		jobs:      make(map[string]*DeadLetterJob),
		maxSize:   maxSize,
		retention: retention,
	}

	// Start background cleanup
	go dlq.cleanupExpired()

	return dlq
}

// MoveToDeadLetter moves a failed job to DLQ
func (dlq *DeadLetterQueue) MoveToDeadLetter(job *Job, manualReview bool) error {
	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	if len(dlq.jobs) >= dlq.maxSize {
		return fmt.Errorf("dead letter queue is full")
	}

	dlqJob := &DeadLetterJob{
		ID:            generateJobID(),
		OriginalJobID: job.ID,
		OriginalType:  job.Type,
		Data:          job.Data,
		Error:         job.Error,
		FailureCount:  job.Retries,
		LastFailedAt:  time.Now(),
		MovedToDLQAt:  time.Now(),
		ExpiresAt:     time.Now().Add(dlq.retention),
		Retryable:     job.Retries < job.MaxRetries,
		ManualReview:  manualReview,
	}

	dlq.jobs[dlqJob.ID] = dlqJob
	logging.Z().Info(fmt.Sprintf("Job moved to DLQ: %s (original: %s)", dlqJob.ID, job.ID))

	return nil
}

// GetDeadLetterJob retrieves a DLQ job
func (dlq *DeadLetterQueue) GetDeadLetterJob(dlqJobID string) (*DeadLetterJob, error) {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	job, exists := dlq.jobs[dlqJobID]
	if !exists {
		return nil, fmt.Errorf("DLQ job not found: %s", dlqJobID)
	}

	return job, nil
}

// ListDeadLetterJobs lists all DLQ jobs with optional filtering
func (dlq *DeadLetterQueue) ListDeadLetterJobs(limit int, jobType JobType) []*DeadLetterJob {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	result := make([]*DeadLetterJob, 0)
	count := 0

	for _, dlqJob := range dlq.jobs {
		if count >= limit {
			break
		}

		if jobType != "" && dlqJob.OriginalType != jobType {
			continue
		}

		result = append(result, dlqJob)
		count++
	}

	return result
}

// RetryDeadLetterJob retries a DLQ job
func (dlq *DeadLetterQueue) RetryDeadLetterJob(ctx context.Context, dlqJobID string, queue Queue) error {
	dlq.mu.Lock()
	dlqJob, exists := dlq.jobs[dlqJobID]
	dlq.mu.Unlock()

	if !exists {
		return fmt.Errorf("DLQ job not found: %s", dlqJobID)
	}

	if !dlqJob.Retryable {
		return fmt.Errorf("job not retryable: %s", dlqJobID)
	}

	// Create new job from DLQ data
	newJob := CreateJob(dlqJob.OriginalType, dlqJob.Data)
	newJob.AddTag("dlq-retry")
	newJob.AddTag(fmt.Sprintf("dlq-%s", dlqJobID))

	if err := queue.Submit(ctx, newJob); err != nil {
		return err
	}

	logging.Z().Info(fmt.Sprintf("DLQ job retried: %s -> %s", dlqJobID, newJob.ID))
	return nil
}

// UpdateDLQJobNotes adds notes to a DLQ job
func (dlq *DeadLetterQueue) UpdateDLQJobNotes(dlqJobID string, notes string) error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	job, exists := dlq.jobs[dlqJobID]
	if !exists {
		return fmt.Errorf("DLQ job not found: %s", dlqJobID)
	}

	job.Notes = notes
	return nil
}

// DeleteDeadLetterJob removes a job from DLQ
func (dlq *DeadLetterQueue) DeleteDeadLetterJob(dlqJobID string) error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	delete(dlq.jobs, dlqJobID)
	logging.Z().Info(fmt.Sprintf("DLQ job deleted: %s", dlqJobID))
	return nil
}

// GetDLQStats returns DLQ statistics
func (dlq *DeadLetterQueue) GetDLQStats() map[string]interface{} {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	stats := map[string]interface{}{
		"total_jobs":     len(dlq.jobs),
		"retryable_jobs": 0,
		"manual_review":  0,
		"jobs_by_type":   make(map[string]int),
	}

	retryableCount := 0
	manualReviewCount := 0
	typeCount := make(map[string]int)

	for _, job := range dlq.jobs {
		if job.Retryable {
			retryableCount++
		}
		if job.ManualReview {
			manualReviewCount++
		}
		typeCount[string(job.OriginalType)]++
	}

	stats["retryable_jobs"] = retryableCount
	stats["manual_review"] = manualReviewCount
	stats["jobs_by_type"] = typeCount

	return stats
}

// cleanupExpired removes expired jobs from DLQ
func (dlq *DeadLetterQueue) cleanupExpired() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		dlq.mu.Lock()

		now := time.Now()
		deleted := 0

		for id, job := range dlq.jobs {
			if job.ExpiresAt.Before(now) {
				delete(dlq.jobs, id)
				deleted++
			}
		}

		dlq.mu.Unlock()

		if deleted > 0 {
			logging.Z().Info(fmt.Sprintf("Cleaned up %d expired DLQ jobs", deleted))
		}
	}
}

// DLQProcessor processes dead letter queue jobs
type DLQProcessor struct {
	dlq     *DeadLetterQueue
	queue   Queue
	manager JobManager
	handler func(ctx context.Context, dlqJob *DeadLetterJob) error
}

// NewDLQProcessor creates a new DLQ processor
func NewDLQProcessor(dlq *DeadLetterQueue, queue Queue, manager JobManager) *DLQProcessor {
	return &DLQProcessor{
		dlq:     dlq,
		queue:   queue,
		manager: manager,
	}
}

// SetHandler sets custom DLQ handler
func (dp *DLQProcessor) SetHandler(handler func(ctx context.Context, dlqJob *DeadLetterJob) error) {
	dp.handler = handler
}

// ProcessJob processes a single DLQ job
func (dp *DLQProcessor) ProcessJob(ctx context.Context, dlqJobID string) error {
	dlqJob, err := dp.dlq.GetDeadLetterJob(dlqJobID)
	if err != nil {
		return err
	}

	// Use custom handler if provided
	if dp.handler != nil {
		return dp.handler(ctx, dlqJob)
	}

	// Default: retry if retryable
	if dlqJob.Retryable {
		return dp.dlq.RetryDeadLetterJob(ctx, dlqJobID, dp.queue)
	}

	return fmt.Errorf("job not retryable and no handler provided")
}

// ProcessAll processes all retryable jobs in DLQ
func (dp *DLQProcessor) ProcessAll(ctx context.Context) (int, error) {
	dlqJobs := dp.dlq.ListDeadLetterJobs(1000, "")

	processed := 0
	for _, dlqJob := range dlqJobs {
		if dlqJob.Retryable {
			if err := dp.ProcessJob(ctx, dlqJob.ID); err != nil {
				logging.Z().Info(fmt.Sprintf("Error processing DLQ job: %v", err))
				continue
			}
			processed++
		}
	}

	logging.Z().Info(fmt.Sprintf("Processed %d DLQ jobs", processed))
	return processed, nil
}

// DLQAnalyzer analyzes DLQ patterns
type DLQAnalyzer struct {
	dlq    *DeadLetterQueue
}

// NewDLQAnalyzer creates a new DLQ analyzer
func NewDLQAnalyzer(dlq *DeadLetterQueue) *DLQAnalyzer {
	return &DLQAnalyzer{
		dlq:    dlq,
	}
}

// AnalyzePatterns analyzes failure patterns in DLQ
func (da *DLQAnalyzer) AnalyzePatterns() map[string]interface{} {
	da.dlq.mu.RLock()
	defer da.dlq.mu.RUnlock()

	patterns := map[string]interface{}{
		"total_jobs":         len(da.dlq.jobs),
		"error_distribution": make(map[string]int),
		"type_distribution":  make(map[string]int),
		"avg_retries":        0,
		"oldest_job":         nil,
		"newest_job":         nil,
	}

	if len(da.dlq.jobs) == 0 {
		return patterns
	}

	errorDist := make(map[string]int)
	typeDist := make(map[string]int)
	totalRetries := 0
	var oldestJob *DeadLetterJob
	var newestJob *DeadLetterJob

	for _, job := range da.dlq.jobs {
		// Error distribution
		errorDist[job.Error]++

		// Type distribution
		typeDist[string(job.OriginalType)]++

		// Average retries
		totalRetries += job.FailureCount

		// Oldest and newest
		if oldestJob == nil || job.MovedToDLQAt.Before(oldestJob.MovedToDLQAt) {
			oldestJob = job
		}
		if newestJob == nil || job.MovedToDLQAt.After(newestJob.MovedToDLQAt) {
			newestJob = job
		}
	}

	patterns["error_distribution"] = errorDist
	patterns["type_distribution"] = typeDist
	patterns["avg_retries"] = totalRetries / len(da.dlq.jobs)
	patterns["oldest_job"] = oldestJob
	patterns["newest_job"] = newestJob

	return patterns
}

// GetTopErrors returns the most common errors
func (da *DLQAnalyzer) GetTopErrors(limit int) []map[string]interface{} {
	patterns := da.AnalyzePatterns()
	errorDist := patterns["error_distribution"].(map[string]int)

	type errorCount struct {
		error string
		count int
	}

	errors := make([]errorCount, 0)
	for err, count := range errorDist {
		errors = append(errors, errorCount{err, count})
	}

	// Sort by count
	for i := 0; i < len(errors); i++ {
		for j := i + 1; j < len(errors); j++ {
			if errors[j].count > errors[i].count {
				errors[i], errors[j] = errors[j], errors[i]
			}
		}
	}

	result := make([]map[string]interface{}, 0)
	for i := 0; i < limit && i < len(errors); i++ {
		result = append(result, map[string]interface{}{
			"error": errors[i].error,
			"count": errors[i].count,
		})
	}

	return result
}
