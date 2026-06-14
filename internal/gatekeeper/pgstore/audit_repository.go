package pgstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/audit"
)

// AuditRepository implements audit.AuditBackend using PostgreSQL.
type AuditRepository struct {
	db *sql.DB
}

// NewAuditRepository creates a new PostgreSQL-backed audit repository.
func NewAuditRepository(db *sql.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

// LogEvent persists an audit event to PostgreSQL.
func (r *AuditRepository) LogEvent(ctx context.Context, event *audit.Event) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	metadataJSON, err := json.Marshal(event.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	query := `
		INSERT INTO twofactor_audit_log (id, event_type, user_id, factor_id, challenge_id, severity, message, source_ip, user_agent, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err = r.db.ExecContext(ctx, query,
		event.ID,
		string(event.EventType),
		event.UserID,
		event.FactorID,
		event.ChallengeID,
		event.Severity,
		event.Message,
		event.SourceIP,
		event.UserAgent,
		metadataJSON,
		event.Timestamp,
	)

	return err
}

// QueryEvents retrieves audit events matching the given filters.
func (r *AuditRepository) QueryEvents(ctx context.Context, filters map[string]interface{}) ([]*audit.Event, error) {
	query := `SELECT id, event_type, user_id, factor_id, challenge_id, severity, message, source_ip, user_agent, metadata, created_at
		FROM twofactor_audit_log WHERE 1=1`

	var args []interface{}
	argIdx := 1

	if userID, ok := filters["user_id"].(string); ok && userID != "" {
		query += ` AND user_id = $` + strconv.Itoa(argIdx)
		args = append(args, userID)
		argIdx++
	}

	if eventType, ok := filters["event_type"].(string); ok && eventType != "" {
		query += ` AND event_type = $` + strconv.Itoa(argIdx)
		args = append(args, eventType)
		argIdx++
	}

	if severity, ok := filters["severity"].(string); ok && severity != "" {
		query += ` AND severity = $` + strconv.Itoa(argIdx)
		args = append(args, severity)
		argIdx++
	}

	query += ` ORDER BY created_at DESC LIMIT 100`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*audit.Event
	for rows.Next() {
		event := &audit.Event{}
		var metadataJSON []byte

		err := rows.Scan(
			&event.ID,
			&event.EventType,
			&event.UserID,
			&event.FactorID,
			&event.ChallengeID,
			&event.Severity,
			&event.Message,
			&event.SourceIP,
			&event.UserAgent,
			&metadataJSON,
			&event.Timestamp,
		)
		if err != nil {
			return nil, err
		}

		if metadataJSON != nil {
			if err := json.Unmarshal(metadataJSON, &event.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal event metadata: %w", err)
			}
		}

		events = append(events, event)
	}

	return events, nil
}
