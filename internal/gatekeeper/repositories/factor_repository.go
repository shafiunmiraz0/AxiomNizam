package repositories

import (
	"context"
	"errors"

	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// ErrFactorNotFound is returned when a factor is not found.
var ErrFactorNotFound = errors.New("factor not found")

// FactorRepository defines CRUD operations for MFA factors.
type FactorRepository interface {
	// Create inserts a new factor and returns the Factor with ID/timestamps populated
	Create(ctx context.Context, factor *models.Factor) (*models.Factor, error)

	// Get retrieves a factor by ID
	Get(ctx context.Context, id models.FactorID) (*models.Factor, error)

	// GetByUserID retrieves all active factors for a user
	GetByUserID(ctx context.Context, userID models.UserID) ([]*models.Factor, error)

	// List retrieves all factors with optional filtering
	List(ctx context.Context, filters map[string]interface{}) ([]*models.Factor, error)

	// Update writes changes to an existing factor (optimistic concurrency via ResourceVersion)
	Update(ctx context.Context, factor *models.Factor) (*models.Factor, error)

	// Delete marks a factor as soft-deleted
	Delete(ctx context.Context, id models.FactorID) error

	// Exists checks if a factor exists and is active
	Exists(ctx context.Context, id models.FactorID) (bool, error)

	// Count returns the number of active factors for a user
	Count(ctx context.Context, userID models.UserID) (int, error)

	// ListExpiredPending returns factors in Pending phase that have exceeded grace period
	ListExpiredPending(ctx context.Context, gracePeriodDays int) ([]*models.Factor, error)
}
