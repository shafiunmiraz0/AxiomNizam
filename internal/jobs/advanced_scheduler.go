package jobs

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// AdvancedScheduler supports full cron expressions and advanced scheduling
type AdvancedScheduler struct {
	mu        sync.RWMutex
	scheduler cron.Cron
	schedules map[string]*ScheduleConfig
	queue     Queue
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	location  *time.Location
}

// ScheduleConfig represents a scheduled job configuration
type ScheduleConfig struct {
	ID            string
	JobType       JobType
	Data          map[string]interface{}
	CronExpr      string
	EntryID       cron.EntryID
	LastRun       *time.Time
	NextRun       time.Time
	RunCount      int64
	Enabled       bool
	Description   string
	Priority      JobPriority
	MaxConcurrent int
	Timeout       time.Duration
}

// NewAdvancedScheduler creates a new advanced scheduler
func NewAdvancedScheduler(location *time.Location) *AdvancedScheduler {
	if location == nil {
		location = time.UTC
	}

	return &AdvancedScheduler{
		scheduler: *cron.New(cron.WithLocation(location)),
		schedules: make(map[string]*ScheduleConfig),
		running:   false,
		location:  location,
	}
}

// Schedule schedules a job with cron expression
func (as *AdvancedScheduler) Schedule(config *ScheduleConfig) error {
	if config.ID == "" {
		config.ID = generateJobID()
	}

	// Validate cron expression
	if _, err := cron.ParseStandard(config.CronExpr); err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	// Create job function
	jobFunc := func() {
		as.submitScheduledJob(config)
	}

	// Add to scheduler
	entryID, err := as.scheduler.AddFunc(config.CronExpr, jobFunc)
	if err != nil {
		return fmt.Errorf("failed to add job: %w", err)
	}

	config.EntryID = entryID
	config.Enabled = true
	config.NextRun = time.Now().Add(time.Minute) // Approximate next run

	as.mu.Lock()
	as.schedules[config.ID] = config
	as.mu.Unlock()

	logging.Z().Info(fmt.Sprintf("Job scheduled: %s (cron: %s, next: %v)", config.ID, config.CronExpr, config.NextRun))
	return nil
}

// submitScheduledJob submits a scheduled job to the queue
func (as *AdvancedScheduler) submitScheduledJob(config *ScheduleConfig) {
	if !config.Enabled {
		return
	}

	job := CreateJobWithPriority(config.JobType, config.Data, config.Priority)
	job.Timeout = config.Timeout
	job.AddTag(fmt.Sprintf("scheduled-%s", config.ID))

	ctx := context.Background()
	if as.ctx != nil {
		ctx = as.ctx
	}

	if err := as.queue.Submit(ctx, job); err != nil {
		logging.Z().Info(fmt.Sprintf("Error submitting scheduled job %s: %v", config.ID, err))
		return
	}

	// Update schedule config
	as.mu.Lock()
	config.RunCount++
	now := time.Now()
	config.LastRun = &now
	as.mu.Unlock()

	logging.Z().Info(fmt.Sprintf("Scheduled job submitted: %s (run #%d)", config.ID, config.RunCount))
}

// Start starts the advanced scheduler
func (as *AdvancedScheduler) Start(ctx context.Context, queue Queue) error {
	as.mu.Lock()
	if as.running {
		as.mu.Unlock()
		return fmt.Errorf("scheduler already running")
	}

	as.running = true
	as.queue = queue
	as.ctx, as.cancel = context.WithCancel(ctx)
	as.mu.Unlock()

	as.scheduler.Start()
	logging.Z().Info(fmt.Sprintf("Advanced scheduler started"))
	return nil
}

// Stop stops the advanced scheduler
func (as *AdvancedScheduler) Stop() error {
	as.mu.Lock()
	if !as.running {
		as.mu.Unlock()
		return fmt.Errorf("scheduler not running")
	}

	as.running = false
	as.mu.Unlock()

	as.scheduler.Stop()
	if as.cancel != nil {
		as.cancel()
	}

	logging.Z().Info(fmt.Sprintf("Advanced scheduler stopped"))
	return nil
}

// UpdateSchedule updates an existing schedule
func (as *AdvancedScheduler) UpdateSchedule(scheduleID string, config *ScheduleConfig) error {
	as.mu.Lock()
	oldConfig, exists := as.schedules[scheduleID]
	as.mu.Unlock()

	if !exists {
		return fmt.Errorf("schedule not found: %s", scheduleID)
	}

	// Remove old schedule
	as.scheduler.Remove(oldConfig.EntryID)

	// Add new schedule
	config.ID = scheduleID
	return as.Schedule(config)
}

// RemoveSchedule removes a schedule
func (as *AdvancedScheduler) RemoveSchedule(scheduleID string) error {
	as.mu.Lock()
	config, exists := as.schedules[scheduleID]
	if exists {
		delete(as.schedules, scheduleID)
	}
	as.mu.Unlock()

	if !exists {
		return fmt.Errorf("schedule not found: %s", scheduleID)
	}

	as.scheduler.Remove(config.EntryID)
	logging.Z().Info(fmt.Sprintf("Schedule removed: %s", scheduleID))
	return nil
}

// ListSchedules returns all schedules
func (as *AdvancedScheduler) ListSchedules() []*ScheduleConfig {
	as.mu.RLock()
	defer as.mu.RUnlock()

	schedules := make([]*ScheduleConfig, 0, len(as.schedules))
	for _, config := range as.schedules {
		schedules = append(schedules, config)
	}

	return schedules
}

// GetSchedule gets a specific schedule
func (as *AdvancedScheduler) GetSchedule(scheduleID string) (*ScheduleConfig, error) {
	as.mu.RLock()
	defer as.mu.RUnlock()

	config, exists := as.schedules[scheduleID]
	if !exists {
		return nil, fmt.Errorf("schedule not found: %s", scheduleID)
	}

	return config, nil
}

// EnableSchedule enables a schedule
func (as *AdvancedScheduler) EnableSchedule(scheduleID string) error {
	as.mu.Lock()
	config, exists := as.schedules[scheduleID]
	as.mu.Unlock()

	if !exists {
		return fmt.Errorf("schedule not found: %s", scheduleID)
	}

	config.Enabled = true
	logging.Z().Info(fmt.Sprintf("Schedule enabled: %s", scheduleID))
	return nil
}

// DisableSchedule disables a schedule
func (as *AdvancedScheduler) DisableSchedule(scheduleID string) error {
	as.mu.Lock()
	config, exists := as.schedules[scheduleID]
	as.mu.Unlock()

	if !exists {
		return fmt.Errorf("schedule not found: %s", scheduleID)
	}

	config.Enabled = false
	logging.Z().Info(fmt.Sprintf("Schedule disabled: %s", scheduleID))
	return nil
}

// TriggerNow manually triggers a schedule
func (as *AdvancedScheduler) TriggerNow(scheduleID string) error {
	as.mu.RLock()
	config, exists := as.schedules[scheduleID]
	as.mu.RUnlock()

	if !exists {
		return fmt.Errorf("schedule not found: %s", scheduleID)
	}

	as.submitScheduledJob(config)
	logging.Z().Info(fmt.Sprintf("Schedule triggered manually: %s", scheduleID))
	return nil
}

// GetScheduleStats returns statistics for a schedule
func (as *AdvancedScheduler) GetScheduleStats(scheduleID string) map[string]interface{} {
	as.mu.RLock()
	config, exists := as.schedules[scheduleID]
	as.mu.RUnlock()

	if !exists {
		return nil
	}

	return map[string]interface{}{
		"id":        config.ID,
		"enabled":   config.Enabled,
		"cron":      config.CronExpr,
		"run_count": config.RunCount,
		"last_run":  config.LastRun,
		"next_run":  config.NextRun,
		"job_type":  config.JobType,
	}
}

// ScheduleBuilder provides a fluent interface for building schedules
type ScheduleBuilder struct {
	config    *ScheduleConfig
	scheduler *AdvancedScheduler
}

// NewScheduleBuilder creates a new schedule builder
func NewScheduleBuilder(scheduler *AdvancedScheduler, jobType JobType) *ScheduleBuilder {
	return &ScheduleBuilder{
		config: &ScheduleConfig{
			JobType:  jobType,
			Priority: PriorityNormal,
			Timeout:  5 * time.Minute,
		},
		scheduler: scheduler,
	}
}

// WithCron sets the cron expression
func (sb *ScheduleBuilder) WithCron(expr string) *ScheduleBuilder {
	sb.config.CronExpr = expr
	return sb
}

// WithData sets the job data
func (sb *ScheduleBuilder) WithData(data map[string]interface{}) *ScheduleBuilder {
	sb.config.Data = data
	return sb
}

// WithDescription sets the description
func (sb *ScheduleBuilder) WithDescription(desc string) *ScheduleBuilder {
	sb.config.Description = desc
	return sb
}

// WithPriority sets the priority
func (sb *ScheduleBuilder) WithPriority(priority JobPriority) *ScheduleBuilder {
	sb.config.Priority = priority
	return sb
}

// WithTimeout sets the timeout
func (sb *ScheduleBuilder) WithTimeout(duration time.Duration) *ScheduleBuilder {
	sb.config.Timeout = duration
	return sb
}

// WithMaxConcurrent sets max concurrent executions
func (sb *ScheduleBuilder) WithMaxConcurrent(max int) *ScheduleBuilder {
	sb.config.MaxConcurrent = max
	return sb
}

// Build builds and schedules the job
func (sb *ScheduleBuilder) Build() error {
	return sb.scheduler.Schedule(sb.config)
}

// CommonCronExpressions provides pre-built cron expressions
var CommonCronExpressions = map[string]string{
	"every_minute":      "* * * * *",
	"every_5_minutes":   "*/5 * * * *",
	"every_30_minutes":  "*/30 * * * *",
	"every_hour":        "0 * * * *",
	"every_6_hours":     "0 */6 * * *",
	"every_12_hours":    "0 */12 * * *",
	"daily_midnight":    "0 0 * * *",
	"daily_noon":        "0 12 * * *",
	"daily_at_2am":      "0 2 * * *",
	"daily_at_3am":      "0 3 * * *",
	"daily_at_6am":      "0 6 * * *",
	"weekdays_9am":      "0 9 * * 1-5",
	"weekends_10am":     "0 10 * * 0,6",
	"every_monday":      "0 0 * * 1",
	"every_sunday":      "0 0 * * 0",
	"monthly_first_day": "0 0 1 * *",
	"monthly_last_day":  "0 0 L * *",
	"quarterly":         "0 0 1 */3 *",
	"yearly":            "0 0 1 1 *",
}

// ScheduleHistory tracks scheduled job execution history
type ScheduleHistory struct {
	mu      sync.RWMutex
	history map[string][]*ScheduleExecution
}

// ScheduleExecution represents a single execution of a scheduled job
type ScheduleExecution struct {
	ScheduleID string
	JobID      string
	ExecutedAt time.Time
	Duration   time.Duration
	Status     string // success, failed
	Error      string
}

// NewScheduleHistory creates a new schedule history tracker
func NewScheduleHistory() *ScheduleHistory {
	return &ScheduleHistory{
		history: make(map[string][]*ScheduleExecution),
	}
}

// RecordExecution records a schedule execution
func (sh *ScheduleHistory) RecordExecution(scheduleID, jobID string, duration time.Duration, success bool, err error) {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	status := "success"
	errMsg := ""
	if !success {
		status = "failed"
		if err != nil {
			errMsg = err.Error()
		}
	}

	exec := &ScheduleExecution{
		ScheduleID: scheduleID,
		JobID:      jobID,
		ExecutedAt: time.Now(),
		Duration:   duration,
		Status:     status,
		Error:      errMsg,
	}

	sh.history[scheduleID] = append(sh.history[scheduleID], exec)
}

// GetHistory returns execution history for a schedule
func (sh *ScheduleHistory) GetHistory(scheduleID string, limit int) []*ScheduleExecution {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	executions := sh.history[scheduleID]
	if len(executions) > limit {
		return executions[len(executions)-limit:]
	}

	return executions
}

// GetStats returns statistics for a schedule's execution history
func (sh *ScheduleHistory) GetStats(scheduleID string) map[string]interface{} {
	sh.mu.RLock()
	executions := sh.history[scheduleID]
	sh.mu.RUnlock()

	if len(executions) == 0 {
		return map[string]interface{}{
			"total_executions": 0,
			"success_count":    0,
			"failure_count":    0,
			"success_rate":     0.0,
		}
	}

	successCount := 0
	failureCount := 0
	totalDuration := time.Duration(0)

	for _, exec := range executions {
		if exec.Status == "success" {
			successCount++
		} else {
			failureCount++
		}
		totalDuration += exec.Duration
	}

	successRate := float64(successCount) / float64(len(executions))
	avgDuration := totalDuration / time.Duration(len(executions))

	return map[string]interface{}{
		"total_executions": len(executions),
		"success_count":    successCount,
		"failure_count":    failureCount,
		"success_rate":     successRate,
		"avg_duration":     avgDuration,
		"last_execution":   executions[len(executions)-1].ExecutedAt,
	}
}
