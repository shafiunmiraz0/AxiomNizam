package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/periodic"
)

// PeriodicScheduler wraps the periodic.Dispatcher as a lightweight
// alternative to AdvancedScheduler. Use it for simple interval-based
// schedules that don't need full cron expressions.
//
// Compared to AdvancedScheduler (robfig/cron):
//   - No external dependency — pure Go min-heap
//   - Standard 5-field cron expressions
//   - No persistence — missed fires during downtime are skipped
//   - Lower overhead for many short-lived schedules
type PeriodicScheduler struct {
	dispatcher *periodic.Dispatcher
	queue      Queue
	mu         sync.RWMutex
	schedules  map[string]*ScheduleConfig
	ctx        context.Context
	cancel     context.CancelFunc
	running    bool
}

// NewPeriodicScheduler creates a new periodic scheduler.
func NewPeriodicScheduler() *PeriodicScheduler {
	ps := &PeriodicScheduler{
		schedules: make(map[string]*ScheduleConfig),
	}
	// The dispatcher fires the same callback for all entries;
	// we route by schedule ID.
	ps.dispatcher = periodic.NewDispatcher(ps.onFire)
	return ps
}

// onFire is the global fire callback — routes to the correct job.
func (ps *PeriodicScheduler) onFire(ctx context.Context, id string, scheduled time.Time) {
	ps.mu.RLock()
	config, exists := ps.schedules[id]
	ps.mu.RUnlock()

	if !exists || !config.Enabled {
		return
	}

	job := CreateJobWithPriority(config.JobType, config.Data, config.Priority)
	job.Timeout = config.Timeout
	job.AddTag(fmt.Sprintf("periodic-%s", id))

	if err := ps.queue.Submit(ctx, job); err != nil {
		logging.Z().Info(fmt.Sprintf("Error submitting periodic job %s: %v", id, err))
		return
	}

	ps.mu.Lock()
	config.RunCount++
	now := time.Now()
	config.LastRun = &now
	ps.mu.Unlock()

	logging.Z().Info(fmt.Sprintf("Periodic job fired: %s (run #%d)", id, config.RunCount))
}

// ScheduleCron schedules a job with a 5-field cron expression
// (minute hour dom month dow). Examples:
//
//	"*/5 * * * *"  — every 5 minutes
//	"0 * * * *"    — every hour
//	"0 9 * * 1-5"  — weekdays at 9am
func (ps *PeriodicScheduler) ScheduleCron(config *ScheduleConfig, cronExpr string) error {
	if config.ID == "" {
		config.ID = generateJobID()
	}

	sched, err := periodic.Parse(cronExpr)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	ps.mu.Lock()
	ps.schedules[config.ID] = config
	ps.dispatcher.Add(config.ID, sched)
	ps.mu.Unlock()

	logging.Z().Info(fmt.Sprintf("Periodic schedule added: %s (cron: %s)", config.ID, cronExpr))
	return nil
}

// Start starts the periodic scheduler.
func (ps *PeriodicScheduler) Start(ctx context.Context, queue Queue) error {
	ps.mu.Lock()
	if ps.running {
		ps.mu.Unlock()
		return fmt.Errorf("periodic scheduler already running")
	}
	ps.running = true
	ps.ctx, ps.cancel = context.WithCancel(ctx)
	ps.queue = queue
	ps.mu.Unlock()

	ps.dispatcher.Start()
	logging.Z().Info("Periodic scheduler started")
	return nil
}

// Stop stops the periodic scheduler.
func (ps *PeriodicScheduler) Stop() error {
	ps.mu.Lock()
	if !ps.running {
		ps.mu.Unlock()
		return fmt.Errorf("periodic scheduler not running")
	}
	ps.running = false
	ps.mu.Unlock()

	ps.dispatcher.Stop()
	if ps.cancel != nil {
		ps.cancel()
	}

	logging.Z().Info("Periodic scheduler stopped")
	return nil
}

// RemoveSchedule removes a periodic schedule.
func (ps *PeriodicScheduler) RemoveSchedule(scheduleID string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if _, exists := ps.schedules[scheduleID]; !exists {
		return fmt.Errorf("schedule not found: %s", scheduleID)
	}

	ps.dispatcher.Remove(scheduleID)
	delete(ps.schedules, scheduleID)
	logging.Z().Info(fmt.Sprintf("Periodic schedule removed: %s", scheduleID))
	return nil
}

// ListSchedules returns all periodic schedules.
func (ps *PeriodicScheduler) ListSchedules() []*ScheduleConfig {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make([]*ScheduleConfig, 0, len(ps.schedules))
	for _, config := range ps.schedules {
		result = append(result, config)
	}
	return result
}
