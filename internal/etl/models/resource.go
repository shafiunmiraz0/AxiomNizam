package models

// =====================================================
// P1.1 — ETL Pipeline as a declarative resource
//
// Historically `Pipeline` was an imperative config object managed by
// `Engine`.  For the declarative control-plane we wrap it in a proper
// `resources.Resource` (Spec/Status/Conditions/ObservedGeneration) so a
// dedicated controller can reconcile it like any other platform resource.
//
// The in-memory `Pipeline` type is preserved untouched (it is still the
// wire/exec shape used by the existing Engine).  `PipelineResource` is the
// declarative envelope and is what the API server / controller operate on.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// PipelineStatus represents the lifecycle state of an ETL pipeline.
type PipelineStatus string

const (
	PipelineCreated PipelineStatus = "created"
	PipelineRunning PipelineStatus = "running"
	PipelinePaused  PipelineStatus = "paused"
	PipelineSuccess PipelineStatus = "succeeded"
	PipelineFailed  PipelineStatus = "failed"
	PipelineStopped PipelineStatus = "stopped"
)

// StepType represents the kind of ETL step.
type StepType string

const (
	StepExtract   StepType = "extract"
	StepTransform StepType = "transform"
	StepLoad      StepType = "load"
	StepFilter    StepType = "filter"
	StepMap       StepType = "map"
	StepAggregate StepType = "aggregate"
	StepJoin      StepType = "join"
	StepValidate  StepType = "validate"
	StepEnrich    StepType = "enrich"
	StepDedupe    StepType = "deduplicate"
)

// Step represents a single step in an ETL pipeline.
type Step struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       StepType               `json:"type"`
	Connector  string                 `json:"connector"` // e.g. mysql, postgres, csv, api, kafka
	Config     map[string]interface{} `json:"config"`
	Order      int                    `json:"order"`
	DependsOn  []string               `json:"depends_on,omitempty"`
	RetryCount int                    `json:"retry_count,omitempty"`
	Timeout    string                 `json:"timeout,omitempty"` // duration string
}

// OrchestrationConfig defines runtime orchestration parameters.
type OrchestrationConfig struct {
	Owner          string   `json:"owner,omitempty"`
	Queue          string   `json:"queue,omitempty"`
	MaxActiveRuns  int      `json:"max_active_runs,omitempty"`
	Concurrency    int      `json:"concurrency,omitempty"`
	PriorityWeight int      `json:"priority_weight,omitempty"`
	Retries        int      `json:"retries,omitempty"`
	RetryDelaySec  int      `json:"retry_delay_sec,omitempty"`
	TimeoutSec     int      `json:"timeout_sec,omitempty"`
	SLASeconds     int      `json:"sla_seconds,omitempty"`
	Catchup        bool     `json:"catchup"`
	DependsOnPast  bool     `json:"depends_on_past"`
	AlertChannels  []string `json:"alert_channels,omitempty"`
}

// PipelineSpec is the *desired* state of an ETL Pipeline.
//
// It is intentionally a superset that mirrors the imperative `Pipeline`
// definition so the controller can deterministically produce / update an
// underlying `*Pipeline` inside `Engine` from the spec alone.
type PipelineSpec struct {
	Description   string              `json:"description,omitempty"`
	Steps         []Step              `json:"steps"`
	Schedule      string              `json:"schedule,omitempty"`
	Orchestration OrchestrationConfig `json:"orchestration,omitempty"`
	Config        map[string]interface{} `json:"config,omitempty"`
	Tags          []string            `json:"tags,omitempty"`

	// Paused, when true, asks the controller to not execute scheduled runs.
	// It maps to `PipelineStatus = paused` on the underlying Pipeline.
	Paused bool `json:"paused,omitempty"`
}

// PipelineResourceStatus extends the canonical object status with
// ETL-specific telemetry.  All fields are controller-owned.
type PipelineResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	// PipelineStatus mirrors the engine's per-pipeline status
	// (`created/running/paused/succeeded/failed/stopped`).
	PipelineStatus PipelineStatus `json:"pipelineStatus,omitempty"`

	// LastRunAt is the most recent run timestamp.
	LastRunAt *time.Time `json:"lastRunAt,omitempty"`

	// RunCount is the total number of runs observed by the controller.
	RunCount int `json:"runCount"`

	// LastRunID is the identifier of the most recent `PipelineRun`.
	LastRunID string `json:"lastRunId,omitempty"`
}

// PipelineResource is the declarative resource for an ETL Pipeline.
type PipelineResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   PipelineSpec           `json:"spec"`
	Status PipelineResourceStatus `json:"status"`
}

// Kind / APIVersion constants used by the controller + API server.
const (
	PipelineKind       = "ETLPipeline"
	PipelineAPIVersion = "etl.axiomnizam.io/v1"
)

// --- resources.Resource implementation ---

func (p *PipelineResource) GetObjectMeta() *resources.ObjectMeta { return &p.ObjectMeta }
func (p *PipelineResource) GetTypeMeta() *resources.TypeMeta     { return &p.TypeMeta }

func (p *PipelineResource) GetStatus() *resources.ObjectStatus {
	return &p.Status.ObjectStatus
}

func (p *PipelineResource) SetStatus(status *resources.ObjectStatus) {
	if status == nil {
		return
	}
	p.Status.ObjectStatus = *status
}

func (p *PipelineResource) DeepCopy() resources.Resource {
	cp := *p
	// Steps slice is the only deep field we realistically need copied.
	if len(p.Spec.Steps) > 0 {
		cp.Spec.Steps = make([]Step, len(p.Spec.Steps))
		copy(cp.Spec.Steps, p.Spec.Steps)
	}
	return &cp
}

// --- reconciler.Resource implementation ---

// GetKey returns the canonical namespace/name key.  If namespace is empty
// we fall back to just the name to remain consistent with global
// resources.
func (p *PipelineResource) GetKey() string {
	if p.Namespace == "" {
		return p.Name
	}
	return p.Namespace + "/" + p.Name
}

func (p *PipelineResource) GetGeneration() int64         { return p.Generation }
func (p *PipelineResource) GetObservedGeneration() int64 { return p.Status.ObservedGeneration }
