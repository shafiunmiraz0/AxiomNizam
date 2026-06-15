package repositories

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"

	"gorm.io/gorm"
)

// BulkRepository interface for bulk operations
type BulkRepository interface {
	Create(op *models.BulkOperationModel) error
	GetByID(id string) (*models.BulkOperationModel, error)
	List(tenantID, status string, limit, offset int) ([]*models.BulkOperationModel, error)
	Update(op *models.BulkOperationModel) error
	AddResult(result *models.BulkResultModel) error
	GetResults(operationID string) ([]*models.BulkResultModel, error)
}

// BulkRepositoryImpl implements BulkRepository
type BulkRepositoryImpl struct {
	db *gorm.DB
}

// NewBulkRepository creates bulk repository
func NewBulkRepository(db *gorm.DB) BulkRepository {
	return &BulkRepositoryImpl{db: db}
}

// Create creates bulk operation
func (r *BulkRepositoryImpl) Create(op *models.BulkOperationModel) error {
	return r.db.Create(op).Error
}

// GetByID retrieves bulk operation by ID
func (r *BulkRepositoryImpl) GetByID(id string) (*models.BulkOperationModel, error) {
	var op models.BulkOperationModel
	err := r.db.Preload("Results").Where("id = ?", id).First(&op).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("operation not found")
	}
	return &op, err
}

// List lists bulk operations
func (r *BulkRepositoryImpl) List(tenantID, status string, limit, offset int) ([]*models.BulkOperationModel, error) {
	var ops []*models.BulkOperationModel
	query := r.db.Preload("Results").Where("tenant_id = ?", tenantID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Order("created_at DESC").Find(&ops).Error
	return ops, err
}

// Update updates bulk operation
func (r *BulkRepositoryImpl) Update(op *models.BulkOperationModel) error {
	return r.db.Save(op).Error
}

// AddResult adds result to operation
func (r *BulkRepositoryImpl) AddResult(result *models.BulkResultModel) error {
	return r.db.Create(result).Error
}

// GetResults gets operation results
func (r *BulkRepositoryImpl) GetResults(operationID string) ([]*models.BulkResultModel, error) {
	var results []*models.BulkResultModel
	err := r.db.Where("operation_id = ?", operationID).Order("item_index ASC").Find(&results).Error
	return results, err
}
