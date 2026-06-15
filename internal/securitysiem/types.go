package securitysiem

import "axiomnizam.bitbd.net/axiomnizam/internal/securitysiem/models"

// Type aliases for backward compatibility.
type SecurityEventResource = models.SecurityEventResource
type ThreatResource = models.ThreatResource
type IncidentResource = models.IncidentResource
type ComplianceRuleResource = models.ComplianceRuleResource
type VulnerabilityResource = models.VulnerabilityResource
type SecurityPolicyResource = models.SecurityPolicyResource
type ThreatIntelFeedResource = models.ThreatIntelFeedResource
type IncidentResponseResource = models.IncidentResponseResource

// Enum aliases.
type Severity = models.Severity
type EventCategory = models.EventCategory
type ThreatStatus = models.ThreatStatus
type IncidentPhase = models.IncidentPhase
type ComplianceFramework = models.ComplianceFramework
type VulnStatus = models.VulnStatus
type FeedType = models.FeedType
type PolicyAction = models.PolicyAction

// Severity constants.
const (
	SeverityCritical = models.SeverityCritical
	SeverityHigh     = models.SeverityHigh
	SeverityMedium   = models.SeverityMedium
	SeverityLow      = models.SeverityLow
	SeverityInfo     = models.SeverityInfo
)

// Incident phase constants.
const (
	PhaseDetection     = models.PhaseDetection
	PhaseTriage        = models.PhaseTriage
	PhaseInvestigation = models.PhaseInvestigation
	PhaseContainment   = models.PhaseContainment
	PhaseEradication   = models.PhaseEradication
	PhaseRecovery      = models.PhaseRecovery
	PhaseResolved      = models.PhaseResolved
	PhaseClosed        = models.PhaseClosed
)
