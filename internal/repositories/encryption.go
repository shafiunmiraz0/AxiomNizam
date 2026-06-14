package repositories

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"

	"gorm.io/gorm"
)

// EncryptionRepository interface for encryption operations
type EncryptionRepository interface {
	CreateKey(key *models.EncryptionKeyModel) error
	GetKey(id string) (*models.EncryptionKeyModel, error)
	ListKeys(tenantID string, limit, offset int) ([]*models.EncryptionKeyModel, error)
	UpdateKey(key *models.EncryptionKeyModel) error
	CreatePolicy(policy *models.EncryptionPolicyModel) error
	GetPolicy(id string) (*models.EncryptionPolicyModel, error)
	ListPolicies(tenantID string) ([]*models.EncryptionPolicyModel, error)
	DeletePolicy(id string) error
	CreateRotation(rotation *models.KeyRotationModel) error
	ListRotations(keyID string) ([]*models.KeyRotationModel, error)
	AddAuditLog(log *models.EncryptionAuditLogModel) error
	GetAuditLogs(keyID string) ([]*models.EncryptionAuditLogModel, error)
}

// EncryptionRepositoryImpl implements EncryptionRepository
type EncryptionRepositoryImpl struct {
	db *gorm.DB
}

// NewEncryptionRepository creates encryption repository
func NewEncryptionRepository(db *gorm.DB) EncryptionRepository {
	return &EncryptionRepositoryImpl{db: db}
}

// CreateKey creates encryption key
func (r *EncryptionRepositoryImpl) CreateKey(key *models.EncryptionKeyModel) error {
	return r.db.Create(key).Error
}

// GetKey retrieves key by ID
func (r *EncryptionRepositoryImpl) GetKey(id string) (*models.EncryptionKeyModel, error) {
	var key models.EncryptionKeyModel
	err := r.db.Preload("Rotations").Preload("AuditLogs").Where("id = ?", id).First(&key).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("key not found")
	}
	return &key, err
}

// ListKeys lists keys
func (r *EncryptionRepositoryImpl) ListKeys(tenantID string, limit, offset int) ([]*models.EncryptionKeyModel, error) {
	var keys []*models.EncryptionKeyModel
	query := r.db.Where("tenant_id = ?", tenantID)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Order("created_at DESC").Find(&keys).Error
	return keys, err
}

// UpdateKey updates key
func (r *EncryptionRepositoryImpl) UpdateKey(key *models.EncryptionKeyModel) error {
	return r.db.Save(key).Error
}

// CreatePolicy creates encryption policy
func (r *EncryptionRepositoryImpl) CreatePolicy(policy *models.EncryptionPolicyModel) error {
	return r.db.Create(policy).Error
}

// GetPolicy retrieves policy by ID
func (r *EncryptionRepositoryImpl) GetPolicy(id string) (*models.EncryptionPolicyModel, error) {
	var policy models.EncryptionPolicyModel
	err := r.db.Where("id = ?", id).First(&policy).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("policy not found")
	}
	return &policy, err
}

// ListPolicies lists policies
func (r *EncryptionRepositoryImpl) ListPolicies(tenantID string) ([]*models.EncryptionPolicyModel, error) {
	var policies []*models.EncryptionPolicyModel
	err := r.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&policies).Error
	return policies, err
}

// DeletePolicy deletes policy
func (r *EncryptionRepositoryImpl) DeletePolicy(id string) error {
	return r.db.Delete(&models.EncryptionPolicyModel{}, "id = ?", id).Error
}

// CreateRotation creates key rotation record
func (r *EncryptionRepositoryImpl) CreateRotation(rotation *models.KeyRotationModel) error {
	return r.db.Create(rotation).Error
}

// ListRotations lists rotations for key
func (r *EncryptionRepositoryImpl) ListRotations(keyID string) ([]*models.KeyRotationModel, error) {
	var rotations []*models.KeyRotationModel
	err := r.db.Where("key_id = ?", keyID).Order("rotated_at DESC").Find(&rotations).Error
	return rotations, err
}

// AddAuditLog adds audit log
func (r *EncryptionRepositoryImpl) AddAuditLog(log *models.EncryptionAuditLogModel) error {
	return r.db.Create(log).Error
}

// GetAuditLogs gets audit logs for key
func (r *EncryptionRepositoryImpl) GetAuditLogs(keyID string) ([]*models.EncryptionAuditLogModel, error) {
	var logs []*models.EncryptionAuditLogModel
	err := r.db.Where("key_id = ?", keyID).Order("timestamp DESC").Find(&logs).Error
	return logs, err
}
