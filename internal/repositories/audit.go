package repositories

import (
	"fmt"

	"example.com/axiomnizam/internal/models"

	"gorm.io/gorm"
)

// AuditRepository interface for audit operations
type AuditRepository interface {
	Create(log *models.AuditLogModel) error
	GetByID(id string) (*models.AuditLogModel, error)
	List(tenantID string, limit, offset int) ([]*models.AuditLogModel, error)
	ListByAction(tenantID, action string, limit, offset int) ([]*models.AuditLogModel, error)
	ListByResource(tenantID, resourceType, resourceID string) ([]*models.AuditLogModel, error)
	DeleteOldLogs(tenantID string, daysOld int) error
}

// AuditRepositoryImpl implements AuditRepository
type AuditRepositoryImpl struct {
	db *gorm.DB
}

// NewAuditRepository creates audit repository
func NewAuditRepository(db *gorm.DB) AuditRepository {
	return &AuditRepositoryImpl{db: db}
}

// Create creates audit log
func (r *AuditRepositoryImpl) Create(log *models.AuditLogModel) error {
	return r.db.Create(log).Error
}

// GetByID retrieves audit log by ID
func (r *AuditRepositoryImpl) GetByID(id string) (*models.AuditLogModel, error) {
	var log models.AuditLogModel
	err := r.db.Where("id = ?", id).First(&log).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("audit log not found")
	}
	return &log, err
}

// List lists audit logs
func (r *AuditRepositoryImpl) List(tenantID string, limit, offset int) ([]*models.AuditLogModel, error) {
	var logs []*models.AuditLogModel
	query := r.db.Where("tenant_id = ?", tenantID)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Order("created_at DESC").Find(&logs).Error
	return logs, err
}

// ListByAction lists logs by action
func (r *AuditRepositoryImpl) ListByAction(tenantID, action string, limit, offset int) ([]*models.AuditLogModel, error) {
	var logs []*models.AuditLogModel
	query := r.db.Where("tenant_id = ? AND action_type = ?", tenantID, action)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Order("created_at DESC").Find(&logs).Error
	return logs, err
}

// ListByResource lists logs by resource
func (r *AuditRepositoryImpl) ListByResource(tenantID, resourceType, resourceID string) ([]*models.AuditLogModel, error) {
	var logs []*models.AuditLogModel
	query := r.db.Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", tenantID, resourceType, resourceID)
	err := query.Order("created_at DESC").Find(&logs).Error
	return logs, err
}

// DeleteOldLogs deletes logs older than specified days.
// Phase 7: Fixed MySQL syntax to PostgreSQL INTERVAL syntax.
func (r *AuditRepositoryImpl) DeleteOldLogs(tenantID string, daysOld int) error {
	return r.db.Where("tenant_id = ? AND created_at < NOW() - INTERVAL '? days'", tenantID, daysOld).
		Delete(&models.AuditLogModel{}).Error
}
