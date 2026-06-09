package securitymon

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// DashboardHandler serves the security dashboard API endpoint.
type DashboardHandler struct {
	metrics    *SecurityMetrics
	detector   *AnomalyDetector
	responder  *ThreatResponder
	verifier   *AuditChainVerifier
	siem       *SIEMExporter
}

// NewDashboardHandler creates a new security dashboard handler.
func NewDashboardHandler(
	metrics *SecurityMetrics,
	detector *AnomalyDetector,
	responder *ThreatResponder,
	verifier *AuditChainVerifier,
	siem *SIEMExporter,
) *DashboardHandler {
	return &DashboardHandler{
		metrics:   metrics,
		detector:  detector,
		responder: responder,
		verifier:  verifier,
		siem:      siem,
	}
}

// RegisterRoutes registers security dashboard routes on the given engine.
func (h *DashboardHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/security/dashboard", h.GetDashboard)
	r.GET("/api/security/metrics", h.GetMetrics)
	r.GET("/api/security/health", h.GetSecurityHealth)
}

// SecurityDashboardResponse is the full dashboard response.
type SecurityDashboardResponse struct {
	Summary        DashboardSummary    `json:"summary"`
	Metrics        SecurityMetrics     `json:"metrics"`
	AnomalyStats   AnomalyStats        `json:"anomaly_stats"`
	ChainStatus    ChainStatus         `json:"audit_chain"`
	GeneratedAt    time.Time           `json:"generated_at"`
	Uptime         string              `json:"uptime"`
}

// DashboardSummary provides a high-level security status.
type DashboardSummary struct {
	Status          string `json:"status"` // "healthy", "degraded", "critical"
	AuthFailures    int64  `json:"auth_failures_24h"`
	HighRiskEvents  int64  `json:"high_risk_events"`
	SessionsRevoked int64  `json:"sessions_revoked"`
	PolicyBlocks    int64  `json:"policy_blocks"`
	RBACDenials     int64  `json:"rbac_denials"`
	ChainIntact     bool   `json:"chain_intact"`
}

// AnomalyStats provides anomaly detection statistics.
type AnomalyStats struct {
	UniqueIPs   int `json:"unique_ips"`
	UniqueUsers int `json:"unique_users"`
}

// ChainStatus provides audit chain verification status.
type ChainStatus struct {
	Verified  bool       `json:"verified"`
	Entries   int        `json:"entries"`
	LastCheck time.Time  `json:"last_check"`
	BrokenAt  *time.Time `json:"broken_at,omitempty"`
}

// GetDashboard returns the full security dashboard.
func (h *DashboardHandler) GetDashboard(c *gin.Context) {
	snap := h.metrics.Snapshot()

	uniqueIPs, uniqueUsers := 0, 0
	if h.detector != nil {
		uniqueIPs, uniqueUsers = h.detector.GetStats()
	}

	status := "healthy"
	if snap.ChainBrokenAt != nil {
		status = "critical"
	} else if snap.HighRiskRequests > 0 || snap.AuthFailures > 100 {
		status = "degraded"
	}

	c.JSON(http.StatusOK, SecurityDashboardResponse{
		Summary: DashboardSummary{
			Status:          status,
			AuthFailures:    snap.AuthFailures,
			HighRiskEvents:  snap.HighRiskRequests,
			SessionsRevoked: snap.SessionsRevoked,
			PolicyBlocks:    snap.PolicyBlocks,
			RBACDenials:     snap.RBACDenials,
			ChainIntact:     snap.ChainVerified,
		},
		Metrics: snap,
		AnomalyStats: AnomalyStats{
			UniqueIPs:   uniqueIPs,
			UniqueUsers: uniqueUsers,
		},
		ChainStatus: ChainStatus{
			Verified:  snap.ChainVerified,
			Entries:   snap.ChainEntries,
			LastCheck: snap.ChainLastCheck,
			BrokenAt:  snap.ChainBrokenAt,
		},
		GeneratedAt: time.Now().UTC(),
		Uptime:      time.Since(snap.StartedAt).Round(time.Second).String(),
	})
}

// GetMetrics returns raw security metrics.
func (h *DashboardHandler) GetMetrics(c *gin.Context) {
	c.JSON(http.StatusOK, h.metrics.Snapshot())
}

// GetSecurityHealth returns a simple health status for load balancer probes.
func (h *DashboardHandler) GetSecurityHealth(c *gin.Context) {
	snap := h.metrics.Snapshot()
	if snap.ChainBrokenAt != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "critical", "chain_broken": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
