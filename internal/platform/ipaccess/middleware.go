package ipaccess

import (
	"fmt"
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/utils"

	"github.com/gin-gonic/gin"
)

// AccessTrackerMiddleware records every request to the IP tracker,
// returns 403 for blocked IPs, and 429 for rate-limited IPs.
func AccessTrackerMiddleware(tracker *Tracker) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := utils.RealIP(c)

		// Check if IP is blocked.
		if tracker.IsBlocked(ip) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Your IP address has been blocked",
				"code":  "IP_BLOCKED",
			})
			c.Abort()
			return
		}

		// Check per-IP rate limit.
		allowed, limit, remaining := tracker.CheckRateLimit(ip)
		if !allowed {
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
			c.Header("X-RateLimit-Remaining", "0")
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded for your IP address",
				"code":  "RATE_LIMITED",
			})
			c.Abort()
			return
		}
		if limit > 0 {
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
			c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		}

		start := time.Now()
		c.Next()
		duration := time.Since(start)

		// Use actual request path, not Gin route pattern.
		path := c.Request.URL.Path
		if path == "" {
			path = "/"
		}

		tracker.Record(AccessEntry{
			Timestamp:  start,
			IP:         ip,
			Method:     c.Request.Method,
			Path:       path,
			StatusCode: c.Writer.Status(),
			UserAgent:  c.Request.UserAgent(),
			Duration:   duration.String(),
		})
	}
}
