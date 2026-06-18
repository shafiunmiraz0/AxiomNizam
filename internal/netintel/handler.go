package netintel

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/netintel/audit"
	nmetrics "axiomnizam.bitbd.net/axiomnizam/internal/netintel/metrics"

	"github.com/gin-gonic/gin"
)

// ===================================================
// Network Intelligence Handler — Full REST API
// Parsers, logs, topology, heatmaps, predictions,
// anomalies, alerts, forecasts, trends, tracks
// ===================================================

type Handler struct {
	parser      *ParserEngine
	analytics   *AnalyticsEngine
	topology    *TopologyEngine
	metrics     *nmetrics.MetricsCollector
	auditLog    *audit.Logger
	wifiScanner *WiFiScanner
}

func NewHandler() *Handler {
	parser := NewParserEngine()
	analytics := NewAnalyticsEngine(parser)
	topo := NewTopologyEngine()
	return &Handler{
		parser:      parser,
		analytics:   analytics,
		topology:    topo,
		metrics:     nmetrics.Collector,
		auditLog:    nil,
		wifiScanner: NewWiFiScanner(),
	}
}

// NewHandlerWithDeps creates a handler with injected dependencies.
func NewHandlerWithDeps(parser *ParserEngine, analytics *AnalyticsEngine, topo *TopologyEngine, metrics *nmetrics.MetricsCollector, auditLog *audit.Logger) *Handler {
	return &Handler{
		parser:      parser,
		analytics:   analytics,
		topology:    topo,
		metrics:     metrics,
		auditLog:    auditLog,
		wifiScanner: NewWiFiScanner(),
	}
}

// RegisterRoutes registers all netintel API routes on the given router group.
func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	// Summary / Observability
	group.GET("/summary", h.GetSummary)
	group.GET("/observability", h.GetObservability)

	// Log Types
	group.GET("/log-types", h.GetLogTypes)

	// Parser CRUD
	group.GET("/parsers", h.ListParsers)
	group.GET("/parsers/:id", h.GetParser)
	group.POST("/parsers", h.CreateParser)
	group.PUT("/parsers/:id", h.UpdateParser)
	group.DELETE("/parsers/:id", h.DeleteParser)

	// Log Entries
	group.GET("/logs", h.ListEntries)
	group.POST("/logs", h.IngestLog)
	group.GET("/logs/stats", h.GetEntryStats)

	// Topology
	group.GET("/topology", h.GetTopology)
	group.GET("/topology/nodes/:id", h.GetTopologyNode)
	group.PUT("/topology/nodes/:id", h.UpdateTopologyNode)

	// Heatmaps
	group.GET("/heatmap", h.GetHeatmap)

	// Trends
	group.GET("/trends", h.GetTrends)

	// Predictions
	group.GET("/predictions", h.GetPredictions)

	// Movement Tracks
	group.GET("/tracks", h.ListTracks)
	group.GET("/tracks/:mac", h.GetTrack)

	// Anomalies
	group.GET("/anomalies", h.ListAnomalies)
	group.POST("/anomalies/:id/acknowledge", h.AcknowledgeAnomaly)
	group.POST("/anomalies/:id/resolve", h.ResolveAnomaly)

	// Alerts
	group.GET("/alerts", h.ListAlerts)
	group.POST("/alerts/:id/acknowledge", h.AcknowledgeAlert)
	group.POST("/alerts/:id/resolve", h.ResolveAlert)

	// Forecasts
	group.GET("/forecasts", h.ListForecasts)
	group.GET("/forecasts/:metric", h.GetForecast)

	// WiFi Scan
	group.POST("/wifi/scan", h.ScanWiFi)
	group.GET("/wifi/networks", h.ListWiFiNetworks)
	group.GET("/wifi/history", h.WiFiScanHistory)

	// Health / Metrics / Audit
	group.GET("/health", h.Health)
	group.GET("/metrics", h.MetricsEndpoint)
	group.GET("/audit", h.AuditLogEndpoint)
}

// ========================
// Summary / Observability
// ========================

// GetSummary GET /api/v1/netintel/summary
func (h *Handler) GetSummary(c *gin.Context) {
	summary := h.analytics.GetSummary()
	c.JSON(http.StatusOK, SummaryResponse{Status: "success", Summary: summary})
}

// GetObservability GET /api/v1/netintel/observability
func (h *Handler) GetObservability(c *gin.Context) {
	stats := h.parser.GetEntryStats()
	c.JSON(http.StatusOK, ObservabilityResponse{Status: "success", Observability: stats})
}

// ========================
// Log Types
// ========================

// GetLogTypes GET /api/v1/netintel/log-types
func (h *Handler) GetLogTypes(c *gin.Context) {
	types := h.parser.GetLogTypes()
	c.JSON(http.StatusOK, LogTypesResponse{Status: "success", LogTypes: types, Total: len(types)})
}

// ========================
// Parser CRUD
// ========================

// ListParsers GET /api/v1/netintel/parsers
func (h *Handler) ListParsers(c *gin.Context) {
	parsers := h.parser.ListParsers()
	c.JSON(http.StatusOK, ParserListResponse{Status: "success", Parsers: parsers, Total: len(parsers)})
}

// GetParser GET /api/v1/netintel/parsers/:id
func (h *Handler) GetParser(c *gin.Context) {
	id := c.Param("id")
	p, ok := h.parser.GetParser(id)
	if !ok {
		c.JSON(http.StatusNotFound, MessageResponse{Error: ErrParserNotFound.Error()})
		return
	}
	c.JSON(http.StatusOK, ParserResponse{Status: "success", Parser: p})
}

// CreateParser POST /api/v1/netintel/parsers
func (h *Handler) CreateParser(c *gin.Context) {
	var p ParserConfig
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	if p.Name == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: ErrParserNameRequired.Error()})
		return
	}
	if err := h.parser.CreateParser(&p); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	if h.metrics != nil {
		h.metrics.RecordParserOperation("create")
	}
	if h.auditLog != nil {
		h.auditLog.LogParser(audit.ActionParserCreated, p.ID, "parser created: "+p.Name)
	}
	c.JSON(http.StatusCreated, ParserResponse{Status: "success", Parser: p})
}

// UpdateParser PUT /api/v1/netintel/parsers/:id
func (h *Handler) UpdateParser(c *gin.Context) {
	id := c.Param("id")
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	if err := h.parser.UpdateParser(id, updates); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: ErrParserNotFound.Error()})
		return
	}
	if h.metrics != nil {
		h.metrics.RecordParserOperation("update")
	}
	if h.auditLog != nil {
		h.auditLog.LogParser(audit.ActionParserUpdated, id, "parser updated")
	}
	p, _ := h.parser.GetParser(id)
	c.JSON(http.StatusOK, ParserResponse{Status: "success", Parser: p})
}

// DeleteParser DELETE /api/v1/netintel/parsers/:id
func (h *Handler) DeleteParser(c *gin.Context) {
	id := c.Param("id")
	if err := h.parser.DeleteParser(id); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: ErrParserNotFound.Error()})
		return
	}
	if h.metrics != nil {
		h.metrics.RecordParserOperation("delete")
	}
	if h.auditLog != nil {
		h.auditLog.LogParser(audit.ActionParserDeleted, id, "parser deleted")
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "parser deleted"})
}

// ========================
// Log Entries
// ========================

// ListEntries GET /api/v1/netintel/logs
func (h *Handler) ListEntries(c *gin.Context) {
	logType := c.Query("type")
	severity := c.Query("severity")
	limit := 200
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 5000 {
			limit = v
		}
	}
	entries := h.parser.ListEntries(logType, limit, severity)
	c.JSON(http.StatusOK, EntryListResponse{Status: "success", Entries: entries, Total: len(entries)})
}

// IngestLog POST /api/v1/netintel/logs
func (h *Handler) IngestLog(c *gin.Context) {
	var entry ParsedEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	start := time.Now()
	if err := h.parser.IngestLog(entry); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	_ = time.Since(start).Milliseconds()
	if h.metrics != nil {
		sev := entry.Severity
		if sev == "" {
			sev = "info"
		}
		h.metrics.RecordLogEntry(string(entry.LogType), sev)
	}
	if h.auditLog != nil {
		h.auditLog.Log(audit.SeverityInfo, audit.CategoryIngest, audit.ActionEntryIngested, "log entry ingested: "+string(entry.LogType))
	}
	c.JSON(http.StatusCreated, MessageResponse{Message: "log ingested"})
}

// GetEntryStats GET /api/v1/netintel/logs/stats
func (h *Handler) GetEntryStats(c *gin.Context) {
	stats := h.parser.GetEntryStats()
	c.JSON(http.StatusOK, StatsResponse{Status: "success", Stats: stats})
}

// ========================
// Topology
// ========================

// GetTopology GET /api/v1/netintel/topology
func (h *Handler) GetTopology(c *gin.Context) {
	graph := h.topology.GetGraph()
	c.JSON(http.StatusOK, TopologyResponse{Status: "success", Topology: graph})
}

// GetTopologyNode GET /api/v1/netintel/topology/nodes/:id
func (h *Handler) GetTopologyNode(c *gin.Context) {
	id := c.Param("id")
	node, ok := h.topology.GetNode(id)
	if !ok {
		c.JSON(http.StatusNotFound, MessageResponse{Error: ErrNodeNotFound.Error()})
		return
	}
	c.JSON(http.StatusOK, TopologyNodeResponse{Status: "success", Node: node})
}

// UpdateTopologyNode PUT /api/v1/netintel/topology/nodes/:id
func (h *Handler) UpdateTopologyNode(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	if err := h.topology.UpdateNodeStatus(id, body.Status); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: ErrNodeNotFound.Error()})
		return
	}
	if h.auditLog != nil {
		h.auditLog.LogWithResource(audit.SeverityInfo, audit.CategoryTopology, audit.ActionTopologyUpdated, id, "node status updated to "+body.Status)
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "node updated"})
}

// ========================
// Heatmaps
// ========================

// GetHeatmap GET /api/v1/netintel/heatmap
func (h *Handler) GetHeatmap(c *gin.Context) {
	category := c.DefaultQuery("category", "wifi_signal")
	heatmap := h.parser.GenerateHeatmap(category)
	c.JSON(http.StatusOK, HeatmapResponse{Status: "success", Heatmap: heatmap})
}

// ========================
// Trends
// ========================

// GetTrends GET /api/v1/netintel/trends
func (h *Handler) GetTrends(c *gin.Context) {
	metric := c.DefaultQuery("metric", "traffic")
	hours := 24
	if hrs := c.Query("hours"); hrs != "" {
		if v, err := strconv.Atoi(hrs); err == nil && v > 0 && v <= 168 {
			hours = v
		}
	}
	points := h.parser.GetTrend(metric, hours)
	c.JSON(http.StatusOK, TrendsResponse{Status: "success", Metric: metric, Hours: hours, Trend: points})
}

// ========================
// Predictions
// ========================

// GetPredictions GET /api/v1/netintel/predictions
func (h *Handler) GetPredictions(c *gin.Context) {
	predictions := h.analytics.PredictMovement()
	if h.metrics != nil {
		for range predictions {
			h.metrics.RecordPrediction()
		}
	}
	c.JSON(http.StatusOK, PredictionsResponse{Status: "success", Predictions: predictions, Total: len(predictions)})
}

// ========================
// Movement Tracks
// ========================

// ListTracks GET /api/v1/netintel/tracks
func (h *Handler) ListTracks(c *gin.Context) {
	tracks := h.parser.ListTracks()
	c.JSON(http.StatusOK, TrackListResponse{Status: "success", Tracks: tracks, Total: len(tracks)})
}

// GetTrack GET /api/v1/netintel/tracks/:mac
func (h *Handler) GetTrack(c *gin.Context) {
	mac := c.Param("mac")
	track, ok := h.parser.GetTrack(mac)
	if !ok {
		c.JSON(http.StatusNotFound, MessageResponse{Error: ErrTrackNotFound.Error()})
		return
	}
	c.JSON(http.StatusOK, TrackResponse{Status: "success", Track: track})
}

// ========================
// Anomalies
// ========================

// ListAnomalies GET /api/v1/netintel/anomalies
func (h *Handler) ListAnomalies(c *gin.Context) {
	status := c.Query("status")
	severity := c.Query("severity")
	anomalies := h.analytics.ListAnomalies(status, severity)
	c.JSON(http.StatusOK, AnomalyListResponse{Status: "success", Anomalies: anomalies, Total: len(anomalies)})
}

// AcknowledgeAnomaly POST /api/v1/netintel/anomalies/:id/acknowledge
func (h *Handler) AcknowledgeAnomaly(c *gin.Context) {
	id := c.Param("id")
	if err := h.analytics.AcknowledgeAnomaly(id); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: ErrAnomalyNotFound.Error()})
		return
	}
	if h.metrics != nil {
		h.metrics.RecordAnomalyOperation("acknowledge")
	}
	if h.auditLog != nil {
		h.auditLog.LogAnomaly(audit.ActionAnomalyAcked, id, "anomaly acknowledged")
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "anomaly acknowledged"})
}

// ResolveAnomaly POST /api/v1/netintel/anomalies/:id/resolve
func (h *Handler) ResolveAnomaly(c *gin.Context) {
	id := c.Param("id")
	if err := h.analytics.ResolveAnomaly(id); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: ErrAnomalyNotFound.Error()})
		return
	}
	if h.metrics != nil {
		h.metrics.RecordAnomalyOperation("resolve")
	}
	if h.auditLog != nil {
		h.auditLog.LogAnomaly(audit.ActionAnomalyResolved, id, "anomaly resolved")
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "anomaly resolved"})
}

// ========================
// Alerts
// ========================

// ListAlerts GET /api/v1/netintel/alerts
func (h *Handler) ListAlerts(c *gin.Context) {
	status := c.Query("status")
	alerts := h.analytics.ListAlerts(status)
	c.JSON(http.StatusOK, AlertListResponse{Status: "success", Alerts: alerts, Total: len(alerts)})
}

// AcknowledgeAlert POST /api/v1/netintel/alerts/:id/acknowledge
func (h *Handler) AcknowledgeAlert(c *gin.Context) {
	id := c.Param("id")
	if err := h.analytics.AcknowledgeAlert(id); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: ErrAlertNotFound.Error()})
		return
	}
	if h.metrics != nil {
		h.metrics.RecordAlertOperation("acknowledge")
	}
	if h.auditLog != nil {
		h.auditLog.LogAlert(audit.ActionAlertAcked, id, "alert acknowledged")
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "alert acknowledged"})
}

// ResolveAlert POST /api/v1/netintel/alerts/:id/resolve
func (h *Handler) ResolveAlert(c *gin.Context) {
	id := c.Param("id")
	if err := h.analytics.ResolveAlert(id); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: ErrAlertNotFound.Error()})
		return
	}
	if h.metrics != nil {
		h.metrics.RecordAlertOperation("resolve")
	}
	if h.auditLog != nil {
		h.auditLog.LogAlert(audit.ActionAlertResolved, id, "alert resolved")
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "alert resolved"})
}

// ========================
// Forecasts
// ========================

// ListForecasts GET /api/v1/netintel/forecasts
func (h *Handler) ListForecasts(c *gin.Context) {
	forecasts := h.analytics.ListForecasts()
	c.JSON(http.StatusOK, ForecastListResponse{Status: "success", Forecasts: forecasts})
}

// GetForecast GET /api/v1/netintel/forecasts/:metric
func (h *Handler) GetForecast(c *gin.Context) {
	metric := c.Param("metric")
	forecast, ok := h.analytics.GetForecast(metric)
	if !ok {
		c.JSON(http.StatusNotFound, MessageResponse{Error: ErrForecastNotFound.Error()})
		return
	}
	c.JSON(http.StatusOK, ForecastResponse{Status: "success", Forecast: forecast})
}

// ========================
// WiFi Scan
// ========================

// ScanWiFi POST /api/v1/netintel/wifi/scan
func (h *Handler) ScanWiFi(c *gin.Context) {
	result, err := h.wifiScanner.Scan()
	if err != nil {
		if h.metrics != nil {
			h.metrics.RecordLogEntry("wifi_scan", "error")
		}
		if h.auditLog != nil {
			h.auditLog.Log(audit.SeverityError, audit.CategoryIngest, audit.ActionEntryIngested, "wifi scan failed: "+err.Error())
		}
		c.JSON(http.StatusInternalServerError, WiFiScanResponse{
			Status: "error",
			Result: result,
		})
		return
	}

	// Ingest scanned networks as WiFi probe entries
	for _, net := range result.Networks {
		if net.SSID != "" {
			entry := ParsedEntry{
				LogType:   LogWiFiProbe,
				Source:    "wifi-scanner",
				Severity:  "info",
				Message:   fmt.Sprintf("WiFi scan: %s (%s) ch=%d signal=%ddBm", net.SSID, net.BSSID, net.Channel, net.SignalDBM),
				DeviceMAC: net.BSSID,
				SSID:      net.SSID,
				Signal:    net.SignalDBM,
				Fields: map[string]interface{}{
					"bssid":      net.BSSID,
					"channel":    net.Channel,
					"frequency":  net.Frequency,
					"band":       net.Band,
					"encryption": net.Encryption,
					"auth":       net.Auth,
					"cipher":     net.Cipher,
					"radio":      net.Radio,
					"distance_m": net.Distance,
					"signal_pct": net.Signal,
				},
			}
			_ = h.parser.IngestLog(entry)
		}
	}

	if h.metrics != nil {
		h.metrics.RecordLogEntry("wifi_scan", "info")
	}
	if h.auditLog != nil {
		h.auditLog.Log(audit.SeverityInfo, audit.CategoryIngest, audit.ActionEntryIngested,
			fmt.Sprintf("wifi scan completed: %d networks found on %s in %dms", result.Total, result.Interface, result.DurationMs))
	}

	c.JSON(http.StatusOK, WiFiScanResponse{Status: "success", Result: result})
}

// ListWiFiNetworks GET /api/v1/netintel/wifi/networks
func (h *Handler) ListWiFiNetworks(c *gin.Context) {
	result := h.wifiScanner.GetLastResult()
	if result == nil {
		c.JSON(http.StatusOK, WiFiNetworksResponse{
			Status:   "success",
			Networks: []WiFiNetwork{},
			Total:    0,
		})
		return
	}
	c.JSON(http.StatusOK, WiFiNetworksResponse{
		Status:   "success",
		Networks: result.Networks,
		Total:    result.Total,
	})
}

// WiFiScanHistory GET /api/v1/netintel/wifi/history
func (h *Handler) WiFiScanHistory(c *gin.Context) {
	history := h.wifiScanner.GetHistory()
	c.JSON(http.StatusOK, WiFiHistoryResponse{
		Status:  "success",
		History: history,
		Total:   len(history),
	})
}

// ========================
// Health / Metrics / Audit
// ========================

// Health GET /api/v1/netintel/health
func (h *Handler) Health(c *gin.Context) {
	snapshot := nmetrics.Collector.Snapshot()
	c.JSON(http.StatusOK, HealthResponse{
		Status:        "healthy",
		UptimeSec:     snapshot.UptimeSeconds,
		TotalIngested: snapshot.TotalIngested,
		Module:        "netintel",
	})
}

// MetricsEndpoint GET /api/v1/netintel/metrics
func (h *Handler) MetricsEndpoint(c *gin.Context) {
	snapshot := nmetrics.Collector.Snapshot()
	c.JSON(http.StatusOK, MetricsEndpointResponse{
		TotalIngested:  snapshot.TotalIngested,
		TotalAnomalies: snapshot.TotalAnomalies,
		TotalAlerts:    snapshot.TotalAlerts,
		UptimeSeconds:  snapshot.UptimeSeconds,
		ByLogType:      snapshot.ByLogType,
		BySeverity:     snapshot.BySeverity,
	})
}

// AuditLogEndpoint GET /api/v1/netintel/audit
func (h *Handler) AuditLogEndpoint(c *gin.Context) {
	if h.auditLog == nil {
		c.JSON(http.StatusOK, AuditLogResponse{Events: []audit.Event{}, Count: 0})
		return
	}
	events := h.auditLog.List()
	c.JSON(http.StatusOK, AuditLogResponse{Events: events, Count: len(events)})
}
