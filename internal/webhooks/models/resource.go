package models

// =====================================================
// P2 resource-ification — Webhook.
//
// WebhookResource is a declarative wrapper around the existing
// imperative `Webhook` struct so a controller can reconcile webhook
// subscriptions as first-class platform resources.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	WebhookKind       = "Webhook"
	WebhookAPIVersion = "webhooks.axiomnizam.io/v1"
)

// --- Domain types referenced by WebhookSpec ---

// WebhookEventType represents types of events
type WebhookEventType string

const (
	EventResourceCreated       WebhookEventType = "resource.created"
	EventResourceUpdated       WebhookEventType = "resource.updated"
	EventResourceDeleted       WebhookEventType = "resource.deleted"
	EventResourceStatusChanged WebhookEventType = "resource.status.changed"
	EventJobCompleted          WebhookEventType = "job.completed"
	EventJobFailed             WebhookEventType = "job.failed"
	EventQueryExecuted         WebhookEventType = "query.executed"
	EventPolicyViolation       WebhookEventType = "policy.violated"
	EventDataAnomalyDetected   WebhookEventType = "data.anomaly.detected"
	EventQuotaExceeded         WebhookEventType = "quota.exceeded"
	EventTenantCreated         WebhookEventType = "tenant.created"
	EventTenantDeleted         WebhookEventType = "tenant.deleted"
	EventUserAdded             WebhookEventType = "user.added"
	EventUserRemoved           WebhookEventType = "user.removed"
	EventAuditEvent            WebhookEventType = "audit.event"
	EventSystemAlert           WebhookEventType = "system.alert"
)

// WebhookFilter filters which events trigger webhook
type WebhookFilter struct {
	ResourceTypes []string          `json:"resourceTypes"` // Empty = all
	ResourceIDs   []string          `json:"resourceIds"`   // Empty = all
	Actions       []string          `json:"actions"`       // CREATE, UPDATE, DELETE
	Users         []string          `json:"users"`         // Empty = all users
	Tags          []string          `json:"tags"`          // Resource tags
	MatchAllTags  bool              `json:"matchAllTags"`
	TenantIDs     []string          `json:"tenantIds"`  // For cross-tenant webhooks
	Conditions    []FilterCondition `json:"conditions"` // Custom conditions
}

// FilterCondition represents custom filter logic
type FilterCondition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // eq, ne, gt, lt, contains, regex
	Value    interface{} `json:"value"`
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxRetries           int     `json:"maxRetries"`
	InitialDelay         int     `json:"initialDelay"` // Milliseconds
	MaxDelay             int     `json:"maxDelay"`
	BackoffFactor        float64 `json:"backoffFactor"`
	ExponentialBackoff   bool    `json:"exponentialBackoff"`
	TimeoutExceededRetry bool    `json:"timeoutExceededRetry"`
	StatusCodes          []int   `json:"statusCodes"` // Retry on these codes
}

// RateLimitConfig limits webhook delivery rate
type RateLimitConfig struct {
	Enabled           bool
	RequestsPerSecond int
	BurstSize         int
	TimeWindow        string // "minute", "hour"
	MaxBurstRequests  int
}

// WebhookAuth configures authentication
type WebhookAuth struct {
	Type     string       `json:"type"` // "none", "basic", "bearer", "oauth2", "api_key"
	Username string       `json:"username,omitempty"`
	Password string       `json:"password,omitempty"`
	Token    string       `json:"token,omitempty"`
	ApiKey   string       `json:"apiKey,omitempty"`
	OAuth2   OAuth2Config `json:"oauth2,omitempty"`
}

// OAuth2Config for OAuth2 authentication
type OAuth2Config struct {
	ClientID     string
	ClientSecret string
	TokenURL     string
	Scopes       []string
}

// SSLConfig for TLS/HTTPS
type SSLConfig struct {
	Enabled             bool
	VerifySSL           bool
	CertFile            string
	KeyFile             string
	CABundle            string
	SkipSSLVerification bool
	MinTLSVersion       string // "1.0", "1.1", "1.2", "1.3"
}

// --- Resource types ---

// WebhookSpec is the desired state of a Webhook subscription.  Fields
// mirror `Webhook` minus operational counters and timestamps (those are
// controller-owned and live on the status).
type WebhookSpec struct {
	TenantID       string             `json:"tenantId"`
	Description    string             `json:"description,omitempty"`
	URL            string             `json:"url"`
	Secret         string             `json:"secret,omitempty"`
	Events         []WebhookEventType `json:"events"`
	Filters        WebhookFilter      `json:"filters,omitempty"`
	Version        string             `json:"version,omitempty"`
	Active         bool               `json:"active"`
	RetryPolicy    RetryPolicy        `json:"retryPolicy,omitempty"`
	RateLimit      RateLimitConfig    `json:"rateLimit,omitempty"`
	Timeout        int                `json:"timeout,omitempty"`
	Headers        map[string]string  `json:"headers,omitempty"`
	Authentication WebhookAuth        `json:"authentication,omitempty"`
	SSL            SSLConfig          `json:"ssl,omitempty"`
	Tags           []string           `json:"tags,omitempty"`
}

// WebhookResourceStatus extends the canonical object status with
// delivery telemetry.  Controller-owned.
type WebhookResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	LastTriggered *time.Time `json:"lastTriggered,omitempty"`
	SuccessCount  int64      `json:"successCount"`
	FailureCount  int64      `json:"failureCount"`
}

// WebhookResource is the declarative resource for a Webhook.
type WebhookResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   WebhookSpec           `json:"spec"`
	Status WebhookResourceStatus `json:"status"`
}

// --- resources.Resource implementation ---

func (r *WebhookResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *WebhookResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *WebhookResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *WebhookResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *WebhookResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Events) > 0 {
		cp.Spec.Events = append([]WebhookEventType(nil), r.Spec.Events...)
	}
	if len(r.Spec.Tags) > 0 {
		cp.Spec.Tags = append([]string(nil), r.Spec.Tags...)
	}
	return &cp
}

// --- reconciler.Resource implementation ---

func (r *WebhookResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *WebhookResource) GetGeneration() int64         { return r.Generation }
func (r *WebhookResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
