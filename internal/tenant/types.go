package tenant

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/tenant/models"
)

// Re-exported resource types from models/.
type TenantV1Resource = models.TenantV1Resource
type TenantSpec = models.TenantSpec
type TenantResourceStatus = models.TenantResourceStatus
type TenantReconciler = models.TenantReconciler

// Re-exported domain types used by TenantSpec / TenantResourceStatus.
type TenantStatus = models.TenantStatus
type TenantTier = models.TenantTier
type TenantIsolation = models.TenantIsolation

// Re-exported constants.
const TenantKind = models.TenantKind
const TenantAPIVersion = models.TenantAPIVersion

// Re-exported status constants.
const (
	TenantActive    = models.TenantActive
	TenantSuspended = models.TenantSuspended
	TenantArchived  = models.TenantArchived
	TenantDeleting  = models.TenantDeleting
)

// Re-exported tier constants.
const (
	TierFree       = models.TierFree
	TierPro        = models.TierPro
	TierEnterprise = models.TierEnterprise
)

// Re-exported isolation constants.
const (
	IsolationLogical  = models.IsolationLogical
	IsolationDatabase = models.IsolationDatabase
	IsolationHybrid   = models.IsolationHybrid
)

// --- Domain types (moved from models.go) ---

type Tenant = models.Tenant
type TenantQuota = models.TenantQuota
type TenantMember = models.TenantMember
type MemberRole = models.MemberRole
type MemberStatus = models.MemberStatus
type TenantResource = models.TenantResource
type TenantContext = models.TenantContext
type TenantConfig = models.TenantConfig

// Re-exported member role constants.
const (
	RoleOwner  = models.RoleOwner
	RoleAdmin  = models.RoleAdmin
	RoleMember = models.RoleMember
	RoleViewer = models.RoleViewer
)

// Re-exported member status constants.
const (
	MemberActive      = models.MemberActive
	MemberInvited     = models.MemberInvited
	MemberSuspended   = models.MemberSuspended
	MemberDeactivated = models.MemberDeactivated
)

// Re-exported constructor.
func NewTenantReconciler(rs store.ResourceStore[*TenantV1Resource]) *TenantReconciler {
	return models.NewTenantReconciler(rs)
}
