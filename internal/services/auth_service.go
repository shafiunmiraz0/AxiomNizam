package services

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/repositories"
	"axiomnizam.bitbd.net/axiomnizam/internal/utils"
)

// AuthService defines the interface for authentication business logic
type AuthService interface {
	// Login authenticates a user with username and password
	Login(ctx context.Context, username string, password string) (*models.User, string, error)

	// Register registers a new user
	Register(ctx context.Context, user *models.User, password string) (*models.User, error)

	// ValidateToken validates an authentication token
	ValidateToken(ctx context.Context, token string) (bool, error)

	// RefreshToken refreshes an authentication token
	RefreshToken(ctx context.Context, token string) (string, error)

	// Logout logs out a user
	Logout(ctx context.Context, token string) error

	// Health checks if the service is healthy
	Health() error
}

// authService implements AuthService interface
type authService struct {
	*BaseService
	userRepo repositories.UserRepository
}

// NewAuthService creates a new auth service
func NewAuthService(userRepo repositories.UserRepository, validator *utils.InputValidator, sqlProtection *utils.SQLInjectionProtection) AuthService {
	return &authService{
		BaseService: NewBaseService(validator, sqlProtection),
		userRepo:    userRepo,
	}
}

// Login authenticates a user with username and password
// Returns the user and a JWT token if successful
func (s *authService) Login(ctx context.Context, username string, password string) (*models.User, string, error) {
	if username == "" || password == "" {
		s.LogError("Login", fmt.Errorf("username or password is empty"))
		return nil, "", ErrInvalidInput
	}

	// Validate input
	batch := s.GetValidator().NewValidationBatch().
		AddStringValidation("username", username, utils.WithMinLength(3)).
		AddStringValidation("password", password, utils.WithMinLength(1))

	if batch.HasErrors() {
		s.LogError("Login validation", fmt.Errorf(batch.Error()))
		return nil, "", ErrValidationFailed
	}

	// Get user by email (username is treated as email)
	_, err := s.userRepo.GetByEmail(ctx, username)
	if err != nil {
		if err == repositories.ErrNotFound {
			logging.Z().Info(fmt.Sprintf("LOGIN_ATTEMPT_FAILED: email=%s, reason=not_found", username))
			// Don't reveal if user exists
			return nil, "", ErrUnauthorized
		}
		s.LogError("GetByEmail", err)
		return nil, "", ErrInternalServer
	}

	// Password verification — defer to IAM system.
	// This service is not used in production; main.go uses IAM directly.
	// Returning an error to prevent insecure placeholder tokens.
	s.LogError("Login", fmt.Errorf("auth_service.Login is deprecated — use IAM system"))
	return nil, "", ErrInternalServer
}

// Register registers a new user with validation
func (s *authService) Register(ctx context.Context, user *models.User, password string) (*models.User, error) {
	if user == nil {
		return nil, ErrInvalidInput
	}

	if user.Email == "" {
		s.LogError("Register", fmt.Errorf("email is empty"))
		return nil, ErrInvalidInput
	}

	// Validate password strength
	if !utils.IsValidPassword(password) {
		s.LogError("Register", fmt.Errorf("weak password for user: %s", user.Email))
		return nil, ErrValidationFailed
	}

	// Validate input
	batch := s.GetValidator().NewValidationBatch().
		AddEmailValidation("email", user.Email).
		AddStringValidation("name", user.Name, utils.WithMinLength(1), utils.WithMaxLength(100)).
		AddStringValidation("password", password, utils.WithMinLength(8))

	if batch.HasErrors() {
		s.LogError("Register validation", fmt.Errorf(batch.Error()))
		return nil, ErrValidationFailed
	}

	// Check if email already exists
	existsByEmail, err := s.userRepo.ExistsByEmail(ctx, user.Email)
	if err != nil {
		s.LogError("Register ExistsByEmail", err)
		return nil, ErrInternalServer
	}
	if existsByEmail {
		logging.Z().Info(fmt.Sprintf("REGISTER_FAILED: email=%s, reason=already_exists", user.Email))
		return nil, ErrDuplicateEntry
	}

	// TODO: Hash password using bcrypt
	// hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	// if err != nil {
	//     s.LogError("Register bcrypt", err)
	//     return nil, ErrInternalServer
	// }
	// user.PasswordHash = string(hashedPassword)

	// Create user
	if err := s.userRepo.Create(ctx, user); err != nil {
		s.LogError("Register Create", err)
		return nil, ErrInternalServer
	}

	logging.Z().Info(fmt.Sprintf("REGISTER_SUCCESS: email=%s, name=%s, user_id=%d", user.Email, user.Name, user.ID))
	return user, nil
}

// ValidateToken validates an authentication token
func (s *authService) ValidateToken(ctx context.Context, token string) (bool, error) {
	// Deprecated — defer to IAM system for token validation.
	return false, fmt.Errorf("auth_service.ValidateToken is deprecated — use IAM system")
}

// RefreshToken refreshes an authentication token
func (s *authService) RefreshToken(ctx context.Context, token string) (string, error) {
	// Deprecated — defer to IAM system for token refresh.
	return "", fmt.Errorf("auth_service.RefreshToken is deprecated — use IAM system")
}

// Logout logs out a user
func (s *authService) Logout(ctx context.Context, token string) error {
	if token == "" {
		return ErrInvalidInput
	}

	// TODO: Implement logout logic
	// This might involve blacklisting the token or removing it from cache
	return nil
}

// Health checks if the service is healthy
func (s *authService) Health() error {
	// Try to get a user to check if repository is accessible
	ctx := context.Background()
	_, err := s.userRepo.GetByEmail(ctx, "test@example.com")
	// We don't care if the user exists, just if we can query
	if err != nil && err != repositories.ErrNotFound {
		return err
	}
	return nil
}
