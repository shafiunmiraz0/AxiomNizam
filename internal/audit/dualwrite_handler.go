package audit

import (
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/dualwrite"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/featureflags"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const auditDWModule = "audit"

type AuditDualWriteStore = store.ResourceStore[*AuditPolicyResource]

func (h *AuditHandler) SetDualWriteStore(s AuditDualWriteStore) { h.dualWriteStore = s }

func (h *AuditHandler) isAuthoritative() bool {
	return h.dualWriteStore != nil && featureflags.ReconcilerAuthoritative(auditDWModule)
}

func (h *AuditHandler) buildPolicyResource(tenantID string) *AuditPolicyResource {
	name := fmt.Sprintf("audit-policy-%s", tenantID)
	return &AuditPolicyResource{
		TypeMeta:   resources.TypeMeta{APIVersion: AuditPolicyAPIVersion, Kind: AuditPolicyKind},
		ObjectMeta: resources.ObjectMeta{Name: name, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		Spec:       AuditPolicySpec{TenantID: tenantID, Enabled: true, ComplianceMode: true, AsyncWrite: true},
		Status:     AuditPolicyResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: "Pending"}, PolicyActive: false},
	}
}

func (h *AuditHandler) dualWritePolicy(tenantID string) {
	if h.dualWriteStore == nil {
		return
	}
	name := fmt.Sprintf("audit-policy-%s", tenantID)
	resource := &AuditPolicyResource{
		TypeMeta:   resources.TypeMeta{APIVersion: AuditPolicyAPIVersion, Kind: AuditPolicyKind},
		ObjectMeta: resources.ObjectMeta{Name: name, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		Spec:       AuditPolicySpec{TenantID: tenantID, Enabled: true, ComplianceMode: true, AsyncWrite: true},
		Status:     AuditPolicyResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: "Active"}, PolicyActive: true},
	}
	dualwrite.Write(auditDWModule, h.dualWriteStore, resource)
}
