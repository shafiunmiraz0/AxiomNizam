package models

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	RuleKind       = "TransformRule"
	RuleAPIVersion = "transform.axiomnizam.io/v1"
)

type RuleSpec struct {
	RuleName    string                 `json:"ruleName"`
	Description string                 `json:"description,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Enabled     bool                   `json:"enabled"`
}

type RuleResourceStatus struct {
	resources.ObjectStatus `json:",inline"`
	RuleActive             bool `json:"ruleActive"`
}

type RuleResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec                 RuleSpec           `json:"spec"`
	Status               RuleResourceStatus `json:"status"`
}

func (r *RuleResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *RuleResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *RuleResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *RuleResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *RuleResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *RuleResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *RuleResource) GetGeneration() int64         { return r.Generation }
func (r *RuleResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
