package models

// DataSource domain types and declarative DataSourceV1Resource.
//
// Contains the canonical Spec/Status type that flows through the
// store -> informer -> workqueue -> reconciler pipeline, along
// with the Prober interface and DataSourceReconciler.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	DataSourceKind       = "DataSource"
	DataSourceAPIVersion = "datasource.axiomnizam.io/v1"
)

// DataSourceSpec is the desired state of a DataSource.  Backend-agnostic:
// `Driver` identifies the concrete adapter (postgres, mysql, kafka, ...)
// and `Config` carries its parameters.
type DataSourceSpec struct {
	Driver      string                 `json:"driver"`
	Description string                 `json:"description,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`

	// Paused, when true, pauses probing and mutes Connected transitions.
	Paused bool `json:"paused,omitempty"`
}

// DataSourceResourceStatus extends the canonical object status.
type DataSourceResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	Connected     bool       `json:"connected"`
	LastCheckedAt *time.Time `json:"lastCheckedAt,omitempty"`
	Message       string     `json:"message,omitempty"`
}

// DataSourceV1Resource is the canonical declarative datasource.
type DataSourceV1Resource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   DataSourceSpec           `json:"spec"`
	Status DataSourceResourceStatus `json:"status"`
}

// --- resources.Resource implementation ---

func (r *DataSourceV1Resource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *DataSourceV1Resource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *DataSourceV1Resource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *DataSourceV1Resource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *DataSourceV1Resource) DeepCopy() resources.Resource { cp := *r; return &cp }

// --- reconciler.Resource implementation ---

func (r *DataSourceV1Resource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *DataSourceV1Resource) GetGeneration() int64         { return r.Generation }
func (r *DataSourceV1Resource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// Prober is the minimal contract a reconciler needs to probe a
// datasource at runtime.  Implementations live in the relevant
// backend packages (e.g. `cdc` for Kafka, `etl` for JDBC).
type Prober interface {
	Probe(ctx context.Context, driver string, config map[string]interface{}) error
}

// DataSourceReconciler reconciles DataSourceV1Resource onto an optional
// Prober.  If Prober is nil the reconciler only flips Phase based on
// Spec.Paused and stamps ObservedGeneration.
type DataSourceReconciler struct {
	store  store.ResourceStore[*DataSourceV1Resource]
	prober Prober
}

// NewDataSourceReconciler builds a reconciler.  `prober` may be nil.
func NewDataSourceReconciler(rs store.ResourceStore[*DataSourceV1Resource], p Prober) *DataSourceReconciler {
	return &DataSourceReconciler{store: rs, prober: p}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *DataSourceReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*DataSourceV1Resource)
	if !ok {
		return reconciler.ReconcileResult{Error: dsErr("datasource: reconciler received non-DataSourceV1Resource")}
	}

	now := time.Now()
	status := res.Status
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.LastCheckedAt = &now

	if res.Spec.Paused {
		status.Phase = "Paused"
		status.Connected = false
		status.Message = ""
	} else if r.prober != nil {
		if err := r.prober.Probe(ctx, res.Spec.Driver, res.Spec.Config); err != nil {
			status.Phase = "Failed"
			status.Connected = false
			status.Message = err.Error()
		} else {
			status.Phase = "Ready"
			status.Connected = true
			status.Message = ""
		}
	} else {
		status.Phase = "Ready"
		status.Connected = true
	}
	res.Status = status

	if r.store != nil {
		_ = r.store.Update(ctx, res)
	}
	return reconciler.ReconcileResult{}
}

type dsErr string

func (e dsErr) Error() string { return string(e) }

// --- Legacy handler types ---

// DataSourceResource represents a datasource on the server (legacy handler type).
type DataSourceResource struct {
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Metadata   DataSourceMetadata     `json:"metadata"`
	Spec       map[string]interface{} `json:"spec,omitempty"`
	Status     DataSourceStatus       `json:"status,omitempty"`
}

// DataSourceMetadata holds datasource metadata.
type DataSourceMetadata struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace,omitempty"`
	UID               string `json:"uid,omitempty"`
	CreationTimestamp string `json:"creationTimestamp,omitempty"`
}

// DataSourceStatus holds datasource status.
type DataSourceStatus struct {
	Connected bool   `json:"connected"`
	LastCheck string `json:"lastCheck,omitempty"`
	Message   string `json:"message,omitempty"`
}
