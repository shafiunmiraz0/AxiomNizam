package models

// =====================================================
// Domain resource types for the EventBus module.
//
// Moved from the parent package to provide a clean
// models/ sub-package that other modules can import
// without pulling in the full eventbus implementation.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	TopicKind              = "EventBusTopic"
	TopicAPIVersion        = "eventbus.axiomnizam.io/v1"
	SubscriptionKind       = "EventBusSubscription"
	SubscriptionAPIVersion = "eventbus.axiomnizam.io/v1"
)

// --- Dependent types (needed by the resource structs) ---

// EventSchema validates event structure
type EventSchema struct {
	Version  string                   `json:"version"`
	Fields   map[string]FieldSchema   `json:"fields"`
	Required []string                 `json:"required"`
	Examples []map[string]interface{} `json:"examples"`
}

// FieldSchema describes event field
type FieldSchema struct {
	Type        string        `json:"type"` // string, int, bool, object, array
	Description string        `json:"description"`
	Required    bool          `json:"required"`
	Pattern     string        `json:"pattern,omitempty"` // Regex validation
	Enum        []interface{} `json:"enum,omitempty"`    // Allowed values
	Min         interface{}   `json:"min,omitempty"`
	Max         interface{}   `json:"max,omitempty"`
}

// RetentionConfig defines data retention
type RetentionConfig struct {
	Type            string `json:"type"` // "time", "size", "both"
	TimeMs          int64  `json:"timeMs"`
	SizeBytes       int64  `json:"sizeBytes"`
	DeletePolicy    string `json:"deletePolicy"` // "delete", "compact"
	CompactDeleteMs int64  `json:"compactDeleteMs"`
}

// TopicConfig configures topic behavior
type TopicConfig struct {
	CompressionType   string `json:"compressionType"` // "none", "gzip", "snappy", "lz4"
	MinInSyncReplicas int    `json:"minInSyncReplicas"`
	MaxMessageBytes   int    `json:"maxMessageBytes"`
}

// EventFilter filters which events trigger handler
type EventFilter struct {
	Types          []string          `json:"types"`          // Event types
	Sources        []string          `json:"sources"`        // From these services
	Subjects       []string          `json:"subjects"`       // About these subjects
	AggregateTypes []string          `json:"aggregateTypes"` // Resource types
	Conditions     []FilterCondition `json:"conditions"`     // Custom logic
	MinPriority    int               `json:"minPriority"`    // Only important events
}

// FilterCondition represents filter condition
type FilterCondition struct {
	Path     string      `json:"path"`     // JSON path in data
	Operator string      `json:"operator"` // eq, ne, gt, lt, contains, exists, matches
	Value    interface{} `json:"value"`
}

// SubscriptionConfig configures subscription behavior
type SubscriptionConfig struct {
	ProcessingMode  string      `json:"processingMode"` // "auto", "manual"
	DeliveryMode    string      `json:"deliveryMode"`   // "at_most_once", "at_least_once", "exactly_once"
	Timeout         int         `json:"timeout"`        // Seconds
	MaxConcurrency  int         `json:"maxConcurrency"` // Parallel handlers
	RetryPolicy     RetryPolicy `json:"retryPolicy"`
	DeadLetterTopic string      `json:"deadLetterTopic"` // Failed events go here
	DLQ             bool        `json:"dlq"`             // Enable DLQ
	AckTimeout      int         `json:"ackTimeout"`      // Seconds before redelivery
	OrderGuarantee  string      `json:"orderGuarantee"`  // "none", "key", "partition"
	StartFromOffset int64       `json:"startFromOffset"`
	StartFromTime   time.Time   `json:"startFromTime"`
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxRetries    int     `json:"maxRetries"`
	InitialDelay  int     `json:"initialDelay"` // Milliseconds
	MaxDelay      int     `json:"maxDelay"`
	BackoffFactor float64 `json:"backoffFactor"`
}

// --- Resource types ---

// TopicSpec is the desired state of an event-bus topic.
type TopicSpec struct {
	Description       string          `json:"description,omitempty"`
	Schema            EventSchema     `json:"schema,omitempty"`
	Partitions        int             `json:"partitions,omitempty"`
	ReplicationFactor int             `json:"replicationFactor,omitempty"`
	Retention         RetentionConfig `json:"retention,omitempty"`
	Config            TopicConfig     `json:"config,omitempty"`
	Active            bool            `json:"active"`
}

// TopicResourceStatus extends the canonical object status.
type TopicResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	MessageCount int64 `json:"messageCount"`
	TopicActive  bool  `json:"topicActive"`
}

// TopicResource is the declarative resource for an EventBusTopic.
type TopicResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   TopicSpec           `json:"spec"`
	Status TopicResourceStatus `json:"status"`
}

func (r *TopicResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *TopicResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *TopicResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *TopicResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *TopicResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *TopicResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *TopicResource) GetGeneration() int64         { return r.Generation }
func (r *TopicResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// SubscriptionSpec is the desired state of an event-bus subscription.
type SubscriptionSpec struct {
	TenantID      string             `json:"tenantId"`
	Topics        []string           `json:"topics"`
	ConsumerGroup string             `json:"consumerGroup,omitempty"`
	Handler       string             `json:"handler,omitempty"`
	Filter        EventFilter        `json:"filter,omitempty"`
	Config        SubscriptionConfig `json:"config,omitempty"`
	Paused        bool               `json:"paused,omitempty"`
}

// SubscriptionResourceStatus extends the canonical object status.
type SubscriptionResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	SubscriptionStatus string    `json:"subscriptionStatus"`
	Offset             int64     `json:"offset"`
	Lag                int64     `json:"lag"`
	ProcessedCount     int64     `json:"processedCount"`
	FailedCount        int64     `json:"failedCount"`
	LastProcessed      time.Time `json:"lastProcessed,omitempty"`
}

// SubscriptionResource is the declarative resource for an EventBusSubscription.
type SubscriptionResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   SubscriptionSpec           `json:"spec"`
	Status SubscriptionResourceStatus `json:"status"`
}

func (r *SubscriptionResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *SubscriptionResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *SubscriptionResource) GetStatus() *resources.ObjectStatus {
	return &r.Status.ObjectStatus
}
func (r *SubscriptionResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *SubscriptionResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Topics) > 0 {
		cp.Spec.Topics = append([]string(nil), r.Spec.Topics...)
	}
	return &cp
}
func (r *SubscriptionResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *SubscriptionResource) GetGeneration() int64         { return r.Generation }
func (r *SubscriptionResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
