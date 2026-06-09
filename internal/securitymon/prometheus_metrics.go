package securitymon

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus counters and gauges for security observability.
// These are auto-registered with the default Prometheus registry.
var (
	PromTotalRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_total_requests_total",
		Help: "Total number of requests processed",
	})
	PromAuthFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_auth_failures_total",
		Help: "Total number of authentication failures",
	})
	PromAuthSuccesses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_auth_successes_total",
		Help: "Total number of successful authentications",
	})
	PromHighRiskRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_high_risk_requests_total",
		Help: "Total number of high-risk requests detected",
	})
	PromRiskDeltaTriggers = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_risk_delta_triggers_total",
		Help: "Total number of risk delta step-up MFA triggers",
	})
	PromSessionsRevoked = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_sessions_revoked_total",
		Help: "Total number of sessions revoked due to security events",
	})
	PromMFAChallenges = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "axiomnizam_mfa_challenges_total",
		Help: "Total MFA challenges by factor type",
	}, []string{"factor_type"})
	PromMFAFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_mfa_failures_total",
		Help: "Total number of MFA verification failures",
	})
	PromStepUpRequired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_step_up_mfa_total",
		Help: "Total number of step-up MFA requirements",
	})
	PromPolicyBlocks = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_policy_blocks_total",
		Help: "Total number of requests blocked by policy engine",
	})
	PromRBACDenials = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_rbac_denials_total",
		Help: "Total number of RBAC authorization denials",
	})
	PromBlockedRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_blocked_requests_total",
		Help: "Total number of blocked requests (all reasons)",
	})
	PromTokenRevocations = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_token_revocations_total",
		Help: "Total number of token revocations",
	})
	PromAuditChainVerified = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "axiomnizam_audit_chain_verified",
		Help: "1 if audit chain is verified, 0 if broken",
	})
	PromAuditChainEntries = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "axiomnizam_audit_chain_entries",
		Help: "Number of entries in the last audit chain verification",
	})
	PromSIEMExports = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_siem_exports_total",
		Help: "Total SIEM event exports",
	})
	PromSIEMExportErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "axiomnizam_siem_export_errors_total",
		Help: "Total SIEM export errors",
	})
)
