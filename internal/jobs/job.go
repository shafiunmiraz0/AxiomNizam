package jobs

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/jobs/config"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"errors"
	"time"
)

// JobStatus, JobPriority, JobType and their constants are defined in
// jobs/models and re-exported via jobs/resource.go type aliases.

// Job represents a background job
type Job struct {
	ID          string                 `json:"id"`
	Type        JobType                `json:"type"`
	Status      JobStatus              `json:"status"`
	Priority    JobPriority            `json:"priority"`
	Data        map[string]interface{} `json:"data"`
	Result      map[string]interface{} `json:"result"`
	Error       string                 `json:"error,omitempty"`
	Retries     int                    `json:"retries"`
	MaxRetries  int                    `json:"max_retries"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   time.Time              `json:"started_at,omitempty"`
	CompletedAt time.Time              `json:"completed_at,omitempty"`
	Timeout     time.Duration          `json:"timeout"`
	Tags        []string               `json:"tags"`
	CallbackURL string                 `json:"callback_url,omitempty"`
	DeadlineAt  time.Time              `json:"deadline_at,omitempty"`
}

// JobHandler is a function that processes a job
type JobHandler func(ctx context.Context, job *Job) error

// Queue defines the interface for job queues
type Queue interface {
	// Submit adds a job to the queue
	Submit(ctx context.Context, job *Job) error

	// Get retrieves a job by ID
	Get(ctx context.Context, jobID string) (*Job, error)

	// GetByStatus retrieves jobs by status
	GetByStatus(ctx context.Context, status JobStatus, limit int) ([]*Job, error)

	// Update updates a job
	Update(ctx context.Context, job *Job) error

	// Delete removes a job
	Delete(ctx context.Context, jobID string) error

	// Clear removes all jobs (use with caution)
	Clear(ctx context.Context) error

	// GetStats returns queue statistics
	GetStats(ctx context.Context) (*QueueStats, error)
}

// QueueStats contains queue statistics
type QueueStats struct {
	Total       int64
	Pending     int64
	Running     int64
	Completed   int64
	Failed      int64
	Cancelled   int64
	AverageTime time.Duration
	OldestJob   *Job
}

// Processor defines a job processor
type Processor interface {
	// Process handles job execution
	Process(ctx context.Context, job *Job) error

	// Register registers a job handler
	Register(jobType JobType, handler JobHandler)

	// Start starts processing jobs
	Start(ctx context.Context, numWorkers int) error

	// Stop stops processing
	Stop() error

	// IsRunning returns if processor is running
	IsRunning() bool

	// GetStats returns processor statistics
	GetStats() *ProcessorStats
}

// ProcessorStats contains processor statistics
type ProcessorStats struct {
	WorkersActive  int
	WorkersTotal   int
	JobsProcessed  int64
	JobsSucceeded  int64
	JobsFailed     int64
	ProcessingTime time.Duration
	AverageJobTime time.Duration
	SuccessRate    float64
}

// Scheduler defines a job scheduler for recurring jobs
type Scheduler interface {
	// Schedule schedules a recurring job
	Schedule(jobType JobType, cron string, data map[string]interface{}) error

	// Unschedule removes a scheduled job
	Unschedule(jobType JobType, cron string) error

	// Start starts the scheduler
	Start(ctx context.Context, queue Queue) error

	// Stop stops the scheduler
	Stop() error

	// ListScheduled lists all scheduled jobs
	ListScheduled() []ScheduledJob
}

// ScheduledJob represents a scheduled recurring job
type ScheduledJob struct {
	ID       string
	Type     JobType
	CronExpr string
	Data     map[string]interface{}
	LastRun  time.Time
	NextRun  time.Time
	Enabled  bool
}

// Common errors
var (
	ErrJobNotFound    = errors.New("job not found")
	ErrInvalidJob     = errors.New("invalid job")
	ErrQueueFull      = errors.New("queue is full")
	ErrProcessorBusy  = errors.New("processor is busy")
	ErrInvalidJobType = errors.New("invalid job type")
	ErrJobCancelled   = errors.New("job cancelled")
	ErrJobTimeout     = errors.New("job timeout")
)

// JobConfig re-exports the config type from the config subpackage.
type JobConfig = config.Config

// defaultCfg holds the package-level default configuration.
var defaultCfg = config.DefaultConfig()

// DefaultJobConfig returns a default configuration
func DefaultJobConfig() *JobConfig {
	cfg := defaultCfg
	return &cfg
}

// SetDefaultConfig updates the package-level default configuration.
func SetDefaultConfig(cfg JobConfig) {
	defaultCfg = cfg
}

// JobResult represents the result of a job execution
type JobResult struct {
	JobID      string
	Status     JobStatus
	Result     map[string]interface{}
	Error      string
	Duration   time.Duration
	RetryCount int
	Timestamp  time.Time
}

// JobLogger provides logging for jobs
type JobLogger struct {
}

// NewJobLogger creates a new job logger
func NewJobLogger() *JobLogger {
	return &JobLogger{
	}
}

// LogJobStart logs job start
func (jl *JobLogger) LogJobStart(job *Job) {
	logging.Z().Info(fmt.Sprintf("Job started: %s (type: %s, id: %s)", job.Type, job.Type, job.ID))
}

// LogJobComplete logs job completion
func (jl *JobLogger) LogJobComplete(job *Job, duration time.Duration) {
	logging.Z().Info(fmt.Sprintf("Job completed: %s (duration: %s, id: %s)", job.Type, duration, job.ID))
}

// LogJobFailed logs job failure
func (jl *JobLogger) LogJobFailed(job *Job, err error) {
	logging.Z().Info(fmt.Sprintf("Job failed: %s (error: %v, id: %s, retries: %d/%d)", job.Type, err, job.ID, job.Retries, job.MaxRetries))
}

// LogJobRetry logs job retry
func (jl *JobLogger) LogJobRetry(job *Job) {
	logging.Z().Info(fmt.Sprintf("Job retrying: %s (attempt: %d/%d, id: %s)", job.Type, job.Retries+1, job.MaxRetries, job.ID))
}

// CreateJob creates a new job using the package-level default config.
func CreateJob(jobType JobType, data map[string]interface{}) *Job {
	return &Job{
		ID:         generateJobID(),
		Type:       jobType,
		Status:     JobStatusPending,
		Priority:   JobPriority(defaultCfg.DefaultPriority),
		Data:       data,
		Result:     make(map[string]interface{}),
		Retries:    0,
		MaxRetries: defaultCfg.MaxRetries,
		CreatedAt:  time.Now(),
		Timeout:    defaultCfg.DefaultTimeout,
		Tags:       []string{},
	}
}

// CreateJobWithPriority creates a job with specific priority
func CreateJobWithPriority(jobType JobType, data map[string]interface{}, priority JobPriority) *Job {
	job := CreateJob(jobType, data)
	job.Priority = priority
	return job
}

// SetCallback sets a callback URL for job completion
func (j *Job) SetCallback(url string) {
	j.CallbackURL = url
}

// SetDeadline sets a deadline for the job
func (j *Job) SetDeadline(deadline time.Time) {
	j.DeadlineAt = deadline
}

// AddTag adds a tag to the job
func (j *Job) AddTag(tag string) {
	j.Tags = append(j.Tags, tag)
}

// HasTag checks if job has a tag
func (j *Job) HasTag(tag string) bool {
	for _, t := range j.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// IsExpired checks if job has exceeded deadline
func (j *Job) IsExpired() bool {
	if j.DeadlineAt.IsZero() {
		return false
	}
	return time.Now().After(j.DeadlineAt)
}

// IsTimedOut checks if job has timed out
func (j *Job) IsTimedOut(startTime time.Time) bool {
	if j.Timeout == 0 {
		return false
	}
	return time.Since(startTime) > j.Timeout
}

// generateJobID generates a unique job ID
func generateJobID() string {
	return "job_" + time.Now().Format("20060102150405") + "_" + randString(8)
}

// randString generates a random string
func randString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// JobValidator validates a job
func JobValidator(job *Job) error {
	if job == nil {
		return ErrInvalidJob
	}
	if job.Type == "" {
		return ErrInvalidJobType
	}
	if job.MaxRetries < 0 {
		return errors.New("max retries cannot be negative")
	}
	if job.Timeout < 0 {
		return errors.New("timeout cannot be negative")
	}
	return nil
}
