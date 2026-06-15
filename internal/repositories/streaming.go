package repositories

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"gorm.io/gorm"
)

// StreamRepository interface for stream operations
type StreamRepository interface {
	Create(stream *models.StreamModel) error
	GetByID(id string) (*models.StreamModel, error)
	List(tenantID string, limit, offset int) ([]*models.StreamModel, error)
	Update(stream *models.StreamModel) error
	Delete(id string) error
	Subscribe(sub *models.StreamSubscriptionModel) error
	Unsubscribe(streamID, userID string) error
	GetSubscriptions(streamID string) ([]*models.StreamSubscriptionModel, error)
}

// StreamRepositoryImpl implements StreamRepository
type StreamRepositoryImpl struct {
	db *gorm.DB
}

// NewStreamRepository creates stream repository
func NewStreamRepository(db *gorm.DB) StreamRepository {
	return &StreamRepositoryImpl{db: db}
}

// Create creates stream
func (r *StreamRepositoryImpl) Create(stream *models.StreamModel) error {
	return r.db.Create(stream).Error
}

// GetByID retrieves stream by ID
func (r *StreamRepositoryImpl) GetByID(id string) (*models.StreamModel, error) {
	var stream models.StreamModel
	err := r.db.Preload("Subscriptions").Where("id = ?", id).First(&stream).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("stream not found")
	}
	return &stream, err
}

// List lists streams
func (r *StreamRepositoryImpl) List(tenantID string, limit, offset int) ([]*models.StreamModel, error) {
	var streams []*models.StreamModel
	query := r.db.Preload("Subscriptions").Where("tenant_id = ?", tenantID)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Order("created_at DESC").Find(&streams).Error
	return streams, err
}

// Update updates stream
func (r *StreamRepositoryImpl) Update(stream *models.StreamModel) error {
	return r.db.Save(stream).Error
}

// Delete deletes stream
func (r *StreamRepositoryImpl) Delete(id string) error {
	return r.db.Delete(&models.StreamModel{}, "id = ?", id).Error
}

// Subscribe creates subscription
func (r *StreamRepositoryImpl) Subscribe(sub *models.StreamSubscriptionModel) error {
	return r.db.Create(sub).Error
}

// Unsubscribe removes subscription
func (r *StreamRepositoryImpl) Unsubscribe(streamID, userID string) error {
	return r.db.Delete(&models.StreamSubscriptionModel{}, "stream_id = ? AND user_id = ?", streamID, userID).Error
}

// GetSubscriptions gets stream subscriptions
func (r *StreamRepositoryImpl) GetSubscriptions(streamID string) ([]*models.StreamSubscriptionModel, error) {
	var subs []*models.StreamSubscriptionModel
	err := r.db.Where("stream_id = ?", streamID).Find(&subs).Error
	return subs, err
}
