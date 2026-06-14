package eventbus

import "axiomnizam.bitbd.net/axiomnizam/internal/eventbus/models"

const (
	TopicKind       = models.TopicKind
	TopicAPIVersion = models.TopicAPIVersion

	SubscriptionKind       = models.SubscriptionKind
	SubscriptionAPIVersion = models.SubscriptionAPIVersion
)

type EventSchema = models.EventSchema
type FieldSchema = models.FieldSchema
type RetentionConfig = models.RetentionConfig
type TopicConfig = models.TopicConfig
type EventFilter = models.EventFilter
type FilterCondition = models.FilterCondition
type SubscriptionConfig = models.SubscriptionConfig
type RetryPolicy = models.RetryPolicy
type TopicSpec = models.TopicSpec
type TopicResourceStatus = models.TopicResourceStatus
type TopicResource = models.TopicResource
type SubscriptionSpec = models.SubscriptionSpec
type SubscriptionResourceStatus = models.SubscriptionResourceStatus
type SubscriptionResource = models.SubscriptionResource
