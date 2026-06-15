package models

// =====================================================
// WS-7.3 -- Data Anonymization as declarative resources
//
// AnonymizationPolicyResource defines PII masking, synthetic data
// generation, and privacy-preserving transformations. The reconciler
// applies masking rules to catalog assets on schedule.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	AnonymizationPolicyKind       = "AnonymizationPolicy"
	AnonymizationPolicyAPIVersion = "anonymization.axiomnizam.io/v1"
)

// --- Masking Techniques ---

type MaskTechnique string

const (
	MaskHash       MaskTechnique = "hash"       // Consistent pseudonymization
	MaskRedact     MaskTechnique = "redact"     // Full removal: [REDACTED]
	MaskPartial    MaskTechnique = "partial"    // j***@e***.com
	MaskTokenize   MaskTechnique = "tokenize"   // Reversible with key
	MaskNoise      MaskTechnique = "noise"      // Statistical noise
	MaskGeneralize MaskTechnique = "generalize" // age:34 -> age:30-40
	MaskSynthetic  MaskTechnique = "synthetic"  // Realistic fake data
	MaskShuffle    MaskTechnique = "shuffle"    // Shuffle column values
)

// --- Anonymization Rule ---

type AnonymRule struct {
	ColumnPattern  string            `json:"columnPattern"`
	Classification string            `json:"classification"`
	Technique      MaskTechnique     `json:"technique"`
	Config         map[string]string `json:"config,omitempty"`
}

// --- Policy Scope ---

type PolicyScope struct {
	Domains     []string `json:"domains,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	DataSources []string `json:"dataSources,omitempty"`
	AssetRefs   []string `json:"assetRefs,omitempty"`
	AllAssets   bool     `json:"allAssets,omitempty"`
}

// --- AnonymizationPolicySpec ---

type AnonymizationPolicySpec struct {
	DisplayName   string       `json:"displayName"`
	Description   string       `json:"description,omitempty"`
	Scope         PolicyScope  `json:"scope"`
	Rules         []AnonymRule `json:"rules"`
	OutputMode    string       `json:"outputMode"`
	OutputTarget  string       `json:"outputTarget,omitempty"`
	Schedule      string       `json:"schedule,omitempty"`
	PreserveStats bool         `json:"preserveStats"`
	DryRun        bool         `json:"dryRun"`
	Enabled       bool         `json:"enabled"`
}

// --- AnonymizationPolicyResourceStatus ---

type AnonymizationPolicyResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	AssetsProcessed   int        `json:"assetsProcessed"`
	ColumnsAnonymized int        `json:"columnsAnonymized"`
	RowsProcessed     int64      `json:"rowsProcessed"`
	LastRunAt         *time.Time `json:"lastRunAt,omitempty"`
	LastRunDuration   string     `json:"lastRunDuration,omitempty"`
	NextRunAt         *time.Time `json:"nextRunAt,omitempty"`
	Errors            []string   `json:"errors,omitempty"`
}

// --- AnonymizationPolicyResource ---

type AnonymizationPolicyResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   AnonymizationPolicySpec           `json:"spec"`
	Status AnonymizationPolicyResourceStatus `json:"status"`
}

func (r *AnonymizationPolicyResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *AnonymizationPolicyResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *AnonymizationPolicyResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *AnonymizationPolicyResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *AnonymizationPolicyResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Rules) > 0 {
		cp.Spec.Rules = make([]AnonymRule, len(r.Spec.Rules))
		copy(cp.Spec.Rules, r.Spec.Rules)
	}
	return &cp
}
func (r *AnonymizationPolicyResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *AnonymizationPolicyResource) GetGeneration() int64         { return r.Generation }
func (r *AnonymizationPolicyResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
