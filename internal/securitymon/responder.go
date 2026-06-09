package securitymon

import (
	"log"
	"time"
)

// ThreatResponder performs automated responses when threat thresholds are exceeded.
// It integrates with the IAM session store and revoked token store.
type ThreatResponder struct {
	sessionRevoker SessionRevoker
	tokenRevoker   TokenRevoker
	metrics        *SecurityMetrics
	thresholds     ThreatThresholds
}

// SessionRevoker revokes IAM sessions by session ID or user ID.
type SessionRevoker interface {
	Revoke(sessionID string) error
	RevokeByUserID(userID string) error
}

// TokenRevoker revokes JWT tokens by JTI.
type TokenRevoker interface {
	Revoke(jti string, ttl time.Duration) error
}

// ThreatThresholds configures automated response triggers.
type ThreatThresholds struct {
	// AuthFailuresPerMinute triggers session revocation when exceeded.
	AuthFailuresPerMinute int
	// HighRiskScore triggers session revocation when risk >= this value.
	HighRiskScore int
	// MFAFailureLimit triggers session lockout after N consecutive MFA failures.
	MFAFailureLimit int
}

// DefaultThreatThresholds returns sensible default thresholds.
func DefaultThreatThresholds() ThreatThresholds {
	return ThreatThresholds{
		AuthFailuresPerMinute: 10,
		HighRiskScore:         90,
		MFAFailureLimit:       5,
	}
}

// NewThreatResponder creates a new threat responder.
func NewThreatResponder(sessions SessionRevoker, tokens TokenRevoker, metrics *SecurityMetrics, thresholds ThreatThresholds) *ThreatResponder {
	return &ThreatResponder{
		sessionRevoker: sessions,
		tokenRevoker:   tokens,
		metrics:        metrics,
		thresholds:     thresholds,
	}
}

// HandleAnomaly processes an anomaly event and takes automated action.
func (r *ThreatResponder) HandleAnomaly(evt AnomalyEvent) {
	if r.sessionRevoker == nil {
		return
	}

	switch evt.Type {
	case "ip_spike":
		// Log the anomaly — IP-level blocking would be done at the reverse proxy/firewall level.
		log.Printf("🚨 [ThreatResponder] IP spike from %s: %d requests (baseline %.1f). Consider IP-level rate limiting.",
			evt.Source, evt.Count, evt.Baseline)
		if r.metrics != nil {
			r.metrics.RecordHighRisk()
		}

	case "user_spike":
		// Automated response: revoke all sessions for the user.
		log.Printf("🚨 [ThreatResponder] User spike from %s: %d requests. Revoking all sessions.",
			evt.Source, evt.Count)
		if err := r.sessionRevoker.RevokeByUserID(evt.Source); err != nil {
			log.Printf("⚠️  [ThreatResponder] Failed to revoke sessions for user %s: %v", evt.Source, err)
		} else {
			log.Printf("✅ [ThreatResponder] All sessions revoked for user %s", evt.Source)
			if r.metrics != nil {
				r.metrics.RecordSessionRevoked()
			}
		}
	}
}

// Thresholds returns the current threat thresholds.
func (r *ThreatResponder) Thresholds() ThreatThresholds {
	return r.thresholds
}

// HandleCriticalRisk processes a critical risk score event.
// Called from main.go authenticateRequest() when risk >= threshold.
func (r *ThreatResponder) HandleCriticalRisk(userID, sessionID string, riskScore int) {
	if r.sessionRevoker == nil {
		return
	}

	if riskScore >= r.thresholds.HighRiskScore {
		log.Printf("🚨 [ThreatResponder] Critical risk %d for user %s — revoking session %s",
			riskScore, userID, sessionID)
		if sessionID != "" {
			if err := r.sessionRevoker.Revoke(sessionID); err != nil {
				log.Printf("⚠️  [ThreatResponder] Failed to revoke session: %v", err)
			} else {
				if r.metrics != nil {
					r.metrics.RecordSessionRevoked()
				}
			}
		}
	}
}
