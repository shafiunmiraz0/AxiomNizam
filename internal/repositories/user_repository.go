package repositories

import (
	"context"
	"errors"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"gorm.io/gorm"
)

// UserRepository defines the interface for user database operations
type UserRepository interface {
	// Create creates a new user
	Create(ctx context.Context, user *models.User) error

	// GetByID retrieves a user by ID
	GetByID(ctx context.Context, id uint) (*models.User, error)

	// GetByEmail retrieves a user by email
	GetByEmail(ctx context.Context, email string) (*models.User, error)

	// Update updates an existing user
	Update(ctx context.Context, user *models.User) error

	// Delete deletes a user by ID
	Delete(ctx context.Context, id string) error

	// FindAll retrieves all users
	FindAll(ctx context.Context, limit int, offset int) ([]*models.User, error)

	// Count returns the total number of users
	Count(ctx context.Context) (int64, error)

	// Exists checks if a user exists by ID
	Exists(ctx context.Context, id string) (bool, error)

	// ExistsByEmail checks if a user exists by email
	ExistsByEmail(ctx context.Context, email string) (bool, error)

	// ExistsByUsername checks if a user exists by username
	ExistsByUsername(ctx context.Context, username string) (bool, error)
}

// userRepository implements UserRepository interface
type userRepository struct {
	*BaseRepository
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// Create creates a new user in the database
func (r *userRepository) Create(ctx context.Context, user *models.User) error {
	if user == nil {
		return ErrInvalidInput
	}

	if user.Email == "" || user.Name == "" {
		return ErrInvalidInput
	}

	// Check for duplicates
	existsByEmail, err := r.ExistsByEmail(ctx, user.Email)
	if err != nil {
		return err
	}
	if existsByEmail {
		return ErrDuplicateEntry
	}

	result := r.GetDB().WithContext(ctx).Create(user)
	return result.Error
}

// GetByID retrieves a user by ID
func (r *userRepository) GetByID(ctx context.Context, id uint) (*models.User, error) {
	if id == 0 {
		return nil, ErrInvalidInput
	}

	var user models.User
	result := r.GetDB().WithContext(ctx).Where("id = ?", id).First(&user)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}

	if result.Error != nil {
		return nil, result.Error
	}

	return &user, nil
}

// GetByEmail retrieves a user by email
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	if email == "" {
		return nil, ErrInvalidInput
	}

	var user models.User
	result := r.GetDB().WithContext(ctx).Where("email = ?", email).First(&user)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}

	if result.Error != nil {
		return nil, result.Error
	}

	return &user, nil
}

// Update updates an existing user
func (r *userRepository) Update(ctx context.Context, user *models.User) error {
	if user == nil || user.ID == 0 {
		return ErrInvalidInput
	}

	result := r.GetDB().WithContext(ctx).Save(user)
	return result.Error
}

// Delete deletes a user by ID
func (r *userRepository) Delete(ctx context.Context, id string) error {
	if id == "" {
		return ErrInvalidInput
	}

	result := r.GetDB().WithContext(ctx).Delete(&models.User{}, "id = ?", id)
	return result.Error
}

// FindAll retrieves all users with pagination
func (r *userRepository) FindAll(ctx context.Context, limit int, offset int) ([]*models.User, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	var users []*models.User
	result := r.GetDB().WithContext(ctx).
		Limit(limit).
		Offset(offset).
		Find(&users)

	return users, result.Error
}

// Count returns the total number of users
func (r *userRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	result := r.GetDB().WithContext(ctx).Model(&models.User{}).Count(&count)
	return count, result.Error
}

// Exists checks if a user exists by ID
func (r *userRepository) Exists(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, ErrInvalidInput
	}

	var count int64
	result := r.GetDB().WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		Count(&count)

	return count > 0, result.Error
}

// ExistsByEmail checks if a user exists by email
func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	if email == "" {
		return false, ErrInvalidInput
	}

	var count int64
	result := r.GetDB().WithContext(ctx).
		Model(&models.User{}).
		Where("email = ?", email).
		Count(&count)

	return count > 0, result.Error
}

// ExistsByUsername checks if a user exists by username
func (r *userRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	if username == "" {
		return false, ErrInvalidInput
	}

	var count int64
	result := r.GetDB().WithContext(ctx).
		Model(&models.User{}).
		Where("username = ?", username).
		Count(&count)

	return count > 0, result.Error
}
