package auth

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// Middleware returns a Gin middleware for JWT validation
func Middleware(validator *TokenValidator) gin.HandlerFunc {
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

		// Validate token
		claims, err := validator.ValidateToken(token)
		if err != nil {
			c.JSON(401, gin.H{
				"error": fmt.Sprintf("invalid token: %v", err),
			})
			c.Abort()
			return
		}

		// Store claims in context
		c.Set("user", claims)
		c.Set("username", claims.PreferredUsername)
		c.Set("email", claims.Email)
		c.Set("roles", claims.collectRoles())

		logging.Z().Info(fmt.Sprintf("✅ Token validated for user: %s (roles: %v)", claims.PreferredUsername, claims.collectRoles()))
		c.Next()
	}
}

// RequireRole returns a middleware that checks if the user has the required role
func RequireRole(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user claims from context
		userInterface, exists := c.Get("user")
		if !exists {
			c.JSON(401, gin.H{
				"error": "unauthorized: no user claims found",
			})
			c.Abort()
			return
		}

		claims, ok := userInterface.(*Claims)
		if !ok {
			c.JSON(401, gin.H{
				"error": "unauthorized: invalid user claims",
			})
			c.Abort()
			return
		}

		// Check if user has required role
		if !claims.HasRole(requiredRole) {
			c.JSON(403, gin.H{
				"error":      fmt.Sprintf("forbidden: user does not have '%s' role", requiredRole),
				"user_roles": claims.collectRoles(),
				"required":   requiredRole,
			})
			c.Abort()
			return
		}

		logging.Z().Info(fmt.Sprintf("✅ User %s authorized with role: %s", claims.PreferredUsername, requiredRole))
		c.Next()
	}
}

// RequireAdmin returns a middleware that checks if the user has admin role
func RequireAdmin() gin.HandlerFunc {
	return RequireRole("admin")
}

// RequireAnyRole returns a middleware that authorizes if the user has any of the provided roles.
func RequireAnyRole(requiredRoles ...string) gin.HandlerFunc {
	normalized := make([]string, 0, len(requiredRoles))
	for _, role := range requiredRoles {
		r := strings.TrimSpace(strings.ToLower(role))
		if r != "" {
			normalized = append(normalized, r)
		}
	}

	return func(c *gin.Context) {
		if len(normalized) == 0 {
			c.Next()
			return
		}

		userInterface, exists := c.Get("user")
		if !exists {
			c.JSON(401, gin.H{
				"error": "unauthorized: no user claims found",
			})
			c.Abort()
			return
		}

		claims, ok := userInterface.(*Claims)
		if !ok {
			c.JSON(401, gin.H{
				"error": "unauthorized: invalid user claims",
			})
			c.Abort()
			return
		}

		for _, role := range normalized {
			if claims.HasRole(role) {
				logging.Z().Info(fmt.Sprintf("✅ User %s authorized with role: %s", claims.PreferredUsername, role))
				c.Next()
				return
			}
		}

		c.JSON(403, gin.H{
			"error":      fmt.Sprintf("forbidden: user must have one of roles %v", normalized),
			"user_roles": claims.collectRoles(),
			"required":   normalized,
		})
		c.Abort()
	}
}

// OptionalMiddleware returns a Gin middleware that doesn't block on missing tokens
func OptionalMiddleware(validator *TokenValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		token, err := ExtractBearerToken(authHeader)
		if err != nil {
			c.Set("user", nil)
			c.Next()
			return
		}

		claims, err := validator.ValidateToken(token)
		if err != nil {
			c.Set("user", nil)
			c.Next()
			return
		}

		c.Set("user", claims)
		c.Set("username", claims.PreferredUsername)
		c.Set("email", claims.Email)
		c.Next()
	}
}

// GetUser extracts the user from context
func GetUser(c *gin.Context) *Claims {
	user, exists := c.Get("user")
	if !exists {
		return nil
	}
	claims, ok := user.(*Claims)
	if !ok {
		return nil
	}
	return claims
}

// GetUsername extracts the username from context
func GetUsername(c *gin.Context) string {
	username, exists := c.Get("username")
	if !exists {
		return ""
	}
	return username.(string)
}
