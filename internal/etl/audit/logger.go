package audit

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

const (
	etlAuditKVKey   = "etl:audit:log"
	etlAuditTTL     = 5 * time.Second
	etlAuditMaxSize = 1000
)

// Severity represents the severity of an audit event.
type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// Category represents the category of an audit event.
type Category string

const (
	CategoryPipeline  Category = "pipeline"
	CategoryRun       Category = "run"
	CategoryConnector Category = "connector"
	CategoryConfig    Category = "config"
)

// Action represents the action of an audit event.
type Action string

const (
	ActionPipelineCreated  Action = "pipeline_created"
	ActionPipelineUpdated  Action = "pipeline_updated"
	ActionPipelineDeleted  Action = "pipeline_deleted"
	ActionPipelineRun      Action = "pipeline_run"
	ActionPipelinePaused   Action = "pipeline_paused"
	ActionPipelineStopped  Action = "pipeline_stopped"
	ActionRunStarted       Action = "run_started"
	ActionRunCompleted     Action = "run_completed"
	ActionRunFailed        Action = "run_failed"
	ActionConnectorCreated Action = "connector_created"
	ActionConnectorUpdated Action = "connector_updated"
	ActionConnectorDeleted Action = "connector_deleted"
	ActionConfigUpdated    Action = "config_updated"
)

// Event represents a single audit event.
type Event struct {
	Timestamp  time.Time              `json:"timestamp"`
	Severity   Severity               `json:"severity"`
	Category   Category               `json:"category"`
	Action     Action                 `json:"action"`
	PipelineID string                 `json:"pipeline_id,omitempty"`
	RunID      string                 `json:"run_id,omitempty"`
	Connector  string                 `json:"connector,omitempty"`
	Message    string                 `json:"message"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// Logger records audit events for the ETL module.
type Logger struct {
	mu      sync.RWMutex
	events  []Event
	kvStore platformstore.KVStore
}

// NewLogger creates a new audit logger.
func NewLogger() *Logger {
	return &Logger{
		events: make([]Event, 0, 64),
	}
}

// ConfigureKVPersistence wires the KVStore for persistence.
func (l *Logger) ConfigureKVPersistence(kv platformstore.KVStore) {
	l.kvStore = kv
	l.loadFromKV()
	logging.Z().Info("etl audit: KVStore persistence configured")
}

// Log records a generic audit event.
func (l *Logger) Log(severity Severity, category Category, action Action, message string) {
	event := Event{
		Timestamp: time.Now().UTC(),
		Severity:  severity,
		Category:  category,
		Action:    action,
		Message:   message,
	}
	l.recordToBuffer(event)
}

// LogPipeline records a pipeline-related audit event.
func (l *Logger) LogPipeline(action Action, pipelineID, message string) {
	event := Event{
		Timestamp:  time.Now().UTC(),
		Severity:   severityForAction(action),
		Category:   CategoryPipeline,
		Action:     action,
		PipelineID: pipelineID,
		Message:    message,
	}
	l.recordToBuffer(event)
}

// LogRun records a run-related audit event.
func (l *Logger) LogRun(action Action, pipelineID, runID, message string) {
	event := Event{
		Timestamp:  time.Now().UTC(),
		Severity:   severityForAction(action),
		Category:   CategoryRun,
		Action:     action,
		PipelineID: pipelineID,
		RunID:      runID,
		Message:    message,
	}
	l.recordToBuffer(event)
}

// LogConnector records a connector-related audit event.
func (l *Logger) LogConnector(action Action, connector, message string) {
	event := Event{
		Timestamp: time.Now().UTC(),
		Severity:  severityForAction(action),
		Category:  CategoryConnector,
		Action:    action,
		Connector: connector,
		Message:   message,
	}
	l.recordToBuffer(event)
}

// List returns all audit events.
func (l *Logger) List() []Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Event, len(l.events))
	copy(out, l.events)
	return out
}

func (l *Logger) recordToBuffer(event Event) {
	l.mu.Lock()
	l.events = append(l.events, event)
	if len(l.events) > etlAuditMaxSize {
		l.events = l.events[len(l.events)-etlAuditMaxSize:]
	}
	l.mu.Unlock()

	go l.saveToKV()
}

func (l *Logger) saveToKV() {
	if l.kvStore == nil {
		return
	}
	l.mu.RLock()
	data := make([]Event, len(l.events))
	copy(data, l.events)
	l.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), etlAuditTTL)
	defer cancel()

	encoded, err := json.Marshal(data)
	if err != nil {
		logging.Z().Error("etl audit: marshal failed")
		return
	}
	if err := l.kvStore.Put(ctx, etlAuditKVKey, string(encoded)); err != nil {
		logging.Z().Error("etl audit: kv persist failed")
	}
}

func (l *Logger) loadFromKV() {
	if l.kvStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), etlAuditTTL)
	defer cancel()
	data, err := l.kvStore.Get(ctx, etlAuditKVKey)
	if err != nil || data == "" {
		return
	}
	var events []Event
	if err := json.Unmarshal([]byte(data), &events); err != nil {
		logging.Z().Error("etl audit: unmarshal failed")
		return
	}
	l.events = events
}

func severityForAction(action Action) Severity {
	switch action {
	case ActionRunFailed:
		return SeverityError
	case ActionPipelineStopped:
		return SeverityWarn
	default:
		return SeverityInfo
	}
}
