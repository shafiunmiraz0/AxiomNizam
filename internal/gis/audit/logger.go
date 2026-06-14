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
	gisAuditKVKey   = "gis:audit:log"
	gisAuditTTL     = 5 * time.Second
	gisAuditMaxSize = 1000
)

type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

type Category string

const (
	CategoryLayer     Category = "layer"
	CategoryRegion    Category = "region"
	CategoryMarker    Category = "marker"
	CategoryDataset   Category = "dataset"
	CategoryDashboard Category = "dashboard"
	CategoryConvert   Category = "convert"
)

type Action string

const (
	ActionLayerCreated    Action = "layer_created"
	ActionLayerUpdated    Action = "layer_updated"
	ActionLayerDeleted    Action = "layer_deleted"
	ActionRegionCreated   Action = "region_created"
	ActionRegionUpdated   Action = "region_updated"
	ActionRegionDeleted   Action = "region_deleted"
	ActionMarkerCreated   Action = "marker_created"
	ActionMarkerUpdated   Action = "marker_updated"
	ActionMarkerDeleted   Action = "marker_deleted"
	ActionDatasetCreated  Action = "dataset_created"
	ActionDatasetUpdated  Action = "dataset_updated"
	ActionDatasetDeleted  Action = "dataset_deleted"
	ActionDashboardViewed Action = "dashboard_viewed"
	ActionConverted       Action = "converted"
)

type Event struct {
	Timestamp  time.Time              `json:"timestamp"`
	Severity   Severity               `json:"severity"`
	Category   Category               `json:"category"`
	Action     Action                 `json:"action"`
	EntityID   string                 `json:"entityId,omitempty"`
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
	logging.Z().Info("gis audit: KVStore persistence configured")
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

func (l *Logger) LogEntity(category Category, action Action, entityID, message string) {
	event := Event{
		Timestamp: time.Now().UTC(),
		Severity:  SeverityInfo,
		Category:  category,
		Action:    action,
		EntityID:  entityID,
		Message:   message,
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
	if len(l.events) > gisAuditMaxSize {
		l.events = l.events[len(l.events)-gisAuditMaxSize:]
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

	ctx, cancel := context.WithTimeout(context.Background(), gisAuditTTL)
	defer cancel()

	encoded, err := json.Marshal(data)
	if err != nil {
		logging.Z().Error("gis audit: marshal failed")
		return
	}
	if err := l.kvStore.Put(ctx, gisAuditKVKey, string(encoded)); err != nil {
		logging.Z().Error("gis audit: kv persist failed")
	}
}

func (l *Logger) loadFromKV() {
	if l.kvStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), gisAuditTTL)
	defer cancel()
	data, err := l.kvStore.Get(ctx, gisAuditKVKey)
	if err != nil || data == "" {
		return
	}
	var events []Event
	if err := json.Unmarshal([]byte(data), &events); err != nil {
		logging.Z().Error("gis audit: unmarshal failed")
		return
	}
	l.events = events
}
