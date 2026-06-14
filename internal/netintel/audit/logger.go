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
	netintelAuditKVKey   = "netintel:audit:log"
	netintelAuditTTL     = 5 * time.Second
	netintelAuditMaxSize = 1000
)

// Event represents a single audit event.
type Event struct {
	Timestamp time.Time              `json:"timestamp"`
	Severity  Severity               `json:"severity"`
	Category  Category               `json:"category"`
	Action    Action                 `json:"action"`
	Resource  string                 `json:"resource,omitempty"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Logger records audit events for the netintel module.
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
	logging.Z().Info("netintel audit: KVStore persistence configured")
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

// LogWithResource records an audit event with a resource identifier.
func (l *Logger) LogWithResource(severity Severity, category Category, action Action, resource, message string) {
	event := Event{
		Timestamp: time.Now().UTC(),
		Severity:  severity,
		Category:  category,
		Action:    action,
		Resource:  resource,
		Message:   message,
	}
	l.recordToBuffer(event)
}

// LogParser records a parser-related audit event.
func (l *Logger) LogParser(action Action, parserID, message string) {
	l.LogWithResource(SeverityInfo, CategoryParser, action, parserID, message)
}

// LogAnomaly records an anomaly-related audit event.
func (l *Logger) LogAnomaly(action Action, anomalyID, message string) {
	l.LogWithResource(SeverityWarn, CategoryAnomaly, action, anomalyID, message)
}

// LogAlert records an alert-related audit event.
func (l *Logger) LogAlert(action Action, alertID, message string) {
	l.LogWithResource(SeverityWarn, CategoryAlert, action, alertID, message)
}

// LogIngest records an ingestion-related audit event.
func (l *Logger) LogIngest(action Action, logType, source, message string) {
	severity := SeverityInfo
	if action == ActionEntryDropped {
		severity = SeverityWarn
	}
	l.LogWithResource(severity, CategoryIngest, action, source, message)
}

// LogMode records a mode-related audit event.
func (l *Logger) LogMode(action Action, mode, message string) {
	l.LogWithResource(SeverityInfo, CategoryMode, action, mode, message)
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
	if len(l.events) > netintelAuditMaxSize {
		l.events = l.events[len(l.events)-netintelAuditMaxSize:]
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

	ctx, cancel := context.WithTimeout(context.Background(), netintelAuditTTL)
	defer cancel()

	encoded, err := json.Marshal(data)
	if err != nil {
		logging.Z().Info("netintel audit: marshal failed")
		return
	}
	if err := l.kvStore.Put(ctx, netintelAuditKVKey, string(encoded)); err != nil {
		logging.Z().Info("netintel audit: kv persist failed")
	}
}

func (l *Logger) loadFromKV() {
	if l.kvStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), netintelAuditTTL)
	defer cancel()
	data, err := l.kvStore.Get(ctx, netintelAuditKVKey)
	if err != nil || data == "" {
		return
	}
	var events []Event
	if err := json.Unmarshal([]byte(data), &events); err != nil {
		logging.Z().Info("netintel audit: unmarshal failed")
		return
	}
	l.events = events
}
