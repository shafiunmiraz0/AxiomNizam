package cdc

import (
	"net/http"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/etl"
	"github.com/gin-gonic/gin"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ==============================================
// CDC & ETL Handler — Full REST API
// Dynamic pipeline management with observability
// ==============================================

type Handler struct {
	etlEngine *etl.Engine
	cdcEngine *PipelineEngine
}

func NewHandler(etcd ...*clientv3.Client) *Handler {
	cdcCore := NewChangeDataCapture(etcd...)
	return &Handler{
		etlEngine: etl.NewEngine(etcd...),
		cdcEngine: NewPipelineEngine(cdcCore, etcd...),
	}
}

// ================
// ETL Endpoints
// ================

// ListETLPipelines GET /api/v1/etl/pipelines
func (h *Handler) ListETLPipelines(c *gin.Context) {
	pipelines := h.etlEngine.ListPipelines()
	c.JSON(http.StatusOK, ETLPipelineListResponse{
		Status:    "success",
		Pipelines: pipelines,
		Total:     len(pipelines),
	})
}

// GetETLPipeline GET /api/v1/etl/pipelines/:id
func (h *Handler) GetETLPipeline(c *gin.Context) {
	id := c.Param("id")
	p, ok := h.etlEngine.GetPipeline(id)
	if !ok {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "pipeline not found"})
		return
	}
	c.JSON(http.StatusOK, ETLPipelineResponse{Status: "success", Pipeline: p})
}

// CreateETLPipeline POST /api/v1/etl/pipelines
func (h *Handler) CreateETLPipeline(c *gin.Context) {
	var p etl.Pipeline
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	if p.Name == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "name is required"})
		return
	}
	if err := h.etlEngine.CreatePipeline(&p); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, ETLPipelineResponse{Status: "success", Pipeline: p})
}

// UpdateETLPipeline PUT /api/v1/etl/pipelines/:id
func (h *Handler) UpdateETLPipeline(c *gin.Context) {
	id := c.Param("id")
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	if err := h.etlEngine.UpdatePipeline(id, updates); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
		return
	}
	p, _ := h.etlEngine.GetPipeline(id)
	c.JSON(http.StatusOK, ETLPipelineResponse{Status: "success", Pipeline: p})
}

// DeleteETLPipeline DELETE /api/v1/etl/pipelines/:id
func (h *Handler) DeleteETLPipeline(c *gin.Context) {
	id := c.Param("id")
	if err := h.etlEngine.DeletePipeline(id); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "pipeline deleted"})
}

// RunETLPipeline POST /api/v1/etl/pipelines/:id/run
func (h *Handler) RunETLPipeline(c *gin.Context) {
	id := c.Param("id")
	run, err := h.etlEngine.RunPipeline(c.Request.Context(), id, "manual")
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ETLRunResponse{Status: "success", Run: run})
}

// ListETLRuns GET /api/v1/etl/runs
func (h *Handler) ListETLRuns(c *gin.Context) {
	pipelineID := c.Query("pipeline_id")
	runs := h.etlEngine.ListRuns(pipelineID)
	c.JSON(http.StatusOK, ETLRunListResponse{Status: "success", Runs: runs, Total: len(runs)})
}

// GetETLRun GET /api/v1/etl/runs/:id
func (h *Handler) GetETLRun(c *gin.Context) {
	id := c.Param("id")
	run, ok := h.etlEngine.GetRun(id)
	if !ok {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "run not found"})
		return
	}
	c.JSON(http.StatusOK, ETLRunResponse{Status: "success", Run: run})
}

// GetETLConnectors GET /api/v1/etl/connectors
func (h *Handler) GetETLConnectors(c *gin.Context) {
	connectors := h.etlEngine.GetConnectors()
	q := strings.TrimSpace(strings.ToLower(c.Query("q")))
	category := strings.TrimSpace(strings.ToLower(c.Query("category")))

	filtered := make([]etl.ConnectorType, 0, len(connectors))
	for _, connector := range connectors {
		if category != "" && strings.ToLower(connector.Category) != category {
			continue
		}
		if q != "" {
			name := strings.ToLower(connector.Name)
			id := strings.ToLower(connector.ID)
			desc := strings.ToLower(connector.Description)
			if !strings.Contains(name, q) && !strings.Contains(id, q) && !strings.Contains(desc, q) {
				continue
			}
		}
		filtered = append(filtered, connector)
	}

	categories := map[string]int{}
	for _, connector := range connectors {
		categories[connector.Category]++
	}

	c.JSON(http.StatusOK, ETLConnectorListResponse{
		Status:     "success",
		Connectors: filtered,
		Total:      len(filtered),
		Categories: categories,
	})
}

// CreateETLConnector POST /api/v1/etl/connectors
func (h *Handler) CreateETLConnector(c *gin.Context) {
	var connector etl.ConnectorType
	if err := c.ShouldBindJSON(&connector); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	if err := h.etlEngine.AddConnector(connector); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, ETLConnectorResponse{Status: "success", Connector: connector})
}

// UpdateETLConnector PUT /api/v1/etl/connectors/:id
func (h *Handler) UpdateETLConnector(c *gin.Context) {
	var updates etl.ConnectorType
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	updated, err := h.etlEngine.UpdateConnector(c.Param("id"), updates)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ETLConnectorResponse{Status: "success", Connector: updated})
}

// DeleteETLConnector DELETE /api/v1/etl/connectors/:id
func (h *Handler) DeleteETLConnector(c *gin.Context) {
	if err := h.etlEngine.DeleteConnector(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "connector deleted"})
}

// GetETLConnectorCatalog GET /api/v1/etl/connectors/catalog
func (h *Handler) GetETLConnectorCatalog(c *gin.Context) {
	connectors := h.etlEngine.GetConnectors()
	byCategory := map[string][]etl.ConnectorType{}
	for _, connector := range connectors {
		byCategory[connector.Category] = append(byCategory[connector.Category], connector)
	}

	c.JSON(http.StatusOK, ETLConnectorCatalogResponse{
		Status:     "success",
		Connectors: connectors,
		ByCategory: byCategory,
		Total:      len(connectors),
	})
}

// GetETLOrchestrationCapabilities GET /api/v1/etl/orchestration/capabilities
func (h *Handler) GetETLOrchestrationCapabilities(c *gin.Context) {
	c.JSON(http.StatusOK, ETLCapabilitiesResponse{
		Status:       "success",
		Capabilities: h.etlEngine.GetOrchestrationCapabilities(),
	})
}

// GetETLBlueprints GET /api/v1/etl/blueprints
func (h *Handler) GetETLBlueprints(c *gin.Context) {
	c.JSON(http.StatusOK, ETLBlueprintsResponse{
		Status:     "success",
		Blueprints: h.etlEngine.GetPipelineBlueprints(),
	})
}

// GetETLObservability GET /api/v1/etl/observability
func (h *Handler) GetETLObservability(c *gin.Context) {
	obs := h.etlEngine.GetObservability()
	c.JSON(http.StatusOK, ETLObservabilityResponse{Status: "success", Observability: obs})
}

// ================
// CDC Endpoints
// ================

// ListCDCPipelines GET /api/v1/cdc/pipelines
func (h *Handler) ListCDCPipelines(c *gin.Context) {
	pipelines := h.cdcEngine.ListPipelines()
	c.JSON(http.StatusOK, CDCPipelineListResponse{Status: "success", Pipelines: pipelines, Total: len(pipelines)})
}

// GetCDCPipeline GET /api/v1/cdc/pipelines/:id
func (h *Handler) GetCDCPipeline(c *gin.Context) {
	id := c.Param("id")
	p, ok := h.cdcEngine.GetPipeline(id)
	if !ok {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "pipeline not found"})
		return
	}
	c.JSON(http.StatusOK, CDCPipelineResponse{Status: "success", Pipeline: p})
}

// CreateCDCPipeline POST /api/v1/cdc/pipelines
func (h *Handler) CreateCDCPipeline(c *gin.Context) {
	var p CDCPipeline
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	if p.Name == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "name is required"})
		return
	}
	if err := h.cdcEngine.CreatePipeline(&p); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, CDCPipelineResponse{Status: "success", Pipeline: p})
}

// UpdateCDCPipeline PUT /api/v1/cdc/pipelines/:id
func (h *Handler) UpdateCDCPipeline(c *gin.Context) {
	id := c.Param("id")
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	if err := h.cdcEngine.UpdatePipeline(id, updates); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
		return
	}
	p, _ := h.cdcEngine.GetPipeline(id)
	c.JSON(http.StatusOK, CDCPipelineResponse{Status: "success", Pipeline: p})
}

// DeleteCDCPipeline DELETE /api/v1/cdc/pipelines/:id
func (h *Handler) DeleteCDCPipeline(c *gin.Context) {
	id := c.Param("id")
	if err := h.cdcEngine.DeletePipeline(id); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "pipeline deleted"})
}

// StartCDCPipeline POST /api/v1/cdc/pipelines/:id/start
func (h *Handler) StartCDCPipeline(c *gin.Context) {
	id := c.Param("id")
	if err := h.cdcEngine.StartPipeline(id); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
		return
	}
	p, _ := h.cdcEngine.GetPipeline(id)
	c.JSON(http.StatusOK, CDCPipelineActionResponse{Status: "success", Message: "pipeline started", Pipeline: p})
}

// PauseCDCPipeline POST /api/v1/cdc/pipelines/:id/pause
func (h *Handler) PauseCDCPipeline(c *gin.Context) {
	id := c.Param("id")
	if err := h.cdcEngine.PausePipeline(id); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
		return
	}
	p, _ := h.cdcEngine.GetPipeline(id)
	c.JSON(http.StatusOK, CDCPipelineActionResponse{Status: "success", Message: "pipeline paused", Pipeline: p})
}

// StopCDCPipeline POST /api/v1/cdc/pipelines/:id/stop
func (h *Handler) StopCDCPipeline(c *gin.Context) {
	id := c.Param("id")
	if err := h.cdcEngine.StopPipeline(id); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
		return
	}
	p, _ := h.cdcEngine.GetPipeline(id)
	c.JSON(http.StatusOK, CDCPipelineActionResponse{Status: "success", Message: "pipeline stopped", Pipeline: p})
}

// GetCDCSourceTypes GET /api/v1/cdc/sources
func (h *Handler) GetCDCSourceTypes(c *gin.Context) {
	c.JSON(http.StatusOK, CDCSourceTypesResponse{Status: "success", Sources: h.cdcEngine.GetSourceTypes()})
}

// GetCDCSinkTypes GET /api/v1/cdc/sinks
func (h *Handler) GetCDCSinkTypes(c *gin.Context) {
	c.JSON(http.StatusOK, CDCSinkTypesResponse{Status: "success", Sinks: h.cdcEngine.GetSinkTypes()})
}

// GetCDCObservability GET /api/v1/cdc/observability
func (h *Handler) GetCDCObservability(c *gin.Context) {
	obs := h.cdcEngine.GetObservability()
	c.JSON(http.StatusOK, CDCObservabilityResponse{Status: "success", Observability: obs})
}

// ================
// Combined Endpoints
// ================

// GetPlatformOverview GET /api/v1/data-platform/overview
func (h *Handler) GetPlatformOverview(c *gin.Context) {
	etlObs := h.etlEngine.GetObservability()
	cdcObs := h.cdcEngine.GetObservability()
	etlPipelines := h.etlEngine.ListPipelines()
	cdcPipelines := h.cdcEngine.ListPipelines()

	c.JSON(http.StatusOK, PlatformOverviewResponse{
		Status: "success",
		Overview: PlatformOverview{
			ETL: ETLOverview{
				PipelinesTotal:     etlObs.PipelinesTotal,
				RunsTotal:          etlObs.RunsTotal,
				RunsSuccess:        etlObs.RunsSuccess,
				RunsFailed:         etlObs.RunsFailed,
				RunsRunning:        etlObs.RunsRunning,
				TotalRowsRead:      etlObs.TotalRowsRead,
				TotalRowsWritten:   etlObs.TotalRowsWrite,
				AvgDurationSeconds: etlObs.AvgDuration,
				Pipelines:          etlPipelines,
			},
			CDC: CDCOverview{
				PipelinesTotal:  cdcObs.PipelinesTotal,
				PipelinesActive: cdcObs.PipelinesActive,
				PipelinesPaused: cdcObs.PipelinesPaused,
				PipelinesFailed: cdcObs.PipelinesFailed,
				TotalEvents:     cdcObs.TotalEvents,
				EventsPerSecond: cdcObs.EventsPerSecond,
				TotalErrors:     cdcObs.TotalErrors,
				ErrorRate:       cdcObs.ErrorRate,
				AvgLagMs:        cdcObs.AvgLagMs,
				Pipelines:       cdcPipelines,
			},
			Connectors: ConnectorSummary{
				ETLConnectors: len(h.etlEngine.GetConnectors()),
				CDCSources:    len(h.cdcEngine.GetSourceTypes()),
				CDCSinks:      len(h.cdcEngine.GetSinkTypes()),
			},
		},
	})
}
