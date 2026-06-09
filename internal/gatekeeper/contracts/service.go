package contracts

import (
	"context"
	"time"

	"github.com/google/uuid"
	"example.com/axiomnizam/internal/gatekeeper/models"
)

// SetupResult contains the enrollment setup response.
type SetupResult struct {
	FactorID models.FactorID `json:"factor_id"`
	Secret   string          `json:"secret"`
}

// EnrollmentService defines the contract for MFA factor enrollment.
type EnrollmentService interface {
	// SetupFactor initiates factor setup
	SetupFactor(ctx context.Context, userID models.UserID, factorType models.FactorType, label string) (*SetupResult, error)

	// ActivateFactor completes enrollment by verifying OTP
	ActivateFactor(ctx context.Context, factorID models.FactorID, code string) ([]string, error)

	// DisableFactor removes a factor
	DisableFactor(ctx context.Context, factorID models.FactorID) error
}

// ChallengeService defines the contract for MFA authentication challenges.
type ChallengeService interface {
	// BeginChallenge creates a new MFA challenge
	BeginChallenge(ctx context.Context, userID models.UserID, factorID models.FactorID) (string, error)

	// VerifyChallenge verifies a user's response
	VerifyChallenge(ctx context.Context, challengeID string, code string) (bool, error)

	// ExpireChallenge marks a challenge as expired
	ExpireChallenge(ctx context.Context, challengeID string) error
}

// FactorService defines the contract for factor management.
type FactorService interface {
	// GetFactor retrieves a factor by ID
	GetFactor(ctx context.Context, factorID models.FactorID) (*models.Factor, error)

	// ListFactors returns all factors for a user
	ListFactors(ctx context.Context, userID models.UserID) ([]*models.Factor, error)

	// DeleteFactor removes a factor
	DeleteFactor(ctx context.Context, factorID models.FactorID) error

	// GetActiveFactorCount returns the number of active factors for a user
	GetActiveFactorCount(ctx context.Context, userID models.UserID) (int, error)
}

// PolicyService defines the contract for MFA policy evaluation.
type PolicyService interface {
	// EvaluatePolicy determines if MFA is required for a request
	EvaluatePolicy(ctx context.Context, userID models.UserID) (requiresMFA bool, factors []models.FactorType, err error)

	// GetPolicy retrieves a specific policy
	GetPolicy(ctx context.Context, policyID uuid.UUID) (*models.MFAPolicy, error)
}

// RiskService defines the contract for adaptive risk assessment.
type RiskService interface {
	// ScoreAuthentication calculates a risk score for an authentication attempt
	ScoreAuthentication(ctx context.Context, userID models.UserID, ipAddress string) (int, error)

	// IsHighRisk returns true if the risk score exceeds the threshold
	IsHighRisk(ctx context.Context, score int) bool
}

// TrustedDeviceService defines the contract for device trust management.
type TrustedDeviceService interface {
	// TrustDevice registers a device after MFA verification
	TrustDevice(ctx context.Context, userID models.UserID, fingerprint, userAgent, ipAddress string) (string, error)

	// VerifyDeviceToken checks if a device token is valid
	VerifyDeviceToken(ctx context.Context, userID models.UserID, token string) (bool, error)

	// ListTrustedDevices returns all active trusted devices for a user
	ListTrustedDevices(ctx context.Context, userID models.UserID) ([]*models.TrustedDevice, error)

	// RevokeTrustedDevice revokes a specific device
	RevokeTrustedDevice(ctx context.Context, deviceID uuid.UUID) error

	// RevokeAllDevices revokes all devices for a user
	RevokeAllDevices(ctx context.Context, userID models.UserID) error
}

// BackupCodeService defines the contract for backup code management.
type BackupCodeService interface {
	// ConsumeBackupCode marks a code as used
	ConsumeBackupCode(ctx context.Context, userID models.UserID, code string) (bool, error)

	// GetRemainingBackupCodes returns the count of unused codes
	GetRemainingBackupCodes(ctx context.Context, userID models.UserID) (int, error)

	// RegenerateBackupCodes generates new backup codes for a factor
	RegenerateBackupCodes(ctx context.Context, factorID models.FactorID) ([]string, error)
}

// WebAuthnService defines the contract for WebAuthn/FIDO2 operations.
type WebAuthnService interface {
	// BeginRegistration starts a registration ceremony (returns session ID + options JSON)
	BeginRegistration(ctx context.Context, userID uuid.UUID) (sessionID string, options map[string]interface{}, err error)
	// FinishRegistration verifies attestation and stores the credential
	FinishRegistration(ctx context.Context, userID uuid.UUID, sessionID string, response []byte) error
	// BeginAuthentication starts an authentication ceremony
	BeginAuthentication(ctx context.Context, userID uuid.UUID) (sessionID string, options map[string]interface{}, err error)
	// FinishAuthentication verifies assertion
	FinishAuthentication(ctx context.Context, userID uuid.UUID, sessionID string, response []byte) (bool, error)
	// ListCredentials returns registered WebAuthn credentials for a user
	ListCredentials(ctx context.Context, userID uuid.UUID) ([]map[string]interface{}, error)
	// DeleteCredential removes a WebAuthn credential
	DeleteCredential(ctx context.Context, userID uuid.UUID, credentialID []byte) error
}

// Provider aggregates all Gatekeeper services for dependency injection.
type Provider interface {
	EnrollmentService() EnrollmentService
	ChallengeService() ChallengeService
	FactorService() FactorService
	PolicyService() PolicyService
	RiskService() RiskService
	TrustedDeviceService() TrustedDeviceService
	BackupCodeService() BackupCodeService
}

// ActivateRequest carries the OTP the user submits to prove possession.
type ActivateRequest struct {
	UserID   uuid.UUID
	FactorID uuid.UUID
	OTP      string
}

// ─── Challenge ────────────────────────────────────────────────────────────────

// ChallengeRuntimeService orchestrates the runtime "prove it" flow.
type ChallengeRuntimeService interface {
	// Begin creates a new Challenge for the user's active factor.
	Begin(ctx context.Context, req BeginRequest) (*models.Challenge, error)

	// Verify submits an OTP against an open challenge.
	Verify(ctx context.Context, req VerifyRequest) (*VerifyResult, error)

	// Get returns a challenge by ID.
	Get(ctx context.Context, challengeID uuid.UUID) (*models.Challenge, error)
}

// BeginRequest carries metadata for creating a challenge.
type BeginRequest struct {
	UserID    uuid.UUID
	FactorID  uuid.UUID
	IPAddress string
	UserAgent string
	TTL       time.Duration // defaults to config.ChallengeTTL if zero
}

// VerifyRequest carries the user's OTP submission.
type VerifyRequest struct {
	ChallengeID uuid.UUID
	UserID      uuid.UUID
	OTP         string
	IPAddress   string
}

// VerifyResult indicates the outcome of a verification attempt.
type VerifyResult struct {
	Success   bool
	Challenge *models.Challenge
	// SessionToken is minted on success; empty on failure.
	SessionToken string
}

// ─── TOTP Provider ────────────────────────────────────────────────────────────

// TOTPProvider is the low-level TOTP primitive used by enrollment and challenge.
type TOTPProvider interface {
	GenerateSecret() (secret string, err error)
	ProvisioningURI(secret, issuer, account string) string
	Validate(secret, otp string, at time.Time) (bool, error)
	GenerateQRCode(uri string) (svgData string, err error)
}

// ─── Reconciler ───────────────────────────────────────────────────────────────

// Reconciler is the K8s-style controller loop interface.
// Each subsystem (factor, challenge, device) implements its own Reconciler.
type Reconciler interface {
	Reconcile(ctx context.Context, req ReconcileRequest) (ReconcileResult, error)
}

// ReconcileRequest carries the object identity to reconcile.
type ReconcileRequest struct {
	// ID is the primary key of the object being reconciled.
	ID uuid.UUID
	// Generation is the object's current generation counter.
	Generation int64
}

// ReconcileResult instructs the controller loop what to do next.
type ReconcileResult struct {
	// Requeue immediately.
	Requeue bool
	// RequeueAfter schedules a re-enqueue after a delay (e.g. TTL expiry).
	RequeueAfter time.Duration
}

// ─── Raft Store ───────────────────────────────────────────────────────────────

// RaftStore is the distributed state machine interface.
// Only the Raft leader may apply commands; followers serve reads from local state.
type RaftStore interface {
	Apply(cmd RaftCommand) error
	IsLeader() bool
	LeaderAddr() string
	// ReadFactor returns the in-memory factor state (may lag by one Raft tick).
	ReadFactor(id uuid.UUID) (*models.Factor, bool)
	// ReadChallenge returns the in-memory challenge state.
	ReadChallenge(id uuid.UUID) (*models.Challenge, bool)
}

// RaftCommand is the opaque payload serialised into the Raft log entry.
type RaftCommand struct {
	Type    models.RaftCommandType `json:"type"`
	Payload []byte                 `json:"payload"` // msgpack-encoded per-command struct
}
