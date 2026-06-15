package waitx

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/waitx/audit"
	wmetrics "axiomnizam.bitbd.net/axiomnizam/internal/waitx/metrics"

	"github.com/gin-gonic/gin"
)

// Handler serves the waitx HTTP API.
type Handler struct {
	auditLogger *audit.Logger
}

// NewHandler creates a new waitx handler.
func NewHandler(auditLogger *audit.Logger) *Handler {
	return &Handler{auditLogger: auditLogger}
}

// RegisterRoutes registers all waitx API routes.
func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/waitx/health", h.Health)
	group.GET("/waitx/metrics", h.Metrics)
	group.GET("/waitx/audit", h.AuditLog)
	group.POST("/waitx/check", h.RunCheck)
	group.POST("/waitx/wait", h.Wait)
}

// Health returns module health.
func (h *Handler) Health(c *gin.Context) {
	snapshot := wmetrics.Collector.Snapshot()
	status := "healthy"
	if snapshot.TotalChecks > 0 && snapshot.SuccessRate < 50 {
		status = "degraded"
	}

	c.JSON(http.StatusOK, HealthResponse{
		Status:      status,
		UptimeSec:   snapshot.UptimeSeconds,
		TotalChecks: snapshot.TotalChecks,
		SuccessRate: fmt.Sprintf("%.1f%%", snapshot.SuccessRate),
		Module:      "waitx",
	})
}

// Metrics returns module metrics.
func (h *Handler) Metrics(c *gin.Context) {
	snapshot := wmetrics.Collector.Snapshot()
	byType := make([]CheckTypeStats, 0, len(snapshot.ByCheckType))
	for _, ct := range snapshot.ByCheckType {
		successRate := float64(0)
		if ct.Runs > 0 {
			successRate = float64(ct.Successes) / float64(ct.Runs) * 100
		}
		byType = append(byType, CheckTypeStats{
			CheckType:   ct.CheckType,
			Runs:        ct.Runs,
			Successes:   ct.Successes,
			Failures:    ct.Failures,
			Timeouts:    ct.Timeouts,
			TotalMs:     ct.TotalMs,
			AvgMs:       ct.AvgMs,
			SuccessRate: fmt.Sprintf("%.1f%%", successRate),
		})
	}

	c.JSON(http.StatusOK, MetricsResponse{
		TotalChecks:    snapshot.TotalChecks,
		TotalSuccesses: snapshot.TotalSuccesses,
		TotalFailures:  snapshot.TotalFailures,
		TotalTimeouts:  snapshot.TotalTimeouts,
		SuccessRate:    fmt.Sprintf("%.1f%%", snapshot.SuccessRate),
		UptimeSeconds:  snapshot.UptimeSeconds,
		ByCheckType:    byType,
	})
}

// AuditLog returns the module audit log.
func (h *Handler) AuditLog(c *gin.Context) {
	events := h.auditLogger.List()
	c.JSON(http.StatusOK, gin.H{"events": events, "count": len(events)})
}

// RunCheck executes a single waitx check and returns the result.
func (h *Handler) RunCheck(c *gin.Context) {
	var req RunCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	checker, err := buildCheckerFromRequest(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	timeout := parseDurationOrDefault(req.Timeout, 30*time.Second)
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	start := time.Now()
	checkErr := checker.Check(ctx)
	durationMs := time.Since(start).Milliseconds()
	success := checkErr == nil

	wmetrics.Collector.RecordCheck(req.CheckType, success, false, durationMs)

	if h.auditLogger != nil {
		status := audit.ActionCheckSucceeded
		message := "check passed"
		if !success {
			status = audit.ActionCheckFailed
			message = checkErr.Error()
		}
		h.auditLogger.LogCheck(status, req.CheckType, req.Target, durationMs, message)
	}

	resp := CheckResponse{
		CheckType:  req.CheckType,
		Target:     req.Target,
		DurationMs: durationMs,
		Attempts:   1,
	}
	if success {
		resp.Status = "ready"
	} else {
		resp.Status = "failed"
		resp.Message = checkErr.Error()
	}

	c.JSON(http.StatusOK, resp)
}

// Wait waits until a check is ready (with retries).
func (h *Handler) Wait(c *gin.Context) {
	var req WaitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	checker, err := buildCheckerFromRequest(RunCheckRequest{
		CheckType: req.CheckType,
		Target:    req.Target,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	timeout := parseDurationOrDefault(req.Timeout, 30*time.Second)
	interval := parseDurationOrDefault(req.Interval, time.Second)

	var strategy RetryStrategy = LinearRetry{}
	if strings.ToLower(strings.TrimSpace(req.RetryPolicy)) == "exponential" {
		strategy = ExponentialRetry{Coefficient: 2}
	}

	start := time.Now()
	var attempts int

	err = WaitContext(c.Request.Context(), checker, WaitOptions{
		Timeout:       timeout,
		Interval:      interval,
		MaxInterval:   30 * time.Second,
		InvertCheck:   req.InvertCheck,
		RetryStrategy: strategy,
		OnRetry: func(event AttemptEvent) {
			attempts = event.Attempt
			wmetrics.Collector.RecordRetry(req.CheckType)
		},
	})

	durationMs := time.Since(start).Milliseconds()
	attempts++
	ready := err == nil

	wmetrics.Collector.RecordCheck(req.CheckType, ready, !ready, durationMs)

	resp := WaitResponse{
		CheckType:  req.CheckType,
		Target:     req.Target,
		Ready:      ready,
		DurationMs: durationMs,
		Attempts:   attempts,
	}
	if err != nil {
		resp.Message = err.Error()
	}

	c.JSON(http.StatusOK, resp)
}

func buildCheckerFromRequest(req RunCheckRequest) (Checker, error) {
	switch strings.ToLower(strings.TrimSpace(req.CheckType)) {
	case "tcp":
		return TCPChecker{Address: req.Target}, nil
	case "http":
		return HTTPChecker{
			URL:              req.Target,
			Method:           req.Method,
			Headers:          req.Headers,
			ExpectStatusCode: req.ExpectStatusCode,
			InsecureSkipTLS:  req.InsecureSkipTLS,
		}, nil
	case "dns":
		return DNSChecker{
			RecordType:     req.RecordType,
			Address:        req.Target,
			ExpectedValues: req.ExpectedValues,
		}, nil
	case "redis":
		return RedisChecker{Address: req.Target, ExpectedKey: req.ExpectedKey}, nil
	case "mysql":
		return MySQLChecker{DSN: req.DSN, ExpectedTable: req.ExpectedTable}, nil
	case "postgresql", "postgres":
		return PostgreSQLChecker{DSN: req.DSN, ExpectedTable: req.ExpectedTable}, nil
	case "mongodb":
		return MongoDBChecker{URI: req.Target}, nil
	case "kafka":
		return KafkaChecker{Brokers: req.Brokers}, nil
	case "rabbitmq":
		return RabbitMQChecker{URL: req.Target}, nil
	default:
		return nil, ErrUnsupportedCheckType
	}
}

func parseDurationOrDefault(s string, def time.Duration) time.Duration {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return def
	}
	d, err := time.ParseDuration(trimmed)
	if err != nil {
		return def
	}
	return d
}
