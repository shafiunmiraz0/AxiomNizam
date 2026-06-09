package apigateway

import "time"

// GatewayStatusResponse is returned by the gateway status endpoint.
type GatewayStatusResponse struct {
	Status             string `json:"status"`
	Enabled            bool   `json:"enabled"`
	DefaultRateLimit   int    `json:"default_rate_limit,omitempty"`
	EndpointLimits     int    `json:"endpoint_limits"`
	RegisteredAPIKeys  int    `json:"registered_api_keys"`
	ValidationEnabled  bool   `json:"validation_enabled"`
	RegisteredSchemas  int    `json:"registered_schemas"`
	DefaultAPIVersion  string `json:"default_api_version"`
}

// APIKeyCreatedResponse is returned when an API key is created.
type APIKeyCreatedResponse struct {
	Status    string    `json:"status"`
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	KeyPrefix string    `json:"key_prefix"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Message   string    `json:"message"`
}

// APIKeyListResponse is returned when listing API keys.
type APIKeyListResponse struct {
	Status string      `json:"status"`
	Keys   []*APIKeyInfo `json:"keys"`
	Count  int         `json:"count"`
}

// APIKeyInfo is a sanitized API key representation (no secret data).
type APIKeyInfo struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	KeyPrefix         string    `json:"key_prefix"`
	Scopes            []string  `json:"scopes"`
	RateLimitMaxCalls int       `json:"rate_limit_max_calls"`
	ExpiresAt         time.Time `json:"expires_at,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	CreatedBy         string    `json:"created_by"`
	Active            bool      `json:"active"`
}

// EndpointRateLimitResponse represents a registered endpoint rate limit.
type EndpointRateLimitResponse struct {
	Path        string `json:"path"`
	Method      string `json:"method"`
	MaxRequests int    `json:"max_requests"`
	Window      string `json:"window"`
	KeyBy       string `json:"key_by"`
}

// EndpointRateLimitListResponse is returned when listing endpoint rate limits.
type EndpointRateLimitListResponse struct {
	Status  string                      `json:"status"`
	Limits  []*EndpointRateLimitResponse `json:"limits"`
	Count   int                         `json:"count"`
}

// EndpointSchemaResponse represents a registered endpoint validation schema.
type EndpointSchemaResponse struct {
	Method         string            `json:"method"`
	Path           string            `json:"path"`
	ContentType    string            `json:"content_type"`
	RequiredFields []string          `json:"required_fields,omitempty"`
	FieldTypes     map[string]string `json:"field_types,omitempty"`
	MaxBodySize    int64             `json:"max_body_size,omitempty"`
}

// EndpointSchemaListResponse is returned when listing endpoint schemas.
type EndpointSchemaListResponse struct {
	Status  string                    `json:"status"`
	Schemas []*EndpointSchemaResponse `json:"schemas"`
	Count   int                       `json:"count"`
}

// MessageResponse is a simple status+message response.
type MessageResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
