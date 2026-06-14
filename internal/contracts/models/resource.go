package models

// =====================================================
// WS-2.2 -- Data Contracts as declarative resources
//
// DataContractResource defines a schema contract between data
// producers and consumers. The reconciler validates that the actual
// schema matches the contract and detects breaking changes.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// --- Constants ---

const (
	Kind       = "DataContract"
	APIVersion = "contracts.axiomnizam.io/v1"
)

// --- Compatibility Modes ---

type CompatibilityMode string

const (
	CompatBackward CompatibilityMode = "backward" // New schema can read old data
	CompatForward  CompatibilityMode = "forward"  // Old schema can read new data
	CompatFull     CompatibilityMode = "full"     // Both directions
	CompatNone     CompatibilityMode = "none"     // No compatibility guarantee
)

// --- Contract Schema ---

type ContractColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Nullable    bool   `json:"nullable"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

type ContractSchema struct {
	Columns          []ContractColumn `json:"columns"`
	PrimaryKey       []string         `json:"primaryKey,omitempty"`
	RequiredColumns  []string         `json:"requiredColumns,omitempty"`
	ForbiddenChanges []string         `json:"forbiddenChanges,omitempty"`
}

// --- Contract SLA ---

type ContractSLA struct {
	MaxFreshnessAge string  `json:"maxFreshnessAge,omitempty"`
	MinAvailability float64 `json:"minAvailability,omitempty"`
	MaxLatencyMs    int64   `json:"maxLatencyMs,omitempty"`
}

// --- Contract Quality ---

type ContractQuality struct {
	MinQualityScore float64  `json:"minQualityScore"`
	RequiredRules   []string `json:"requiredRules,omitempty"`
}

// --- DataContractSpec ---

type DataContractSpec struct {
	// Producer is the team/service that owns the data
	Producer string `json:"producer"`

	// Consumers are teams that depend on this data
	Consumers []string `json:"consumers"`

	// AssetRef references the CatalogAsset this contract covers
	AssetRef string `json:"assetRef"`

	// SchemaVersion is the semver version of this contract
	SchemaVersion string `json:"schemaVersion"`

	// Schema defines the expected schema
	Schema ContractSchema `json:"schema"`

	// SLA defines freshness and availability guarantees
	SLA ContractSLA `json:"sla,omitempty"`

	// Quality defines minimum quality requirements
	Quality ContractQuality `json:"quality,omitempty"`

	// Compatibility mode for schema evolution
	Compatibility CompatibilityMode `json:"compatibility"`

	// NotifyOnBreak lists channels to notify on contract violation
	NotifyOnBreak []string `json:"notifyOnBreak,omitempty"`

	// Description of the contract
	Description string `json:"description,omitempty"`

	// Enabled controls whether this contract is actively enforced
	Enabled bool `json:"enabled"`
}

// --- Violation ---

type ContractViolation struct {
	Type       string    `json:"type"`
	Severity   string    `json:"severity"`
	Message    string    `json:"message"`
	Field      string    `json:"field,omitempty"`
	Expected   string    `json:"expected,omitempty"`
	Actual     string    `json:"actual,omitempty"`
	DetectedAt time.Time `json:"detectedAt"`
}

// --- DataContractResourceStatus ---

type DataContractResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	// Compliant indicates whether the contract is currently met
	Compliant bool `json:"compliant"`

	// Violations lists current contract violations
	Violations []ContractViolation `json:"violations,omitempty"`

	// LastValidatedAt is when the contract was last checked
	LastValidatedAt *time.Time `json:"lastValidatedAt,omitempty"`

	// SchemaMatchPercent is how much of the contract schema matches actual
	SchemaMatchPercent float64 `json:"schemaMatchPercent"`

	// SLAStatus: met, at_risk, breached
	SLAStatus string `json:"slaStatus,omitempty"`

	// QualityStatus: met, at_risk, breached
	QualityStatus string `json:"qualityStatus,omitempty"`

	// ConsecutiveViolations counts sequential validation failures
	ConsecutiveViolations int `json:"consecutiveViolations"`

	// LastCompliantAt is when the contract was last fully met
	LastCompliantAt *time.Time `json:"lastCompliantAt,omitempty"`
}

// --- DataContractResource ---

type DataContractResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   DataContractSpec           `json:"spec"`
	Status DataContractResourceStatus `json:"status"`
}

// --- resources.Resource implementation ---

func (r *DataContractResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *DataContractResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *DataContractResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *DataContractResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *DataContractResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Consumers) > 0 {
		cp.Spec.Consumers = append([]string(nil), r.Spec.Consumers...)
	}
	if len(r.Spec.Schema.Columns) > 0 {
		cp.Spec.Schema.Columns = make([]ContractColumn, len(r.Spec.Schema.Columns))
		copy(cp.Spec.Schema.Columns, r.Spec.Schema.Columns)
	}
	if len(r.Status.Violations) > 0 {
		cp.Status.Violations = make([]ContractViolation, len(r.Status.Violations))
		copy(cp.Status.Violations, r.Status.Violations)
	}
	return &cp
}
func (r *DataContractResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *DataContractResource) GetGeneration() int64         { return r.Generation }
func (r *DataContractResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
