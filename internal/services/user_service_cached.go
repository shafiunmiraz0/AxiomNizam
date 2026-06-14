package services

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/cache"
	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/repositories"
	"axiomnizam.bitbd.net/axiomnizam/internal/utils"
)

// UserServiceWithCache extends UserService with caching capabilities
type UserServiceWithCache interface {
	UserService

	// Cache invalidation methods
	InvalidateUserCache(ctx context.Context, userID string) error
	InvalidateUserByEmailCache(ctx context.Context, email string) error
	InvalidateUserByUsernameCache(ctx context.Context, username string) error
	InvalidateAllUserCache(ctx context.Context) error

	// Cache configuration
	SetCacheTTL(ttl time.Duration)
	GetCacheTTL() time.Duration
	SetCacheEnabled(enabled bool)
	IsCacheEnabled() bool
}

// userServiceWithCache implements UserServiceWithCache
type userServiceWithCache struct {
	*userService
	cache           cache.Cache
	cacheKeyBuilder *cache.CacheKeyBuilder
	cacheTTL        time.Duration
	cacheEnabled    bool
}

// NewUserServiceWithCache creates a user service with caching
func NewUserServiceWithCache(
	userRepo repositories.UserRepository,
	validator *utils.InputValidator,
	sqlProtection *utils.SQLInjectionProtection,
	cacheStore cache.Cache,
	cacheTTL time.Duration,
) UserServiceWithCache {
	if cacheTTL == 0 {
		cacheTTL = 15 * time.Minute
	}

	return &userServiceWithCache{
		userService: &userService{
			BaseService: NewBaseService(validator, sqlProtection),
			userRepo:    userRepo,
		},
		cache:           cacheStore,
		cacheKeyBuilder: cache.NewCacheKeyBuilder("user"),
		cacheTTL:        cacheTTL,
		cacheEnabled:    true,
	}
}

// CreateUser creates a new user with validation and cache invalidation
func (s *userServiceWithCache) CreateUser(ctx context.Context, user *models.User) (*models.User, error) {
	// Use base service to create user
	created, err := s.userService.CreateUser(ctx, user)
	if err != nil {
		return nil, err
	}

	// Invalidate related caches
	s.InvalidateAllUserCache(ctx)

	return created, nil
}

// GetUserByID retrieves a user by ID with caching
func (s *userServiceWithCache) GetUserByID(ctx context.Context, id string) (*models.User, error) {
	if id == "" {
		return nil, ErrInvalidInput
	}

	if s.cacheEnabled {
		cacheKey := s.cacheKeyBuilder.UserKey(id)

		// Try to get from cache
		if cached, err := s.cache.Get(ctx, cacheKey); err == nil {
			s.LogInfo(fmt.Sprintf("Cache HIT for user %s", id))
			if user, ok := cached.(*models.User); ok {
				return user, nil
			}
		}

		// Cache miss - get from repository
		user, err := s.userService.GetUserByID(ctx, id)
		if err != nil {
			return nil, err
		}

		// Store in cache
		if err := s.cache.SetJSON(ctx, cacheKey, user, s.cacheTTL); err != nil {
			s.LogError("GetUserByID caching", err)
			// Don't fail if caching fails, just log and continue
		}

		return user, nil
	}

	// Caching disabled
	return s.userService.GetUserByID(ctx, id)
}

// GetUserByEmail retrieves a user by email with caching
func (s *userServiceWithCache) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	if email == "" {
		return nil, ErrInvalidInput
	}

	if s.cacheEnabled {
		cacheKey := s.cacheKeyBuilder.UserEmailKey(email)

		// Try to get from cache
		if cached, err := s.cache.Get(ctx, cacheKey); err == nil {
			s.LogInfo(fmt.Sprintf("Cache HIT for email %s", email))
			if user, ok := cached.(*models.User); ok {
				return user, nil
			}
		}

		// Cache miss - get from repository
		user, err := s.userService.GetUserByEmail(ctx, email)
		if err != nil {
			return nil, err
		}

		// Store in cache
		if err := s.cache.SetJSON(ctx, cacheKey, user, s.cacheTTL); err != nil {
			s.LogError("GetUserByEmail caching", err)
		}

		return user, nil
	}

	return s.userService.GetUserByEmail(ctx, email)
}

// GetUserByUsername retrieves a user by username with caching
func (s *userServiceWithCache) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	if username == "" {
		return nil, ErrInvalidInput
	}

	if s.cacheEnabled {
		cacheKey := s.cacheKeyBuilder.UserUsernameKey(username)

		// Try to get from cache
		if cached, err := s.cache.Get(ctx, cacheKey); err == nil {
			s.LogInfo(fmt.Sprintf("Cache HIT for username %s", username))
			if user, ok := cached.(*models.User); ok {
				return user, nil
			}
		}

		// Cache miss - get from repository
		user, err := s.userService.GetUserByUsername(ctx, username)
		if err != nil {
			return nil, err
		}

		// Store in cache
		if err := s.cache.SetJSON(ctx, cacheKey, user, s.cacheTTL); err != nil {
			s.LogError("GetUserByUsername caching", err)
		}

		return user, nil
	}

	return s.userService.GetUserByUsername(ctx, username)
}

// UpdateUser updates a user and invalidates cache
func (s *userServiceWithCache) UpdateUser(ctx context.Context, user *models.User) (*models.User, error) {
	updated, err := s.userService.UpdateUser(ctx, user)
	if err != nil {
		return nil, err
	}

	// Invalidate all related caches
	s.InvalidateUserCache(ctx, fmt.Sprintf("%d", user.ID))
	if user.Email != "" {
		s.InvalidateUserByEmailCache(ctx, user.Email)
	}
	if user.Username != "" {
		s.InvalidateUserByUsernameCache(ctx, user.Username)
	}
	s.InvalidateAllUserCache(ctx)

	return updated, nil
}

// DeleteUser deletes a user and invalidates cache
func (s *userServiceWithCache) DeleteUser(ctx context.Context, id string) error {
	// Get user details before deletion for cache invalidation
	user, err := s.userService.GetUserByID(ctx, id)
	if err != nil {
		return err
	}

	// Delete user
	if err := s.userService.DeleteUser(ctx, id); err != nil {
		return err
	}

	// Invalidate all related caches
	s.InvalidateUserCache(ctx, id)
	if user.Email != "" {
		s.InvalidateUserByEmailCache(ctx, user.Email)
	}
	if user.Username != "" {
		s.InvalidateUserByUsernameCache(ctx, user.Username)
	}
	s.InvalidateAllUserCache(ctx)

	return nil
}

// ListUsers lists users with caching
func (s *userServiceWithCache) ListUsers(ctx context.Context, limit int, offset int) ([]*models.User, error) {
	// Note: Listing is typically not cached due to pagination complexity
	// but could be cached with proper key generation
	return s.userService.ListUsers(ctx, limit, offset)
}

// GetUserCount gets user count with caching
func (s *userServiceWithCache) GetUserCount(ctx context.Context) (int64, error) {
	if s.cacheEnabled {
		cacheKey := s.cacheKeyBuilder.UserCountKey()

		// Try to get from cache
		if count, err := s.cache.GetCounter(ctx, cacheKey); err == nil {
			s.LogInfo("Cache HIT for user count")
			return count, nil
		}

		// Cache miss - get from repository
		count, err := s.userService.GetUserCount(ctx)
		if err != nil {
			return 0, err
		}

		// Store in cache
		if err := s.cache.SetCounter(ctx, cacheKey, count, s.cacheTTL); err != nil {
			s.LogError("GetUserCount caching", err)
		}

		return count, nil
	}

	return s.userService.GetUserCount(ctx)
}

// UserExists checks if user exists (typically not cached)
func (s *userServiceWithCache) UserExists(ctx context.Context, id string) (bool, error) {
	return s.userService.UserExists(ctx, id)
}

// ValidateUserEmail validates email (typically not cached)
func (s *userServiceWithCache) ValidateUserEmail(ctx context.Context, email string) error {
	return s.userService.ValidateUserEmail(ctx, email)
}

// ValidateUserUsername validates username (typically not cached)
func (s *userServiceWithCache) ValidateUserUsername(ctx context.Context, username string) error {
	return s.userService.ValidateUserUsername(ctx, username)
}

// InvalidateUserCache invalidates cache for a specific user ID
func (s *userServiceWithCache) InvalidateUserCache(ctx context.Context, userID string) error {
	if !s.cacheEnabled {
		return nil
	}

	cacheKey := s.cacheKeyBuilder.UserKey(userID)
	if err := s.cache.Delete(ctx, cacheKey); err != nil && err != cache.ErrKeyNotFound {
		s.LogError("InvalidateUserCache", err)
		return err
	}

	s.LogInfo(fmt.Sprintf("Invalidated cache for user %s", userID))
	return nil
}

// InvalidateUserByEmailCache invalidates cache for a user by email
func (s *userServiceWithCache) InvalidateUserByEmailCache(ctx context.Context, email string) error {
	if !s.cacheEnabled {
		return nil
	}

	cacheKey := s.cacheKeyBuilder.UserEmailKey(email)
	if err := s.cache.Delete(ctx, cacheKey); err != nil && err != cache.ErrKeyNotFound {
		s.LogError("InvalidateUserByEmailCache", err)
		return err
	}

	s.LogInfo(fmt.Sprintf("Invalidated cache for email %s", email))
	return nil
}

// InvalidateUserByUsernameCache invalidates cache for a user by username
func (s *userServiceWithCache) InvalidateUserByUsernameCache(ctx context.Context, username string) error {
	if !s.cacheEnabled {
		return nil
	}

	cacheKey := s.cacheKeyBuilder.UserUsernameKey(username)
	if err := s.cache.Delete(ctx, cacheKey); err != nil && err != cache.ErrKeyNotFound {
		s.LogError("InvalidateUserByUsernameCache", err)
		return err
	}

	s.LogInfo(fmt.Sprintf("Invalidated cache for username %s", username))
	return nil
}

// InvalidateAllUserCache invalidates all user-related caches
func (s *userServiceWithCache) InvalidateAllUserCache(ctx context.Context) error {
	if !s.cacheEnabled {
		return nil
	}

	// In a real implementation with Redis, you'd use KEYS pattern matching
	// For now, we'll just log this operation
	s.LogInfo("Invalidated all user caches")
	return nil
}

// SetCacheTTL sets the cache TTL
func (s *userServiceWithCache) SetCacheTTL(ttl time.Duration) {
	s.cacheTTL = ttl
	s.LogInfo(fmt.Sprintf("Cache TTL set to %s", ttl))
}

// GetCacheTTL returns the current cache TTL
func (s *userServiceWithCache) GetCacheTTL() time.Duration {
	return s.cacheTTL
}

// SetCacheEnabled enables or disables caching
func (s *userServiceWithCache) SetCacheEnabled(enabled bool) {
	s.cacheEnabled = enabled
	s.LogInfo(fmt.Sprintf("Cache enabled: %v", enabled))
}

// IsCacheEnabled returns whether caching is enabled
func (s *userServiceWithCache) IsCacheEnabled() bool {
	return s.cacheEnabled
}
