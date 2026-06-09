// Package securitymon provides security observability for AxiomNizam (Phase 13).
//
// Components:
//   - SecurityMetrics: Prometheus counters/gauges for auth failures, risk events, MFA
//   - AnomalyDetector: per-IP/per-user request pattern analysis
//   - ThreatResponder: automated session revocation on threat detection
//   - AuditVerifier: scheduled audit chain integrity verification
//   - SIEMExporter: exports audit events to external SIEM (webhook/file/stdout)
//   - DashboardHandler: security dashboard API endpoint for frontend
package securitymon

import (
	"sync"
	"time"
)

// SecurityMetrics tracks security-relevant counters and gauges in-memory.
// These are also exported as Prometheus metrics via RegisterPrometheus().
type SecurityMetrics struct {
	mu sync.RWMutex

	// Auth events
	AuthFailures     int64 `json:"auth_failures"`
	AuthSuccesses    int64 `json:"auth_successes"`
	TokenRevocations int64 `json:"token_revocations"`

	// Risk events
	HighRiskRequests  int64 `json:"high_risk_requests"`
	RiskDeltaTriggers int64 `json:"risk_delta_triggers"`
	SessionsRevoked   int64 `json:"sessions_revoked"`

	// MFA events
	MFAChallenges   int64 `json:"mfa_challenges"`
	MFASuccesses    int64 `json:"mfa_successes"`
	MFAFailures     int64 `json:"mfa_failures"`
	WebAuthnUsed    int64 `json:"webauthn_used"`
	StepUpRequired  int64 `json:"step_up_required"`

	// Policy events
	PolicyBlocks    int64 `json:"policy_blocks"`
	RBACDenials     int64 `json:"rbac_denials"`

	// Request tracking
	TotalRequests   int64 `json:"total_requests"`
	BlockedRequests int64 `json:"blocked_requests"`

	// Anomaly tracking
	UniqueIPs       int   `json:"unique_ips"`
	UniqueUsers     int   `json:"unique_users"`

	// Audit chain
	ChainVerified   bool      `json:"chain_verified"`
	ChainBrokenAt   *time.Time `json:"chain_broken_at,omitempty"`
	ChainLastCheck  time.Time `json:"chain_last_check"`
	ChainEntries    int       `json:"chain_entries"`

	// SIEM
	SIEMEventsExported int64 `json:"siem_events_exported"`
	SIEMExportErrors   int64 `json:"siem_export_errors"`

	// Timestamps
	StartedAt time.Time `json:"started_at"`
	LastReset time.Time `json:"last_reset"`
}

// NewSecurityMetrics creates a new SecurityMetrics instance.
func NewSecurityMetrics() *SecurityMetrics {
	now := time.Now().UTC()
	return &SecurityMetrics{
		StartedAt: now,
		LastReset: now,
	}
}

// Snapshot returns a copy of the current metrics.
func (m *SecurityMetrics) Snapshot() SecurityMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snap := *m
	return snap
}

// RecordAuthFailure increments the auth failure counter.
func (m *SecurityMetrics) RecordAuthFailure() {
	m.mu.Lock()
	m.AuthFailures++
	m.mu.Unlock()
}

// RecordAuthSuccess increments the auth success counter.
func (m *SecurityMetrics) RecordAuthSuccess() {
	m.mu.Lock()
	m.AuthSuccesses++
	m.mu.Unlock()
}

// RecordHighRisk increments the high-risk request counter.
func (m *SecurityMetrics) RecordHighRisk() {
	m.mu.Lock()
	m.HighRiskRequests++
	m.mu.Unlock()
}

// RecordRiskDeltaTrigger increments the risk delta trigger counter.
func (m *SecurityMetrics) RecordRiskDeltaTrigger() {
	m.mu.Lock()
	m.RiskDeltaTriggers++
	m.mu.Unlock()
}

// RecordSessionRevoked increments the session revoked counter.
func (m *SecurityMetrics) RecordSessionRevoked() {
	m.mu.Lock()
	m.SessionsRevoked++
	m.mu.Unlock()
}

// RecordMFAChallenge increments the MFA challenge counter.
func (m *SecurityMetrics) RecordMFAChallenge(factorType string) {
	m.mu.Lock()
	m.MFAChallenges++
	if factorType == "webauthn" {
		m.WebAuthnUsed++
	}
	m.mu.Unlock()
}

// RecordMFASuccess increments the MFA success counter.
func (m *SecurityMetrics) RecordMFASuccess() {
	m.mu.Lock()
	m.MFASuccesses++
	m.mu.Unlock()
}

// RecordMFAFailure increments the MFA failure counter.
func (m *SecurityMetrics) RecordMFAFailure() {
	m.mu.Lock()
	m.MFAFailures++
	m.mu.Unlock()
}

// RecordStepUp increments the step-up MFA counter.
func (m *SecurityMetrics) RecordStepUp() {
	m.mu.Lock()
	m.StepUpRequired++
	m.mu.Unlock()
}

// RecordPolicyBlock increments the policy block counter.
func (m *SecurityMetrics) RecordPolicyBlock() {
	m.mu.Lock()
	m.PolicyBlocks++
	m.mu.Unlock()
}

// RecordRBACDenial increments the RBAC denial counter.
func (m *SecurityMetrics) RecordRBACDenial() {
	m.mu.Lock()
	m.RBACDenials++
	m.mu.Unlock()
}

// RecordTokenRevocation increments the token revocation counter.
func (m *SecurityMetrics) RecordTokenRevocation() {
	m.mu.Lock()
	m.TokenRevocations++
	m.mu.Unlock()
}

// RecordBlockedRequest increments the blocked request counter.
func (m *SecurityMetrics) RecordBlockedRequest() {
	m.mu.Lock()
	m.BlockedRequests++
	m.mu.Unlock()
}

// RecordTotalRequest increments the total request counter.
func (m *SecurityMetrics) RecordTotalRequest() {
	m.mu.Lock()
	m.TotalRequests++
	m.mu.Unlock()
}

// SetChainVerified updates the audit chain verification status.
func (m *SecurityMetrics) SetChainVerified(verified bool, entries int, brokenAt *time.Time) {
	m.mu.Lock()
	m.ChainVerified = verified
	m.ChainEntries = entries
	m.ChainLastCheck = time.Now().UTC()
	m.ChainBrokenAt = brokenAt
	m.mu.Unlock()
}

// RecordSIEMExport increments SIEM export counters.
func (m *SecurityMetrics) RecordSIEMExport(success bool) {
	m.mu.Lock()
	if success {
		m.SIEMEventsExported++
	} else {
		m.SIEMExportErrors++
	}
	m.mu.Unlock()
}

// RecordTokenRevocationAlias is an exported alias for package-level use.
func (m *SecurityMetrics) RecordTokenRevocationAlias() {
	m.RecordTokenRevocation()
}
