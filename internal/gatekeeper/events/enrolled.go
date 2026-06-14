package events

import (
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// FactorEnrolled is emitted when a new factor is enrolled.
type FactorEnrolled struct {
	FactorID   uuid.UUID         `json:"factor_id"`
	UserID     models.UserID     `json:"user_id"`
	FactorType models.FactorType `json:"factor_type"`
	EnrolledAt time.Time         `json:"enrolled_at"`
}
