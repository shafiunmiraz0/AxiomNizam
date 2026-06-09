package securitymon

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// System is the top-level bootstrap struct for the security monitoring module.
// It wraps SecurityMetrics, AnomalyDetector, ThreatResponder, AuditChainVerifier,
// SIEMExporter, and DashboardHandler into a single lifecycle-managed unit.
type System struct {
	metrics   *SecurityMetrics
	detector  *AnomalyDetector
	responder *ThreatResponder
	verifier  *AuditChainVerifier
	siem      *SIEMExporter
	handler   *DashboardHandler
}

// NewSystem creates a new security monitoring system with all components
// initialized from environment variables and the provided dependencies.
func NewSystem(sessions SessionRevoker, tokens TokenRevoker) *System {
	metrics := NewSecurityMetrics()
	siem := LoadSIEMExporterFromEnv(metrics)

	thresholds := DefaultThreatThresholds()
	responder := NewThreatResponder(sessions, tokens, metrics, thresholds)

	detector := NewAnomalyDetector(5*time.Minute, 3.0, func(evt AnomalyEvent) {
		responder.HandleAnomaly(evt)
	})

	handler := NewDashboardHandler(metrics, detector, responder, nil, siem)

	return &System{
		metrics:   metrics,
		detector:  detector,
		responder: responder,
		siem:      siem,
		handler:   handler,
	}
}

// Metrics returns the SecurityMetrics instance.
func (s *System) Metrics() *SecurityMetrics {
	return s.metrics
}

// Detector returns the AnomalyDetector instance.
func (s *System) Detector() *AnomalyDetector {
	return s.detector
}

// Responder returns the ThreatResponder instance.
func (s *System) Responder() *ThreatResponder {
	return s.responder
}

// Verifier returns the AuditChainVerifier instance (may be nil if not started).
func (s *System) Verifier() *AuditChainVerifier {
	return s.verifier
}

// SIEM returns the SIEMExporter instance.
func (s *System) SIEM() *SIEMExporter {
	return s.siem
}

// RegisterRoutes registers security dashboard API endpoints on the given engine.
func (s *System) RegisterRoutes(r *gin.Engine) {
	s.handler.RegisterRoutes(r)
}

// StartAuditVerifier starts the scheduled audit chain verification loop.
// provider: the audit log provider for chain verification.
// interval: how often to verify the chain (e.g., 24*time.Hour).
func (s *System) StartAuditVerifier(provider AuditLogProvider, interval time.Duration) {
	s.verifier = NewAuditChainVerifier(provider, s.metrics, interval)
	s.verifier.Start()
	s.handler.verifier = s.verifier
}

// Start is a lifecycle hook — starts the audit verifier if configured.
func (s *System) Start(_ any) error {
	if s.verifier != nil {
		s.verifier.Start()
	}
	return nil
}

// Stop halts all background goroutines.
func (s *System) Stop() {
	if s.verifier != nil {
		s.verifier.Stop()
	}
}

// Name returns the module name.
func (s *System) Name() string {
	return "securitymon"
}

// RecordAuthSuccess records a successful authentication event.
func (s *System) RecordAuthSuccess() {
	s.metrics.RecordAuthSuccess()
	PromAuthSuccesses.Inc()
}

// RecordAuthFailure records a failed authentication event.
func (s *System) RecordAuthFailure() {
	s.metrics.RecordAuthFailure()
	PromAuthFailures.Inc()
}

// RecordHighRisk records a high-risk request detection.
func (s *System) RecordHighRisk() {
	s.metrics.RecordHighRisk()
	PromHighRiskRequests.Inc()
}

// RecordSessionRevoked records a session revocation event.
func (s *System) RecordSessionRevoked() {
	s.metrics.RecordSessionRevoked()
	PromSessionsRevoked.Inc()
}

// RecordMFAChallenge records an MFA challenge event.
func (s *System) RecordMFAChallenge(factorType string) {
	s.metrics.RecordMFAChallenge(factorType)
	PromMFAChallenges.WithLabelValues(factorType).Inc()
}

// RecordMFAFailure records an MFA verification failure.
func (s *System) RecordMFAFailure() {
	s.metrics.RecordMFAFailure()
	PromMFAFailures.Inc()
}

// RecordMFASuccess records a successful MFA verification.
func (s *System) RecordMFASuccess() {
	s.metrics.RecordMFASuccess()
}

// RecordPolicyBlock records a policy engine block.
func (s *System) RecordPolicyBlock() {
	s.metrics.RecordPolicyBlock()
	PromPolicyBlocks.Inc()
}

// RecordRBACDenial records an RBAC authorization denial.
func (s *System) RecordRBACDenial() {
	s.metrics.RecordRBACDenial()
	PromRBACDenials.Inc()
}

// RecordRiskDeltaTrigger records a risk delta step-up MFA trigger.
func (s *System) RecordRiskDeltaTrigger() {
	s.metrics.RecordRiskDeltaTrigger()
	PromRiskDeltaTriggers.Inc()
}

// RecordStepUp records a step-up MFA requirement.
func (s *System) RecordStepUp() {
	s.metrics.RecordStepUp()
	PromStepUpRequired.Inc()
}

// RecordTotalRequest records a total request for anomaly detection.
func (s *System) RecordTotalRequest() {
	s.metrics.RecordTotalRequest()
	PromTotalRequests.Inc()
}

// ExportSIEM exports a SIEM event asynchronously.
func (s *System) ExportSIEM(event SIEMEvent) {
	go s.siem.Export(context.Background(), event)
}
