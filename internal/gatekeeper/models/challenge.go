package models

import (
	"time"

	"github.com/google/uuid"
)

// ChallengeID is a typed UUID for an MFA challenge.
type ChallengeID = uuid.UUID

// Challenge represents a single runtime authentication event.
// It is created by challenge.Begin() and resolved by challenge.Verify().
//
// Like a K8s Job, a Challenge is ephemeral: it has a TTL and a terminal phase.
type Challenge struct {
	ID       ChallengeID `db:"id"        json:"id"`
	UserID   UserID      `db:"user_id"   json:"user_id"`
	FactorID FactorID    `db:"factor_id" json:"factor_id"`

	Phase ChallengePhase `db:"phase" json:"phase"`

	// Nonce is the OTP value for TOTP; empty for WebAuthn (assertion handles it).
	Nonce string `db:"nonce" json:"-" classification:"Confidential"`

	// Attempts counts failed verification tries; enforced by policy.
	Attempts int `db:"attempts" json:"attempts"`

	// ExpiresAt is the hard TTL; the reconciler requeues to expire stale challenges.
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`

	// ResolvedAt is non-nil once the challenge enters a terminal phase.
	ResolvedAt *time.Time `db:"resolved_at" json:"resolved_at,omitempty"`

	// IPAddress and UserAgent for risk scoring.
	IPAddress string `db:"ip_address" json:"ip_address" classification:"PII"`
	UserAgent string `db:"user_agent" json:"user_agent" classification:"Sensitive"`

	CreatedAt       time.Time `db:"created_at"        json:"created_at"`
	ResourceVersion int64     `db:"resource_version"  json:"resource_version"`
}

// IsTerminal returns true if no further state transitions are possible.
func (c *Challenge) IsTerminal() bool {
	switch c.Phase {
	case ChallengePhaseVerified, ChallengePhaseExpired, ChallengePhaseFailed, ChallengePhaseRejected:
		return true
	}
	return false
}

// IsExpired returns true if the wall-clock TTL has passed.
func (c *Challenge) IsExpired(now time.Time) bool {
	return now.After(c.ExpiresAt)
}
