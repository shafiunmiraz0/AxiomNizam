package events

import (
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// BackupCodeUsed is emitted when a backup code is consumed.
type BackupCodeUsed struct {
	CodeID   uuid.UUID     `json:"code_id"`
	UserID   models.UserID `json:"user_id"`
	UsedAt   time.Time     `json:"used_at"`
	IPAddress string       `json:"ip_address"`
}
