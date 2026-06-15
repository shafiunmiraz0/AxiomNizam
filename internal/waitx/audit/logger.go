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
	waitxAuditKVKey   = "waitx:audit:log"
	waitxAuditTTL     = 5 * time.Second
	waitxAuditMaxSize = 1000
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
	CategoryCheck     Category = "check"
	CategoryGroup     Category = "group"
	CategoryConfig    Category = "config"
	CategoryLifecycle Category = "lifecycle"
)

// Action represents the action of an audit event.
type Action string

const (
	ActionCheckStarted   Action = "check_started"
	ActionCheckSucceeded Action = "check_succeeded"
	ActionCheckFailed    Action = "check_failed"
	ActionCheckTimedOut  Action = "check_timed_out"
	ActionGroupCreated   Action = "group_created"
	ActionGroupChecked   Action = "group_checked"
	ActionConfigUpdated  Action = "config_updated"
)

// Event represents a single audit event.
type Event struct {
	Timestamp time.Time              `json:"timestamp"`
	Severity  Severity               `json:"severity"`
	Category  Category               `json:"category"`
	Action    Action                 `json:"action"`
	CheckType string                 `json:"check_type,omitempty"`
	Target    string                 `json:"target,omitempty"`
	Duration  int64                  `json:"duration_ms,omitempty"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Logger records audit events for the waitx module.
type Logger struct {
	mu     sync.RWMutex
	events []Event
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
	logging.Z().Info("waitx audit: KVStore persistence configured")
}

// Log records an audit event.
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

// LogCheck records a check-related audit event.
func (l *Logger) LogCheck(action Action, checkType, target string, durationMs int64, message string) {
	event := Event{
		Timestamp: time.Now().UTC(),
		Severity:  severityForAction(action),
		Category:  CategoryCheck,
		Action:    action,
		CheckType: checkType,
		Target:    target,
		Duration:  durationMs,
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
	if len(l.events) > waitxAuditMaxSize {
		l.events = l.events[len(l.events)-waitxAuditMaxSize:]
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

	ctx, cancel := context.WithTimeout(context.Background(), waitxAuditTTL)
	defer cancel()

	encoded, err := json.Marshal(data)
	if err != nil {
		logging.Z().Error("waitx audit: marshal failed")
		return
	}
	if err := l.kvStore.Put(ctx, waitxAuditKVKey, string(encoded)); err != nil {
		logging.Z().Error("waitx audit: kv persist failed")
	}
}

func (l *Logger) loadFromKV() {
	if l.kvStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitxAuditTTL)
	defer cancel()
	data, err := l.kvStore.Get(ctx, waitxAuditKVKey)
	if err != nil || data == "" {
		return
	}
	var events []Event
	if err := json.Unmarshal([]byte(data), &events); err != nil {
		logging.Z().Error("waitx audit: unmarshal failed")
		return
	}
	l.events = events
}

func severityForAction(action Action) Severity {
	switch action {
	case ActionCheckFailed, ActionCheckTimedOut:
		return SeverityWarn
	default:
		return SeverityInfo
	}
}
