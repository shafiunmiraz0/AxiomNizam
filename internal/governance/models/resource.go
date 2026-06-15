package models

// =====================================================
// WS-6.1 — Governance and Compliance as declarative resources
//
// CompliancePolicyResource defines compliance rules for frameworks
// like GDPR, HIPAA, SOC2, PCI-DSS. The reconciler audits catalog
// assets against policy rules and reports violations.
//
// RetentionPolicyResource defines data lifecycle rules — archive,
// delete, or anonymize data based on age.
//
// AccessRequestResource models self-service access request workflows
// with approval chains and time-bound grants.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// --- Constants ---

const (
	CompliancePolicyKind       = "CompliancePolicy"
	CompliancePolicyAPIVersion = "governance.axiomnizam.io/v1"

	RetentionPolicyKind       = "RetentionPolicy"
	RetentionPolicyAPIVersion = "governance.axiomnizam.io/v1"

	AccessRequestKind       = "AccessRequest"
	AccessRequestAPIVersion = "governance.axiomnizam.io/v1"
)

// --- Compliance Frameworks ---

type ComplianceFramework string

const (
	FrameworkGDPR   ComplianceFramework = "gdpr"
	FrameworkHIPAA  ComplianceFramework = "hipaa"
	FrameworkSOC2   ComplianceFramework = "soc2"
	FrameworkPCIDSS ComplianceFramework = "pci_dss"
	FrameworkCustom ComplianceFramework = "custom"
)

// --- Enforcement Mode ---

type EnforcementMode string

const (
	EnforcementAudit EnforcementMode = "audit" // Log only
	EnforcementWarn  EnforcementMode = "warn"  // Log + alert
	EnforcementBlock EnforcementMode = "block" // Prevent non-compliant actions
)

// --- Policy Scope ---

type PolicyScope struct {
	Domains         []string `json:"domains,omitempty"`
	Classifications []string `json:"classifications,omitempty"`
	DataSources     []string `json:"dataSources,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	AllAssets        bool     `json:"allAssets,omitempty"`
}

// --- Compliance Rule ---

type ComplianceRule struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Type        string          `json:"type"` // retention, access, encryption, masking, audit, location

	Retention  *RetentionRule  `json:"retention,omitempty"`
	Access     *AccessRule     `json:"access,omitempty"`
	Encryption *EncryptionRule `json:"encryption,omitempty"`
	Masking    *MaskingRule    `json:"masking,omitempty"`
	Location   *LocationRule   `json:"location,omitempty"`
}

type RetentionRule struct {
	MaxRetentionDays int    `json:"maxRetentionDays"`
	MinRetentionDays int    `json:"minRetentionDays,omitempty"`
	Action           string `json:"action"` // archive, delete, anonymize
	GracePeriodDays  int    `json:"gracePeriodDays,omitempty"`
}

type AccessRule struct {
	MaxAccessLevel   string   `json:"maxAccessLevel,omitempty"`   // read, write, admin
	RequireApproval  bool     `json:"requireApproval"`
	MaxGrantDays     int      `json:"maxGrantDays,omitempty"`
	AllowedRoles     []string `json:"allowedRoles,omitempty"`
	ForbiddenRoles   []string `json:"forbiddenRoles,omitempty"`
}

type EncryptionRule struct {
	RequireAtRest    bool   `json:"requireAtRest"`
	RequireInTransit bool   `json:"requireInTransit"`
	MinKeyLength     int    `json:"minKeyLength,omitempty"`
	Algorithm        string `json:"algorithm,omitempty"` // AES-256-GCM, etc.
}

type MaskingRule struct {
	ColumnPatterns []string `json:"columnPatterns"`
	MaskType       string   `json:"maskType"`   // hash, redact, partial, tokenize
	ExemptRoles    []string `json:"exemptRoles,omitempty"`
}

type LocationRule struct {
	AllowedRegions   []string `json:"allowedRegions,omitempty"`
	ForbiddenRegions []string `json:"forbiddenRegions,omitempty"`
}

// --- CompliancePolicySpec ---

type CompliancePolicySpec struct {
	Framework      ComplianceFramework `json:"framework"`
	DisplayName    string              `json:"displayName"`
	Description    string              `json:"description,omitempty"`
	Scope          PolicyScope         `json:"scope"`
	Rules          []ComplianceRule    `json:"rules"`
	Enforcement    EnforcementMode     `json:"enforcement"`
	ReviewSchedule string              `json:"reviewSchedule,omitempty"`
	Owner          string              `json:"owner,omitempty"`
	Approvers      []string            `json:"approvers,omitempty"`
	Enabled        bool                `json:"enabled"`
}

// --- Compliance Violation ---

type ComplianceViolation struct {
	RuleID     string    `json:"ruleId"`
	RuleName   string    `json:"ruleName"`
	AssetRef   string    `json:"assetRef"`
	Severity   string    `json:"severity"` // critical, warning, info
	Message    string    `json:"message"`
	DetectedAt time.Time `json:"detectedAt"`
	Remediation string   `json:"remediation,omitempty"`
}

// --- CompliancePolicyResourceStatus ---

type CompliancePolicyResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	Compliant          bool                  `json:"compliant"`
	Violations         []ComplianceViolation `json:"violations,omitempty"`
	AssetsAudited      int                   `json:"assetsAudited"`
	AssetsCompliant    int                   `json:"assetsCompliant"`
	AssetsNonCompliant int                   `json:"assetsNonCompliant"`
	LastAuditAt        *time.Time            `json:"lastAuditAt,omitempty"`
	NextAuditAt        *time.Time            `json:"nextAuditAt,omitempty"`
	ComplianceScore    float64               `json:"complianceScore"` // 0-100
}

// --- CompliancePolicyResource ---

type CompliancePolicyResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   CompliancePolicySpec           `json:"spec"`
	Status CompliancePolicyResourceStatus `json:"status"`
}

func (r *CompliancePolicyResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *CompliancePolicyResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *CompliancePolicyResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *CompliancePolicyResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *CompliancePolicyResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Rules) > 0 {
		cp.Spec.Rules = make([]ComplianceRule, len(r.Spec.Rules))
		copy(cp.Spec.Rules, r.Spec.Rules)
	}
	if len(r.Status.Violations) > 0 {
		cp.Status.Violations = make([]ComplianceViolation, len(r.Status.Violations))
		copy(cp.Status.Violations, r.Status.Violations)
	}
	return &cp
}
func (r *CompliancePolicyResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *CompliancePolicyResource) GetGeneration() int64         { return r.Generation }
func (r *CompliancePolicyResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// =====================================================
// RetentionPolicyResource
// =====================================================

type RetentionPolicySpec struct {
	DisplayName    string          `json:"displayName"`
	Description    string          `json:"description,omitempty"`
	Scope          PolicyScope     `json:"scope"`
	MaxRetentionDays int           `json:"maxRetentionDays"`
	Action         string          `json:"action"` // delete, archive, anonymize, aggregate
	GracePeriodDays int            `json:"gracePeriodDays,omitempty"`
	DryRun         bool            `json:"dryRun"`
	Schedule       string          `json:"schedule,omitempty"` // Cron
	Enabled        bool            `json:"enabled"`
}

type RetentionPolicyResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	AssetsScanned    int        `json:"assetsScanned"`
	RecordsPurged    int64      `json:"recordsPurged"`
	BytesFreed       int64      `json:"bytesFreed"`
	LastRunAt        *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt        *time.Time `json:"nextRunAt,omitempty"`
	LastRunDuration  string     `json:"lastRunDuration,omitempty"`
	Errors           []string   `json:"errors,omitempty"`
}

type RetentionPolicyResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   RetentionPolicySpec           `json:"spec"`
	Status RetentionPolicyResourceStatus `json:"status"`
}

func (r *RetentionPolicyResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *RetentionPolicyResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *RetentionPolicyResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *RetentionPolicyResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *RetentionPolicyResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *RetentionPolicyResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *RetentionPolicyResource) GetGeneration() int64         { return r.Generation }
func (r *RetentionPolicyResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// =====================================================
// AccessRequestResource
// =====================================================

type AccessRequestSpec struct {
	Requestor     string `json:"requestor"`
	AssetRef      string `json:"assetRef"`
	AccessLevel   string `json:"accessLevel"` // read, write, admin
	Justification string `json:"justification"`
	Duration      string `json:"duration"` // "30d", "90d", "permanent"
	Approvers     []string `json:"approvers,omitempty"`
}

type AccessRequestResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	ApprovalStatus string     `json:"approvalStatus"` // pending, approved, denied, expired, revoked
	ApprovedBy     []string   `json:"approvedBy,omitempty"`
	DeniedBy       string     `json:"deniedBy,omitempty"`
	DenyReason     string     `json:"denyReason,omitempty"`
	GrantedAt      *time.Time `json:"grantedAt,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	RevokedAt      *time.Time `json:"revokedAt,omitempty"`
	RevokeReason   string     `json:"revokeReason,omitempty"`
}

type AccessRequestResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   AccessRequestSpec           `json:"spec"`
	Status AccessRequestResourceStatus `json:"status"`
}

func (r *AccessRequestResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *AccessRequestResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *AccessRequestResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *AccessRequestResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *AccessRequestResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *AccessRequestResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *AccessRequestResource) GetGeneration() int64         { return r.Generation }
func (r *AccessRequestResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
