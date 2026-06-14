package conductor

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------
// GORM models for persistent storage
// ---------------------------------------------------------------

// DBProducer is the GORM model for persisting producers.
type DBProducer struct {
	ID           string `gorm:"primaryKey;column:id"`
	Name         string `gorm:"column:name"`
	Backend      string `gorm:"column:backend"`
	Exchange     string `gorm:"column:exchange"`
	RoutingKey   string `gorm:"column:routing_key"`
	Topic        string `gorm:"column:topic"`
	ContentType  string `gorm:"column:content_type"`
	HeadersJSON  string `gorm:"column:headers_json;type:text"`
	Status       string `gorm:"column:status"`
	ConfigJSON   string `gorm:"column:config_json;type:text"`
	MessagesSent int64  `gorm:"column:messages_sent;default:0"`
	LastSentAt   time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (DBProducer) TableName() string { return "conductor_producers" }

// DBConsumer is the GORM model for persisting consumers.
type DBConsumer struct {
	ID               string `gorm:"primaryKey;column:id"`
	Name             string `gorm:"column:name"`
	Backend          string `gorm:"column:backend"`
	Queue            string `gorm:"column:queue"`
	Exchange         string `gorm:"column:exchange"`
	RoutingKey       string `gorm:"column:routing_key"`
	Topic            string `gorm:"column:topic"`
	ConsumerGroup    string `gorm:"column:consumer_group"`
	Status           string `gorm:"column:status"`
	ConfigJSON       string `gorm:"column:config_json;type:text"`
	MessagesReceived int64  `gorm:"column:messages_received;default:0"`
	MessagesAcked    int64  `gorm:"column:messages_acked;default:0"`
	MessagesFailed   int64  `gorm:"column:messages_failed;default:0"`
	LastReceivedAt   time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (DBConsumer) TableName() string { return "conductor_consumers" }

// ---------------------------------------------------------------
// Conversion helpers
// ---------------------------------------------------------------

func producerToDB(p *Producer) *DBProducer {
	hdr, _ := json.Marshal(p.Headers)
	cfg, _ := json.Marshal(p.Config)
	return &DBProducer{
		ID:           p.ID,
		Name:         p.Name,
		Backend:      p.Backend,
		Exchange:     p.Exchange,
		RoutingKey:   p.RoutingKey,
		Topic:        p.Topic,
		ContentType:  p.ContentType,
		HeadersJSON:  string(hdr),
		Status:       p.Status,
		ConfigJSON:   string(cfg),
		MessagesSent: p.MessagesSent,
		LastSentAt:   p.LastSentAt,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	}
}

func producerFromDB(d *DBProducer) *Producer {
	p := &Producer{
		ID:           d.ID,
		Name:         d.Name,
		Backend:      d.Backend,
		Exchange:     d.Exchange,
		RoutingKey:   d.RoutingKey,
		Topic:        d.Topic,
		ContentType:  d.ContentType,
		Status:       d.Status,
		MessagesSent: d.MessagesSent,
		LastSentAt:   d.LastSentAt,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
	}
	if d.HeadersJSON != "" {
		json.Unmarshal([]byte(d.HeadersJSON), &p.Headers)
	}
	if d.ConfigJSON != "" {
		json.Unmarshal([]byte(d.ConfigJSON), &p.Config)
	}
	return p
}

func consumerToDB(c *Consumer) *DBConsumer {
	cfg, _ := json.Marshal(c.Config)
	return &DBConsumer{
		ID:               c.ID,
		Name:             c.Name,
		Backend:          c.Backend,
		Queue:            c.Queue,
		Exchange:         c.Exchange,
		RoutingKey:       c.RoutingKey,
		Topic:            c.Topic,
		ConsumerGroup:    c.ConsumerGroup,
		Status:           c.Status,
		ConfigJSON:       string(cfg),
		MessagesReceived: c.MessagesReceived,
		MessagesAcked:    c.MessagesAcked,
		MessagesFailed:   c.MessagesFailed,
		LastReceivedAt:   c.LastReceivedAt,
		CreatedAt:        c.CreatedAt,
		UpdatedAt:        c.UpdatedAt,
	}
}

func consumerFromDB(d *DBConsumer) *Consumer {
	c := &Consumer{
		ID:               d.ID,
		Name:             d.Name,
		Backend:          d.Backend,
		Queue:            d.Queue,
		Exchange:         d.Exchange,
		RoutingKey:       d.RoutingKey,
		Topic:            d.Topic,
		ConsumerGroup:    d.ConsumerGroup,
		Status:           d.Status,
		MessagesReceived: d.MessagesReceived,
		MessagesAcked:    d.MessagesAcked,
		MessagesFailed:   d.MessagesFailed,
		LastReceivedAt:   d.LastReceivedAt,
		CreatedAt:        d.CreatedAt,
		UpdatedAt:        d.UpdatedAt,
	}
	if d.ConfigJSON != "" {
		json.Unmarshal([]byte(d.ConfigJSON), &c.Config)
	}
	return c
}

// ---------------------------------------------------------------
// Persistence operations on Manager
// ---------------------------------------------------------------

// InitPersistence auto-migrates the conductor tables and loads saved
// producers/consumers, reconnecting active ones to their backends.
func (m *Manager) InitPersistence(db *gorm.DB) {
	m.db = db
	if db == nil {
		return
	}

	if err := db.AutoMigrate(&DBProducer{}, &DBConsumer{}); err != nil {
		logging.Z().Info(fmt.Sprintf("[conductor] auto-migrate warning: %v", err))
		return
	}

	// Load saved producers
	var dbProducers []DBProducer
	if err := db.Find(&dbProducers).Error; err != nil {
		logging.Z().Info(fmt.Sprintf("[conductor] failed to load producers: %v", err))
	} else {
		for i := range dbProducers {
			p := producerFromDB(&dbProducers[i])
			m.producers[p.ID] = p
		}
		if len(dbProducers) > 0 {
			logging.Z().Info(fmt.Sprintf("[conductor] restored %d producer(s) from database", len(dbProducers)))
		}
	}

	// Load saved consumers and reconnect active ones
	var dbConsumers []DBConsumer
	if err := db.Find(&dbConsumers).Error; err != nil {
		logging.Z().Info(fmt.Sprintf("[conductor] failed to load consumers: %v", err))
	} else {
		for i := range dbConsumers {
			c := consumerFromDB(&dbConsumers[i])
			m.consumers[c.ID] = c

			if c.Status == StatusActive {
				m.reconnectConsumer(c)
			}
		}
		if len(dbConsumers) > 0 {
			logging.Z().Info(fmt.Sprintf("[conductor] restored %d consumer(s) from database", len(dbConsumers)))
		}
	}
}

// reconnectConsumer re-attaches a loaded consumer to its broker backend.
func (m *Manager) reconnectConsumer(c *Consumer) {
	handler := m.buildConsumerHandler(c)

	switch c.Backend {
	case BackendRabbitMQ:
		if m.rabbitmq == nil {
			logging.Z().Info(fmt.Sprintf("[conductor] skipping consumer %s: rabbitmq not configured", c.ID))
			c.Status = StatusStopped
			return
		}
		if err := m.rabbitmq.Connect(); err != nil {
			logging.Z().Info(fmt.Sprintf("[conductor] rabbitmq reconnect failed for consumer %s: %v", c.ID, err))
			c.Status = StatusError
			return
		}
		if c.Queue != "" {
			if err := m.rabbitmq.EnsureQueue(c.Queue, c.Exchange, c.RoutingKey); err != nil {
				logging.Z().Info(fmt.Sprintf("[conductor] queue setup failed for consumer %s: %v", c.ID, err))
				c.Status = StatusError
				return
			}
		}
		if err := m.rabbitmq.StartConsumer(c, handler); err != nil {
			logging.Z().Info(fmt.Sprintf("[conductor] consumer %s reconnect failed: %v", c.ID, err))
			c.Status = StatusError
			return
		}
		logging.Z().Info(fmt.Sprintf("[conductor] consumer %s reconnected to rabbitmq", c.ID))

	case BackendKafka:
		if m.kafka == nil {
			logging.Z().Info(fmt.Sprintf("[conductor] skipping consumer %s: kafka not configured", c.ID))
			c.Status = StatusStopped
			return
		}
		if err := m.kafka.StartConsumer(c, handler); err != nil {
			logging.Z().Info(fmt.Sprintf("[conductor] consumer %s reconnect failed: %v", c.ID, err))
			c.Status = StatusError
			return
		}
		logging.Z().Info(fmt.Sprintf("[conductor] consumer %s reconnected to kafka", c.ID))

	case BackendMemory:
		// no-op
	}
}

// saveProducer persists a producer to the database.
func (m *Manager) saveProducer(p *Producer) {
	if m.db == nil {
		return
	}
	rec := producerToDB(p)
	if err := m.db.Save(rec).Error; err != nil {
		logging.Z().Info(fmt.Sprintf("[conductor] failed to save producer %s: %v", p.ID, err))
	}
}

// deleteProducerDB removes a producer from the database.
func (m *Manager) deleteProducerDB(id string) {
	if m.db == nil {
		return
	}
	if err := m.db.Delete(&DBProducer{}, "id = ?", id).Error; err != nil {
		logging.Z().Info(fmt.Sprintf("[conductor] failed to delete producer %s from db: %v", id, err))
	}
}

// saveConsumer persists a consumer to the database.
func (m *Manager) saveConsumer(c *Consumer) {
	if m.db == nil {
		return
	}
	rec := consumerToDB(c)
	if err := m.db.Save(rec).Error; err != nil {
		logging.Z().Info(fmt.Sprintf("[conductor] failed to save consumer %s: %v", c.ID, err))
	}
}

// deleteConsumerDB removes a consumer from the database.
func (m *Manager) deleteConsumerDB(id string) {
	if m.db == nil {
		return
	}
	if err := m.db.Delete(&DBConsumer{}, "id = ?", id).Error; err != nil {
		logging.Z().Info(fmt.Sprintf("[conductor] failed to delete consumer %s from db: %v", id, err))
	}
}
