package models

import (
	"time"

	"github.com/google/uuid"
)

// TrustedDevice represents a "remember this device" token.
// Once a user verifies MFA on a device, they can mark it as trusted to skip
// MFA for subsequent authentications (configurable per policy).
type TrustedDevice struct {
	ID          uuid.UUID  `db:"id"           json:"id"`
	UserID      UserID     `db:"user_id"      json:"user_id"`
	TokenHash   []byte     `db:"token_hash"   json:"-"`           // bcrypt/argon2id
	Fingerprint string     `db:"fingerprint"  json:"fingerprint" classification:"PII"` // Browser fingerprint
	UserAgent   string     `db:"user_agent"   json:"user_agent" classification:"Sensitive"`
	IPAddress   string     `db:"ip_address"   json:"ip_address" classification:"PII"`
	ExpiresAt   time.Time  `db:"expires_at"   json:"expires_at"`
	RevokedAt   *time.Time `db:"revoked_at"   json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `db:"created_at"   json:"created_at"`
}

// IsExpired returns true if the device token has reached its TTL.
func (d *TrustedDevice) IsExpired(now time.Time) bool {
	return now.After(d.ExpiresAt)
}

// IsActive returns true if the device is not revoked and not expired.
func (d *TrustedDevice) IsActive(now time.Time) bool {
	return d.RevokedAt == nil && !d.IsExpired(now)
}

// TrustedDeviceCookie represents a persistent device cookie/token.
type TrustedDeviceCookie struct {
	Token    string    `json:"token"`
	Expires  time.Time `json:"expires"`
	Secure   bool      `json:"secure"`
	HttpOnly bool      `json:"http_only"`
	SameSite string    `json:"same_site"`
}
