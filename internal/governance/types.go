package governance

// Type aliases re-exported from models/ for backward compatibility.
// New code should import governance/models directly.

import "axiomnizam.bitbd.net/axiomnizam/internal/governance/models"

type ComplianceFramework = models.ComplianceFramework
type EnforcementMode = models.EnforcementMode
type PolicyScope = models.PolicyScope
type ComplianceRule = models.ComplianceRule
type RetentionRule = models.RetentionRule
type AccessRule = models.AccessRule
type EncryptionRule = models.EncryptionRule
type MaskingRule = models.MaskingRule
type LocationRule = models.LocationRule
type CompliancePolicySpec = models.CompliancePolicySpec
type ComplianceViolation = models.ComplianceViolation
type CompliancePolicyResourceStatus = models.CompliancePolicyResourceStatus
type CompliancePolicyResource = models.CompliancePolicyResource
type RetentionPolicySpec = models.RetentionPolicySpec
type RetentionPolicyResourceStatus = models.RetentionPolicyResourceStatus
type RetentionPolicyResource = models.RetentionPolicyResource
type AccessRequestSpec = models.AccessRequestSpec
type AccessRequestResourceStatus = models.AccessRequestResourceStatus
type AccessRequestResource = models.AccessRequestResource

// Constants
const (
	FrameworkGDPR   = models.FrameworkGDPR
	FrameworkHIPAA  = models.FrameworkHIPAA
	FrameworkSOC2   = models.FrameworkSOC2
	FrameworkPCIDSS = models.FrameworkPCIDSS
	FrameworkCustom = models.FrameworkCustom

	EnforcementAudit = models.EnforcementAudit
	EnforcementWarn  = models.EnforcementWarn
	EnforcementBlock = models.EnforcementBlock
)
