package ratelimit

import (
	"fmt"
	"net/http"
	"strconv"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"github.com/gin-gonic/gin"
)

// RateLimitMiddleware enforces rate limiting
type RateLimitMiddleware struct {
	quotaManager *QuotaManager
}

// NewRateLimitMiddleware creates a new rate limit middleware
func NewRateLimitMiddleware(qm *QuotaManager) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		quotaManager: qm,
	}
}

// Handler is the middleware handler
func (rlm *RateLimitMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		if userID == "" {
			userID = c.ClientIP()
		}

		contentLength := int64(0)
		if c.Request.ContentLength > 0 {
			contentLength = c.Request.ContentLength
		}

		endpoint := c.Request.URL.Path
		allowed, remaining, err := rlm.quotaManager.CheckQuota(userID, endpoint, contentLength)

		if !allowed {
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", 1000))
			c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", 0))

			c.JSON(http.StatusTooManyRequests, models.Response{
				Status: "error",
				Error:  fmt.Sprintf("Rate limit exceeded: %v", err),
			})
			c.Abort()
			return
		}

		// Add rate limit headers
		c.Header("X-RateLimit-Limit", "10000")
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", 0))

		c.Next()

		// Release quota after request
		rlm.quotaManager.ReleaseQuota(userID)
	}
}

// QuotaHandler handles quota management endpoints
type QuotaHandler struct {
	quotaManager *QuotaManager
}

// NewQuotaHandler creates a new quota handler
func NewQuotaHandler(qm *QuotaManager) *QuotaHandler {
	return &QuotaHandler{
		quotaManager: qm,
	}
}

// GetQuota handles GET /api/v1/quota/:user_id
func (qh *QuotaHandler) GetQuota(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		userID = c.GetString("user_id")
	}

	if userID == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "user_id is required",
		})
		return
	}

	status := qh.quotaManager.GetQuotaStatus(userID)
	c.JSON(http.StatusOK, models.Response{
		Status: "ok",
		Data:   status,
	})
}

// SetUserQuota handles PUT /api/v1/quota/:user_id
func (qh *QuotaHandler) SetUserQuota(c *gin.Context) {
	userID := c.Param("user_id")

	var req struct {
		DailyLimit int64 `json:"daily_limit"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request: " + err.Error(),
		})
		return
	}

	qh.quotaManager.SetUserDailyQuota(userID, req.DailyLimit)

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Quota updated successfully",
		Data: map[string]interface{}{
			"user_id":     userID,
			"daily_limit": req.DailyLimit,
		},
	})
}

// ResetQuota handles POST /api/v1/quota/:user_id/reset
func (qh *QuotaHandler) ResetQuota(c *gin.Context) {
	userID := c.Param("user_id")

	qh.quotaManager.ResetUserQuota(userID)

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Quota reset successfully",
	})
}

// ListQuotas handles GET /api/v1/quotas
func (qh *QuotaHandler) ListQuotas(c *gin.Context) {
	quotas := qh.quotaManager.GetAllUserQuotas()
	c.JSON(http.StatusOK, models.Response{
		Status: "ok",
		Data:   quotas,
	})
}

// SetEndpointLimit handles POST /api/v1/endpoints/:endpoint/limit
func (qh *QuotaHandler) SetEndpointLimit(c *gin.Context) {
	endpoint := c.Param("endpoint")

	var req QuotaLimit

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request: " + err.Error(),
		})
		return
	}

	qh.quotaManager.SetEndpointLimit(endpoint, req)

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Endpoint limit set successfully",
		Data: map[string]interface{}{
			"endpoint": endpoint,
			"limit":    req,
		},
	})
}

// ParseRateLimitHeader parses rate limit from request
func ParseRateLimitHeader(c *gin.Context) (limit int, err error) {
	limitStr := c.GetHeader("X-RateLimit-Limit")
	if limitStr == "" {
		return 1000, nil
	}

	limit, err = strconv.Atoi(limitStr)
	if err != nil {
		return 1000, err
	}

	return limit, nil
}
