package apigateway

import "errors"

var (
	// ErrInvalidAPIKey is returned when an API key fails validation.
	ErrInvalidAPIKey = errors.New("invalid API key")

	// ErrAPIKeyExpired is returned when an API key has expired.
	ErrAPIKeyExpired = errors.New("API key expired")

	// ErrAPIKeyInactive is returned when an API key is deactivated.
	ErrAPIKeyInactive = errors.New("API key inactive")

	// ErrInsufficientScope is returned when an API key lacks required scope.
	ErrInsufficientScope = errors.New("insufficient API key scope")

	// ErrRateLimitExceeded is returned when an endpoint rate limit is exceeded.
	ErrRateLimitExceeded = errors.New("endpoint rate limit exceeded")

	// ErrValidationFailed is returned when request body validation fails.
	ErrValidationFailed = errors.New("request validation failed")

	// ErrSchemaNotFound is returned when no schema is registered for an endpoint.
	ErrSchemaNotFound = errors.New("no schema registered for endpoint")
)
