package securitymon

import (
	"context"
	"time"
)

// AuditLoggerAdapter wraps an audit.AuditLogger to satisfy AuditLogProvider.
// This avoids importing the audit package directly (circular dependency).
type AuditLoggerAdapter struct {
	queryFunc func(ctx context.Context, limit int) ([]ChainEntry, error)
}

// NewAuditLoggerAdapter creates an adapter from a query function.
// The queryFunc should return audit entries in insertion order, limited to `limit`.
func NewAuditLoggerAdapter(queryFunc func(ctx context.Context, limit int) ([]ChainEntry, error)) *AuditLoggerAdapter {
	return &AuditLoggerAdapter{queryFunc: queryFunc}
}

// ListRecent returns the N most recent audit log entries in insertion order.
func (a *AuditLoggerAdapter) ListRecent(limit int) ([]ChainEntry, error) {
	return a.queryFunc(context.Background(), limit)
}

// ChainEntryFromAuditLog creates a ChainEntry from audit log fields.
// This is a helper for callers building the adapter from audit.AuditLog.
func ChainEntryFromAuditLog(id, immutableHash string, timestamp time.Time) ChainEntry {
	return ChainEntry{
		ID:            id,
		ImmutableHash: immutableHash,
		Timestamp:     timestamp,
	}
}
