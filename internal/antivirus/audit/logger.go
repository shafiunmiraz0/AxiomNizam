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
	avAuditKVKey = "antivirus:audit:log"
	avAuditMax   = 1000
	avAuditTTL   = 5 * time.Second
)

// Event represents an antivirus audit log entry.
type Event struct {
	ID        uuid.UUID              `json:"id"`
	EventType string                 `json:"eventType"`
	Category  string                 `json:"category"`
	Action    string                 `json:"action"`
	Severity  string                 `json:"severity"`
	Message   string                 `json:"message"`
	Filename  string                 `json:"filename,omitempty"`
	SHA256    string                 `json:"sha256,omitempty"`
	Threats   []string               `json:"threats,omitempty"`
	Duration  time.Duration          `json:"duration,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Logger provides security audit logging for antivirus events.
type Logger struct {
	mu      sync.RWMutex
	events  []*Event
	kvStore platformstore.KVStore
}

// NewLogger creates a new antivirus audit logger.
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

	ctx, cancel := context.WithTimeout(context.Background(), avAuditTTL)
	defer cancel()

	val, err := kv.Get(ctx, avAuditKVKey)
	if err != nil || val == "" {
		return // not found or empty
	}

	var events []*Event
	if err := json.Unmarshal([]byte(val), &events); err != nil {
		logging.Z().Info(fmt.Sprintf("antivirus audit: failed to unmarshal events: %v", err))
		return
	}

	l.mu.Lock()
	l.events = events
	l.mu.Unlock()
	logging.Z().Info(fmt.Sprintf("antivirus audit: loaded %d persistent events", len(events)))
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
	if len(persistEvents) > avAuditMax {
		persistEvents = persistEvents[len(persistEvents)-avAuditMax:]
	}

	data, err := json.Marshal(persistEvents)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), avAuditTTL)
	defer cancel()
	if err := kv.Put(ctx, avAuditKVKey, string(data)); err != nil {
		logging.Z().Error(fmt.Sprintf("antivirus audit: kv persist failed: %v", err))
	}
}

func (l *Logger) recordToBuffer(event *Event) {
	l.mu.Lock()
	if len(l.events) >= avAuditMax {
		evict := avAuditMax / 10
		if evict < 1 {
			evict = 1
		}
		l.events = l.events[evict:]
	}
	l.events = append(l.events, event)
	l.mu.Unlock()

	go l.saveToKV()
}

// LogScanResult logs a completed scan result.
func (l *Logger) LogScanResult(filename, sha256, verdict string, threats []string, duration time.Duration) {
	severity := SeverityInfo
	if verdict == "malware" {
		severity = SeverityCritical
	} else if verdict == "suspicious" {
		severity = SeverityWarning
	}

	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "scan." + verdict,
		Category:  CategoryScan,
		Action:    ActionScanned,
		Severity:  severity,
		Message:   fmt.Sprintf("Scan completed: %s — %s", filename, verdict),
		Filename:  filename,
		SHA256:    sha256,
		Threats:   threats,
		Duration:  duration,
		Timestamp: time.Now().UTC(),
	})
}

// LogThreatDetected logs a threat detection event.
func (l *Logger) LogThreatDetected(filename, sha256, threatName string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "threat.detected",
		Category:  CategoryThreat,
		Action:    ActionDetected,
		Severity:  SeverityCritical,
		Message:   fmt.Sprintf("Threat detected: %s in %s", threatName, filename),
		Filename:  filename,
		SHA256:    sha256,
		Threats:   []string{threatName},
		Timestamp: time.Now().UTC(),
	})
}

// LogEngineEvent logs engine lifecycle events.
func (l *Logger) LogEngineEvent(action, message string) {
	severity := SeverityInfo
	if action == ActionError {
		severity = SeverityError
	}

	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "engine." + action,
		Category:  CategoryEngine,
		Action:    action,
		Severity:  severity,
		Message:   message,
		Timestamp: time.Now().UTC(),
	})
}

// LogSignatureReload logs a signature database reload event.
func (l *Logger) LogSignatureReload(version string, count int) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "signature.reloaded",
		Category:  CategorySignature,
		Action:    ActionReloaded,
		Severity:  SeverityInfo,
		Message:   fmt.Sprintf("Signatures reloaded: version=%s count=%d", version, count),
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]interface{}{"version": version, "count": count},
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
		if filter.Category != "" && ev.Category != filter.Category {
			continue
		}
		if filter.Severity != "" && ev.Severity != filter.Severity {
			continue
		}
		if filter.Filename != "" && ev.Filename != filter.Filename {
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
