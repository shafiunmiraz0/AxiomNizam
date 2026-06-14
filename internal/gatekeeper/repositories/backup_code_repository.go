package repositories

import (
	"context"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// BackupCodeRepository defines operations for one-time backup codes.
type BackupCodeRepository interface {
	// Create inserts new backup codes (bulk operation)
	Create(ctx context.Context, codes []*models.BackupCode) error

	// Get retrieves a single backup code by ID
	Get(ctx context.Context, id uuid.UUID) (*models.BackupCode, error)

	// GetByCodeHash retrieves an unused backup code by its hash
	GetByCodeHash(ctx context.Context, codeHash []byte) (*models.BackupCode, error)

	// GetByUserID retrieves all backup codes for a user
	GetByUserID(ctx context.Context, userID models.UserID) ([]*models.BackupCode, error)

	// GetByFactorID retrieves all backup codes for a factor
	GetByFactorID(ctx context.Context, factorID models.FactorID) ([]*models.BackupCode, error)

	// MarkUsed marks a code as consumed
	MarkUsed(ctx context.Context, id uuid.UUID) error

	// CountUnused returns the number of unused backup codes for a user
	CountUnused(ctx context.Context, userID models.UserID) (int, error)

	// DeleteByFactorID removes all backup codes for a factor
	DeleteByFactorID(ctx context.Context, factorID models.FactorID) error
}
