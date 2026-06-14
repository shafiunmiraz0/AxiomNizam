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
	auditKVKey   = "apibanks:audit:log"
	auditTTL     = 5 * time.Second
	auditMaxSize = 1000
)

// Severity represents the severity of an audit event.
type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// Action represents the action of an audit event.
type Action string

const (
	ActionBankCreated    Action = "bank_created"
	ActionBankUpdated    Action = "bank_updated"
	ActionBankDeleted    Action = "bank_deleted"
	ActionAPIAdded       Action = "api_added"
	ActionAPIRemoved     Action = "api_removed"
	ActionCatalogSearch  Action = "catalog_search"
)

// Event represents a single audit event.
type Event struct {
	Timestamp time.Time              `json:"timestamp"`
	Severity  Severity               `json:"severity"`
	Action    Action                 `json:"action"`
	BankName  string                 `json:"bank_name,omitempty"`
	APIName   string                 `json:"api_name,omitempty"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Logger records audit events for the apibanks module.
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
	logging.Z().Info("apibanks audit: KVStore persistence configured")
}

// LogBank records a bank-related audit event.
func (l *Logger) LogBank(action Action, bankName, message string) {
	l.record(Event{
		Timestamp: time.Now().UTC(),
		Severity:  SeverityInfo,
		Action:    action,
		BankName:  bankName,
		Message:   message,
	})
}

// LogAPI records an API-related audit event.
func (l *Logger) LogAPI(action Action, bankName, apiName, message string) {
	l.record(Event{
		Timestamp: time.Now().UTC(),
		Severity:  SeverityInfo,
		Action:    action,
		BankName:  bankName,
		APIName:   apiName,
		Message:   message,
	})
}

// LogSearch records a catalog search event.
func (l *Logger) LogSearch(query, message string) {
	l.record(Event{
		Timestamp: time.Now().UTC(),
		Severity:  SeverityInfo,
		Action:    ActionCatalogSearch,
		Message:   message,
		Metadata:  map[string]interface{}{"query": query},
	})
}

// List returns all audit events.
func (l *Logger) List() []Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Event, len(l.events))
	copy(out, l.events)
	return out
}

func (l *Logger) record(event Event) {
	l.mu.Lock()
	l.events = append(l.events, event)
	if len(l.events) > auditMaxSize {
		l.events = l.events[len(l.events)-auditMaxSize:]
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

	ctx, cancel := context.WithTimeout(context.Background(), auditTTL)
	defer cancel()

	encoded, err := json.Marshal(data)
	if err != nil {
		logging.Z().Error("apibanks audit: marshal failed")
		return
	}
	if err := l.kvStore.Put(ctx, auditKVKey, string(encoded)); err != nil {
		logging.Z().Error("apibanks audit: kv persist failed")
	}
}

func (l *Logger) loadFromKV() {
	if l.kvStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), auditTTL)
	defer cancel()
	data, err := l.kvStore.Get(ctx, auditKVKey)
	if err != nil || data == "" {
		return
	}
	var events []Event
	if err := json.Unmarshal([]byte(data), &events); err != nil {
		logging.Z().Error("apibanks audit: unmarshal failed")
		return
	}
	l.events = events
}
