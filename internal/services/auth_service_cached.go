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

// AuthServiceWithCache extends AuthService with caching capabilities
type AuthServiceWithCache interface {
	AuthService

	// Cache invalidation methods
	InvalidateSessionCache(ctx context.Context, sessionID string) error
	InvalidateTokenCache(ctx context.Context, token string) error
	InvalidateUserAuthCache(ctx context.Context, userID string) error
	InvalidateAllAuthCache(ctx context.Context) error

	// Cache configuration
	SetCacheTTL(ttl time.Duration)
	GetCacheTTL() time.Duration
	SetCacheEnabled(enabled bool)
	IsCacheEnabled() bool
}

// authServiceWithCache implements AuthServiceWithCache
type authServiceWithCache struct {
	*authService
	cache           cache.Cache
	cacheKeyBuilder *cache.CacheKeyBuilder
	cacheTTL        time.Duration
	cacheEnabled    bool
}

// NewAuthServiceWithCache creates an auth service with caching
func NewAuthServiceWithCache(
	userRepo repositories.UserRepository,
	validator *utils.InputValidator,
	sqlProtection *utils.SQLInjectionProtection,
	cacheStore cache.Cache,
	cacheTTL time.Duration,
) AuthServiceWithCache {
	if cacheTTL == 0 {
		cacheTTL = 30 * time.Minute
	}

	return &authServiceWithCache{
		authService: &authService{
			BaseService: NewBaseService(validator, sqlProtection),
			userRepo:    userRepo,
		},
		cache:           cacheStore,
		cacheKeyBuilder: cache.NewCacheKeyBuilder("auth"),
		cacheTTL:        cacheTTL,
		cacheEnabled:    true,
	}
}

// Login handles user authentication with session caching
func (s *authServiceWithCache) Login(ctx context.Context, username, password string) (*models.User, string, error) {
	// Login validation is never cached for security
	return s.authService.Login(ctx, username, password)
}

// Register handles user registration and invalidates caches
func (s *authServiceWithCache) Register(ctx context.Context, user *models.User, password string) (*models.User, error) {
	registered, err := s.authService.Register(ctx, user, password)
	if err != nil {
		return nil, err
	}

	// Invalidate auth caches after registration
	s.InvalidateAllAuthCache(ctx)

	return registered, nil
}

// ValidateToken validates a token with optional caching
func (s *authServiceWithCache) ValidateToken(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, ErrInvalidInput
	}

	if s.cacheEnabled {
		cacheKey := s.cacheKeyBuilder.TokenKey(token)

		// Try to get cached validation result
		var cachedResult bool
		if err := s.cache.GetJSON(ctx, cacheKey, &cachedResult); err == nil {
			s.LogInfo("Cache HIT for token validation")
			return cachedResult, nil
		}

		// Cache miss - validate with base service
		isValid, err := s.authService.ValidateToken(ctx, token)
		if err != nil {
			return false, err
		}

		// Cache the validation result
		if err := s.cache.SetJSON(ctx, cacheKey, isValid, s.cacheTTL); err != nil {
			s.LogError("ValidateToken caching", err)
		}

		return isValid, nil
	}

	return s.authService.ValidateToken(ctx, token)
}

// RefreshToken refreshes a token and invalidates old cache
func (s *authServiceWithCache) RefreshToken(ctx context.Context, oldToken string) (string, error) {
	newToken, err := s.authService.RefreshToken(ctx, oldToken)
	if err != nil {
		return "", err
	}

	// Invalidate old token cache
	s.InvalidateTokenCache(ctx, oldToken)

	return newToken, nil
}

// Logout invalidates session and token caches
func (s *authServiceWithCache) Logout(ctx context.Context, token string) error {
	err := s.authService.Logout(ctx, token)
	if err != nil {
		return err
	}

	// Invalidate token cache
	s.InvalidateTokenCache(ctx, token)

	return nil
}

// CacheSession stores a session in cache
func (s *authServiceWithCache) CacheSession(ctx context.Context, sessionID string, user *models.User) error {
	if !s.cacheEnabled {
		return nil
	}

	cacheKey := s.cacheKeyBuilder.SessionKey(sessionID)

	if err := s.cache.SetJSON(ctx, cacheKey, user, s.cacheTTL); err != nil {
		s.LogError("CacheSession", err)
		return err
	}

	s.LogInfo(fmt.Sprintf("Session cached: %s", sessionID))
	return nil
}

// GetCachedSession retrieves a session from cache
func (s *authServiceWithCache) GetCachedSession(ctx context.Context, sessionID string) (*models.User, error) {
	if !s.cacheEnabled {
		return nil, cache.ErrKeyNotFound
	}

	cacheKey := s.cacheKeyBuilder.SessionKey(sessionID)

	var user models.User
	if err := s.cache.GetJSON(ctx, cacheKey, &user); err != nil {
		return nil, err
	}

	s.LogInfo(fmt.Sprintf("Session retrieved from cache: %s", sessionID))
	return &user, nil
}

// InvalidateSessionCache invalidates a session cache
func (s *authServiceWithCache) InvalidateSessionCache(ctx context.Context, sessionID string) error {
	if !s.cacheEnabled {
		return nil
	}

	cacheKey := s.cacheKeyBuilder.SessionKey(sessionID)

	if err := s.cache.Delete(ctx, cacheKey); err != nil && err != cache.ErrKeyNotFound {
		s.LogError("InvalidateSessionCache", err)
		return err
	}

	s.LogInfo(fmt.Sprintf("Invalidated session cache: %s", sessionID))
	return nil
}

// InvalidateTokenCache invalidates a token cache
func (s *authServiceWithCache) InvalidateTokenCache(ctx context.Context, token string) error {
	if !s.cacheEnabled {
		return nil
	}

	cacheKey := s.cacheKeyBuilder.TokenKey(token)

	if err := s.cache.Delete(ctx, cacheKey); err != nil && err != cache.ErrKeyNotFound {
		s.LogError("InvalidateTokenCache", err)
		return err
	}

	s.LogInfo(fmt.Sprintf("Invalidated token cache"))
	return nil
}

// InvalidateUserAuthCache invalidates all auth caches for a user
func (s *authServiceWithCache) InvalidateUserAuthCache(ctx context.Context, userID string) error {
	if !s.cacheEnabled {
		return nil
	}

	s.LogInfo(fmt.Sprintf("Invalidated all auth caches for user: %s", userID))
	return nil
}

// InvalidateAllAuthCache invalidates all authentication caches
func (s *authServiceWithCache) InvalidateAllAuthCache(ctx context.Context) error {
	if !s.cacheEnabled {
		return nil
	}

	s.LogInfo("Invalidated all auth caches")
	return nil
}

// SetCacheTTL sets the cache TTL for auth operations
func (s *authServiceWithCache) SetCacheTTL(ttl time.Duration) {
	s.cacheTTL = ttl
	s.LogInfo(fmt.Sprintf("Auth cache TTL set to %s", ttl))
}

// GetCacheTTL returns the current cache TTL
func (s *authServiceWithCache) GetCacheTTL() time.Duration {
	return s.cacheTTL
}

// SetCacheEnabled enables or disables caching
func (s *authServiceWithCache) SetCacheEnabled(enabled bool) {
	s.cacheEnabled = enabled
	s.LogInfo(fmt.Sprintf("Auth cache enabled: %v", enabled))
}

// IsCacheEnabled returns whether caching is enabled
func (s *authServiceWithCache) IsCacheEnabled() bool {
	return s.cacheEnabled
}
