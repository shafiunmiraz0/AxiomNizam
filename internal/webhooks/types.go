package webhooks

import "axiomnizam.bitbd.net/axiomnizam/internal/webhooks/models"

// Re-exported resource types from models/.
type WebhookResource = models.WebhookResource
type WebhookSpec = models.WebhookSpec
type WebhookResourceStatus = models.WebhookResourceStatus

// Re-exported domain types used by WebhookSpec.
type WebhookEventType = models.WebhookEventType
type WebhookFilter = models.WebhookFilter
type FilterCondition = models.FilterCondition
type RetryPolicy = models.RetryPolicy
type RateLimitConfig = models.RateLimitConfig
type WebhookAuth = models.WebhookAuth
type OAuth2Config = models.OAuth2Config
type SSLConfig = models.SSLConfig

// Re-exported constants.
const WebhookKind = models.WebhookKind
const WebhookAPIVersion = models.WebhookAPIVersion

// Re-exported event constants.
const (
	EventResourceCreated       = models.EventResourceCreated
	EventResourceUpdated       = models.EventResourceUpdated
	EventResourceDeleted       = models.EventResourceDeleted
	EventResourceStatusChanged = models.EventResourceStatusChanged
	EventJobCompleted          = models.EventJobCompleted
	EventJobFailed             = models.EventJobFailed
	EventQueryExecuted         = models.EventQueryExecuted
	EventPolicyViolation       = models.EventPolicyViolation
	EventDataAnomalyDetected   = models.EventDataAnomalyDetected
	EventQuotaExceeded         = models.EventQuotaExceeded
	EventTenantCreated         = models.EventTenantCreated
	EventTenantDeleted         = models.EventTenantDeleted
	EventUserAdded             = models.EventUserAdded
	EventUserRemoved           = models.EventUserRemoved
	EventAuditEvent            = models.EventAuditEvent
	EventSystemAlert           = models.EventSystemAlert
)
