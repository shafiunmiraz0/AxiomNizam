package ipaccess

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessTrackerMiddleware records every request to the IP tracker
// and returns 403 for blocked IPs.
func AccessTrackerMiddleware(tracker *Tracker) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		// Check if IP is blocked.
		if tracker.IsBlocked(ip) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Your IP address has been blocked",
				"code":  "IP_BLOCKED",
			})
			c.Abort()
			return
		}

		start := time.Now()
		c.Next()
		duration := time.Since(start)

		tracker.Record(AccessEntry{
			Timestamp:  start,
			IP:         ip,
			Method:     c.Request.Method,
			Path:       c.FullPath(),
			StatusCode: c.Writer.Status(),
			UserAgent:  c.Request.UserAgent(),
			Duration:   duration.String(),
		})
	}
}
