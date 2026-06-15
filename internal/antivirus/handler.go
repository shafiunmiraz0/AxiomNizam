package antivirus

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"github.com/gin-gonic/gin"
)

// ─────────────────────────────────────────────────────────────────────────────
// API Handler
//
// Provides HTTP endpoints for antivirus engine management. These are
// designed to be mounted into the storage admin router group.
//
// Endpoints:
//
//	GET  /antivirus/status   — engine status + signature DB version + layer info
//	GET  /antivirus/stats    — scan statistics (totals, cache hit rate, avg scan time)
//	POST /antivirus/scan     — manual scan of raw content (multipart upload)
//	GET  /antivirus/threats  — list recent threat detections from audit log
//	POST /antivirus/update   — trigger immediate signature database update
//	PUT  /antivirus/config   — view current engine configuration
// ─────────────────────────────────────────────────────────────────────────────

// ThreatRecord is a compact threat entry returned by the /threats endpoint.
type ThreatRecord struct {
	Filename  string      `json:"filename"`
	SHA256    string      `json:"sha256"`
	Verdict   ScanVerdict `json:"verdict"`
	Threats   []string    `json:"threats"`
	Severity  string      `json:"severity"`
	ScannedAt time.Time   `json:"scannedAt"`
	DurationMs int64     `json:"durationMs"`
}

// APIHandler exposes antivirus management endpoints. It references the
// Engine directly and is embedded into the storage admin handler.
type APIHandler struct {
	engine  *Engine
	kvStore platformstore.KVStore
}

// NewAPIHandler creates a new antivirus API handler.
func NewAPIHandler(engine *Engine) *APIHandler {
	return &APIHandler{engine: engine}
}

// SetKVStore wires the KV store for config persistence.
func (h *APIHandler) SetKVStore(kv platformstore.KVStore) {
	h.kvStore = kv
}

// RegisterRoutes mounts antivirus routes under the given router group.
// sysadminMiddleware is applied to write endpoints (PUT /config).
func (h *APIHandler) RegisterRoutes(rg *gin.RouterGroup, sysadminMiddleware ...gin.HandlerFunc) {
	if h == nil || h.engine == nil {
		return
	}

	av := rg.Group("/antivirus")
	{
		av.GET("/status", h.Status)
		av.GET("/stats", h.ScanStats)
		av.POST("/scan", h.ManualScan)
		av.GET("/threats", h.ListThreats)
		av.GET("/config", h.GetConfig)
	}

	// Admin-only write endpoints.
	admin := av.Group("/")
	for _, mw := range sysadminMiddleware {
		admin.Use(mw)
	}
	{
		admin.PUT("/config", h.UpdateConfig)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /antivirus/status
// ─────────────────────────────────────────────────────────────────────────────

// Status returns the current engine status, including whether it's running,
// the signature database version, enabled layers, and uptime.
func (h *APIHandler) Status(c *gin.Context) {
	stats := h.engine.Stats()

	cfg := h.engine.ConfigSnapshot()

	avStatus := "disabled"
	if h.engine.IsRunning() {
		avStatus = "running"
	} else if cfg.Enabled {
		avStatus = "stopped"
	}

	c.JSON(http.StatusOK, StatusToResponse(avStatus, EngineVersion, stats, cfg))
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /antivirus/stats
// ─────────────────────────────────────────────────────────────────────────────

// ScanStats returns scan performance statistics including totals, cache
// performance, and average scan times.
func (h *APIHandler) ScanStats(c *gin.Context) {
	stats := h.engine.Stats()
	c.JSON(http.StatusOK, StatsToResponse(stats))
}

// ─────────────────────────────────────────────────────────────────────────────
// POST /antivirus/scan
// ─────────────────────────────────────────────────────────────────────────────

// ManualScan allows manual scanning of uploaded content. Accepts a multipart
// file upload with field name "file" and returns the full scan result.
//
// Request: multipart/form-data with "file" field
// Response: ScanResult JSON
func (h *APIHandler) ManualScan(c *gin.Context) {
	if !h.engine.IsRunning() {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "antivirus engine is not running"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "file upload required (form field: 'file'): " + err.Error()})
		return
	}
	defer file.Close()

	// Read content with size limit.
	maxSize := h.engine.MaxFileSize()
	content, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to read uploaded file: " + err.Error()})
		return
	}
	if int64(len(content)) > maxSize {
		c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{Error: "file exceeds maximum scan size", MaxSize: maxSize})
		return
	}

	filename := header.Filename
	if filename == "" {
		filename = "manual-upload"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	result, err := h.engine.Scan(ctx, content, filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "scan failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /antivirus/threats
// ─────────────────────────────────────────────────────────────────────────────

// ListThreats returns recent threat detections. It searches the engine's
// internal threat log. Supports ?limit=N query parameter (default: 50).
func (h *APIHandler) ListThreats(c *gin.Context) {
	threats := h.engine.RecentThreats()
	c.JSON(http.StatusOK, ThreatsToResponse(threats))
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /antivirus/config
// ─────────────────────────────────────────────────────────────────────────────

// GetConfig returns the current engine configuration (read-only view).
func (h *APIHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, ConfigToResponse(h.engine.ConfigSnapshot()))
}

// UpdateConfig applies a partial config update and persists to KV store.
func (h *APIHandler) UpdateConfig(c *gin.Context) {
	var req UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	current := h.engine.ConfigSnapshot()
	updated := ApplyUpdateRequest(current, &req)

	if warnings := updated.Validate(); len(warnings) > 0 {
		for _, w := range warnings {
			logging.Z().Warn("antivirus config update warning: " + w)
		}
	}

	if _, err := h.engine.UpdateConfig(updated); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if h.kvStore != nil {
		if err := SaveConfigToKV(c.Request.Context(), h.kvStore, updated); err != nil {
			logging.Z().Error("antivirus: failed to persist config to KV: " + err.Error())
		}
	}

	c.JSON(http.StatusOK, ConfigToResponse(updated))
}

// redactURL masks sensitive parts of URLs for safe display.
func redactURL(url string) string {
	if url == "" {
		return "(not configured)"
	}
	// Mask anything after :// and before the first /path segment.
	if idx := strings.Index(url, "://"); idx >= 0 {
		rest := url[idx+3:]
		if slashIdx := strings.Index(rest, "/"); slashIdx > 0 {
			return url[:idx+3] + "***" + rest[slashIdx:]
		}
	}
	return url
}
