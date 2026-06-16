package versioning

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// VersionEnforcementMiddleware returns a Gin middleware that:
// - Adds Deprecation/Sunset headers for deprecated endpoints
// - Returns 410 Gone for sunset endpoints
// - Logs version usage for analytics
func VersionEnforcementMiddleware(manager *APIVersionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.Next()
			return
		}

		path := c.FullPath()
		method := c.Request.Method
		version := extractVersionFromPath(path)

		if version == "" {
			c.Next()
			return
		}

		// Log request for version usage tracking.
		manager.LogRequest("anonymous", version, path, method)

		// Check if the version exists.
		info, err := manager.GetVersion(version)
		if err != nil {
			c.Next()
			return
		}

		// Check if sunset.
		if info.Status == "sunset" && info.SunsetDate != nil && time.Now().After(*info.SunsetDate) {
			c.JSON(http.StatusGone, gin.H{
				"error":   "This API version has been sunset",
				"code":    "API_VERSION_SUNSET",
				"detail":  "This endpoint is no longer available. Please migrate to a newer API version.",
				"sunset":  info.SunsetDate.Format(time.RFC3339),
				"version": version,
			})
			c.Abort()
			return
		}

		// Add deprecation headers if deprecated.
		if info.Status == "deprecated" {
			c.Header("Deprecation", "true")
			if info.SunsetDate != nil {
				c.Header("Sunset", info.SunsetDate.Format(time.RFC3339))
			}
		}

		// Check endpoint-level deprecation.
		key := method + "-" + path
		if endpoint, exists := info.Endpoints[key]; exists && endpoint.Deprecated {
			c.Header("Deprecation", "true")
			if endpoint.DeprecatedReason != "" {
				c.Header("X-Deprecation-Reason", endpoint.DeprecatedReason)
			}
			if endpoint.ReplacedBy != "" {
				c.Header("Link", "<"+endpoint.ReplacedBy+">; rel=\"successor-version\"")
			}
		}

		c.Next()
	}
}

// extractVersionFromPath extracts the API version from a path like /api/v1/...
func extractVersionFromPath(path string) string {
	const prefix = "/api/v"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	// Find the end of the version number.
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return ""
	}
	// Must be followed by / or end of path.
	if end < len(rest) && rest[end] != '/' {
		return ""
	}
	return "v" + rest[:end]
}
