package conductor

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/conductor/config"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Manager orchestrates producers, consumers, and message flow across backends.
type Manager struct {
	mu sync.RWMutex
	db *gorm.DB

	rabbitmq *RabbitMQBackend
	kafka    *KafkaBackend

	producers map[string]*Producer
	consumers map[string]*Consumer
	messages  map[string]*Message // recent messages (ring-buffer style)
	dlq       map[string]*DLQEntry
	stream    []Message // last N messages for live streaming

	maxStream   int
	maxMessages int
}

// Config re-exports the config type from the config subpackage.
type Config = config.Config

// NewManager creates a conductor manager.
func NewManager(cfg Config) *Manager {
	m := &Manager{
		producers:   make(map[string]*Producer),
		consumers:   make(map[string]*Consumer),
		messages:    make(map[string]*Message),
		dlq:         make(map[string]*DLQEntry),
		maxStream:   cfg.MaxStream,
		maxMessages: cfg.MaxMessages,
	}
	if m.maxStream == 0 {
		m.maxStream = 500
	}

	if cfg.RabbitMQURL != "" {
		m.rabbitmq = NewRabbitMQBackend(cfg.RabbitMQURL)
		if err := m.rabbitmq.Connect(); err != nil {
			logging.Z().Info(fmt.Sprintf("[conductor] rabbitmq connect warning: %v (will retry on first use)", err))
		}
	}
	if len(cfg.KafkaBrokers) > 0 {
		m.kafka = NewKafkaBackend(cfg.KafkaBrokers)
	}

	return m
}

// Close shuts down all backends.
func (m *Manager) Close() {
	// flush all producer/consumer stats to DB before shutdown
	m.mu.RLock()
	for _, p := range m.producers {
		m.saveProducer(p)
	}
	for _, c := range m.consumers {
		m.saveConsumer(c)
	}
	m.mu.RUnlock()

	if m.rabbitmq != nil {
		m.rabbitmq.Close()
	}
	if m.kafka != nil {
		m.kafka.Close()
	}
}

// ---------------------------------------------------------------
// Producer CRUD
// ---------------------------------------------------------------

// CreateProducer registers a new producer.
func (m *Manager) CreateProducer(req *CreateProducerRequest) (*Producer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("prod-%d", time.Now().UnixNano())
	ct := req.ContentType
	if ct == "" {
		ct = "application/json"
	}

	p := &Producer{
		ID:          id,
		Name:        req.Name,
		Backend:     req.Backend,
		Exchange:    req.Exchange,
		RoutingKey:  req.RoutingKey,
		Topic:       req.Topic,
		ContentType: ct,
		Headers:     req.Headers,
		Status:      StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Config:      req.Config,
	}

	// Ensure backend resources exist
	switch req.Backend {
	case BackendRabbitMQ:
		if m.rabbitmq == nil {
			return nil, fmt.Errorf("rabbitmq backend not configured")
		}
		if err := m.rabbitmq.Connect(); err != nil {
			return nil, fmt.Errorf("rabbitmq connection failed: %w — check RABBITMQ_URL and that the broker is reachable", err)
		}
		if p.Exchange != "" {
			if err := m.rabbitmq.EnsureExchange(p.Exchange, "topic"); err != nil {
				return nil, fmt.Errorf("rabbitmq exchange setup failed: %w", err)
			}
		}
	case BackendKafka:
		if m.kafka == nil {
			return nil, fmt.Errorf("kafka backend not configured")
		}
	case BackendMemory:
		// no-op
	default:
		return nil, fmt.Errorf("unsupported backend: %s", req.Backend)
	}

	m.producers[id] = p
	m.saveProducer(p)
	return p, nil
}

// GetProducer returns a single producer.
func (m *Manager) GetProducer(id string) (*Producer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.producers[id]
	if !ok {
		return nil, fmt.Errorf("producer not found: %s", id)
	}
	return p, nil
}

// ListProducers returns all producers.
func (m *Manager) ListProducers() []*Producer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Producer, 0, len(m.producers))
	for _, p := range m.producers {
		out = append(out, p)
	}
	return out
}

// UpdateProducer patches a producer.
func (m *Manager) UpdateProducer(id string, req *CreateProducerRequest) (*Producer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.producers[id]
	if !ok {
		return nil, fmt.Errorf("producer not found: %s", id)
	}
	if req.Name != "" {
		p.Name = req.Name
	}
	if req.Exchange != "" {
		p.Exchange = req.Exchange
	}
	if req.RoutingKey != "" {
		p.RoutingKey = req.RoutingKey
	}
	if req.Topic != "" {
		p.Topic = req.Topic
	}
	if req.ContentType != "" {
		p.ContentType = req.ContentType
	}
	if req.Headers != nil {
		p.Headers = req.Headers
	}
	p.Config = req.Config
	p.UpdatedAt = time.Now()
	m.saveProducer(p)
	return p, nil
}

// UpdateConsumer patches a consumer.
func (m *Manager) UpdateConsumer(id string, req *CreateConsumerRequest) (*Consumer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.consumers[id]
	if !ok {
		return nil, fmt.Errorf("consumer not found: %s", id)
	}
	if req.Name != "" {
		c.Name = req.Name
	}
	if req.Queue != "" {
		c.Queue = req.Queue
	}
	if req.Exchange != "" {
		c.Exchange = req.Exchange
	}
	if req.RoutingKey != "" {
		c.RoutingKey = req.RoutingKey
	}
	if req.Topic != "" {
		c.Topic = req.Topic
	}
	if req.ConsumerGroup != "" {
		c.ConsumerGroup = req.ConsumerGroup
	}
	c.Config = req.Config
	c.UpdatedAt = time.Now()
	m.saveConsumer(c)
	return c, nil
}

// DeleteProducer removes a producer.
func (m *Manager) DeleteProducer(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.producers[id]
	if !ok {
		return fmt.Errorf("producer not found: %s", id)
	}
	switch p.Backend {
	case BackendKafka:
		if m.kafka != nil {
			m.kafka.StopProducer(id)
		}
	}
	delete(m.producers, id)
	m.deleteProducerDB(id)
	return nil
}

// PauseProducer pauses a producer.
func (m *Manager) PauseProducer(id string) (*Producer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.producers[id]
	if !ok {
		return nil, fmt.Errorf("producer not found: %s", id)
	}
	p.Status = StatusPaused
	p.UpdatedAt = time.Now()
	m.saveProducer(p)
	return p, nil
}

// ResumeProducer resumes a paused producer.
func (m *Manager) ResumeProducer(id string) (*Producer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.producers[id]
	if !ok {
		return nil, fmt.Errorf("producer not found: %s", id)
	}
	p.Status = StatusActive
	p.UpdatedAt = time.Now()
	m.saveProducer(p)
	return p, nil
}

// ---------------------------------------------------------------
// Consumer CRUD
// ---------------------------------------------------------------

// CreateConsumer registers and starts a consumer.
func (m *Manager) CreateConsumer(req *CreateConsumerRequest) (*Consumer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("cons-%d", time.Now().UnixNano())
	c := &Consumer{
		ID:            id,
		Name:          req.Name,
		Backend:       req.Backend,
		Queue:         req.Queue,
		Exchange:      req.Exchange,
		RoutingKey:    req.RoutingKey,
		Topic:         req.Topic,
		ConsumerGroup: req.ConsumerGroup,
		Status:        StatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Config:        req.Config,
	}

	handler := m.buildConsumerHandler(c)

	switch req.Backend {
	case BackendRabbitMQ:
		if m.rabbitmq == nil {
			return nil, fmt.Errorf("rabbitmq backend not configured")
		}
		if err := m.rabbitmq.Connect(); err != nil {
			return nil, err
		}
		if c.Queue != "" {
			if err := m.rabbitmq.EnsureQueue(c.Queue, c.Exchange, c.RoutingKey); err != nil {
				return nil, err
			}
		}
		if c.Config.DLQEnabled && c.Config.DLQExchange != "" {
			m.rabbitmq.EnsureExchange(c.Config.DLQExchange, "topic")
			dlqQueue := c.Queue + ".dlq"
			m.rabbitmq.EnsureQueue(dlqQueue, c.Config.DLQExchange, c.Config.DLQRoutingKey)
		}
		if err := m.rabbitmq.StartConsumer(c, handler); err != nil {
			return nil, err
		}
	case BackendKafka:
		if m.kafka == nil {
			return nil, fmt.Errorf("kafka backend not configured")
		}
		if err := m.kafka.StartConsumer(c, handler); err != nil {
			return nil, err
		}
	case BackendMemory:
		// no-op for memory backend
	default:
		return nil, fmt.Errorf("unsupported backend: %s", req.Backend)
	}

	m.consumers[id] = c
	m.saveConsumer(c)
	return c, nil
}

// GetConsumer returns a single consumer.
func (m *Manager) GetConsumer(id string) (*Consumer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.consumers[id]
	if !ok {
		return nil, fmt.Errorf("consumer not found: %s", id)
	}
	return c, nil
}

// ListConsumers returns all consumers.
func (m *Manager) ListConsumers() []*Consumer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Consumer, 0, len(m.consumers))
	for _, c := range m.consumers {
		out = append(out, c)
	}
	return out
}

// DeleteConsumer stops and removes a consumer.
func (m *Manager) DeleteConsumer(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.consumers[id]
	if !ok {
		return fmt.Errorf("consumer not found: %s", id)
	}
	switch c.Backend {
	case BackendRabbitMQ:
		if m.rabbitmq != nil {
			m.rabbitmq.StopConsumer(id)
		}
	case BackendKafka:
		if m.kafka != nil {
			m.kafka.StopConsumer(id)
		}
	}
	delete(m.consumers, id)
	m.deleteConsumerDB(id)
	return nil
}

// PauseConsumer pauses a consumer.
func (m *Manager) PauseConsumer(id string) (*Consumer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.consumers[id]
	if !ok {
		return nil, fmt.Errorf("consumer not found: %s", id)
	}
	switch c.Backend {
	case BackendRabbitMQ:
		if m.rabbitmq != nil {
			m.rabbitmq.StopConsumer(id)
		}
	case BackendKafka:
		if m.kafka != nil {
			m.kafka.StopConsumer(id)
		}
	}
	c.Status = StatusPaused
	c.UpdatedAt = time.Now()
	m.saveConsumer(c)
	return c, nil
}

// ResumeConsumer resumes a paused consumer.
func (m *Manager) ResumeConsumer(id string) (*Consumer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.consumers[id]
	if !ok {
		return nil, fmt.Errorf("consumer not found: %s", id)
	}

	handler := m.buildConsumerHandler(c)
	switch c.Backend {
	case BackendRabbitMQ:
		if m.rabbitmq != nil {
			if err := m.rabbitmq.StartConsumer(c, handler); err != nil {
				return nil, err
			}
		}
	case BackendKafka:
		if m.kafka != nil {
			if err := m.kafka.StartConsumer(c, handler); err != nil {
				return nil, err
			}
		}
	}
	c.Status = StatusActive
	c.UpdatedAt = time.Now()
	m.saveConsumer(c)
	return c, nil
}

// ---------------------------------------------------------------
// Publish
// ---------------------------------------------------------------

// Publish sends a message through the specified producer.
func (m *Manager) Publish(_ context.Context, req *PublishRequest) (*Message, error) {
	m.mu.Lock()
	p, ok := m.producers[req.ProducerID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("producer not found: %s", req.ProducerID)
	}
	if p.Status != StatusActive {
		m.mu.Unlock()
		return nil, fmt.Errorf("producer %s is %s", req.ProducerID, p.Status)
	}
	m.mu.Unlock()

	msg := &Message{
		ID:            fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		ProducerID:    req.ProducerID,
		Body:          req.Body,
		Headers:       req.Headers,
		ContentType:   p.ContentType,
		Timestamp:     time.Now(),
		Status:        "pending",
		CorrelationID: req.CorrelationID,
	}
	if msg.Headers == nil {
		msg.Headers = make(map[string]string)
	}
	if req.RoutingKey != "" {
		msg.Headers["routingKey"] = req.RoutingKey
	}

	var err error
	switch p.Backend {
	case BackendRabbitMQ:
		err = m.rabbitmq.Publish(p, msg)
	case BackendKafka:
		err = m.kafka.Publish(p, msg)
	case BackendMemory:
		// store directly
	default:
		err = fmt.Errorf("unsupported backend: %s", p.Backend)
	}
	if err != nil {
		msg.Status = "error"
		msg.ErrorMessage = err.Error()
		return msg, err
	}

	msg.Status = "sent"
	m.mu.Lock()
	p.MessagesSent++
	p.LastSentAt = time.Now()
	p.UpdatedAt = time.Now()

	m.messages[msg.ID] = msg
	m.appendStream(*msg)

	// evict old messages if over limit
	if len(m.messages) > m.maxMessages {
		m.evictOldMessages()
	}

	// persist producer stats every 10 messages to avoid DB thrashing
	shouldPersist := p.MessagesSent%10 == 0
	m.mu.Unlock()

	if shouldPersist {
		m.saveProducer(p)
	}

	return msg, nil
}

// ---------------------------------------------------------------
// DLQ
// ---------------------------------------------------------------

// ListDLQ returns all DLQ entries.
func (m *Manager) ListDLQ() []*DLQEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*DLQEntry, 0, len(m.dlq))
	for _, e := range m.dlq {
		out = append(out, e)
	}
	return out
}

// ReplayDLQ replays a DLQ entry through its original producer.
func (m *Manager) ReplayDLQ(dlqID string) (*Message, error) {
	m.mu.Lock()
	entry, ok := m.dlq[dlqID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("dlq entry not found: %s", dlqID)
	}
	entry.Replayed = true
	entry.ReplayedAt = time.Now()
	m.mu.Unlock()

	// find the consumer's associated producer to replay
	msg := &Message{
		ID:          fmt.Sprintf("replay-%d", time.Now().UnixNano()),
		Body:        entry.Body,
		Headers:     entry.Headers,
		ContentType: "application/json",
		Timestamp:   time.Now(),
		Status:      "pending",
	}

	return msg, nil
}

// ---------------------------------------------------------------
// Stream / Live feed
// ---------------------------------------------------------------

// GetStream returns the last N messages for the live view.
func (m *Manager) GetStream(limit int) []Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > len(m.stream) {
		limit = len(m.stream)
	}
	start := len(m.stream) - limit
	if start < 0 {
		start = 0
	}
	out := make([]Message, limit)
	copy(out, m.stream[start:])
	return out
}

// GetStreamJSON returns the stream as JSON bytes (for SSE).
func (m *Manager) GetStreamJSON(limit int) ([]byte, error) {
	return json.Marshal(m.GetStream(limit))
}

// ---------------------------------------------------------------
// Stats
// ---------------------------------------------------------------

// GetStats returns aggregate conductor statistics.
func (m *Manager) GetStats() *ConductorStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := &ConductorStats{
		Producers:      len(m.producers),
		Consumers:      len(m.consumers),
		DLQSize:        len(m.dlq),
		ActiveMessages: len(m.messages),
	}
	for _, p := range m.producers {
		s.TotalSent += p.MessagesSent
	}
	for _, c := range m.consumers {
		s.TotalReceived += c.MessagesReceived
		s.TotalAcked += c.MessagesAcked
		s.TotalFailed += c.MessagesFailed
	}
	return s
}

// ---------------------------------------------------------------
// Connection Management
// ---------------------------------------------------------------

// maskURL redacts passwords from URLs for display.
func maskURL(raw string) string {
	if raw == "" {
		return ""
	}
	idx := 0
	for i := 0; i < len(raw)-2; i++ {
		if raw[i] == ':' && raw[i+1] == '/' && raw[i+2] == '/' {
			idx = i + 3
			break
		}
	}
	if idx == 0 {
		return raw
	}
	atIdx := -1
	for i := idx; i < len(raw); i++ {
		if raw[i] == '@' {
			atIdx = i
			break
		}
	}
	if atIdx < 0 {
		return raw
	}
	return raw[:idx] + "***:***@" + raw[atIdx+1:]
}

// GetConnections returns the status of all configured backends.
func (m *Manager) GetConnections() []BackendConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var conns []BackendConnection

	rmqProducers, rmqConsumers := 0, 0
	kafkaProducers, kafkaConsumers := 0, 0
	for _, p := range m.producers {
		switch p.Backend {
		case BackendRabbitMQ:
			rmqProducers++
		case BackendKafka:
			kafkaProducers++
		}
	}
	for _, c := range m.consumers {
		switch c.Backend {
		case BackendRabbitMQ:
			rmqConsumers++
		case BackendKafka:
			kafkaConsumers++
		}
	}

	rmq := BackendConnection{
		ID:        "rabbitmq",
		Type:      "rabbitmq",
		Status:    "disconnected",
		Producers: rmqProducers,
		Consumers: rmqConsumers,
	}
	if m.rabbitmq != nil {
		rmq.URL = maskURL(m.rabbitmq.url)
		m.rabbitmq.mu.RLock()
		if m.rabbitmq.conn != nil && !m.rabbitmq.conn.IsClosed() {
			rmq.Status = "connected"
		}
		m.rabbitmq.mu.RUnlock()
	}
	conns = append(conns, rmq)

	kfk := BackendConnection{
		ID:        "kafka",
		Type:      "kafka",
		Status:    "disconnected",
		Producers: kafkaProducers,
		Consumers: kafkaConsumers,
	}
	if m.kafka != nil {
		kfk.URL = fmt.Sprintf("%v", m.kafka.brokers)
		kfk.Status = "connected"
	}
	conns = append(conns, kfk)

	return conns
}

// ConnectBackend dynamically connects or reconnects a backend.
func (m *Manager) ConnectBackend(req *ConnectBackendRequest) (*BackendConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch req.Type {
	case BackendRabbitMQ:
		url := req.URL
		if url == "" {
			return nil, fmt.Errorf("url is required for rabbitmq")
		}
		if m.rabbitmq != nil {
			m.rabbitmq.Close()
		}
		m.rabbitmq = NewRabbitMQBackend(url)
		if err := m.rabbitmq.Connect(); err != nil {
			return &BackendConnection{
				ID:     "rabbitmq",
				Type:   "rabbitmq",
				Status: "error",
				URL:    maskURL(url),
				Error:  err.Error(),
			}, err
		}
		return &BackendConnection{
			ID:     "rabbitmq",
			Type:   "rabbitmq",
			Status: "connected",
			URL:    maskURL(url),
		}, nil

	case BackendKafka:
		if len(req.Brokers) == 0 {
			return nil, fmt.Errorf("brokers list is required for kafka")
		}
		if m.kafka != nil {
			m.kafka.Close()
		}
		m.kafka = NewKafkaBackend(req.Brokers)
		return &BackendConnection{
			ID:     "kafka",
			Type:   "kafka",
			Status: "connected",
			URL:    fmt.Sprintf("%v", req.Brokers),
		}, nil

	default:
		return nil, fmt.Errorf("unsupported backend type: %s", req.Type)
	}
}

// DisconnectBackend disconnects a backend.
func (m *Manager) DisconnectBackend(backendType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch backendType {
	case BackendRabbitMQ:
		if m.rabbitmq != nil {
			m.rabbitmq.Close()
			m.rabbitmq = nil
		}
		return nil
	case BackendKafka:
		if m.kafka != nil {
			m.kafka.Close()
			m.kafka = nil
		}
		return nil
	default:
		return fmt.Errorf("unsupported backend type: %s", backendType)
	}
}

// ListMessages returns recent messages.
func (m *Manager) ListMessages(limit int) []*Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Message, 0, len(m.messages))
	for _, msg := range m.messages {
		out = append(out, msg)
	}
	if limit > 0 && limit < len(out) {
		out = out[len(out)-limit:]
	}
	return out
}

// ---------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------

func (m *Manager) buildConsumerHandler(c *Consumer) func(*Message) error {
	return func(msg *Message) error {
		m.mu.Lock()

		c.MessagesReceived++
		c.LastReceivedAt = time.Now()
		c.UpdatedAt = time.Now()

		m.messages[msg.ID] = msg
		m.appendStream(*msg)

		if msg.ErrorMessage != "" {
			c.MessagesFailed++
			if c.Config.DLQEnabled && msg.RetryCount >= c.Config.MaxRetries {
				dlqEntry := &DLQEntry{
					ID:             fmt.Sprintf("dlq-%d", time.Now().UnixNano()),
					OriginalID:     msg.ID,
					ConsumerID:     c.ID,
					Body:           msg.Body,
					Headers:        msg.Headers,
					ErrorMessage:   msg.ErrorMessage,
					RetryCount:     msg.RetryCount,
					OriginalQueue:  c.Queue,
					DeadLetteredAt: time.Now(),
				}
				m.dlq[dlqEntry.ID] = dlqEntry
				msg.Status = "dlq"
			}
		} else {
			c.MessagesAcked++
		}

		// persist consumer stats every 10 messages to avoid DB thrashing
		shouldPersist := c.MessagesReceived%10 == 0
		m.mu.Unlock()

		if shouldPersist {
			m.saveConsumer(c)
		}

		return nil
	}
}

func (m *Manager) appendStream(msg Message) {
	m.stream = append(m.stream, msg)
	if len(m.stream) > m.maxStream {
		m.stream = m.stream[len(m.stream)-m.maxStream:]
	}
}

func (m *Manager) evictOldMessages() {
	// keep only the newest maxMessages/2
	keep := m.maxMessages / 2
	if len(m.messages) <= keep {
		return
	}
	// simple eviction: remove oldest half
	count := 0
	for id := range m.messages {
		if count >= len(m.messages)-keep {
			break
		}
		delete(m.messages, id)
		count++
	}
}
