package contracts

import (
	"context"
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// ─── Factor Repository ────────────────────────────────────────────────────────

// FactorRepository is the persistence contract for Factor objects.
// Implementations: pgstore.FactorRepository (Postgres), raft.FactorRepository (in-mem).
type FactorRepository interface {
	// Get returns a factor by ID; returns ErrNotFound if absent.
	Get(ctx context.Context, id uuid.UUID) (*models.Factor, error)

	// GetByUser returns all factors for a user (including disabled/revoked).
	GetByUser(ctx context.Context, userID uuid.UUID) ([]*models.Factor, error)

	// GetActiveByUser returns only Active-phase factors for a user.
	GetActiveByUser(ctx context.Context, userID uuid.UUID) ([]*models.Factor, error)

	// Create persists a new factor (must be in Pending phase).
	Create(ctx context.Context, f *models.Factor) error

	// UpdateSpec patches the Spec fields; bumps ResourceVersion.
	UpdateSpec(ctx context.Context, f *models.Factor) error

	// UpdateStatus patches the Status fields; bumps ResourceVersion.
	// This is the ONLY write path for the reconciler.
	UpdateStatus(ctx context.Context, f *models.Factor) error

	// Delete soft-deletes a factor (sets DeletedAt).
	Delete(ctx context.Context, id uuid.UUID) error

	// ListPendingStale returns Pending factors older than staleBefore.
	// Used by the GC reconciler to clean up abandoned enrollments.
	ListPendingStale(ctx context.Context, staleBefore time.Time) ([]*models.Factor, error)
}

// ─── Challenge Repository ─────────────────────────────────────────────────────

// ChallengeRepository is the persistence contract for Challenge objects.
type ChallengeRepository interface {
	Get(ctx context.Context, id uuid.UUID) (*models.Challenge, error)
	GetOpenByUser(ctx context.Context, userID uuid.UUID) ([]*models.Challenge, error)
	Create(ctx context.Context, c *models.Challenge) error

	// UpdatePhase transitions the challenge phase and sets ResolvedAt if terminal.
	UpdatePhase(ctx context.Context, id uuid.UUID, phase models.ChallengePhase, resolvedAt *time.Time) error

	// IncrementAttempts bumps the attempt counter and returns the new count.
	IncrementAttempts(ctx context.Context, id uuid.UUID) (int, error)

	// ListExpired returns all Pending challenges whose ExpiresAt is before now.
	// Consumed by the challenge GC reconciler.
	ListExpired(ctx context.Context, now time.Time) ([]*models.Challenge, error)
}

// ─── Backup Code Repository ───────────────────────────────────────────────────

// BackupCodeRepository manages one-time recovery credentials.
type BackupCodeRepository interface {
	// BulkCreate atomically replaces all backup codes for a user.
	BulkCreate(ctx context.Context, userID uuid.UUID, codes []*models.BackupCode) error

	// MarkUsed atomically marks a code as used; returns ErrAlreadyUsed if consumed.
	MarkUsed(ctx context.Context, id uuid.UUID) error

	// ListByUser returns unused codes for a user.
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*models.BackupCode, error)

	// CountRemaining returns the number of unconsumed codes.
	CountRemaining(ctx context.Context, userID uuid.UUID) (int, error)
}

// ─── Trusted Device Repository ────────────────────────────────────────────────

// TrustedDeviceRepository manages "remember this device" tokens.
type TrustedDeviceRepository interface {
	Create(ctx context.Context, d *models.TrustedDevice) error
	GetByTokenHash(ctx context.Context, hash []byte) (*models.TrustedDevice, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*models.TrustedDevice, error)
	Revoke(ctx context.Context, id uuid.UUID) error
	RevokeAll(ctx context.Context, userID uuid.UUID) error
	ListExpired(ctx context.Context, now time.Time) ([]*models.TrustedDevice, error)
}
