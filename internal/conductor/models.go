package conductor

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/conductor/models"
)

// Backend types for message brokers.
const (
	BackendRabbitMQ = "rabbitmq"
	BackendKafka    = "kafka"
	BackendMemory   = "memory"
)

// ProducerStatus enumerates producer lifecycle states.
const (
	StatusActive  = "active"
	StatusPaused  = "paused"
	StatusStopped = "stopped"
	StatusError   = "error"
)

// ---------------------------------------------------------------
// Producer / Consumer definitions
// ---------------------------------------------------------------

// Producer publishes messages to a backend exchange/topic.
type Producer struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Backend      string                `json:"backend"` // "rabbitmq", "kafka", "memory"
	Exchange     string                `json:"exchange,omitempty"`
	RoutingKey   string                `json:"routingKey,omitempty"`
	Topic        string                `json:"topic,omitempty"` // Kafka topic
	ContentType  string                `json:"contentType"`
	Headers      map[string]string     `json:"headers,omitempty"`
	Status       string                `json:"status"`
	CreatedAt    time.Time             `json:"createdAt"`
	UpdatedAt    time.Time             `json:"updatedAt"`
	MessagesSent int64                 `json:"messagesSent"`
	LastSentAt   time.Time             `json:"lastSentAt,omitempty"`
	Config       models.ProducerConfig `json:"config"`
}

// Consumer reads messages from a backend queue/topic.
type Consumer struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Backend          string               `json:"backend"`
	Queue            string               `json:"queue,omitempty"`
	Exchange         string               `json:"exchange,omitempty"`
	RoutingKey       string               `json:"routingKey,omitempty"`
	Topic            string               `json:"topic,omitempty"` // Kafka topic
	ConsumerGroup    string               `json:"consumerGroup,omitempty"`
	Status           string               `json:"status"`
	CreatedAt        time.Time            `json:"createdAt"`
	UpdatedAt        time.Time            `json:"updatedAt"`
	MessagesReceived int64                `json:"messagesReceived"`
	MessagesAcked    int64                `json:"messagesAcked"`
	MessagesFailed   int64                `json:"messagesFailed"`
	LastReceivedAt   time.Time            `json:"lastReceivedAt,omitempty"`
	Config           models.ConsumerConfig `json:"config"`
}

// ---------------------------------------------------------------
// Messages flowing through the conductor
// ---------------------------------------------------------------

// Message is a unit of data passed between producers and consumers.
type Message struct {
	ID            string                 `json:"id"`
	ProducerID    string                 `json:"producerId"`
	ConsumerID    string                 `json:"consumerId,omitempty"`
	Body          map[string]interface{} `json:"body"`
	Headers       map[string]string      `json:"headers,omitempty"`
	ContentType   string                 `json:"contentType"`
	Timestamp     time.Time              `json:"timestamp"`
	Status        string                 `json:"status"` // "pending", "delivered", "acked", "nacked", "dlq"
	RetryCount    int                    `json:"retryCount"`
	DeliveredAt   time.Time              `json:"deliveredAt,omitempty"`
	AckedAt       time.Time              `json:"ackedAt,omitempty"`
	ErrorMessage  string                 `json:"errorMessage,omitempty"`
	CorrelationID string                 `json:"correlationId,omitempty"`
}

// DLQEntry represents a message that exhausted retries.
type DLQEntry struct {
	ID             string                 `json:"id"`
	OriginalID     string                 `json:"originalId"`
	ConsumerID     string                 `json:"consumerId"`
	Body           map[string]interface{} `json:"body"`
	Headers        map[string]string      `json:"headers,omitempty"`
	ErrorMessage   string                 `json:"errorMessage"`
	RetryCount     int                    `json:"retryCount"`
	OriginalQueue  string                 `json:"originalQueue"`
	DeadLetteredAt time.Time              `json:"deadLetteredAt"`
	ReplayedAt     time.Time              `json:"replayedAt,omitempty"`
	Replayed       bool                   `json:"replayed"`
}

// ---------------------------------------------------------------
// API request / response helpers
// ---------------------------------------------------------------

// PublishRequest is the REST body for publishing a message.
type PublishRequest struct {
	ProducerID    string                 `json:"producerId" binding:"required"`
	Body          map[string]interface{} `json:"body" binding:"required"`
	Headers       map[string]string      `json:"headers,omitempty"`
	CorrelationID string                 `json:"correlationId,omitempty"`
	RoutingKey    string                 `json:"routingKey,omitempty"`
}

// CreateProducerRequest is the REST body for creating a producer.
type CreateProducerRequest struct {
	Name        string                `json:"name" binding:"required"`
	Backend     string                `json:"backend" binding:"required"`
	Exchange    string                `json:"exchange,omitempty"`
	RoutingKey  string                `json:"routingKey,omitempty"`
	Topic       string                `json:"topic,omitempty"`
	ContentType string                `json:"contentType,omitempty"`
	Headers     map[string]string     `json:"headers,omitempty"`
	Config      models.ProducerConfig `json:"config"`
}

// CreateConsumerRequest is the REST body for creating a consumer.
type CreateConsumerRequest struct {
	Name          string               `json:"name" binding:"required"`
	Backend       string               `json:"backend" binding:"required"`
	Queue         string               `json:"queue,omitempty"`
	Exchange      string               `json:"exchange,omitempty"`
	RoutingKey    string               `json:"routingKey,omitempty"`
	Topic         string               `json:"topic,omitempty"`
	ConsumerGroup string               `json:"consumerGroup,omitempty"`
	Config        models.ConsumerConfig `json:"config"`
}

// ConductorStats aggregate metrics.
type ConductorStats struct {
	Producers      int   `json:"producers"`
	Consumers      int   `json:"consumers"`
	TotalSent      int64 `json:"totalSent"`
	TotalReceived  int64 `json:"totalReceived"`
	TotalAcked     int64 `json:"totalAcked"`
	TotalFailed    int64 `json:"totalFailed"`
	DLQSize        int   `json:"dlqSize"`
	ActiveMessages int   `json:"activeMessages"`
}

// BackendConnection represents a live backend connection status.
type BackendConnection struct {
	ID        string `json:"id"`
	Type      string `json:"type"`   // "rabbitmq", "kafka"
	Status    string `json:"status"` // "connected", "disconnected", "error"
	URL       string `json:"url"`    // display-safe URL (no passwords)
	Error     string `json:"error,omitempty"`
	Producers int    `json:"producers"`
	Consumers int    `json:"consumers"`
}

// ConnectBackendRequest is the REST body for connecting a new backend.
type ConnectBackendRequest struct {
	Type    string   `json:"type" binding:"required"` // "rabbitmq", "kafka"
	URL     string   `json:"url,omitempty"`           // RabbitMQ AMQP URL
	Brokers []string `json:"brokers,omitempty"`       // Kafka broker list
}
