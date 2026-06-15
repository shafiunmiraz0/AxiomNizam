package lineage

// =====================================================
// P2 resource-ification — Lineage.
//
// LineageNodeResource wraps the imperative LineageNode so a
// controller can reconcile lineage nodes as first-class platform
// resources.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	LineageNodeKind       = "LineageNode"
	LineageNodeAPIVersion = "lineage.axiomnizam.io/v1"
)

// LineageNodeSpec is the desired state of a lineage node.
type LineageNodeSpec struct {
	TenantID       string             `json:"tenantId,omitempty"`
	NodeType       NodeType           `json:"nodeType"`
	Description    string             `json:"description,omitempty"`
	ResourceType   string             `json:"resourceType,omitempty"`
	ResourceID     string             `json:"resourceId,omitempty"`
	System         string             `json:"system,omitempty"`
	Schema         string             `json:"schema,omitempty"`
	Location       string             `json:"location,omitempty"`
	Format         string             `json:"format,omitempty"`
	Columns        []ColumnInfo       `json:"columns,omitempty"`
	Owner          string             `json:"owner,omitempty"`
	Classification DataClassification `json:"classification,omitempty"`
	Tags           []string           `json:"tags,omitempty"`
	Active         bool               `json:"active"`
}

// LineageNodeResourceStatus extends the canonical object status.
type LineageNodeResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	NodeActive   bool       `json:"nodeActive"`
	RecordCount  int64      `json:"recordCount"`
	SizeBytes    int64      `json:"sizeBytes"`
	LastModified *time.Time `json:"lastModified,omitempty"`
	QualityScore float64    `json:"qualityScore"`
}

// LineageNodeResource is the declarative resource for a LineageNode.
type LineageNodeResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   LineageNodeSpec           `json:"spec"`
	Status LineageNodeResourceStatus `json:"status"`
}

func (r *LineageNodeResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *LineageNodeResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *LineageNodeResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *LineageNodeResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *LineageNodeResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Columns) > 0 {
		cp.Spec.Columns = append([]ColumnInfo(nil), r.Spec.Columns...)
	}
	if len(r.Spec.Tags) > 0 {
		cp.Spec.Tags = append([]string(nil), r.Spec.Tags...)
	}
	return &cp
}
func (r *LineageNodeResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *LineageNodeResource) GetGeneration() int64         { return r.Generation }
func (r *LineageNodeResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
