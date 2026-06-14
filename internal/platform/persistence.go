package platform

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/bulk"
	"axiomnizam.bitbd.net/axiomnizam/internal/database"
	"axiomnizam.bitbd.net/axiomnizam/internal/eventbus"
	exportpkg "axiomnizam.bitbd.net/axiomnizam/internal/export"
	"axiomnizam.bitbd.net/axiomnizam/internal/lineage"
	"axiomnizam.bitbd.net/axiomnizam/internal/rbac"
	"axiomnizam.bitbd.net/axiomnizam/internal/streaming"
	"axiomnizam.bitbd.net/axiomnizam/internal/tenant"
	"axiomnizam.bitbd.net/axiomnizam/internal/tracing"
	"axiomnizam.bitbd.net/axiomnizam/internal/versioning"
	"axiomnizam.bitbd.net/axiomnizam/internal/webhooks"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	platformStateKeyBulk     = "platform:bulk"
	platformStateKeyEventBus = "platform:eventbus"
	platformStateKeyStreams  = "platform:streams"
	platformStateKeyWebhooks = "platform:webhooks"
	platformStateKeyTenants  = "platform:tenants"
	platformStateKeyVersion  = "platform:versioning"
	platformStateKeyExport   = "platform:exports"
	platformStateKeyRBAC     = "platform:rbac"
	platformStateKeyLineage  = "platform:lineage"
	platformStateKeyTracing  = "platform:tracing"
)

// platformStateStore persists JSON snapshots to etcd.
type platformStateStore struct {
	etcd   *clientv3.Client
	prefix string
}

func newPlatformStateStore(conns *database.Connections, prefix string) *platformStateStore {
	store := &platformStateStore{
		prefix: prefix,
	}
	if conns != nil {
		store.etcd = conns.Etcd
	}
	return store
}

func (s *platformStateStore) key(k string) string {
	if s.prefix == "" {
		return k
	}
	return s.prefix + ":" + k
}

func (s *platformStateStore) getRaw(ctx context.Context, key string) ([]byte, error) {
	nk := s.key(key)
	if s.etcd == nil {
		return nil, nil // no persistence backend — in-memory only
	}

	resp, err := s.etcd.Get(ctx, nk)
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, nil
	}
	return resp.Kvs[0].Value, nil
}

func (s *platformStateStore) putRaw(ctx context.Context, key string, payload []byte) error {
	nk := s.key(key)
	if s.etcd == nil {
		return nil // no persistence backend — in-memory only
	}

	_, err := s.etcd.Put(ctx, nk, string(payload))
	return err
}

func (s *platformStateStore) loadJSON(key string, out interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	payload, err := s.getRaw(ctx, key)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, out)
}

func (s *platformStateStore) saveJSON(key string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return s.putRaw(ctx, key, payload)
}

func platformID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// ---------- Bulk ----------

type persistentBulkState struct {
	Operations map[string]*bulk.BulkOperation         `json:"operations"`
	Results    map[string]*bulk.BulkOperationResponse `json:"results"`
}

type persistentBulkManager struct {
	mu    sync.RWMutex
	store *platformStateStore
	state persistentBulkState
}

func newPersistentBulkManager(store *platformStateStore) *persistentBulkManager {
	m := &persistentBulkManager{
		store: store,
		state: persistentBulkState{
			Operations: make(map[string]*bulk.BulkOperation),
			Results:    make(map[string]*bulk.BulkOperationResponse),
		},
	}
	if err := store.loadJSON(platformStateKeyBulk, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("bulk state load failed: %v", err))
	}
	if m.state.Operations == nil {
		m.state.Operations = make(map[string]*bulk.BulkOperation)
	}
	if m.state.Results == nil {
		m.state.Results = make(map[string]*bulk.BulkOperationResponse)
	}
	return m
}

func (m *persistentBulkManager) persist() {
	if err := m.store.saveJSON(platformStateKeyBulk, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("bulk state persist failed: %v", err))
	}
}

func (m *persistentBulkManager) SubmitOperation(op *bulk.BulkOperation) (*bulk.BulkOperation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if op.ID == "" {
		op.ID = platformID("bulk")
	}
	now := time.Now()
	op.Status = bulk.BulkOpPending
	op.CreatedAt = now
	op.UpdatedAt = now
	op.TotalItems = int64(len(op.Items))
	m.state.Operations[op.ID] = op
	m.persist()
	return op, nil
}

func (m *persistentBulkManager) GetOperation(id string) (*bulk.BulkOperation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	op, ok := m.state.Operations[id]
	if !ok {
		return nil, fmt.Errorf("operation not found")
	}
	return op, nil
}

func (m *persistentBulkManager) ListOperations(tenantID, status string) ([]*bulk.BulkOperation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*bulk.BulkOperation, 0, len(m.state.Operations))
	for _, op := range m.state.Operations {
		if tenantID != "" && op.TenantID != tenantID {
			continue
		}
		if status != "" && !strings.EqualFold(string(op.Status), status) {
			continue
		}
		res = append(res, op)
	}
	return res, nil
}

func (m *persistentBulkManager) CancelOperation(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	op, ok := m.state.Operations[id]
	if !ok {
		return fmt.Errorf("operation not found")
	}
	now := time.Now()
	op.Status = bulk.BulkOpCancelled
	op.CompletedAt = &now
	op.UpdatedAt = now
	m.persist()
	return nil
}

func (m *persistentBulkManager) RetryFailed(id string) (*bulk.BulkOperation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	op, ok := m.state.Operations[id]
	if !ok {
		return nil, fmt.Errorf("operation not found")
	}
	op.Status = bulk.BulkOpRunning
	op.UpdatedAt = time.Now()
	m.persist()
	return op, nil
}

func (m *persistentBulkManager) GetResults(id string) (*bulk.BulkOperationResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res, ok := m.state.Results[id]
	if !ok {
		return nil, fmt.Errorf("results not found")
	}
	return res, nil
}

// ---------- Event Bus ----------

type persistentEventBusState struct {
	Events        map[string]*eventbus.EventBusEvent     `json:"events"`
	Topics        map[string]*eventbus.EventTopic        `json:"topics"`
	Subscriptions map[string]*eventbus.EventSubscription `json:"subscriptions"`
	DLQ           map[string]*eventbus.DLQEvent          `json:"dlq"`
}

type persistentEventBusManager struct {
	mu    sync.RWMutex
	store *platformStateStore
	state persistentEventBusState
}

func newPersistentEventBusManager(store *platformStateStore) *persistentEventBusManager {
	m := &persistentEventBusManager{
		store: store,
		state: persistentEventBusState{
			Events:        make(map[string]*eventbus.EventBusEvent),
			Topics:        make(map[string]*eventbus.EventTopic),
			Subscriptions: make(map[string]*eventbus.EventSubscription),
			DLQ:           make(map[string]*eventbus.DLQEvent),
		},
	}
	if err := store.loadJSON(platformStateKeyEventBus, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("eventbus state load failed: %v", err))
	}
	if m.state.Events == nil {
		m.state.Events = make(map[string]*eventbus.EventBusEvent)
	}
	if m.state.Topics == nil {
		m.state.Topics = make(map[string]*eventbus.EventTopic)
	}
	if m.state.Subscriptions == nil {
		m.state.Subscriptions = make(map[string]*eventbus.EventSubscription)
	}
	if m.state.DLQ == nil {
		m.state.DLQ = make(map[string]*eventbus.DLQEvent)
	}
	return m
}

func (m *persistentEventBusManager) persist() {
	if err := m.store.saveJSON(platformStateKeyEventBus, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("eventbus state persist failed: %v", err))
	}
}

func (m *persistentEventBusManager) PublishEvent(event *eventbus.EventBusEvent) (*eventbus.EventPublishResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if event.ID == "" {
		event.ID = platformID("event")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	m.state.Events[event.ID] = event
	if topic := m.state.Topics[event.Type]; topic != nil {
		topic.MessageCount++
	}
	m.persist()
	return &eventbus.EventPublishResponse{EventID: event.ID, Timestamp: event.Timestamp, Topic: event.Type}, nil
}

func (m *persistentEventBusManager) ListEvents(tenantID, eventType, processed string) ([]*eventbus.EventBusEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*eventbus.EventBusEvent, 0, len(m.state.Events))
	for _, e := range m.state.Events {
		if tenantID != "" && e.TenantID != tenantID {
			continue
		}
		if eventType != "" && e.Type != eventType {
			continue
		}
		if processed == "true" && !e.IsProcessed {
			continue
		}
		if processed == "false" && e.IsProcessed {
			continue
		}
		res = append(res, e)
	}
	return res, nil
}

func (m *persistentEventBusManager) AckEvent(eventID, subscriptionID, acknowledgedBy, message string) (*eventbus.EventBusEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	event, ok := m.state.Events[eventID]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	now := time.Now()
	event.IsProcessed = true
	event.ProcessedAt = now

	if event.Metadata == nil {
		event.Metadata = map[string]string{}
	}
	if acknowledgedBy != "" {
		event.Metadata["acknowledgedBy"] = acknowledgedBy
	}
	event.Metadata["acknowledgedAt"] = now.UTC().Format(time.RFC3339)
	if message != "" {
		event.Metadata["ackMessage"] = message
	}
	if subscriptionID != "" {
		event.Metadata["subscriptionId"] = subscriptionID
		if sub, exists := m.state.Subscriptions[subscriptionID]; exists {
			sub.ProcessedCount++
			sub.LastProcessed = now
			sub.UpdatedAt = now
			if event.EventSequence > sub.Offset {
				sub.Offset = event.EventSequence
			}
		}
	}

	m.persist()
	return event, nil
}

func (m *persistentEventBusManager) CreateTopic(topic *eventbus.EventTopic) (*eventbus.EventTopic, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if topic.Name == "" {
		return nil, fmt.Errorf("topic name required")
	}
	if topic.CreatedAt.IsZero() {
		topic.CreatedAt = time.Now()
	}
	topic.IsActive = true
	m.state.Topics[topic.Name] = topic
	m.persist()
	return topic, nil
}

func (m *persistentEventBusManager) ListTopics() ([]*eventbus.EventTopic, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*eventbus.EventTopic, 0, len(m.state.Topics))
	for _, t := range m.state.Topics {
		res = append(res, t)
	}
	return res, nil
}

func (m *persistentEventBusManager) CreateSubscription(sub *eventbus.EventSubscription) (*eventbus.EventSubscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sub.ID == "" {
		sub.ID = platformID("sub")
	}
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = time.Now()
	}
	m.state.Subscriptions[sub.ID] = sub
	m.persist()
	return sub, nil
}

func (m *persistentEventBusManager) GetSubscription(id string) (*eventbus.EventSubscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sub, ok := m.state.Subscriptions[id]
	if !ok {
		return nil, fmt.Errorf("subscription not found")
	}
	return sub, nil
}

func (m *persistentEventBusManager) ListSubscriptions(tenantID string) ([]*eventbus.EventSubscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*eventbus.EventSubscription, 0, len(m.state.Subscriptions))
	for _, sub := range m.state.Subscriptions {
		if tenantID != "" && sub.TenantID != tenantID {
			continue
		}
		res = append(res, sub)
	}
	return res, nil
}

func (m *persistentEventBusManager) ListDLQEvents(tenantID string) ([]*eventbus.DLQEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*eventbus.DLQEvent, 0, len(m.state.DLQ))
	for _, e := range m.state.DLQ {
		if tenantID != "" && e.TenantID != tenantID {
			continue
		}
		res = append(res, e)
	}
	return res, nil
}

func (m *persistentEventBusManager) ReplayDLQEvent(dlqID, replayToTopic, replayedBy string) (*eventbus.EventPublishResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dlqEvent, ok := m.state.DLQ[dlqID]
	if !ok {
		return nil, fmt.Errorf("dlq event not found")
	}

	topic := replayToTopic
	if topic == "" {
		topic = dlqEvent.ReplayToTopic
	}
	if topic == "" {
		topic = dlqEvent.Topic
	}
	if topic == "" {
		topic = dlqEvent.Event.Type
	}
	if topic == "" {
		return nil, fmt.Errorf("replay topic is required")
	}

	now := time.Now()
	replayEvent := dlqEvent.Event
	replayEvent.ID = platformID("event")
	replayEvent.Type = topic
	replayEvent.Timestamp = now
	replayEvent.IsProcessed = false
	replayEvent.ProcessedAt = time.Time{}
	replayEvent.DeadLettered = false
	replayEvent.RetryCount = dlqEvent.FailureCount + 1
	if replayEvent.Metadata == nil {
		replayEvent.Metadata = map[string]string{}
	}
	replayEvent.Metadata["replayedFromDLQ"] = dlqID
	if replayedBy != "" {
		replayEvent.Metadata["replayedBy"] = replayedBy
	}

	m.state.Events[replayEvent.ID] = &replayEvent
	if topicState := m.state.Topics[topic]; topicState != nil {
		topicState.MessageCount++
	}

	dlqEvent.ManuallyResolved = true
	dlqEvent.ResolutionAction = "retry"
	dlqEvent.ResolutionTime = now
	dlqEvent.ReplayToTopic = topic

	m.persist()
	return &eventbus.EventPublishResponse{
		EventID:   replayEvent.ID,
		Timestamp: replayEvent.Timestamp,
		Topic:     replayEvent.Type,
	}, nil
}

// ---------- Streaming ----------

type persistentStreamState struct {
	Streams       map[string]*streaming.StreamSession      `json:"streams"`
	Subscriptions map[string]*streaming.StreamSubscription `json:"subscriptions"`
}

type persistentStreamManager struct {
	mu    sync.RWMutex
	store *platformStateStore
	state persistentStreamState
}

func newPersistentStreamManager(store *platformStateStore) *persistentStreamManager {
	m := &persistentStreamManager{
		store: store,
		state: persistentStreamState{
			Streams:       make(map[string]*streaming.StreamSession),
			Subscriptions: make(map[string]*streaming.StreamSubscription),
		},
	}
	if err := store.loadJSON(platformStateKeyStreams, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("stream state load failed: %v", err))
	}
	if m.state.Streams == nil {
		m.state.Streams = make(map[string]*streaming.StreamSession)
	}
	if m.state.Subscriptions == nil {
		m.state.Subscriptions = make(map[string]*streaming.StreamSubscription)
	}
	return m
}

func (m *persistentStreamManager) persist() {
	if err := m.store.saveJSON(platformStateKeyStreams, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("stream state persist failed: %v", err))
	}
}

func (m *persistentStreamManager) CreateStream(req *streaming.StreamRequest) (*streaming.StreamSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	session := &streaming.StreamSession{
		ID:           platformID("stream"),
		TenantID:     req.TenantID,
		RequestID:    req.RequestID,
		Query:        req.Query,
		Format:       req.Format,
		ChunkSize:    req.ChunkSize,
		CreatedAt:    now,
		LastActivity: now,
		Active:       true,
	}
	m.state.Streams[session.ID] = session
	m.persist()
	return session, nil
}

func (m *persistentStreamManager) GetStream(id string) (*streaming.StreamSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.state.Streams[id]
	if !ok {
		return nil, fmt.Errorf("stream not found")
	}
	return s, nil
}

func (m *persistentStreamManager) ListStreams(tenantID, status string) ([]*streaming.StreamSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*streaming.StreamSession, 0, len(m.state.Streams))
	for _, s := range m.state.Streams {
		if tenantID != "" && s.TenantID != tenantID {
			continue
		}
		if status == "active" && !s.Active {
			continue
		}
		if status == "inactive" && s.Active {
			continue
		}
		res = append(res, s)
	}
	return res, nil
}

func (m *persistentStreamManager) CancelStream(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.state.Streams[id]
	if !ok {
		return fmt.Errorf("stream not found")
	}
	s.Active = false
	s.LastActivity = time.Now()
	m.persist()
	return nil
}

func (m *persistentStreamManager) Subscribe(sub *streaming.StreamSubscription) (*streaming.StreamSubscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sub.ID == "" {
		sub.ID = platformID("stream-sub")
	}
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = time.Now()
	}
	sub.Active = true
	m.state.Subscriptions[sub.ID] = sub
	m.persist()
	return sub, nil
}

func (m *persistentStreamManager) Unsubscribe(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.Subscriptions, id)
	m.persist()
	return nil
}

// ---------- Webhooks ----------

type persistentWebhookState struct {
	Hooks map[string]*webhooks.Webhook              `json:"hooks"`
	Logs  map[string][]*webhooks.WebhookDeliveryLog `json:"logs"`
}

type persistentWebhookManager struct {
	mu    sync.RWMutex
	store *platformStateStore
	state persistentWebhookState
}

func newPersistentWebhookManager(store *platformStateStore) *persistentWebhookManager {
	m := &persistentWebhookManager{
		store: store,
		state: persistentWebhookState{
			Hooks: make(map[string]*webhooks.Webhook),
			Logs:  make(map[string][]*webhooks.WebhookDeliveryLog),
		},
	}
	if err := store.loadJSON(platformStateKeyWebhooks, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("webhook state load failed: %v", err))
	}
	if m.state.Hooks == nil {
		m.state.Hooks = make(map[string]*webhooks.Webhook)
	}
	if m.state.Logs == nil {
		m.state.Logs = make(map[string][]*webhooks.WebhookDeliveryLog)
	}
	return m
}

func (m *persistentWebhookManager) persist() {
	if err := m.store.saveJSON(platformStateKeyWebhooks, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("webhook state persist failed: %v", err))
	}
}

func (m *persistentWebhookManager) CreateWebhook(webhook *webhooks.Webhook) (*webhooks.Webhook, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if webhook.ID == "" {
		webhook.ID = platformID("webhook")
	}
	if webhook.CreatedAt.IsZero() {
		webhook.CreatedAt = time.Now()
	}
	m.state.Hooks[webhook.ID] = webhook
	if _, ok := m.state.Logs[webhook.ID]; !ok {
		m.state.Logs[webhook.ID] = []*webhooks.WebhookDeliveryLog{}
	}
	m.persist()
	return webhook, nil
}

func (m *persistentWebhookManager) GetWebhook(id string) (*webhooks.Webhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	wh, ok := m.state.Hooks[id]
	if !ok {
		return nil, fmt.Errorf("webhook not found")
	}
	return wh, nil
}

func (m *persistentWebhookManager) ListWebhooks(tenantID, eventType string) ([]*webhooks.Webhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*webhooks.Webhook, 0, len(m.state.Hooks))
	for _, wh := range m.state.Hooks {
		if tenantID != "" && wh.TenantID != tenantID {
			continue
		}
		if eventType != "" {
			matched := false
			for _, evt := range wh.Events {
				if string(evt) == eventType {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		res = append(res, wh)
	}
	return res, nil
}

func (m *persistentWebhookManager) UpdateWebhook(webhook *webhooks.Webhook) (*webhooks.Webhook, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.state.Hooks[webhook.ID]; !ok {
		return nil, fmt.Errorf("webhook not found")
	}
	webhook.UpdatedAt = time.Now()
	m.state.Hooks[webhook.ID] = webhook
	m.persist()
	return webhook, nil
}

func (m *persistentWebhookManager) DeleteWebhook(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.Hooks, id)
	delete(m.state.Logs, id)
	m.persist()
	return nil
}

func (m *persistentWebhookManager) TestWebhook(webhook *webhooks.Webhook) (interface{}, error) {
	return map[string]interface{}{"status": "ok", "latency": 100}, nil
}

func (m *persistentWebhookManager) GetDeliveryLogs(webhookID string) ([]*webhooks.WebhookDeliveryLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	logs, ok := m.state.Logs[webhookID]
	if !ok {
		return nil, fmt.Errorf("logs not found")
	}
	return logs, nil
}

// ---------- Tenant ----------

type persistentTenantState struct {
	Tenants map[string]*tenant.Tenant         `json:"tenants"`
	Members map[string][]*tenant.TenantMember `json:"members"`
	Quotas  map[string]*tenant.TenantQuota    `json:"quotas"`
}

type persistentTenantManager struct {
	mu    sync.RWMutex
	store *platformStateStore
	state persistentTenantState
}

func newPersistentTenantManager(store *platformStateStore) *persistentTenantManager {
	m := &persistentTenantManager{
		store: store,
		state: persistentTenantState{
			Tenants: make(map[string]*tenant.Tenant),
			Members: make(map[string][]*tenant.TenantMember),
			Quotas:  make(map[string]*tenant.TenantQuota),
		},
	}
	if err := store.loadJSON(platformStateKeyTenants, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("tenant state load failed: %v", err))
	}
	if m.state.Tenants == nil {
		m.state.Tenants = make(map[string]*tenant.Tenant)
	}
	if m.state.Members == nil {
		m.state.Members = make(map[string][]*tenant.TenantMember)
	}
	if m.state.Quotas == nil {
		m.state.Quotas = make(map[string]*tenant.TenantQuota)
	}
	return m
}

func (m *persistentTenantManager) persist() {
	if err := m.store.saveJSON(platformStateKeyTenants, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("tenant state persist failed: %v", err))
	}
}

func tenantIDFromName(name string) string {
	base := strings.TrimSpace(strings.ToLower(name))
	if base == "" {
		return platformID("tenant")
	}
	base = strings.ReplaceAll(base, " ", "-")
	return base + "-" + fmt.Sprintf("%d", time.Now().UnixNano())
}

func (m *persistentTenantManager) CreateTenant(ctx context.Context, t *tenant.Tenant, ownerID string) (*tenant.Tenant, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	if t.ID == "" {
		t.ID = tenantIDFromName(t.Name)
	}
	if _, ok := m.state.Tenants[t.ID]; ok {
		return nil, fmt.Errorf("tenant already exists: %s", t.ID)
	}

	now := time.Now()
	t.Owner = ownerID
	t.Status = tenant.TenantActive
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	m.state.Tenants[t.ID] = t

	m.state.Quotas[t.ID] = &tenant.TenantQuota{
		TenantID:      t.ID,
		MaxUsers:      10,
		MaxResources:  100,
		MaxQueries:    10000,
		MaxStorage:    1073741824,
		MaxAPIcalls:   100000,
		MaxConcurrent: 10,
		QueryTimeout:  300,
		ResetDate:     now.Add(24 * time.Hour),
	}

	m.state.Members[t.ID] = []*tenant.TenantMember{{
		ID:       fmt.Sprintf("%s-owner", t.ID),
		TenantID: t.ID,
		UserID:   ownerID,
		Role:     tenant.RoleOwner,
		Status:   tenant.MemberActive,
		JoinedAt: now,
	}}

	m.persist()
	return t, nil
}

func (m *persistentTenantManager) GetTenant(ctx context.Context, tenantID string) (*tenant.Tenant, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.state.Tenants[tenantID]
	if !ok {
		return nil, fmt.Errorf("tenant not found: %s", tenantID)
	}
	return t, nil
}

func (m *persistentTenantManager) UpdateTenant(ctx context.Context, t *tenant.Tenant) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.state.Tenants[t.ID]; !ok {
		return fmt.Errorf("tenant not found: %s", t.ID)
	}
	t.UpdatedAt = time.Now()
	m.state.Tenants[t.ID] = t
	m.persist()
	return nil
}

func (m *persistentTenantManager) DeleteTenant(ctx context.Context, tenantID string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.state.Tenants[tenantID]
	if !ok {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}
	t.Status = tenant.TenantArchived
	now := time.Now()
	t.DeletedAt = &now
	t.UpdatedAt = now
	m.persist()
	return nil
}

func (m *persistentTenantManager) ListTenants(ctx context.Context, ownerID string) ([]*tenant.Tenant, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*tenant.Tenant, 0, len(m.state.Tenants))
	for _, t := range m.state.Tenants {
		if ownerID != "" && t.Owner != ownerID {
			continue
		}
		if t.Status == tenant.TenantArchived {
			continue
		}
		res = append(res, t)
	}
	return res, nil
}

func (m *persistentTenantManager) GetQuota(ctx context.Context, tenantID string) (*tenant.TenantQuota, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	q, ok := m.state.Quotas[tenantID]
	if !ok {
		return nil, fmt.Errorf("quota not found: %s", tenantID)
	}
	return q, nil
}

func (m *persistentTenantManager) UpdateQuota(ctx context.Context, tenantID string, quota *tenant.TenantQuota) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.state.Quotas[tenantID]; !ok {
		return fmt.Errorf("quota not found: %s", tenantID)
	}
	m.state.Quotas[tenantID] = quota
	m.persist()
	return nil
}

func (m *persistentTenantManager) CheckQuota(ctx context.Context, tenantID string, quotaType string, requested int64) error {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	q, ok := m.state.Quotas[tenantID]
	if !ok {
		return fmt.Errorf("quota not found: %s", tenantID)
	}
	switch quotaType {
	case "users":
		if q.UsedUsers+int(requested) > q.MaxUsers {
			return fmt.Errorf("user quota exceeded: %d/%d", q.UsedUsers, q.MaxUsers)
		}
	case "resources":
		if q.UsedResources+int(requested) > q.MaxResources {
			return fmt.Errorf("resource quota exceeded: %d/%d", q.UsedResources, q.MaxResources)
		}
	case "queries":
		if q.UsedQueries+requested > q.MaxQueries {
			return fmt.Errorf("query quota exceeded: %d/%d", q.UsedQueries, q.MaxQueries)
		}
	case "storage":
		if q.UsedStorage+requested > q.MaxStorage {
			return fmt.Errorf("storage quota exceeded: %d/%d", q.UsedStorage, q.MaxStorage)
		}
	case "api":
		if q.UsedAPICalls+requested > q.MaxAPIcalls {
			return fmt.Errorf("API call quota exceeded: %d/%d", q.UsedAPICalls, q.MaxAPIcalls)
		}
	}
	return nil
}

func (m *persistentTenantManager) AddMember(ctx context.Context, tenantID, userID string, role tenant.MemberRole) (*tenant.TenantMember, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.state.Tenants[tenantID]; !ok {
		return nil, fmt.Errorf("tenant not found: %s", tenantID)
	}
	member := &tenant.TenantMember{
		ID:       fmt.Sprintf("%s-%s", tenantID, userID),
		TenantID: tenantID,
		UserID:   userID,
		Role:     role,
		Status:   tenant.MemberActive,
		JoinedAt: time.Now(),
	}
	m.state.Members[tenantID] = append(m.state.Members[tenantID], member)
	m.persist()
	return member, nil
}

func (m *persistentTenantManager) RemoveMember(ctx context.Context, tenantID, userID string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	members := m.state.Members[tenantID]
	updated := make([]*tenant.TenantMember, 0, len(members))
	found := false
	for _, member := range members {
		if member.UserID == userID {
			found = true
			continue
		}
		updated = append(updated, member)
	}
	if !found {
		return fmt.Errorf("member not found: %s", userID)
	}
	m.state.Members[tenantID] = updated
	m.persist()
	return nil
}

func (m *persistentTenantManager) ListMembers(ctx context.Context, tenantID string) ([]*tenant.TenantMember, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := m.state.Members[tenantID]
	out := make([]*tenant.TenantMember, len(items))
	copy(out, items)
	return out, nil
}

func (m *persistentTenantManager) UpdateMemberRole(ctx context.Context, tenantID, userID string, role tenant.MemberRole) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, member := range m.state.Members[tenantID] {
		if member.UserID == userID {
			member.Role = role
			m.persist()
			return nil
		}
	}
	return fmt.Errorf("member not found: %s", userID)
}

func (m *persistentTenantManager) CanAccess(ctx context.Context, tenantID, userID string) (bool, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, member := range m.state.Members[tenantID] {
		if member.UserID == userID && member.Status == tenant.MemberActive {
			return true, nil
		}
	}
	return false, nil
}

func (m *persistentTenantManager) GetIsolationStrategy(ctx context.Context, tenantID string) (tenant.TenantIsolation, error) {
	t, err := m.GetTenant(ctx, tenantID)
	if err != nil {
		return "", err
	}
	return t.IsolationLevel, nil
}

// ---------- Versioning ----------

type persistentVersionState struct {
	Versions  map[string][]*versioning.ResourceVersion `json:"versions"`
	Histories map[string]*versioning.VersionHistory    `json:"histories"`
	Snapshots map[string]*versioning.Snapshot          `json:"snapshots"`
}

type persistentVersionManager struct {
	mu    sync.RWMutex
	store *platformStateStore
	state persistentVersionState
}

func newPersistentVersionManager(store *platformStateStore) *persistentVersionManager {
	m := &persistentVersionManager{
		store: store,
		state: persistentVersionState{
			Versions:  make(map[string][]*versioning.ResourceVersion),
			Histories: make(map[string]*versioning.VersionHistory),
			Snapshots: make(map[string]*versioning.Snapshot),
		},
	}
	if err := store.loadJSON(platformStateKeyVersion, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("versioning state load failed: %v", err))
	}
	if m.state.Versions == nil {
		m.state.Versions = make(map[string][]*versioning.ResourceVersion)
	}
	if m.state.Histories == nil {
		m.state.Histories = make(map[string]*versioning.VersionHistory)
	}
	if m.state.Snapshots == nil {
		m.state.Snapshots = make(map[string]*versioning.Snapshot)
	}
	return m
}

func versionKey(resourceType, resourceID string) string {
	return resourceType + ":" + resourceID
}

func (m *persistentVersionManager) persist() {
	if err := m.store.saveJSON(platformStateKeyVersion, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("versioning state persist failed: %v", err))
	}
}

func (m *persistentVersionManager) GetVersion(resourceType, resourceID string, version int64) (*versioning.ResourceVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := m.state.Versions[versionKey(resourceType, resourceID)]
	for _, v := range items {
		if v.Version == version {
			return v, nil
		}
	}
	return nil, fmt.Errorf("version not found")
}

func (m *persistentVersionManager) ListVersions(resourceType, resourceID string) ([]*versioning.ResourceVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := m.state.Versions[versionKey(resourceType, resourceID)]
	if items == nil {
		return []*versioning.ResourceVersion{}, nil
	}
	return items, nil
}

func (m *persistentVersionManager) GetHistory(resourceType, resourceID string) (*versioning.VersionHistory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h := m.state.Histories[versionKey(resourceType, resourceID)]
	if h == nil {
		return nil, fmt.Errorf("history not found")
	}
	return h, nil
}

func (m *persistentVersionManager) GetDiff(resourceType, resourceID string, from, to int64) (*versioning.VersionDiff, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return &versioning.VersionDiff{FromVersion: from, ToVersion: to}, nil
}

func (m *persistentVersionManager) CreateSnapshot(snapshot *versioning.Snapshot) (*versioning.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if snapshot.ID == "" {
		snapshot.ID = platformID("snapshot")
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}

	key := versionKey(snapshot.ResourceType, snapshot.ResourceID)
	versions := m.state.Versions[key]
	nextVersion := int64(len(versions) + 1)
	snapshot.Version = nextVersion

	for _, v := range versions {
		v.IsLatest = false
		v.CurrentVersion = false
	}

	rv := &versioning.ResourceVersion{
		ID:             platformID("version"),
		TenantID:       snapshot.TenantID,
		ResourceType:   snapshot.ResourceType,
		ResourceID:     snapshot.ResourceID,
		Version:        nextVersion,
		CreatedAt:      snapshot.CreatedAt,
		Action:         versioning.ActionUpdate,
		Reason:         snapshot.Description,
		IsLatest:       true,
		CurrentVersion: true,
	}
	m.state.Versions[key] = append(versions, rv)
	m.state.Snapshots[snapshot.ID] = snapshot

	history := m.state.Histories[key]
	if history == nil {
		history = &versioning.VersionHistory{
			ResourceType: snapshot.ResourceType,
			ResourceID:   snapshot.ResourceID,
			TenantID:     snapshot.TenantID,
			CreatedAt:    snapshot.CreatedAt,
		}
	}
	history.LatestVersion = nextVersion
	history.CurrentVersion = nextVersion
	history.TotalVersions = int64(len(m.state.Versions[key]))
	history.UpdatedAt = time.Now()
	history.Versions = make([]versioning.ResourceVersion, 0, len(m.state.Versions[key]))
	for _, item := range m.state.Versions[key] {
		history.Versions = append(history.Versions, *item)
	}
	history.Snapshots = append(history.Snapshots, *snapshot)
	m.state.Histories[key] = history

	m.persist()
	return snapshot, nil
}

func (m *persistentVersionManager) Rollback(resourceType, resourceID string, targetVersion int64, reason string) (*versioning.RollbackResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := versionKey(resourceType, resourceID)
	versions := m.state.Versions[key]
	if len(versions) == 0 {
		return &versioning.RollbackResult{Success: true, ToVersion: targetVersion, CreatedAt: time.Now()}, nil
	}

	current := versions[len(versions)-1].Version
	found := false
	for _, v := range versions {
		if v.Version == targetVersion {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("target version not found")
	}

	versions = append(versions, &versioning.ResourceVersion{
		ID:             platformID("version"),
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		Version:        int64(len(versions) + 1),
		CreatedAt:      time.Now(),
		Action:         versioning.ActionRestore,
		Reason:         reason,
		IsLatest:       true,
		CurrentVersion: true,
	})
	for i := 0; i < len(versions)-1; i++ {
		versions[i].IsLatest = false
		versions[i].CurrentVersion = false
	}
	m.state.Versions[key] = versions

	h := m.state.Histories[key]
	if h != nil {
		h.LatestVersion = versions[len(versions)-1].Version
		h.CurrentVersion = targetVersion
		h.TotalVersions = int64(len(versions))
		h.UpdatedAt = time.Now()
	}

	m.persist()
	return &versioning.RollbackResult{Success: true, FromVersion: current, ToVersion: targetVersion, CreatedAt: time.Now()}, nil
}

// ---------- Lineage Core (used by adapter) ----------

type persistentLineageState struct {
	Nodes  map[string]*lineage.LineageNode  `json:"nodes"`
	Edges  map[string]*lineage.LineageEdge  `json:"edges"`
	Graphs map[string]*lineage.LineageGraph `json:"graphs"`
}

type persistentLineageCoreManager struct {
	mu    sync.RWMutex
	store *platformStateStore
	state persistentLineageState
}

func newPersistentLineageCoreManager(store *platformStateStore) *persistentLineageCoreManager {
	m := &persistentLineageCoreManager{
		store: store,
		state: persistentLineageState{
			Nodes:  make(map[string]*lineage.LineageNode),
			Edges:  make(map[string]*lineage.LineageEdge),
			Graphs: make(map[string]*lineage.LineageGraph),
		},
	}
	if err := store.loadJSON(platformStateKeyLineage, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("lineage state load failed: %v", err))
	}
	if m.state.Nodes == nil {
		m.state.Nodes = make(map[string]*lineage.LineageNode)
	}
	if m.state.Edges == nil {
		m.state.Edges = make(map[string]*lineage.LineageEdge)
	}
	if m.state.Graphs == nil {
		m.state.Graphs = make(map[string]*lineage.LineageGraph)
	}
	return m
}

func (m *persistentLineageCoreManager) persist() {
	if err := m.store.saveJSON(platformStateKeyLineage, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("lineage state persist failed: %v", err))
	}
}

func (m *persistentLineageCoreManager) GetNode(id string) (*lineage.LineageNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	node, ok := m.state.Nodes[id]
	if !ok {
		return nil, fmt.Errorf("node not found")
	}
	return node, nil
}

func (m *persistentLineageCoreManager) ListNodes(nodeType string) ([]*lineage.LineageNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*lineage.LineageNode, 0, len(m.state.Nodes))
	for _, node := range m.state.Nodes {
		if nodeType != "" && string(node.NodeType) != nodeType {
			continue
		}
		res = append(res, node)
	}
	return res, nil
}

func (m *persistentLineageCoreManager) BuildGraph(startNodeID string, direction string, depth int) (*lineage.LineageGraph, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	graph := &lineage.LineageGraph{
		ID:        platformID("graph"),
		RootNodes: []string{startNodeID},
		Depth:     depth,
		CreatedAt: time.Now(),
	}

	if root, ok := m.state.Nodes[startNodeID]; ok {
		graph.Nodes = append(graph.Nodes, *root)
	}

	for _, edge := range m.state.Edges {
		if edge.SourceNodeID == startNodeID || edge.TargetNodeID == startNodeID {
			graph.Edges = append(graph.Edges, *edge)
		}
	}

	m.state.Graphs[graph.ID] = graph
	m.persist()
	_ = direction
	return graph, nil
}

func (m *persistentLineageCoreManager) GetUpstream(nodeID string, depth int) ([]*lineage.LineageNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*lineage.LineageNode, 0)
	visited := make(map[string]bool)

	var walk func(string, int)
	walk = func(id string, d int) {
		if d <= 0 || visited[id] {
			return
		}
		visited[id] = true
		for _, edge := range m.state.Edges {
			if edge.TargetNodeID != id {
				continue
			}
			node, ok := m.state.Nodes[edge.SourceNodeID]
			if !ok {
				continue
			}
			result = append(result, node)
			walk(edge.SourceNodeID, d-1)
		}
	}

	walk(nodeID, depth)
	return result, nil
}

func (m *persistentLineageCoreManager) GetDownstream(nodeID string, depth int) ([]*lineage.LineageNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*lineage.LineageNode, 0)
	visited := make(map[string]bool)

	var walk func(string, int)
	walk = func(id string, d int) {
		if d <= 0 || visited[id] {
			return
		}
		visited[id] = true
		for _, edge := range m.state.Edges {
			if edge.SourceNodeID != id {
				continue
			}
			node, ok := m.state.Nodes[edge.TargetNodeID]
			if !ok {
				continue
			}
			result = append(result, node)
			walk(edge.TargetNodeID, d-1)
		}
	}

	walk(nodeID, depth)
	return result, nil
}

func (m *persistentLineageCoreManager) AnalyzeImpact(nodeID string) ([]*lineage.ImpactAnalysis, error) {
	downstream, err := m.GetDownstream(nodeID, 100)
	if err != nil {
		return nil, err
	}
	affected := make([]string, 0, len(downstream))
	for _, node := range downstream {
		affected = append(affected, node.ID)
	}
	return []*lineage.ImpactAnalysis{{
		SourceNodeID:      nodeID,
		AffectedNodeCount: len(affected),
		AffectedNodes:     affected,
		EstimatedImpact:   "high",
	}}, nil
}

func (m *persistentLineageCoreManager) GetColumnLineage(nodeID, columnName string) ([]*lineage.ColumnLineage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.state.Nodes[nodeID]; !ok {
		return nil, fmt.Errorf("node not found")
	}
	res := make([]*lineage.ColumnLineage, 0)
	for _, edge := range m.state.Edges {
		if edge.TargetNodeID != nodeID {
			continue
		}
		res = append(res, &lineage.ColumnLineage{
			ID:           platformID("col-lineage"),
			SourceColumn: fmt.Sprintf("%s.%s", edge.SourceNodeID, columnName),
			TargetColumn: fmt.Sprintf("%s.%s", nodeID, columnName),
			LastModified: time.Now(),
		})
	}
	return res, nil
}

func (m *persistentLineageCoreManager) GetLineageStatistics() (*lineage.LineageStatistics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return &lineage.LineageStatistics{
		TotalNodes: int64(len(m.state.Nodes)),
		TotalEdges: int64(len(m.state.Edges)),
	}, nil
}

// ---------- Tracing Core (used by adapter) ----------

type persistentTracingState struct {
	Traces   map[string]*tracing.Trace         `json:"traces"`
	Spans    map[string]*tracing.Span          `json:"spans"`
	Services map[string]*tracing.TraceMetrics  `json:"services"`
	Audits   []*tracing.TraceIngestionAuditLog `json:"audits"`
}

type persistentTracingCoreManager struct {
	mu    sync.RWMutex
	store *platformStateStore
	state persistentTracingState
}

func newPersistentTracingCoreManager(store *platformStateStore) *persistentTracingCoreManager {
	m := &persistentTracingCoreManager{
		store: store,
		state: persistentTracingState{
			Traces:   make(map[string]*tracing.Trace),
			Spans:    make(map[string]*tracing.Span),
			Services: make(map[string]*tracing.TraceMetrics),
			Audits:   make([]*tracing.TraceIngestionAuditLog, 0, 256),
		},
	}
	if err := store.loadJSON(platformStateKeyTracing, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("tracing state load failed: %v", err))
	}
	if m.state.Traces == nil {
		m.state.Traces = make(map[string]*tracing.Trace)
	}
	if m.state.Spans == nil {
		m.state.Spans = make(map[string]*tracing.Span)
	}
	if m.state.Services == nil {
		m.state.Services = make(map[string]*tracing.TraceMetrics)
	}
	if m.state.Audits == nil {
		m.state.Audits = make([]*tracing.TraceIngestionAuditLog, 0, 256)
	}
	return m
}

func (m *persistentTracingCoreManager) persist() {
	if err := m.store.saveJSON(platformStateKeyTracing, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("tracing state persist failed: %v", err))
	}
}

func normalizeTraceForPersistence(trace *tracing.Trace) {
	if trace == nil {
		return
	}

	now := time.Now().UTC()
	if trace.ID == "" {
		trace.ID = platformID("trace")
	}
	if trace.StartTime.IsZero() {
		trace.StartTime = now
	}
	if trace.EndTime.IsZero() {
		trace.EndTime = trace.StartTime
	}
	if trace.Duration <= 0 {
		trace.Duration = trace.EndTime.Sub(trace.StartTime).Milliseconds()
	}

	if trace.TotalSpans == 0 {
		trace.TotalSpans = len(trace.Spans)
	}

	serviceSet := make(map[string]bool)
	errorSpans := 0
	for i := range trace.Spans {
		span := &trace.Spans[i]
		if span.ID == "" {
			span.ID = platformID("span")
		}
		if span.TraceID == "" {
			span.TraceID = trace.ID
		}
		if span.TenantID == "" {
			span.TenantID = trace.TenantID
		}
		if span.Service != "" {
			serviceSet[span.Service] = true
		}
		if span.Error || span.Status == tracing.SpanStatusError {
			errorSpans++
		}
	}

	if len(trace.Services) == 0 && len(serviceSet) > 0 {
		trace.Services = make([]string, 0, len(serviceSet))
		for svc := range serviceSet {
			trace.Services = append(trace.Services, svc)
		}
	}

	trace.ErrorSpans = errorSpans
	if trace.Status == "" {
		if errorSpans > 0 {
			trace.Status = "error"
		} else {
			trace.Status = "success"
		}
	}
}

func recalculateTraceForPersistence(trace *tracing.Trace) {
	if trace == nil {
		return
	}

	serviceSet := make(map[string]bool)
	errorSpans := 0
	for _, span := range trace.Spans {
		if span.Service != "" {
			serviceSet[span.Service] = true
		}
		if span.Error || span.Status == tracing.SpanStatusError {
			errorSpans++
		}
	}

	trace.TotalSpans = len(trace.Spans)
	trace.ErrorSpans = errorSpans
	if errorSpans > 0 {
		trace.Status = "error"
	} else if trace.Status == "" || strings.EqualFold(trace.Status, "error") {
		trace.Status = "success"
	}

	trace.Services = trace.Services[:0]
	for svc := range serviceSet {
		trace.Services = append(trace.Services, svc)
	}

	if len(trace.Spans) > 0 {
		start := trace.Spans[0].StartTime
		end := trace.Spans[0].EndTime
		for _, span := range trace.Spans[1:] {
			if !span.StartTime.IsZero() && (start.IsZero() || span.StartTime.Before(start)) {
				start = span.StartTime
			}
			if span.EndTime.After(end) {
				end = span.EndTime
			}
		}
		if !start.IsZero() {
			trace.StartTime = start
		}
		if !end.IsZero() {
			trace.EndTime = end
		}
		if !trace.EndTime.IsZero() && !trace.StartTime.IsZero() {
			trace.Duration = trace.EndTime.Sub(trace.StartTime).Milliseconds()
		}
	}
}

func (m *persistentTracingCoreManager) IngestTrace(trace *tracing.Trace) (*tracing.Trace, error) {
	if trace == nil {
		return nil, fmt.Errorf("trace payload is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	normalizeTraceForPersistence(trace)
	m.state.Traces[trace.ID] = trace

	for i := range trace.Spans {
		span := trace.Spans[i]
		if span.ID == "" {
			span.ID = platformID("span")
		}
		if span.TraceID == "" {
			span.TraceID = trace.ID
		}
		if span.TenantID == "" {
			span.TenantID = trace.TenantID
		}
		trace.Spans[i] = span
		spanCopy := span
		m.state.Spans[span.ID] = &spanCopy
	}

	svc := traceServiceName(trace)
	if svc != "" {
		if m.state.Services[svc] == nil {
			m.state.Services[svc] = &tracing.TraceMetrics{Service: svc}
		}
		m.state.Services[svc].TraceCount++
		if trace.ErrorSpans > 0 || strings.EqualFold(trace.Status, "error") {
			m.state.Services[svc].ErrorTraceCount++
		}
	}

	m.persist()
	return trace, nil
}

func (m *persistentTracingCoreManager) IngestSpan(span *tracing.Span) (*tracing.Span, error) {
	if span == nil {
		return nil, fmt.Errorf("span payload is required")
	}
	if strings.TrimSpace(span.TraceID) == "" {
		return nil, fmt.Errorf("traceId is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	trace, exists := m.state.Traces[span.TraceID]
	if !exists {
		return nil, fmt.Errorf("trace not found")
	}

	now := time.Now().UTC()
	if span.ID == "" {
		span.ID = platformID("span")
	}
	if span.TenantID == "" {
		span.TenantID = trace.TenantID
	}
	if span.StartTime.IsZero() {
		span.StartTime = now
	}
	if span.EndTime.IsZero() {
		span.EndTime = span.StartTime
	}
	if span.Duration <= 0 {
		span.Duration = span.EndTime.Sub(span.StartTime).Microseconds()
	}

	spanCopy := *span
	m.state.Spans[spanCopy.ID] = &spanCopy

	updated := false
	for i := range trace.Spans {
		if trace.Spans[i].ID == spanCopy.ID {
			trace.Spans[i] = spanCopy
			updated = true
			break
		}
	}
	if !updated {
		trace.Spans = append(trace.Spans, spanCopy)
	}

	recalculateTraceForPersistence(trace)

	svc := traceServiceName(trace)
	if svc != "" {
		if m.state.Services[svc] == nil {
			m.state.Services[svc] = &tracing.TraceMetrics{Service: svc}
		}
		if spanCopy.Error || spanCopy.Status == tracing.SpanStatusError {
			m.state.Services[svc].ErrorTraceCount++
		}
	}

	m.persist()
	return &spanCopy, nil
}

func (m *persistentTracingCoreManager) RecordIngestionAudit(entry *tracing.TraceIngestionAuditLog) error {
	if entry == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if entry.ID == "" {
		entry.ID = platformID("trace-audit")
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	m.state.Audits = append(m.state.Audits, entry)
	if len(m.state.Audits) > 10000 {
		m.state.Audits = append([]*tracing.TraceIngestionAuditLog(nil), m.state.Audits[len(m.state.Audits)-10000:]...)
	}

	m.persist()
	return nil
}

func (m *persistentTracingCoreManager) ListIngestionAudits(filter *tracing.TraceIngestionAuditFilter) ([]*tracing.TraceIngestionAuditLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if filter == nil {
		filter = &tracing.TraceIngestionAuditFilter{}
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	results := make([]*tracing.TraceIngestionAuditLog, 0, limit)
	for i := len(m.state.Audits) - 1; i >= 0; i-- {
		entry := m.state.Audits[i]
		if filter.TenantID != "" && !strings.EqualFold(strings.TrimSpace(entry.TenantID), strings.TrimSpace(filter.TenantID)) {
			continue
		}
		if filter.Username != "" && !strings.EqualFold(strings.TrimSpace(entry.Username), strings.TrimSpace(filter.Username)) {
			continue
		}
		if filter.ResourceType != "" && !strings.EqualFold(strings.TrimSpace(entry.ResourceType), strings.TrimSpace(filter.ResourceType)) {
			continue
		}
		if filter.Result != "" && !strings.EqualFold(strings.TrimSpace(entry.Result), strings.TrimSpace(filter.Result)) {
			continue
		}

		results = append(results, entry)
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

func (m *persistentTracingCoreManager) GetTrace(id string) (*tracing.Trace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	trace, ok := m.state.Traces[id]
	if !ok {
		return nil, fmt.Errorf("trace not found")
	}
	return trace, nil
}

func traceServiceName(trace *tracing.Trace) string {
	if trace != nil && len(trace.Services) > 0 {
		return trace.Services[0]
	}
	return ""
}

func (m *persistentTracingCoreManager) SearchTraces(req *tracing.TraceSearchRequest) ([]*tracing.Trace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*tracing.Trace, 0, len(m.state.Traces))
	for _, item := range m.state.Traces {
		if req.Service != "" && traceServiceName(item) != req.Service {
			continue
		}
		if req.MinDuration > 0 && item.Duration < req.MinDuration {
			continue
		}
		res = append(res, item)
		if req.Limit > 0 && len(res) >= req.Limit {
			break
		}
	}
	return res, nil
}

func (m *persistentTracingCoreManager) GetSpan(id string) (*tracing.Span, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	span, ok := m.state.Spans[id]
	if !ok {
		return nil, fmt.Errorf("span not found")
	}
	return span, nil
}

func (m *persistentTracingCoreManager) GetServiceMap() (map[string][]*tracing.DependencyMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	deps := make(map[string][]*tracing.DependencyMetrics)
	for _, span := range m.state.Spans {
		if span.ParentSpanID == "" {
			continue
		}
		parent := m.state.Spans[span.ParentSpanID]
		if parent == nil {
			continue
		}
		deps[parent.Service] = append(deps[parent.Service], &tracing.DependencyMetrics{
			Source:      parent.Service,
			Destination: span.Service,
		})
	}
	return deps, nil
}

func (m *persistentTracingCoreManager) GetServiceMetrics(service string) (*tracing.TraceMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	metrics, ok := m.state.Services[service]
	if !ok {
		return nil, fmt.Errorf("service metrics not found")
	}
	return metrics, nil
}

func (m *persistentTracingCoreManager) GetOperationMetrics(service, operation string) (*tracing.SpanMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var (
		count int64
		total int64
		errs  int64
	)
	for _, span := range m.state.Spans {
		if span.Service != service || span.OperationName != operation {
			continue
		}
		count++
		total += span.Duration
		if span.Status == tracing.SpanStatusError {
			errs++
		}
	}
	if count == 0 {
		return nil, fmt.Errorf("no metrics found")
	}
	return &tracing.SpanMetrics{
		Service:         service,
		Operation:       operation,
		SpanCount:       count,
		ErrorSpanCount:  errs,
		AverageDuration: total / count,
	}, nil
}

func (m *persistentTracingCoreManager) AnalyzeErrors(service string) ([]*tracing.ErrorAnalysis, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	counts := make(map[string]int64)
	for _, span := range m.state.Spans {
		if span.Service == service && span.Error {
			counts[span.ErrorMessage]++
		}
	}
	res := make([]*tracing.ErrorAnalysis, 0, len(counts))
	for msg, count := range counts {
		res = append(res, &tracing.ErrorAnalysis{ErrorType: msg, Count: count})
	}
	return res, nil
}

// ---------- Export Core (used by adapter) ----------

type persistentExportState struct {
	Exports   map[string]*exportpkg.ExportJob      `json:"exports"`
	Templates map[string]*exportpkg.ExportTemplate `json:"templates"`
}

type persistentExportCoreManager struct {
	mu    sync.RWMutex
	store *platformStateStore
	state persistentExportState
}

func newPersistentExportCoreManager(store *platformStateStore) *persistentExportCoreManager {
	m := &persistentExportCoreManager{
		store: store,
		state: persistentExportState{
			Exports:   make(map[string]*exportpkg.ExportJob),
			Templates: make(map[string]*exportpkg.ExportTemplate),
		},
	}
	if err := store.loadJSON(platformStateKeyExport, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("export state load failed: %v", err))
	}
	if m.state.Exports == nil {
		m.state.Exports = make(map[string]*exportpkg.ExportJob)
	}
	if m.state.Templates == nil {
		m.state.Templates = make(map[string]*exportpkg.ExportTemplate)
	}
	return m
}

func (m *persistentExportCoreManager) persist() {
	if err := m.store.saveJSON(platformStateKeyExport, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("export state persist failed: %v", err))
	}
}

func (m *persistentExportCoreManager) SubmitExport(job *exportpkg.ExportJob) (*exportpkg.ExportJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job.ID == "" {
		job.ID = platformID("export")
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	job.Status = exportpkg.ExportPending
	job.Progress = 0
	m.state.Exports[job.ID] = job
	m.persist()
	return job, nil
}

func (m *persistentExportCoreManager) GetExport(id string) (*exportpkg.ExportJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.state.Exports[id]
	if !ok {
		return nil, fmt.Errorf("export not found")
	}
	return job, nil
}

func (m *persistentExportCoreManager) ListExports(tenantID string) ([]*exportpkg.ExportJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*exportpkg.ExportJob, 0, len(m.state.Exports))
	for _, job := range m.state.Exports {
		if tenantID != "" && job.TenantID != tenantID {
			continue
		}
		res = append(res, job)
	}
	return res, nil
}

func (m *persistentExportCoreManager) CancelExport(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.state.Exports[id]
	if !ok {
		return fmt.Errorf("export not found")
	}
	job.Status = exportpkg.ExportCancelled
	m.persist()
	return nil
}

func (m *persistentExportCoreManager) CreateTemplate(template *exportpkg.ExportTemplate) (*exportpkg.ExportTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if template.ID == "" {
		template.ID = platformID("template")
	}
	if template.CreatedAt.IsZero() {
		template.CreatedAt = time.Now()
	}
	m.state.Templates[template.ID] = template
	m.persist()
	return template, nil
}

func (m *persistentExportCoreManager) ListTemplates(tenantID string) ([]*exportpkg.ExportTemplate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*exportpkg.ExportTemplate, 0, len(m.state.Templates))
	for _, tpl := range m.state.Templates {
		if tenantID != "" && tpl.TenantID != tenantID {
			continue
		}
		res = append(res, tpl)
	}
	return res, nil
}

// ---------- RBAC Core (used by adapter) ----------

type persistentRBACState struct {
	Roles          map[string]*rbac.Role          `json:"roles"`
	Bindings       map[string]*rbac.RoleBinding   `json:"bindings"`
	Permissions    map[string]*rbac.Permission    `json:"permissions"`
	AccessRequests map[string]*rbac.AccessRequest `json:"accessRequests"`
}

type persistentRBACCoreManager struct {
	mu    sync.RWMutex
	store *platformStateStore
	state persistentRBACState
}

func newPersistentRBACCoreManager(store *platformStateStore) *persistentRBACCoreManager {
	m := &persistentRBACCoreManager{
		store: store,
		state: persistentRBACState{
			Roles:          make(map[string]*rbac.Role),
			Bindings:       make(map[string]*rbac.RoleBinding),
			Permissions:    make(map[string]*rbac.Permission),
			AccessRequests: make(map[string]*rbac.AccessRequest),
		},
	}
	if err := store.loadJSON(platformStateKeyRBAC, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("rbac state load failed: %v", err))
	}
	if m.state.Roles == nil {
		m.state.Roles = make(map[string]*rbac.Role)
	}
	if m.state.Bindings == nil {
		m.state.Bindings = make(map[string]*rbac.RoleBinding)
	}
	if m.state.Permissions == nil {
		m.state.Permissions = make(map[string]*rbac.Permission)
	}
	if m.state.AccessRequests == nil {
		m.state.AccessRequests = make(map[string]*rbac.AccessRequest)
	}
	return m
}

func (m *persistentRBACCoreManager) persist() {
	if err := m.store.saveJSON(platformStateKeyRBAC, &m.state); err != nil {
		logging.Z().Info(fmt.Sprintf("rbac state persist failed: %v", err))
	}
}

func (m *persistentRBACCoreManager) CreateRole(role *rbac.Role) (*rbac.Role, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if role.ID == "" {
		role.ID = platformID("role")
	}
	if role.CreatedAt.IsZero() {
		role.CreatedAt = time.Now()
	}
	role.PermissionCount = len(role.Permissions)
	for idx := range role.Permissions {
		perm := role.Permissions[idx]
		if perm.ID == "" {
			perm.ID = platformID("perm")
		}
		if perm.CreatedAt.IsZero() {
			perm.CreatedAt = time.Now()
		}
		if perm.TenantID == "" {
			perm.TenantID = role.TenantID
		}
		role.Permissions[idx] = perm
		permCopy := perm
		m.state.Permissions[perm.ID] = &permCopy
	}
	m.state.Roles[role.ID] = role
	m.persist()
	return role, nil
}

func (m *persistentRBACCoreManager) GetRole(id string) (*rbac.Role, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	role, ok := m.state.Roles[id]
	if !ok {
		return nil, fmt.Errorf("role not found")
	}
	return role, nil
}

func (m *persistentRBACCoreManager) ListRoles(tenantID string) ([]*rbac.Role, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*rbac.Role, 0, len(m.state.Roles))
	for _, role := range m.state.Roles {
		if tenantID != "" && role.TenantID != tenantID {
			continue
		}
		res = append(res, role)
	}
	return res, nil
}

func (m *persistentRBACCoreManager) UpdateRole(role *rbac.Role) (*rbac.Role, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.state.Roles[role.ID]; !ok {
		return nil, fmt.Errorf("role not found")
	}
	role.UpdatedAt = time.Now()
	role.PermissionCount = len(role.Permissions)
	m.state.Roles[role.ID] = role
	for idx := range role.Permissions {
		perm := role.Permissions[idx]
		if perm.ID == "" {
			perm.ID = platformID("perm")
		}
		if perm.CreatedAt.IsZero() {
			perm.CreatedAt = time.Now()
		}
		if perm.TenantID == "" {
			perm.TenantID = role.TenantID
		}
		permCopy := perm
		m.state.Permissions[perm.ID] = &permCopy
	}
	m.persist()
	return role, nil
}

func (m *persistentRBACCoreManager) DeleteRole(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.Roles, id)
	m.persist()
	return nil
}

func (m *persistentRBACCoreManager) CreateRoleBinding(binding *rbac.RoleBinding) (*rbac.RoleBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if binding.ID == "" {
		binding.ID = platformID("binding")
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = time.Now()
	}
	m.state.Bindings[binding.ID] = binding
	m.persist()
	return binding, nil
}

func (m *persistentRBACCoreManager) ListRoleBindings(roleID, subjectID string) ([]*rbac.RoleBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*rbac.RoleBinding, 0, len(m.state.Bindings))
	for _, binding := range m.state.Bindings {
		if roleID != "" && binding.RoleID != roleID {
			continue
		}
		if subjectID != "" && binding.PrincipalID != subjectID {
			continue
		}
		res = append(res, binding)
	}
	return res, nil
}

func (m *persistentRBACCoreManager) DeleteRoleBinding(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.Bindings, id)
	m.persist()
	return nil
}

func (m *persistentRBACCoreManager) CheckPermission(subjectID, resource, action string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, binding := range m.state.Bindings {
		if binding.PrincipalID != subjectID {
			continue
		}
		role, ok := m.state.Roles[binding.RoleID]
		if !ok {
			continue
		}
		for _, perm := range role.Permissions {
			if perm.Resource == resource && perm.Action == action {
				return true, nil
			}
		}
	}
	return false, nil
}

func (m *persistentRBACCoreManager) ListPermissions(roleID string) ([]*rbac.Permission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*rbac.Permission, 0, len(m.state.Permissions))
	for _, perm := range m.state.Permissions {
		if roleID != "" && perm.TenantID != roleID {
			continue
		}
		res = append(res, perm)
	}
	return res, nil
}

func (m *persistentRBACCoreManager) CreateAccessRequest(req *rbac.AccessRequest) (*rbac.AccessRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if req.ID == "" {
		req.ID = platformID("request")
	}
	if req.RequestedAt.IsZero() {
		req.RequestedAt = time.Now()
	}
	if req.Duration > 0 && req.ExpiresAt.IsZero() {
		req.ExpiresAt = req.RequestedAt.Add(time.Duration(req.Duration) * time.Second)
	}
	req.Status = rbac.RequestStatusPending
	m.state.AccessRequests[req.ID] = req
	m.persist()
	return req, nil
}

func (m *persistentRBACCoreManager) ListAccessRequests(tenantID, principalID, status string) ([]*rbac.AccessRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	statusFilter, hasStatusFilter := normalizeAccessRequestStatus(status)
	result := make([]*rbac.AccessRequest, 0, len(m.state.AccessRequests))
	changed := false

	for _, req := range m.state.AccessRequests {
		if maybeExpireAccessRequest(req, now) {
			changed = true
		}
		if tenantID != "" && req.TenantID != tenantID {
			continue
		}
		if principalID != "" && req.PrincipalID != principalID {
			continue
		}
		if hasStatusFilter && req.Status != statusFilter {
			continue
		}
		result = append(result, req)
	}

	if changed {
		m.persist()
	}

	return result, nil
}

func (m *persistentRBACCoreManager) ApproveAccessRequest(requestID, approverID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	req, ok := m.state.AccessRequests[requestID]
	if !ok {
		return fmt.Errorf("request not found")
	}
	if maybeExpireAccessRequest(req, time.Now()) {
		m.persist()
		return fmt.Errorf("request expired")
	}
	if req.Status != rbac.RequestStatusPending {
		return fmt.Errorf("request is not pending")
	}
	req.Status = rbac.RequestStatusApproved
	req.ApprovedAt = time.Now()
	req.ApprovedBy = approverID
	binding := &rbac.RoleBinding{
		ID:          platformID("binding"),
		TenantID:    req.TenantID,
		RoleID:      req.ResourceID,
		PrincipalID: req.PrincipalID,
		CreatedAt:   time.Now(),
	}
	m.state.Bindings[binding.ID] = binding
	m.persist()
	return nil
}

func (m *persistentRBACCoreManager) RejectAccessRequest(requestID, approverID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	req, ok := m.state.AccessRequests[requestID]
	if !ok {
		return fmt.Errorf("request not found")
	}
	if maybeExpireAccessRequest(req, time.Now()) {
		m.persist()
		return fmt.Errorf("request expired")
	}
	if req.Status != rbac.RequestStatusPending {
		return fmt.Errorf("request is not pending")
	}
	req.Status = rbac.RequestStatusRejected
	req.RejectedAt = time.Now()
	req.RejectionReason = reason
	if req.Metadata == nil {
		req.Metadata = map[string]interface{}{}
	}
	req.Metadata["rejectedBy"] = approverID
	m.persist()
	return nil
}

func (m *persistentRBACCoreManager) GetAccessRequest(id string) (*rbac.AccessRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	req, ok := m.state.AccessRequests[id]
	if !ok {
		return nil, fmt.Errorf("request not found")
	}
	if maybeExpireAccessRequest(req, time.Now()) {
		m.persist()
	}
	return req, nil
}

func normalizeAccessRequestStatus(status string) (rbac.RequestStatus, bool) {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "":
		return "", false
	case string(rbac.RequestStatusPending):
		return rbac.RequestStatusPending, true
	case string(rbac.RequestStatusApproved):
		return rbac.RequestStatusApproved, true
	case string(rbac.RequestStatusRejected):
		return rbac.RequestStatusRejected, true
	case string(rbac.RequestStatusExpired):
		return rbac.RequestStatusExpired, true
	case string(rbac.RequestStatusCancelled):
		return rbac.RequestStatusCancelled, true
	default:
		return "", false
	}
}

func maybeExpireAccessRequest(req *rbac.AccessRequest, now time.Time) bool {
	if req == nil {
		return false
	}
	if req.Status != rbac.RequestStatusPending {
		return false
	}
	if req.ExpiresAt.IsZero() {
		return false
	}
	if req.ExpiresAt.After(now) {
		return false
	}
	req.Status = rbac.RequestStatusExpired
	return true
}
