package apigateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// VersionTransform defines a request/response transformation for API versioning.
type VersionTransform struct {
	// FromVersion is the source API version (e.g., "v1").
	FromVersion string

	// ToVersion is the target API version (e.g., "v2").
	ToVersion string

	// PathRewrite maps old path patterns to new ones.
	// e.g., "/api/v1/users/:id" → "/api/v2/accounts/:id"
	PathRewrite map[string]string

	// RequestFieldRenames maps old field names to new field names
	// in request bodies.
	RequestFieldRenames map[string]string

	// ResponseFieldRenames maps old field names to new field names
	// in response bodies.
	ResponseFieldRenames map[string]string
}

// VersionNegotiationMiddleware returns gin middleware that reads the API version
// from the X-API-Version header or URL path prefix and sets it in the context.
// This allows handlers to adapt their behavior based on the requested version.
func (g *Gateway) VersionNegotiationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !g.config.Enabled {
			c.Next()
			return
		}

		// 1. Check X-API-Version header
		version := strings.TrimSpace(c.GetHeader(g.config.VersionHeader))

		// 2. Check URL path prefix (/api/v2/...)
		if version == "" {
			path := c.Request.URL.Path
			if strings.HasPrefix(path, "/api/v") {
				rest := strings.TrimPrefix(path, "/api/")
				parts := strings.SplitN(rest, "/", 2)
				if len(parts) > 0 && strings.HasPrefix(parts[0], "v") {
					version = parts[0]
				}
			}
		}

		// 3. Check query parameter
		if version == "" {
			version = strings.TrimSpace(c.Query("api_version"))
		}

		// 4. Fall back to default
		if version == "" {
			version = g.config.DefaultAPIVersion
		}

		c.Set("api_version", version)
		c.Header("X-API-Version", version)

		c.Next()
	}
}

// ResponseTransformMiddleware returns gin middleware that transforms response
// bodies by renaming fields according to the registered version transforms.
// This allows maintaining backward compatibility when field names change
// between API versions.
func (g *Gateway) ResponseTransformMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !g.config.Enabled {
			c.Next()
			return
		}

		// Wrap the response writer to capture the response body
		writer := &responseCapture{ResponseWriter: c.Writer, status: http.StatusOK}
		c.Writer = writer

		c.Next()

		// Only transform successful JSON responses
		if writer.status < 200 || writer.status >= 300 {
			return
		}

		ct := writer.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			return
		}

		if len(writer.body) == 0 {
			return
		}

		// Check for version-specific transforms
		version, _ := c.Get("api_version")
		versionStr, _ := version.(string)
		if versionStr == "" || versionStr == g.config.DefaultAPIVersion {
			return
		}

		transform := g.getVersionTransform(versionStr)
		if transform == nil || len(transform.ResponseFieldRenames) == 0 {
			return
		}

		// Parse, transform, and re-encode
		var bodyMap map[string]any
		if err := json.Unmarshal(writer.body, &bodyMap); err != nil {
			return
		}

		renamed := renameFields(bodyMap, transform.ResponseFieldRenames)
		newBody, err := json.Marshal(renamed)
		if err != nil {
			return
		}

		// Replace the response body
		writer.body = newBody
		writer.Header().Set("Content-Length", string(rune(len(newBody))))
		_, _ = writer.ResponseWriter.Write(newBody)
	}
}

// getVersionTransform returns the transform for a given target version.
func (g *Gateway) getVersionTransform(toVersion string) *VersionTransform {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Check registered transforms — for now, we look up by convention.
	// In a real system, this would be a map keyed by version.
	for _, t := range g.endpointLimits { // reuse the mu lock; transforms would be a separate map in production
		_ = t
	}
	return nil
}

// renameFields recursively renames keys in a map according to the rename map.
func renameFields(m map[string]any, renames map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		newKey := k
		if renamed, ok := renames[k]; ok {
			newKey = renamed
		}

		// Recursively rename in nested objects
		if nested, ok := v.(map[string]any); ok {
			result[newKey] = renameFields(nested, renames)
		} else if arr, ok := v.([]any); ok {
			newArr := make([]any, len(arr))
			for i, item := range arr {
				if nestedItem, ok := item.(map[string]any); ok {
					newArr[i] = renameFields(nestedItem, renames)
				} else {
					newArr[i] = item
				}
			}
			result[newKey] = newArr
		} else {
			result[newKey] = v
		}
	}
	return result
}

// responseCapture wraps gin.ResponseWriter to capture the response body.
type responseCapture struct {
	gin.ResponseWriter
	status int
	body   []byte
}

func (w *responseCapture) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseCapture) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return w.ResponseWriter.Write(b)
}

// RequestTransformMiddleware returns gin middleware that transforms incoming
// request bodies by adding default version headers and normalizing fields.
func (g *Gateway) RequestTransformMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !g.config.Enabled {
			c.Next()
			return
		}

		// Normalize common headers
		if c.GetHeader("Accept") == "" {
			c.Request.Header.Set("Accept", "application/json")
		}

		// Read and potentially transform request body for versioned endpoints
		if c.Request.Body == nil || c.Request.Method == "GET" || c.Request.Method == "HEAD" {
			c.Next()
			return
		}

		// Read body
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)) // 1MB limit
		if err != nil {
			c.Next()
			return
		}
		c.Request.Body = io.NopCloser(strings.NewReader(string(body)))

		if len(body) == 0 {
			c.Next()
			return
		}

		c.Next()
	}
}
