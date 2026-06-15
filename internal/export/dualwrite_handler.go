package export

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/dualwrite"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/featureflags"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const exportDWModule = "export"

type ExportDualWriteStore = store.ResourceStore[*ExportJobResource]

func (h *ExportHandler) SetDualWriteStore(s ExportDualWriteStore) { h.dualWriteStore = s }

func (h *ExportHandler) isAuthoritative() bool {
	return h.dualWriteStore != nil && featureflags.ReconcilerAuthoritative(exportDWModule)
}

func (h *ExportHandler) buildExportResource(job *ExportJob) *ExportJobResource {
	return &ExportJobResource{
		TypeMeta:   resources.TypeMeta{APIVersion: ExportJobAPIVersion, Kind: ExportJobKind},
		ObjectMeta: resources.ObjectMeta{Name: job.ID, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		Spec:       ExportJobSpec{TenantID: job.TenantID, Description: job.Description, Format: job.Format, Source: job.Source, Query: job.Query, Filters: job.Filters, Columns: job.Columns, Compression: job.Compression, Encryption: job.Encryption, Destination: job.Destination, Schedule: job.Schedule},
		Status:     ExportJobResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: "Pending"}, ExportStatus: ExportPending},
	}
}

func (h *ExportHandler) dualWriteExport(job *ExportJob) {
	if h.dualWriteStore == nil || job == nil {
		return
	}
	resource := h.buildExportResource(job)
	resource.Status.Phase = string(job.Status)
	resource.Status.ExportStatus = job.Status
	dualwrite.Write(exportDWModule, h.dualWriteStore, resource)
}
