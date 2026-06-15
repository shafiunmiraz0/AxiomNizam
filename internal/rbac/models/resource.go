package models

// Domain types for the RBAC resource layer.
//
// RoleType, Permission, Condition and PrincipalType are the shared
// primitives that the Resource wrappers depend on.  The parent rbac
// package re-exports them via type aliases for backward compatibility.

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// ── Shared primitives ────────────────────────────────────────────

// RoleType represents role classification
type RoleType string

const (
	RoleTypeSystem RoleType = "SYSTEM" // Built-in
	RoleTypeCustom RoleType = "CUSTOM" // User-defined
	RoleTypeTenant RoleType = "TENANT" // Tenant-scoped
)

// Condition represents when permission applies
type Condition struct {
	Type     string      `json:"type"`     // "field", "attribute", "time", "ip", "mfa"
	Key      string      `json:"key"`      // Field name
	Value    interface{} `json:"value"`    // Condition value
	Operator string      `json:"operator"` // "eq", "ne", "lt", "gt", "in", "matches"
}

// Permission represents capability/action
type Permission struct {
	ID           string                 `json:"id"`
	TenantID     string                 `json:"tenantId,omitempty"` // Empty = system permission
	Name         string                 `json:"name"`               // e.g., "resources.create"
	Description  string                 `json:"description"`
	Resource     string                 `json:"resource"`             // What it applies to
	Action       string                 `json:"action"`               // create, read, update, delete, execute
	Scope        string                 `json:"scope"`                // "global", "tenant", "own"
	Conditions   []Condition            `json:"conditions,omitempty"` // When permission applies
	CreatedAt    time.Time              `json:"createdAt"`
	IsDeprecated bool                   `json:"isDeprecated"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// PrincipalType represents type of principal
type PrincipalType string

const (
	PrincipalTypeUser    PrincipalType = "USER"
	PrincipalTypeService PrincipalType = "SERVICE"
	PrincipalTypeTeam    PrincipalType = "TEAM"
	PrincipalTypeRole    PrincipalType = "ROLE"
)

// ── Kind / APIVersion constants ──────────────────────────────────

const (
	RoleKind              = "Role"
	RoleAPIVersion        = "rbac.axiomnizam.io/v1"
	RoleBindingKind       = "RoleBinding"
	RoleBindingAPIVersion = "rbac.axiomnizam.io/v1"
)

// ── RoleResource ─────────────────────────────────────────────────

// RoleSpec is the desired state of an RBAC role.
type RoleSpec struct {
	TenantID       string       `json:"tenantId,omitempty"`
	Description    string       `json:"description,omitempty"`
	Type           RoleType     `json:"type"`
	Permissions    []Permission `json:"permissions"`
	InheritedRoles []string     `json:"inheritedRoles,omitempty"`
	IsDefault      bool         `json:"isDefault,omitempty"`
	Active         bool         `json:"active"`
	Tags           []string     `json:"tags,omitempty"`
}

// RoleResourceStatus extends the canonical object status.
type RoleResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	RoleActive      bool  `json:"roleActive"`
	PermissionCount int   `json:"permissionCount"`
	UsageCount      int64 `json:"usageCount"`
}

// RoleResource is the declarative resource for an RBAC Role.
type RoleResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   RoleSpec           `json:"spec"`
	Status RoleResourceStatus `json:"status"`
}

func (r *RoleResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *RoleResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *RoleResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *RoleResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *RoleResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Permissions) > 0 {
		cp.Spec.Permissions = append([]Permission(nil), r.Spec.Permissions...)
	}
	if len(r.Spec.Tags) > 0 {
		cp.Spec.Tags = append([]string(nil), r.Spec.Tags...)
	}
	return &cp
}
func (r *RoleResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *RoleResource) GetGeneration() int64         { return r.Generation }
func (r *RoleResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// ── RoleBindingResource ──────────────────────────────────────────

// RoleBindingSpec is the desired state of an RBAC role binding.
type RoleBindingSpec struct {
	TenantID      string        `json:"tenantId,omitempty"`
	RoleID        string        `json:"roleId"`
	PrincipalType PrincipalType `json:"principalType"`
	PrincipalID   string        `json:"principalId"`
	Scope         string        `json:"scope,omitempty"`
	Justification string        `json:"justification,omitempty"`
	Active        bool          `json:"active"`
}

// RoleBindingResourceStatus extends the canonical object status.
type RoleBindingResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	BindingActive bool `json:"bindingActive"`
}

// RoleBindingResource is the declarative resource for an RBAC RoleBinding.
type RoleBindingResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   RoleBindingSpec           `json:"spec"`
	Status RoleBindingResourceStatus `json:"status"`
}

func (r *RoleBindingResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *RoleBindingResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *RoleBindingResource) GetStatus() *resources.ObjectStatus {
	return &r.Status.ObjectStatus
}
func (r *RoleBindingResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *RoleBindingResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *RoleBindingResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *RoleBindingResource) GetGeneration() int64         { return r.Generation }
func (r *RoleBindingResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
