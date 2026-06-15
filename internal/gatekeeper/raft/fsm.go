package raft

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	hraft "github.com/hashicorp/raft"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// FSM implements hashicorp/raft.FSM.
//
// Design mirrors Nomad's state store:
//   - All state is held in-memory (memdb / plain maps here for clarity).
//   - Every mutation arrives as an Apply() call from the Raft leader.
//   - Followers apply the same log deterministically → identical state.
//   - Snapshot / Restore provide point-in-time state transfer for new peers.
//
// The FSM is NOT a cache — it is the authoritative distributed state.
// Postgres is the durable audit trail and serves cold reads.
type FSM struct {
	mu sync.RWMutex

	// factors holds the live factor state keyed by FactorID.
	factors map[uuid.UUID]*models.Factor

	// challenges holds open challenges keyed by ChallengeID.
	challenges map[uuid.UUID]*models.Challenge

	// backupCodesUsed tracks consumed backup code IDs.
	backupCodesUsed map[uuid.UUID]time.Time

	// trustedDevices holds active device tokens keyed by DeviceID.
	trustedDevices map[uuid.UUID]*models.TrustedDevice

	// index is the last applied Raft log index (for snapshot consistency).
	index uint64
}

// NewFSM constructs an empty FSM.
func NewFSM() *FSM {
	return &FSM{
		factors:         make(map[uuid.UUID]*models.Factor),
		challenges:      make(map[uuid.UUID]*models.Challenge),
		backupCodesUsed: make(map[uuid.UUID]time.Time),
		trustedDevices:  make(map[uuid.UUID]*models.TrustedDevice),
	}
}

// ─── hraft.FSM interface ──────────────────────────────────────────────────────

// Apply is called by the Raft library when a log entry is committed.
// It MUST be deterministic and side-effect-free (no I/O, no randomness).
func (f *FSM) Apply(log *hraft.Log) interface{} {
	cmdType, payload, err := decodeCommand(log.Data)
	if err != nil {
		return fmt.Errorf("fsm apply decode: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.index = log.Index

	switch cmdType {
	case models.RaftCmdEnrollFactor:
		return f.applyEnrollFactor(payload)
	case models.RaftCmdActivateFactor:
		return f.applyActivateFactor(payload)
	case models.RaftCmdDisableFactor:
		return f.applyDisableFactor(payload)
	case models.RaftCmdCreateChallenge:
		return f.applyBeginChallenge(payload)
	case models.RaftCmdVerifyChallenge:
		return f.applyVerifyChallenge(payload)
	case models.RaftCmdExpireChallenge:
		return f.applyExpireChallenge(payload)
	case models.RaftCmdGenerateBackupCodes:
		return f.applyUseBackupCode(payload)
	case models.RaftCmdConsumeBackupCode:
		return f.applyUseBackupCode(payload)
	case models.RaftCmdTrustDevice:
		return f.applyTrustDevice(payload)
	case models.RaftCmdRevokeDevice:
		return f.applyRevokeDevice(payload)
	default:
		return fmt.Errorf("fsm apply: unknown command type %q", cmdType)
	}
}

// Snapshot returns a point-in-time snapshot of the FSM state.
func (f *FSM) Snapshot() (hraft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	snap := &fsmSnapshot{
		Index:           f.index,
		Factors:         cloneFactorMap(f.factors),
		Challenges:      cloneChallengeMap(f.challenges),
		BackupCodesUsed: cloneBackupMap(f.backupCodesUsed),
		TrustedDevices:  cloneDeviceMap(f.trustedDevices),
	}
	return snap, nil
}

// Restore replaces FSM state from a snapshot (used during peer bootstrap).
func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()

	var snap fsmSnapshot
	if err := json.NewDecoder(rc).Decode(&snap); err != nil {
		return fmt.Errorf("fsm restore decode: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.index = snap.Index
	f.factors = snap.Factors
	f.challenges = snap.Challenges
	f.backupCodesUsed = snap.BackupCodesUsed
	f.trustedDevices = snap.TrustedDevices
	return nil
}

// ─── Apply helpers (all called under f.mu.Lock) ───────────────────────────────

func (f *FSM) applyEnrollFactor(payload []byte) error {
	var cmd EnrollFactorCmd
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	now := cmd.EnrolledAt
	f.factors[cmd.FactorID] = &models.Factor{
		ID:     cmd.FactorID,
		UserID: cmd.UserID,
		Spec: models.FactorSpec{
			Type:            cmd.Type,
			Issuer:          cmd.Issuer,
			EncryptedSecret: cmd.EncryptedSecret,
		},
		Status: models.FactorStatus{
			Phase: models.FactorPhasePending,
			Conditions: []models.Condition{
				{
					Type:               models.ConditionTypeReady,
					Status:             models.ConditionStatusFalse,
					Reason:             "PendingActivation",
					Message:            "Factor enrolled; awaiting OTP verification.",
					LastTransitionTime: now,
				},
			},
		},
		CreatedAt:       now,
		UpdatedAt:       now,
		ResourceVersion: cmd.ResourceVersion,
	}
	return nil
}

func (f *FSM) applyActivateFactor(payload []byte) error {
	var cmd ActivateFactorCmd
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	factor, ok := f.factors[cmd.FactorID]
	if !ok {
		return fmt.Errorf("activate: factor %s not found", cmd.FactorID)
	}
	factor.Status.Phase = models.FactorPhaseActive
	factor.Status.ActivatedAt = &cmd.ActivatedAt
	factor.Status.SetCondition(models.Condition{
		Type:    models.ConditionTypeReady,
		Status:  models.ConditionStatusTrue,
		Reason:  "Activated",
		Message: "Factor verified and active.",
	})
	factor.UpdatedAt = cmd.ActivatedAt
	factor.ResourceVersion = cmd.ResourceVersion
	return nil
}

func (f *FSM) applyDisableFactor(payload []byte) error {
	var cmd DisableFactorCmd
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	factor, ok := f.factors[cmd.FactorID]
	if !ok {
		return fmt.Errorf("disable: factor %s not found", cmd.FactorID)
	}
	factor.Status.Phase = models.FactorPhaseDisabled
	factor.Status.DisabledAt = &cmd.DisabledAt
	factor.Status.SetCondition(models.Condition{
		Type:    models.ConditionTypeReady,
		Status:  models.ConditionStatusFalse,
		Reason:  "Disabled",
		Message: "Factor disabled by user.",
	})
	factor.UpdatedAt = cmd.DisabledAt
	factor.ResourceVersion = cmd.ResourceVersion
	return nil
}

func (f *FSM) applyRevokeFactor(payload []byte) error {
	var cmd RevokeFactorCmd
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	factor, ok := f.factors[cmd.FactorID]
	if !ok {
		return fmt.Errorf("revoke: factor %s not found", cmd.FactorID)
	}
	factor.Status.Phase = models.FactorPhaseRevoked
	factor.Status.RevokedAt = &cmd.RevokedAt
	factor.Status.SetCondition(models.Condition{
		Type:    models.ConditionTypeReady,
		Status:  models.ConditionStatusFalse,
		Reason:  "Revoked",
		Message: cmd.Reason,
	})
	factor.UpdatedAt = cmd.RevokedAt
	factor.ResourceVersion = cmd.ResourceVersion
	return nil
}

func (f *FSM) applyBeginChallenge(payload []byte) error {
	var cmd BeginChallengeCmd
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	f.challenges[cmd.ChallengeID] = &models.Challenge{
		ID:        cmd.ChallengeID,
		UserID:    cmd.UserID,
		FactorID:  cmd.FactorID,
		Phase:     models.ChallengePhaseWaiting,
		Nonce:     cmd.Nonce,
		ExpiresAt: cmd.ExpiresAt,
		IPAddress: cmd.IPAddress,
		UserAgent: cmd.UserAgent,
		CreatedAt: cmd.CreatedAt,
	}
	return nil
}

func (f *FSM) applyVerifyChallenge(payload []byte) error {
	var cmd VerifyChallengeCmd
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	c, ok := f.challenges[cmd.ChallengeID]
	if !ok {
		return fmt.Errorf("verify: challenge %s not found", cmd.ChallengeID)
	}
	c.Phase = models.ChallengePhaseVerified
	c.ResolvedAt = &cmd.ResolvedAt
	return nil
}

func (f *FSM) applyExpireChallenge(payload []byte) error {
	var cmd ExpireChallengeCmd
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	c, ok := f.challenges[cmd.ChallengeID]
	if !ok {
		return nil // already removed; idempotent
	}
	c.Phase = models.ChallengePhaseExpired
	c.ResolvedAt = &cmd.ExpiredAt
	return nil
}

func (f *FSM) applyFailChallenge(payload []byte) error {
	var cmd FailChallengeCmd
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	c, ok := f.challenges[cmd.ChallengeID]
	if !ok {
		return fmt.Errorf("fail: challenge %s not found", cmd.ChallengeID)
	}
	c.Attempts = cmd.Attempts
	if cmd.Terminal {
		c.Phase = models.ChallengePhaseFailed
		c.ResolvedAt = &cmd.FailedAt
	}
	return nil
}

func (f *FSM) applyUseBackupCode(payload []byte) error {
	var cmd UseBackupCodeCmd
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	if _, used := f.backupCodesUsed[cmd.CodeID]; used {
		return fmt.Errorf("backup code %s already used", cmd.CodeID)
	}
	f.backupCodesUsed[cmd.CodeID] = cmd.UsedAt
	return nil
}

func (f *FSM) applyTrustDevice(payload []byte) error {
	var cmd TrustDeviceCmd
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	f.trustedDevices[cmd.DeviceID] = &models.TrustedDevice{
		ID:          cmd.DeviceID,
		UserID:      cmd.UserID,
		TokenHash:   cmd.TokenHash,
		Fingerprint: cmd.Fingerprint,
		ExpiresAt:   cmd.ExpiresAt,
		CreatedAt:   cmd.CreatedAt,
	}
	return nil
}

func (f *FSM) applyRevokeDevice(payload []byte) error {
	var cmd RevokeDeviceCmd
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	d, ok := f.trustedDevices[cmd.DeviceID]
	if !ok {
		return nil // idempotent
	}
	d.RevokedAt = &cmd.RevokedAt
	return nil
}

// ─── Read helpers (safe for concurrent use) ───────────────────────────────────

// ReadFactor returns a shallow copy of the factor state.
func (f *FSM) ReadFactor(id uuid.UUID) (*models.Factor, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	factor, ok := f.factors[id]
	if !ok {
		return nil, false
	}
	cp := *factor
	return &cp, true
}

// ReadChallenge returns a shallow copy of the challenge state.
func (f *FSM) ReadChallenge(id uuid.UUID) (*models.Challenge, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	c, ok := f.challenges[id]
	if !ok {
		return nil, false
	}
	cp := *c
	return &cp, true
}

// ─── Clone helpers for snapshot ───────────────────────────────────────────────

func cloneFactorMap(m map[uuid.UUID]*models.Factor) map[uuid.UUID]*models.Factor {
	out := make(map[uuid.UUID]*models.Factor, len(m))
	for k, v := range m {
		cp := *v
		out[k] = &cp
	}
	return out
}

func cloneChallengeMap(m map[uuid.UUID]*models.Challenge) map[uuid.UUID]*models.Challenge {
	out := make(map[uuid.UUID]*models.Challenge, len(m))
	for k, v := range m {
		cp := *v
		out[k] = &cp
	}
	return out
}

func cloneBackupMap(m map[uuid.UUID]time.Time) map[uuid.UUID]time.Time {
	out := make(map[uuid.UUID]time.Time, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneDeviceMap(m map[uuid.UUID]*models.TrustedDevice) map[uuid.UUID]*models.TrustedDevice {
	out := make(map[uuid.UUID]*models.TrustedDevice, len(m))
	for k, v := range m {
		cp := *v
		out[k] = &cp
	}
	return out
}
