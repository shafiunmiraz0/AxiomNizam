package repositories

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"

	"gorm.io/gorm"
)

// ExportRepository interface for export operations
type ExportRepository interface {
	Create(job *models.ExportJobModel) error
	GetByID(id string) (*models.ExportJobModel, error)
	List(tenantID, status string, limit, offset int) ([]*models.ExportJobModel, error)
	Update(job *models.ExportJobModel) error
	AddResult(result *models.ExportResultModel) error
	GetResults(exportID string) ([]*models.ExportResultModel, error)
	CreateTemplate(template *models.ExportTemplateModel) error
	GetTemplate(id string) (*models.ExportTemplateModel, error)
	ListTemplates(tenantID string) ([]*models.ExportTemplateModel, error)
	DeleteTemplate(id string) error
}

// ExportRepositoryImpl implements ExportRepository
type ExportRepositoryImpl struct {
	db *gorm.DB
}

// NewExportRepository creates export repository
func NewExportRepository(db *gorm.DB) ExportRepository {
	return &ExportRepositoryImpl{db: db}
}

// Create creates export job
func (r *ExportRepositoryImpl) Create(job *models.ExportJobModel) error {
	return r.db.Create(job).Error
}

// GetByID retrieves export job by ID
func (r *ExportRepositoryImpl) GetByID(id string) (*models.ExportJobModel, error) {
	var job models.ExportJobModel
	err := r.db.Preload("Results").Where("id = ?", id).First(&job).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("export not found")
	}
	return &job, err
}

// List lists export jobs
func (r *ExportRepositoryImpl) List(tenantID, status string, limit, offset int) ([]*models.ExportJobModel, error) {
	var jobs []*models.ExportJobModel
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
	err := query.Order("created_at DESC").Find(&jobs).Error
	return jobs, err
}

// Update updates export job
func (r *ExportRepositoryImpl) Update(job *models.ExportJobModel) error {
	return r.db.Save(job).Error
}

// AddResult adds result to export
func (r *ExportRepositoryImpl) AddResult(result *models.ExportResultModel) error {
	return r.db.Create(result).Error
}

// GetResults gets export results
func (r *ExportRepositoryImpl) GetResults(exportID string) ([]*models.ExportResultModel, error) {
	var results []*models.ExportResultModel
	err := r.db.Where("export_id = ?", exportID).Order("created_at ASC").Find(&results).Error
	return results, err
}

// CreateTemplate creates export template
func (r *ExportRepositoryImpl) CreateTemplate(template *models.ExportTemplateModel) error {
	return r.db.Create(template).Error
}

// GetTemplate retrieves template
func (r *ExportRepositoryImpl) GetTemplate(id string) (*models.ExportTemplateModel, error) {
	var template models.ExportTemplateModel
	err := r.db.Where("id = ?", id).First(&template).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("template not found")
	}
	return &template, err
}

// ListTemplates lists templates
func (r *ExportRepositoryImpl) ListTemplates(tenantID string) ([]*models.ExportTemplateModel, error) {
	var templates []*models.ExportTemplateModel
	err := r.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&templates).Error
	return templates, err
}

// DeleteTemplate deletes template
func (r *ExportRepositoryImpl) DeleteTemplate(id string) error {
	return r.db.Delete(&models.ExportTemplateModel{}, "id = ?", id).Error
}
