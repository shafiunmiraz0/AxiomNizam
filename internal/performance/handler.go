package performance

import (
	"net/http"
	"strconv"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"github.com/gin-gonic/gin"
)

// Handler handles performance monitoring endpoints.
type Handler struct {
	analyzer *QueryPerformanceAnalyzer
}

// NewHandler creates a new performance handler.
func NewHandler() *Handler {
	return &Handler{
		analyzer: NewQueryPerformanceAnalyzer(1000, 10000),
	}
}

// GetStats handles GET /api/v1/performance/stats
func (h *Handler) GetStats(c *gin.Context) {
	stats := map[string]interface{}{
		"queries": 0,
		"avgTime": 0,
	}
	c.JSON(http.StatusOK, models.Response{
		Status: "ok",
		Data:   stats,
	})
}

// GetSlowQueries handles GET /api/v1/performance/slow-queries
func (h *Handler) GetSlowQueries(c *gin.Context) {
	slowQueries := h.analyzer.GetSlowQueries()
	c.JSON(http.StatusOK, models.Response{
		Status: "ok",
		Data: map[string]interface{}{
			"queries": slowQueries,
			"count":   len(slowQueries),
		},
	})
}

// GetQueryTypeStats handles GET /api/v1/performance/query-types
func (h *Handler) GetQueryTypeStats(c *gin.Context) {
	stats := h.analyzer.GetQueryTypeStats()
	c.JSON(http.StatusOK, models.Response{
		Status: "ok",
		Data:   stats,
	})
}

// GetUserStats handles GET /api/v1/performance/user-stats
func (h *Handler) GetUserStats(c *gin.Context) {
	stats := h.analyzer.GetUserStats()
	c.JSON(http.StatusOK, models.Response{
		Status: "ok",
		Data:   stats,
	})
}

// GetRecommendations handles GET /api/v1/performance/recommendations
func (h *Handler) GetRecommendations(c *gin.Context) {
	recommendations := h.analyzer.GetRecommendations()
	c.JSON(http.StatusOK, models.Response{
		Status: "ok",
		Data: map[string]interface{}{
			"recommendations": recommendations,
			"count":           len(recommendations),
		},
	})
}

// GetPercentile handles GET /api/v1/performance/percentile/:value
func (h *Handler) GetPercentile(c *gin.Context) {
	percentileStr := c.Param("value")

	percentile, err := strconv.ParseFloat(percentileStr, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid percentile value",
		})
		return
	}

	p99 := h.analyzer.GetPercentile(percentile)
	c.JSON(http.StatusOK, models.Response{
		Status: "ok",
		Data: map[string]interface{}{
			"percentile": percentile,
			"duration":   p99,
		},
	})
}

// RecordQuery handles POST /api/v1/performance/record
func (h *Handler) RecordQuery(c *gin.Context) {
	var qp QueryPerformance

	if err := c.ShouldBindJSON(&qp); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request: " + err.Error(),
		})
		return
	}

	h.analyzer.RecordQuery(qp)

	c.JSON(http.StatusCreated, models.Response{
		Status:  "ok",
		Message: "Query recorded successfully",
	})
}

// GetDashboard handles GET /api/v1/performance/dashboard
func (h *Handler) GetDashboard(c *gin.Context) {
	stats := h.analyzer.GetQueryStats()
	typeStats := h.analyzer.GetQueryTypeStats()
	userStats := h.analyzer.GetUserStats()
	recommendations := h.analyzer.GetRecommendations()

	c.JSON(http.StatusOK, models.Response{
		Status: "ok",
		Data: map[string]interface{}{
			"overall_stats":    stats,
			"query_type_stats": typeStats,
			"user_stats":       userStats,
			"recommendations":  recommendations,
			"p50_duration":     h.analyzer.GetPercentile(50),
			"p95_duration":     h.analyzer.GetPercentile(95),
			"p99_duration":     h.analyzer.GetPercentile(99),
		},
	})
}

// RegisterRoutes registers performance routes on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/performance/stats", h.GetStats)
	rg.GET("/performance/slow-queries", h.GetSlowQueries)
	rg.GET("/performance/query-types", h.GetQueryTypeStats)
	rg.GET("/performance/user-stats", h.GetUserStats)
	rg.GET("/performance/recommendations", h.GetRecommendations)
	rg.GET("/performance/percentile/:value", h.GetPercentile)
	rg.POST("/performance/record", h.RecordQuery)
	rg.GET("/performance/dashboard", h.GetDashboard)
}
