package tracing

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/auth"
	"github.com/gin-gonic/gin"
)

type TracingHandler struct {
	manager        TracingManager
	dualWriteStore TracingDualWriteStore
}

// NewTracingHandler creates handler
func NewTracingHandler(manager TracingManager) *TracingHandler {
	return &TracingHandler{manager: manager}
}

func (h *TracingHandler) writeIngestionAudit(c *gin.Context, audit *TraceIngestionAuditLog) {
	if audit == nil {
		return
	}

	if audit.Timestamp.IsZero() {
		audit.Timestamp = time.Now().UTC()
	}

	if audit.Username == "" {
		audit.Username = strings.TrimSpace(c.GetString("username"))
	}

	if rawUser, ok := c.Get("user"); ok {
		if claims, ok := rawUser.(*auth.Claims); ok && claims != nil {
			if audit.UserID == "" {
				audit.UserID = strings.TrimSpace(claims.Sub)
			}
			if audit.Username == "" {
				audit.Username = strings.TrimSpace(claims.PreferredUsername)
			}
		}
	}

	if audit.SourceIP == "" {
		audit.SourceIP = strings.TrimSpace(c.ClientIP())
	}
	if audit.UserAgent == "" && c.Request != nil {
		audit.UserAgent = strings.TrimSpace(c.Request.UserAgent())
	}
	if audit.Method == "" && c.Request != nil {
		audit.Method = strings.TrimSpace(c.Request.Method)
	}
	if audit.Path == "" && c.Request != nil && c.Request.URL != nil {
		audit.Path = strings.TrimSpace(c.Request.URL.Path)
	}

	if audit.RequestID == "" {
		audit.RequestID = strings.TrimSpace(c.GetHeader("X-Request-ID"))
	}
	if audit.RequestID == "" {
		audit.RequestID = strings.TrimSpace(c.GetHeader("X-Correlation-ID"))
	}

	_ = h.manager.RecordIngestionAudit(audit)
}

// IngestTrace handles POST /api/v1/tracing/traces.
func (h *TracingHandler) IngestTrace(c *gin.Context) {
	started := time.Now().UTC()

	var req Trace
	if err := c.ShouldBindJSON(&req); err != nil {
		h.writeIngestionAudit(c, &TraceIngestionAuditLog{
			ResourceType: "trace",
			Result:       "FAILURE",
			StatusCode:   http.StatusBadRequest,
			Message:      err.Error(),
			DurationMs:   time.Since(started).Milliseconds(),
		})
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	// Phase 3: reconciler-authoritative path
	if h.isAuthoritative() {
		resource := h.buildConfigResource(req.TenantID)
		if h.dualWriteStore != nil {
			if err := h.dualWriteStore.Create(c.Request.Context(), resource); err != nil {
				_ = h.dualWriteStore.Update(c.Request.Context(), resource)
			}
		}
		c.JSON(http.StatusAccepted, MessageResponse{Status: "Pending", Message: "trace config resource created", Name: req.ID})
		return
	}

	trace, err := h.manager.IngestTrace(&req)
	if err != nil {
		h.writeIngestionAudit(c, &TraceIngestionAuditLog{
			TenantID:     req.TenantID,
			ResourceType: "trace",
			ResourceID:   req.ID,
			Result:       "FAILURE",
			StatusCode:   http.StatusInternalServerError,
			Message:      err.Error(),
			DurationMs:   time.Since(started).Milliseconds(),
		})
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	h.writeIngestionAudit(c, &TraceIngestionAuditLog{
		TenantID:     trace.TenantID,
		ResourceType: "trace",
		ResourceID:   trace.ID,
		Result:       "SUCCESS",
		StatusCode:   http.StatusCreated,
		Message:      "trace ingested",
		DurationMs:   time.Since(started).Milliseconds(),
	})

	c.JSON(http.StatusCreated, trace)

	// Phase 2: dual-write tracing config to etcd
	h.dualWriteConfig(trace.TenantID)
}

// IngestSpan handles POST /api/v1/tracing/spans.
func (h *TracingHandler) IngestSpan(c *gin.Context) {
	started := time.Now().UTC()

	var req Span
	if err := c.ShouldBindJSON(&req); err != nil {
		h.writeIngestionAudit(c, &TraceIngestionAuditLog{
			ResourceType: "span",
			Result:       "FAILURE",
			StatusCode:   http.StatusBadRequest,
			Message:      err.Error(),
			DurationMs:   time.Since(started).Milliseconds(),
		})
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	span, err := h.manager.IngestSpan(&req)
	if err != nil {
		h.writeIngestionAudit(c, &TraceIngestionAuditLog{
			TenantID:     req.TenantID,
			ResourceType: "span",
			ResourceID:   req.ID,
			Result:       "FAILURE",
			StatusCode:   http.StatusInternalServerError,
			Message:      err.Error(),
			DurationMs:   time.Since(started).Milliseconds(),
		})
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	h.writeIngestionAudit(c, &TraceIngestionAuditLog{
		TenantID:     span.TenantID,
		ResourceType: "span",
		ResourceID:   span.ID,
		Result:       "SUCCESS",
		StatusCode:   http.StatusCreated,
		Message:      "span ingested",
		DurationMs:   time.Since(started).Milliseconds(),
	})

	c.JSON(http.StatusCreated, span)
}

// ListIngestionAudits handles GET /api/v1/tracing/ingestion/audit.
func (h *TracingHandler) ListIngestionAudits(c *gin.Context) {
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, MessageResponse{Error: "limit must be a positive integer"})
			return
		}
		limit = parsed
	}

	filter := &TraceIngestionAuditFilter{
		TenantID:     strings.TrimSpace(c.Query("tenantId")),
		Username:     strings.TrimSpace(c.Query("username")),
		ResourceType: strings.TrimSpace(c.Query("resourceType")),
		Result:       strings.TrimSpace(c.Query("result")),
		Limit:        limit,
	}

	logs, err := h.manager.ListIngestionAudits(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs, "count": len(logs)})
}

// GetTrace handles GET /api/v1/traces/:traceId
func (h *TracingHandler) GetTrace(c *gin.Context) {
	traceID := c.Param("traceId")
	trace, err := h.manager.GetTrace(traceID)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "trace not found"})
		return
	}

	c.JSON(http.StatusOK, trace)
}

// SearchTraces handles GET /api/v1/traces/search
func (h *TracingHandler) SearchTraces(c *gin.Context) {
	var req TraceSearchRequest
	if err := c.BindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	results, err := h.manager.SearchTraces(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"traces": results, "count": len(results)})
}

// GetSpan handles GET /api/v1/spans/:spanId
func (h *TracingHandler) GetSpan(c *gin.Context) {
	spanID := c.Param("spanId")
	span, err := h.manager.GetSpan(spanID)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "span not found"})
		return
	}

	c.JSON(http.StatusOK, span)
}

// GetServiceMap handles GET /api/v1/service-map
func (h *TracingHandler) GetServiceMap(c *gin.Context) {
	tenantID := c.Query("tenantId")
	serviceMap, err := h.manager.GetServiceMap(tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, serviceMap)
}

// GetServiceMetrics handles GET /api/v1/services/:service/metrics
func (h *TracingHandler) GetServiceMetrics(c *gin.Context) {
	service := c.Param("service")
	metrics, err := h.manager.GetServiceMetrics(service)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// GetOperationMetrics handles GET /api/v1/services/:service/operations/:operation/metrics
func (h *TracingHandler) GetOperationMetrics(c *gin.Context) {
	service := c.Param("service")
	operation := c.Param("operation")

	metrics, err := h.manager.GetOperationMetrics(service, operation)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// ListServices handles GET /api/v1/services
func (h *TracingHandler) ListServices(c *gin.Context) {
	tenantID := c.Query("tenantId")
	services, err := h.manager.ListServices(tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"services": services, "count": len(services)})
}

// GetErrorAnalysis handles GET /api/v1/errors/analysis
func (h *TracingHandler) GetErrorAnalysis(c *gin.Context) {
	tenantID := c.Query("tenantId")
	service := c.Query("service")

	analysis, err := h.manager.GetErrorAnalysis(tenantID, service)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, analysis)
}

// RegisterTracingRoutes registers all tracing routes
func RegisterTracingRoutes(router *gin.Engine, manager TracingManager) {
	handler := NewTracingHandler(manager)

	group := router.Group("/api/v1")
	{
		group.POST("/traces", handler.IngestTrace)
		group.GET("/traces/:traceId", handler.GetTrace)
		group.GET("/traces/search", handler.SearchTraces)
		group.POST("/spans", handler.IngestSpan)
		group.GET("/spans/:spanId", handler.GetSpan)
		group.GET("/service-map", handler.GetServiceMap)
		group.GET("/services", handler.ListServices)
		group.GET("/services/:service/metrics", handler.GetServiceMetrics)
		group.GET("/services/:service/operations/:operation/metrics", handler.GetOperationMetrics)
		group.GET("/errors/analysis", handler.GetErrorAnalysis)
		group.GET("/tracing/ingestion/audit", handler.ListIngestionAudits)
	}
}

// TracingManager interface
type TracingManager interface {
	IngestTrace(trace *Trace) (*Trace, error)
	IngestSpan(span *Span) (*Span, error)
	RecordIngestionAudit(entry *TraceIngestionAuditLog) error
	ListIngestionAudits(filter *TraceIngestionAuditFilter) ([]*TraceIngestionAuditLog, error)

	GetTrace(traceID string) (*Trace, error)
	SearchTraces(req *TraceSearchRequest) ([]*TraceSearchResult, error)
	GetSpan(spanID string) (*Span, error)
	GetServiceMap(tenantID string) (*ServiceMap, error)
	GetServiceMetrics(service string) (*TraceMetrics, error)
	GetOperationMetrics(service, operation string) (*SpanMetrics, error)
	ListServices(tenantID string) ([]*ServiceInfo, error)
	GetErrorAnalysis(tenantID, service string) (*ErrorAnalysis, error)
}
