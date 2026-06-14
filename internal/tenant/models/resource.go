package models

// =====================================================
// P2 resource-ification — Tenant.
//
// TenantV1Resource (named to avoid colliding with the legacy
// `TenantResource` in models.go, which is a per-tenant generic data
// container, not a tenant-lifecycle resource) is the declarative
// envelope around the imperative `Tenant` struct.  A dedicated
// reconciler drives tenant lifecycle (Active / Suspended / Archived /
// Deleting) through `TenantManager`.
// =====================================================

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	TenantKind       = "Tenant"
	TenantAPIVersion = "tenant.axiomnizam.io/v1"
)

// TenantStatus represents tenant lifecycle status.
type TenantStatus string

const (
	TenantActive    TenantStatus = "ACTIVE"
	TenantSuspended TenantStatus = "SUSPENDED"
	TenantArchived  TenantStatus = "ARCHIVED"
	TenantDeleting  TenantStatus = "DELETING"
)

// TenantTier defines resource limits.
type TenantTier string

const (
	TierFree       TenantTier = "FREE"
	TierPro        TenantTier = "PRO"
	TierEnterprise TenantTier = "ENTERPRISE"
)

// TenantIsolation defines isolation level.
type TenantIsolation string

const (
	IsolationLogical  TenantIsolation = "LOGICAL"  // Shared database, filtered by tenant_id
	IsolationDatabase TenantIsolation = "DATABASE" // Separate database per tenant
	IsolationHybrid   TenantIsolation = "HYBRID"   // Critical data isolated, other shared
)

// TenantSpec is the desired state of a tenant.
type TenantSpec struct {
	DisplayName    string            `json:"displayName"`
	Description    string            `json:"description,omitempty"`
	Owner          string            `json:"owner"`
	Plan           string            `json:"plan,omitempty"`
	Tier           TenantTier        `json:"tier,omitempty"`
	IsolationLevel TenantIsolation   `json:"isolationLevel,omitempty"`
	DataLocation   string            `json:"dataLocation,omitempty"`
	Features       map[string]bool   `json:"features,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`

	// Suspended, when true, asks the controller to transition the
	// tenant to Suspended state.  Clearing it flips back to Active.
	Suspended bool `json:"suspended,omitempty"`
}

// TenantResourceStatus extends the canonical object status.
type TenantResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	// TenantStatus mirrors `Tenant.Status` (ACTIVE/SUSPENDED/...).
	TenantStatus TenantStatus `json:"tenantStatus,omitempty"`

	// SuspendedAt / DeletedAt are controller-recorded transitions.
	SuspendedAt *time.Time `json:"suspendedAt,omitempty"`
	DeletedAt   *time.Time `json:"deletedAt,omitempty"`
}

// TenantV1Resource is the declarative resource for a Tenant.
type TenantV1Resource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   TenantSpec           `json:"spec"`
	Status TenantResourceStatus `json:"status"`
}

// --- resources.Resource implementation ---

func (r *TenantV1Resource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *TenantV1Resource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *TenantV1Resource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *TenantV1Resource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *TenantV1Resource) DeepCopy() resources.Resource { cp := *r; return &cp }

// --- reconciler.Resource implementation ---

func (r *TenantV1Resource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *TenantV1Resource) GetGeneration() int64         { return r.Generation }
func (r *TenantV1Resource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// TenantReconciler reconciles TenantV1Resource objects.
//
// Skeleton only: it computes the target TenantStatus from Spec.Suspended
// and records it on the status.  A production reconciler also drives
// quota setup, DB provisioning, and soft-delete timing through
// TenantManager.
type TenantReconciler struct {
	store store.ResourceStore[*TenantV1Resource]
}

// NewTenantReconciler builds a reconciler.
func NewTenantReconciler(rs store.ResourceStore[*TenantV1Resource]) *TenantReconciler {
	return &TenantReconciler{store: rs}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *TenantReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*TenantV1Resource)
	if !ok {
		return reconciler.ReconcileResult{Error: tenantErr("tenant: reconciler received non-TenantV1Resource")}
	}

	now := time.Now()
	status := res.Status
	target := TenantActive
	if res.Spec.Suspended {
		target = TenantSuspended
		if status.SuspendedAt == nil {
			status.SuspendedAt = &now
		}
	} else {
		status.SuspendedAt = nil
	}
	if res.DeletedAt != nil && status.DeletedAt == nil {
		status.DeletedAt = res.DeletedAt
		target = TenantDeleting
	}

	status.TenantStatus = target
	status.Phase = string(target)
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	res.Status = status

	if r.store != nil {
		_ = r.store.Update(ctx, res)
	}
	return reconciler.ReconcileResult{}
}

type tenantErr string

func (e tenantErr) Error() string { return string(e) }

// =====================================================
// Domain types (moved from models.go)
// =====================================================

// Tenant represents organization/workspace
type Tenant struct {
	ID             string            `json:"id" db:"id"`
	Name           string            `json:"name" db:"name"`
	DisplayName    string            `json:"displayName" db:"display_name"`
	Description    string            `json:"description" db:"description"`
	Owner          string            `json:"owner" db:"owner"`   // User ID
	Status         TenantStatus      `json:"status" db:"status"` // Active/Suspended/Archived
	Plan           string            `json:"plan" db:"plan"`     // Free/Pro/Enterprise
	Tier           TenantTier        `json:"tier" db:"tier"`     // Limits tier
	CreatedAt      time.Time         `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time         `json:"updatedAt" db:"updated_at"`
	SuspendedAt    *time.Time        `json:"suspendedAt" db:"suspended_at"`
	DeletedAt      *time.Time        `json:"deletedAt" db:"deleted_at"`           // Soft delete
	Metadata       map[string]string `json:"metadata" db:"-"`                     // Custom metadata
	Features       map[string]bool   `json:"features" db:"-"`                     // Feature flags
	IsolationLevel TenantIsolation   `json:"isolationLevel" db:"isolation_level"` // Data isolation strategy
	DataLocation   string            `json:"dataLocation" db:"data_location"`     // Geographic region
}

// TenantQuota represents resource limits
type TenantQuota struct {
	TenantID      string    `json:"tenantId" db:"tenant_id"`
	MaxUsers      int       `json:"maxUsers" db:"max_users"`
	MaxResources  int       `json:"maxResources" db:"max_resources"`
	MaxQueries    int64     `json:"maxQueries" db:"max_queries"`       // Per month
	MaxStorage    int64     `json:"maxStorage" db:"max_storage"`       // In bytes
	MaxAPIcalls   int64     `json:"maxApiCalls" db:"max_api_calls"`    // Per day
	MaxConcurrent int       `json:"maxConcurrent" db:"max_concurrent"` // Concurrent requests
	QueryTimeout  int       `json:"queryTimeout" db:"query_timeout"`   // Seconds
	UsedUsers     int       `json:"usedUsers" db:"used_users"`
	UsedResources int       `json:"usedResources" db:"used_resources"`
	UsedQueries   int64     `json:"usedQueries" db:"used_queries"`
	UsedStorage   int64     `json:"usedStorage" db:"used_storage"`
	UsedAPICalls  int64     `json:"usedApiCalls" db:"used_api_calls"`
	ResetDate     time.Time `json:"resetDate" db:"reset_date"` // When quota resets
}

// TenantMember represents user membership in tenant
type TenantMember struct {
	ID          string       `json:"id" db:"id"`
	TenantID    string       `json:"tenantId" db:"tenant_id"`
	UserID      string       `json:"userId" db:"user_id"`
	Username    string       `json:"username" db:"username"`
	Email       string       `json:"email" db:"email"`
	Role        MemberRole   `json:"role" db:"role"`     // Owner/Admin/Member/Viewer
	Status      MemberStatus `json:"status" db:"status"` // Active/Invited/Suspended
	JoinedAt    time.Time    `json:"joinedAt" db:"joined_at"`
	InvitedAt   *time.Time   `json:"invitedAt" db:"invited_at"`
	Permissions []string     `json:"permissions" db:"-"` // Custom permissions
}

// MemberRole in tenant
type MemberRole string

const (
	RoleOwner  MemberRole = "OWNER"
	RoleAdmin  MemberRole = "ADMIN"
	RoleMember MemberRole = "MEMBER"
	RoleViewer MemberRole = "VIEWER"
)

// MemberStatus in tenant
type MemberStatus string

const (
	MemberActive      MemberStatus = "ACTIVE"
	MemberInvited     MemberStatus = "INVITED"
	MemberSuspended   MemberStatus = "SUSPENDED"
	MemberDeactivated MemberStatus = "DEACTIVATED"
)

// TenantResource represents resource scoped to tenant
type TenantResource struct {
	ID        string                 `json:"id"`
	TenantID  string                 `json:"tenantId"` // Required for all resources
	Kind      string                 `json:"kind"`     // Resource type
	Name      string                 `json:"name"`
	Data      map[string]interface{} `json:"data"`
	CreatedAt time.Time              `json:"createdAt"`
	UpdatedAt time.Time              `json:"updatedAt"`
}

// TenantContext passed through requests
type TenantContext struct {
	TenantID       string
	UserID         string
	Role           MemberRole
	Quota          *TenantQuota
	IsolationLevel TenantIsolation
}

// TenantConfig configures multi-tenancy behavior
type TenantConfig struct {
	Enabled           bool
	IsolationLevel    TenantIsolation
	AllowSubdomains   bool // api-{tenant}.example.com
	AllowCustomDomain bool // custom.example.com
	QuotaEnforcement  bool
	SoftDeleteTenants bool
	DataEncryption    bool // Per-tenant encryption key
}
