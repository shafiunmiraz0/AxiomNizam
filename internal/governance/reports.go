package governance

// =====================================================
// WS-6.1 — Compliance Report Generation
//
// Generates compliance reports for GDPR, HIPAA, SOC2, PCI-DSS
// audits. Reports include policy status, violations, remediation
// actions, and historical trends.
// =====================================================

import (
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/governance/models"
)

// ReportFormat defines the output format.
type ReportFormat string

const (
	ReportFormatJSON ReportFormat = "json"
	ReportFormatText ReportFormat = "text"
)

// ComplianceReport is a point-in-time compliance audit report.
type ComplianceReport struct {
	Title           string                `json:"title"`
	Framework       models.ComplianceFramework   `json:"framework"`
	GeneratedAt     time.Time             `json:"generatedAt"`
	Period          ReportPeriod          `json:"period"`
	Summary         ReportSummary         `json:"summary"`
	PolicyResults   []PolicyReportEntry   `json:"policyResults"`
	TopViolations   []ViolationSummary    `json:"topViolations"`
	Recommendations []string              `json:"recommendations"`
}

// ReportPeriod defines the time range for the report.
type ReportPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ReportSummary provides high-level compliance metrics.
type ReportSummary struct {
	TotalPolicies      int     `json:"totalPolicies"`
	CompliantPolicies  int     `json:"compliantPolicies"`
	NonCompliant       int     `json:"nonCompliant"`
	TotalAssets        int     `json:"totalAssets"`
	CompliantAssets    int     `json:"compliantAssets"`
	TotalViolations    int     `json:"totalViolations"`
	CriticalViolations int     `json:"criticalViolations"`
	OverallScore       float64 `json:"overallScore"` // 0-100
}

// PolicyReportEntry summarizes a single policy's compliance status.
type PolicyReportEntry struct {
	PolicyName      string  `json:"policyName"`
	Framework       string  `json:"framework"`
	Compliant       bool    `json:"compliant"`
	Score           float64 `json:"score"`
	ViolationCount  int     `json:"violationCount"`
	AssetsAudited   int     `json:"assetsAudited"`
	LastAuditAt     string  `json:"lastAuditAt"`
}

// ViolationSummary groups violations by type for the report.
type ViolationSummary struct {
	RuleType    string `json:"ruleType"`
	Count       int    `json:"count"`
	Severity    string `json:"severity"`
	Example     string `json:"example"`
	Remediation string `json:"remediation"`
}

// ReportGenerator creates compliance reports from policy data.
type ReportGenerator struct{}

// NewReportGenerator creates a new generator.
func NewReportGenerator() *ReportGenerator {
	return &ReportGenerator{}
}

// Generate creates a compliance report from a set of policies.
func (g *ReportGenerator) Generate(policies []*models.CompliancePolicyResource, framework models.ComplianceFramework) *ComplianceReport {
	now := time.Now()

	report := &ComplianceReport{
		Title:       fmt.Sprintf("%s Compliance Report", frameworkDisplayName(framework)),
		Framework:   framework,
		GeneratedAt: now,
		Period: ReportPeriod{
			Start: now.AddDate(0, -1, 0), // Last 30 days
			End:   now,
		},
	}

	// Filter policies by framework.
	var relevant []*models.CompliancePolicyResource
	for _, p := range policies {
		if framework == "" || p.Spec.Framework == framework || framework == models.FrameworkCustom {
			relevant = append(relevant, p)
		}
	}

	// Build summary.
	summary := ReportSummary{TotalPolicies: len(relevant)}
	violationsByType := make(map[string]*ViolationSummary)

	for _, p := range relevant {
		entry := PolicyReportEntry{
			PolicyName:     p.Spec.DisplayName,
			Framework:      string(p.Spec.Framework),
			Compliant:      p.Status.Compliant,
			Score:          p.Status.ComplianceScore,
			ViolationCount: len(p.Status.Violations),
			AssetsAudited:  p.Status.AssetsAudited,
		}
		if p.Status.LastAuditAt != nil {
			entry.LastAuditAt = p.Status.LastAuditAt.Format(time.RFC3339)
		}
		report.PolicyResults = append(report.PolicyResults, entry)

		if p.Status.Compliant {
			summary.CompliantPolicies++
		} else {
			summary.NonCompliant++
		}
		summary.TotalAssets += p.Status.AssetsAudited
		summary.CompliantAssets += p.Status.AssetsCompliant
		summary.TotalViolations += len(p.Status.Violations)

		// Aggregate violations by type.
		for _, v := range p.Status.Violations {
			key := v.RuleName
			if _, ok := violationsByType[key]; !ok {
				violationsByType[key] = &ViolationSummary{
					RuleType:    v.RuleName,
					Severity:    v.Severity,
					Example:     v.Message,
					Remediation: v.Remediation,
				}
			}
			violationsByType[key].Count++
			if v.Severity == "critical" {
				summary.CriticalViolations++
			}
		}
	}

	if summary.TotalPolicies > 0 {
		summary.OverallScore = float64(summary.CompliantPolicies) / float64(summary.TotalPolicies) * 100.0
	}
	report.Summary = summary

	// Top violations (sorted by count).
	for _, vs := range violationsByType {
		report.TopViolations = append(report.TopViolations, *vs)
	}

	// Generate recommendations.
	report.Recommendations = g.generateRecommendations(report)

	return report
}

// generateRecommendations produces actionable suggestions.
func (g *ReportGenerator) generateRecommendations(report *ComplianceReport) []string {
	var recs []string

	if report.Summary.CriticalViolations > 0 {
		recs = append(recs, fmt.Sprintf("Address %d critical violations immediately — these represent compliance risk", report.Summary.CriticalViolations))
	}

	if report.Summary.OverallScore < 80 {
		recs = append(recs, "Overall compliance score is below 80% — schedule a remediation sprint")
	}

	if report.Summary.NonCompliant > 0 {
		recs = append(recs, fmt.Sprintf("Review %d non-compliant policies and update asset configurations", report.Summary.NonCompliant))
	}

	for _, v := range report.TopViolations {
		if v.Count > 5 {
			recs = append(recs, fmt.Sprintf("Recurring violation '%s' (%d occurrences) — consider a platform-wide fix: %s", v.RuleType, v.Count, v.Remediation))
		}
	}

	if len(recs) == 0 {
		recs = append(recs, "All policies are compliant — maintain current controls and continue monitoring")
	}

	return recs
}

func frameworkDisplayName(f models.ComplianceFramework) string {
	switch f {
	case models.FrameworkGDPR:
		return "GDPR"
	case models.FrameworkHIPAA:
		return "HIPAA"
	case models.FrameworkSOC2:
		return "SOC2"
	case models.FrameworkPCIDSS:
		return "PCI-DSS"
	default:
		return "Compliance"
	}
}
