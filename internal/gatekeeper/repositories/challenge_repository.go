package repositories

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// ChallengeRepository defines CRUD operations for MFA challenges.
type ChallengeRepository interface {
	// Create inserts a new challenge
	Create(ctx context.Context, challenge *models.Challenge) (*models.Challenge, error)

	// Get retrieves a challenge by ID
	Get(ctx context.Context, id models.ChallengeID) (*models.Challenge, error)

	// GetByUserID retrieves active (non-terminal) challenges for a user
	GetByUserID(ctx context.Context, userID models.UserID) ([]*models.Challenge, error)

	// Update writes changes to a challenge
	Update(ctx context.Context, challenge *models.Challenge) (*models.Challenge, error)

	// Delete removes a challenge record
	Delete(ctx context.Context, id models.ChallengeID) error

	// ListExpired returns challenges past their TTL
	ListExpired(ctx context.Context) ([]*models.Challenge, error)

	// CountAttempts returns the number of failed attempts for a challenge
	CountAttempts(ctx context.Context, challengeID models.ChallengeID) (int, error)
}
