package tenant

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/dualwrite"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/featureflags"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const tenantDWModule = "tenant"

type TenantDualWriteStore = store.ResourceStore[*TenantV1Resource]

func (h *TenantHandler) SetDualWriteStore(s TenantDualWriteStore) { h.dualWriteStore = s }

func (h *TenantHandler) isAuthoritative() bool {
	return h.dualWriteStore != nil && featureflags.ReconcilerAuthoritative(tenantDWModule)
}

func (h *TenantHandler) buildTenantResource(t *Tenant) *TenantV1Resource {
	return &TenantV1Resource{
		TypeMeta:   resources.TypeMeta{APIVersion: TenantAPIVersion, Kind: TenantKind},
		ObjectMeta: resources.ObjectMeta{Name: t.ID, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		Spec:       TenantSpec{DisplayName: t.Name, Description: t.Description, Owner: t.Owner, Tier: t.Tier},
		Status:     TenantResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: "Pending"}},
	}
}

func (h *TenantHandler) dualWriteTenant(t *Tenant) {
	if h.dualWriteStore == nil || t == nil {
		return
	}
	resource := h.buildTenantResource(t)
	resource.Status.Phase = string(t.Status)
	resource.Status.TenantStatus = t.Status
	dualwrite.Write(tenantDWModule, h.dualWriteStore, resource)
}
