package models

// =====================================================
// WS-5.1 -- Federated Query and Virtualization
//
// VirtualTableResource defines a cross-database virtual table that
// maps columns from one or more real datasources. The reconciler
// maintains metadata freshness and validates source availability.
//
// FederatedQueryResource represents an ad-hoc cross-source query
// submitted for execution.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// --- Virtual Source ---

type VirtualSource struct {
	Alias         string `json:"alias"`
	DataSourceRef string `json:"dataSourceRef"`
	Database      string `json:"database,omitempty"`
	Schema        string `json:"schema,omitempty"`
	Table         string `json:"table"`
}

// --- Join Condition ---

type JoinCondition struct {
	LeftAlias   string `json:"leftAlias"`
	LeftColumn  string `json:"leftColumn"`
	RightAlias  string `json:"rightAlias"`
	RightColumn string `json:"rightColumn"`
	JoinType    string `json:"joinType"` // inner, left, right, full
}

// --- Virtual Column ---

type VirtualColumn struct {
	Name         string `json:"name"`
	SourceAlias  string `json:"sourceAlias"`
	SourceColumn string `json:"sourceColumn"`
	Type         string `json:"type,omitempty"`
	Description  string `json:"description,omitempty"`
}

// --- Default Filter ---

type DefaultFilter struct {
	Column   string `json:"column"`
	Operator string `json:"operator"` // eq, ne, gt, lt, gte, lte, in, like
	Value    string `json:"value"`
}

// --- Cache Policy ---

type CachePolicy struct {
	Enabled bool   `json:"enabled"`
	TTL     string `json:"ttl,omitempty"` // "5m", "1h"
	MaxRows int64  `json:"maxRows,omitempty"`
}

// --- VirtualTableSpec ---

type VirtualTableSpec struct {
	DisplayName     string          `json:"displayName"`
	Description     string          `json:"description,omitempty"`
	Sources         []VirtualSource `json:"sources"`
	JoinConditions  []JoinCondition `json:"joinConditions,omitempty"`
	Columns         []VirtualColumn `json:"columns"`
	Filters         []DefaultFilter `json:"filters,omitempty"`
	CachePolicy     *CachePolicy    `json:"cachePolicy,omitempty"`
	RefreshSchedule string          `json:"refreshSchedule,omitempty"`
	Materialized    bool            `json:"materialized"`
	Owner           string          `json:"owner,omitempty"`
}

// --- VirtualTableResourceStatus ---

type VirtualTableResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	SourceCount      int        `json:"sourceCount"`
	ColumnCount      int        `json:"columnCount"`
	SourcesHealthy   int        `json:"sourcesHealthy"`
	SourcesUnhealthy int       `json:"sourcesUnhealthy"`
	LastValidatedAt  *time.Time `json:"lastValidatedAt,omitempty"`
	EstimatedRows    int64      `json:"estimatedRows,omitempty"`
	CacheHits        int64      `json:"cacheHits"`
	CacheMisses      int64      `json:"cacheMisses"`
	QueryCount       int64      `json:"queryCount"`
	AvgLatencyMs     float64    `json:"avgLatencyMs"`
}

// --- VirtualTableResource ---

type VirtualTableResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   VirtualTableSpec           `json:"spec"`
	Status VirtualTableResourceStatus `json:"status"`
}

func (r *VirtualTableResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *VirtualTableResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *VirtualTableResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *VirtualTableResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *VirtualTableResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Sources) > 0 {
		cp.Spec.Sources = make([]VirtualSource, len(r.Spec.Sources))
		copy(cp.Spec.Sources, r.Spec.Sources)
	}
	if len(r.Spec.Columns) > 0 {
		cp.Spec.Columns = make([]VirtualColumn, len(r.Spec.Columns))
		copy(cp.Spec.Columns, r.Spec.Columns)
	}
	return &cp
}
func (r *VirtualTableResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *VirtualTableResource) GetGeneration() int64         { return r.Generation }
func (r *VirtualTableResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// =====================================================
// FederatedQueryResource -- ad-hoc cross-source query
// =====================================================

type QueryFormat string

const (
	QueryFormatJSON  QueryFormat = "json"
	QueryFormatCSV   QueryFormat = "csv"
	QueryFormatArrow QueryFormat = "arrow"
)

type FederatedQuerySpec struct {
	SQL      string      `json:"sql"`
	Timeout  string      `json:"timeout,omitempty"` // "30s"
	MaxRows  int64       `json:"maxRows,omitempty"`
	Format   QueryFormat `json:"format,omitempty"`
	TenantID string      `json:"tenantId,omitempty"`
	Explain  bool        `json:"explain,omitempty"` // Return plan only
}

type QueryPlanNode struct {
	Type          string          `json:"type"`                    // remote_scan, merge_join, hash_join, filter, project, sort
	DataSource    string          `json:"datasource,omitempty"`
	Table         string          `json:"table,omitempty"`
	EstimatedRows int64           `json:"estimatedRows,omitempty"`
	EstimatedCost float64         `json:"estimatedCost,omitempty"`
	Children      []QueryPlanNode `json:"children,omitempty"`
}

type FederatedQueryResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	QueryStatus    string         `json:"queryStatus"` // pending, running, completed, failed, cancelled
	Plan           *QueryPlanNode `json:"plan,omitempty"`
	RowsReturned   int64          `json:"rowsReturned"`
	BytesScanned   int64          `json:"bytesScanned"`
	DurationMs     int64          `json:"durationMs"`
	StartedAt      *time.Time     `json:"startedAt,omitempty"`
	CompletedAt    *time.Time     `json:"completedAt,omitempty"`
	ErrorMessage   string         `json:"errorMessage,omitempty"`
	SourcesQueried []string       `json:"sourcesQueried,omitempty"`
}

type FederatedQueryResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   FederatedQuerySpec           `json:"spec"`
	Status FederatedQueryResourceStatus `json:"status"`
}

func (r *FederatedQueryResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *FederatedQueryResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *FederatedQueryResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *FederatedQueryResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *FederatedQueryResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *FederatedQueryResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *FederatedQueryResource) GetGeneration() int64         { return r.Generation }
func (r *FederatedQueryResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
