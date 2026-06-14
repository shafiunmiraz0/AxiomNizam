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
	cdcAuditKVKey   = "cdc:audit:log"
	cdcAuditTTL     = 5 * time.Second
	cdcAuditMaxSize = 1000
)

type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

type Category string

const (
	CategoryPipeline  Category = "pipeline"
	CategoryETL       Category = "etl"
	CategoryEvent     Category = "event"
	CategoryConnector Category = "connector"
	CategoryStream    Category = "stream"
)

type Action string

const (
	ActionPipelineCreated Action = "pipeline_created"
	ActionPipelineStarted Action = "pipeline_started"
	ActionPipelinePaused  Action = "pipeline_paused"
	ActionPipelineStopped Action = "pipeline_stopped"
	ActionPipelineFailed  Action = "pipeline_failed"
	ActionPipelineDeleted Action = "pipeline_deleted"
	ActionETLRunStarted   Action = "etl_run_started"
	ActionETLRunCompleted Action = "etl_run_completed"
	ActionETLRunFailed    Action = "etl_run_failed"
	ActionEventCaptured   Action = "event_captured"
	ActionConnectorAdded  Action = "connector_added"
	ActionStreamCreated   Action = "stream_created"
)

type Event struct {
	Timestamp  time.Time              `json:"timestamp"`
	Severity   Severity               `json:"severity"`
	Category   Category               `json:"category"`
	Action     Action                 `json:"action"`
	PipelineID string                 `json:"pipelineId,omitempty"`
	Table      string                 `json:"table,omitempty"`
	Message    string                 `json:"message"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type Logger struct {
	mu      sync.RWMutex
	events  []Event
	kvStore platformstore.KVStore
}

func NewLogger() *Logger {
	return &Logger{
		events: make([]Event, 0, 64),
	}
}

func (l *Logger) ConfigureKVPersistence(kv platformstore.KVStore) {
	l.kvStore = kv
	l.loadFromKV()
	logging.Z().Info("cdc audit: KVStore persistence configured")
}

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

func (l *Logger) LogPipeline(action Action, pipelineID, message string) {
	severity := SeverityInfo
	if action == ActionPipelineFailed {
		severity = SeverityError
	}
	event := Event{
		Timestamp:  time.Now().UTC(),
		Severity:   severity,
		Category:   CategoryPipeline,
		Action:     action,
		PipelineID: pipelineID,
		Message:    message,
	}
	l.recordToBuffer(event)
}

func (l *Logger) LogETLRun(action Action, pipelineID, message string) {
	severity := SeverityInfo
	if action == ActionETLRunFailed {
		severity = SeverityWarn
	}
	event := Event{
		Timestamp:  time.Now().UTC(),
		Severity:   severity,
		Category:   CategoryETL,
		Action:     action,
		PipelineID: pipelineID,
		Message:    message,
	}
	l.recordToBuffer(event)
}

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
	if len(l.events) > cdcAuditMaxSize {
		l.events = l.events[len(l.events)-cdcAuditMaxSize:]
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

	ctx, cancel := context.WithTimeout(context.Background(), cdcAuditTTL)
	defer cancel()

	encoded, err := json.Marshal(data)
	if err != nil {
		logging.Z().Error("cdc audit: marshal failed")
		return
	}
	if err := l.kvStore.Put(ctx, cdcAuditKVKey, string(encoded)); err != nil {
		logging.Z().Error("cdc audit: kv persist failed")
	}
}

func (l *Logger) loadFromKV() {
	if l.kvStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cdcAuditTTL)
	defer cancel()
	data, err := l.kvStore.Get(ctx, cdcAuditKVKey)
	if err != nil || data == "" {
		return
	}
	var events []Event
	if err := json.Unmarshal([]byte(data), &events); err != nil {
		logging.Z().Error("cdc audit: unmarshal failed")
		return
	}
	l.events = events
}
