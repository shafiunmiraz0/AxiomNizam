package models

// =====================================================
// P2 resource-ification -- Conductor domain Resource types.
//
// ProducerResource and ConsumerResource wrap the imperative Producer
// and Consumer structs so a controller can reconcile message-queue
// primitives as first-class platform resources.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	ProducerKind       = "ConductorProducer"
	ProducerAPIVersion = "conductor.axiomnizam.io/v1"
	ConsumerKind       = "ConductorConsumer"
	ConsumerAPIVersion = "conductor.axiomnizam.io/v1"
)

// --- ProducerResource ---

// ProducerConfig holds tunables for a producer.
type ProducerConfig struct {
	Persistent    bool `json:"persistent"`
	Mandatory     bool `json:"mandatory"`
	Immediate     bool `json:"immediate"`
	BatchSize     int  `json:"batchSize,omitempty"`
	FlushInterval int  `json:"flushIntervalMs,omitempty"` // ms
}

// ProducerSpec is the desired state of a conductor producer.
type ProducerSpec struct {
	Backend     string            `json:"backend"`
	Exchange    string            `json:"exchange,omitempty"`
	RoutingKey  string            `json:"routingKey,omitempty"`
	Topic       string            `json:"topic,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Config      ProducerConfig    `json:"config,omitempty"`
	Active      bool              `json:"active"`
}

// ProducerResourceStatus extends the canonical object status.
type ProducerResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	ProducerStatus string     `json:"producerStatus"`
	MessagesSent   int64      `json:"messagesSent"`
	LastSentAt     *time.Time `json:"lastSentAt,omitempty"`
}

// ProducerResource is the declarative resource for a ConductorProducer.
type ProducerResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   ProducerSpec           `json:"spec"`
	Status ProducerResourceStatus `json:"status"`
}

func (r *ProducerResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *ProducerResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *ProducerResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *ProducerResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *ProducerResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *ProducerResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *ProducerResource) GetGeneration() int64         { return r.Generation }
func (r *ProducerResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// --- ConsumerResource ---

// ConsumerConfig holds tunables for a consumer.
type ConsumerConfig struct {
	AutoAck        bool   `json:"autoAck"`
	PrefetchCount  int    `json:"prefetchCount"`
	MaxRetries     int    `json:"maxRetries"`
	RetryDelayMs   int    `json:"retryDelayMs"`
	DLQEnabled     bool   `json:"dlqEnabled"`
	DLQExchange    string `json:"dlqExchange,omitempty"`
	DLQRoutingKey  string `json:"dlqRoutingKey,omitempty"`
	DLQTopic       string `json:"dlqTopic,omitempty"`
	MaxConcurrency int    `json:"maxConcurrency"`
}

// ConsumerSpec is the desired state of a conductor consumer.
type ConsumerSpec struct {
	Backend       string         `json:"backend"`
	Queue         string         `json:"queue,omitempty"`
	Exchange      string         `json:"exchange,omitempty"`
	RoutingKey    string         `json:"routingKey,omitempty"`
	Topic         string         `json:"topic,omitempty"`
	ConsumerGroup string         `json:"consumerGroup,omitempty"`
	Config        ConsumerConfig `json:"config,omitempty"`
	Active        bool           `json:"active"`
}

// ConsumerResourceStatus extends the canonical object status.
type ConsumerResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	ConsumerStatus   string     `json:"consumerStatus"`
	MessagesReceived int64      `json:"messagesReceived"`
	MessagesAcked    int64      `json:"messagesAcked"`
	MessagesFailed   int64      `json:"messagesFailed"`
	LastReceivedAt   *time.Time `json:"lastReceivedAt,omitempty"`
}

// ConsumerResource is the declarative resource for a ConductorConsumer.
type ConsumerResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   ConsumerSpec           `json:"spec"`
	Status ConsumerResourceStatus `json:"status"`
}

func (r *ConsumerResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *ConsumerResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *ConsumerResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *ConsumerResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *ConsumerResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *ConsumerResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *ConsumerResource) GetGeneration() int64         { return r.Generation }
func (r *ConsumerResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
