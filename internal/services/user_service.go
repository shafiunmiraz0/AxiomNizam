package services

import (
	"context"
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/repositories"
	"axiomnizam.bitbd.net/axiomnizam/internal/utils"
)

// UserService defines the interface for user business logic
type UserService interface {
	// CreateUser creates a new user with validation and business logic
	CreateUser(ctx context.Context, user *models.User) (*models.User, error)

	// GetUserByID retrieves a user by ID
	GetUserByID(ctx context.Context, id string) (*models.User, error)

	// GetUserByEmail retrieves a user by email
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)

	// GetUserByUsername retrieves a user by username
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)

	// UpdateUser updates an existing user
	UpdateUser(ctx context.Context, user *models.User) (*models.User, error)

	// DeleteUser deletes a user
	DeleteUser(ctx context.Context, id string) error

	// ListUsers lists all users with pagination
	ListUsers(ctx context.Context, limit int, offset int) ([]*models.User, error)

	// GetUserCount returns the total number of users
	GetUserCount(ctx context.Context) (int64, error)

	// UserExists checks if a user exists
	UserExists(ctx context.Context, id string) (bool, error)

	// ValidateUserEmail validates user email and checks for duplicates
	ValidateUserEmail(ctx context.Context, email string) error

	// ValidateUserUsername validates username and checks for duplicates
	ValidateUserUsername(ctx context.Context, username string) error

	// Health checks if the service is healthy
	Health() error
}

// userService implements UserService interface
type userService struct {
	*BaseService
	userRepo repositories.UserRepository
}

// NewUserService creates a new user service
func NewUserService(userRepo repositories.UserRepository, validator *utils.InputValidator, sqlProtection *utils.SQLInjectionProtection) UserService {
	return &userService{
		BaseService: NewBaseService(validator, sqlProtection),
		userRepo:    userRepo,
	}
}

// CreateUser creates a new user with full validation and business logic
func (s *userService) CreateUser(ctx context.Context, user *models.User) (*models.User, error) {
	if user == nil {
		s.LogError("CreateUser", ErrInvalidInput)
		return nil, ErrInvalidInput
	}

	// Validate input using batch validation
	batch := s.GetValidator().NewValidationBatch().
		AddStringValidation("email", user.Email, utils.WithMaxLength(254)).
		AddStringValidation("username", user.Username, utils.WithMinLength(3), utils.WithMaxLength(20)).
		AddStringValidation("password", user.Password, utils.WithMinLength(8))

	if batch.HasErrors() {
		s.LogError("CreateUser validation failed", fmt.Errorf(batch.Error()))
		return nil, ErrValidationFailed
	}

	// Validate email format
	if !utils.IsValidEmail(user.Email) {
		s.LogError("CreateUser email format invalid", fmt.Errorf("invalid email: %s", user.Email))
		return nil, ErrInvalidInput
	}

	// Check for duplicate email
	existsByEmail, err := s.userRepo.ExistsByEmail(ctx, user.Email)
	if err != nil {
		s.LogError("CreateUser ExistsByEmail", err)
		return nil, ErrInternalServer
	}
	if existsByEmail {
		s.LogError("CreateUser duplicate email", fmt.Errorf("email already exists: %s", user.Email))
		return nil, ErrDuplicateEntry
	}

	// Check for duplicate username
	existsByUsername, err := s.userRepo.ExistsByUsername(ctx, user.Username)
	if err != nil {
		s.LogError("CreateUser ExistsByUsername", err)
		return nil, ErrInternalServer
	}
	if existsByUsername {
		s.LogError("CreateUser duplicate username", fmt.Errorf("username already exists: %s", user.Username))
		return nil, ErrDuplicateEntry
	}

	// Create user in repository (password hashing should be done here or in handler)
	if err := s.userRepo.Create(ctx, user); err != nil {
		s.LogError("CreateUser repository create", err)
		return nil, ErrInternalServer
	}

	s.LogInfo(fmt.Sprintf("User created successfully: %s", user.Email))
	return user, nil
}

// GetUserByID retrieves a user by ID
func (s *userService) GetUserByID(ctx context.Context, id string) (*models.User, error) {
	if id == "" {
		return nil, ErrInvalidInput
	}

	var userID uint
	if _, err := fmt.Sscanf(id, "%d", &userID); err != nil {
		return nil, ErrInvalidInput
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if err == repositories.ErrNotFound {
			return nil, ErrNotFound
		}
		s.LogError("GetUserByID", err)
		return nil, ErrInternalServer
	}

	return user, nil
}

// GetUserByEmail retrieves a user by email
func (s *userService) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	if email == "" {
		return nil, ErrInvalidInput
	}

	// Validate email format
	if !utils.IsValidEmail(email) {
		return nil, ErrInvalidInput
	}

	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if err == repositories.ErrNotFound {
			return nil, ErrNotFound
		}
		s.LogError("GetUserByEmail", err)
		return nil, ErrInternalServer
	}

	return user, nil
}

// GetUserByUsername retrieves a user by username
func (s *userService) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	if username == "" {
		return nil, ErrInvalidInput
	}

	// Validate username format
	if !utils.IsValidUsername(username) {
		return nil, ErrInvalidInput
	}

	var user *models.User
	var err error
	// Since GetByUsername is not in the interface, use GetByEmail as alternative
	// or fetch all users and filter - using GetByEmail for now if username matches email pattern
	// TODO: Add GetByUsername to UserRepository interface
	user, err = s.userRepo.GetByEmail(ctx, username)
	if err != nil {
		if err == repositories.ErrNotFound {
			return nil, ErrNotFound
		}
		s.LogError("GetUserByUsername", err)
		return nil, ErrInternalServer
	}

	return user, nil
}

// UpdateUser updates an existing user
func (s *userService) UpdateUser(ctx context.Context, user *models.User) (*models.User, error) {
	if user == nil || user.ID == 0 {
		return nil, ErrInvalidInput
	}

	// Validate input
	if user.Email != "" && !utils.IsValidEmail(user.Email) {
		return nil, ErrInvalidInput
	}

	// Check if user exists
	exists, err := s.userRepo.Exists(ctx, fmt.Sprintf("%d", user.ID))
	if err != nil {
		s.LogError("UpdateUser Exists", err)
		return nil, ErrInternalServer
	}
	if !exists {
		return nil, ErrNotFound
	}

	// Update user
	if err := s.userRepo.Update(ctx, user); err != nil {
		s.LogError("UpdateUser", err)
		return nil, ErrInternalServer
	}

	s.LogInfo(fmt.Sprintf("User updated successfully: %s", user.ID))
	return user, nil
}

// DeleteUser deletes a user
func (s *userService) DeleteUser(ctx context.Context, id string) error {
	if id == "" {
		return ErrInvalidInput
	}

	// Check if user exists
	exists, err := s.userRepo.Exists(ctx, id)
	if err != nil {
		s.LogError("DeleteUser Exists", err)
		return ErrInternalServer
	}
	if !exists {
		return ErrNotFound
	}

	// Delete user
	if err := s.userRepo.Delete(ctx, id); err != nil {
		s.LogError("DeleteUser", err)
		return ErrInternalServer
	}

	s.LogInfo(fmt.Sprintf("User deleted successfully: %s", id))
	return nil
}

// ListUsers lists all users with pagination
func (s *userService) ListUsers(ctx context.Context, limit int, offset int) ([]*models.User, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100 // Max limit
	}
	if offset < 0 {
		offset = 0
	}

	users, err := s.userRepo.FindAll(ctx, limit, offset)
	if err != nil {
		s.LogError("ListUsers", err)
		return nil, ErrInternalServer
	}

	return users, nil
}

// GetUserCount returns the total number of users
func (s *userService) GetUserCount(ctx context.Context) (int64, error) {
	count, err := s.userRepo.Count(ctx)
	if err != nil {
		s.LogError("GetUserCount", err)
		return 0, ErrInternalServer
	}

	return count, nil
}

// UserExists checks if a user exists
func (s *userService) UserExists(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, ErrInvalidInput
	}

	exists, err := s.userRepo.Exists(ctx, id)
	if err != nil {
		s.LogError("UserExists", err)
		return false, ErrInternalServer
	}

	return exists, nil
}

// ValidateUserEmail validates email format and checks for duplicates
func (s *userService) ValidateUserEmail(ctx context.Context, email string) error {
	if email == "" {
		return ErrInvalidInput
	}

	// Validate email format
	if !utils.IsValidEmail(email) {
		return ErrInvalidInput
	}

	// Check if email already exists
	exists, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		s.LogError("ValidateUserEmail", err)
		return ErrInternalServer
	}

	if exists {
		return ErrDuplicateEntry
	}

	return nil
}

// ValidateUserUsername validates username format and checks for duplicates
func (s *userService) ValidateUserUsername(ctx context.Context, username string) error {
	if username == "" {
		return ErrInvalidInput
	}

	// Validate username format
	if !utils.IsValidUsername(username) {
		return ErrInvalidInput
	}

	// Check if username already exists
	exists, err := s.userRepo.ExistsByUsername(ctx, username)
	if err != nil {
		s.LogError("ValidateUserUsername", err)
		return ErrInternalServer
	}

	if exists {
		return ErrDuplicateEntry
	}

	return nil
}

// Health checks if the service is healthy
func (s *userService) Health() error {
	// Check if repository is accessible by trying a count
	ctx := context.Background()
	_, err := s.GetUserCount(ctx)
	return err
}
