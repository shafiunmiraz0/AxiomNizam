package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
	"github.com/google/uuid"
)

const (
	auditKVKey   = "storage:audit:log"
	auditTimeout = 5 * time.Second
)

// AuditLog records and stores storage operation events.
type AuditLog struct {
	mu     sync.RWMutex
	events []models.StorageEvent
	max    int

	kvStore platformstore.KVStore
}

// NewAuditLog creates a new audit log with max event capacity.
func NewAuditLog(maxEvents int) *AuditLog {
	if maxEvents <= 0 {
		maxEvents = 10000
	}
	return &AuditLog{
		events: make([]models.StorageEvent, 0, 256),
		max:    maxEvents,
	}
}

// Record adds a new storage event to the audit log.
func (a *AuditLog) Record(ev models.StorageEvent) {
	if ev.ID == "" {
		ev.ID = uuid.New().String()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}

	a.mu.Lock()
	if len(a.events) >= a.max {
		evict := a.max / 10
		if evict < 1 {
			evict = 1
		}
		a.events = a.events[evict:]
	}
	a.events = append(a.events, ev)
	a.mu.Unlock()

	logging.Z().Info(fmt.Sprintf("Storage audit: %s tenant=%s bucket=%s key=%s", ev.Type, ev.TenantID, ev.Bucket, ev.Key))

	go a.save()
}

// ConfigureKVPersistence enables KVStore-backed persistence for the audit log.
func (a *AuditLog) ConfigureKVPersistence(kv platformstore.KVStore) {
	a.mu.Lock()
	a.kvStore = kv
	a.mu.Unlock()
	a.load()
}

func (a *AuditLog) load() {
	a.mu.Lock()
	kv := a.kvStore
	a.mu.Unlock()
	if kv == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), auditTimeout)
	defer cancel()

	val, err := kv.Get(ctx, auditKVKey)
	if err != nil || val == "" {
		return
	}

	var events []models.StorageEvent
	if err := json.Unmarshal([]byte(val), &events); err != nil {
		logging.Z().Info(fmt.Sprintf("storage audit: failed to unmarshal events: %v", err))
		return
	}

	a.mu.Lock()
	a.events = events
	a.mu.Unlock()
	logging.Z().Info(fmt.Sprintf("storage audit: loaded %d persistent events", len(events)))
}

func (a *AuditLog) save() {
	a.mu.RLock()
	kv := a.kvStore
	if kv == nil {
		a.mu.RUnlock()
		return
	}
	const maxPersistentEvents = 1000
	persistEvents := a.events
	if len(persistEvents) > maxPersistentEvents {
		persistEvents = persistEvents[len(persistEvents)-maxPersistentEvents:]
	}
	a.mu.RUnlock()

	data, err := json.Marshal(persistEvents)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), auditTimeout)
	defer cancel()
	if err := kv.Put(ctx, auditKVKey, string(data)); err != nil {
		logging.Z().Error(fmt.Sprintf("storage audit: kv persist failed: %v", err))
	}
}

// List returns events filtered by optional tenant and event type.
// Returns most recent first, up to limit.
func (a *AuditLog) List(tenantID, eventType string, limit int) []models.StorageEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	result := make([]models.StorageEvent, 0)
	for i := len(a.events) - 1; i >= 0 && len(result) < limit; i-- {
		ev := a.events[i]
		if tenantID != "" && ev.TenantID != tenantID {
			continue
		}
		if eventType != "" && ev.Type != eventType {
			continue
		}
		result = append(result, ev)
	}
	return result
}

// Count returns the total number of stored events.
func (a *AuditLog) Count() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.events)
}

// ListByBucket returns events for a specific bucket.
func (a *AuditLog) ListByBucket(bucket string, limit int) []models.StorageEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	result := make([]models.StorageEvent, 0)
	for i := len(a.events) - 1; i >= 0 && len(result) < limit; i-- {
		ev := a.events[i]
		if ev.Bucket == bucket {
			result = append(result, ev)
		}
	}
	return result
}
