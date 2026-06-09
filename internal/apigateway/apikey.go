package apigateway

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// GenerateAPIKey creates a new random API key and returns the raw key and
// its SHA-256 hash. The raw key should be shown to the user once and never
// stored — only the hash is persisted.
func GenerateAPIKey(prefix string) (rawKey string, keyHash string, keyPrefix string, err error) {
	// Generate 32 random bytes
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", "", fmt.Errorf("failed to generate random key: %w", err)
	}

	// Format: prefix_hex
	randomPart := hex.EncodeToString(bytes)
	if prefix == "" {
		prefix = "axm"
	}
	rawKey = fmt.Sprintf("%s_%s", prefix, randomPart)

	// Hash for storage
	hash := sha256.Sum256([]byte(rawKey))
	keyHash = hex.EncodeToString(hash[:])

	// Prefix for identification (first 8 chars after the prefix)
	if len(randomPart) >= 8 {
		keyPrefix = randomPart[:8]
	} else {
		keyPrefix = randomPart
	}

	return rawKey, keyHash, keyPrefix, nil
}

// HashAPIKey returns the SHA-256 hash of a raw API key.
func HashAPIKey(rawKey string) string {
	hash := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(hash[:])
}

// APIKeyMiddleware returns gin middleware that authenticates requests via the
// X-API-Key header. This is an alternative authentication method for
// service-to-service calls and external consumers. When a valid API key is
// present, the request is authenticated without requiring a JWT token.
//
// The middleware sets the following context values:
//   - "api_key": the validated APIKey struct
//   - "api_key_id": the key ID
//   - "api_key_scopes": the key's scopes
//
// If the API key header is not present, the middleware passes through to the
// next handler (allowing JWT auth to handle it). If the header is present but
// invalid, the request is rejected with 401.
func (g *Gateway) APIKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !g.config.Enabled {
			c.Next()
			return
		}

		apiKeyHeader := strings.TrimSpace(c.GetHeader(g.config.APIKeyHeader))
		if apiKeyHeader == "" {
			// No API key header — pass through to JWT auth
			c.Next()
			return
		}

		// Validate the API key
		apiKey, valid := g.ValidateAPIKey(apiKeyHeader)
		if !valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "invalid API key",
				"message": "the provided API key is invalid, expired, or inactive",
			})
			c.Abort()
			return
		}

		// Check scope if the endpoint requires a specific scope
		if requiredScope, exists := c.Get("required_api_scope"); exists {
			if scope, ok := requiredScope.(string); ok && scope != "" {
				if !apiKey.HasScope(scope) {
					c.JSON(http.StatusForbidden, gin.H{
						"error":            "insufficient API key scope",
						"required_scope":   scope,
						"available_scopes": apiKey.Scopes,
					})
					c.Abort()
					return
				}
			}
		}

		// Set context values for downstream handlers
		c.Set("api_key", apiKey)
		c.Set("api_key_id", apiKey.ID)
		c.Set("api_key_scopes", apiKey.Scopes)
		c.Set("api_key_authenticated", true)

		c.Next()
	}
}

// HasScope checks if the API key has a specific scope.
// Wildcard scope "*" matches everything.
func (k *APIKey) HasScope(scope string) bool {
	for _, s := range k.Scopes {
		if s == "*" || s == scope {
			return true
		}
		// Check prefix match: "storage:*" matches "storage:read"
		if prefix, ok := strings.CutSuffix(s, ":*"); ok {
			if strings.HasPrefix(scope, prefix+":") {
				return true
			}
		}
	}
	return false
}

// RequireAPIScope returns middleware that requires a specific API key scope.
// If the request was authenticated via API key (not JWT), the scope is checked.
// JWT-authenticated requests bypass scope checking (they use RBAC instead).
func RequireAPIScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only enforce scope for API key authenticated requests
		if authenticated, _ := c.Get("api_key_authenticated"); authenticated != true {
			c.Next()
			return
		}

		apiKeyVal, exists := c.Get("api_key")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"error":  "no API key in context",
				"reason": "scope check requires API key authentication",
			})
			c.Abort()
			return
		}

		apiKey, ok := apiKeyVal.(*APIKey)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal API key type error"})
			c.Abort()
			return
		}

		if !apiKey.HasScope(scope) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":            "insufficient API key scope",
				"required_scope":   scope,
				"available_scopes": apiKey.Scopes,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// NewAPIKey creates a new APIKey with the given parameters.
func NewAPIKey(id, name, createdBy string, scopes []string, rateLimitMaxCalls int, ttl time.Duration) *APIKey {
	now := time.Now()
	key := &APIKey{
		ID:                id,
		Name:              name,
		Scopes:            scopes,
		RateLimitMaxCalls: rateLimitMaxCalls,
		CreatedAt:         now,
		CreatedBy:         createdBy,
		Active:            true,
	}
	if ttl > 0 {
		key.ExpiresAt = now.Add(ttl)
	}
	return key
}
