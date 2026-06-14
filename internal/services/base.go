package services

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"errors"

	"axiomnizam.bitbd.net/axiomnizam/internal/repositories"
	"axiomnizam.bitbd.net/axiomnizam/internal/utils"
)

// Service is the base interface for all services
// Services contain business logic and coordinate between handlers and repositories
type Service interface {
	// Health checks if the service is healthy
	Health() error
}

// BaseService provides common service functionality
type BaseService struct {
	validator     *utils.InputValidator
	sqlProtection *utils.SQLInjectionProtection
}

// NewBaseService creates a new base service
func NewBaseService(validator *utils.InputValidator, sqlProtection *utils.SQLInjectionProtection) *BaseService {
	return &BaseService{
		validator:     validator,
		sqlProtection: sqlProtection,
	}
}

// Health returns a nil error indicating the service is healthy
func (s *BaseService) Health() error {
	return nil
}

// GetValidator returns the input validator
func (s *BaseService) GetValidator() *utils.InputValidator {
	return s.validator
}

// GetSQLProtection returns the SQL injection protection utility
func (s *BaseService) GetSQLProtection() *utils.SQLInjectionProtection {
	return s.sqlProtection
}

// GetLogger returns the service logger

// LogError logs an error message
func (s *BaseService) LogError(msg string, err error) {
	if err != nil {
		logging.Z().Info(fmt.Sprintf("ERROR: %s - %v", msg, err))
	}
}

// LogInfo logs an info message
func (s *BaseService) LogInfo(msg string) {
	logging.Z().Info(fmt.Sprintf("INFO: %s", msg))
}

// Common service errors
var (
	// ErrNotFound is returned when a resource is not found
	ErrNotFound = repositories.ErrNotFound

	// ErrInvalidInput is returned when input is invalid
	ErrInvalidInput = repositories.ErrInvalidInput

	// ErrDuplicateEntry is returned when trying to create a duplicate
	ErrDuplicateEntry = repositories.ErrDuplicateEntry

	// ErrUnauthorized is returned when user is not authorized
	ErrUnauthorized = repositories.ErrUnauthorized

	// ErrForbidden is returned when user is forbidden
	ErrForbidden = repositories.ErrForbidden

	// ErrInternalServer is a generic internal server error
	ErrInternalServer = errors.New("internal server error")

	// ErrValidationFailed is returned when validation fails
	ErrValidationFailed = errors.New("validation failed")
)

// ServiceContainer holds all service instances
// This is useful for dependency injection
type ServiceContainer struct {
	UserService UserService
	AuthService AuthService
	// Add other services here as they are created
}

// NewServiceContainer creates a new service container
func NewServiceContainer(userService UserService, authService AuthService) *ServiceContainer {
	return &ServiceContainer{
		UserService: userService,
		AuthService: authService,
	}
}

// Health checks the health of all services
func (sc *ServiceContainer) Health(ctx context.Context) map[string]error {
	health := make(map[string]error)

	if err := sc.UserService.Health(); err != nil {
		health["user_service"] = err
	}

	if err := sc.AuthService.Health(); err != nil {
		health["auth_service"] = err
	}

	return health
}
