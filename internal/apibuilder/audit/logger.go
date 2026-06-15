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
	apibuilderAuditKVKey   = "apibuilder:audit:log"
	apibuilderAuditTTL     = 5 * time.Second
	apibuilderAuditMaxSize = 1000
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
	CategoryAPI        Category = "api"
	CategoryUpload     Category = "upload"
	CategoryScan       Category = "scan"
	CategoryConversion Category = "conversion"
	CategoryDashboard  Category = "dashboard"
)

// Action represents the action of an audit event.
type Action string

const (
	ActionAPICreated    Action = "api_created"
	ActionAPIUpdated    Action = "api_updated"
	ActionAPIDeleted    Action = "api_deleted"
	ActionAPIInvoked    Action = "api_invoked"
	ActionFileUploaded  Action = "file_uploaded"
	ActionFileScanned   Action = "file_scanned"
	ActionConverted     Action = "converted"
	ActionDashboardCreated Action = "dashboard_created"
	ActionSQLQuery      Action = "sql_query"
)

// Event represents a single audit event.
type Event struct {
	Timestamp time.Time              `json:"timestamp"`
	Severity  Severity               `json:"severity"`
	Category  Category               `json:"category"`
	Action    Action                 `json:"action"`
	Resource  string                 `json:"resource,omitempty"`
	User      string                 `json:"user,omitempty"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Logger records audit events for the apibuilder module.
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
	logging.Z().Info("apibuilder audit: KVStore persistence configured")
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

// LogAPI records an API-related audit event.
func (l *Logger) LogAPI(action Action, apiName, user, message string) {
	event := Event{
		Timestamp: time.Now().UTC(),
		Severity:  SeverityInfo,
		Category:  CategoryAPI,
		Action:    action,
		Resource:  apiName,
		User:      user,
		Message:   message,
	}
	l.recordToBuffer(event)
}

// LogScan records a scan audit event.
func (l *Logger) LogScan(filename, verdict string, safe bool) {
	severity := SeverityInfo
	if !safe {
		severity = SeverityWarn
	}
	event := Event{
		Timestamp: time.Now().UTC(),
		Severity:  severity,
		Category:  CategoryScan,
		Action:    ActionFileScanned,
		Resource:  filename,
		Message:   "file scan: " + verdict,
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
	if len(l.events) > apibuilderAuditMaxSize {
		l.events = l.events[len(l.events)-apibuilderAuditMaxSize:]
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

	ctx, cancel := context.WithTimeout(context.Background(), apibuilderAuditTTL)
	defer cancel()

	encoded, err := json.Marshal(data)
	if err != nil {
		logging.Z().Error("apibuilder audit: marshal failed")
		return
	}
	if err := l.kvStore.Put(ctx, apibuilderAuditKVKey, string(encoded)); err != nil {
		logging.Z().Error("apibuilder audit: kv persist failed")
	}
}

func (l *Logger) loadFromKV() {
	if l.kvStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), apibuilderAuditTTL)
	defer cancel()
	data, err := l.kvStore.Get(ctx, apibuilderAuditKVKey)
	if err != nil || data == "" {
		return
	}
	var events []Event
	if err := json.Unmarshal([]byte(data), &events); err != nil {
		logging.Z().Error("apibuilder audit: unmarshal failed")
		return
	}
	l.events = events
}
