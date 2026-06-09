package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// TokenValidatorFunc validates a bearer token and returns the user ID.
// Returns empty string and error if the token is invalid.
type TokenValidatorFunc func(token string) (userID string, err error)

// AuthMiddleware extracts and validates the Bearer token from the Authorization header.
// When a validator is provided, it performs JWT validation; otherwise it only checks format.
func AuthMiddleware(validator TokenValidatorFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			return
		}

		token := parts[1]
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "empty token"})
			return
		}

		if validator != nil {
			userID, err := validator(token)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
			}
			c.Set("user_id", userID)
		}

		c.Set("token", token)
		c.Next()
	}
}

// CORSMiddleware handles Cross-Origin Resource Sharing.
// Pass a set of allowed origins; requests from origins not in the set receive no CORS headers.
func CORSMiddleware(allowedOrigins map[string]struct{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		c.Header("Vary", "Origin")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if origin != "" {
			if _, ok := allowedOrigins[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Credentials", "true")
			}
		}

		if c.Request.Method == "OPTIONS" {
			if origin != "" {
				if _, ok := allowedOrigins[origin]; !ok {
					c.AbortWithStatus(http.StatusForbidden)
					return
				}
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
