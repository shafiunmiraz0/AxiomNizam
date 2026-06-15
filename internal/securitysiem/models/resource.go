package models

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// --- Severity ---

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// --- EventCategory ---

type EventCategory string

const (
	CategoryAuth      EventCategory = "auth"
	CategoryAccess    EventCategory = "access"
	CategoryMalware   EventCategory = "malware"
	CategoryNetwork   EventCategory = "network"
	CategoryDataLoss  EventCategory = "data_loss"
	CategoryCompliance EventCategory = "compliance"
	CategoryCustom    EventCategory = "custom"
)

// --- ThreatStatus ---

type ThreatStatus string

const (
	ThreatDetected  ThreatStatus = "detected"
	ThreatConfirmed ThreatStatus = "confirmed"
	ThreatMitigated ThreatStatus = "mitigated"
	ThreatResolved  ThreatStatus = "resolved"
)

// --- IncidentPhase ---

type IncidentPhase string

const (
	PhaseDetection     IncidentPhase = "detection"
	PhaseTriage        IncidentPhase = "triage"
	PhaseInvestigation IncidentPhase = "investigation"
	PhaseContainment   IncidentPhase = "containment"
	PhaseEradication   IncidentPhase = "eradication"
	PhaseRecovery      IncidentPhase = "recovery"
	PhaseResolved      IncidentPhase = "resolved"
	PhaseClosed        IncidentPhase = "closed"
)

// --- ComplianceFramework ---

type ComplianceFramework string

const (
	FrameworkSOC2    ComplianceFramework = "soc2"
	FrameworkGDPR    ComplianceFramework = "gdpr"
	FrameworkHIPAA   ComplianceFramework = "hipaa"
	FrameworkPCIDSS  ComplianceFramework = "pci_dss"
	FrameworkISO27001 ComplianceFramework = "iso27001"
)

// --- VulnStatus ---

type VulnStatus string

const (
	VulnOpen       VulnStatus = "open"
	VulnInProgress VulnStatus = "in_progress"
	VulnFixed      VulnStatus = "fixed"
	VulnAccepted   VulnStatus = "accepted"
	VulnDeferred   VulnStatus = "deferred"
)

// --- FeedType ---

type FeedType string

const (
	FeedTypeSTIX    FeedType = "stix"
	FeedTypeMISP    FeedType = "misp"
	FeedTypeCSV     FeedType = "csv"
	FeedTypeCustom  FeedType = "custom"
)

// --- PolicyAction ---

type PolicyAction string

const (
	ActionAllow    PolicyAction = "allow"
	ActionDeny     PolicyAction = "deny"
	ActionQuarantine PolicyAction = "quarantine"
	ActionAlert    PolicyAction = "alert"
	ActionBlock    PolicyAction = "block"
)

// =====================================================
// SecurityEventResource — security audit event
// =====================================================

type SecurityEventResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   SecurityEventSpec   `json:"spec"`
	Status SecurityEventStatus `json:"status"`
}

type SecurityEventSpec struct {
	Category    EventCategory     `json:"category"`
	Severity    Severity          `json:"severity"`
	Source      string            `json:"source"`
	Actor       string            `json:"actor,omitempty"`
	Target      string            `json:"target,omitempty"`
	Action      string            `json:"action"`
	Outcome     string            `json:"outcome"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type SecurityEventStatus struct {
	resources.ObjectStatus `json:",inline"`
}

func (r *SecurityEventResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *SecurityEventResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *SecurityEventResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *SecurityEventResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *SecurityEventResource) DeepCopy() resources.Resource {
	out := *r
	return &out
}
func (r *SecurityEventResource) GetKey() string {
	return r.Namespace + "/" + r.Name
}

// =====================================================
// ThreatResource — detected threat
// =====================================================

type ThreatResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   ThreatSpec       `json:"spec"`
	Status ThreatResourceStatus `json:"status"`
}

type ThreatResourceStatus struct {
	resources.ObjectStatus `json:",inline"`
	ThreatStatus ThreatStatus `json:"threatStatus"`
}

type ThreatSpec struct {
	ThreatType  string   `json:"threatType"`
	Severity    Severity `json:"severity"`
	Source      string   `json:"source,omitempty"`
	Indicators  []string `json:"indicators,omitempty"`
	Description string   `json:"description,omitempty"`
}

func (r *ThreatResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *ThreatResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *ThreatResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *ThreatResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *ThreatResource) DeepCopy() resources.Resource {
	out := *r
	return &out
}
func (r *ThreatResource) GetKey() string {
	return r.Namespace + "/" + r.Name
}

// =====================================================
// IncidentResource — security incident
// =====================================================

type IncidentResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   IncidentSpec   `json:"spec"`
	Status IncidentStatus `json:"status"`
}

type IncidentSpec struct {
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Severity    Severity        `json:"severity"`
	Phase       IncidentPhase   `json:"phase"`
	Assignee    string          `json:"assignee,omitempty"`
	Threats     []string        `json:"threats,omitempty"`
}

type IncidentStatus struct {
	resources.ObjectStatus `json:",inline"`
}

func (r *IncidentResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *IncidentResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *IncidentResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *IncidentResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *IncidentResource) DeepCopy() resources.Resource {
	out := *r
	return &out
}
func (r *IncidentResource) GetKey() string {
	return r.Namespace + "/" + r.Name
}

// =====================================================
// ComplianceRuleResource — compliance policy rule
// =====================================================

type ComplianceRuleResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   ComplianceRuleSpec   `json:"spec"`
	Status ComplianceRuleStatus `json:"status"`
}

type ComplianceRuleSpec struct {
	Framework   ComplianceFramework `json:"framework"`
	RuleID      string              `json:"ruleID"`
	Title       string              `json:"title"`
	Description string              `json:"description,omitempty"`
	Severity    Severity            `json:"severity"`
	Enabled     bool                `json:"enabled"`
}

type ComplianceRuleStatus struct {
	resources.ObjectStatus `json:",inline"`
}

func (r *ComplianceRuleResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *ComplianceRuleResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *ComplianceRuleResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *ComplianceRuleResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *ComplianceRuleResource) DeepCopy() resources.Resource {
	out := *r
	return &out
}
func (r *ComplianceRuleResource) GetKey() string {
	return r.Namespace + "/" + r.Name
}

// =====================================================
// VulnerabilityResource — tracked vulnerability
// =====================================================

type VulnerabilityResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   VulnerabilitySpec   `json:"spec"`
	Status VulnerabilityStatus `json:"status"`
}

type VulnerabilitySpec struct {
	CVE         string   `json:"cve,omitempty"`
	Package     string   `json:"package"`
	Version     string   `json:"version,omitempty"`
	Severity    Severity `json:"severity"`
	CVSS        float64  `json:"cvss,omitempty"`
	Description string   `json:"description,omitempty"`
	FixVersion  string   `json:"fixVersion,omitempty"`
}

type VulnerabilityStatus struct {
	resources.ObjectStatus `json:",inline"`
	VulnStatus VulnStatus `json:"vulnStatus"`
}

func (r *VulnerabilityResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *VulnerabilityResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *VulnerabilityResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *VulnerabilityResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *VulnerabilityResource) DeepCopy() resources.Resource {
	out := *r
	return &out
}
func (r *VulnerabilityResource) GetKey() string {
	return r.Namespace + "/" + r.Name
}

// =====================================================
// SecurityPolicyResource — security enforcement policy
// =====================================================

type SecurityPolicyResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   SecurityPolicySpec   `json:"spec"`
	Status SecurityPolicyStatus `json:"status"`
}

type SecurityPolicySpec struct {
	DisplayName string       `json:"displayName"`
	Description string       `json:"description,omitempty"`
	Action      PolicyAction `json:"action"`
	Enabled     bool         `json:"enabled"`
	Rules       []string     `json:"rules,omitempty"`
}

type SecurityPolicyStatus struct {
	resources.ObjectStatus `json:",inline"`
}

func (r *SecurityPolicyResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *SecurityPolicyResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *SecurityPolicyResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *SecurityPolicyResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *SecurityPolicyResource) DeepCopy() resources.Resource {
	out := *r
	return &out
}
func (r *SecurityPolicyResource) GetKey() string {
	return r.Namespace + "/" + r.Name
}

// =====================================================
// ThreatIntelFeedResource — threat intelligence feed
// =====================================================

type ThreatIntelFeedResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   ThreatIntelFeedSpec   `json:"spec"`
	Status ThreatIntelFeedStatus `json:"status"`
}

type ThreatIntelFeedSpec struct {
	DisplayName string   `json:"displayName"`
	FeedType    FeedType `json:"feedType"`
	URL         string   `json:"url,omitempty"`
	Enabled     bool     `json:"enabled"`
	Interval    string   `json:"interval,omitempty"` // "1h", "6h", "24h"
}

type ThreatIntelFeedStatus struct {
	resources.ObjectStatus `json:",inline"`
	LastSync string `json:"lastSync,omitempty"`
}

func (r *ThreatIntelFeedResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *ThreatIntelFeedResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *ThreatIntelFeedResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *ThreatIntelFeedResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *ThreatIntelFeedResource) DeepCopy() resources.Resource {
	out := *r
	return &out
}
func (r *ThreatIntelFeedResource) GetKey() string {
	return r.Namespace + "/" + r.Name
}

// =====================================================
// IncidentResponseResource — incident response playbook
// =====================================================

type IncidentResponseResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   IncidentResponseSpec   `json:"spec"`
	Status IncidentResponseStatus `json:"status"`
}

type IncidentResponseSpec struct {
	DisplayName string          `json:"displayName"`
	Description string          `json:"description,omitempty"`
	Phase       IncidentPhase   `json:"phase"`
	Steps       []ResponseStep  `json:"steps,omitempty"`
	AutoAssign  bool            `json:"autoAssign"`
}

type ResponseStep struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Action      string `json:"action"`
	Timeout     string `json:"timeout,omitempty"`
}

type IncidentResponseStatus struct {
	resources.ObjectStatus `json:",inline"`
}

func (r *IncidentResponseResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *IncidentResponseResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *IncidentResponseResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *IncidentResponseResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *IncidentResponseResource) DeepCopy() resources.Resource {
	out := *r
	return &out
}
func (r *IncidentResponseResource) GetKey() string {
	return r.Namespace + "/" + r.Name
}
