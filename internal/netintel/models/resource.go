package models

// NetIntel resource domain types.

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	ConfigKind       = "NetIntelConfig"
	ConfigAPIVersion = "netintel.axiomnizam.io/v1"
)

type ConfigSpec struct {
	TenantID string `json:"tenantId,omitempty"`
	Enabled  bool   `json:"enabled"`
}

type ConfigResourceStatus struct {
	resources.ObjectStatus `json:",inline"`
	ConfigActive           bool `json:"configActive"`
}

type ConfigResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec                 ConfigSpec           `json:"spec"`
	Status               ConfigResourceStatus `json:"status"`
}

func (r *ConfigResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *ConfigResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *ConfigResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *ConfigResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *ConfigResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *ConfigResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *ConfigResource) GetGeneration() int64         { return r.Generation }
func (r *ConfigResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
