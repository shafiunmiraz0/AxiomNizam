package jobs

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"time"
)

// DataCleanupConfig holds cleanup configuration
type DataCleanupConfig struct {
	RetentionDays   int
	BatchSize       int
	MaxDuration     time.Duration
	ParallelWorkers int
}

// DefaultDataCleanupConfig returns default cleanup config
func DefaultDataCleanupConfig() *DataCleanupConfig {
	return &DataCleanupConfig{
		RetentionDays:   30,
		BatchSize:       1000,
		MaxDuration:     5 * time.Minute,
		ParallelWorkers: 4,
	}
}

// CleanupOperation represents a data cleanup operation
type CleanupOperation struct {
	ID            string
	OperationType string // logs, temp_files, expired_tokens, old_sessions, etc.
	Status        string // pending, running, completed, failed
	StartTime     time.Time
	EndTime       *time.Time
	ItemsDeleted  int
	Error         string
	Config        *DataCleanupConfig
}

// DataCleanupService handles data cleanup operations
type DataCleanupService struct {
	config     *DataCleanupConfig
	operations map[string]*CleanupOperation
}

// NewDataCleanupService creates a new cleanup service
func NewDataCleanupService(config *DataCleanupConfig) *DataCleanupService {
	if config == nil {
		config = DefaultDataCleanupConfig()
	}

	return &DataCleanupService{
		config:     config,
		operations: make(map[string]*CleanupOperation),
	}
}

// CleanupLogs cleans up old log entries
func (dcs *DataCleanupService) CleanupLogs(ctx context.Context, retentionDays int) (*CleanupOperation, error) {
	operation := &CleanupOperation{
		ID:            generateJobID(),
		OperationType: "logs",
		Status:        "running",
		StartTime:     time.Now(),
		Config:        dcs.config,
	}

	logging.Z().Info(fmt.Sprintf("Starting log cleanup (retention: %d days)", retentionDays))

	// Simulate log cleanup
	// In real implementation, this would query a logs database/table
	itemsDeleted := 0
	for i := 0; i < 5000; i += dcs.config.BatchSize {
		select {
		case <-ctx.Done():
			operation.Error = "cleanup cancelled"
			operation.Status = "failed"
			return operation, ctx.Err()
		default:
		}

		// Simulate database query and deletion
		batchSize := dcs.config.BatchSize
		if i+batchSize > 5000 {
			batchSize = 5000 - i
		}

		itemsDeleted += batchSize
		logging.Z().Info(fmt.Sprintf("Deleted %d log entries (batch)", batchSize))

		time.Sleep(100 * time.Millisecond) // Simulate DB operation
	}

	operation.ItemsDeleted = itemsDeleted
	operation.Status = "completed"
	now := time.Now()
	operation.EndTime = &now

	logging.Z().Info(fmt.Sprintf("Log cleanup completed: deleted %d entries", itemsDeleted))
	dcs.operations[operation.ID] = operation

	return operation, nil
}

// CleanupExpiredTokens cleans up expired authentication tokens
func (dcs *DataCleanupService) CleanupExpiredTokens(ctx context.Context) (*CleanupOperation, error) {
	operation := &CleanupOperation{
		ID:            generateJobID(),
		OperationType: "expired_tokens",
		Status:        "running",
		StartTime:     time.Now(),
		Config:        dcs.config,
	}

	logging.Z().Info(fmt.Sprintf("Starting expired tokens cleanup"))

	// Simulate token cleanup
	itemsDeleted := 0
	for i := 0; i < 2000; i += dcs.config.BatchSize {
		select {
		case <-ctx.Done():
			operation.Error = "cleanup cancelled"
			operation.Status = "failed"
			return operation, ctx.Err()
		default:
		}

		batchSize := dcs.config.BatchSize
		if i+batchSize > 2000 {
			batchSize = 2000 - i
		}

		itemsDeleted += batchSize
		logging.Z().Info(fmt.Sprintf("Deleted %d expired tokens (batch)", batchSize))

		time.Sleep(50 * time.Millisecond)
	}

	operation.ItemsDeleted = itemsDeleted
	operation.Status = "completed"
	now := time.Now()
	operation.EndTime = &now

	logging.Z().Info(fmt.Sprintf("Token cleanup completed: deleted %d tokens", itemsDeleted))
	dcs.operations[operation.ID] = operation

	return operation, nil
}

// CleanupOldSessions cleans up old user sessions
func (dcs *DataCleanupService) CleanupOldSessions(ctx context.Context, olderThanDays int) (*CleanupOperation, error) {
	operation := &CleanupOperation{
		ID:            generateJobID(),
		OperationType: "old_sessions",
		Status:        "running",
		StartTime:     time.Now(),
		Config:        dcs.config,
	}

	logging.Z().Info(fmt.Sprintf("Starting old sessions cleanup (older than %d days)", olderThanDays))

	// Simulate session cleanup
	itemsDeleted := 0
	for i := 0; i < 3000; i += dcs.config.BatchSize {
		select {
		case <-ctx.Done():
			operation.Error = "cleanup cancelled"
			operation.Status = "failed"
			return operation, ctx.Err()
		default:
		}

		batchSize := dcs.config.BatchSize
		if i+batchSize > 3000 {
			batchSize = 3000 - i
		}

		itemsDeleted += batchSize
		logging.Z().Info(fmt.Sprintf("Deleted %d old sessions (batch)", batchSize))

		time.Sleep(75 * time.Millisecond)
	}

	operation.ItemsDeleted = itemsDeleted
	operation.Status = "completed"
	now := time.Now()
	operation.EndTime = &now

	logging.Z().Info(fmt.Sprintf("Session cleanup completed: deleted %d sessions", itemsDeleted))
	dcs.operations[operation.ID] = operation

	return operation, nil
}

// CleanupTemporaryFiles cleans up temporary files
func (dcs *DataCleanupService) CleanupTemporaryFiles(ctx context.Context, olderThanHours int) (*CleanupOperation, error) {
	operation := &CleanupOperation{
		ID:            generateJobID(),
		OperationType: "temp_files",
		Status:        "running",
		StartTime:     time.Now(),
		Config:        dcs.config,
	}

	logging.Z().Info(fmt.Sprintf("Starting temporary files cleanup (older than %d hours)", olderThanHours))

	// Simulate file cleanup
	itemsDeleted := 0
	for i := 0; i < 1500; i += dcs.config.BatchSize {
		select {
		case <-ctx.Done():
			operation.Error = "cleanup cancelled"
			operation.Status = "failed"
			return operation, ctx.Err()
		default:
		}

		batchSize := dcs.config.BatchSize
		if i+batchSize > 1500 {
			batchSize = 1500 - i
		}

		itemsDeleted += batchSize
		logging.Z().Info(fmt.Sprintf("Deleted %d temporary files (batch)", batchSize))

		time.Sleep(60 * time.Millisecond)
	}

	operation.ItemsDeleted = itemsDeleted
	operation.Status = "completed"
	now := time.Now()
	operation.EndTime = &now

	logging.Z().Info(fmt.Sprintf("Temporary files cleanup completed: deleted %d files", itemsDeleted))
	dcs.operations[operation.ID] = operation

	return operation, nil
}

// GetOperation retrieves a cleanup operation status
func (dcs *DataCleanupService) GetOperation(operationID string) (*CleanupOperation, error) {
	if operation, exists := dcs.operations[operationID]; exists {
		return operation, nil
	}
	return nil, fmt.Errorf("operation not found: %s", operationID)
}

// DataCleanupJobHandler handles cleanup job processing
type DataCleanupJobHandler struct {
	service *DataCleanupService
}

// NewDataCleanupJobHandler creates a new cleanup job handler
func NewDataCleanupJobHandler(service *DataCleanupService) *DataCleanupJobHandler {
	return &DataCleanupJobHandler{
		service: service,
	}
}

// Handle processes a cleanup job
func (dcjh *DataCleanupJobHandler) Handle(ctx context.Context, job *Job) error {
	// Extract cleanup type
	cleanupType, ok := job.Data["type"].(string)
	if !ok || cleanupType == "" {
		return fmt.Errorf("missing or invalid 'type' field")
	}

	logging.Z().Info(fmt.Sprintf("Processing cleanup job %s: type=%s", job.ID, cleanupType))

	var operation *CleanupOperation
	var err error

	switch cleanupType {
	case "logs":
		retentionDays := 30
		if val, ok := job.Data["retention_days"].(float64); ok {
			retentionDays = int(val)
		}
		operation, err = dcjh.service.CleanupLogs(ctx, retentionDays)

	case "tokens":
		operation, err = dcjh.service.CleanupExpiredTokens(ctx)

	case "sessions":
		olderThanDays := 30
		if val, ok := job.Data["older_than_days"].(float64); ok {
			olderThanDays = int(val)
		}
		operation, err = dcjh.service.CleanupOldSessions(ctx, olderThanDays)

	case "temp_files":
		olderThanHours := 24
		if val, ok := job.Data["older_than_hours"].(float64); ok {
			olderThanHours = int(val)
		}
		operation, err = dcjh.service.CleanupTemporaryFiles(ctx, olderThanHours)

	default:
		return fmt.Errorf("unknown cleanup type: %s", cleanupType)
	}

	if err != nil {
		logging.Z().Info(fmt.Sprintf("Cleanup job %s failed: %v", job.ID, err))
		return err
	}

	// Store result
	job.Result = map[string]interface{}{
		"operation_id":   operation.ID,
		"operation_type": operation.OperationType,
		"items_deleted":  operation.ItemsDeleted,
		"duration":       operation.EndTime.Sub(operation.StartTime).String(),
		"completed_at":   operation.EndTime,
	}

	logging.Z().Info(fmt.Sprintf("Cleanup job %s completed: deleted %d items", job.ID, operation.ItemsDeleted))
	return nil
}

// ScheduledCleanupConfig defines scheduled cleanup patterns
type ScheduledCleanupConfig struct {
	CleanupLogs            bool // Run daily at 2 AM
	CleanupExpiredTokens   bool // Run every 6 hours
	CleanupOldSessions     bool // Run daily at 3 AM
	CleanupTemporaryFiles  bool // Run every 12 hours
	LogRetentionDays       int
	SessionRetentionDays   int
	TempFileRetentionHours int
}

// DefaultScheduledCleanupConfig returns default scheduled cleanup
func DefaultScheduledCleanupConfig() *ScheduledCleanupConfig {
	return &ScheduledCleanupConfig{
		CleanupLogs:            true,
		CleanupExpiredTokens:   true,
		CleanupOldSessions:     true,
		CleanupTemporaryFiles:  true,
		LogRetentionDays:       30,
		SessionRetentionDays:   30,
		TempFileRetentionHours: 24,
	}
}

// ScheduleCleanupJobs schedules all cleanup jobs with the manager
func ScheduleCleanupJobs(manager JobManager, config *ScheduledCleanupConfig) error {
	if config.CleanupLogs {
		if err := manager.ScheduleJob(JobTypeDataCleanup, "24h", map[string]interface{}{
			"type":           "logs",
			"retention_days": config.LogRetentionDays,
		}); err != nil {
			return err
		}
	}

	if config.CleanupExpiredTokens {
		if err := manager.ScheduleJob(JobTypeDataCleanup, "6h", map[string]interface{}{
			"type": "tokens",
		}); err != nil {
			return err
		}
	}

	if config.CleanupOldSessions {
		if err := manager.ScheduleJob(JobTypeDataCleanup, "24h", map[string]interface{}{
			"type":            "sessions",
			"older_than_days": config.SessionRetentionDays,
		}); err != nil {
			return err
		}
	}

	if config.CleanupTemporaryFiles {
		if err := manager.ScheduleJob(JobTypeDataCleanup, "12h", map[string]interface{}{
			"type":             "temp_files",
			"older_than_hours": config.TempFileRetentionHours,
		}); err != nil {
			return err
		}
	}

	return nil
}
