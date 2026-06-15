package bulk

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/dualwrite"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/featureflags"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const bulkDWModule = "bulk"

type BulkDualWriteStore = store.ResourceStore[*BulkOperationResource]

func (h *BulkHandler) SetDualWriteStore(s BulkDualWriteStore) { h.dualWriteStore = s }

func (h *BulkHandler) isAuthoritative() bool {
	return h.dualWriteStore != nil && featureflags.ReconcilerAuthoritative(bulkDWModule)
}

func (h *BulkHandler) buildOperationResource(op *BulkOperation) *BulkOperationResource {
	return &BulkOperationResource{
		TypeMeta:   resources.TypeMeta{APIVersion: BulkOperationAPIVersion, Kind: BulkOperationKind},
		ObjectMeta: resources.ObjectMeta{Name: op.ID, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		Spec:       BulkOperationSpec{TenantID: op.TenantID, Type: op.Type, Items: op.Items, Options: op.Options, Timeout: op.Timeout, Atomic: op.Atomic},
		Status:     BulkOperationResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: "Pending"}, OperationStatus: BulkOpPending, TotalItems: int64(len(op.Items))},
	}
}

func (h *BulkHandler) dualWriteOperation(op *BulkOperation) {
	if h.dualWriteStore == nil || op == nil {
		return
	}
	resource := h.buildOperationResource(op)
	resource.Status.Phase = string(op.Status)
	resource.Status.OperationStatus = op.Status
	dualwrite.Write(bulkDWModule, h.dualWriteStore, resource)
}
