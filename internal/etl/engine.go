package etl

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// =====================================================
// ETL Engine — Extract, Transform, Load
// Dynamic pipelines with observability
// =====================================================

// Domain types (PipelineStatus, StepType, Step, OrchestrationConfig)
// are re-exported via type aliases in pipeline_resource.go from the
// models sub-package.  Pipeline and runtime types remain here.

// Pipeline is the imperative runtime shape used by Engine.
type Pipeline struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Description   string              `json:"description"`
	Steps         []Step              `json:"steps"`
	Schedule      string              `json:"schedule,omitempty"` // cron expression
	Orchestration OrchestrationConfig `json:"orchestration,omitempty"`
	Config        map[string]interface{} `json:"config,omitempty"`
	Status        PipelineStatus      `json:"status"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
	LastRunAt     *time.Time          `json:"last_run_at,omitempty"`
	RunCount      int                 `json:"run_count"`
	Tags          []string            `json:"tags,omitempty"`
}

// --- Run / Execution ---

type PipelineRun struct {
	ID          string         `json:"id"`
	PipelineID  string         `json:"pipeline_id"`
	Status      PipelineStatus `json:"status"`
	StartedAt   time.Time      `json:"started_at"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
	Duration    string         `json:"duration,omitempty"`
	StepResults []StepResult   `json:"step_results"`
	RowsRead    int64          `json:"rows_read"`
	RowsWritten int64          `json:"rows_written"`
	RowsFailed  int64          `json:"rows_failed"`
	ErrorMsg    string         `json:"error_msg,omitempty"`
	Trigger     string         `json:"trigger"` // manual, schedule, api, event
}

type StepResult struct {
	StepID     string      `json:"step_id"`
	StepName   string      `json:"step_name"`
	StepType   StepType    `json:"step_type"`
	Status     string      `json:"status"` // running, succeeded, failed, skipped
	StartedAt  time.Time   `json:"started_at"`
	FinishedAt *time.Time  `json:"finished_at,omitempty"`
	Duration   string      `json:"duration,omitempty"`
	RowsIn     int64       `json:"rows_in"`
	RowsOut    int64       `json:"rows_out"`
	RowsError  int64       `json:"rows_error"`
	ErrorMsg   string      `json:"error_msg,omitempty"`
	Metrics    StepMetrics `json:"metrics"`
}

type StepMetrics struct {
	BytesProcessed int64   `json:"bytes_processed"`
	ThroughputRPS  float64 `json:"throughput_rps"` // rows per second
	AvgRowSizeB    int64   `json:"avg_row_size_bytes"`
	MemoryUsedMB   float64 `json:"memory_used_mb"`
	CPUPercent     float64 `json:"cpu_percent"`
}

// --- Connector Registry ---

type ConnectorType struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Category            string   `json:"category"`     // database, file, api, queue, stream
	SupportedAs         []string `json:"supported_as"` // extract, load, both
	ConfigKeys          []string `json:"config_keys"`
	Icon                string   `json:"icon"`
	Description         string   `json:"description,omitempty"`
	Version             string   `json:"version,omitempty"`
	AuthModes           []string `json:"auth_modes,omitempty"`
	SupportsCDC         bool     `json:"supports_cdc,omitempty"`
	SupportsIncremental bool     `json:"supports_incremental,omitempty"`
	SchemaDiscovery     bool     `json:"schema_discovery,omitempty"`
}

type OrchestrationCapability struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

type PipelineBlueprintStep struct {
	Name      string   `json:"name"`
	Type      StepType `json:"type"`
	Connector string   `json:"connector,omitempty"`
}

type PipelineBlueprint struct {
	ID              string                  `json:"id"`
	Name            string                  `json:"name"`
	Category        string                  `json:"category"`
	Description     string                  `json:"description"`
	DefaultSchedule string                  `json:"default_schedule,omitempty"`
	Steps           []PipelineBlueprintStep `json:"steps"`
	Tags            []string                `json:"tags"`
}

// --- Observability ---

type ETLObservability struct {
	mu             sync.RWMutex
	PipelinesTotal int               `json:"pipelines_total"`
	RunsTotal      int               `json:"runs_total"`
	RunsSuccess    int               `json:"runs_success"`
	RunsFailed     int               `json:"runs_failed"`
	RunsRunning    int               `json:"runs_running"`
	TotalRowsRead  int64             `json:"total_rows_read"`
	TotalRowsWrite int64             `json:"total_rows_written"`
	AvgDuration    float64           `json:"avg_duration_seconds"`
	StepTypeStats  map[string]int    `json:"step_type_stats"`
	ErrorsByType   map[string]int    `json:"errors_by_type"`
	ThroughputLog  []ThroughputPoint `json:"throughput_log"`
	LastUpdated    time.Time         `json:"last_updated"`
}

type ThroughputPoint struct {
	Timestamp  time.Time `json:"timestamp"`
	RowsPerSec float64   `json:"rows_per_sec"`
	Pipeline   string    `json:"pipeline"`
}

// --- Engine ---

type Engine struct {
	mu            sync.RWMutex
	pipelines     map[string]*Pipeline
	runs          map[string]*PipelineRun
	connectors    []ConnectorType
	observability *ETLObservability
	sequence      int64
	etcd          *clientv3.Client
	stateKey      string
}

type engineState struct {
	Pipelines     map[string]*Pipeline    `json:"pipelines"`
	Runs          map[string]*PipelineRun `json:"runs"`
	Connectors    []ConnectorType         `json:"connectors"`
	Observability *ETLObservability       `json:"observability"`
	Sequence      int64                   `json:"sequence"`
}

func NewEngine(etcd ...*clientv3.Client) *Engine {
	var etcdClient *clientv3.Client
	if len(etcd) > 0 {
		etcdClient = etcd[0]
	}

	e := &Engine{
		pipelines: make(map[string]*Pipeline),
		runs:      make(map[string]*PipelineRun),
		observability: &ETLObservability{
			StepTypeStats: make(map[string]int),
			ErrorsByType:  make(map[string]int),
			ThroughputLog: make([]ThroughputPoint, 0),
			LastUpdated:   time.Now(),
		},
		etcd:     etcdClient,
		stateKey: "etl:engine:state",
	}
	e.registerConnectors()
	if !e.loadState() {
		e.persistStateLocked()
	}
	return e
}

func (e *Engine) loadState() bool {
	if e.etcd == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := e.etcd.Get(ctx, e.stateKey)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("etl: failed to load persisted state from etcd: %v", err))
		return false
	}
	if len(resp.Kvs) == 0 {
		return false
	}

	var state engineState
	if err := json.Unmarshal(resp.Kvs[0].Value, &state); err != nil {
		logging.Z().Info(fmt.Sprintf("etl: failed to decode persisted state: %v", err))
		return false
	}

	if state.Pipelines != nil {
		e.pipelines = state.Pipelines
	}
	if state.Runs != nil {
		e.runs = state.Runs
	}
	// Do NOT overwrite built-in connectors from persisted state;
	// registerConnectors() is the canonical source for the connector registry.
	if state.Observability != nil {
		e.observability = state.Observability
	}
	e.sequence = state.Sequence
	logging.Z().Info(fmt.Sprintf("etl: restored state from etcd (%d pipelines, %d runs)", len(e.pipelines), len(e.runs)))
	return true
}

func (e *Engine) persistStateLocked() {
	if e.etcd == nil {
		return
	}

	state := engineState{
		Pipelines:     e.pipelines,
		Runs:          e.runs,
		Connectors:    nil, // connectors are code-defined, not persisted
		Observability: e.observability,
		Sequence:      e.sequence,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("etl: failed to encode state: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := e.etcd.Put(ctx, e.stateKey, string(payload)); err != nil {
		logging.Z().Info(fmt.Sprintf("etl: failed to persist state to etcd: %v", err))
	}
}

// --- Connector Registry ---

func (e *Engine) registerConnectors() {
	e.connectors = []ConnectorType{
		{ID: "mysql", Name: "MySQL", Category: "database", SupportedAs: []string{"extract", "load"}, ConfigKeys: []string{"host", "port", "database", "table", "query"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon ax-icon-mysql"><ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v6c0 1.66 3.58 3 8 3s8-1.34 8-3V6"/><path d="M4 12v6c0 1.66 3.58 3 8 3s8-1.34 8-3v-6"/></svg>`, Description: "Operational MySQL datasets", Version: "8.x", AuthModes: []string{"password", "iam"}, SupportsIncremental: true, SchemaDiscovery: true, SupportsCDC: true},
		{ID: "postgres", Name: "PostgreSQL", Category: "database", SupportedAs: []string{"extract", "load"}, ConfigKeys: []string{"host", "port", "database", "table", "query"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon ax-icon-postgres"><ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v6c0 1.66 3.58 3 8 3s8-1.34 8-3V6"/><path d="M4 12v6c0 1.66 3.58 3 8 3s8-1.34 8-3v-6"/><path d="M15 6v12" opacity="0.4"/></svg>`, Description: "Warehouse and OLTP PostgreSQL", Version: "15+", AuthModes: []string{"password", "ssl"}, SupportsIncremental: true, SchemaDiscovery: true, SupportsCDC: true},
		{ID: "mariadb", Name: "MariaDB", Category: "database", SupportedAs: []string{"extract", "load"}, ConfigKeys: []string{"host", "port", "database", "table", "query"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon ax-icon-mariadb"><ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v6c0 1.66 3.58 3 8 3s8-1.34 8-3V6"/><path d="M4 12v6c0 1.66 3.58 3 8 3s8-1.34 8-3v-6"/><path d="M9 6v12" opacity="0.4"/></svg>`, Description: "MariaDB transactional source/target", Version: "10.6+", AuthModes: []string{"password"}, SupportsIncremental: true, SchemaDiscovery: true, SupportsCDC: true},
		{ID: "mongodb", Name: "MongoDB", Category: "database", SupportedAs: []string{"extract", "load"}, ConfigKeys: []string{"uri", "database", "collection", "filter"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon ax-icon-mongodb"><path d="M12 2C9 5 8 8 8 12c0 4 2 7 4 10"/><path d="M12 2c3 3 4 6 4 10 0 4-2 7-4 10"/><path d="M12 6v16"/></svg>`, Description: "Document datasets and collections", Version: "6+", AuthModes: []string{"uri", "x509"}, SupportsIncremental: true, SchemaDiscovery: true, SupportsCDC: true},
		{ID: "oracle", Name: "Oracle", Category: "database", SupportedAs: []string{"extract", "load"}, ConfigKeys: []string{"host", "port", "service_name", "table", "query"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon ax-icon-oracle"><ellipse cx="12" cy="12" rx="9" ry="5"/><path d="M7 12h10"/><circle cx="12" cy="12" r="2"/></svg>`, Description: "Enterprise Oracle workloads", Version: "19c+", AuthModes: []string{"password"}, SupportsIncremental: true, SchemaDiscovery: true, SupportsCDC: true},
		{ID: "csv", Name: "CSV File", Category: "file", SupportedAs: []string{"extract", "load"}, ConfigKeys: []string{"path", "delimiter", "encoding", "has_header"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon ax-icon-csv"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><path d="M8 13h2"/><path d="M8 17h2"/><path d="M14 13h2"/><path d="M14 17h2"/></svg>`, Description: "Flat-file ingestion/export", Version: "1.0", AuthModes: []string{"none"}, SupportsIncremental: false, SchemaDiscovery: false},
		{ID: "json", Name: "JSON File", Category: "file", SupportedAs: []string{"extract", "load"}, ConfigKeys: []string{"path", "json_path"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon ax-icon-json"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><path d="M8 13c0-1 1-2 2-2s2 1 2 2-1 2-2 2"/><path d="M14 13c0-1 1-2 2-2"/></svg>`, Description: "JSON blob ingestion/export", Version: "1.0", AuthModes: []string{"none"}, SupportsIncremental: false, SchemaDiscovery: false},
		{ID: "api", Name: "REST API", Category: "api", SupportedAs: []string{"extract", "load"}, ConfigKeys: []string{"url", "method", "headers", "body", "auth"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon ax-icon-api"><circle cx="12" cy="12" r="9"/><path d="M3.6 9h16.8"/><path d="M3.6 15h16.8"/><path d="M12 3c-2.8 2.4-4 5.6-4 9s1.2 6.6 4 9c2.8-2.4 4-5.6 4-9s-1.2-6.6-4-9"/></svg>`, Description: "HTTP data source and destination", Version: "v1", AuthModes: []string{"apikey", "oauth2", "basic"}, SupportsIncremental: true, SchemaDiscovery: false},
		{ID: "kafka", Name: "Apache Kafka", Category: "stream", SupportedAs: []string{"extract", "load"}, ConfigKeys: []string{"brokers", "topic", "group_id", "offset"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon ax-icon-kafka"><circle cx="8" cy="6" r="2"/><circle cx="8" cy="18" r="2"/><circle cx="16" cy="12" r="2"/><path d="M10 6h4l2 6-2 6h-4"/><path d="M8 8v8"/></svg>`, Description: "Real-time event streaming", Version: "3.x", AuthModes: []string{"sasl", "tls"}, SupportsIncremental: true, SchemaDiscovery: true, SupportsCDC: true},
		{ID: "redis", Name: "Redis/Valkey", Category: "queue", SupportedAs: []string{"extract", "load"}, ConfigKeys: []string{"host", "port", "key", "type"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon ax-icon-redis"><path d="M4 12l8-4 8 4-8 4-8-4z"/><path d="M4 12v4l8 4 8-4v-4"/><path d="M4 8v4l8 4 8-4V8"/><path d="M4 8l8-4 8 4-8 4-8-4z"/></svg>`, Description: "Low-latency cache and streams", Version: "7+", AuthModes: []string{"password"}, SupportsIncremental: true, SchemaDiscovery: false},
		{ID: "elasticsearch", Name: "Elasticsearch", Category: "search", SupportedAs: []string{"extract", "load"}, ConfigKeys: []string{"url", "index", "query"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon ax-icon-elastic"><circle cx="11" cy="11" r="7"/><path d="M21 21l-4.35-4.35"/><path d="M8 11h6"/></svg>`, Description: "Search and analytics indexing", Version: "8+", AuthModes: []string{"apikey", "basic"}, SupportsIncremental: true, SchemaDiscovery: true},
		{ID: "s3", Name: "S3/MinIO", Category: "storage", SupportedAs: []string{"extract", "load"}, ConfigKeys: []string{"endpoint", "bucket", "key", "region"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon ax-icon-s3"><path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z"/></svg>`, Description: "Object storage lakehouse layer", Version: "v1", AuthModes: []string{"access_key", "iam"}, SupportsIncremental: true, SchemaDiscovery: false},
	}
}

func (e *Engine) GetConnectors() []ConnectorType {
	return e.connectors
}

func (e *Engine) AddConnector(connector ConnectorType) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	connector.ID = strings.TrimSpace(strings.ToLower(connector.ID))
	connector.Name = strings.TrimSpace(connector.Name)
	connector.Category = strings.TrimSpace(strings.ToLower(connector.Category))
	if connector.ID == "" || connector.Name == "" || connector.Category == "" {
		return fmt.Errorf("id, name and category are required")
	}

	for _, c := range e.connectors {
		if c.ID == connector.ID {
			return fmt.Errorf("connector already exists: %s", connector.ID)
		}
	}

	if len(connector.SupportedAs) == 0 {
		connector.SupportedAs = []string{"extract", "load"}
	}
	if connector.Icon == "" {
		connector.Icon = "🔌"
	}
	if connector.Version == "" {
		connector.Version = "1.0"
	}

	e.connectors = append(e.connectors, connector)
	e.persistStateLocked()
	return nil
}

// UpdateConnector patches an existing connector by ID.
func (e *Engine) UpdateConnector(id string, updates ConnectorType) (*ConnectorType, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	id = strings.TrimSpace(strings.ToLower(id))
	for i, c := range e.connectors {
		if c.ID == id {
			if updates.Name != "" {
				e.connectors[i].Name = strings.TrimSpace(updates.Name)
			}
			if updates.Category != "" {
				e.connectors[i].Category = strings.TrimSpace(strings.ToLower(updates.Category))
			}
			if updates.Icon != "" {
				e.connectors[i].Icon = updates.Icon
			}
			if updates.Version != "" {
				e.connectors[i].Version = updates.Version
			}
			if updates.Description != "" {
				e.connectors[i].Description = updates.Description
			}
			if len(updates.SupportedAs) > 0 {
				e.connectors[i].SupportedAs = updates.SupportedAs
			}
			if len(updates.AuthModes) > 0 {
				e.connectors[i].AuthModes = updates.AuthModes
			}
			if len(updates.ConfigKeys) > 0 {
				e.connectors[i].ConfigKeys = updates.ConfigKeys
			}
			e.connectors[i].SupportsIncremental = updates.SupportsIncremental
			e.connectors[i].SchemaDiscovery = updates.SchemaDiscovery
			e.connectors[i].SupportsCDC = updates.SupportsCDC
			e.persistStateLocked()
			return &e.connectors[i], nil
		}
	}
	return nil, fmt.Errorf("connector not found: %s", id)
}

// DeleteConnector removes a connector by ID.
func (e *Engine) DeleteConnector(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	id = strings.TrimSpace(strings.ToLower(id))
	for i, c := range e.connectors {
		if c.ID == id {
			e.connectors = append(e.connectors[:i], e.connectors[i+1:]...)
			e.persistStateLocked()
			return nil
		}
	}
	return fmt.Errorf("connector not found: %s", id)
}

func (e *Engine) GetOrchestrationCapabilities() []OrchestrationCapability {
	return []OrchestrationCapability{
		{ID: "retries_backoff", Name: "Retries with Backoff", Category: "Reliability", Description: "Task-level retries, retry delay, and exponential backoff controls."},
		{ID: "sla_management", Name: "SLA Management", Category: "Reliability", Description: "Define SLA thresholds per pipeline and monitor SLA misses."},
		{ID: "catchup_backfill", Name: "Catchup and Backfill", Category: "Scheduling", Description: "Run historical intervals for missed schedules and backfill windows."},
		{ID: "depends_on_past", Name: "Depends On Past", Category: "Scheduling", Description: "Enforce sequential dependency between historical pipeline runs."},
		{ID: "concurrency_control", Name: "Concurrency and Pools", Category: "Execution", Description: "Cap active runs and task-level parallelism by queue/pool."},
		{ID: "priority_queueing", Name: "Priority Queues", Category: "Execution", Description: "Set queue and priority weight for fair and urgent scheduling."},
		{ID: "event_triggering", Name: "Event Triggers", Category: "Execution", Description: "Support manual, API, schedule, and event-driven triggers."},
		{ID: "lineage_and_observability", Name: "Lineage and Observability", Category: "Observability", Description: "Track throughput, errors, step stats, and lineage-aligned metrics."},
		{ID: "alert_channels", Name: "Alert Channels", Category: "Operations", Description: "Attach alert channels like email, slack, pagerduty, and webhook."},
		{ID: "template_blueprints", Name: "Pipeline Blueprints", Category: "Authoring", Description: "Bootstrap common ETL patterns from curated templates."},
	}
}

func (e *Engine) GetPipelineBlueprints() []PipelineBlueprint {
	return []PipelineBlueprint{
		{
			ID:              "batch-incremental",
			Name:            "Incremental Batch Sync",
			Category:        "batch",
			Description:     "Incremental extraction with validation, transform, and warehouse load.",
			DefaultSchedule: "*/30 * * * *",
			Tags:            []string{"incremental", "warehouse"},
			Steps: []PipelineBlueprintStep{
				{Name: "Extract Incremental", Type: StepExtract, Connector: "mysql"},
				{Name: "Validate Schema", Type: StepValidate},
				{Name: "Transform Fields", Type: StepTransform},
				{Name: "Load Warehouse", Type: StepLoad, Connector: "postgres"},
			},
		},
		{
			ID:              "streaming-lakehouse",
			Name:            "Streaming to Lakehouse",
			Category:        "streaming",
			Description:     "Consume stream events, enrich and write partitioned objects to S3/MinIO.",
			DefaultSchedule: "",
			Tags:            []string{"streaming", "lakehouse"},
			Steps: []PipelineBlueprintStep{
				{Name: "Consume Topic", Type: StepExtract, Connector: "kafka"},
				{Name: "Filter Events", Type: StepFilter},
				{Name: "Enrich Dimensions", Type: StepEnrich, Connector: "api"},
				{Name: "Write Object Store", Type: StepLoad, Connector: "s3"},
			},
		},
		{
			ID:              "quality-gate",
			Name:            "Quality Gate ETL",
			Category:        "quality",
			Description:     "Validation-first pattern with deduplication and SLA-aware loading.",
			DefaultSchedule: "0 * * * *",
			Tags:            []string{"quality", "dedupe", "sla"},
			Steps: []PipelineBlueprintStep{
				{Name: "Extract Source", Type: StepExtract, Connector: "api"},
				{Name: "Validate Rules", Type: StepValidate},
				{Name: "Deduplicate", Type: StepDedupe},
				{Name: "Load Curated", Type: StepLoad, Connector: "elasticsearch"},
			},
		},
	}
}

// --- Pipeline CRUD ---

func (e *Engine) CreatePipeline(p *Pipeline) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if p.ID == "" {
		e.sequence++
		p.ID = fmt.Sprintf("etl-pipe-%d", e.sequence)
	}
	if p.Config == nil {
		p.Config = map[string]interface{}{}
	}
	if p.Steps == nil {
		p.Steps = []Step{}
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	p.Orchestration = normalizeOrchestration(p.Orchestration)
	p.Status = PipelineCreated
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	e.pipelines[p.ID] = p
	e.observability.mu.Lock()
	e.observability.PipelinesTotal++
	e.observability.mu.Unlock()
	e.persistStateLocked()
	return nil
}

func (e *Engine) GetPipeline(id string) (*Pipeline, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	p, ok := e.pipelines[id]
	return p, ok
}

func (e *Engine) ListPipelines() []*Pipeline {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]*Pipeline, 0, len(e.pipelines))
	for _, p := range e.pipelines {
		result = append(result, p)
	}
	return result
}

func (e *Engine) UpdatePipeline(id string, updates map[string]interface{}) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.pipelines[id]
	if !ok {
		return fmt.Errorf("pipeline not found: %s", id)
	}
	if name, ok := updates["name"].(string); ok && name != "" {
		p.Name = name
	}
	if desc, ok := updates["description"].(string); ok {
		p.Description = desc
	}
	if sched, ok := updates["schedule"].(string); ok {
		p.Schedule = sched
	}
	if tagsRaw, ok := updates["tags"].([]interface{}); ok {
		tags := make([]string, 0, len(tagsRaw))
		for _, t := range tagsRaw {
			if tv, ok := t.(string); ok && strings.TrimSpace(tv) != "" {
				tags = append(tags, strings.TrimSpace(tv))
			}
		}
		p.Tags = tags
	}
	if stepsRaw, ok := updates["steps"]; ok {
		if payload, err := json.Marshal(stepsRaw); err == nil {
			var steps []Step
			if err := json.Unmarshal(payload, &steps); err == nil {
				normalized := make([]Step, 0, len(steps))
				for i := range steps {
					step := steps[i]
					if strings.TrimSpace(step.ID) == "" {
						step.ID = fmt.Sprintf("step-%d", i+1)
					}
					if strings.TrimSpace(step.Name) == "" {
						step.Name = fmt.Sprintf("Step %d", i+1)
					}
					if step.Order <= 0 {
						step.Order = i + 1
					}
					if step.Config == nil {
						step.Config = map[string]interface{}{}
					}
					normalized = append(normalized, step)
				}
				p.Steps = normalized
			}
		}
	}
	if cfg, ok := updates["config"].(map[string]interface{}); ok {
		p.Config = cfg
	}
	if orchRaw, ok := updates["orchestration"].(map[string]interface{}); ok {
		orch := p.Orchestration
		if v, ok := orchRaw["owner"].(string); ok {
			orch.Owner = strings.TrimSpace(v)
		}
		if v, ok := orchRaw["queue"].(string); ok {
			orch.Queue = strings.TrimSpace(v)
		}
		if v, ok := orchRaw["max_active_runs"].(float64); ok {
			orch.MaxActiveRuns = int(v)
		}
		if v, ok := orchRaw["concurrency"].(float64); ok {
			orch.Concurrency = int(v)
		}
		if v, ok := orchRaw["priority_weight"].(float64); ok {
			orch.PriorityWeight = int(v)
		}
		if v, ok := orchRaw["retries"].(float64); ok {
			orch.Retries = int(v)
		}
		if v, ok := orchRaw["retry_delay_sec"].(float64); ok {
			orch.RetryDelaySec = int(v)
		}
		if v, ok := orchRaw["timeout_sec"].(float64); ok {
			orch.TimeoutSec = int(v)
		}
		if v, ok := orchRaw["sla_seconds"].(float64); ok {
			orch.SLASeconds = int(v)
		}
		if v, ok := orchRaw["catchup"].(bool); ok {
			orch.Catchup = v
		}
		if v, ok := orchRaw["depends_on_past"].(bool); ok {
			orch.DependsOnPast = v
		}
		if channelsRaw, ok := orchRaw["alert_channels"].([]interface{}); ok {
			channels := make([]string, 0, len(channelsRaw))
			for _, c := range channelsRaw {
				if cv, ok := c.(string); ok && strings.TrimSpace(cv) != "" {
					channels = append(channels, strings.TrimSpace(cv))
				}
			}
			orch.AlertChannels = channels
		}
		p.Orchestration = normalizeOrchestration(orch)
	}
	p.UpdatedAt = time.Now()
	e.persistStateLocked()
	return nil
}

func normalizeOrchestration(o OrchestrationConfig) OrchestrationConfig {
	if o.Queue == "" {
		o.Queue = "default"
	}
	if o.MaxActiveRuns <= 0 {
		o.MaxActiveRuns = 1
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 4
	}
	if o.PriorityWeight <= 0 {
		o.PriorityWeight = 5
	}
	if o.Retries < 0 {
		o.Retries = 0
	}
	if o.RetryDelaySec <= 0 {
		o.RetryDelaySec = 60
	}
	if o.TimeoutSec <= 0 {
		o.TimeoutSec = 1800
	}
	if o.SLASeconds <= 0 {
		o.SLASeconds = 3600
	}
	if o.AlertChannels == nil {
		o.AlertChannels = []string{"slack"}
	}
	return o
}

func (e *Engine) DeletePipeline(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.pipelines[id]; !ok {
		return fmt.Errorf("pipeline not found: %s", id)
	}
	delete(e.pipelines, id)
	e.observability.mu.Lock()
	e.observability.PipelinesTotal--
	e.observability.mu.Unlock()
	e.persistStateLocked()
	return nil
}

// --- Execution ---

func (e *Engine) RunPipeline(ctx context.Context, pipelineID string, trigger string) (*PipelineRun, error) {
	e.mu.Lock()
	p, ok := e.pipelines[pipelineID]
	if !ok {
		e.mu.Unlock()
		return nil, fmt.Errorf("pipeline not found: %s", pipelineID)
	}
	e.sequence++
	runID := fmt.Sprintf("run-%d-%d", time.Now().UnixMilli(), e.sequence)
	now := time.Now()

	run := &PipelineRun{
		ID:          runID,
		PipelineID:  pipelineID,
		Status:      PipelineRunning,
		StartedAt:   now,
		StepResults: make([]StepResult, 0),
		Trigger:     trigger,
	}
	e.runs[runID] = run
	p.Status = PipelineRunning
	p.LastRunAt = &now
	p.RunCount++
	e.persistStateLocked()
	e.mu.Unlock()

	// Update observability
	e.observability.mu.Lock()
	e.observability.RunsTotal++
	e.observability.RunsRunning++
	e.observability.mu.Unlock()

	// Execute steps in order (simulated)
	go e.executeSteps(ctx, p, run)

	return run, nil
}

func (e *Engine) executeSteps(ctx context.Context, p *Pipeline, run *PipelineRun) {
	var totalRead, totalWritten, totalFailed int64
	allSuccess := true

	for _, step := range p.Steps {
		select {
		case <-ctx.Done():
			e.mu.Lock()
			run.Status = PipelineStopped
			run.ErrorMsg = "cancelled"
			finTime := time.Now()
			run.FinishedAt = &finTime
			p.Status = PipelineStopped
			e.persistStateLocked()
			e.mu.Unlock()
			return
		default:
		}

		stepStart := time.Now()
		// Simulate step execution based on type
		rowsIn, rowsOut, rowsErr, err := e.simulateStep(step)

		stepEnd := time.Now()
		dur := stepEnd.Sub(stepStart)
		status := "succeeded"
		errMsg := ""
		if err != nil {
			status = "failed"
			errMsg = err.Error()
			allSuccess = false
		}

		throughput := float64(0)
		if dur.Seconds() > 0 {
			throughput = float64(rowsOut) / dur.Seconds()
		}

		result := StepResult{
			StepID:     step.ID,
			StepName:   step.Name,
			StepType:   step.Type,
			Status:     status,
			StartedAt:  stepStart,
			FinishedAt: &stepEnd,
			Duration:   dur.String(),
			RowsIn:     rowsIn,
			RowsOut:    rowsOut,
			RowsError:  rowsErr,
			ErrorMsg:   errMsg,
			Metrics: StepMetrics{
				BytesProcessed: rowsOut * 256,
				ThroughputRPS:  throughput,
				AvgRowSizeB:    256,
				MemoryUsedMB:   float64(rowsOut) * 0.001,
				CPUPercent:     math.Min(float64(rowsOut)*0.002, 95),
			},
		}

		e.mu.Lock()
		run.StepResults = append(run.StepResults, result)
		e.persistStateLocked()
		e.mu.Unlock()

		totalRead += rowsIn
		totalWritten += rowsOut
		totalFailed += rowsErr

		// Track step type stats
		e.observability.mu.Lock()
		e.observability.StepTypeStats[string(step.Type)]++
		if err != nil {
			errorType := "step_failure"
			if strings.Contains(errMsg, "timeout") {
				errorType = "timeout"
			} else if strings.Contains(errMsg, "connection") {
				errorType = "connection"
			}
			e.observability.ErrorsByType[errorType]++
		}
		// Track throughput
		if len(e.observability.ThroughputLog) > 200 {
			e.observability.ThroughputLog = e.observability.ThroughputLog[1:]
		}
		e.observability.ThroughputLog = append(e.observability.ThroughputLog, ThroughputPoint{
			Timestamp:  time.Now(),
			RowsPerSec: throughput,
			Pipeline:   p.Name,
		})
		e.observability.mu.Unlock()

		if !allSuccess {
			break
		}
	}

	finTime := time.Now()
	dur := finTime.Sub(run.StartedAt)

	e.mu.Lock()
	run.FinishedAt = &finTime
	run.Duration = dur.String()
	run.RowsRead = totalRead
	run.RowsWritten = totalWritten
	run.RowsFailed = totalFailed
	if allSuccess {
		run.Status = PipelineSuccess
		p.Status = PipelineSuccess
	} else {
		run.Status = PipelineFailed
		p.Status = PipelineFailed
	}
	e.persistStateLocked()
	e.mu.Unlock()

	e.observability.mu.Lock()
	e.observability.RunsRunning--
	if allSuccess {
		e.observability.RunsSuccess++
	} else {
		e.observability.RunsFailed++
	}
	e.observability.TotalRowsRead += totalRead
	e.observability.TotalRowsWrite += totalWritten
	n := float64(e.observability.RunsTotal)
	if n > 0 {
		e.observability.AvgDuration = (e.observability.AvgDuration*(n-1) + dur.Seconds()) / n
	}
	e.observability.LastUpdated = time.Now()
	e.observability.mu.Unlock()

	e.mu.Lock()
	e.persistStateLocked()
	e.mu.Unlock()
}

func (e *Engine) simulateStep(step Step) (rowsIn, rowsOut, rowsErr int64, err error) {
	// Simulate different row counts based on step type
	switch step.Type {
	case StepExtract:
		rowsIn = 10000
		rowsOut = 10000
	case StepTransform:
		rowsIn = 10000
		rowsOut = 9800
		rowsErr = 200
	case StepFilter:
		rowsIn = 9800
		rowsOut = 7500
	case StepMap:
		rowsIn = 7500
		rowsOut = 7500
	case StepAggregate:
		rowsIn = 7500
		rowsOut = 150
	case StepValidate:
		rowsIn = 7500
		rowsOut = 7200
		rowsErr = 300
	case StepEnrich:
		rowsIn = 7200
		rowsOut = 7200
	case StepDedupe:
		rowsIn = 7200
		rowsOut = 6800
	case StepLoad:
		rowsIn = 6800
		rowsOut = 6800
	default:
		rowsIn = 1000
		rowsOut = 1000
	}
	// Simulate small delay
	time.Sleep(50 * time.Millisecond)
	return
}

// --- Run Queries ---

func (e *Engine) GetRun(runID string) (*PipelineRun, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	r, ok := e.runs[runID]
	return r, ok
}

func (e *Engine) ListRuns(pipelineID string) []*PipelineRun {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]*PipelineRun, 0)
	for _, r := range e.runs {
		if pipelineID == "" || r.PipelineID == pipelineID {
			result = append(result, r)
		}
	}
	return result
}

func (e *Engine) GetObservability() *ETLObservability {
	e.observability.mu.RLock()
	defer e.observability.mu.RUnlock()

	stepTypeStats := make(map[string]int, len(e.observability.StepTypeStats))
	for key, value := range e.observability.StepTypeStats {
		stepTypeStats[key] = value
	}

	errorsByType := make(map[string]int, len(e.observability.ErrorsByType))
	for key, value := range e.observability.ErrorsByType {
		errorsByType[key] = value
	}

	throughputLog := make([]ThroughputPoint, len(e.observability.ThroughputLog))
	copy(throughputLog, e.observability.ThroughputLog)

	return &ETLObservability{
		PipelinesTotal: e.observability.PipelinesTotal,
		RunsTotal:      e.observability.RunsTotal,
		RunsSuccess:    e.observability.RunsSuccess,
		RunsFailed:     e.observability.RunsFailed,
		RunsRunning:    e.observability.RunsRunning,
		TotalRowsRead:  e.observability.TotalRowsRead,
		TotalRowsWrite: e.observability.TotalRowsWrite,
		AvgDuration:    e.observability.AvgDuration,
		StepTypeStats:  stepTypeStats,
		ErrorsByType:   errorsByType,
		ThroughputLog:  throughputLog,
		LastUpdated:    e.observability.LastUpdated,
	}
}

// --- Seed ---

func (e *Engine) seedPipelines() {
	now := time.Now()
	pastRun := now.Add(-2 * time.Hour)

	// Pipeline 1: MySQL to PostgreSQL sync
	p1 := &Pipeline{
		ID:          "etl-mysql-pg-sync",
		Name:        "MySQL → PostgreSQL User Sync",
		Description: "Extracts user data from MySQL, transforms and loads into PostgreSQL",
		Steps: []Step{
			{ID: "s1", Name: "Extract from MySQL", Type: StepExtract, Connector: "mysql", Order: 1, Config: map[string]interface{}{"host": "localhost", "port": 3306, "database": "axiomnizam", "table": "users", "query": "SELECT * FROM users WHERE updated_at > ?"}},
			{ID: "s2", Name: "Validate Schema", Type: StepValidate, Connector: "", Order: 2, Config: map[string]interface{}{"rules": []string{"email_format", "not_null:name"}}},
			{ID: "s3", Name: "Transform Fields", Type: StepTransform, Connector: "", Order: 3, Config: map[string]interface{}{"operations": []string{"rename:fullname->name", "lowercase:email", "hash:password"}}},
			{ID: "s4", Name: "Deduplicate", Type: StepDedupe, Connector: "", Order: 4, Config: map[string]interface{}{"key": "email"}},
			{ID: "s5", Name: "Load to PostgreSQL", Type: StepLoad, Connector: "postgres", Order: 5, Config: map[string]interface{}{"host": "localhost", "port": 5432, "database": "axiomnizam_dw", "table": "users_synced"}},
		},
		Schedule:      "*/30 * * * *",
		Status:        PipelineSuccess,
		CreatedAt:     now.Add(-72 * time.Hour),
		UpdatedAt:     now,
		LastRunAt:     &pastRun,
		RunCount:      145,
		Tags:          []string{"sync", "users", "critical"},
		Orchestration: OrchestrationConfig{Owner: "data-platform", Queue: "critical", MaxActiveRuns: 1, Concurrency: 4, PriorityWeight: 10, Retries: 3, RetryDelaySec: 120, TimeoutSec: 1800, SLASeconds: 2400, Catchup: true, DependsOnPast: true, AlertChannels: []string{"slack", "pagerduty"}},
	}
	e.pipelines[p1.ID] = p1

	// Pipeline 2: API → Data Warehouse
	p2 := &Pipeline{
		ID:          "etl-api-warehouse",
		Name:        "REST API → Data Warehouse",
		Description: "Fetches order data from external API, enriches, and loads to warehouse",
		Steps: []Step{
			{ID: "s1", Name: "Fetch from API", Type: StepExtract, Connector: "api", Order: 1, Config: map[string]interface{}{"url": "https://api.example.com/orders", "method": "GET", "headers": map[string]string{"Authorization": "Bearer ****"}}},
			{ID: "s2", Name: "Filter Active", Type: StepFilter, Connector: "", Order: 2, Config: map[string]interface{}{"condition": "status == 'active'"}},
			{ID: "s3", Name: "Enrich with Customer Data", Type: StepEnrich, Connector: "mysql", Order: 3, Config: map[string]interface{}{"lookup_table": "customers", "join_key": "customer_id"}},
			{ID: "s4", Name: "Aggregate by Region", Type: StepAggregate, Connector: "", Order: 4, Config: map[string]interface{}{"group_by": "region", "functions": []string{"SUM(amount)", "COUNT(*)"}}},
			{ID: "s5", Name: "Load to Warehouse", Type: StepLoad, Connector: "postgres", Order: 5, Config: map[string]interface{}{"host": "localhost", "port": 5432, "database": "axiomnizam_dw", "table": "order_aggregates"}},
		},
		Schedule:      "0 */6 * * *",
		Status:        PipelineSuccess,
		CreatedAt:     now.Add(-48 * time.Hour),
		UpdatedAt:     now,
		LastRunAt:     &pastRun,
		RunCount:      24,
		Tags:          []string{"orders", "warehouse", "aggregation"},
		Orchestration: OrchestrationConfig{Owner: "analytics", Queue: "batch", MaxActiveRuns: 2, Concurrency: 6, PriorityWeight: 7, Retries: 2, RetryDelaySec: 90, TimeoutSec: 2400, SLASeconds: 3600, Catchup: false, DependsOnPast: false, AlertChannels: []string{"slack"}},
	}
	e.pipelines[p2.ID] = p2

	// Pipeline 3: CSV → MongoDB Analytics
	p3 := &Pipeline{
		ID:          "etl-csv-mongo-analytics",
		Name:        "CSV Import → MongoDB Analytics",
		Description: "Imports CSV reports, validates, transforms, and stores in MongoDB for analytics",
		Steps: []Step{
			{ID: "s1", Name: "Read CSV Files", Type: StepExtract, Connector: "csv", Order: 1, Config: map[string]interface{}{"path": "/data/reports/*.csv", "delimiter": ",", "has_header": true}},
			{ID: "s2", Name: "Validate Data", Type: StepValidate, Connector: "", Order: 2, Config: map[string]interface{}{"rules": []string{"numeric:amount", "date:created_at", "not_empty:product_id"}}},
			{ID: "s3", Name: "Map Fields", Type: StepMap, Connector: "", Order: 3, Config: map[string]interface{}{"mappings": map[string]string{"prod_id": "product_id", "amt": "amount", "dt": "created_at"}}},
			{ID: "s4", Name: "Load to MongoDB", Type: StepLoad, Connector: "mongodb", Order: 4, Config: map[string]interface{}{"uri": "mongodb://localhost:27017", "database": "analytics", "collection": "reports"}},
		},
		Schedule:      "0 2 * * *",
		Status:        PipelineCreated,
		CreatedAt:     now.Add(-24 * time.Hour),
		UpdatedAt:     now,
		RunCount:      0,
		Tags:          []string{"csv", "import", "analytics"},
		Orchestration: OrchestrationConfig{Owner: "ops", Queue: "low", MaxActiveRuns: 1, Concurrency: 2, PriorityWeight: 3, Retries: 1, RetryDelaySec: 60, TimeoutSec: 1200, SLASeconds: 7200, Catchup: false, DependsOnPast: false, AlertChannels: []string{"email"}},
	}
	e.pipelines[p3.ID] = p3

	// Pipeline 4: Kafka Stream → Elasticsearch
	p4 := &Pipeline{
		ID:          "etl-kafka-elastic",
		Name:        "Kafka Events → Elasticsearch",
		Description: "Consumes events from Kafka, transforms, and indexes in Elasticsearch",
		Steps: []Step{
			{ID: "s1", Name: "Consume Kafka Topic", Type: StepExtract, Connector: "kafka", Order: 1, Config: map[string]interface{}{"brokers": "localhost:9092", "topic": "user-events", "group_id": "etl-consumer"}},
			{ID: "s2", Name: "Filter by Type", Type: StepFilter, Connector: "", Order: 2, Config: map[string]interface{}{"condition": "event_type IN ('login', 'purchase', 'signup')"}},
			{ID: "s3", Name: "Transform to ES Doc", Type: StepTransform, Connector: "", Order: 3, Config: map[string]interface{}{"operations": []string{"flatten_json", "add_timestamp", "geo_resolve:ip"}}},
			{ID: "s4", Name: "Index in Elasticsearch", Type: StepLoad, Connector: "elasticsearch", Order: 4, Config: map[string]interface{}{"url": "http://localhost:9200", "index": "user-events-2026"}},
		},
		Status:        PipelineRunning,
		CreatedAt:     now.Add(-96 * time.Hour),
		UpdatedAt:     now,
		LastRunAt:     &now,
		RunCount:      1200,
		Tags:          []string{"streaming", "events", "search"},
		Orchestration: OrchestrationConfig{Owner: "streaming", Queue: "realtime", MaxActiveRuns: 3, Concurrency: 10, PriorityWeight: 9, Retries: 5, RetryDelaySec: 30, TimeoutSec: 900, SLASeconds: 600, Catchup: false, DependsOnPast: false, AlertChannels: []string{"slack", "webhook"}},
	}
	e.pipelines[p4.ID] = p4

	// Seed some historical runs
	e.seedRuns(p1, now)

	e.observability.PipelinesTotal = len(e.pipelines)
}

func (e *Engine) seedRuns(p *Pipeline, now time.Time) {
	statuses := []PipelineStatus{PipelineSuccess, PipelineSuccess, PipelineSuccess, PipelineFailed, PipelineSuccess}
	for i, status := range statuses {
		startTime := now.Add(-time.Duration(5-i) * time.Hour)
		finTime := startTime.Add(45 * time.Second)
		dur := finTime.Sub(startTime)

		run := &PipelineRun{
			ID:          fmt.Sprintf("run-seed-%d", i+1),
			PipelineID:  p.ID,
			Status:      status,
			StartedAt:   startTime,
			FinishedAt:  &finTime,
			Duration:    dur.String(),
			Trigger:     "schedule",
			RowsRead:    10000,
			RowsWritten: 9500,
			StepResults: []StepResult{
				{StepID: "s1", StepName: "Extract from MySQL", StepType: StepExtract, Status: "succeeded", StartedAt: startTime, FinishedAt: &finTime, Duration: "12s", RowsIn: 10000, RowsOut: 10000, Metrics: StepMetrics{BytesProcessed: 2560000, ThroughputRPS: 833}},
				{StepID: "s2", StepName: "Validate Schema", StepType: StepValidate, Status: "succeeded", StartedAt: startTime, FinishedAt: &finTime, Duration: "5s", RowsIn: 10000, RowsOut: 9800, RowsError: 200, Metrics: StepMetrics{BytesProcessed: 2500000, ThroughputRPS: 1960}},
				{StepID: "s3", StepName: "Transform Fields", StepType: StepTransform, Status: "succeeded", StartedAt: startTime, FinishedAt: &finTime, Duration: "8s", RowsIn: 9800, RowsOut: 9800, Metrics: StepMetrics{BytesProcessed: 2500000, ThroughputRPS: 1225}},
				{StepID: "s4", StepName: "Deduplicate", StepType: StepDedupe, Status: "succeeded", StartedAt: startTime, FinishedAt: &finTime, Duration: "6s", RowsIn: 9800, RowsOut: 9500, Metrics: StepMetrics{BytesProcessed: 2430000, ThroughputRPS: 1583}},
				{StepID: "s5", StepName: "Load to PostgreSQL", StepType: StepLoad, Status: "succeeded", StartedAt: startTime, FinishedAt: &finTime, Duration: "14s", RowsIn: 9500, RowsOut: 9500, Metrics: StepMetrics{BytesProcessed: 2430000, ThroughputRPS: 678}},
			},
		}
		if status == PipelineFailed {
			run.ErrorMsg = "connection timeout: PostgreSQL host unreachable"
			run.RowsWritten = 0
			run.RowsFailed = 9500
			run.StepResults[4].Status = "failed"
			run.StepResults[4].ErrorMsg = "connection timeout"
		}
		e.runs[run.ID] = run
	}

	// Update observability
	e.observability.RunsTotal = len(statuses)
	e.observability.RunsSuccess = 4
	e.observability.RunsFailed = 1
	e.observability.TotalRowsRead = 50000
	e.observability.TotalRowsWrite = 38000
	e.observability.AvgDuration = 45.0
}
