package raft

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	hraft "github.com/hashicorp/raft"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// ─── Command Payloads ─────────────────────────────────────────────────────────
// Each command is the unit of work committed through the Raft log.
// Nomad uses msgpack; we use JSON for debuggability (swap trivially).

// EnrollFactorCmd is applied when a user begins 2FA setup.
type EnrollFactorCmd struct {
	FactorID        uuid.UUID         `json:"factor_id"`
	UserID          uuid.UUID         `json:"user_id"`
	Type            models.FactorType `json:"type"`
	EncryptedSecret []byte            `json:"encrypted_secret"`
	Issuer          string            `json:"issuer"`
	EnrolledAt      time.Time         `json:"enrolled_at"`
	ResourceVersion int64             `json:"resource_version"`
}

// ActivateFactorCmd transitions a factor Pending → Active.
type ActivateFactorCmd struct {
	FactorID        uuid.UUID `json:"factor_id"`
	ActivatedAt     time.Time `json:"activated_at"`
	ResourceVersion int64     `json:"resource_version"`
}

// DisableFactorCmd transitions a factor Active → Disabled.
type DisableFactorCmd struct {
	FactorID        uuid.UUID `json:"factor_id"`
	DisabledAt      time.Time `json:"disabled_at"`
	ResourceVersion int64     `json:"resource_version"`
}

// RevokeFactorCmd hard-revokes a factor (admin / policy action).
type RevokeFactorCmd struct {
	FactorID        uuid.UUID `json:"factor_id"`
	Reason          string    `json:"reason"`
	RevokedAt       time.Time `json:"revoked_at"`
	ResourceVersion int64     `json:"resource_version"`
}

// BeginChallengeCmd creates a new challenge in Pending phase.
type BeginChallengeCmd struct {
	ChallengeID uuid.UUID `json:"challenge_id"`
	UserID      uuid.UUID `json:"user_id"`
	FactorID    uuid.UUID `json:"factor_id"`
	Nonce       string    `json:"nonce"`
	ExpiresAt   time.Time `json:"expires_at"`
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
	CreatedAt   time.Time `json:"created_at"`
}

// VerifyChallengeCmd transitions a challenge Pending → Verified.
type VerifyChallengeCmd struct {
	ChallengeID uuid.UUID `json:"challenge_id"`
	ResolvedAt  time.Time `json:"resolved_at"`
}

// ExpireChallengeCmd transitions Pending → Expired (TTL GC).
type ExpireChallengeCmd struct {
	ChallengeID uuid.UUID `json:"challenge_id"`
	ExpiredAt   time.Time `json:"expired_at"`
}

// FailChallengeCmd records a failed attempt and optionally locks the challenge.
type FailChallengeCmd struct {
	ChallengeID uuid.UUID `json:"challenge_id"`
	Attempts    int       `json:"attempts"`
	// Terminal=true when max attempts exceeded.
	Terminal bool      `json:"terminal"`
	FailedAt time.Time `json:"failed_at"`
}

// UseBackupCodeCmd marks a backup code as consumed.
type UseBackupCodeCmd struct {
	CodeID uuid.UUID `json:"code_id"`
	UserID uuid.UUID `json:"user_id"`
	UsedAt time.Time `json:"used_at"`
}

// TrustDeviceCmd registers a trusted device token.
type TrustDeviceCmd struct {
	DeviceID    uuid.UUID `json:"device_id"`
	UserID      uuid.UUID `json:"user_id"`
	TokenHash   []byte    `json:"token_hash"`
	Fingerprint string    `json:"fingerprint"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// RevokeDeviceCmd removes a trusted device.
type RevokeDeviceCmd struct {
	DeviceID  uuid.UUID `json:"device_id"`
	RevokedAt time.Time `json:"revoked_at"`
}

// ─── Codec ────────────────────────────────────────────────────────────────────

// encodeCommand marshals a typed payload into a RaftCommand.
func encodeCommand(t models.RaftCommandType, payload interface{}) ([]byte, error) {
	p, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("raft encode payload: %w", err)
	}
	cmd := struct {
		Type    models.RaftCommandType `json:"type"`
		Payload []byte                 `json:"payload"`
	}{Type: t, Payload: p}
	return json.Marshal(cmd)
}

// decodeCommand extracts the command type and raw payload bytes from a log entry.
func decodeCommand(data []byte) (models.RaftCommandType, []byte, error) {
	var cmd struct {
		Type    models.RaftCommandType `json:"type"`
		Payload []byte                 `json:"payload"`
	}
	if err := json.Unmarshal(data, &cmd); err != nil {
		return "", nil, fmt.Errorf("raft decode command: %w", err)
	}
	return cmd.Type, cmd.Payload, nil
}

// fsmSnapshot is the serializable snapshot of the FSM state.
type fsmSnapshot struct {
	Index           uint64                                      `json:"index"`
	Factors         map[uuid.UUID]*models.Factor                `json:"factors"`
	Challenges      map[uuid.UUID]*models.Challenge             `json:"challenges"`
	BackupCodesUsed map[uuid.UUID]time.Time                     `json:"backup_codes_used"`
	TrustedDevices  map[uuid.UUID]*models.TrustedDevice         `json:"trusted_devices"`
}

// Persist writes the snapshot to the sink.
func (s *fsmSnapshot) Persist(sink hraft.SnapshotSink) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("fsm snapshot persist: %w", err)
	}
	if _, err := sink.Write(data); err != nil {
		return fmt.Errorf("fsm snapshot write: %w", err)
	}
	return sink.Close()
}

// Release is a no-op for this snapshot.
func (s *fsmSnapshot) Release() {}
