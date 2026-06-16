package accesslog

import (
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/utils"

	"github.com/gin-gonic/gin"
)

// AccessLogCaptureMiddleware captures each HTTP request into the access log store.
func AccessLogCaptureMiddleware(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		// Skip logging the access log API itself to avoid infinite loops.
		if len(c.FullPath()) > 0 && c.FullPath()[:len("/api/v1/access-log")] == "/api/v1/access-log" {
			return
		}

		entry := Entry{
			IP:         utils.RealIP(c),
			Method:     c.Request.Method,
			Path:       c.FullPath(),
			StatusCode: c.Writer.Status(),
			Latency:    time.Since(start),
			UserAgent:  c.Request.UserAgent(),
			Timestamp:  start,
		}

		store.Record(entry)
	}
}

// IPBlockMiddleware returns 403 for blocked IPs.
// Should be placed early in the middleware chain (before auth).
func IPBlockMiddleware(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := utils.RealIP(c)
		if store.IsBlocked(ip) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "Your IP address has been blocked",
				"code":    "IP_BLOCKED",
				"detail":  "Contact the administrator to unblock this IP address.",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
