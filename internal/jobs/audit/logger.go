package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"github.com/google/uuid"
)

const (
	jobsAuditKVKey = "jobs:audit:log"
	jobsAuditMax   = 1000
	jobsAuditTTL   = 5 * time.Second
)

// Event represents a jobs audit log entry.
type Event struct {
	ID        uuid.UUID              `json:"id"`
	EventType string                 `json:"eventType"`
	JobID     string                 `json:"jobID,omitempty"`
	JobType   string                 `json:"jobType,omitempty"`
	Category  string                 `json:"category"`
	Action    string                 `json:"action"`
	Severity  string                 `json:"severity"`
	Message   string                 `json:"message"`
	Duration  time.Duration          `json:"duration,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Logger provides security audit logging for jobs events.
type Logger struct {
	mu      sync.RWMutex
	events  []*Event
	kvStore platformstore.KVStore
}

// NewLogger creates a new jobs audit logger.
func NewLogger() *Logger {
	return &Logger{
		events: make([]*Event, 0, 128),
	}
}

// ConfigureKVPersistence enables KVStore-backed persistence for the audit log.
func (l *Logger) ConfigureKVPersistence(kv platformstore.KVStore) {
	l.mu.Lock()
	l.kvStore = kv
	l.mu.Unlock()
	l.loadFromKV()
}

func (l *Logger) loadFromKV() {
	l.mu.Lock()
	kv := l.kvStore
	l.mu.Unlock()
	if kv == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), jobsAuditTTL)
	defer cancel()

	val, err := kv.Get(ctx, jobsAuditKVKey)
	if err != nil || val == "" {
		return // not found or empty
	}

	var events []*Event
	if err := json.Unmarshal([]byte(val), &events); err != nil {
		logging.Z().Info(fmt.Sprintf("jobs audit: failed to unmarshal events: %v", err))
		return
	}

	l.mu.Lock()
	l.events = events
	l.mu.Unlock()
	logging.Z().Info(fmt.Sprintf("jobs audit: loaded %d persistent events", len(events)))
}

func (l *Logger) saveToKV() {
	l.mu.RLock()
	kv := l.kvStore
	events := l.events
	l.mu.RUnlock()
	if kv == nil {
		return
	}

	persistEvents := events
	if len(persistEvents) > jobsAuditMax {
		persistEvents = persistEvents[len(persistEvents)-jobsAuditMax:]
	}

	data, err := json.Marshal(persistEvents)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), jobsAuditTTL)
	defer cancel()
	if err := kv.Put(ctx, jobsAuditKVKey, string(data)); err != nil {
		logging.Z().Error(fmt.Sprintf("jobs audit: kv persist failed: %v", err))
	}
}

func (l *Logger) recordToBuffer(event *Event) {
	l.mu.Lock()
	if len(l.events) >= jobsAuditMax {
		evict := jobsAuditMax / 10
		if evict < 1 {
			evict = 1
		}
		l.events = l.events[evict:]
	}
	l.events = append(l.events, event)
	l.mu.Unlock()

	go l.saveToKV()
}

// LogJobCreated logs a job creation event.
func (l *Logger) LogJobCreated(jobID, jobType string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "job.created",
		JobID:     jobID,
		JobType:   jobType,
		Category:  CategoryJob,
		Action:    ActionCreated,
		Severity:  SeverityInfo,
		Message:   fmt.Sprintf("Job created: %s (%s)", jobID, jobType),
		Timestamp: time.Now().UTC(),
	})
}

// LogJobStarted logs a job start event.
func (l *Logger) LogJobStarted(jobID, jobType string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "job.started",
		JobID:     jobID,
		JobType:   jobType,
		Category:  CategoryJob,
		Action:    ActionStarted,
		Severity:  SeverityInfo,
		Message:   "Job started: " + jobID,
		Timestamp: time.Now().UTC(),
	})
}

// LogJobCompleted logs a job completion event.
func (l *Logger) LogJobCompleted(jobID, jobType string, duration time.Duration) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "job.completed",
		JobID:     jobID,
		JobType:   jobType,
		Category:  CategoryJob,
		Action:    ActionCompleted,
		Severity:  SeverityInfo,
		Message:   fmt.Sprintf("Job completed: %s (%s)", jobID, jobType),
		Duration:  duration,
		Timestamp: time.Now().UTC(),
	})
}

// LogJobFailed logs a job failure event.
func (l *Logger) LogJobFailed(jobID, jobType, errorMsg string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "job.failed",
		JobID:     jobID,
		JobType:   jobType,
		Category:  CategoryJob,
		Action:    ActionFailed,
		Severity:  SeverityError,
		Message:   fmt.Sprintf("Job failed: %s — %s", jobID, errorMsg),
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]interface{}{"error": errorMsg},
	})
}

// LogJobCancelled logs a job cancellation event.
func (l *Logger) LogJobCancelled(jobID, jobType string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "job.cancelled",
		JobID:     jobID,
		JobType:   jobType,
		Category:  CategoryJob,
		Action:    ActionCancelled,
		Severity:  SeverityWarning,
		Message:   "Job cancelled: " + jobID,
		Timestamp: time.Now().UTC(),
	})
}

// LogJobRetried logs a job retry event.
func (l *Logger) LogJobRetried(jobID, jobType string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "job.retried",
		JobID:     jobID,
		JobType:   jobType,
		Category:  CategoryJob,
		Action:    ActionRetried,
		Severity:  SeverityInfo,
		Message:   "Job retried: " + jobID,
		Timestamp: time.Now().UTC(),
	})
}

// LogDLQEvent logs a dead-letter queue event.
func (l *Logger) LogDLQEvent(jobID, jobType, reason string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "dlq.enqueued",
		JobID:     jobID,
		JobType:   jobType,
		Category:  CategoryDLQ,
		Action:    ActionEnqueued,
		Severity:  SeverityError,
		Message:   fmt.Sprintf("Job moved to DLQ: %s — %s", jobID, reason),
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]interface{}{"reason": reason},
	})
}

// List returns events matching the given filter.
func (l *Logger) List(filter *EventFilter) []*Event {
	if filter == nil {
		filter = DefaultEventFilter()
	}
	l.mu.RLock()
	defer l.mu.RUnlock()

	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	result := make([]*Event, 0)
	for i := len(l.events) - 1; i >= 0 && len(result) < filter.Limit; i-- {
		ev := l.events[i]
		if filter.JobType != "" && ev.JobType != filter.JobType {
			continue
		}
		if filter.Category != "" && ev.Category != filter.Category {
			continue
		}
		if filter.Severity != "" && ev.Severity != filter.Severity {
			continue
		}
		result = append(result, ev)
	}
	return result
}

// Count returns the total number of stored events.
func (l *Logger) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.events)
}
