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
	iamAuditKVKey = "iam:audit:log"
	iamAuditMax   = 1000
	iamAuditTTL   = 5 * time.Second
)

// Event represents an IAM security audit log entry.
type Event struct {
	ID        uuid.UUID              `json:"id"`
	EventType string                 `json:"eventType"`
	UserID    string                 `json:"userID"`
	TargetID  string                 `json:"targetID,omitempty"`
	Category  string                 `json:"category"`
	Action    string                 `json:"action"`
	Severity  string                 `json:"severity"`
	Message   string                 `json:"message"`
	SourceIP  string                 `json:"sourceIP,omitempty"`
	UserAgent string                 `json:"userAgent,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Logger provides security audit logging for IAM events.
type Logger struct {
	mu      sync.RWMutex
	events  []*Event
	kvStore platformstore.KVStore
}

// NewLogger creates a new IAM audit logger.
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

	ctx, cancel := context.WithTimeout(context.Background(), iamAuditTTL)
	defer cancel()

	val, err := kv.Get(ctx, iamAuditKVKey)
	if err != nil || val == "" {
		return // not found or empty
	}

	var events []*Event
	if err := json.Unmarshal([]byte(val), &events); err != nil {
		logging.Z().Info(fmt.Sprintf("iam audit: failed to unmarshal events: %v", err))
		return
	}

	l.mu.Lock()
	l.events = events
	l.mu.Unlock()
	logging.Z().Info(fmt.Sprintf("iam audit: loaded %d persistent events", len(events)))
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
	if len(persistEvents) > iamAuditMax {
		persistEvents = persistEvents[len(persistEvents)-iamAuditMax:]
	}

	data, err := json.Marshal(persistEvents)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), iamAuditTTL)
	defer cancel()
	if err := kv.Put(ctx, iamAuditKVKey, string(data)); err != nil {
		logging.Z().Error(fmt.Sprintf("iam audit: kv persist failed: %v", err))
	}
}

func (l *Logger) recordToBuffer(event *Event) {
	l.mu.Lock()
	if len(l.events) >= iamAuditMax {
		evict := iamAuditMax / 10
		if evict < 1 {
			evict = 1
		}
		l.events = l.events[evict:]
	}
	l.events = append(l.events, event)
	l.mu.Unlock()

	go l.saveToKV()
}

// LogAuth logs an authentication attempt.
func (l *Logger) LogAuth(userID, outcome, sourceIP, userAgent string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "auth." + outcome,
		UserID:    userID,
		Category:  CategoryAuth,
		Action:    ActionLogin,
		Severity:  SeverityInfo,
		Message:   "Authentication " + outcome,
		SourceIP:  sourceIP,
		UserAgent: userAgent,
		Timestamp: time.Now().UTC(),
	})
}

// LogTokenIssued logs a token issuance event.
func (l *Logger) LogTokenIssued(userID, grantType, sourceIP string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "token.issued",
		UserID:    userID,
		Category:  CategoryToken,
		Action:    ActionCreated,
		Severity:  SeverityInfo,
		Message:   "Token issued: " + grantType,
		SourceIP:  sourceIP,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]interface{}{"grant_type": grantType},
	})
}

// LogTokenRevoked logs a token revocation event.
func (l *Logger) LogTokenRevoked(userID, tokenID, sourceIP string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "token.revoked",
		UserID:    userID,
		TargetID:  tokenID,
		Category:  CategoryToken,
		Action:    ActionRevoked,
		Severity:  SeverityWarning,
		Message:   "Token revoked",
		SourceIP:  sourceIP,
		Timestamp: time.Now().UTC(),
	})
}

// LogPermissionCheck logs a permission check.
func (l *Logger) LogPermissionCheck(userID, resource, outcome string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "permission." + outcome,
		UserID:    userID,
		TargetID:  resource,
		Category:  CategoryPermission,
		Action:    outcome,
		Severity:  SeverityInfo,
		Message:   "Permission " + outcome + " for " + resource,
		Timestamp: time.Now().UTC(),
	})
}

// LogUserCreated logs a user creation event.
func (l *Logger) LogUserCreated(adminID, newUserID, sourceIP string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "user.created",
		UserID:    adminID,
		TargetID:  newUserID,
		Category:  CategoryUser,
		Action:    ActionCreated,
		Severity:  SeverityInfo,
		Message:   "User created: " + newUserID,
		SourceIP:  sourceIP,
		Timestamp: time.Now().UTC(),
	})
}

// LogSessionCreated logs a session creation event.
func (l *Logger) LogSessionCreated(userID, sessionID, sourceIP string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "session.created",
		UserID:    userID,
		TargetID:  sessionID,
		Category:  CategorySession,
		Action:    ActionCreated,
		Severity:  SeverityInfo,
		Message:   "Session created",
		SourceIP:  sourceIP,
		Timestamp: time.Now().UTC(),
	})
}

// LogSessionRevoked logs a session revocation event.
func (l *Logger) LogSessionRevoked(userID, sessionID string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "session.revoked",
		UserID:    userID,
		TargetID:  sessionID,
		Category:  CategorySession,
		Action:    ActionRevoked,
		Severity:  SeverityWarning,
		Message:   "Session revoked",
		Timestamp: time.Now().UTC(),
	})
}

// LogRoleAssigned logs a role assignment event.
func (l *Logger) LogRoleAssigned(adminID, userID, roleID string) {
	l.recordToBuffer(&Event{
		ID:        uuid.New(),
		EventType: "role.assigned",
		UserID:    adminID,
		TargetID:  userID,
		Category:  CategoryRole,
		Action:    ActionAssigned,
		Severity:  SeverityInfo,
		Message:   "Role " + roleID + " assigned to " + userID,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]interface{}{"role_id": roleID},
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
		if filter.UserID != "" && ev.UserID != filter.UserID {
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
