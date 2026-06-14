package jobs

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// PersistentJob represents a job stored in database
type PersistentJob struct {
	ID          string `gorm:"primaryKey;type:varchar(100)"`
	Type        string `gorm:"index;type:varchar(50)"`
	Status      string `gorm:"index;type:varchar(20)"`
	Priority    int    `gorm:"index"`
	Data        string `gorm:"type:jsonb"` // JSON stored as text
	Result      string `gorm:"type:jsonb"`
	Error       string `gorm:"type:text"`
	Retries     int
	MaxRetries  int
	CreatedAt   time.Time `gorm:"autoCreateTime;index"`
	StartedAt   *time.Time
	CompletedAt *time.Time
	Timeout     string // Duration as string
	Tags        string // JSON array as string
	CallbackURL string
	DeadlineAt  *time.Time
	UpdatedAt   time.Time `gorm:"autoUpdateTime;index"`
}

// TableName sets the table name for the model
func (PersistentJob) TableName() string {
	return "jobs"
}

// JobRepository defines the interface for job persistence
type JobRepository interface {
	// Create saves a new job
	Create(ctx context.Context, job *Job) error

	// Update updates an existing job
	Update(ctx context.Context, job *Job) error

	// Get retrieves a job by ID
	Get(ctx context.Context, jobID string) (*Job, error)

	// GetByStatus retrieves jobs by status
	GetByStatus(ctx context.Context, status JobStatus, limit int) ([]*Job, error)

	// GetByType retrieves jobs by type
	GetByType(ctx context.Context, jobType JobType, limit int) ([]*Job, error)

	// Delete deletes a job
	Delete(ctx context.Context, jobID string) error

	// DeleteCompleted deletes completed jobs older than duration
	DeleteCompleted(ctx context.Context, olderThan time.Duration) (int64, error)

	// DeleteFailed deletes failed jobs older than duration
	DeleteFailed(ctx context.Context, olderThan time.Duration) (int64, error)

	// GetStats retrieves statistics
	GetStats(ctx context.Context) (*RepositoryStats, error)

	// GetPending retrieves all pending jobs (for recovery)
	GetPending(ctx context.Context) ([]*Job, error)

	// ClearExpired removes expired jobs
	ClearExpired(ctx context.Context) (int64, error)
}

// RepositoryStats contains repository statistics
type RepositoryStats struct {
	Total       int64
	Pending     int64
	Running     int64
	Completed   int64
	Failed      int64
	Cancelled   int64
	AverageTime time.Duration
	OldestJob   *Job
}

// PostgresJobRepository implements JobRepository using PostgreSQL
type PostgresJobRepository struct {
	db     *gorm.DB
}

// NewPostgresJobRepository creates a new PostgreSQL job repository
func NewPostgresJobRepository(db *gorm.DB) *PostgresJobRepository {
	repo := &PostgresJobRepository{
		db:     db,
	}

	// Auto-migrate schema
	repo.migrateSchema()

	return repo
}

// migrateSchema creates the jobs table if it doesn't exist
func (pjr *PostgresJobRepository) migrateSchema() {
	if err := pjr.db.AutoMigrate(&PersistentJob{}); err != nil {
		logging.Z().Info(fmt.Sprintf("Error migrating schema: %v", err))
	}

	// Create indexes
	pjr.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
		CREATE INDEX IF NOT EXISTS idx_jobs_type ON jobs(type);
		CREATE INDEX IF NOT EXISTS idx_jobs_priority ON jobs(priority);
		CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_jobs_completed_at ON jobs(completed_at DESC);
	`)

	logging.Z().Info(fmt.Sprintf("Schema migration completed"))
}

// toPersistent converts Job to PersistentJob
func (pjr *PostgresJobRepository) toPersistent(job *Job) (*PersistentJob, error) {
	dataJSON, _ := marshalJSON(job.Data)
	resultJSON, _ := marshalJSON(job.Result)
	tagsJSON, _ := marshalJSON(job.Tags)

	pj := &PersistentJob{
		ID:          job.ID,
		Type:        string(job.Type),
		Status:      string(job.Status),
		Priority:    int(job.Priority),
		Data:        dataJSON,
		Result:      resultJSON,
		Error:       job.Error,
		Retries:     job.Retries,
		MaxRetries:  job.MaxRetries,
		CreatedAt:   job.CreatedAt,
		Timeout:     job.Timeout.String(),
		Tags:        tagsJSON,
		CallbackURL: job.CallbackURL,
	}

	if !job.StartedAt.IsZero() {
		pj.StartedAt = &job.StartedAt
	}

	if !job.CompletedAt.IsZero() {
		pj.CompletedAt = &job.CompletedAt
	}

	if !job.DeadlineAt.IsZero() {
		pj.DeadlineAt = &job.DeadlineAt
	}

	return pj, nil
}

// toJob converts PersistentJob to Job
func (pjr *PostgresJobRepository) toJob(pj *PersistentJob) (*Job, error) {
	data := make(map[string]interface{})
	if pj.Data != "" {
		unmarshalJSON(pj.Data, &data)
	}

	result := make(map[string]interface{})
	if pj.Result != "" {
		unmarshalJSON(pj.Result, &result)
	}

	var tags []string
	if pj.Tags != "" {
		unmarshalJSON(pj.Tags, &tags)
	}

	timeout, _ := time.ParseDuration(pj.Timeout)

	job := &Job{
		ID:          pj.ID,
		Type:        JobType(pj.Type),
		Status:      JobStatus(pj.Status),
		Priority:    JobPriority(pj.Priority),
		Data:        data,
		Result:      result,
		Error:       pj.Error,
		Retries:     pj.Retries,
		MaxRetries:  pj.MaxRetries,
		CreatedAt:   pj.CreatedAt,
		StartedAt:   time.Time{},
		CompletedAt: time.Time{},
		Timeout:     timeout,
		Tags:        tags,
		CallbackURL: pj.CallbackURL,
	}

	if pj.StartedAt != nil {
		job.StartedAt = *pj.StartedAt
	}

	if pj.CompletedAt != nil {
		job.CompletedAt = *pj.CompletedAt
	}

	if pj.DeadlineAt != nil {
		job.DeadlineAt = *pj.DeadlineAt
	}

	return job, nil
}

// Create saves a new job
func (pjr *PostgresJobRepository) Create(ctx context.Context, job *Job) error {
	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	pj, err := pjr.toPersistent(job)
	if err != nil {
		return err
	}

	if err := pjr.db.WithContext(ctx).Create(pj).Error; err != nil {
		logging.Z().Info(fmt.Sprintf("Error creating job: %v", err))
		return err
	}

	logging.Z().Info(fmt.Sprintf("Job created: %s", job.ID))
	return nil
}

// Update updates an existing job
func (pjr *PostgresJobRepository) Update(ctx context.Context, job *Job) error {
	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	pj, err := pjr.toPersistent(job)
	if err != nil {
		return err
	}

	if err := pjr.db.WithContext(ctx).Save(pj).Error; err != nil {
		logging.Z().Info(fmt.Sprintf("Error updating job: %v", err))
		return err
	}

	logging.Z().Info(fmt.Sprintf("Job updated: %s", job.ID))
	return nil
}

// Get retrieves a job by ID
func (pjr *PostgresJobRepository) Get(ctx context.Context, jobID string) (*Job, error) {
	var pj PersistentJob
	if err := pjr.db.WithContext(ctx).First(&pj, "id = ?", jobID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrJobNotFound
		}
		return nil, err
	}

	return pjr.toJob(&pj)
}

// GetByStatus retrieves jobs by status
func (pjr *PostgresJobRepository) GetByStatus(ctx context.Context, status JobStatus, limit int) ([]*Job, error) {
	var pjs []PersistentJob
	if err := pjr.db.WithContext(ctx).
		Where("status = ?", string(status)).
		Order("priority DESC, created_at ASC").
		Limit(limit).
		Find(&pjs).Error; err != nil {
		return nil, err
	}

	var jobs []*Job
	for _, pj := range pjs {
		job, err := pjr.toJob(&pj)
		if err != nil {
			continue
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// GetByType retrieves jobs by type
func (pjr *PostgresJobRepository) GetByType(ctx context.Context, jobType JobType, limit int) ([]*Job, error) {
	var pjs []PersistentJob
	if err := pjr.db.WithContext(ctx).
		Where("type = ?", string(jobType)).
		Order("priority DESC, created_at ASC").
		Limit(limit).
		Find(&pjs).Error; err != nil {
		return nil, err
	}

	var jobs []*Job
	for _, pj := range pjs {
		job, err := pjr.toJob(&pj)
		if err != nil {
			continue
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// Delete deletes a job
func (pjr *PostgresJobRepository) Delete(ctx context.Context, jobID string) error {
	if err := pjr.db.WithContext(ctx).Delete(&PersistentJob{}, "id = ?", jobID).Error; err != nil {
		return err
	}

	logging.Z().Info(fmt.Sprintf("Job deleted: %s", jobID))
	return nil
}

// DeleteCompleted deletes completed jobs older than duration
func (pjr *PostgresJobRepository) DeleteCompleted(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result := pjr.db.WithContext(ctx).
		Where("status = ? AND completed_at < ?", string(JobStatusCompleted), cutoff).
		Delete(&PersistentJob{})

	return result.RowsAffected, result.Error
}

// DeleteFailed deletes failed jobs older than duration
func (pjr *PostgresJobRepository) DeleteFailed(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result := pjr.db.WithContext(ctx).
		Where("status = ? AND completed_at < ?", string(JobStatusFailed), cutoff).
		Delete(&PersistentJob{})

	return result.RowsAffected, result.Error
}

// GetStats retrieves statistics
func (pjr *PostgresJobRepository) GetStats(ctx context.Context) (*RepositoryStats, error) {
	stats := &RepositoryStats{}

	// Count by status
	pjr.db.WithContext(ctx).Model(&PersistentJob{}).
		Where("status = ?", string(JobStatusPending)).
		Count(&stats.Pending)

	pjr.db.WithContext(ctx).Model(&PersistentJob{}).
		Where("status = ?", string(JobStatusRunning)).
		Count(&stats.Running)

	pjr.db.WithContext(ctx).Model(&PersistentJob{}).
		Where("status = ?", string(JobStatusCompleted)).
		Count(&stats.Completed)

	pjr.db.WithContext(ctx).Model(&PersistentJob{}).
		Where("status = ?", string(JobStatusFailed)).
		Count(&stats.Failed)

	pjr.db.WithContext(ctx).Model(&PersistentJob{}).
		Where("status = ?", string(JobStatusCancelled)).
		Count(&stats.Cancelled)

	stats.Total = stats.Pending + stats.Running + stats.Completed + stats.Failed + stats.Cancelled

	// Get oldest job
	var pj PersistentJob
	if err := pjr.db.WithContext(ctx).
		Order("created_at ASC").
		First(&pj).Error; err == nil {
		stats.OldestJob, _ = pjr.toJob(&pj)
	}

	return stats, nil
}

// GetPending retrieves all pending jobs (for recovery on startup)
func (pjr *PostgresJobRepository) GetPending(ctx context.Context) ([]*Job, error) {
	var pjs []PersistentJob
	if err := pjr.db.WithContext(ctx).
		Where("status IN ?", []string{
			string(JobStatusPending),
			string(JobStatusRunning),
			string(JobStatusRetrying),
		}).
		Order("priority DESC, created_at ASC").
		Find(&pjs).Error; err != nil {
		return nil, err
	}

	var jobs []*Job
	for _, pj := range pjs {
		job, err := pjr.toJob(&pj)
		if err != nil {
			continue
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// ClearExpired removes expired jobs
func (pjr *PostgresJobRepository) ClearExpired(ctx context.Context) (int64, error) {
	result := pjr.db.WithContext(ctx).
		Where("deadline_at IS NOT NULL AND deadline_at < ?", time.Now()).
		Delete(&PersistentJob{})

	return result.RowsAffected, result.Error
}
