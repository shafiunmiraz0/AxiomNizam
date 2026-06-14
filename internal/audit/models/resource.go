package models

// =====================================================
// P2 resource-ification -- Audit.
//
// AuditPolicyResource wraps the imperative AuditConfig so a
// controller can reconcile audit policies as first-class platform
// resources.
// =====================================================

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	Kind       = "AuditPolicy"
	APIVersion = "audit.axiomnizam.io/v1"
)

// Backward-compatible prefixed aliases.
const (
	AuditPolicyKind       = Kind
	AuditPolicyAPIVersion = APIVersion
)

// AuditPolicySpec is the desired state of an audit policy.
type AuditPolicySpec struct {
	TenantID             string   `json:"tenantId,omitempty"`
	LogActions           []string `json:"logActions,omitempty"`
	IgnoreActions        []string `json:"ignoreActions,omitempty"`
	LogRequestBody       bool     `json:"logRequestBody,omitempty"`
	LogResponseBody      bool     `json:"logResponseBody,omitempty"`
	SensitiveFields      []string `json:"sensitiveFields,omitempty"`
	RetentionDays        int      `json:"retentionDays,omitempty"`
	StorageBackend       string   `json:"storageBackend,omitempty"`
	AsyncWrite           bool     `json:"asyncWrite,omitempty"`
	HighRiskActions      []string `json:"highRiskActions,omitempty"`
	ComplianceMode       bool     `json:"complianceMode,omitempty"`
	EncryptSensitiveData bool     `json:"encryptSensitiveData,omitempty"`
	Enabled              bool     `json:"enabled"`
}

// AuditPolicyResourceStatus extends the canonical object status.
type AuditPolicyResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	PolicyActive bool  `json:"policyActive"`
	TotalRecords int64 `json:"totalRecords"`
}

// AuditPolicyResource is the declarative resource for an AuditPolicy.
type AuditPolicyResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   AuditPolicySpec           `json:"spec"`
	Status AuditPolicyResourceStatus `json:"status"`
}

func (r *AuditPolicyResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *AuditPolicyResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *AuditPolicyResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *AuditPolicyResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *AuditPolicyResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.LogActions) > 0 {
		cp.Spec.LogActions = append([]string(nil), r.Spec.LogActions...)
	}
	if len(r.Spec.SensitiveFields) > 0 {
		cp.Spec.SensitiveFields = append([]string(nil), r.Spec.SensitiveFields...)
	}
	return &cp
}
func (r *AuditPolicyResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *AuditPolicyResource) GetGeneration() int64         { return r.Generation }
func (r *AuditPolicyResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
