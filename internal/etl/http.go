package etl

import (
	"net/http"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/etl/audit"
	"axiomnizam.bitbd.net/axiomnizam/internal/etl/metrics"
	"github.com/gin-gonic/gin"
)

// Handler provides HTTP endpoints for the ETL module.
type Handler struct {
	engine      *Engine
	auditLogger *audit.Logger
}

// NewHandler creates a new ETL HTTP handler.
func NewHandler(engine *Engine, auditLog *audit.Logger) *Handler {
	return &Handler{
		engine:      engine,
		auditLogger: auditLog,
	}
}

// RegisterRoutes registers ETL routes on the given router group.
// adminMW is an optional middleware applied to write endpoints (POST/PUT/DELETE).
// Pass nil to skip admin enforcement.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, adminMW ...gin.HandlerFunc) {
	rg.GET("/pipelines", h.ListPipelines)
	rg.GET("/pipelines/:id", h.GetPipeline)
	rg.POST("/pipelines", append(adminMW, h.CreatePipeline)...)
	rg.PUT("/pipelines/:id", append(adminMW, h.UpdatePipeline)...)
	rg.DELETE("/pipelines/:id", append(adminMW, h.DeletePipeline)...)
	rg.POST("/pipelines/:id/run", append(adminMW, h.RunPipeline)...)
	rg.GET("/runs", h.ListRuns)
	rg.GET("/runs/:id", h.GetRun)
	rg.POST("/connectors", append(adminMW, h.CreateConnector)...)
	rg.PUT("/connectors/:id", append(adminMW, h.UpdateConnector)...)
	rg.DELETE("/connectors/:id", append(adminMW, h.DeleteConnector)...)
	rg.GET("/connectors", h.GetConnectors)
	rg.GET("/connectors/catalog", h.GetConnectorCatalog)
	rg.GET("/orchestration/capabilities", h.GetOrchestrationCapabilities)
	rg.GET("/blueprints", h.GetBlueprints)
	rg.GET("/observability", h.GetObservability)
	rg.GET("/audit", h.GetAuditLog)
	rg.GET("/metrics", h.GetMetrics)
}

// --- Pipeline Endpoints ---

// ListPipelines GET /api/v1/etl/pipelines
func (h *Handler) ListPipelines(c *gin.Context) {
	pipelines := h.engine.ListPipelines()
	items := make([]PipelineListItem, 0, len(pipelines))
	for _, p := range pipelines {
		items = append(items, PipelineListItem{
			ID:            p.ID,
			Name:          p.Name,
			Description:   p.Description,
			Status:        p.Status,
			Schedule:      p.Schedule,
			Steps:         len(p.Steps),
			RunCount:      p.RunCount,
			Tags:          p.Tags,
			Orchestration: p.Orchestration,
			CreatedAt:     p.CreatedAt,
			UpdatedAt:     p.UpdatedAt,
			LastRunAt:     p.LastRunAt,
		})
	}
	c.JSON(http.StatusOK, ETLPipelineListResponse{
		Status:    "success",
		Pipelines: items,
		Total:     len(items),
	})
}

// GetPipeline GET /api/v1/etl/pipelines/:id
func (h *Handler) GetPipeline(c *gin.Context) {
	id := c.Param("id")
	p, ok := h.engine.GetPipeline(id)
	if !ok {
		c.JSON(http.StatusNotFound, ETLMessageResponse{Error: "pipeline not found"})
		return
	}
	c.JSON(http.StatusOK, ETLPipelineResponse{Status: "success", Pipeline: p})
}

// CreatePipeline POST /api/v1/etl/pipelines
func (h *Handler) CreatePipeline(c *gin.Context) {
	var p Pipeline
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, ETLMessageResponse{Error: err.Error()})
		return
	}
	if p.Name == "" {
		c.JSON(http.StatusBadRequest, ETLMessageResponse{Error: "name is required"})
		return
	}
	if err := h.engine.CreatePipeline(&p); err != nil {
		c.JSON(http.StatusInternalServerError, ETLMessageResponse{Error: err.Error()})
		return
	}
	if h.auditLogger != nil {
		h.auditLogger.LogPipeline(audit.ActionPipelineCreated, p.ID, "pipeline created: "+p.Name)
	}
	c.JSON(http.StatusCreated, ETLPipelineResponse{Status: "success", Pipeline: &p})
}

// UpdatePipeline PUT /api/v1/etl/pipelines/:id
func (h *Handler) UpdatePipeline(c *gin.Context) {
	id := c.Param("id")
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, ETLMessageResponse{Error: err.Error()})
		return
	}
	if err := h.engine.UpdatePipeline(id, updates); err != nil {
		c.JSON(http.StatusNotFound, ETLMessageResponse{Error: err.Error()})
		return
	}
	if h.auditLogger != nil {
		h.auditLogger.LogPipeline(audit.ActionPipelineUpdated, id, "pipeline updated")
	}
	p, _ := h.engine.GetPipeline(id)
	c.JSON(http.StatusOK, ETLPipelineResponse{Status: "success", Pipeline: p})
}

// DeletePipeline DELETE /api/v1/etl/pipelines/:id
func (h *Handler) DeletePipeline(c *gin.Context) {
	id := c.Param("id")
	if err := h.engine.DeletePipeline(id); err != nil {
		c.JSON(http.StatusNotFound, ETLMessageResponse{Error: err.Error()})
		return
	}
	if h.auditLogger != nil {
		h.auditLogger.LogPipeline(audit.ActionPipelineDeleted, id, "pipeline deleted")
	}
	c.JSON(http.StatusOK, ETLMessageResponse{Message: "pipeline deleted"})
}

// RunPipeline POST /api/v1/etl/pipelines/:id/run
func (h *Handler) RunPipeline(c *gin.Context) {
	id := c.Param("id")
	run, err := h.engine.RunPipeline(c.Request.Context(), id, "manual")
	if err != nil {
		c.JSON(http.StatusNotFound, ETLMessageResponse{Error: err.Error()})
		return
	}
	if h.auditLogger != nil {
		h.auditLogger.LogRun(audit.ActionPipelineRun, id, run.ID, "pipeline run triggered")
	}
	c.JSON(http.StatusOK, ETLRunResponse{Status: "success", Run: run})
}

// --- Run Endpoints ---

// ListRuns GET /api/v1/etl/runs
func (h *Handler) ListRuns(c *gin.Context) {
	pipelineID := c.Query("pipeline_id")
	runs := h.engine.ListRuns(pipelineID)
	items := make([]RunListItem, 0, len(runs))
	for _, r := range runs {
		items = append(items, RunListItem{
			ID:          r.ID,
			PipelineID:  r.PipelineID,
			Status:      r.Status,
			Trigger:     r.Trigger,
			StartedAt:   r.StartedAt,
			FinishedAt:  r.FinishedAt,
			Duration:    r.Duration,
			RowsRead:    r.RowsRead,
			RowsWritten: r.RowsWritten,
			RowsFailed:  r.RowsFailed,
			ErrorMsg:    r.ErrorMsg,
		})
	}
	c.JSON(http.StatusOK, ETLRunListResponse{Status: "success", Runs: items, Total: len(items)})
}

// GetRun GET /api/v1/etl/runs/:id
func (h *Handler) GetRun(c *gin.Context) {
	id := c.Param("id")
	run, ok := h.engine.GetRun(id)
	if !ok {
		c.JSON(http.StatusNotFound, ETLMessageResponse{Error: "run not found"})
		return
	}
	c.JSON(http.StatusOK, ETLRunResponse{Status: "success", Run: run})
}

// --- Connector Endpoints ---

// GetConnectors GET /api/v1/etl/connectors
func (h *Handler) GetConnectors(c *gin.Context) {
	connectors := h.engine.GetConnectors()
	q := strings.TrimSpace(strings.ToLower(c.Query("q")))
	category := strings.TrimSpace(strings.ToLower(c.Query("category")))

	filtered := make([]ConnectorType, 0, len(connectors))
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

// CreateConnector POST /api/v1/etl/connectors
func (h *Handler) CreateConnector(c *gin.Context) {
	var connector ConnectorType
	if err := c.ShouldBindJSON(&connector); err != nil {
		c.JSON(http.StatusBadRequest, ETLMessageResponse{Error: err.Error()})
		return
	}
	if err := h.engine.AddConnector(connector); err != nil {
		c.JSON(http.StatusBadRequest, ETLMessageResponse{Error: err.Error()})
		return
	}
	if h.auditLogger != nil {
		h.auditLogger.LogConnector(audit.ActionConnectorCreated, connector.ID, "connector created: "+connector.Name)
	}
	c.JSON(http.StatusCreated, ETLConnectorResponse{Status: "success", Connector: &connector})
}

// UpdateConnector PUT /api/v1/etl/connectors/:id
func (h *Handler) UpdateConnector(c *gin.Context) {
	var updates ConnectorType
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, ETLMessageResponse{Error: err.Error()})
		return
	}
	updated, err := h.engine.UpdateConnector(c.Param("id"), updates)
	if err != nil {
		c.JSON(http.StatusNotFound, ETLMessageResponse{Error: err.Error()})
		return
	}
	if h.auditLogger != nil {
		h.auditLogger.LogConnector(audit.ActionConnectorUpdated, c.Param("id"), "connector updated")
	}
	c.JSON(http.StatusOK, ETLConnectorResponse{Status: "success", Connector: updated})
}

// DeleteConnector DELETE /api/v1/etl/connectors/:id
func (h *Handler) DeleteConnector(c *gin.Context) {
	id := c.Param("id")
	if err := h.engine.DeleteConnector(id); err != nil {
		c.JSON(http.StatusNotFound, ETLMessageResponse{Error: err.Error()})
		return
	}
	if h.auditLogger != nil {
		h.auditLogger.LogConnector(audit.ActionConnectorDeleted, id, "connector deleted")
	}
	c.JSON(http.StatusOK, ETLMessageResponse{Message: "connector deleted"})
}

// GetConnectorCatalog GET /api/v1/etl/connectors/catalog
func (h *Handler) GetConnectorCatalog(c *gin.Context) {
	connectors := h.engine.GetConnectors()
	byCategory := map[string][]ConnectorType{}
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

// --- Orchestration / Blueprint Endpoints ---

// GetOrchestrationCapabilities GET /api/v1/etl/orchestration/capabilities
func (h *Handler) GetOrchestrationCapabilities(c *gin.Context) {
	c.JSON(http.StatusOK, ETLCapabilitiesResponse{
		Status:       "success",
		Capabilities: h.engine.GetOrchestrationCapabilities(),
	})
}

// GetBlueprints GET /api/v1/etl/blueprints
func (h *Handler) GetBlueprints(c *gin.Context) {
	c.JSON(http.StatusOK, ETLBlueprintsResponse{
		Status:     "success",
		Blueprints: h.engine.GetPipelineBlueprints(),
	})
}

// --- Observability ---

// GetObservability GET /api/v1/etl/observability
func (h *Handler) GetObservability(c *gin.Context) {
	obs := h.engine.GetObservability()
	c.JSON(http.StatusOK, ETLObservabilityResponse{Status: "success", Observability: obs})
}

// --- Audit ---

// GetAuditLog GET /api/v1/etl/audit
func (h *Handler) GetAuditLog(c *gin.Context) {
	if h.auditLogger == nil {
		c.JSON(http.StatusOK, ETLAuditListResponse{Status: "success", Events: nil, Total: 0})
		return
	}
	events := h.auditLogger.List()
	c.JSON(http.StatusOK, ETLAuditListResponse{Status: "success", Events: events, Total: len(events)})
}

// --- Metrics ---

// GetMetrics GET /api/v1/etl/metrics
func (h *Handler) GetMetrics(c *gin.Context) {
	snapshot := metrics.Collector.Snapshot()
	c.JSON(http.StatusOK, ETLMetricsResponse{Status: "success", Metrics: snapshot})
}
