package middleware

import (
	"net/http"
	"os"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/iam/authz"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/storage"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/token"
	"github.com/gin-gonic/gin"
)

type contextKey string

const (
	CtxKeyClaims contextKey = "iam_claims"
	CtxKeyUserID contextKey = "iam_user_id"
	CtxKeyRoles  contextKey = "iam_roles"
)

// JWTAuth validates the Authorization: Bearer <token> header using the IAM Issuer.
// On success it sets "iam_claims", "iam_user_id", and "iam_roles" in the Gin context.
func JWTAuth(issuer *token.Issuer, revokedStore *storage.EtcdRevokedTokenStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := extractBearer(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		claims, err := issuer.ValidateAccessToken(raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		if claims != nil && len(claims.Roles) == 0 {
			configuredSysadminEmail := strings.ToLower(strings.TrimSpace(os.Getenv("IAM_SYSADMIN_EMAIL")))
			claimEmail := strings.ToLower(strings.TrimSpace(claims.Email))
			if configuredSysadminEmail != "" && claimEmail != "" && claimEmail == configuredSysadminEmail {
				claims.Roles = []string{"sysadmin", "system-manager", "admin"}
			}
		}

		// Check revocation (replay-attack prevention)
		if revokedStore != nil && claims.ID != "" {
			revoked, _ := revokedStore.IsRevoked(claims.ID)
			if revoked {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token has been revoked"})
				return
			}
		}

		c.Set(string(CtxKeyClaims), claims)
		c.Set(string(CtxKeyUserID), claims.Sub)
		c.Set(string(CtxKeyRoles), claims.Roles)
		c.Next()
	}
}

// RequirePermission checks that the authenticated user holds a specific permission.
func RequirePermission(authorizer *authz.Authorizer, resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get(string(CtxKeyUserID))
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		allowed, err := authorizer.CheckPermission(userID.(string), resource, action)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "permission check failed"})
			return
		}
		if !allowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":    "insufficient permissions",
				"resource": resource,
				"action":   action,
			})
			return
		}
		c.Next()
	}
}

// RequireSysadmin is a convenience middleware that only allows users with the "sysadmin" role.
func RequireSysadmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		rolesRaw, exists := c.Get(string(CtxKeyRoles))
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		roles, ok := rolesRaw.([]string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		for _, r := range roles {
			normalizedRole := strings.ToLower(strings.TrimSpace(r))
			if normalizedRole == "sysadmin" || normalizedRole == "system-manager" || normalizedRole == "system_admin" || normalizedRole == "system-admin" {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":    "forbidden: sysadmin/system-manager role required",
			"required": []string{"sysadmin", "system-manager", "system_admin", "system-admin"},
		})
	}
}

// RequireRole checks whether the authenticated user holds any of the given roles.
func RequireRole(allowed ...string) gin.HandlerFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		set[strings.ToLower(r)] = struct{}{}
	}
	return func(c *gin.Context) {
		rolesRaw, exists := c.Get(string(CtxKeyRoles))
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		roles, ok := rolesRaw.([]string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		for _, r := range roles {
			if _, match := set[strings.ToLower(r)]; match {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":    "forbidden: required role not found",
			"required": allowed,
		})
	}
}

// GetClaims extracts IAM claims from the Gin context.
func GetClaims(c *gin.Context) *token.IAMClaims {
	v, exists := c.Get(string(CtxKeyClaims))
	if !exists {
		return nil
	}
	claims, ok := v.(*token.IAMClaims)
	if !ok {
		return nil
	}
	return claims
}

// GetUserID extracts the user ID from the Gin context.
func GetUserID(c *gin.Context) string {
	v, _ := c.Get(string(CtxKeyUserID))
	s, _ := v.(string)
	return s
}

func extractBearer(c *gin.Context) (string, error) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", errMissingAuth
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errInvalidAuth
	}
	tok := strings.TrimSpace(parts[1])
	if tok == "" {
		return "", errMissingAuth
	}
	return tok, nil
}

var (
	errMissingAuth = &authError{"missing authorization header"}
	errInvalidAuth = &authError{"invalid authorization header format"}
)

type authError struct{ msg string }

func (e *authError) Error() string { return e.msg }
