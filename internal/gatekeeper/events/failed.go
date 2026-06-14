package events

import (
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// VerificationFailed is emitted when a verification attempt fails.
type VerificationFailed struct {
	FactorID    uuid.UUID     `json:"factor_id"`
	UserID      models.UserID `json:"user_id"`
	Reason      string        `json:"reason"`
	FailedAt    time.Time     `json:"failed_at"`
	IPAddress   string        `json:"ip_address"`
}
