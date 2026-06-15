package repositories

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"gorm.io/gorm"
)

// JobRepository interface for job operations
type JobRepository interface {
	Create(job *models.JobModel) error
	GetByID(id string) (*models.JobModel, error)
	List(tenantID, status string, limit, offset int) ([]*models.JobModel, error)
	Update(job *models.JobModel) error
	AddLog(log *models.JobLogModel) error
	GetLogs(jobID string) ([]*models.JobLogModel, error)
}

// JobRepositoryImpl implements JobRepository
type JobRepositoryImpl struct {
	db *gorm.DB
}

// NewJobRepository creates job repository
func NewJobRepository(db *gorm.DB) JobRepository {
	return &JobRepositoryImpl{db: db}
}

// Create creates job
func (r *JobRepositoryImpl) Create(job *models.JobModel) error {
	return r.db.Create(job).Error
}

// GetByID retrieves job by ID
func (r *JobRepositoryImpl) GetByID(id string) (*models.JobModel, error) {
	var job models.JobModel
	err := r.db.Preload("Logs").Where("id = ?", id).First(&job).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("job not found")
	}
	return &job, err
}

// List lists jobs
func (r *JobRepositoryImpl) List(tenantID, status string, limit, offset int) ([]*models.JobModel, error) {
	var jobs []*models.JobModel
	query := r.db.Preload("Logs").Where("tenant_id = ?", tenantID)
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

// Update updates job
func (r *JobRepositoryImpl) Update(job *models.JobModel) error {
	return r.db.Save(job).Error
}

// AddLog adds job log
func (r *JobRepositoryImpl) AddLog(log *models.JobLogModel) error {
	return r.db.Create(log).Error
}

// GetLogs gets job logs
func (r *JobRepositoryImpl) GetLogs(jobID string) ([]*models.JobLogModel, error) {
	var logs []*models.JobLogModel
	err := r.db.Where("job_id = ?", jobID).Order("created_at ASC").Find(&logs).Error
	return logs, err
}
