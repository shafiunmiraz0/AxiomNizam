package repositories

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"

	"gorm.io/gorm"
)

// WebhookRepository interface for webhook operations
type WebhookRepository interface {
	Create(webhook *models.WebhookModel) error
	GetByID(id string) (*models.WebhookModel, error)
	List(tenantID string, limit, offset int) ([]*models.WebhookModel, error)
	Update(webhook *models.WebhookModel) error
	Delete(id string) error
	AddDeliveryLog(log *models.WebhookDeliveryLogModel) error
	GetDeliveryLogs(webhookID string) ([]*models.WebhookDeliveryLogModel, error)
}

// WebhookRepositoryImpl implements WebhookRepository
type WebhookRepositoryImpl struct {
	db *gorm.DB
}

// NewWebhookRepository creates webhook repository
func NewWebhookRepository(db *gorm.DB) WebhookRepository {
	return &WebhookRepositoryImpl{db: db}
}

// Create creates webhook
func (r *WebhookRepositoryImpl) Create(webhook *models.WebhookModel) error {
	return r.db.Create(webhook).Error
}

// GetByID retrieves webhook by ID
func (r *WebhookRepositoryImpl) GetByID(id string) (*models.WebhookModel, error) {
	var webhook models.WebhookModel
	err := r.db.Preload("DeliveryLogs").Where("id = ?", id).First(&webhook).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("webhook not found")
	}
	return &webhook, err
}

// List lists webhooks
func (r *WebhookRepositoryImpl) List(tenantID string, limit, offset int) ([]*models.WebhookModel, error) {
	var webhooks []*models.WebhookModel
	query := r.db.Preload("DeliveryLogs").Where("tenant_id = ?", tenantID)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Order("created_at DESC").Find(&webhooks).Error
	return webhooks, err
}

// Update updates webhook
func (r *WebhookRepositoryImpl) Update(webhook *models.WebhookModel) error {
	return r.db.Save(webhook).Error
}

// Delete deletes webhook
func (r *WebhookRepositoryImpl) Delete(id string) error {
	return r.db.Delete(&models.WebhookModel{}, "id = ?", id).Error
}

// AddDeliveryLog adds delivery log
func (r *WebhookRepositoryImpl) AddDeliveryLog(log *models.WebhookDeliveryLogModel) error {
	return r.db.Create(log).Error
}

// GetDeliveryLogs gets webhook delivery logs
func (r *WebhookRepositoryImpl) GetDeliveryLogs(webhookID string) ([]*models.WebhookDeliveryLogModel, error) {
	var logs []*models.WebhookDeliveryLogModel
	err := r.db.Where("webhook_id = ?", webhookID).Order("created_at DESC").Find(&logs).Error
	return logs, err
}
