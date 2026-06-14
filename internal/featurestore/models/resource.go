package models

// =====================================================
// WS-7.1 — Feature Store as declarative resources
//
// FeatureGroupResource defines a set of ML features derived from
// a datasource. The reconciler materializes features on schedule
// for both online (low-latency) and offline (batch) serving.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	FeatureGroupKind       = "FeatureGroup"
	FeatureGroupAPIVersion = "ml.axiomnizam.io/v1"
)

// --- Feature Spec ---

type FeatureSpec struct {
	Name         string `json:"name"`
	Type         string `json:"type"`         // int64, float64, string, bool, embedding
	Description  string `json:"description,omitempty"`
	Transform    string `json:"transform"`    // SQL expression or function
	DefaultValue string `json:"defaultValue,omitempty"`
	Validator    string `json:"validator,omitempty"`
}

// --- Feature Source ---

type FeatureSource struct {
	Type          string `json:"type"`          // sql, stream, request
	DataSourceRef string `json:"dataSourceRef,omitempty"`
	Query         string `json:"query,omitempty"`
	StreamRef     string `json:"streamRef,omitempty"`
}

// --- Store Configs ---

type OnlineStoreConfig struct {
	Backend     string `json:"backend"`     // redis, postgres, memory
	TTL         string `json:"ttl,omitempty"`
	MaxEntities int64  `json:"maxEntities,omitempty"`
}

type OfflineStoreConfig struct {
	Backend       string `json:"backend"`       // postgres, parquet, s3
	DataSourceRef string `json:"dataSourceRef,omitempty"`
	Table         string `json:"table,omitempty"`
}

// --- FeatureGroupSpec ---

type FeatureGroupSpec struct {
	DisplayName  string              `json:"displayName"`
	Description  string              `json:"description,omitempty"`
	Entity       string              `json:"entity"`       // Primary entity (user, product, etc.)
	EntityKey    []string            `json:"entityKey"`     // Key columns
	Features     []FeatureSpec       `json:"features"`
	Source       FeatureSource       `json:"source"`
	Schedule     string              `json:"schedule,omitempty"`     // Materialization cron
	TTL          string              `json:"ttl,omitempty"`          // Feature freshness
	OnlineStore  *OnlineStoreConfig  `json:"onlineStore,omitempty"`
	OfflineStore *OfflineStoreConfig `json:"offlineStore,omitempty"`
	Tags         []string            `json:"tags,omitempty"`
	Owner        string              `json:"owner,omitempty"`
	Enabled      bool                `json:"enabled"`
}

// --- FeatureGroupResourceStatus ---

type FeatureGroupResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	FeatureCount           int        `json:"featureCount"`
	EntityCount            int64      `json:"entityCount"`
	LastMaterializedAt     *time.Time `json:"lastMaterializedAt,omitempty"`
	MaterializationDuration string   `json:"materializationDuration,omitempty"`
	OnlineStoreStatus      string     `json:"onlineStoreStatus,omitempty"`  // ready, stale, error
	OfflineStoreStatus     string     `json:"offlineStoreStatus,omitempty"` // ready, stale, error
	OnlineServingLatencyMs float64   `json:"onlineServingLatencyMs"`
	TotalServingRequests   int64      `json:"totalServingRequests"`
	FreshnessStatus        string     `json:"freshnessStatus,omitempty"` // fresh, stale
}

// --- FeatureGroupResource ---

type FeatureGroupResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   FeatureGroupSpec           `json:"spec"`
	Status FeatureGroupResourceStatus `json:"status"`
}

func (r *FeatureGroupResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *FeatureGroupResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *FeatureGroupResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *FeatureGroupResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *FeatureGroupResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Features) > 0 {
		cp.Spec.Features = make([]FeatureSpec, len(r.Spec.Features))
		copy(cp.Spec.Features, r.Spec.Features)
	}
	if len(r.Spec.EntityKey) > 0 {
		cp.Spec.EntityKey = make([]string, len(r.Spec.EntityKey))
		copy(cp.Spec.EntityKey, r.Spec.EntityKey)
	}
	return &cp
}
func (r *FeatureGroupResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *FeatureGroupResource) GetGeneration() int64         { return r.Generation }
func (r *FeatureGroupResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
