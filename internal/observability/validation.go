package observability

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	// DefaultMaxBodySize is the default maximum request body size (10 MB).
	DefaultMaxBodySize int64 = 10 << 20

	// DefaultMaxMultipartMemory is the default max memory for multipart forms (32 MB).
	DefaultMaxMultipartMemory int64 = 32 << 20
)

// RequestValidationConfig controls request validation behavior.
type RequestValidationConfig struct {
	// MaxBodySize is the maximum request body size in bytes.
	// Default: 10 MB.
	MaxBodySize int64

	// EnforceJSONContentType rejects POST/PUT/PATCH requests that don't
	// have Content-Type: application/json (unless the path is exempt).
	EnforceJSONContentType bool

	// JSONExemptPaths are paths that skip Content-Type enforcement
	// (e.g., file upload endpoints).
	JSONExemptPaths []string
}

// DefaultRequestValidationConfig returns safe defaults.
func DefaultRequestValidationConfig() RequestValidationConfig {
	return RequestValidationConfig{
		MaxBodySize:            DefaultMaxBodySize,
		EnforceJSONContentType: false, // opt-in to avoid breaking existing clients
	}
}

// RequestValidationMiddleware enforces body size limits and optional
// Content-Type validation.  Should be registered early in the middleware chain.
func RequestValidationMiddleware(cfg RequestValidationConfig) gin.HandlerFunc {
	if cfg.MaxBodySize <= 0 {
		cfg.MaxBodySize = DefaultMaxBodySize
	}
	exemptSet := make(map[string]struct{}, len(cfg.JSONExemptPaths))
	for _, p := range cfg.JSONExemptPaths {
		exemptSet[p] = struct{}{}
	}

	return func(c *gin.Context) {
		// Enforce body size limit — prevents unbounded reads (SEC-19).
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, cfg.MaxBodySize)
		}

		// Set max multipart memory.
		c.Request.ParseMultipartForm(DefaultMaxMultipartMemory)

		// Optional Content-Type enforcement for state-changing requests.
		if cfg.EnforceJSONContentType {
			method := c.Request.Method
			if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
				ct := c.GetHeader("Content-Type")
				if ct != "" && !isJSONContentType(ct) {
					// Check exempt paths
				_exempt := false
					for p := range exemptSet {
						if strings.HasPrefix(c.Request.URL.Path, p) {
							_exempt = true
							break
						}
					}
					if !_exempt {
						c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{
							"error": "Content-Type must be application/json",
						})
						return
					}
				}
			}
		}

		c.Next()
	}
}

// SecurityHeadersMiddleware sets standard security headers on every response.
func SecurityHeadersMiddleware() gin.HandlerFunc {
	headers := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"X-XSS-Protection":          "1; mode=block",
		"Referrer-Policy":            "strict-origin-when-cross-origin",
		"Permissions-Policy":         "camera=(), microphone=(), geolocation=()",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Content-Security-Policy":   "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; font-src 'self' data:; img-src 'self' data: https:; connect-src 'self' ws: wss: https: http:; frame-ancestors 'none'",
		"Cache-Control":             "no-store",
	}

	return func(c *gin.Context) {
		for k, v := range headers {
			c.Writer.Header().Set(k, v)
		}
		c.Next()
	}
}

// isJSONContentType checks if the Content-Type is application/json
// (allowing charset suffixes like application/json; charset=utf-8).
func isJSONContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	return strings.HasPrefix(ct, "application/json")
}
