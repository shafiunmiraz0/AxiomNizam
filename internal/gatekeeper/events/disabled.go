package events

import (
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// FactorDisabled is emitted when a factor is disabled.
type FactorDisabled struct {
	FactorID   uuid.UUID     `json:"factor_id"`
	UserID     models.UserID `json:"user_id"`
	DisabledAt time.Time     `json:"disabled_at"`
	Reason     string        `json:"reason"`
}
