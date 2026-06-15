package events

import (
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// FactorVerified is emitted when a user successfully verifies a factor.
type FactorVerified struct {
	FactorID  uuid.UUID         `json:"factor_id"`
	UserID    models.UserID     `json:"user_id"`
	VerifiedAt time.Time        `json:"verified_at"`
	IPAddress string            `json:"ip_address"`
}
