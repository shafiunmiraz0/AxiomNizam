package observability

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// CSRFTokenLength is the byte length of generated CSRF tokens.
	CSRFTokenLength = 32

	// CSRFTokenHeader is the header clients must send on state-changing requests.
	CSRFTokenHeader = "X-CSRF-Token"

	// CSRFTokenCookie is the cookie name for the double-submit pattern.
	CSRFTokenCookie = "csrf_token"

	// CSRFTokenMaxAge is the cookie lifetime (12 hours).
	CSRFTokenMaxAge = 12 * 60 * 60
)

// CSRFConfig controls CSRF protection behavior.
type CSRFConfig struct {
	// Secure sets the Secure flag on the CSRF cookie (true in production with HTTPS).
	Secure bool

	// SameSite controls the SameSite cookie attribute.
	// Defaults to "Lax" if empty.
	SameSite string

	// ExemptMethods are HTTP methods that skip CSRF checks (default: GET, HEAD, OPTIONS).
	ExemptMethods []string

	// ExemptPaths are URL path prefixes that skip CSRF checks entirely.
	// Use for login/signup/token endpoints where the user has no cookie yet.
	ExemptPaths []string
}

// DefaultCSRFConfig returns safe defaults for CSRF protection.
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		Secure:        false,
		SameSite:      "Lax",
		ExemptMethods: []string{http.MethodGet, http.MethodHead, http.MethodOptions},
		ExemptPaths: []string{
			"/auth/login",
			"/auth/signup",
			"/auth/register",
			"/auth/token",
			"/auth/refresh",
			"/auth/forgot",
			"/auth/reset",
			"/iam/auth/login",
			"/iam/auth/refresh",
			"/api/v1/auth/login",
			"/api/v1/auth/refresh",
			"/api/v1/auth/logout",
			"/oauth/token",
			"/realms/",
			"/api/health",
			"/api/status",
			"/api/v1/stream",
		},
	}
}

// CSRFConfigWithTLS returns a CSRFConfig with the Secure flag set based on
// whether TLS is enabled. When TLS is active, SameSite is upgraded to Strict
// for maximum CSRF protection. Use this instead of DefaultCSRFConfig when TLS
// configuration is available.
func CSRFConfigWithTLS(tlsEnabled bool) CSRFConfig {
	cfg := DefaultCSRFConfig()
	cfg.Secure = tlsEnabled
	if tlsEnabled {
		cfg.SameSite = "Strict"
	}
	return cfg
}

// CSRFMiddleware implements double-submit cookie CSRF protection.
//
// How it works:
//  1. On safe methods (GET/HEAD/OPTIONS), a random CSRF token is set as a cookie.
//  2. On state-changing methods (POST/PUT/PATCH/DELETE), the middleware checks
//     that the X-CSRF-Token header matches the cookie value.
//  3. The cookie is HttpOnly=false so JavaScript can read it and set the header.
//  4. Requests carrying an Authorization: Bearer token skip CSRF entirely —
//     browsers don't auto-attach Authorization headers cross-site, so CSRF
//     attacks are impossible when using token-based auth.
func CSRFMiddleware(cfg CSRFConfig) gin.HandlerFunc {
	exemptSet := make(map[string]struct{}, len(cfg.ExemptMethods))
	for _, m := range cfg.ExemptMethods {
		exemptSet[m] = struct{}{}
	}
	if cfg.SameSite == "" {
		cfg.SameSite = "Lax"
	}

	return func(c *gin.Context) {
		method := c.Request.Method
		path := c.Request.URL.Path

		// Always issue/refresh the CSRF cookie on safe methods.
		if _, exempt := exemptSet[method]; exempt {
			ensureCSRFCookie(c, cfg)
			c.Next()
			return
		}

		// Exempt requests carrying a Bearer token — JWT auth is CSRF-immune.
		if strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ") {
			c.Next()
			return
		}

		// Exempt paths — login/signup/token endpoints where the user
		// may not have a CSRF cookie yet.
		for _, prefix := range cfg.ExemptPaths {
			if strings.HasPrefix(path, prefix) {
				ensureCSRFCookie(c, cfg)
				c.Next()
				return
			}
		}

		// State-changing method — validate token.
		cookieToken, err := c.Cookie(CSRFTokenCookie)
		if err != nil || cookieToken == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "CSRF cookie missing",
			})
			return
		}

		headerToken := c.GetHeader(CSRFTokenHeader)
		if headerToken == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "CSRF token header missing",
			})
			return
		}

		if subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "CSRF token mismatch",
			})
			return
		}

		// Refresh cookie to extend session.
		ensureCSRFCookie(c, cfg)
		c.Next()
	}
}

// ensureCSRFCookie sets a CSRF cookie if one isn't already present.
func ensureCSRFCookie(c *gin.Context, cfg CSRFConfig) {
	// Only set if not already present.
	if _, err := c.Cookie(CSRFTokenCookie); err == nil {
		return
	}

	token, err := generateCSRFToken()
	if err != nil {
		return
	}

	sameSite := http.SameSiteLaxMode
	switch cfg.SameSite {
	case "Strict":
		sameSite = http.SameSiteStrictMode
	case "None":
		sameSite = http.SameSiteNoneMode
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CSRFTokenCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   CSRFTokenMaxAge,
		HttpOnly: false, // JS must read this to set X-CSRF-Token header
		Secure:   cfg.Secure,
		SameSite: sameSite,
	})

	// Also expose the token in a response header so JS can read it.
	c.Writer.Header().Set(CSRFTokenHeader, token)
}

// generateCSRFToken generates a cryptographically secure random token.
func generateCSRFToken() (string, error) {
	b := make([]byte, CSRFTokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// EnsureFreshness is a helper middleware that adds anti-caching headers
// to API responses to prevent cached responses from being replayed.
func EnsureFreshness() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		c.Writer.Header().Set("Pragma", "no-cache")
		c.Writer.Header().Set("Expires", time.Now().UTC().Format(http.TimeFormat))
		c.Next()
	}
}
