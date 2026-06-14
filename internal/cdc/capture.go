package cdc

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ChangeEvent represents a data change event
type ChangeEvent struct {
	ID            string                 `json:"id"`
	Timestamp     time.Time              `json:"timestamp"`
	TableName     string                 `json:"table_name"`
	Operation     string                 `json:"operation"` // INSERT, UPDATE, DELETE
	BeforeData    map[string]interface{} `json:"before_data,omitempty"`
	AfterData     map[string]interface{} `json:"after_data,omitempty"`
	Metadata      map[string]string      `json:"metadata,omitempty"`
	SourceID      string                 `json:"source_id"`
	Sequence      int64                  `json:"sequence"`
	TransactionID string                 `json:"transaction_id,omitempty"`
}

// CDCStream represents a stream of changes
type CDCStream struct {
	ID            string
	TableName     string
	StartPosition int64
	Status        string // active, paused, stopped
	CreatedAt     time.Time
	LastUpdate    time.Time
}

// ChangeDataCapture manages CDC operations
type ChangeDataCapture struct {
	mu              sync.RWMutex
	streams         map[string]*CDCStream
	events          []*ChangeEvent
	webhooks        map[string]*WebhookSubscription
	subscribers     map[string][]chan *ChangeEvent
	eventSequence   int64
	maxEvents       int
	handlers        map[string]func(*ChangeEvent) error
	pollingInterval time.Duration
	lastPolledAt    map[string]time.Time
	eventBuffer     map[string][]*ChangeEvent
	bufferSize      int
	etcd            *clientv3.Client
	stateKey        string
}

type changeDataCaptureState struct {
	Streams         map[string]*CDCStream           `json:"streams"`
	Events          []*ChangeEvent                  `json:"events"`
	Webhooks        map[string]*WebhookSubscription `json:"webhooks"`
	EventSequence   int64                           `json:"event_sequence"`
	MaxEvents       int                             `json:"max_events"`
	PollingInterval time.Duration                   `json:"polling_interval"`
	LastPolledAt    map[string]time.Time            `json:"last_polled_at"`
	EventBuffer     map[string][]*ChangeEvent       `json:"event_buffer"`
	BufferSize      int                             `json:"buffer_size"`
}

// WebhookSubscription represents a webhook subscription
type WebhookSubscription struct {
	ID          string
	URL         string
	TableNames  []string
	EventTypes  []string
	Active      bool
	RetryPolicy *RetryPolicy
	CreatedAt   time.Time
	LastTriedAt time.Time
	FailCount   int
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxRetries        int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
}

// ChangeSubscription represents an event subscription
type ChangeSubscription struct {
	ID        string
	Channel   chan *ChangeEvent
	Filters   *SubscriptionFilter
	CreatedAt time.Time
}

// SubscriptionFilter filters events
type SubscriptionFilter struct {
	Tables     []string
	Operations []string
	UserID     string
}

// NewChangeDataCapture creates a new CDC instance
func NewChangeDataCapture(etcd ...*clientv3.Client) *ChangeDataCapture {
	var etcdClient *clientv3.Client
	if len(etcd) > 0 {
		etcdClient = etcd[0]
	}

	cdc := &ChangeDataCapture{
		streams:         make(map[string]*CDCStream),
		events:          make([]*ChangeEvent, 0),
		webhooks:        make(map[string]*WebhookSubscription),
		subscribers:     make(map[string][]chan *ChangeEvent),
		handlers:        make(map[string]func(*ChangeEvent) error),
		pollingInterval: 5 * time.Second,
		lastPolledAt:    make(map[string]time.Time),
		eventBuffer:     make(map[string][]*ChangeEvent),
		bufferSize:      1000,
		maxEvents:       100000,
		etcd:            etcdClient,
		stateKey:        "cdc:core:state",
	}
	cdc.loadState()
	return cdc
}

func (cdc *ChangeDataCapture) loadState() {
	if cdc.etcd == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := cdc.etcd.Get(ctx, cdc.stateKey)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("cdc-core: failed to load persisted state from etcd: %v", err))
		return
	}
	if len(resp.Kvs) == 0 {
		return
	}

	var state changeDataCaptureState
	if err := json.Unmarshal(resp.Kvs[0].Value, &state); err != nil {
		logging.Z().Info(fmt.Sprintf("cdc-core: failed to decode persisted state: %v", err))
		return
	}

	if state.Streams != nil {
		cdc.streams = state.Streams
	}
	if state.Events != nil {
		cdc.events = state.Events
	}
	if state.Webhooks != nil {
		cdc.webhooks = state.Webhooks
	}
	if state.LastPolledAt != nil {
		cdc.lastPolledAt = state.LastPolledAt
	}
	if state.EventBuffer != nil {
		cdc.eventBuffer = state.EventBuffer
	}
	cdc.eventSequence = state.EventSequence
	if state.MaxEvents > 0 {
		cdc.maxEvents = state.MaxEvents
	}
	if state.PollingInterval > 0 {
		cdc.pollingInterval = state.PollingInterval
	}
	if state.BufferSize > 0 {
		cdc.bufferSize = state.BufferSize
	}
}

func (cdc *ChangeDataCapture) persistStateLocked() {
	if cdc.etcd == nil {
		return
	}

	state := changeDataCaptureState{
		Streams:         cdc.streams,
		Events:          cdc.events,
		Webhooks:        cdc.webhooks,
		EventSequence:   cdc.eventSequence,
		MaxEvents:       cdc.maxEvents,
		PollingInterval: cdc.pollingInterval,
		LastPolledAt:    cdc.lastPolledAt,
		EventBuffer:     cdc.eventBuffer,
		BufferSize:      cdc.bufferSize,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("cdc-core: failed to encode state: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := cdc.etcd.Put(ctx, cdc.stateKey, string(payload)); err != nil {
		logging.Z().Info(fmt.Sprintf("cdc-core: failed to persist state to etcd: %v", err))
	}
}

// CaptureChange captures a data change event
func (cdc *ChangeDataCapture) CaptureChange(ctx context.Context, event *ChangeEvent) error {
	cdc.mu.Lock()
	defer cdc.mu.Unlock()

	if event.ID == "" {
		event.ID = fmt.Sprintf("cdc-%d", time.Now().UnixNano())
	}

	event.Timestamp = time.Now()
	event.Sequence = cdc.eventSequence
	cdc.eventSequence++

	// Store event
	cdc.events = append(cdc.events, event)
	if len(cdc.events) > cdc.maxEvents {
		cdc.events = cdc.events[1:]
	}

	// Buffer event
	if _, exists := cdc.eventBuffer[event.TableName]; !exists {
		cdc.eventBuffer[event.TableName] = make([]*ChangeEvent, 0)
	}
	cdc.eventBuffer[event.TableName] = append(cdc.eventBuffer[event.TableName], event)
	if len(cdc.eventBuffer[event.TableName]) > cdc.bufferSize {
		cdc.eventBuffer[event.TableName] = cdc.eventBuffer[event.TableName][1:]
	}

	// Update last polled
	cdc.lastPolledAt[event.TableName] = time.Now()
	cdc.persistStateLocked()

	return nil
}

// PublishChange publishes a change to subscribers and webhooks
func (cdc *ChangeDataCapture) PublishChange(event *ChangeEvent) error {
	cdc.mu.RLock()
	defer cdc.mu.RUnlock()

	// Publish to channel subscribers
	if subs, exists := cdc.subscribers[event.TableName]; exists {
		for _, ch := range subs {
			select {
			case ch <- event:
			default:
				// Channel full, drop event
			}
		}
	}

	// Publish to broadcast channel
	if broadcastSubs, exists := cdc.subscribers["*"]; exists {
		for _, ch := range broadcastSubs {
			select {
			case ch <- event:
			default:
			}
		}
	}

	// Trigger handlers
	if handler, exists := cdc.handlers[event.TableName]; exists {
		go func() {
			if err := handler(event); err != nil {
				fmt.Printf("Handler error: %v\n", err)
			}
		}()
	}

	// Publish to webhooks
	go cdc.publishToWebhooks(event)

	return nil
}

// Subscribe subscribes to change events
func (cdc *ChangeDataCapture) Subscribe(ctx context.Context, filter *SubscriptionFilter) (*ChangeSubscription, error) {
	cdc.mu.Lock()
	defer cdc.mu.Unlock()

	ch := make(chan *ChangeEvent, 100)
	sub := &ChangeSubscription{
		ID:        fmt.Sprintf("sub-%d", time.Now().UnixNano()),
		Channel:   ch,
		Filters:   filter,
		CreatedAt: time.Now(),
	}

	// Subscribe to each table
	if len(filter.Tables) == 0 {
		filter.Tables = []string{"*"}
	}

	for _, table := range filter.Tables {
		if _, exists := cdc.subscribers[table]; !exists {
			cdc.subscribers[table] = make([]chan *ChangeEvent, 0)
		}
		cdc.subscribers[table] = append(cdc.subscribers[table], ch)
	}

	return sub, nil
}

// Unsubscribe unsubscribes from events
func (cdc *ChangeDataCapture) Unsubscribe(sub *ChangeSubscription) error {
	cdc.mu.Lock()
	defer cdc.mu.Unlock()

	for _, table := range sub.Filters.Tables {
		if subs, exists := cdc.subscribers[table]; exists {
			for i, ch := range subs {
				if ch == sub.Channel {
					cdc.subscribers[table] = append(subs[:i], subs[i+1:]...)
					break
				}
			}
		}
	}

	close(sub.Channel)
	return nil
}

// RegisterHandler registers an event handler
func (cdc *ChangeDataCapture) RegisterHandler(tableName string, handler func(*ChangeEvent) error) {
	cdc.mu.Lock()
	defer cdc.mu.Unlock()

	cdc.handlers[tableName] = handler
}

// AddWebhook adds a webhook subscription
func (cdc *ChangeDataCapture) AddWebhook(webhook *WebhookSubscription) error {
	cdc.mu.Lock()
	defer cdc.mu.Unlock()

	if webhook.ID == "" {
		webhook.ID = fmt.Sprintf("wh-%d", time.Now().UnixNano())
	}

	webhook.CreatedAt = time.Now()

	if webhook.RetryPolicy == nil {
		webhook.RetryPolicy = &RetryPolicy{
			MaxRetries:        3,
			InitialBackoff:    time.Second,
			MaxBackoff:        time.Minute,
			BackoffMultiplier: 2,
		}
	}

	cdc.webhooks[webhook.ID] = webhook
	cdc.persistStateLocked()
	return nil
}

// RemoveWebhook removes a webhook
func (cdc *ChangeDataCapture) RemoveWebhook(webhookID string) error {
	cdc.mu.Lock()
	defer cdc.mu.Unlock()

	if _, exists := cdc.webhooks[webhookID]; !exists {
		return fmt.Errorf("webhook not found")
	}

	delete(cdc.webhooks, webhookID)
	cdc.persistStateLocked()
	return nil
}

// publishToWebhooks publishes event to webhooks
func (cdc *ChangeDataCapture) publishToWebhooks(event *ChangeEvent) {
	cdc.mu.RLock()
	webhooks := make([]*WebhookSubscription, 0)
	for _, wh := range cdc.webhooks {
		if cdc.webhookMatches(wh, event) {
			webhooks = append(webhooks, wh)
		}
	}
	cdc.mu.RUnlock()

	for _, wh := range webhooks {
		go cdc.deliverWebhook(wh, event)
	}
}

// webhookMatches checks if webhook matches event
func (cdc *ChangeDataCapture) webhookMatches(wh *WebhookSubscription, event *ChangeEvent) bool {
	if !wh.Active {
		return false
	}

	// Check table match
	tableMatches := len(wh.TableNames) == 0
	for _, table := range wh.TableNames {
		if table == event.TableName {
			tableMatches = true
			break
		}
	}

	if !tableMatches {
		return false
	}

	// Check operation match
	opMatches := len(wh.EventTypes) == 0
	for _, op := range wh.EventTypes {
		if op == event.Operation {
			opMatches = true
			break
		}
	}

	return opMatches
}

// deliverWebhook delivers webhook payload
func (cdc *ChangeDataCapture) deliverWebhook(wh *WebhookSubscription, event *ChangeEvent) {
	payload, _ := json.Marshal(event)

	// In production, implement actual HTTP delivery with retry logic
	fmt.Printf("Webhook %s: %s\n", wh.ID, string(payload))
}

// CreateStream creates a CDC stream
func (cdc *ChangeDataCapture) CreateStream(tableName string) (*CDCStream, error) {
	cdc.mu.Lock()
	defer cdc.mu.Unlock()

	streamID := fmt.Sprintf("stream-%s-%d", tableName, time.Now().UnixNano())

	stream := &CDCStream{
		ID:            streamID,
		TableName:     tableName,
		StartPosition: cdc.eventSequence,
		Status:        "active",
		CreatedAt:     time.Now(),
		LastUpdate:    time.Now(),
	}

	cdc.streams[streamID] = stream
	cdc.persistStateLocked()
	return stream, nil
}

// GetStreamEvents gets events from a stream
func (cdc *ChangeDataCapture) GetStreamEvents(streamID string, limit int) ([]*ChangeEvent, error) {
	cdc.mu.RLock()
	defer cdc.mu.RUnlock()

	stream, exists := cdc.streams[streamID]
	if !exists {
		return nil, fmt.Errorf("stream not found")
	}

	events := make([]*ChangeEvent, 0)

	for _, event := range cdc.events {
		if event.TableName == stream.TableName && event.Sequence >= stream.StartPosition {
			events = append(events, event)
			if len(events) >= limit {
				break
			}
		}
	}

	return events, nil
}

// GetChangeHistory gets change history for a table
func (cdc *ChangeDataCapture) GetChangeHistory(tableName string, limit int) []*ChangeEvent {
	cdc.mu.RLock()
	defer cdc.mu.RUnlock()

	events := make([]*ChangeEvent, 0)

	for _, event := range cdc.events {
		if event.TableName == tableName {
			events = append(events, event)
		}
	}

	// Return last 'limit' events
	if len(events) > limit {
		events = events[len(events)-limit:]
	}

	return events
}

// GetCDCStats returns CDC statistics
func (cdc *ChangeDataCapture) GetCDCStats() map[string]interface{} {
	cdc.mu.RLock()
	defer cdc.mu.RUnlock()

	insertCount := 0
	updateCount := 0
	deleteCount := 0

	for _, event := range cdc.events {
		switch event.Operation {
		case "INSERT":
			insertCount++
		case "UPDATE":
			updateCount++
		case "DELETE":
			deleteCount++
		}
	}

	return map[string]interface{}{
		"total_events":       len(cdc.events),
		"total_streams":      len(cdc.streams),
		"total_webhooks":     len(cdc.webhooks),
		"active_streams":     cdc.countActiveStreams(),
		"insert_events":      insertCount,
		"update_events":      updateCount,
		"delete_events":      deleteCount,
		"sequence_number":    cdc.eventSequence,
		"buffer_utilization": cdc.getBufferUtilization(),
	}
}

// countActiveStreams counts active streams
func (cdc *ChangeDataCapture) countActiveStreams() int {
	count := 0
	for _, stream := range cdc.streams {
		if stream.Status == "active" {
			count++
		}
	}
	return count
}

// getBufferUtilization returns buffer utilization percentage
func (cdc *ChangeDataCapture) getBufferUtilization() float64 {
	totalBuffered := 0
	for _, events := range cdc.eventBuffer {
		totalBuffered += len(events)
	}

	maxCapacity := cdc.bufferSize * len(cdc.eventBuffer)
	if maxCapacity == 0 {
		return 0
	}

	return float64(totalBuffered) / float64(maxCapacity) * 100
}
