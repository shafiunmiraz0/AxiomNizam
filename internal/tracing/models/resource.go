package models

// =====================================================
// P2 resource-ification — Tracing.
//
// TracingConfigResource wraps the imperative TracingConfig so a
// controller can reconcile tracing configuration as a first-class
// platform resource.
// =====================================================

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	TracingConfigKind       = "TracingConfig"
	TracingConfigAPIVersion = "tracing.axiomnizam.io/v1"
)

// TracingConfigSpec is the desired state of a tracing configuration.
type TracingConfigSpec struct {
	TenantID         string            `json:"tenantId,omitempty"`
	ExporterType     string            `json:"exporterType,omitempty"`
	Endpoint         string            `json:"endpoint,omitempty"`
	SamplingRate     float64           `json:"samplingRate,omitempty"`
	SamplingStrategy string            `json:"samplingStrategy,omitempty"`
	MaxTraceSize     int               `json:"maxTraceSize,omitempty"`
	MaxSpansPerTrace int               `json:"maxSpansPerTrace,omitempty"`
	BatchSize        int               `json:"batchSize,omitempty"`
	ServiceName      string            `json:"serviceName,omitempty"`
	Environment      string            `json:"environment,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Enabled          bool              `json:"enabled"`
}

// TracingConfigResourceStatus extends the canonical object status.
type TracingConfigResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	ConfigActive bool  `json:"configActive"`
	TraceCount   int64 `json:"traceCount"`
	SpanCount    int64 `json:"spanCount"`
}

// TracingConfigResource is the declarative resource for a TracingConfig.
type TracingConfigResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   TracingConfigSpec           `json:"spec"`
	Status TracingConfigResourceStatus `json:"status"`
}

func (r *TracingConfigResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *TracingConfigResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *TracingConfigResource) GetStatus() *resources.ObjectStatus {
	return &r.Status.ObjectStatus
}
func (r *TracingConfigResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *TracingConfigResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *TracingConfigResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *TracingConfigResource) GetGeneration() int64         { return r.Generation }
func (r *TracingConfigResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
