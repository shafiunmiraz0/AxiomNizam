package etl

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/etl/audit"
	"axiomnizam.bitbd.net/axiomnizam/internal/etl/metrics"
)

// --- Pipeline DTOs ---

// PipelineListItem is a typed entry in ListETLPipelines.
type PipelineListItem struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Description   string              `json:"description"`
	Status        PipelineStatus      `json:"status"`
	Schedule      string              `json:"schedule,omitempty"`
	Steps         int                 `json:"steps"`
	RunCount      int                 `json:"run_count"`
	Tags          []string            `json:"tags,omitempty"`
	Orchestration OrchestrationConfig `json:"orchestration,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
	LastRunAt     *time.Time          `json:"last_run_at,omitempty"`
}

// ETLPipelineListResponse is the typed response for GET /api/v1/etl/pipelines.
type ETLPipelineListResponse struct {
	Status    string             `json:"status"`
	Pipelines []PipelineListItem `json:"pipelines"`
	Total     int                `json:"total"`
}

// ETLPipelineResponse is the typed response for a single pipeline.
type ETLPipelineResponse struct {
	Status   string    `json:"status"`
	Pipeline *Pipeline `json:"pipeline"`
}

// --- Run DTOs ---

// RunListItem is a typed entry in ListETLRuns.
type RunListItem struct {
	ID          string         `json:"id"`
	PipelineID  string         `json:"pipeline_id"`
	Status      PipelineStatus `json:"status"`
	Trigger     string         `json:"trigger"`
	StartedAt   time.Time      `json:"started_at"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
	Duration    string         `json:"duration,omitempty"`
	RowsRead    int64          `json:"rows_read"`
	RowsWritten int64          `json:"rows_written"`
	RowsFailed  int64          `json:"rows_failed"`
	ErrorMsg    string         `json:"error_msg,omitempty"`
}

// ETLRunListResponse is the typed response for GET /api/v1/etl/runs.
type ETLRunListResponse struct {
	Status string        `json:"status"`
	Runs   []RunListItem `json:"runs"`
	Total  int           `json:"total"`
}

// ETLRunResponse is the typed response for a single run.
type ETLRunResponse struct {
	Status string       `json:"status"`
	Run    *PipelineRun `json:"run"`
}

// --- Connector DTOs ---

// ETLConnectorListResponse is the typed response for GET /api/v1/etl/connectors.
type ETLConnectorListResponse struct {
	Status     string          `json:"status"`
	Connectors []ConnectorType `json:"connectors"`
	Total      int             `json:"total"`
	Categories map[string]int  `json:"categories"`
}

// ETLConnectorResponse is the typed response for a single connector.
type ETLConnectorResponse struct {
	Status    string         `json:"status"`
	Connector *ConnectorType `json:"connector"`
}

// ETLConnectorCatalogResponse is the typed response for the connector catalog.
type ETLConnectorCatalogResponse struct {
	Status     string                       `json:"status"`
	Connectors []ConnectorType              `json:"connectors"`
	ByCategory map[string][]ConnectorType   `json:"by_category"`
	Total      int                          `json:"total"`
}

// --- Orchestration / Blueprint DTOs ---

// ETLCapabilitiesResponse is the typed response for orchestration capabilities.
type ETLCapabilitiesResponse struct {
	Status       string                   `json:"status"`
	Capabilities []OrchestrationCapability `json:"capabilities"`
}

// ETLBlueprintsResponse is the typed response for pipeline blueprints.
type ETLBlueprintsResponse struct {
	Status     string              `json:"status"`
	Blueprints []PipelineBlueprint `json:"blueprints"`
}

// --- Observability DTOs ---

// ETLObservabilityResponse is the typed response for GET /api/v1/etl/observability.
type ETLObservabilityResponse struct {
	Status        string            `json:"status"`
	Observability *ETLObservability `json:"observability"`
}

// --- Generic DTOs ---

// ETLMessageResponse is a generic ack/error response.
type ETLMessageResponse struct {
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// --- Audit DTOs ---

// ETLAuditListResponse is the typed response for GET /api/v1/etl/audit.
type ETLAuditListResponse struct {
	Status string       `json:"status"`
	Events []audit.Event `json:"events"`
	Total  int          `json:"total"`
}

// --- Metrics DTOs ---

// ETLMetricsResponse is the typed response for GET /api/v1/etl/metrics.
type ETLMetricsResponse struct {
	Status  string                `json:"status"`
	Metrics metrics.MetricsSnapshot `json:"metrics"`
}
