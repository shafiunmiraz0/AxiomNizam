package apigateway

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler provides HTTP endpoints for managing the API gateway configuration.
type Handler struct {
	gateway *Gateway
}

// NewHandler creates a new gateway management handler.
func NewHandler(gw *Gateway) *Handler {
	return &Handler{gateway: gw}
}

// RegisterRoutes registers gateway management endpoints on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/status", h.GetStatus)
	rg.POST("/rate-limits", h.CreateEndpointRateLimit)
	rg.GET("/rate-limits", h.ListEndpointRateLimits)
	rg.DELETE("/rate-limits/:method/:path", h.DeleteEndpointRateLimit)

	rg.POST("/api-keys", h.CreateAPIKey)
	rg.GET("/api-keys", h.ListAPIKeys)
	rg.DELETE("/api-keys/:id", h.RevokeAPIKey)

	rg.POST("/schemas", h.RegisterSchema)
	rg.GET("/schemas", h.ListSchemas)
	rg.DELETE("/schemas/:method/:path", h.DeleteSchema)
}

// GetStatus returns the current gateway status and configuration.
func (h *Handler) GetStatus(c *gin.Context) {
	cfg := h.gateway.Config()
	h.gateway.mu.RLock()
	endpointLimits := len(h.gateway.endpointLimits)
	apiKeys := len(h.gateway.apiKeys)
	schemas := len(h.gateway.schemas)
	h.gateway.mu.RUnlock()

	c.JSON(http.StatusOK, GatewayStatusResponse{
		Status:            "success",
		Enabled:           cfg.Enabled,
		DefaultRateLimit:  cfg.DefaultRateLimit,
		EndpointLimits:    endpointLimits,
		RegisteredAPIKeys: apiKeys,
		ValidationEnabled: cfg.ValidationEnabled,
		RegisteredSchemas: schemas,
		DefaultAPIVersion: cfg.DefaultAPIVersion,
	})
}

// CreateEndpointRateLimitRequest is the request body for creating a rate limit.
type CreateEndpointRateLimitRequest struct {
	Path        string `json:"path" binding:"required"`
	Method      string `json:"method"`
	MaxRequests int    `json:"max_requests" binding:"required,min=1"`
	Window      string `json:"window"` // e.g., "1m", "30s", "1h"
	KeyBy       string `json:"key_by"` // "ip", "token", "apikey"
}

// CreateEndpointRateLimit registers a new per-endpoint rate limit.
func (h *Handler) CreateEndpointRateLimit(c *gin.Context) {
	var req CreateEndpointRateLimitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Status: "error", Message: err.Error()})
		return
	}

	window := 1 * time.Minute
	if req.Window != "" {
		if d, err := time.ParseDuration(req.Window); err == nil {
			window = d
		}
	}

	keyBy := req.KeyBy
	if keyBy == "" {
		keyBy = "ip"
	}

	limit := &EndpointRateLimit{
		Path:        req.Path,
		Method:      req.Method,
		MaxRequests: req.MaxRequests,
		Window:      window,
		KeyBy:       keyBy,
	}

	h.gateway.RegisterEndpointRateLimit(limit)

	c.JSON(http.StatusOK, MessageResponse{
		Status:  "success",
		Message: "endpoint rate limit registered",
	})
}

// ListEndpointRateLimits returns all registered endpoint rate limits.
func (h *Handler) ListEndpointRateLimits(c *gin.Context) {
	h.gateway.mu.RLock()
	limits := make([]*EndpointRateLimitResponse, 0, len(h.gateway.endpointLimits))
	for _, l := range h.gateway.endpointLimits {
		limits = append(limits, &EndpointRateLimitResponse{
			Path:        l.Path,
			Method:      l.Method,
			MaxRequests: l.MaxRequests,
			Window:      l.Window.String(),
			KeyBy:       l.KeyBy,
		})
	}
	h.gateway.mu.RUnlock()

	c.JSON(http.StatusOK, EndpointRateLimitListResponse{
		Status: "success",
		Limits: limits,
		Count:  len(limits),
	})
}

// DeleteEndpointRateLimit removes a registered endpoint rate limit.
func (h *Handler) DeleteEndpointRateLimit(c *gin.Context) {
	method := c.Param("method")
	path := c.Param("path")

	h.gateway.mu.Lock()
	key := method + ":" + path
	_, exists := h.gateway.endpointLimits[key]
	if exists {
		delete(h.gateway.endpointLimits, key)
	}
	h.gateway.mu.Unlock()

	if !exists {
		c.JSON(http.StatusNotFound, MessageResponse{Status: "error", Message: "rate limit not found"})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Status: "success", Message: "endpoint rate limit removed"})
}

// CreateAPIKeyRequest is the request body for creating an API key.
type CreateAPIKeyRequest struct {
	Name              string   `json:"name" binding:"required"`
	Scopes            []string `json:"scopes"`
	RateLimitMaxCalls int      `json:"rate_limit_max_calls"`
	TTLHours          int      `json:"ttl_hours"` // 0 = no expiration
}

// CreateAPIKey generates a new API key for external consumers.
func (h *Handler) CreateAPIKey(c *gin.Context) {
	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Status: "error", Message: err.Error()})
		return
	}

	if len(req.Scopes) == 0 {
		req.Scopes = []string{"*"} // default: full access
	}

	var ttl time.Duration
	if req.TTLHours > 0 {
		ttl = time.Duration(req.TTLHours) * time.Hour
	}

	createdBy := "system"
	if username, exists := c.Get("username"); exists {
		if s, ok := username.(string); ok {
			createdBy = s
		}
	}

	id := uuid.New().String()
	key := NewAPIKey(id, req.Name, createdBy, req.Scopes, req.RateLimitMaxCalls, ttl)

	rawKey, keyHash, keyPrefix, err := GenerateAPIKey("axm")
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Status: "error", Message: "failed to generate API key"})
		return
	}

	key.KeyHash = keyHash
	key.KeyPrefix = keyPrefix

	h.gateway.RegisterAPIKey(key, rawKey)

	c.JSON(http.StatusCreated, APIKeyCreatedResponse{
		Status:    "success",
		ID:        id,
		Name:      req.Name,
		Key:       rawKey,
		KeyPrefix: keyPrefix,
		Scopes:    req.Scopes,
		ExpiresAt: key.ExpiresAt,
		CreatedAt: key.CreatedAt,
		Message:   "store this key securely — it will not be shown again",
	})
}

// ListAPIKeys returns all registered API keys (without secret data).
func (h *Handler) ListAPIKeys(c *gin.Context) {
	h.gateway.mu.RLock()
	keys := make([]*APIKeyInfo, 0, len(h.gateway.apiKeys))
	for _, k := range h.gateway.apiKeys {
		keys = append(keys, &APIKeyInfo{
			ID:                k.ID,
			Name:              k.Name,
			KeyPrefix:         k.KeyPrefix,
			Scopes:            k.Scopes,
			RateLimitMaxCalls: k.RateLimitMaxCalls,
			ExpiresAt:         k.ExpiresAt,
			CreatedAt:         k.CreatedAt,
			CreatedBy:         k.CreatedBy,
			Active:            k.Active,
		})
	}
	h.gateway.mu.RUnlock()

	c.JSON(http.StatusOK, APIKeyListResponse{
		Status: "success",
		Keys:   keys,
		Count:  len(keys),
	})
}

// RevokeAPIKey deactivates an API key by ID.
func (h *Handler) RevokeAPIKey(c *gin.Context) {
	id := c.Param("id")

	h.gateway.mu.Lock()
	key, exists := h.gateway.apiKeys[id]
	if exists {
		key.Active = false
	}
	h.gateway.mu.Unlock()

	if !exists {
		c.JSON(http.StatusNotFound, MessageResponse{Status: "error", Message: "API key not found"})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Status: "success", Message: "API key revoked"})
}

// RegisterSchemaRequest is the request body for registering an endpoint schema.
type RegisterSchemaRequest struct {
	Method         string            `json:"method" binding:"required"`
	Path           string            `json:"path" binding:"required"`
	ContentType    string            `json:"content_type"`
	RequiredFields []string          `json:"required_fields"`
	FieldTypes     map[string]string `json:"field_types"`
	MaxBodySize    int64             `json:"max_body_size"`
}

// RegisterSchema registers a request body validation schema for an endpoint.
func (h *Handler) RegisterSchema(c *gin.Context) {
	var req RegisterSchemaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Status: "error", Message: err.Error()})
		return
	}

	if req.ContentType == "" {
		req.ContentType = "application/json"
	}

	schema := &EndpointSchema{
		Method:         req.Method,
		Path:           req.Path,
		ContentType:    req.ContentType,
		RequiredFields: req.RequiredFields,
		FieldTypes:     req.FieldTypes,
		MaxBodySize:    req.MaxBodySize,
	}

	h.gateway.RegisterEndpointSchema(schema)

	c.JSON(http.StatusOK, MessageResponse{Status: "success", Message: "endpoint schema registered"})
}

// ListSchemas returns all registered endpoint schemas.
func (h *Handler) ListSchemas(c *gin.Context) {
	h.gateway.mu.RLock()
	schemas := make([]*EndpointSchemaResponse, 0, len(h.gateway.schemas))
	for _, s := range h.gateway.schemas {
		schemas = append(schemas, &EndpointSchemaResponse{
			Method:         s.Method,
			Path:           s.Path,
			ContentType:    s.ContentType,
			RequiredFields: s.RequiredFields,
			FieldTypes:     s.FieldTypes,
			MaxBodySize:    s.MaxBodySize,
		})
	}
	h.gateway.mu.RUnlock()

	c.JSON(http.StatusOK, EndpointSchemaListResponse{
		Status:  "success",
		Schemas: schemas,
		Count:   len(schemas),
	})
}

// DeleteSchema removes a registered endpoint schema.
func (h *Handler) DeleteSchema(c *gin.Context) {
	method := c.Param("method")
	path := c.Param("path")

	h.gateway.mu.Lock()
	key := method + ":" + path
	_, exists := h.gateway.schemas[key]
	if exists {
		delete(h.gateway.schemas, key)
	}
	h.gateway.mu.Unlock()

	if !exists {
		c.JSON(http.StatusNotFound, MessageResponse{Status: "error", Message: "schema not found"})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Status: "success", Message: "endpoint schema removed"})
}
