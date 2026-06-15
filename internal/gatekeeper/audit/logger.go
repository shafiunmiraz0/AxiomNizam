package audit

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"sync"
	"time"

	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

const (
	gkAuditKVKey = "gatekeeper:audit:log"
	gkAuditMax   = 1000
	gkAuditTTL   = 5 * time.Second
)

// Logger provides security audit logging for MFA events.
type Logger struct {
	backend AuditBackend

	// KV persistence for Raft mode
	mu     sync.RWMutex
	events []*Event
	kvStore platformstore.KVStore
}

// AuditBackend defines where audit logs are written.
type AuditBackend interface {
	LogEvent(ctx context.Context, event *Event) error
	QueryEvents(ctx context.Context, filters map[string]interface{}) ([]*Event, error)
}

// Event represents a security audit log entry.
type Event struct {
	ID          uuid.UUID
	EventType   models.AuditEventType
	UserID      models.UserID
	FactorID    *models.FactorID
	ChallengeID *models.ChallengeID
	Severity    string // "info", "warning", "error"
	Message     string
	SourceIP    string
	UserAgent   string
	Timestamp   time.Time
	Metadata    map[string]interface{}
}

// NewLogger creates a new audit logger.
func NewLogger(backend AuditBackend) *Logger {
	return &Logger{
		backend: backend,
		events:  make([]*Event, 0, 128),
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

	ctx, cancel := context.WithTimeout(context.Background(), gkAuditTTL)
	defer cancel()

	val, err := kv.Get(ctx, gkAuditKVKey)
	if err != nil || val == "" {
		return // not found or empty
	}

	var events []*Event
	if err := json.Unmarshal([]byte(val), &events); err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  gatekeeper audit: failed to unmarshal events: %v", err))
		return
	}

	l.mu.Lock()
	l.events = events
	l.mu.Unlock()
	logging.Z().Info(fmt.Sprintf("✅ gatekeeper audit: loaded %d persistent events", len(events)))
}

func (l *Logger) saveToKV() {
	l.mu.RLock()
	kv := l.kvStore
	events := l.events
	l.mu.RUnlock()
	if kv == nil {
		return
	}

	// Cap persistent events to avoid large KV entries
	persistEvents := events
	if len(persistEvents) > gkAuditMax {
		persistEvents = persistEvents[len(persistEvents)-gkAuditMax:]
	}

	data, err := json.Marshal(persistEvents)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), gkAuditTTL)
	defer cancel()
	if err := kv.Put(ctx, gkAuditKVKey, string(data)); err != nil {
		logging.Z().Error(fmt.Sprintf("gatekeeper audit: kv persist failed: %v", err))
	}
}

// recordToBuffer adds an event to the in-memory buffer for KV persistence.
func (l *Logger) recordToBuffer(event *Event) {
	l.mu.Lock()
	if len(l.events) >= gkAuditMax {
		// Evict oldest 10%
		evict := gkAuditMax / 10
		if evict < 1 {
			evict = 1
		}
		l.events = l.events[evict:]
	}
	l.events = append(l.events, event)
	l.mu.Unlock()

	go l.saveToKV()
}

// LogEnrollment logs factor enrollment events.
func (l *Logger) LogEnrollment(ctx context.Context, userID models.UserID, factorID models.FactorID, factorType models.FactorType, sourceIP string) error {
	event := &Event{
		ID:        uuid.New(),
		EventType: models.AuditEventEnrolled,
		UserID:    userID,
		FactorID:  &factorID,
		Severity:  "info",
		Message:   "Factor enrolled: " + string(factorType),
		SourceIP:  sourceIP,
		Timestamp: time.Now().UTC(),
		Metadata: map[string]interface{}{
			"factor_type": string(factorType),
		},
	}

	l.recordToBuffer(event)
	return l.backend.LogEvent(ctx, event)
}

// LogVerification logs successful MFA verification.
func (l *Logger) LogVerification(ctx context.Context, userID models.UserID, factorID models.FactorID, challengeID models.ChallengeID, sourceIP, userAgent string) error {
	event := &Event{
		ID:          uuid.New(),
		EventType:   models.AuditEventVerified,
		UserID:      userID,
		FactorID:    &factorID,
		ChallengeID: &challengeID,
		Severity:    "info",
		Message:     "MFA verification successful",
		SourceIP:    sourceIP,
		UserAgent:   userAgent,
		Timestamp:   time.Now().UTC(),
	}

	l.recordToBuffer(event)
	return l.backend.LogEvent(ctx, event)
}

// LogVerificationFailure logs failed MFA verification attempts.
func (l *Logger) LogVerificationFailure(ctx context.Context, userID models.UserID, factorID models.FactorID, reason string, sourceIP string) error {
	event := &Event{
		ID:        uuid.New(),
		EventType: models.AuditEventVerificationFailed,
		UserID:    userID,
		FactorID:  &factorID,
		Severity:  "warning",
		Message:   "MFA verification failed: " + reason,
		SourceIP:  sourceIP,
		Timestamp: time.Now().UTC(),
		Metadata: map[string]interface{}{
			"reason": reason,
		},
	}

	l.recordToBuffer(event)
	return l.backend.LogEvent(ctx, event)
}

// LogDisabled logs factor disablement.
func (l *Logger) LogDisabled(ctx context.Context, userID models.UserID, factorID models.FactorID, sourceIP string) error {
	event := &Event{
		ID:        uuid.New(),
		EventType: models.AuditEventDisabled,
		UserID:    userID,
		FactorID:  &factorID,
		Severity:  "info",
		Message:   "Factor disabled",
		SourceIP:  sourceIP,
		Timestamp: time.Now().UTC(),
	}

	l.recordToBuffer(event)
	return l.backend.LogEvent(ctx, event)
}

// LogBackupCodeUsed logs backup code consumption.
func (l *Logger) LogBackupCodeUsed(ctx context.Context, userID models.UserID, factorID models.FactorID, sourceIP string) error {
	event := &Event{
		ID:        uuid.New(),
		EventType: models.AuditEventBackupCodeUsed,
		UserID:    userID,
		FactorID:  &factorID,
		Severity:  "warning",
		Message:   "Backup code consumed",
		SourceIP:  sourceIP,
		Timestamp: time.Now().UTC(),
	}

	l.recordToBuffer(event)
	return l.backend.LogEvent(ctx, event)
}

// LogHighRiskDetected logs high-risk authentication attempts.
func (l *Logger) LogHighRiskDetected(ctx context.Context, userID models.UserID, riskScore int, sourceIP, reason string) error {
	event := &Event{
		ID:        uuid.New(),
		EventType: models.AuditEventHighRiskDetected,
		UserID:    userID,
		Severity:  "warning",
		Message:   "High-risk authentication detected",
		SourceIP:  sourceIP,
		Timestamp: time.Now().UTC(),
		Metadata: map[string]interface{}{
			"risk_score": riskScore,
			"reason":     reason,
		},
	}

	l.recordToBuffer(event)
	return l.backend.LogEvent(ctx, event)
}

// QueryEvents retrieves audit events based on filters.
func (l *Logger) QueryEvents(ctx context.Context, filters map[string]interface{}) ([]*Event, error) {
	return l.backend.QueryEvents(ctx, filters)
}

// InMemoryBackend provides a simple in-memory audit backend for testing.
type InMemoryBackend struct {
	events []*Event
}

// NewInMemoryBackend creates a new in-memory audit backend.
func NewInMemoryBackend() *InMemoryBackend {
	return &InMemoryBackend{
		events: make([]*Event, 0),
	}
}

// LogEvent stores an event in memory.
func (b *InMemoryBackend) LogEvent(ctx context.Context, event *Event) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	b.events = append(b.events, event)
	return nil
}

// QueryEvents retrieves events matching the filter.
func (b *InMemoryBackend) QueryEvents(ctx context.Context, filters map[string]interface{}) ([]*Event, error) {
	result := make([]*Event, 0)

	for _, event := range b.events {
		// Simple filter matching (in production, use proper query language)
		if userID, ok := filters["user_id"].(models.UserID); ok {
			if event.UserID != userID {
				continue
			}
		}

		if eventType, ok := filters["event_type"].(models.AuditEventType); ok {
			if event.EventType != eventType {
				continue
			}
		}

		result = append(result, event)
	}

	return result, nil
}
