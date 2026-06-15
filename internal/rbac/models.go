package rbac

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/rbac/models"
)

// Re-export shared primitives so existing rbac-package callers keep compiling.
type (
	RoleType     = models.RoleType
	Condition    = models.Condition
	Permission   = models.Permission
	PrincipalType = models.PrincipalType
)

const (
	RoleTypeSystem = models.RoleTypeSystem
	RoleTypeCustom = models.RoleTypeCustom
	RoleTypeTenant = models.RoleTypeTenant
)

const (
	PrincipalTypeUser    = models.PrincipalTypeUser
	PrincipalTypeService = models.PrincipalTypeService
	PrincipalTypeTeam    = models.PrincipalTypeTeam
	PrincipalTypeRole    = models.PrincipalTypeRole
)

// Re-export Resource types and constants.
type (
	RoleSpec                   = models.RoleSpec
	RoleResourceStatus         = models.RoleResourceStatus
	RoleResource               = models.RoleResource
	RoleBindingSpec            = models.RoleBindingSpec
	RoleBindingResourceStatus  = models.RoleBindingResourceStatus
	RoleBindingResource        = models.RoleBindingResource
)

const (
	RoleKind              = models.RoleKind
	RoleAPIVersion        = models.RoleAPIVersion
	RoleBindingKind       = models.RoleBindingKind
	RoleBindingAPIVersion = models.RoleBindingAPIVersion
)

// Role represents a role in the system
type Role struct {
	ID              string                 `json:"id"`
	TenantID        string                 `json:"tenantId"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Type            RoleType               `json:"type"` // SYSTEM, CUSTOM, TENANT
	Permissions     []Permission           `json:"permissions"`
	PermissionCount int                    `json:"permissionCount"`
	InheritedRoles  []string               `json:"inheritedRoles"` // Parent roles
	IsDefault       bool                   `json:"isDefault"`
	IsActive        bool                   `json:"isActive"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
	CreatedBy       string                 `json:"createdBy"`
	UsageCount      int64                  `json:"usageCount"` // How many users/services have it
	LastUsedAt      time.Time              `json:"lastUsedAt"`
	Metadata        map[string]interface{} `json:"metadata"`
	Tags            []string               `json:"tags"`
}

// RoleBinding binds role to principal
type RoleBinding struct {
	ID            string                 `json:"id"`
	TenantID      string                 `json:"tenantId"`
	RoleID        string                 `json:"roleId"`
	PrincipalType PrincipalType          `json:"principalType"` // user, service, team
	PrincipalID   string                 `json:"principalId"`
	ResourceType  string                 `json:"resourceType,omitempty"` // If scoped to resource
	ResourceID    string                 `json:"resourceId,omitempty"`
	Scope         string                 `json:"scope"`                // "global", "tenant", "resource"
	Effective     bool                   `json:"effective"`            // Role is active/effective
	Conditions    []Condition            `json:"conditions,omitempty"` // Additional conditions
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
	CreatedBy     string                 `json:"createdBy"`
	ExpiresAt     time.Time              `json:"expiresAt,omitempty"` // Temporary grant
	ReviewBy      time.Time              `json:"reviewBy,omitempty"`
	Reviewed      bool                   `json:"reviewed"`
	ReviewedBy    string                 `json:"reviewedBy,omitempty"`
	Justification string                 `json:"justification"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// PolicyBinding represents fine-grained access policy
type PolicyBinding struct {
	ID            string                 `json:"id"`
	TenantID      string                 `json:"tenantId"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	PrincipalType PrincipalType          `json:"principalType"`
	Principals    []string               `json:"principals"` // User/service IDs
	Resources     []ResourceSelector     `json:"resources"`
	Actions       []string               `json:"actions"` // Specific actions
	Effect        string                 `json:"effect"`  // "allow", "deny"
	Conditions    []Condition            `json:"conditions"`
	Priority      int                    `json:"priority"` // For conflict resolution
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
	CreatedBy     string                 `json:"createdBy"`
	IsActive      bool                   `json:"isActive"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// ResourceSelector selects resources
type ResourceSelector struct {
	Type       string                   `json:"type"`       // e.g., "database", "table", "api"
	Patterns   []string                 `json:"patterns"`   // Wildcard patterns
	Tags       []string                 `json:"tags"`       // By tag
	IDs        []string                 `json:"ids"`        // Specific IDs
	Attributes map[string][]interface{} `json:"attributes"` // By attribute
}

// AccessToken represents granted access
type AccessToken struct {
	ID            string                 `json:"id"`
	TenantID      string                 `json:"tenantId"`
	PrincipalType PrincipalType          `json:"principalType"`
	PrincipalID   string                 `json:"principalId"`
	Permissions   []PermissionGrant      `json:"permissions"`
	RoleBindings  []RoleBinding          `json:"roleBindings"`
	IssuedAt      time.Time              `json:"issuedAt"`
	ExpiresAt     time.Time              `json:"expiresAt"`
	Scopes        []string               `json:"scopes"`
	ContextData   map[string]interface{} `json:"contextData"` // Request context
	Attributes    map[string]interface{} `json:"attributes"`  // User attributes for evaluation
}

// PermissionGrant represents granted permission
type PermissionGrant struct {
	PermissionID string    `json:"permissionId"`
	Granted      bool      `json:"granted"`
	Reason       string    `json:"reason"` // Why granted
	EffectiveAt  time.Time `json:"effectiveAt"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
}

// AccessRequest represents request for access
type AccessRequest struct {
	ID              string                 `json:"id"`
	TenantID        string                 `json:"tenantId"`
	PrincipalType   PrincipalType          `json:"principalType"`
	PrincipalID     string                 `json:"principalId"`
	ResourceType    string                 `json:"resourceType"`
	ResourceID      string                 `json:"resourceId"`
	Action          string                 `json:"action"`
	Duration        int                    `json:"duration"` // Seconds, 0 = permanent
	Justification   string                 `json:"justification"`
	Status          RequestStatus          `json:"status"`
	RequestedAt     time.Time              `json:"requestedAt"`
	ExpiresAt       time.Time              `json:"expiresAt"`
	ApprovedAt      time.Time              `json:"approvedAt,omitempty"`
	ApprovedBy      string                 `json:"approvedBy,omitempty"`
	RejectedAt      time.Time              `json:"rejectedAt,omitempty"`
	RejectionReason string                 `json:"rejectionReason,omitempty"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// RequestStatus represents access request state
type RequestStatus string

const (
	RequestStatusPending   RequestStatus = "PENDING"
	RequestStatusApproved  RequestStatus = "APPROVED"
	RequestStatusRejected  RequestStatus = "REJECTED"
	RequestStatusExpired   RequestStatus = "EXPIRED"
	RequestStatusCancelled RequestStatus = "CANCELLED"
)

// AccessAuditLog tracks access decisions
type AccessAuditLog struct {
	ID              string                 `json:"id"`
	TenantID        string                 `json:"tenantId"`
	PrincipalType   PrincipalType          `json:"principalType"`
	PrincipalID     string                 `json:"principalId"`
	Resource        string                 `json:"resource"`
	Action          string                 `json:"action"`
	Decision        DecisionType           `json:"decision"`
	Reason          string                 `json:"reason"`
	Timestamp       time.Time              `json:"timestamp"`
	SourceIP        string                 `json:"sourceIP"`
	UserAgent       string                 `json:"userAgent"`
	RequestID       string                 `json:"requestID"`
	MatchedPolicies []string               `json:"matchedPolicies"` // Which policies matched
	Attributes      map[string]interface{} `json:"attributes"`
}

// DecisionType represents access decision
type DecisionType string

const (
	DecisionAllowed DecisionType = "ALLOWED"
	DecisionDenied  DecisionType = "DENIED"
	DecisionError   DecisionType = "ERROR"
)

// RBACQuery filters RBAC data
type RBACQuery struct {
	TenantID          string
	PrincipalType     PrincipalType
	PrincipalID       string
	ResourceType      string
	Action            string
	IsActive          *bool
	ExpiresAfter      time.Time
	ExpiresAtOrBefore time.Time
	Limit             int
	Offset            int
}

// PermissionCheck requests permission evaluation
type PermissionCheck struct {
	TenantID      string                 `json:"tenantId"`
	PrincipalType PrincipalType          `json:"principalType"`
	PrincipalID   string                 `json:"principalId"`
	Resource      string                 `json:"resource"`
	Action        string                 `json:"action"`
	Context       map[string]interface{} `json:"context"`    // Request context for conditions
	Attributes    map[string]interface{} `json:"attributes"` // User attributes
}

// PermissionCheckResult returns permission decision
type PermissionCheckResult struct {
	Allowed         bool              `json:"allowed"`
	Reason          string            `json:"reason"`
	MatchedPolicies []PolicyMatchInfo `json:"matchedPolicies"`
	DeniedPolicies  []PolicyMatchInfo `json:"deniedPolicies"`
}

// PolicyMatchInfo describes policy match
type PolicyMatchInfo struct {
	PolicyID       string `json:"policyId"`
	PolicyName     string `json:"policyName"`
	RoleID         string `json:"roleId,omitempty"`
	RoleName       string `json:"roleName,omitempty"`
	PermissionID   string `json:"permissionId"`
	PermissionName string `json:"permissionName"`
}

// RBACStats aggregates RBAC metrics
type RBACStats struct {
	TenantID              string
	TotalRoles            int64
	TotalPermissions      int64
	TotalPolicies         int64
	TotalRoleBindings     int64
	TotalUsers            int64
	TotalServices         int64
	AvgPermissionsPerRole float64
	AccessAllowRate       float64 // Percentage allowed
	AccessDenyRate        float64 // Percentage denied
	TopPermissions        map[string]int64
	TopRoles              map[string]int64
	Timestamp             time.Time
}

// RoleTemplate predefined role templates
type RoleTemplate struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	Category       string       `json:"category"` // "viewer", "editor", "admin"
	Permissions    []Permission `json:"permissions"`
	BaselineRole   bool         `json:"baselineRole"`   // Recommended minimum
	EnterpriseRole bool         `json:"enterpriseRole"` // Enterprise recommended
	CreatedAt      time.Time    `json:"createdAt"`
}

// PermissionAuditLog for permission changes
type PermissionAuditLog struct {
	ID               string      `json:"id"`
	TenantID         string      `json:"tenantId"`
	PermissionID     string      `json:"permissionId"`
	Operation        string      `json:"operation"` // "created", "modified", "deleted"
	OldValue         interface{} `json:"oldValue,omitempty"`
	NewValue         interface{} `json:"newValue,omitempty"`
	ChangedBy        string      `json:"changedBy"`
	ChangedAt        time.Time   `json:"changedAt"`
	Reason           string      `json:"reason,omitempty"`
	DownstreamImpact []string    `json:"downstreamImpact"` // Affected roles/policies
}

// ServiceAccount represents service principal
type ServiceAccount struct {
	ID           string                 `json:"id"`
	TenantID     string                 `json:"tenantId"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Type         string                 `json:"type"` // "system", "user", "integration"
	IsActive     bool                   `json:"isActive"`
	CreatedAt    time.Time              `json:"createdAt"`
	CreatedBy    string                 `json:"createdBy"`
	LastUsedAt   time.Time              `json:"lastUsedAt"`
	ExpiresAt    time.Time              `json:"expiresAt,omitempty"`
	RoleBindings []RoleBinding          `json:"roleBindings"`
	APIKeys      []APIKey               `json:"apiKeys"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// APIKey for service authentication
type APIKey struct {
	ID            string    `json:"id"`
	ServiceID     string    `json:"serviceId"`
	KeyHash       string    `json:"keyHash"`       // Hash of actual key
	LastFourChars string    `json:"lastFourChars"` // For display
	CreatedAt     time.Time `json:"createdAt"`
	ExpiresAt     time.Time `json:"expiresAt,omitempty"`
	LastUsedAt    time.Time `json:"lastUsedAt,omitempty"`
	IsActive      bool      `json:"isActive"`
	Permissions   []string  `json:"permissions"` // Scoped permissions
}
