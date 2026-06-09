package models

import (
	"time"

	"github.com/google/uuid"
)

// FactorID is a typed UUID for a 2FA factor.
type FactorID = uuid.UUID

// UserID is a typed UUID for an IAM user.
type UserID = uuid.UUID

// Factor is the central domain object, modeled after a K8s Custom Resource.
//
// Spec  → the *desired* state (what the operator declared).
// Status → the *observed* state (what the reconciler last wrote).
//
// The reconciler continuously drives Spec → Status.
type Factor struct {
	// Identity
	ID     FactorID `db:"id"     json:"id"`
	UserID UserID   `db:"user_id" json:"user_id"`

	// Desired state — written by users / enrollment service.
	Spec FactorSpec `db:"spec" json:"spec"`

	// Observed state — written exclusively by the reconciler.
	Status FactorStatus `db:"status" json:"status"`

	// Immutable bookkeeping.
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt time.Time  `db:"updated_at" json:"updated_at"`
	DeletedAt *time.Time `db:"deleted_at" json:"deleted_at,omitempty"`

	// ResourceVersion is bumped on every write; used for optimistic concurrency
	// (mirrors K8s resourceVersion / etcd modRevision).
	ResourceVersion int64 `db:"resource_version" json:"resource_version"`
}

// FactorSpec is the desired configuration for a Factor.
type FactorSpec struct {
	Type        FactorType `db:"type"         json:"type"`
	Label       string     `db:"label"        json:"label,omitempty"`        // User-friendly name
	PhoneNumber string     `db:"phone_number" json:"phone_number,omitempty" classification:"PII"` // SMS only
	Email       string     `db:"email"        json:"email,omitempty" classification:"PII"`        // email only
	// Secret is AES-GCM encrypted at rest; persisted in JSONB but excluded from API responses.
	EncryptedSecret []byte `db:"encrypted_secret" json:"encrypted_secret,omitempty" classification:"Confidential"`
	// Issuer shown inside authenticator apps (e.g. "Acme Corp").
	Issuer string `db:"issuer" json:"issuer"`
}

// FactorStatus is the reconciler-owned observed state.
type FactorStatus struct {
	Phase FactorPhase `db:"phase" json:"phase"`

	// Conditions give fine-grained readiness information (K8s pattern).
	Conditions []Condition `db:"conditions" json:"conditions"`

	// LastVerifiedAt is the timestamp of the most recent successful verification.
	LastVerifiedAt *time.Time `db:"last_verified_at" json:"last_verified_at,omitempty"`

	// ActivatedAt is when the factor transitioned Pending → Active.
	ActivatedAt *time.Time `db:"activated_at" json:"activated_at,omitempty"`

	// DisabledAt / RevokedAt for auditability.
	DisabledAt *time.Time `db:"disabled_at" json:"disabled_at,omitempty"`
	RevokedAt  *time.Time `db:"revoked_at"  json:"revoked_at,omitempty"`

	// ObservedGeneration mirrors K8s: the generation this status was computed from.
	ObservedGeneration int64 `db:"observed_generation" json:"observed_generation"`
}

// Condition is a K8s-style status condition.
type Condition struct {
	Type               ConditionType   `json:"type"`
	Status             ConditionStatus `json:"status"`
	Reason             string          `json:"reason"`
	Message            string          `json:"message"`
	LastTransitionTime time.Time       `json:"last_transition_time"`
}

// IsActive returns true when the factor is in the Active phase.
func (f *Factor) IsActive() bool {
	return f.Status.Phase == FactorPhaseActive
}

// SetCondition upserts a condition by type (last-write wins).
func (s *FactorStatus) SetCondition(c Condition) {
	for i, existing := range s.Conditions {
		if existing.Type == c.Type {
			if existing.Status != c.Status {
				c.LastTransitionTime = time.Now().UTC()
			} else {
				c.LastTransitionTime = existing.LastTransitionTime
			}
			s.Conditions[i] = c
			return
		}
	}
	c.LastTransitionTime = time.Now().UTC()
	s.Conditions = append(s.Conditions, c)
}
