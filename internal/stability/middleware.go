package stability

import (
	"encoding/json"
	"fmt"
	"strings"

	stderrors "axiomnizam.bitbd.net/axiomnizam/internal/errors"
	"github.com/gin-gonic/gin"
)

// RespondWithError writes a standardized error response.
// This is the canonical way to return errors from handlers.
func RespondWithError(c *gin.Context, err error) {
	status := stderrors.HTTPStatusFromError(err)
	code := stderrors.CodeFromError(err)
	c.JSON(status, stderrors.ErrorResponse{
		Error: err.Error(),
		Code:  code,
	})
}

// RespondWithTypedError writes a standardized error response with detail.
func RespondWithTypedError(c *gin.Context, status int, message, code, detail string) {
	c.JSON(status, stderrors.ErrorResponse{
		Error:   message,
		Code:    code,
		Details: detail,
	})
}

// DeprecationHeaderMiddleware adds Deprecation/Sunset headers to responses
// for endpoints marked as deprecated in the versioning system.
// This is a lightweight alternative to the full versioning middleware.
func DeprecationHeaderMiddleware(deprecatedPaths map[string]DeprecationInfo) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.FullPath()
		method := c.Request.Method
		key := method + " " + path

		if info, exists := deprecatedPaths[key]; exists {
			c.Header("Deprecation", "true")
			if info.SunsetDate != "" {
				c.Header("Sunset", info.SunsetDate)
			}
			if info.Reason != "" {
				c.Header("X-Deprecation-Reason", info.Reason)
			}
			if info.ReplacedBy != "" {
				c.Header("Link", fmt.Sprintf("<%s>; rel=\"successor-version\"", info.ReplacedBy))
			}
		}

		c.Next()
	}
}

// DeprecationInfo holds deprecation metadata for an endpoint.
type DeprecationInfo struct {
	SunsetDate string `json:"sunsetDate,omitempty"`
	Reason     string `json:"reason,omitempty"`
	ReplacedBy string `json:"replacedBy,omitempty"`
}

// RateLimitHeaderMiddleware adds standard rate limit headers to responses.
func RateLimitHeaderMiddleware(limit int, window string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		c.Header("X-RateLimit-Window", window)
		c.Next()
	}
}

// NormalizeErrorResponse checks if a response body is a non-standard error
// format and returns the normalized version. Used for testing.
func NormalizeErrorResponse(body []byte) *stderrors.ErrorResponse {
	// Try standard format first.
	var standard stderrors.ErrorResponse
	if err := json.Unmarshal(body, &standard); err == nil && standard.Error != "" {
		return &standard
	}

	// Try gin.H format: {"error": "..."} or {"message": "..."}
	var adhoc map[string]interface{}
	if err := json.Unmarshal(body, &adhoc); err != nil {
		return nil
	}

	resp := &stderrors.ErrorResponse{}
	if e, ok := adhoc["error"].(string); ok {
		resp.Error = e
	}
	if m, ok := adhoc["message"].(string); ok && resp.Error == "" {
		resp.Error = m
	}
	if c, ok := adhoc["code"].(string); ok {
		resp.Code = c
	}
	if d, ok := adhoc["details"].(string); ok {
		resp.Details = d
	}
	if d, ok := adhoc["detail"].(string); ok && resp.Details == "" {
		resp.Details = d
	}
	return resp
}

// ValidateErrorFormat checks if a JSON response follows the standard error format.
// Returns true if the response has at least the "error" and "code" fields.
func ValidateErrorFormat(body []byte) bool {
	var resp stderrors.ErrorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return false
	}
	return resp.Error != "" && resp.Code != ""
}

// IsStandardErrorCode checks if the error code is a recognized standard code.
func IsStandardErrorCode(code string) bool {
	standardCodes := map[string]bool{
		"NOT_FOUND":            true,
		"ALREADY_EXISTS":       true,
		"CONFLICT":             true,
		"UNAUTHORIZED":         true,
		"FORBIDDEN":            true,
		"INVALID_INPUT":        true,
		"TIMEOUT":              true,
		"UNAVAILABLE":          true,
		"NOT_IMPLEMENTED":      true,
		"RATE_LIMITED":         true,
		"INTERNAL_ERROR":       true,
		"PRECONDITION_FAILED":  true,
	}
	return standardCodes[strings.ToUpper(code)]
}
