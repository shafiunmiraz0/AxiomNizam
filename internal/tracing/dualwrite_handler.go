package tracing

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/dualwrite"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/featureflags"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const tracingDWModule = "tracing"

type TracingDualWriteStore = store.ResourceStore[*TracingConfigResource]

func (h *TracingHandler) SetDualWriteStore(s TracingDualWriteStore) { h.dualWriteStore = s }

func (h *TracingHandler) isAuthoritative() bool {
	return h.dualWriteStore != nil && featureflags.ReconcilerAuthoritative(tracingDWModule)
}

func (h *TracingHandler) buildConfigResource(tenantID string) *TracingConfigResource {
	name := "tracing-config-default"
	if tenantID != "" {
		name = "tracing-config-" + tenantID
	}
	return &TracingConfigResource{
		TypeMeta:   resources.TypeMeta{APIVersion: TracingConfigAPIVersion, Kind: TracingConfigKind},
		ObjectMeta: resources.ObjectMeta{Name: name, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		Spec:       TracingConfigSpec{TenantID: tenantID, Enabled: true, SamplingRate: 1.0, SamplingStrategy: "always_on"},
		Status:     TracingConfigResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: "Pending"}, ConfigActive: false},
	}
}

func (h *TracingHandler) dualWriteConfig(tenantID string) {
	if h.dualWriteStore == nil {
		return
	}
	resource := h.buildConfigResource(tenantID)
	resource.Status.Phase = "Active"
	resource.Status.ConfigActive = true
	dualwrite.Write(tracingDWModule, h.dualWriteStore, resource)
}
