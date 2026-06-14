package rbac

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/dualwrite"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/featureflags"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const rbacDWModule = "rbac"

type RBACRoleDualWriteStore = store.ResourceStore[*RoleResource]

func (h *RBACHandler) SetRoleDualWriteStore(s RBACRoleDualWriteStore) { h.roleDualWriteStore = s }

func (h *RBACHandler) isAuthoritative() bool {
	return h.roleDualWriteStore != nil && featureflags.ReconcilerAuthoritative(rbacDWModule)
}

func (h *RBACHandler) buildRoleResource(role *Role) *RoleResource {
	return &RoleResource{
		TypeMeta:   resources.TypeMeta{APIVersion: RoleAPIVersion, Kind: RoleKind},
		ObjectMeta: resources.ObjectMeta{Name: role.ID, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		Spec:       RoleSpec{TenantID: role.TenantID, Description: role.Description, Type: role.Type, Permissions: role.Permissions, IsDefault: role.IsDefault, Active: role.IsActive, Tags: role.Tags},
		Status:     RoleResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: "Active"}, RoleActive: role.IsActive, PermissionCount: len(role.Permissions)},
	}
}

func (h *RBACHandler) dualWriteRole(role *Role) {
	if h.roleDualWriteStore == nil || role == nil {
		return
	}
	dualwrite.Write(rbacDWModule, h.roleDualWriteStore, h.buildRoleResource(role))
}
