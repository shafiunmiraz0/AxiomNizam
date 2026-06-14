package repositories

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"gorm.io/gorm"
)

// TenantRepository interface for tenant operations
type TenantRepository interface {
	Create(tenant *models.TenantModel) error
	GetByID(id string) (*models.TenantModel, error)
	List(limit, offset int) ([]*models.TenantModel, error)
	Update(tenant *models.TenantModel) error
	Delete(id string) error
	AddMember(member *models.TenantMemberModel) error
	RemoveMember(tenantID, userID string) error
	GetMembers(tenantID string) ([]*models.TenantMemberModel, error)
	GetQuota(tenantID, resource string) (*models.TenantQuotaModel, error)
	UpdateQuota(quota *models.TenantQuotaModel) error
}

// TenantRepositoryImpl implements TenantRepository
type TenantRepositoryImpl struct {
	db *gorm.DB
}

// NewTenantRepository creates tenant repository
func NewTenantRepository(db *gorm.DB) TenantRepository {
	return &TenantRepositoryImpl{db: db}
}

// Create creates tenant
func (r *TenantRepositoryImpl) Create(tenant *models.TenantModel) error {
	return r.db.Create(tenant).Error
}

// GetByID retrieves tenant by ID
func (r *TenantRepositoryImpl) GetByID(id string) (*models.TenantModel, error) {
	var tenant models.TenantModel
	err := r.db.Preload("Members").Preload("Quotas").Where("id = ?", id).First(&tenant).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("tenant not found")
	}
	return &tenant, err
}

// List lists tenants
func (r *TenantRepositoryImpl) List(limit, offset int) ([]*models.TenantModel, error) {
	var tenants []*models.TenantModel
	query := r.db.Preload("Members").Preload("Quotas")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Order("created_at DESC").Find(&tenants).Error
	return tenants, err
}

// Update updates tenant
func (r *TenantRepositoryImpl) Update(tenant *models.TenantModel) error {
	return r.db.Save(tenant).Error
}

// Delete deletes tenant
func (r *TenantRepositoryImpl) Delete(id string) error {
	return r.db.Delete(&models.TenantModel{}, "id = ?", id).Error
}

// AddMember adds tenant member
func (r *TenantRepositoryImpl) AddMember(member *models.TenantMemberModel) error {
	return r.db.Create(member).Error
}

// RemoveMember removes tenant member
func (r *TenantRepositoryImpl) RemoveMember(tenantID, userID string) error {
	return r.db.Delete(&models.TenantMemberModel{}, "tenant_id = ? AND user_id = ?", tenantID, userID).Error
}

// GetMembers gets tenant members
func (r *TenantRepositoryImpl) GetMembers(tenantID string) ([]*models.TenantMemberModel, error) {
	var members []*models.TenantMemberModel
	err := r.db.Where("tenant_id = ?", tenantID).Find(&members).Error
	return members, err
}

// GetQuota gets tenant quota
func (r *TenantRepositoryImpl) GetQuota(tenantID, resource string) (*models.TenantQuotaModel, error) {
	var quota models.TenantQuotaModel
	err := r.db.Where("tenant_id = ? AND resource = ?", tenantID, resource).First(&quota).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("quota not found")
	}
	return &quota, err
}

// UpdateQuota updates quota
func (r *TenantRepositoryImpl) UpdateQuota(quota *models.TenantQuotaModel) error {
	return r.db.Save(quota).Error
}
