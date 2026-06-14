package events

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// EventType is defined in event.go with common event types
// RecordedEvent and related types below

// RecordedEvent represents a recorded event for audit/tracing
type RecordedEvent struct {
	// Event metadata
	EventID   string    `json:"event_id"`
	EventType EventType `json:"event_type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`

	// Involved object
	InvolvedObject InvolvedObject `json:"involved_object"`

	// Context
	Action     string            `json:"action"`
	ReportedBy string            `json:"reported_by"`
	FirstTime  time.Time         `json:"first_time"`
	LastTime   time.Time         `json:"last_time"`
	Count      int64             `json:"count"`
	Metadata   map[string]string `json:"metadata"`

	// Related resources
	RelatedObjects []InvolvedObject `json:"related_objects,omitempty"`
}

// InvolvedObject describes the object involved in an event
type InvolvedObject struct {
	APIVersion string `json:"api_version"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	UID        string `json:"uid"`
}

// EventRecorder records events for resources
type EventRecorder interface {
	// Record records an event
	Record(ctx context.Context, resource resources.Resource, eventType EventType, reason, message string)

	// RecordWithMetadata records an event with metadata
	RecordWithMetadata(ctx context.Context, resource resources.Resource, eventType EventType, reason, message string, metadata map[string]string)

	// RecordRelated records an event with related objects
	RecordRelated(ctx context.Context, resource resources.Resource, eventType EventType, reason, message string, relatedObjects ...resources.Resource)

	// GetEvents returns recorded events for a resource
	GetEvents(ctx context.Context, resourceName, namespace string) []RecordedEvent

	// GetEventsByType returns recorded events by type
	GetEventsByType(ctx context.Context, resourceName, namespace string, eventType EventType) []RecordedEvent

	// GetRecentEvents returns recent events
	GetRecentEvents(ctx context.Context, duration time.Duration) []RecordedEvent

	// ClearEvents clears old events
	ClearEvents(ctx context.Context, before time.Time) error
}

// SimpleEventRecorder implements EventRecorder with in-memory storage
type SimpleEventRecorder struct {
	events  map[string][]*RecordedEvent
	maxSize int
	mu      sync.RWMutex
}

// NewEventRecorder creates a new event recorder
func NewEventRecorder() *SimpleEventRecorder {
	return &SimpleEventRecorder{
		events:  make(map[string][]*RecordedEvent),
		maxSize: 1000,
	}
}

// Record records an event
func (ser *SimpleEventRecorder) Record(ctx context.Context, resource resources.Resource, eventType EventType, reason, message string) {
	ser.RecordWithMetadata(ctx, resource, eventType, reason, message, nil)
}

// RecordWithMetadata records an event with metadata
func (ser *SimpleEventRecorder) RecordWithMetadata(ctx context.Context, resource resources.Resource, eventType EventType, reason, message string, metadata map[string]string) {
	meta := resource.GetObjectMeta()

	event := &RecordedEvent{
		EventID:   generateEventID(),
		EventType: eventType,
		Reason:    reason,
		Message:   message,
		Timestamp: time.Now(),
		InvolvedObject: InvolvedObject{
			Kind:      "Resource",
			Name:      meta.Name,
			Namespace: meta.Namespace,
			UID:       meta.UID,
		},
		ReportedBy: "axiom-nizam",
		FirstTime:  time.Now(),
		LastTime:   time.Now(),
		Count:      1,
		Metadata:   metadata,
	}

	key := fmt.Sprintf("%s/%s", meta.Namespace, meta.Name)

	ser.mu.Lock()
	defer ser.mu.Unlock()

	// Check if similar event exists
	if events, exists := ser.events[key]; exists {
		for _, e := range events {
			if e.EventType == eventType && e.Reason == reason {
				e.Count++
				e.LastTime = time.Now()
				return
			}
		}
	}

	// Add new event
	if ser.events[key] == nil {
		ser.events[key] = make([]*RecordedEvent, 0)
	}
	ser.events[key] = append(ser.events[key], event)

	// Enforce size limit
	if len(ser.events[key]) > ser.maxSize {
		ser.events[key] = ser.events[key][1:]
	}

	// Publish event to bus (bus field removed, event can be stored separately)
	eventData := map[string]interface{}{
		"resource_name": meta.Name,
		"namespace":     meta.Namespace,
		"reason":        reason,
		"message":       message,
	}
	_ = eventData // event data prepared but bus not configured
}

// RecordRelated records an event with related objects
func (ser *SimpleEventRecorder) RecordRelated(ctx context.Context, resource resources.Resource, eventType EventType, reason, message string, relatedObjects ...resources.Resource) {
	meta := resource.GetObjectMeta()

	relatedInvolvedObjects := make([]InvolvedObject, 0, len(relatedObjects))
	for _, related := range relatedObjects {
		relMeta := related.GetObjectMeta()
		relatedInvolvedObjects = append(relatedInvolvedObjects, InvolvedObject{
			Kind:      "Resource",
			Name:      relMeta.Name,
			Namespace: relMeta.Namespace,
			UID:       relMeta.UID,
		})
	}

	event := &RecordedEvent{
		EventID:   generateEventID(),
		EventType: eventType,
		Reason:    reason,
		Message:   message,
		Timestamp: time.Now(),
		InvolvedObject: InvolvedObject{
			Kind:      "Resource",
			Name:      meta.Name,
			Namespace: meta.Namespace,
			UID:       meta.UID,
		},
		ReportedBy:     "axiom-nizam",
		FirstTime:      time.Now(),
		LastTime:       time.Now(),
		Count:          1,
		RelatedObjects: relatedInvolvedObjects,
	}

	key := fmt.Sprintf("%s/%s", meta.Namespace, meta.Name)

	ser.mu.Lock()
	defer ser.mu.Unlock()

	if ser.events[key] == nil {
		ser.events[key] = make([]*RecordedEvent, 0)
	}
	ser.events[key] = append(ser.events[key], event)

	if len(ser.events[key]) > ser.maxSize {
		ser.events[key] = ser.events[key][1:]
	}

	logging.Z().Info(fmt.Sprintf("recorded related event: %s/%s %s with %d related objects", meta.Namespace, meta.Name, reason, len(relatedObjects)))
}

// GetEvents returns recorded events for a resource
func (ser *SimpleEventRecorder) GetEvents(ctx context.Context, resourceName, namespace string) []RecordedEvent {
	key := fmt.Sprintf("%s/%s", namespace, resourceName)

	ser.mu.RLock()
	defer ser.mu.RUnlock()

	events := ser.events[key]
	if events == nil {
		return []RecordedEvent{}
	}

	result := make([]RecordedEvent, len(events))
	for i, e := range events {
		result[i] = *e
	}
	return result
}

// GetEventsByType returns recorded events by type
func (ser *SimpleEventRecorder) GetEventsByType(ctx context.Context, resourceName, namespace string, eventType EventType) []RecordedEvent {
	key := fmt.Sprintf("%s/%s", namespace, resourceName)

	ser.mu.RLock()
	defer ser.mu.RUnlock()

	events := ser.events[key]
	if events == nil {
		return []RecordedEvent{}
	}

	result := make([]RecordedEvent, 0)
	for _, e := range events {
		if e.EventType == eventType {
			result = append(result, *e)
		}
	}
	return result
}

// GetRecentEvents returns recent events
func (ser *SimpleEventRecorder) GetRecentEvents(ctx context.Context, duration time.Duration) []RecordedEvent {
	cutoff := time.Now().Add(-duration)

	ser.mu.RLock()
	defer ser.mu.RUnlock()

	result := make([]RecordedEvent, 0)
	for _, events := range ser.events {
		for _, e := range events {
			if e.Timestamp.After(cutoff) {
				result = append(result, *e)
			}
		}
	}

	return result
}

// ClearEvents clears old events
func (ser *SimpleEventRecorder) ClearEvents(ctx context.Context, before time.Time) error {
	ser.mu.Lock()
	defer ser.mu.Unlock()

	for key, events := range ser.events {
		filtered := make([]*RecordedEvent, 0)
		for _, e := range events {
			if e.Timestamp.After(before) {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			delete(ser.events, key)
		} else {
			ser.events[key] = filtered
		}
	}

	return nil
}

// EventFactory provides helper functions to create events
type EventFactory struct {
	recorder EventRecorder
}

// NewEventFactory creates a new event factory
func NewEventFactory(recorder EventRecorder) *EventFactory {
	return &EventFactory{
		recorder: recorder,
	}
}

// RecordAPICreated records API creation event
func (ef *EventFactory) RecordAPICreated(ctx context.Context, resource resources.Resource) {
	ef.recorder.Record(ctx, resource, EventTypeAPICreated, "Created", "API resource created")
}

// RecordAPIUpdated records API update event
func (ef *EventFactory) RecordAPIUpdated(ctx context.Context, resource resources.Resource) {
	ef.recorder.Record(ctx, resource, EventTypeAPIUpdated, "Updated", "API resource updated")
}

// RecordAPIDeleted records API deletion event
func (ef *EventFactory) RecordAPIDeleted(ctx context.Context, resource resources.Resource) {
	ef.recorder.Record(ctx, resource, EventTypeAPIDeleted, "Deleted", "API resource deleted")
}

// RecordPolicyAdmitted records policy admission event
func (ef *EventFactory) RecordPolicyAdmitted(ctx context.Context, resource resources.Resource, message string) {
	ef.recorder.Record(ctx, resource, EventTypePolicyAdmitted, "Admitted", message)
}

// RecordPolicyDenied records policy denial event
func (ef *EventFactory) RecordPolicyDenied(ctx context.Context, resource resources.Resource, reason, message string) {
	ef.recorder.RecordWithMetadata(ctx, resource, EventTypePolicyDenied, reason, message, map[string]string{
		"action": "denied",
	})
}

// RecordReconcileSuccess records successful reconciliation event
func (ef *EventFactory) RecordReconcileSuccess(ctx context.Context, resource resources.Resource, duration time.Duration) {
	ef.recorder.RecordWithMetadata(ctx, resource, EventTypeReconcileSuccess, "Reconciled", "reconciliation successful", map[string]string{
		"duration": duration.String(),
	})
}

// RecordReconcileFailed records failed reconciliation event
func (ef *EventFactory) RecordReconcileFailed(ctx context.Context, resource resources.Resource, err string) {
	ef.recorder.Record(ctx, resource, EventTypeReconcileFailed, "ReconciliationFailed", err)
}

// RecordSyncSuccess records successful sync event
func (ef *EventFactory) RecordSyncSuccess(ctx context.Context, resource resources.Resource) {
	ef.recorder.Record(ctx, resource, EventTypeSyncSuccess, "Synced", "resource synchronized successfully")
}

// RecordSyncFailed records failed sync event
func (ef *EventFactory) RecordSyncFailed(ctx context.Context, resource resources.Resource, reason string) {
	ef.recorder.Record(ctx, resource, EventTypeSyncFailed, "SyncFailed", reason)
}

// generateEventID is defined in event.go
