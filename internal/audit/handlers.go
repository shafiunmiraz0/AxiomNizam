package audit

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type AuditHandler struct {
	logger         AuditLogger
	dualWriteStore AuditDualWriteStore
}

// NewAuditHandler creates handler
func NewAuditHandler(logger AuditLogger) *AuditHandler {
	return &AuditHandler{logger: logger}
}

// LogAction handles POST /api/v1/audit/logs
func (h *AuditHandler) LogAction(c *gin.Context) {
	var req struct {
		TenantID   string      `json:"tenantId" binding:"required"`
		UserID     string      `json:"userId" binding:"required"`
		Username   string      `json:"username"`
		Action     AuditAction `json:"action" binding:"required"`
		ResourceID string      `json:"resourceId"`
		Result     AuditResult `json:"result" binding:"required"`
		SourceIP   string      `json:"sourceIp"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	// Phase 3: reconciler-authoritative path
	if h.isAuthoritative() {
		resource := h.buildPolicyResource(req.TenantID)
		if h.dualWriteStore != nil {
			if err := h.dualWriteStore.Create(c.Request.Context(), resource); err != nil {
				_ = h.dualWriteStore.Update(c.Request.Context(), resource)
			}
		}
		c.JSON(http.StatusAccepted, MessageResponse{Status: "Pending", Message: "audit policy resource created", Name: req.TenantID})
		return
	}

	// Old path: direct logger call
	log := &AuditLog{
		TenantID:   req.TenantID,
		UserID:     req.UserID,
		Username:   req.Username,
		Action:     req.Action,
		ResourceID: req.ResourceID,
		Result:     req.Result,
		SourceIP:   req.SourceIP,
		Timestamp:  time.Now(),
	}

	if err := h.logger.LogAction(c.Request.Context(), log); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": log.ID, "timestamp": log.Timestamp})

	// Phase 2: dual-write audit policy to etcd
	h.dualWritePolicy(req.TenantID)
}

// QueryLogs handles GET /api/v1/audit/logs
func (h *AuditHandler) QueryLogs(c *gin.Context) {
	filter := &AuditFilter{
		TenantID: c.Query("tenantId"),
		UserID:   c.Query("userId"),
		Username: c.Query("username"),
		Limit:    100,
	}

	logs, err := h.logger.QueryLogs(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs, "count": len(logs)})
}

// GetReport handles GET /api/v1/audit/report
func (h *AuditHandler) GetReport(c *gin.Context) {
	tenantID := c.Query("tenantId")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "tenantId required"})
		return
	}

	filter := &AuditFilter{TenantID: tenantID}
	report, err := h.logger.GetReport(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, report)
}

// DeleteOldLogs handles DELETE /api/v1/audit/logs
// Phase 7: Retention is configurable via AUDIT_RETENTION_DAYS env var (default 90).
func (h *AuditHandler) DeleteOldLogs(c *gin.Context) {
	olderThanDays := 90 // default
	if v := strings.TrimSpace(os.Getenv("AUDIT_RETENTION_DAYS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			olderThanDays = n
		}
	}

	// Allow query parameter override: DELETE /api/v1/audit/logs?days=30
	if daysStr := c.Query("days"); daysStr != "" {
		if n, err := strconv.Atoi(daysStr); err == nil && n > 0 {
			olderThanDays = n
		}
	}

	if err := h.logger.DeleteOldLogs(c.Request.Context(), olderThanDays); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "old logs deleted"})
}

// RegisterAuditRoutes registers all audit routes.
// Phase 7: Added unified query endpoint and configurable retention.
func RegisterAuditRoutes(router *gin.Engine, logger AuditLogger) {
	handler := NewAuditHandler(logger)

	group := router.Group("/api/v1/audit")
	{
		group.POST("/logs", handler.LogAction)
		group.GET("/logs", handler.QueryLogs)
		group.GET("/report", handler.GetReport)
		group.DELETE("/logs", handler.DeleteOldLogs)

		// Phase 7: Unified audit query — queries the central audit system.
		// Supports filtering by resourceType, action, userId, tenantId, and time range.
		group.GET("/unified", handler.QueryUnified)
	}
}

// QueryUnified handles GET /api/v1/audit/unified
// Phase 7: Unified query across all audit events stored in the central system.
func (h *AuditHandler) QueryUnified(c *gin.Context) {
	filter := &AuditFilter{
		TenantID:     c.Query("tenantId"),
		UserID:       c.Query("userId"),
		Username:     c.Query("username"),
		ResourceType: c.Query("resourceType"),
		ResourceID:   c.Query("resourceId"),
		Namespace:    c.Query("namespace"),
		SourceIP:     c.Query("sourceIp"),
		Limit:        100,
	}

	if action := c.Query("action"); action != "" {
		filter.Action = AuditAction(action)
	}
	if result := c.Query("result"); result != "" {
		filter.Result = AuditResult(result)
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			filter.Limit = n
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if n, err := strconv.Atoi(offsetStr); err == nil && n >= 0 {
			filter.Offset = n
		}
	}

	logs, err := h.logger.QueryLogs(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":   logs,
		"count":  len(logs),
		"filter": filter,
	})
}
