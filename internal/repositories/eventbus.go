package repositories

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"

	"gorm.io/gorm"
)

// EventBusRepository interface for event bus operations
type EventBusRepository interface {
	CreateEvent(event *models.EventModel) error
	GetEvent(id string) (*models.EventModel, error)
	ListEvents(tenantID, topic string, limit, offset int) ([]*models.EventModel, error)
	CreateTopic(topic *models.TopicModel) error
	GetTopic(name string) (*models.TopicModel, error)
	ListTopics(tenantID string) ([]*models.TopicModel, error)
	CreateSubscription(sub *models.SubscriptionModel) error
	GetSubscription(id string) (*models.SubscriptionModel, error)
	ListSubscriptions(tenantID, topic string) ([]*models.SubscriptionModel, error)
	DeleteSubscription(id string) error
	CreateDeadLetterEvent(event *models.DeadLetterEventModel) error
	ListDLQ(tenantID, topic string) ([]*models.DeadLetterEventModel, error)
	DeleteDLQEvent(id string) error
}

// EventBusRepositoryImpl implements EventBusRepository
type EventBusRepositoryImpl struct {
	db *gorm.DB
}

// NewEventBusRepository creates event bus repository
func NewEventBusRepository(db *gorm.DB) EventBusRepository {
	return &EventBusRepositoryImpl{db: db}
}

// CreateEvent creates event
func (r *EventBusRepositoryImpl) CreateEvent(event *models.EventModel) error {
	return r.db.Create(event).Error
}

// GetEvent retrieves event by ID
func (r *EventBusRepositoryImpl) GetEvent(id string) (*models.EventModel, error) {
	var event models.EventModel
	err := r.db.Where("id = ?", id).First(&event).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("event not found")
	}
	return &event, err
}

// ListEvents lists events
func (r *EventBusRepositoryImpl) ListEvents(tenantID, topic string, limit, offset int) ([]*models.EventModel, error) {
	var events []*models.EventModel
	query := r.db.Where("tenant_id = ?", tenantID)
	if topic != "" {
		query = query.Where("topic = ?", topic)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Order("created_at DESC").Find(&events).Error
	return events, err
}

// CreateTopic creates topic
func (r *EventBusRepositoryImpl) CreateTopic(topic *models.TopicModel) error {
	return r.db.Create(topic).Error
}

// GetTopic retrieves topic
func (r *EventBusRepositoryImpl) GetTopic(name string) (*models.TopicModel, error) {
	var topic models.TopicModel
	err := r.db.Where("name = ?", name).First(&topic).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("topic not found")
	}
	return &topic, err
}

// ListTopics lists topics
func (r *EventBusRepositoryImpl) ListTopics(tenantID string) ([]*models.TopicModel, error) {
	var topics []*models.TopicModel
	err := r.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&topics).Error
	return topics, err
}

// CreateSubscription creates subscription
func (r *EventBusRepositoryImpl) CreateSubscription(sub *models.SubscriptionModel) error {
	return r.db.Create(sub).Error
}

// GetSubscription retrieves subscription
func (r *EventBusRepositoryImpl) GetSubscription(id string) (*models.SubscriptionModel, error) {
	var sub models.SubscriptionModel
	err := r.db.Where("id = ?", id).First(&sub).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("subscription not found")
	}
	return &sub, err
}

// ListSubscriptions lists subscriptions
func (r *EventBusRepositoryImpl) ListSubscriptions(tenantID, topic string) ([]*models.SubscriptionModel, error) {
	var subs []*models.SubscriptionModel
	query := r.db.Where("tenant_id = ?", tenantID)
	if topic != "" {
		query = query.Where("topic = ?", topic)
	}
	err := query.Order("created_at DESC").Find(&subs).Error
	return subs, err
}

// DeleteSubscription deletes subscription
func (r *EventBusRepositoryImpl) DeleteSubscription(id string) error {
	return r.db.Delete(&models.SubscriptionModel{}, "id = ?", id).Error
}

// CreateDeadLetterEvent creates DLQ event
func (r *EventBusRepositoryImpl) CreateDeadLetterEvent(event *models.DeadLetterEventModel) error {
	return r.db.Create(event).Error
}

// ListDLQ lists dead letter events
func (r *EventBusRepositoryImpl) ListDLQ(tenantID, topic string) ([]*models.DeadLetterEventModel, error) {
	var events []*models.DeadLetterEventModel
	query := r.db.Where("tenant_id = ?", tenantID)
	if topic != "" {
		query = query.Where("topic = ?", topic)
	}
	err := query.Order("created_at DESC").Find(&events).Error
	return events, err
}

// DeleteDLQEvent deletes DLQ event
func (r *EventBusRepositoryImpl) DeleteDLQEvent(id string) error {
	return r.db.Delete(&models.DeadLetterEventModel{}, "id = ?", id).Error
}
