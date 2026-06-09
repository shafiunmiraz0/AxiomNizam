// Package audit — Phase 7: PostgreSQL-backed AuditLogger implementation.
// Wraps the existing GORM AuditRepository to satisfy the AuditLogger interface,
// providing persistent audit storage with hash-chain integrity verification.

package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"example.com/axiomnizam/internal/models"
	"example.com/axiomnizam/internal/repositories"
)

// PostgresAuditLogger implements AuditLogger backed by PostgreSQL via GORM.
type PostgresAuditLogger struct {
	repo     repositories.AuditRepository
	prevHash string
}

// NewPostgresAuditLogger creates a new PostgreSQL-backed audit logger.
func NewPostgresAuditLogger(repo repositories.AuditRepository) *PostgresAuditLogger {
	return &PostgresAuditLogger{
		repo:     repo,
		prevHash: GenesisHash,
	}
}

// LogAction persists an audit event to PostgreSQL with hash-chain sealing.
func (l *PostgresAuditLogger) LogAction(ctx context.Context, log *AuditLog) error {
	if log.ID == "" {
		log.ID = fmt.Sprintf("audit-%d", time.Now().UnixNano())
	}
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}

	// Seal with hash chain for tamper evidence.
	if err := Seal(log, l.prevHash); err != nil {
		return fmt.Errorf("seal audit log: %w", err)
	}
	l.prevHash = log.ImmutableHash

	// Convert to GORM model.
	model := auditLogToModel(log)
	return l.repo.Create(model)
}

// QueryLogs retrieves audit logs from PostgreSQL matching the filter.
func (l *PostgresAuditLogger) QueryLogs(ctx context.Context, filter *AuditFilter) ([]*AuditLog, error) {
	if filter == nil {
		filter = &AuditFilter{Limit: 100}
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := filter.Offset

	var modelsList []*models.AuditLogModel
	var err error

	if filter.Action != "" {
		modelsList, err = l.repo.ListByAction(filter.TenantID, string(filter.Action), limit, offset)
	} else if filter.ResourceType != "" {
		modelsList, err = l.repo.ListByResource(filter.TenantID, filter.ResourceType, filter.ResourceID)
	} else {
		modelsList, err = l.repo.List(filter.TenantID, limit, offset)
	}

	if err != nil {
		return nil, err
	}

	result := make([]*AuditLog, 0, len(modelsList))
	for _, m := range modelsList {
		result = append(result, modelToAuditLog(m))
	}
	return result, nil
}

// GetReport generates an audit report from PostgreSQL data.
func (l *PostgresAuditLogger) GetReport(ctx context.Context, filter *AuditFilter) (*AuditReport, error) {
	logs, err := l.QueryLogs(ctx, filter)
	if err != nil {
		return nil, err
	}

	report := &AuditReport{
		TotalRecords: int64(len(logs)),
	}

	// Build breakdowns.
	byAction := make(map[string]int64)
	byResult := make(map[string]int64)
	byUser := make(map[string]int64)
	byResource := make(map[string]int64)
	for _, log := range logs {
		byAction[string(log.Action)]++
		byResult[string(log.Result)]++
		byUser[log.UserID]++
		byResource[log.ResourceType]++
	}
	report.ActionBreakdown = byAction
	report.ResultBreakdown = byResult
	report.UserBreakdown = byUser
	report.ResourceBreakdown = byResource

	// Calculate failure rate.
	if failures, ok := byResult[string(ResultFailure)]; ok && report.TotalRecords > 0 {
		report.FailureRate = float64(failures) / float64(report.TotalRecords) * 100
	}

	return report, nil
}

// DeleteOldLogs removes audit logs older than the specified number of days.
func (l *PostgresAuditLogger) DeleteOldLogs(ctx context.Context, olderThanDays int) error {
	return l.repo.DeleteOldLogs("", olderThanDays)
}

// VerifyIntegrity verifies the hash chain integrity of an audit log entry.
func (l *PostgresAuditLogger) VerifyIntegrity(ctx context.Context, logID string) (bool, error) {
	m, err := l.repo.GetByID(logID)
	if err != nil {
		return false, err
	}
	log := modelToAuditLog(m)
	if log.ImmutableHash == "" {
		return false, nil
	}
	expected, err := ComputeHash(log, "")
	if err != nil {
		return false, err
	}
	return expected == log.ImmutableHash, nil
}

// auditLogToModel converts an AuditLog to the GORM AuditLogModel.
func auditLogToModel(log *AuditLog) *models.AuditLogModel {
	var details []byte
	detailMap := map[string]interface{}{
		"username":      log.Username,
		"resource_name": log.ResourceName,
		"namespace":     log.Namespace,
		"source_ip":     log.SourceIP,
		"user_agent":    log.UserAgent,
		"method":        log.Method,
		"path":          log.Path,
		"request_id":    log.RequestID,
		"status_code":   log.StatusCode,
		"error_message": log.ErrorMessage,
		"duration":      log.Duration,
		"labels":        log.Labels,
		"reason":        log.Reason,
		"changes":       log.Changes,
	}
	details, _ = json.Marshal(detailMap)

	return &models.AuditLogModel{
		ID:           log.ID,
		TenantID:     log.TenantID,
		UserID:       log.UserID,
		ActionType:   string(log.Action),
		ResourceType: log.ResourceType,
		ResourceID:   log.ResourceID,
		Details:      details,
		Hash:         log.ImmutableHash,
		CreatedAt:    log.Timestamp,
	}
}

// modelToAuditLog converts a GORM AuditLogModel to an AuditLog.
func modelToAuditLog(m *models.AuditLogModel) *AuditLog {
	log := &AuditLog{
		ID:            m.ID,
		Timestamp:     m.CreatedAt,
		TenantID:      m.TenantID,
		UserID:        m.UserID,
		Action:        AuditAction(m.ActionType),
		ResourceType:  m.ResourceType,
		ResourceID:    m.ResourceID,
		ImmutableHash: m.Hash,
	}

	if len(m.Details) > 0 {
		var detailMap map[string]interface{}
		if json.Unmarshal(m.Details, &detailMap) == nil {
			if v, ok := detailMap["username"].(string); ok {
				log.Username = v
			}
			if v, ok := detailMap["source_ip"].(string); ok {
				log.SourceIP = v
			}
			if v, ok := detailMap["user_agent"].(string); ok {
				log.UserAgent = v
			}
			if v, ok := detailMap["method"].(string); ok {
				log.Method = v
			}
			if v, ok := detailMap["path"].(string); ok {
				log.Path = v
			}
			if v, ok := detailMap["request_id"].(string); ok {
				log.RequestID = v
			}
			if v, ok := detailMap["error_message"].(string); ok {
				log.ErrorMessage = v
			}
			if v, ok := detailMap["reason"].(string); ok {
				log.Reason = v
			}
			if v, ok := detailMap["resource_name"].(string); ok {
				log.ResourceName = v
			}
			if v, ok := detailMap["namespace"].(string); ok {
				log.Namespace = v
			}
			if v, ok := detailMap["duration"].(float64); ok {
				log.Duration = int64(v)
			}
			if v, ok := detailMap["status_code"].(float64); ok {
				log.StatusCode = int(v)
			}
		}
	}

	return log
}
