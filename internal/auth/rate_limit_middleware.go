package auth

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"fmt"

	"github.com/gin-gonic/gin"
)

// RateLimitMiddleware returns a middleware that enforces rate limiting and token validity
func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(401, gin.H{
				"error": "missing authorization header",
			})
			c.Abort()
			return
		}

		// Extract Bearer token
		token, err := ExtractBearerToken(authHeader)
		if err != nil {
			c.JSON(401, gin.H{
				"error": fmt.Sprintf("invalid authorization header: %v", err),
			})
			c.Abort()
			return
		}

		// Check rate limit and token validity
		allowed, callsRemaining, expiresAt, err := limiter.CheckRateLimit(token)
		if !allowed {
			if err.Error() == "token expired" {
				c.JSON(401, gin.H{
					"error":      "token expired",
					"message":    "your token is no longer valid. please login again to get a new token",
					"expired_at": expiresAt.Format("2006-01-02 15:04:05"),
				})
			} else if err.Error() == "token not tracked or invalid" {
				c.JSON(401, gin.H{
					"error": "invalid or unregistered token",
				})
			} else {
				// Call limit exceeded
				c.JSON(401, gin.H{
					"error":           "api call limit exceeded",
					"message":         "you have used all 500 api calls allowed per token",
					"calls_limit":     500,
					"expires_at":      expiresAt.Format("2006-01-02 15:04:05"),
					"action_required": "login again to get a fresh token with new 500 calls",
					"action_endpoint": "/auth/login",
				})
			}
			c.Abort()
			return
		}

		// Increment call count
		if err := limiter.IncrementCallCount(token); err != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  Failed to increment call count: %v", err))
		}

		// Add rate limit info to context
		c.Set("calls_remaining", callsRemaining)
		c.Set("token_expires_at", expiresAt.Format("2006-01-02 15:04:05"))
		c.Set("token", token)

		// Add X-RateLimit headers to response
		c.Header("X-RateLimit-Limit", "500")
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", callsRemaining))
		c.Header("X-Token-Expires-At", expiresAt.Format("2006-01-02 15:04:05"))

		logging.Z().Info(fmt.Sprintf("✅ Rate limit check passed - Calls remaining: %d", callsRemaining))
		c.Next()
	}
}

// CombinedAuthMiddleware validates token AND enforces rate limiting
// Use this for all protected endpoints
func CombinedAuthMiddleware(validator *TokenValidator, limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(401, gin.H{
				"error": "missing authorization header",
			})
			c.Abort()
			return
		}

		// Extract Bearer token
		token, err := ExtractBearerToken(authHeader)
		if err != nil {
			c.JSON(401, gin.H{
				"error": fmt.Sprintf("invalid authorization header: %v", err),
			})
			c.Abort()
			return
		}

		// Check rate limit and token validity first
		allowed, callsRemaining, expiresAt, err := limiter.CheckRateLimit(token)
		if !allowed {
			if err.Error() == "token expired" {
				c.JSON(401, gin.H{
					"error":      "token expired",
					"message":    "your token is no longer valid. please login again to get a new token",
					"expired_at": expiresAt.Format("2006-01-02 15:04:05"),
				})
			} else if err.Error() == "token not tracked or invalid" {
				c.JSON(401, gin.H{
					"error": "invalid or unregistered token",
				})
			} else {
				// Call limit exceeded
				c.JSON(401, gin.H{
					"error":           "api call limit exceeded",
					"message":         "you have used all 500 api calls allowed per token",
					"calls_limit":     500,
					"expires_at":      expiresAt.Format("2006-01-02 15:04:05"),
					"action_required": "login again to get a fresh token with new 500 calls",
					"action_endpoint": "/auth/login",
				})
			}
			c.Abort()
			return
		}

		// Validate JWT token signature and claims
		claims, err := validator.ValidateToken(token)
		if err != nil {
			c.JSON(401, gin.H{
				"error": fmt.Sprintf("invalid token: %v", err),
			})
			c.Abort()
			return
		}

		// Increment call count
		if err := limiter.IncrementCallCount(token); err != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  Failed to increment call count: %v", err))
		}

		// Store claims in context
		c.Set("user", claims)
		c.Set("username", claims.PreferredUsername)
		c.Set("email", claims.Email)
		c.Set("roles", claims.collectRoles())
		c.Set("calls_remaining", callsRemaining)
		c.Set("token_expires_at", expiresAt.Format("2006-01-02 15:04:05"))
		c.Set("token", token)

		// Add X-RateLimit headers to response
		c.Header("X-RateLimit-Limit", "500")
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", callsRemaining))
		c.Header("X-Token-Expires-At", expiresAt.Format("2006-01-02 15:04:05"))

		logging.Z().Info(fmt.Sprintf("✅ Token validated & rate limit OK for user: %s (calls remaining: %d)", claims.PreferredUsername, callsRemaining))
		c.Next()
	}
}
