package models

// =====================================================
// CDC Pipeline domain types and declarative resource
//
// Contains the CDC pipeline resource envelope (Spec / Status /
// Conditions) plus the supporting domain types (CDCSource, CDCSink,
// CDCFilters, PipelineStatus) that the resource structs reference.
//
// The parent cdc package re-exports all types via aliases so existing
// code continues to compile unchanged.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// --- Domain types used by the resource structs ---

// PipelineStatus represents the lifecycle status of a CDC pipeline.
type PipelineStatus string

const (
	CDCActive  PipelineStatus = "active"
	CDCPaused  PipelineStatus = "paused"
	CDCStopped PipelineStatus = "stopped"
	CDCFailed  PipelineStatus = "failed"
	CDCCreated PipelineStatus = "created"
)

// CDCSource describes the upstream data source for a CDC pipeline.
type CDCSource struct {
	Type      string                 `json:"type"`      // mysql_binlog, pg_wal, mongo_oplog, polling, api_webhook
	Connector string                 `json:"connector"` // mysql, postgres, mongodb, etc.
	Config    map[string]interface{} `json:"config"`
	Tables    []string               `json:"tables,omitempty"`
}

// CDCSink describes the downstream data sink for a CDC pipeline.
type CDCSink struct {
	Type      string                 `json:"type"` // kafka, webhook, api, database, elasticsearch, s3
	Connector string                 `json:"connector"`
	Config    map[string]interface{} `json:"config"`
	BatchSize int                    `json:"batch_size,omitempty"`
}

// CDCFilters defines table, operation, and schema filters for a CDC pipeline.
type CDCFilters struct {
	Tables     []string `json:"tables,omitempty"`
	Operations []string `json:"operations,omitempty"` // INSERT, UPDATE, DELETE
	Schemas    []string `json:"schemas,omitempty"`
	Exclude    []string `json:"exclude,omitempty"`
}

// --- Resource constants ---

const (
	CDCPipelineKind       = "CDCPipeline"
	CDCPipelineAPIVersion = "cdc.axiomnizam.io/v1"
)

// --- Resource types ---

// CDCPipelineSpec is the *desired* state of a CDC Pipeline.
type CDCPipelineSpec struct {
	Description string                 `json:"description,omitempty"`
	Source      CDCSource              `json:"source"`
	Sink        CDCSink                `json:"sink"`
	Filters     CDCFilters             `json:"filters"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Tags        []string               `json:"tags,omitempty"`

	// Paused asks the controller to keep the underlying CDCPipeline in
	// the `paused` state.
	Paused bool `json:"paused,omitempty"`
}

// CDCPipelineResourceStatus extends canonical status with CDC telemetry.
type CDCPipelineResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	CDCStatus   PipelineStatus `json:"cdcStatus,omitempty"`
	EventCount  int64          `json:"eventCount"`
	ErrorCount  int64          `json:"errorCount"`
	LastEventAt *time.Time     `json:"lastEventAt,omitempty"`
	Lag         string         `json:"lag,omitempty"`
}

// CDCPipelineResource is the declarative resource for a CDC pipeline.
type CDCPipelineResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   CDCPipelineSpec           `json:"spec"`
	Status CDCPipelineResourceStatus `json:"status"`
}

// --- resources.Resource ---

func (c *CDCPipelineResource) GetObjectMeta() *resources.ObjectMeta { return &c.ObjectMeta }
func (c *CDCPipelineResource) GetTypeMeta() *resources.TypeMeta     { return &c.TypeMeta }
func (c *CDCPipelineResource) GetStatus() *resources.ObjectStatus   { return &c.Status.ObjectStatus }
func (c *CDCPipelineResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		c.Status.ObjectStatus = *s
	}
}
func (c *CDCPipelineResource) DeepCopy() resources.Resource {
	cp := *c
	return &cp
}

// --- reconciler.Resource ---

func (c *CDCPipelineResource) GetKey() string {
	if c.Namespace == "" {
		return c.Name
	}
	return c.Namespace + "/" + c.Name
}
func (c *CDCPipelineResource) GetGeneration() int64         { return c.Generation }
func (c *CDCPipelineResource) GetObservedGeneration() int64 { return c.Status.ObservedGeneration }
