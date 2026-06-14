package query

import (
	"net/http"
	"strconv"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"github.com/gin-gonic/gin"
)

// GetEnterpriseQueryStats handles GET /api/query/stats/enterprise endpoint
func (h *Handler) GetEnterpriseQueryStats(c *gin.Context) {
	if h.logger == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Query logging not available",
		})
		return
	}

	database := c.Query("database")
	if database == "" {
		database = "all"
	}

	metrics := h.logger.GetMetrics()

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Enterprise query statistics retrieved successfully",
		Data: map[string]interface{}{
			"total_queries":  metrics.TotalQueries,
			"success_count":  metrics.SuccessCount,
			"error_count":    metrics.ErrorCount,
			"success_rate":   float64(metrics.SuccessCount) / float64(metrics.TotalQueries) * 100,
			"avg_duration":   metrics.AvgDuration,
			"total_duration": metrics.TotalDuration,
			"total_rows":     metrics.TotalRows,
			"by_database":    metrics.ByDatabase,
			"by_user":        metrics.ByUser,
			"by_query_type":  metrics.ByQueryType,
		},
	})
}

// GetSlowQueries handles GET /api/query/slow endpoint
func (h *Handler) GetSlowQueries(c *gin.Context) {
	if h.logger == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Query logging not available",
		})
		return
	}

	database := c.Query("database")
	if database == "" {
		database = "all"
	}

	thresholdStr := c.Query("threshold")
	threshold := int64(1000)
	if thresholdStr != "" {
		if t, err := strconv.ParseInt(thresholdStr, 10, 64); err == nil {
			threshold = t
		}
	}

	limitStr := c.Query("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	slowQueries, err := h.logger.GetSlowQueries(database, threshold, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to retrieve slow queries: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Slow queries retrieved successfully",
		Data: map[string]interface{}{
			"threshold":    threshold,
			"count":        len(slowQueries),
			"slow_queries": slowQueries,
		},
	})
}

// GetErroredQueries handles GET /api/query/errors endpoint
func (h *Handler) GetErroredQueries(c *gin.Context) {
	if h.logger == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Query logging not available",
		})
		return
	}

	database := c.Query("database")
	if database == "" {
		database = "all"
	}

	limitStr := c.Query("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	erroredQueries, err := h.logger.GetErroredQueries(database, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to retrieve errored queries: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Errored queries retrieved successfully",
		Data: map[string]interface{}{
			"count":           len(erroredQueries),
			"errored_queries": erroredQueries,
		},
	})
}

// GetUserMetrics handles GET /api/query/user/:user endpoint
func (h *Handler) GetUserMetrics(c *gin.Context) {
	if h.logger == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Query logging not available",
		})
		return
	}

	user := c.Param("user")
	if user == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "User parameter is required",
		})
		return
	}

	userMetrics := h.logger.GetUserMetrics(user)
	if userMetrics == nil {
		c.JSON(http.StatusNotFound, models.Response{
			Status: "error",
			Error:  "No metrics found for user: " + user,
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "User metrics retrieved successfully",
		Data: map[string]interface{}{
			"user":            userMetrics.User,
			"role":            userMetrics.Role,
			"total_queries":   userMetrics.TotalQueries,
			"success_count":   userMetrics.SuccessCount,
			"error_count":     userMetrics.ErrorCount,
			"success_rate":    float64(userMetrics.SuccessCount) / float64(userMetrics.TotalQueries) * 100,
			"avg_duration":    userMetrics.AvgDuration,
			"last_query_time": userMetrics.LastQueryTime,
		},
	})
}

// GetDatabaseMetrics handles GET /api/query/database/:db endpoint
func (h *Handler) GetDatabaseMetrics(c *gin.Context) {
	if h.logger == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Query logging not available",
		})
		return
	}

	database := c.Param("db")
	if database == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Database parameter is required",
		})
		return
	}

	dbMetrics := h.logger.GetDatabaseMetrics(database)
	if dbMetrics == nil {
		c.JSON(http.StatusNotFound, models.Response{
			Status: "error",
			Error:  "No metrics found for database: " + database,
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Database metrics retrieved successfully",
		Data: map[string]interface{}{
			"database":      dbMetrics.Database,
			"total_queries": dbMetrics.TotalQueries,
			"success_count": dbMetrics.SuccessCount,
			"error_count":   dbMetrics.ErrorCount,
			"success_rate":  float64(dbMetrics.SuccessCount) / float64(dbMetrics.TotalQueries) * 100,
			"avg_duration":  dbMetrics.AvgDuration,
			"total_rows":    dbMetrics.TotalRows,
		},
	})
}

// GetQueryTypeMetrics handles GET /api/query/type/:type endpoint
func (h *Handler) GetQueryTypeMetrics(c *gin.Context) {
	if h.logger == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Query logging not available",
		})
		return
	}

	queryType := c.Param("type")
	if queryType == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Query type parameter is required",
		})
		return
	}

	typeMetrics := h.logger.GetQueryTypeMetrics(queryType)
	if typeMetrics == nil {
		c.JSON(http.StatusNotFound, models.Response{
			Status: "error",
			Error:  "No metrics found for query type: " + queryType,
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Query type metrics retrieved successfully",
		Data: map[string]interface{}{
			"query_type":    typeMetrics.QueryType,
			"total_queries": typeMetrics.TotalQueries,
			"success_count": typeMetrics.SuccessCount,
			"error_count":   typeMetrics.ErrorCount,
			"success_rate":  float64(typeMetrics.SuccessCount) / float64(typeMetrics.TotalQueries) * 100,
			"avg_duration":  typeMetrics.AvgDuration,
			"total_rows":    typeMetrics.TotalRows,
		},
	})
}

// GetUserQueries handles GET /api/query/user/:user/queries endpoint
func (h *Handler) GetUserQueries(c *gin.Context) {
	if h.logger == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Query logging not available",
		})
		return
	}

	user := c.Param("user")
	if user == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "User parameter is required",
		})
		return
	}

	limitStr := c.Query("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	userQueries, err := h.logger.GetQuerysByUser(user, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to retrieve user queries: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "User queries retrieved successfully",
		Data: map[string]interface{}{
			"user":    user,
			"count":   len(userQueries),
			"queries": userQueries,
		},
	})
}

// GetMetricsReport handles GET /api/query/report endpoint
func (h *Handler) GetMetricsReport(c *gin.Context) {
	if h.logger == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Query logging not available",
		})
		return
	}

	report := h.logger.GetMetricsReport()

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Metrics report retrieved successfully",
		Data:    report,
	})
}

// DeleteOldLogs handles DELETE /api/query/logs/old endpoint
func (h *Handler) DeleteOldLogs(c *gin.Context) {
	if h.logger == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Query logging not available",
		})
		return
	}

	database := c.Query("database")
	if database == "" {
		database = "all"
	}

	daysStr := c.Query("days")
	days := 30
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil {
			days = d
		}
	}

	if err := h.logger.DeleteOldLogs(database, days); err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to delete old logs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Old logs deleted successfully",
		Data: map[string]interface{}{
			"database": database,
			"days":     days,
		},
	})
}
