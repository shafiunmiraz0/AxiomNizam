package repositories

import (
	"context"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// TrustedDeviceRepository defines operations for trusted device tokens.
type TrustedDeviceRepository interface {
	// Create registers a new trusted device
	Create(ctx context.Context, device *models.TrustedDevice) (*models.TrustedDevice, error)

	// Get retrieves a trusted device by ID
	Get(ctx context.Context, id uuid.UUID) (*models.TrustedDevice, error)

	// GetByUserID retrieves active trusted devices for a user
	GetByUserID(ctx context.Context, userID models.UserID) ([]*models.TrustedDevice, error)

	// GetByFingerprint retrieves a trusted device by fingerprint
	GetByFingerprint(ctx context.Context, userID models.UserID, fingerprint string) (*models.TrustedDevice, error)

	// Update updates device metadata
	Update(ctx context.Context, device *models.TrustedDevice) (*models.TrustedDevice, error)

	// Revoke marks a device as revoked
	Revoke(ctx context.Context, id uuid.UUID) error

	// RevokeByUserID revokes all devices for a user
	RevokeByUserID(ctx context.Context, userID models.UserID) error

	// DeleteExpired removes all expired device records (cleanup)
	DeleteExpired(ctx context.Context) error

	// Count returns the number of active devices for a user
	Count(ctx context.Context, userID models.UserID) (int, error)
}
