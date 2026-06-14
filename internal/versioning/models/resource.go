package models

// =====================================================
// P2 resource-ification -- Versioning.
//
// VersionPolicyResource wraps the imperative VersioningConfig so a
// controller can reconcile versioning policies as first-class
// platform resources.
// =====================================================

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// RetentionPolicy defines how long version is kept
type RetentionPolicy struct {
	DaysToKeep       int  `json:"daysToKeep"`
	VersionsToKeep   int  `json:"versionsToKeep"`   // Keep last N versions
	ArchiveAfterDays int  `json:"archiveAfterDays"` // Move to cold storage
	DeleteAfterDays  int  `json:"deleteAfterDays"`
	KeepMinor        bool `json:"keepMinor"` // Keep minor revisions
	KeepMajor        bool `json:"keepMajor"` // Always keep major releases
}

const (
	VersionPolicyKind       = "VersionPolicy"
	VersionPolicyAPIVersion = "versioning.axiomnizam.io/v1"
)

// VersionPolicySpec is the desired state of a versioning policy.
type VersionPolicySpec struct {
	TenantID             string          `json:"tenantId,omitempty"`
	ResourceType         string          `json:"resourceType"`
	MaxVersions          int             `json:"maxVersions,omitempty"`
	RetentionPolicy      RetentionPolicy `json:"retentionPolicy,omitempty"`
	CompressionEnabled   bool            `json:"compressionEnabled,omitempty"`
	DeduplicationEnabled bool            `json:"deduplicationEnabled,omitempty"`
	AutoSnapshot         bool            `json:"autoSnapshot,omitempty"`
	BranchingEnabled     bool            `json:"branchingEnabled,omitempty"`
	DiffEnabled          bool            `json:"diffEnabled,omitempty"`
	RollbackEnabled      bool            `json:"rollbackEnabled,omitempty"`
	Enabled              bool            `json:"enabled"`
}

// VersionPolicyResourceStatus extends the canonical object status.
type VersionPolicyResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	PolicyActive  bool  `json:"policyActive"`
	TotalVersions int64 `json:"totalVersions"`
}

// VersionPolicyResource is the declarative resource for a VersionPolicy.
type VersionPolicyResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   VersionPolicySpec           `json:"spec"`
	Status VersionPolicyResourceStatus `json:"status"`
}

func (r *VersionPolicyResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *VersionPolicyResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *VersionPolicyResource) GetStatus() *resources.ObjectStatus {
	return &r.Status.ObjectStatus
}
func (r *VersionPolicyResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *VersionPolicyResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *VersionPolicyResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *VersionPolicyResource) GetGeneration() int64         { return r.Generation }
func (r *VersionPolicyResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
