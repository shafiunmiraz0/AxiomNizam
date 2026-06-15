package jobs

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// PersistentQueue wraps MemoryQueue with database persistence
type PersistentQueue struct {
	memoryQueue *MemoryQueue
	repository  JobRepository
	syncTicker  *time.Ticker
	stopChan    chan bool
}

// NewPersistentQueue creates a persistent job queue
func NewPersistentQueue(db *gorm.DB, maxSize int) *PersistentQueue {
	repo := NewPostgresJobRepository(db)

	pq := &PersistentQueue{
		memoryQueue: NewMemoryQueue(maxSize),
		repository:  repo,
		stopChan:    make(chan bool),
	}

	// Start background sync
	pq.startSync()

	return pq
}

// startSync starts background synchronization with database
func (pq *PersistentQueue) startSync() {
	pq.syncTicker = time.NewTicker(30 * time.Second)

	go func() {
		for {
			select {
			case <-pq.syncTicker.C:
				pq.syncToDB(context.Background())
			case <-pq.stopChan:
				pq.syncTicker.Stop()
				return
			}
		}
	}()
}

// syncToDB synchronizes in-memory queue to database
func (pq *PersistentQueue) syncToDB(ctx context.Context) error {
	pq.memoryQueue.mu.RLock()
	jobsCopy := make([]*Job, 0, len(pq.memoryQueue.jobs))
	for _, job := range pq.memoryQueue.jobs {
		jobsCopy = append(jobsCopy, job)
	}
	pq.memoryQueue.mu.RUnlock()

	for _, job := range jobsCopy {
		if err := pq.repository.Update(ctx, job); err != nil {
			logging.Z().Info(fmt.Sprintf("Error syncing job %s: %v", job.ID, err))
		}
	}

	return nil
}

// Submit adds a job to queue and database
func (pq *PersistentQueue) Submit(ctx context.Context, job *Job) error {
	// Save to database
	if err := pq.repository.Create(ctx, job); err != nil {
		return err
	}

	// Add to memory queue
	return pq.memoryQueue.Submit(ctx, job)
}

// Get retrieves a job
func (pq *PersistentQueue) Get(ctx context.Context, jobID string) (*Job, error) {
	// Try memory first
	if job, err := pq.memoryQueue.Get(ctx, jobID); err == nil {
		return job, nil
	}

	// Fall back to database
	return pq.repository.Get(ctx, jobID)
}

// GetByStatus retrieves jobs by status
func (pq *PersistentQueue) GetByStatus(ctx context.Context, status JobStatus, limit int) ([]*Job, error) {
	// Get from memory queue first
	jobs, err := pq.memoryQueue.GetByStatus(ctx, status, limit)
	if err != nil {
		// Fall back to database
		return pq.repository.GetByStatus(ctx, status, limit)
	}

	// If memory has some, return them
	if len(jobs) > 0 {
		return jobs, nil
	}

	// Otherwise try database
	return pq.repository.GetByStatus(ctx, status, limit)
}

// Update updates a job
func (pq *PersistentQueue) Update(ctx context.Context, job *Job) error {
	// Update in memory
	if err := pq.memoryQueue.Update(ctx, job); err != nil {
		// If not in memory, add to database
		return pq.repository.Update(ctx, job)
	}

	// Async database update
	go func() {
		pq.repository.Update(context.Background(), job)
	}()

	return nil
}

// Delete deletes a job
func (pq *PersistentQueue) Delete(ctx context.Context, jobID string) error {
	_ = pq.memoryQueue.Delete(ctx, jobID)
	return pq.repository.Delete(ctx, jobID)
}

// Clear clears all jobs
func (pq *PersistentQueue) Clear(ctx context.Context) error {
	// Note: This is dangerous, implement with caution
	return fmt.Errorf("clear not implemented for persistent queue")
}

// GetStats returns queue statistics
func (pq *PersistentQueue) GetStats(ctx context.Context) (*QueueStats, error) {
	repoStats, err := pq.repository.GetStats(ctx)
	if err != nil {
		return nil, err
	}

	// Convert RepositoryStats to QueueStats
	return &QueueStats{
		Total:       repoStats.Total,
		Pending:     repoStats.Pending,
		Running:     repoStats.Running,
		Completed:   repoStats.Completed,
		Failed:      repoStats.Failed,
		Cancelled:   repoStats.Cancelled,
		AverageTime: repoStats.AverageTime,
		OldestJob:   repoStats.OldestJob,
	}, nil
}

// RecoverPendingJobs recovers pending jobs from database on startup
func (pq *PersistentQueue) RecoverPendingJobs(ctx context.Context) (int, error) {
	pendingJobs, err := pq.repository.GetPending(ctx)
	if err != nil {
		return 0, err
	}

	for _, job := range pendingJobs {
		// Reset status to pending if it was running
		if job.Status == JobStatusRunning {
			job.Status = JobStatusPending
			job.Error = "recovered from crash"
		}

		// Add to memory queue
		if err := pq.memoryQueue.Submit(ctx, job); err != nil {
			logging.Z().Info(fmt.Sprintf("Error recovering job %s: %v", job.ID, err))
		}
	}

	logging.Z().Info(fmt.Sprintf("Recovered %d pending jobs from database", len(pendingJobs)))
	return len(pendingJobs), nil
}

// Stop stops the persistent queue
func (pq *PersistentQueue) Stop() error {
	// Final sync
	pq.syncToDB(context.Background())

	// Stop sync goroutine
	pq.stopChan <- true

	return nil
}

// CleanupOldJobs removes old completed/failed jobs from database
func (pq *PersistentQueue) CleanupOldJobs(ctx context.Context, retentionDays int) error {
	retention := time.Duration(retentionDays) * 24 * time.Hour

	// Delete old completed jobs
	deleted, err := pq.repository.DeleteCompleted(ctx, retention)
	if err != nil {
		return err
	}
	logging.Z().Info(fmt.Sprintf("Deleted %d old completed jobs", deleted))

	// Delete old failed jobs
	deleted, err = pq.repository.DeleteFailed(ctx, retention)
	if err != nil {
		return err
	}
	logging.Z().Info(fmt.Sprintf("Deleted %d old failed jobs", deleted))

	// Clear expired jobs
	deleted, err = pq.repository.ClearExpired(ctx)
	if err != nil {
		return err
	}
	logging.Z().Info(fmt.Sprintf("Deleted %d expired jobs", deleted))

	return nil
}

// Event represents a job event for tracking
type Event struct {
	ID            string                 `json:"id" gorm:"primaryKey"`
	JobID         string                 `json:"jobId" gorm:"index"`
	Type          EventType              `json:"type" gorm:"index"`
	Message       string                 `json:"message"`
	Context       map[string]interface{} `json:"context" gorm:"serializer:json"`
	Timestamp     time.Time              `json:"timestamp" gorm:"index"`
	Source        string                 `json:"source"`
	Data          map[string]interface{} `json:"data"`
	Metadata      map[string]string      `json:"metadata,omitempty"`
	UserID        string                 `json:"user_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
}

// EventType represents the type of job event
type EventType string

// Job event types
const (
	EventTypeJobCreated   EventType = "job.created"
	EventTypeJobStarted   EventType = "job.started"
	EventTypeJobCompleted EventType = "job.completed"
	EventTypeJobFailed    EventType = "job.failed"
	EventTypeJobCanceled  EventType = "job.canceled"
	EventTypeJobRetried   EventType = "job.retried"
)

// EventFilter filters job events
type EventFilter struct {
	JobID     string
	Type      EventType
	Source    string
	UserID    string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	Offset    int
}

// EventRepository defines the interface for event persistence
type EventRepository interface {
	// StoreEvent saves an event to database
	StoreEvent(ctx context.Context, event *Event) error

	// GetEventHistory retrieves event history with filtering
	GetEventHistory(ctx context.Context, limit int, filter *EventFilter) ([]*Event, error)

	// GetEventsByType retrieves events by type
	GetEventsByType(ctx context.Context, eventType EventType, limit int) ([]*Event, error)

	// DeleteOldEvents removes old events
	DeleteOldEvents(ctx context.Context, olderThan time.Duration) (int64, error)

	// GetStats returns event statistics
	GetStats(ctx context.Context) (*EventRepositoryStats, error)
}

// EventRepositoryStats contains event statistics
type EventRepositoryStats struct {
	TotalEvents   int64
	EventsByType  map[string]int64
	OldestEvent   *Event
	NewestEvent   *Event
	AveragePerDay float64
}

// PersistentEvent represents an event stored in database
type PersistentEvent struct {
	ID            string    `gorm:"primaryKey;type:varchar(100)"`
	Type          string    `gorm:"index;type:varchar(100)"`
	Source        string    `gorm:"type:varchar(100)"`
	Data          string    `gorm:"type:jsonb"`
	Timestamp     time.Time `gorm:"autoCreateTime;index"`
	UserID        string    `gorm:"index;type:varchar(100)"`
	CorrelationID string    `gorm:"type:varchar(100)"`
	Metadata      string    `gorm:"type:jsonb"`
}

// TableName sets the table name for events
func (PersistentEvent) TableName() string {
	return "events"
}

// PostgresEventRepository implements EventRepository using PostgreSQL
type PostgresEventRepository struct {
	db     *gorm.DB
}

// NewPostgresEventRepository creates a new PostgreSQL event repository
func NewPostgresEventRepository(db *gorm.DB) *PostgresEventRepository {
	repo := &PostgresEventRepository{
		db:     db,
	}

	repo.migrateSchema()
	return repo
}

// migrateSchema creates the events table
func (per *PostgresEventRepository) migrateSchema() {
	if err := per.db.AutoMigrate(&PersistentEvent{}); err != nil {
		logging.Z().Info(fmt.Sprintf("Error migrating schema: %v", err))
	}

	per.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
		CREATE INDEX IF NOT EXISTS idx_events_user_id ON events(user_id);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_events_correlation_id ON events(correlation_id);
	`)

	logging.Z().Info(fmt.Sprintf("Event schema migration completed"))
}

// StoreEvent saves an event
func (per *PostgresEventRepository) StoreEvent(ctx context.Context, event *Event) error {
	dataJSON, _ := json.Marshal(event.Data)
	metadataJSON, _ := json.Marshal(event.Metadata)

	pe := &PersistentEvent{
		ID:            event.ID,
		Type:          string(event.Type),
		Source:        event.Source,
		Data:          string(dataJSON),
		UserID:        event.UserID,
		CorrelationID: event.CorrelationID,
		Metadata:      string(metadataJSON),
		Timestamp:     event.Timestamp,
	}

	if err := per.db.WithContext(ctx).Create(pe).Error; err != nil {
		logging.Z().Info(fmt.Sprintf("Error storing event: %v", err))
		return err
	}

	return nil
}

// GetEventHistory retrieves events with filtering
func (per *PostgresEventRepository) GetEventHistory(ctx context.Context, limit int, filter *EventFilter) ([]*Event, error) {
	var pes []PersistentEvent
	query := per.db.WithContext(ctx)

	if filter != nil {
		if filter.Type != "" {
			query = query.Where("type = ?", string(filter.Type))
		}
		if filter.Source != "" {
			query = query.Where("source = ?", filter.Source)
		}
		if filter.UserID != "" {
			query = query.Where("user_id = ?", filter.UserID)
		}
		if !filter.StartTime.IsZero() {
			query = query.Where("timestamp >= ?", filter.StartTime)
		}
		if !filter.EndTime.IsZero() {
			query = query.Where("timestamp <= ?", filter.EndTime)
		}
	}

	if err := query.
		Order("timestamp DESC").
		Limit(limit).
		Find(&pes).Error; err != nil {
		return nil, err
	}

	var events []*Event
	for _, pe := range pes {
		event := per.toEvent(&pe)
		events = append(events, event)
	}

	return events, nil
}

// GetEventsByType retrieves events by type
func (per *PostgresEventRepository) GetEventsByType(ctx context.Context, eventType EventType, limit int) ([]*Event, error) {
	var pes []PersistentEvent
	if err := per.db.WithContext(ctx).
		Where("type = ?", string(eventType)).
		Order("timestamp DESC").
		Limit(limit).
		Find(&pes).Error; err != nil {
		return nil, err
	}

	var events []*Event
	for _, pe := range pes {
		events = append(events, per.toEvent(&pe))
	}

	return events, nil
}

// DeleteOldEvents removes old events
func (per *PostgresEventRepository) DeleteOldEvents(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result := per.db.WithContext(ctx).
		Where("timestamp < ?", cutoff).
		Delete(&PersistentEvent{})

	return result.RowsAffected, result.Error
}

// GetStats returns event statistics
func (per *PostgresEventRepository) GetStats(ctx context.Context) (*EventRepositoryStats, error) {
	stats := &EventRepositoryStats{
		EventsByType: make(map[string]int64),
	}

	// Total events
	per.db.WithContext(ctx).Model(&PersistentEvent{}).Count(&stats.TotalEvents)

	// Events by type
	var typeCounts []struct {
		Type  string
		Count int64
	}
	per.db.WithContext(ctx).Model(&PersistentEvent{}).
		Select("type, count(*) as count").
		Group("type").
		Find(&typeCounts)

	for _, tc := range typeCounts {
		stats.EventsByType[tc.Type] = tc.Count
	}

	// Oldest event
	var oldestPe PersistentEvent
	if err := per.db.WithContext(ctx).
		Order("timestamp ASC").
		First(&oldestPe).Error; err == nil {
		stats.OldestEvent = per.toEvent(&oldestPe)
	}

	// Newest event
	var newestPe PersistentEvent
	if err := per.db.WithContext(ctx).
		Order("timestamp DESC").
		First(&newestPe).Error; err == nil {
		stats.NewestEvent = per.toEvent(&newestPe)
	}

	return stats, nil
}

// toEvent converts PersistentEvent to Event
func (per *PostgresEventRepository) toEvent(pe *PersistentEvent) *Event {
	data := make(map[string]interface{})
	if pe.Data != "" {
		json.Unmarshal([]byte(pe.Data), &data)
	}

	metadata := make(map[string]string)
	if pe.Metadata != "" {
		json.Unmarshal([]byte(pe.Metadata), &metadata)
	}

	return &Event{
		ID:            pe.ID,
		Type:          EventType(pe.Type),
		Source:        pe.Source,
		Data:          data,
		Timestamp:     pe.Timestamp,
		UserID:        pe.UserID,
		CorrelationID: pe.CorrelationID,
		Metadata:      metadata,
	}
}

// Helper functions for JSON marshaling
func marshalJSON(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func unmarshalJSON(data string, v interface{}) error {
	return json.Unmarshal([]byte(data), v)
}
