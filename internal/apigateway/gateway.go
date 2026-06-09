package apigateway

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Gateway provides centralized API gateway functionality: per-endpoint rate
// limiting, API key management for external consumers, OpenAPI request
// validation, and request/response transformation.
type Gateway struct {
	config         Config
	endpointLimits map[string]*EndpointRateLimit // path → limit config
	apiKeys        map[string]*APIKey            // keyID → APIKey
	apiKeysByKey   map[string]*APIKey            // raw key → APIKey (for fast lookup)
	schemas        map[string]*EndpointSchema    // method+path → schema
	mu             sync.RWMutex
}

// Config holds gateway configuration loaded from environment variables.
type Config struct {
	// Enabled enables the API gateway middleware chain.
	Enabled bool

	// DefaultRateLimit is the default requests-per-minute for endpoints
	// without a specific limit. 0 means no per-endpoint limit.
	DefaultRateLimit int

	// DefaultRateWindow is the default window duration for rate limiting.
	DefaultRateWindow time.Duration

	// APIKeyHeader is the header name for API key authentication.
	APIKeyHeader string

	// ValidationEnabled enables OpenAPI request body validation.
	ValidationEnabled bool

	// VersionHeader is the header name for API version negotiation.
	VersionHeader string

	// DefaultAPIVersion is the default API version when none specified.
	DefaultAPIVersion string
}

// EndpointRateLimit defines rate limiting for a specific endpoint.
type EndpointRateLimit struct {
	// Path is the route path pattern (e.g., "/api/v1/storage/upload").
	Path string

	// Method is the HTTP method (GET, POST, etc.). Empty means all methods.
	Method string

	// MaxRequests is the maximum number of requests allowed in the window.
	MaxRequests int

	// Window is the time window for the rate limit.
	Window time.Duration

	// KeyBy determines what to rate limit by: "ip", "token", or "apikey".
	KeyBy string
}

// APIKey represents an API key for external consumer authentication.
type APIKey struct {
	// ID is the unique identifier for this API key.
	ID string `json:"id"`

	// Name is a human-readable name for the key.
	Name string `json:"name"`

	// KeyHash is the SHA-256 hash of the actual key (never store raw keys).
	KeyHash string `json:"key_hash"`

	// KeyPrefix is the first 8 characters of the key for identification.
	KeyPrefix string `json:"key_prefix"`

	// Scopes are the allowed API scopes (e.g., "storage:read", "jobs:write").
	Scopes []string `json:"scopes"`

	// RateLimitMaxCalls is the per-minute call limit for this key.
	RateLimitMaxCalls int `json:"rate_limit_max_calls"`

	// ExpiresAt is when this key expires. Zero means no expiration.
	ExpiresAt time.Time `json:"expires_at"`

	// CreatedAt is when this key was created.
	CreatedAt time.Time `json:"created_at"`

	// CreatedBy is the user who created this key.
	CreatedBy string `json:"created_by"`

	// Active indicates whether this key is currently active.
	Active bool `json:"active"`
}

// EndpointSchema defines the expected request body schema for an endpoint.
type EndpointSchema struct {
	// Method is the HTTP method.
	Method string

	// Path is the route path pattern.
	Path string

	// ContentType is the expected content type (e.g., "application/json").
	ContentType string

	// RequiredFields lists field names that must be present in the request body.
	RequiredFields []string

	// FieldTypes maps field names to expected types ("string", "number", "boolean", "object", "array").
	FieldTypes map[string]string

	// MaxBodySize is the maximum request body size in bytes for this endpoint.
	MaxBodySize int64
}

// NewGateway creates a new Gateway instance with configuration loaded from
// environment variables.
func NewGateway() *Gateway {
	cfg := Config{
		Enabled:           strings.EqualFold(strings.TrimSpace(os.Getenv("API_GATEWAY_ENABLED")), "true"),
		DefaultRateLimit:  envInt("API_GATEWAY_DEFAULT_RATE_LIMIT", 0),
		DefaultRateWindow: envDuration("API_GATEWAY_DEFAULT_RATE_WINDOW", 1*time.Minute),
		APIKeyHeader:      envStr("API_GATEWAY_API_KEY_HEADER", "X-API-Key"),
		ValidationEnabled: strings.EqualFold(strings.TrimSpace(os.Getenv("API_GATEWAY_VALIDATION_ENABLED")), "true"),
		VersionHeader:     envStr("API_GATEWAY_VERSION_HEADER", "X-API-Version"),
		DefaultAPIVersion: envStr("API_GATEWAY_DEFAULT_VERSION", "v1"),
	}

	return &Gateway{
		config:         cfg,
		endpointLimits: make(map[string]*EndpointRateLimit),
		apiKeys:        make(map[string]*APIKey),
		apiKeysByKey:   make(map[string]*APIKey),
		schemas:        make(map[string]*EndpointSchema),
	}
}

// Config returns the gateway configuration.
func (g *Gateway) Config() Config {
	return g.config
}

// RegisterEndpointRateLimit adds or updates a per-endpoint rate limit.
func (g *Gateway) RegisterEndpointRateLimit(limit *EndpointRateLimit) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := limit.Method + ":" + limit.Path
	g.endpointLimits[key] = limit
}

// GetEndpointRateLimit returns the rate limit config for a given method+path.
func (g *Gateway) GetEndpointRateLimit(method, path string) *EndpointRateLimit {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Try exact match: METHOD:PATH
	key := method + ":" + path
	if limit, ok := g.endpointLimits[key]; ok {
		return limit
	}

	// Try wildcard method: :PATH
	key = ":" + path
	if limit, ok := g.endpointLimits[key]; ok {
		return limit
	}

	// Try pattern matching (e.g., /api/v1/storage/:id matches /api/v1/storage/abc)
	for k, limit := range g.endpointLimits {
		parts := strings.SplitN(k, ":", 2)
		if len(parts) == 2 {
			limitMethod := parts[0]
			limitPath := parts[1]
			if (limitMethod == "" || limitMethod == method) && matchPath(limitPath, path) {
				return limit
			}
		}
	}

	return nil
}

// RegisterAPIKey adds an API key to the gateway.
func (g *Gateway) RegisterAPIKey(key *APIKey, rawKey string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.apiKeys[key.ID] = key
	if rawKey != "" {
		g.apiKeysByKey[rawKey] = key
	}
}

// ValidateAPIKey checks if a raw API key is valid and returns the associated APIKey.
func (g *Gateway) ValidateAPIKey(rawKey string) (*APIKey, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	key, ok := g.apiKeysByKey[rawKey]
	if !ok {
		return nil, false
	}

	if !key.Active {
		return nil, false
	}

	if !key.ExpiresAt.IsZero() && time.Now().After(key.ExpiresAt) {
		return nil, false
	}

	return key, true
}

// RegisterEndpointSchema adds a request body schema for an endpoint.
func (g *Gateway) RegisterEndpointSchema(schema *EndpointSchema) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := schema.Method + ":" + schema.Path
	g.schemas[key] = schema
}

// GetEndpointSchema returns the schema for a given method+path.
func (g *Gateway) GetEndpointSchema(method, path string) *EndpointSchema {
	g.mu.RLock()
	defer g.mu.RUnlock()
	key := method + ":" + path
	if schema, ok := g.schemas[key]; ok {
		return schema
	}
	// Try wildcard method
	key = ":" + path
	if schema, ok := g.schemas[key]; ok {
		return schema
	}
	return nil
}

// matchPath checks if a request path matches a pattern with :param placeholders.
func matchPath(pattern, path string) bool {
	patternParts := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	pathParts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	if len(patternParts) != len(pathParts) {
		return false
	}

	for i, p := range patternParts {
		if strings.HasPrefix(p, ":") {
			continue // wildcard segment
		}
		if p != pathParts[i] {
			return false
		}
	}

	return true
}

// envStr reads an environment variable with a default fallback.
func envStr(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}

// envInt reads an integer environment variable with a default fallback.
func envInt(key string, defaultVal int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

// envDuration reads a duration environment variable with a default fallback.
// Supports "s", "m", "h" suffixes (e.g., "30s", "5m", "1h").
func envDuration(key string, defaultVal time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}

// RateLimitKey generates a rate limit key from the request context.
func RateLimitKey(c *gin.Context, keyBy string) string {
	switch keyBy {
	case "ip":
		return c.ClientIP()
	case "token":
		if token, exists := c.Get("token"); exists {
			if s, ok := token.(string); ok {
				return s
			}
		}
		return c.ClientIP()
	case "apikey":
		return c.GetHeader("X-API-Key")
	default:
		return c.ClientIP()
	}
}

// LogKey returns a short identifier for logging.
func (k *APIKey) LogKey() string {
	if k.KeyPrefix != "" {
		return fmt.Sprintf("key:%s(%s)", k.ID, k.KeyPrefix)
	}
	return fmt.Sprintf("key:%s", k.ID)
}
