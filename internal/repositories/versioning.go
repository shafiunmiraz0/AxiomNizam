package repositories

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"

	"gorm.io/gorm"
)

// VersionRepository interface for version operations
type VersionRepository interface {
	Create(version *models.ResourceVersionModel) error
	GetByID(id string) (*models.ResourceVersionModel, error)
	GetByResourceVersion(tenantID, resourceType, resourceID string, versionNum int) (*models.ResourceVersionModel, error)
	List(tenantID, resourceType, resourceID string) ([]*models.ResourceVersionModel, error)
	CreateSnapshot(snapshot *models.VersionSnapshotModel) error
	GetSnapshots(versionID string) ([]*models.VersionSnapshotModel, error)
}

// VersionRepositoryImpl implements VersionRepository
type VersionRepositoryImpl struct {
	db *gorm.DB
}

// NewVersionRepository creates version repository
func NewVersionRepository(db *gorm.DB) VersionRepository {
	return &VersionRepositoryImpl{db: db}
}

// Create creates resource version
func (r *VersionRepositoryImpl) Create(version *models.ResourceVersionModel) error {
	return r.db.Create(version).Error
}

// GetByID retrieves version by ID
func (r *VersionRepositoryImpl) GetByID(id string) (*models.ResourceVersionModel, error) {
	var version models.ResourceVersionModel
	err := r.db.Preload("Snapshots").Where("id = ?", id).First(&version).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("version not found")
	}
	return &version, err
}

// GetByResourceVersion retrieves specific resource version
func (r *VersionRepositoryImpl) GetByResourceVersion(tenantID, resourceType, resourceID string, versionNum int) (*models.ResourceVersionModel, error) {
	var version models.ResourceVersionModel
	err := r.db.Preload("Snapshots").
		Where("tenant_id = ? AND resource_type = ? AND resource_id = ? AND version_number = ?",
			tenantID, resourceType, resourceID, versionNum).
		First(&version).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("version not found")
	}
	return &version, err
}

// List lists resource versions
func (r *VersionRepositoryImpl) List(tenantID, resourceType, resourceID string) ([]*models.ResourceVersionModel, error) {
	var versions []*models.ResourceVersionModel
	query := r.db.Preload("Snapshots").
		Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", tenantID, resourceType, resourceID)
	err := query.Order("version_number DESC").Find(&versions).Error
	return versions, err
}

// CreateSnapshot creates version snapshot
func (r *VersionRepositoryImpl) CreateSnapshot(snapshot *models.VersionSnapshotModel) error {
	return r.db.Create(snapshot).Error
}

// GetSnapshots gets version snapshots
func (r *VersionRepositoryImpl) GetSnapshots(versionID string) ([]*models.VersionSnapshotModel, error) {
	var snapshots []*models.VersionSnapshotModel
	err := r.db.Where("version_id = ?", versionID).Order("created_at DESC").Find(&snapshots).Error
	return snapshots, err
}
