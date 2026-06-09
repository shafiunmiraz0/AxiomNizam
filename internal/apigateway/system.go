package apigateway

import (
	"log"

	"github.com/gin-gonic/gin"
)

// System is the top-level bootstrap struct for the API gateway module.
// It wraps the Gateway, Handler, and middleware chain into a single
// lifecycle-managed unit.
type System struct {
	gateway *Gateway
	handler *Handler
}

// NewSystem creates a new API gateway system.
func NewSystem() *System {
	gw := NewGateway()
	return &System{
		gateway: gw,
		handler: NewHandler(gw),
	}
}

// Gateway returns the underlying Gateway instance.
func (s *System) Gateway() *Gateway {
	return s.gateway
}

// Handler returns the HTTP handler for gateway management endpoints.
func (s *System) Handler() *Handler {
	return s.handler
}

// RegisterRoutes registers gateway management API endpoints on the router.
func (s *System) RegisterRoutes(rg *gin.RouterGroup) {
	s.handler.RegisterRoutes(rg)
}

// RegisterMiddleware registers the gateway middleware chain on the Gin engine.
// This should be called after global middleware but before route registration.
func (s *System) RegisterMiddleware(router *gin.Engine) {
	if !s.gateway.Config().Enabled {
		log.Println("ℹ️  API Gateway middleware disabled (set API_GATEWAY_ENABLED=true to enable)")
		return
	}

	// Per-endpoint rate limiting
	router.Use(s.gateway.EndpointRateLimitMiddleware())

	// API key authentication (alternative to JWT)
	router.Use(s.gateway.APIKeyMiddleware())

	// Request body validation
	router.Use(s.gateway.RequestValidationMiddleware())

	// API version negotiation
	router.Use(s.gateway.VersionNegotiationMiddleware())

	// Request/response transformation
	router.Use(s.gateway.RequestTransformMiddleware())

	log.Println("✅ API Gateway middleware registered (per-endpoint rate limits, API key auth, validation, versioning)")
}

// Start is a no-op for the gateway (no background goroutines needed beyond
// the rate limiter cleanup which starts automatically).
func (s *System) Start(_ any) error {
	return nil
}

// Name returns the module name.
func (s *System) Name() string {
	return "apigateway"
}
