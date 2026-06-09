package apigateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequestValidationMiddleware returns gin middleware that validates incoming
// request bodies against registered endpoint schemas. When a schema is
// registered for the current method+path, the middleware checks:
//   - Content-Type header matches the expected type
//   - Required fields are present in the JSON body
//   - Field types match the expected types
//   - Body size does not exceed the endpoint's max
//
// Endpoints without a registered schema pass through unvalidated.
func (g *Gateway) RequestValidationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !g.config.Enabled || !g.config.ValidationEnabled {
			c.Next()
			return
		}

		// Only validate request bodies for methods that have them
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "DELETE" {
			c.Next()
			return
		}

		schema := g.GetEndpointSchema(c.Request.Method, c.Request.URL.Path)
		if schema == nil {
			c.Next()
			return
		}

		// Check Content-Type
		if schema.ContentType != "" {
			ct := strings.TrimSpace(strings.SplitN(c.GetHeader("Content-Type"), ";", 2)[0])
			if ct != schema.ContentType {
				c.JSON(http.StatusUnsupportedMediaType, gin.H{
					"error":             "unsupported content type",
					"expected":          schema.ContentType,
					"received":          ct,
				})
				c.Abort()
				return
			}
		}

		// Check body size
		if schema.MaxBodySize > 0 && c.Request.ContentLength > schema.MaxBodySize {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":       "request body too large",
				"max_size":    schema.MaxBodySize,
				"actual_size": c.Request.ContentLength,
			})
			c.Abort()
			return
		}

		// Read and validate JSON body
		if c.Request.Body == nil {
			if len(schema.RequiredFields) > 0 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":           "request body is required",
					"required_fields": schema.RequiredFields,
				})
				c.Abort()
				return
			}
			c.Next()
			return
		}

		// Limit body read to prevent memory exhaustion
		maxRead := int64(1 << 20) // 1MB max for validation
		if schema.MaxBodySize > 0 && schema.MaxBodySize < maxRead {
			maxRead = schema.MaxBodySize
		}

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxRead))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to read request body",
				"message": err.Error(),
			})
			c.Abort()
			return
		}

		// Restore the body for downstream handlers
		c.Request.Body = io.NopCloser(strings.NewReader(string(body)))

		// Parse JSON for field validation
		if len(schema.RequiredFields) > 0 || len(schema.FieldTypes) > 0 {
			var bodyMap map[string]any
			if err := json.Unmarshal(body, &bodyMap); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "invalid JSON in request body",
					"message": err.Error(),
				})
				c.Abort()
				return
			}

			// Check required fields
			missing := make([]string, 0)
			for _, field := range schema.RequiredFields {
				if _, exists := bodyMap[field]; !exists {
					missing = append(missing, field)
				}
			}
			if len(missing) > 0 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":           "missing required fields",
					"missing_fields":  missing,
					"required_fields": schema.RequiredFields,
				})
				c.Abort()
				return
			}

			// Check field types
			typeErrors := make([]string, 0)
			for field, expectedType := range schema.FieldTypes {
				value, exists := bodyMap[field]
				if !exists {
					continue // missing fields already caught above
				}
				if !checkFieldType(value, expectedType) {
					actualType := jsonTypeOf(value)
					typeErrors = append(typeErrors, fmt.Sprintf(
						"field '%s': expected %s, got %s", field, expectedType, actualType))
				}
			}
			if len(typeErrors) > 0 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":      "invalid field types",
					"violations": typeErrors,
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// checkFieldType validates that a JSON value matches the expected type string.
func checkFieldType(value any, expectedType string) bool {
	switch expectedType {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case float64, float32, int, int64, int32:
			return true
		}
		return false
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	default:
		return true // unknown type = no validation
	}
}

// jsonTypeOf returns the JSON type name of a Go value.
func jsonTypeOf(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case float64, float32, int, int64, int32:
		return "number"
	case bool:
		return "boolean"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}
