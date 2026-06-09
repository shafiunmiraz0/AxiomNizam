package auth

import (
	"example.com/axiomnizam/internal/logging"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// RealmAccess contains realm-style roles (legacy compatibility).
type RealmAccess struct {
	Roles []string `json:"roles"`
}

// Claims represents JWT claims from IAM/OIDC tokens.
type Claims struct {
	Sub               string                 `json:"sub"`
	PreferredUsername string                 `json:"preferred_username"`
	Email             string                 `json:"email"`
	DisplayName       string                 `json:"display_name,omitempty"`
	Name              string                 `json:"name"`
	ClientID          string                 `json:"client_id,omitempty"`
	Roles             []string               `json:"roles,omitempty"`
	RealmAccess       RealmAccess            `json:"realm_access"`
	ResourceAccess    map[string]interface{} `json:"resource_access"`
	RiskScore       int    `json:"risk_score,omitempty"`       // 0-100
	LastRiskScore   int    `json:"last_risk_score,omitempty"`  // Phase 9: previous risk for delta
	LastVerifiedAt  int64  `json:"last_verified_at,omitempty"` // Phase 9: last MFA verification
	LastIPAddress   string `json:"last_ip,omitempty"`          // Phase 9: IP change detection
	LastDeviceFP    string `json:"last_fp,omitempty"`          // Phase 9: device change detection
	jwt.RegisteredClaims
}

func normalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func (c *Claims) collectRoles() []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 8)
	add := func(values []string) {
		for _, v := range values {
			n := normalizeRole(v)
			if n == "" {
				continue
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			out = append(out, n)
		}
	}

	add(c.Roles)
	add(c.RealmAccess.Roles)

	for _, clientAccessRaw := range c.ResourceAccess {
		clientAccess, ok := clientAccessRaw.(map[string]interface{})
		if !ok {
			continue
		}

		rolesRaw, ok := clientAccess["roles"]
		if !ok {
			continue
		}

		switch typedRoles := rolesRaw.(type) {
		case []interface{}:
			vals := make([]string, 0, len(typedRoles))
			for _, rr := range typedRoles {
				if roleStr, ok := rr.(string); ok {
					vals = append(vals, roleStr)
				}
			}
			add(vals)
		case []string:
			add(typedRoles)
		}
	}

	return out
}

// RolesList returns normalized roles from both IAM and legacy claim formats.
func (c *Claims) RolesList() []string {
	if c == nil {
		return nil
	}
	return c.collectRoles()
}

func (c *Claims) applyCompatibility() {
	if c == nil {
		return
	}

	roles := c.collectRoles()
	if len(roles) > 0 {
		c.Roles = roles
		c.RealmAccess.Roles = roles
	}

	if strings.TrimSpace(c.PreferredUsername) == "" {
		switch {
		case strings.TrimSpace(c.DisplayName) != "":
			c.PreferredUsername = strings.TrimSpace(c.DisplayName)
		case strings.TrimSpace(c.Name) != "":
			c.PreferredUsername = strings.TrimSpace(c.Name)
		case strings.TrimSpace(c.Email) != "":
			c.PreferredUsername = strings.TrimSpace(c.Email)
		default:
			c.PreferredUsername = strings.TrimSpace(c.Sub)
		}
	}

	if strings.TrimSpace(c.Name) == "" {
		c.Name = c.PreferredUsername
	}
}

// HasRole checks if the claims contain a specific role
func (c *Claims) HasRole(role string) bool {
	if c == nil {
		return false
	}
	targetRole := normalizeRole(role)
	if targetRole == "" {
		return false
	}

	for _, r := range c.collectRoles() {
		if normalizeRole(r) == targetRole {
			return true
		}
	}

	return false
}

// TokenValidatorConfig holds IAM/OIDC token validator configuration.
type TokenValidatorConfig struct {
	// IssuerURL is the URL that hosts OIDC discovery and JWKS, e.g. http://localhost:8000.
	IssuerURL string
	// JWKSURL overrides the JWKS endpoint when discovery URL is not used.
	JWKSURL string

	// Legacy compatibility fields.
	ServerURL string
	Realm     string
	ClientID  string
}

// IAMConfig is an alias for the token validator configuration.
type IAMConfig = TokenValidatorConfig

// JWKS represents the JSON Web Key Set
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a JSON Web Key
type JWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// TokenValidator validates JWT tokens from IAM/OIDC JWKS.
type TokenValidator struct {
	config       *TokenValidatorConfig
	publicKeys   map[string]*rsa.PublicKey
	publicKeysMu sync.RWMutex
	lastFetch    time.Time
}

// NewTokenValidator creates a new token validator
func NewTokenValidator(config *TokenValidatorConfig) (*TokenValidator, error) {
	if config == nil {
		config = &TokenValidatorConfig{}
	}

	tv := &TokenValidator{
		config:     config,
		publicKeys: make(map[string]*rsa.PublicKey),
	}

	// Fetch JWKS from IAM/OIDC provider
	if err := tv.refreshPublicKeys(); err != nil {
		return nil, err
	}

	return tv, nil
}

// refreshPublicKeys fetches the public keys from the configured JWKS endpoint.
func (tv *TokenValidator) refreshPublicKeys() error {
	jwksURL := strings.TrimSpace(tv.config.JWKSURL)
	if jwksURL == "" {
		issuerURL := strings.TrimRight(strings.TrimSpace(tv.config.IssuerURL), "/")
		if issuerURL != "" {
			jwksURL = issuerURL + "/.well-known/jwks.json"
		}
	}

	// Fallback for explicit server URL without issuer metadata.
	if jwksURL == "" {
		serverURL := strings.TrimRight(strings.TrimSpace(tv.config.ServerURL), "/")
		if serverURL != "" {
			jwksURL = serverURL + "/.well-known/jwks.json"
		}
	}

	if jwksURL == "" {
		return fmt.Errorf("token validator: JWKS endpoint is not configured")
	}

	resp, err := http.Get(jwksURL)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read JWKS response: %w", err)
	}

	var jwks JWKS
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("failed to parse JWKS: %w", err)
	}

	tv.publicKeysMu.Lock()
	defer tv.publicKeysMu.Unlock()

	tv.publicKeys = make(map[string]*rsa.PublicKey)
	for _, key := range jwks.Keys {
		if key.Kty == "RSA" {
			pubKey, err := decodeRSAPublicKey(key)
			if err != nil {
				logging.Z().Info(fmt.Sprintf("⚠️  Failed to decode RSA key %s: %v", key.Kid, err))
				continue
			}
			tv.publicKeys[key.Kid] = pubKey
		}
	}

	tv.lastFetch = time.Now()
	logging.Z().Info(fmt.Sprintf("✅ Loaded %d public keys from IAM/OIDC JWKS", len(tv.publicKeys)))
	return nil
}

var (
	demoJWTSecretMu sync.RWMutex
	demoJWTSecret   = loadDemoJWTSecret()
)

func loadDemoJWTSecret() string {
	if secret := strings.TrimSpace(os.Getenv("DEMO_JWT_SECRET")); secret != "" {
		return secret
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err == nil {
		generated := base64.RawURLEncoding.EncodeToString(b)
		logging.Z().Info(fmt.Sprintf("⚠️  DEMO_JWT_SECRET is not set, using generated ephemeral demo token secret"))
		return generated
	}

	fallback := fmt.Sprintf("ephemeral-demo-secret-%d", time.Now().UnixNano())
	logging.Z().Info(fmt.Sprintf("⚠️  DEMO_JWT_SECRET generation failed, falling back to process-ephemeral secret"))
	return fallback
}

// DemoJWTSecret returns the HMAC secret used for demo account tokens.
func DemoJWTSecret() string {
	demoJWTSecretMu.RLock()
	defer demoJWTSecretMu.RUnlock()
	return demoJWTSecret
}

// SetDemoJWTSecret overrides the demo token signing secret at runtime.
func SetDemoJWTSecret(secret string) bool {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return false
	}

	demoJWTSecretMu.Lock()
	demoJWTSecret = trimmed
	demoJWTSecretMu.Unlock()
	return true
}

// ValidateDemoToken validates an HMAC-signed demo token
func (tv *TokenValidator) ValidateDemoToken(tokenString string) (*Claims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(DemoJWTSecret()), nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid demo token")
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid demo token claims")
	}

	claims := &Claims{}
	if v, ok := mapClaims["preferred_username"].(string); ok {
		claims.PreferredUsername = v
	}
	if v, ok := mapClaims["display_name"].(string); ok {
		claims.DisplayName = v
	}
	if v, ok := mapClaims["name"].(string); ok {
		claims.Name = v
	}
	if v, ok := mapClaims["email"].(string); ok {
		claims.Email = v
	}
	if rawRoles, ok := mapClaims["roles"].([]interface{}); ok {
		for _, r := range rawRoles {
			if s, ok := r.(string); ok {
				claims.Roles = append(claims.Roles, s)
			}
		}
	}
	if ra, ok := mapClaims["realm_access"].(map[string]interface{}); ok {
		if roles, ok := ra["roles"].([]interface{}); ok {
			for _, r := range roles {
				if s, ok := r.(string); ok {
					claims.RealmAccess.Roles = append(claims.RealmAccess.Roles, s)
				}
			}
		}
	}
	claims.applyCompatibility()
	return claims, nil
}

// demoTokensAllowed returns true when HMAC demo tokens are explicitly enabled.
// Demo tokens are disabled by default; set ALLOW_DEMO_TOKENS=true to enable them.
func demoTokensAllowed() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ALLOW_DEMO_TOKENS")), "true")
}

// ValidateToken validates a JWT token (JWKS RSA only; demo HMAC fallback is gated
// behind ALLOW_DEMO_TOKENS=true).
func (tv *TokenValidator) ValidateToken(tokenString string) (*Claims, error) {
	// Parse the token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{},
		func(token *jwt.Token) (interface{}, error) {
			// Get the key ID from the token header
			kid, ok := token.Header["kid"].(string)
			if !ok {
				// No kid — only try demo HMAC if explicitly enabled
				if demoTokensAllowed() {
					if claims, demoErr := tv.ValidateDemoToken(tokenString); demoErr == nil {
						return nil, fmt.Errorf("demo:ok:%s", claims.PreferredUsername)
					}
				}
				return nil, fmt.Errorf("kid not found in token header")
			}

			tv.publicKeysMu.RLock()
			pubKey, exists := tv.publicKeys[kid]
			tv.publicKeysMu.RUnlock()

			if !exists {
				// Try refreshing keys if key not found
				if err := tv.refreshPublicKeys(); err != nil {
					return nil, fmt.Errorf("failed to refresh public keys: %w", err)
				}

				tv.publicKeysMu.RLock()
				pubKey, exists = tv.publicKeys[kid]
				tv.publicKeysMu.RUnlock()

				if !exists {
					return nil, fmt.Errorf("key not found in JWKS: %s", kid)
				}
			}

			return pubKey, nil
		})

	if err != nil {
		// Only fall back to demo token validation when explicitly enabled
		if demoTokensAllowed() {
			if demoClaims, demoErr := tv.ValidateDemoToken(tokenString); demoErr == nil {
				logging.Z().Warn(fmt.Sprintf("⚠️  Demo token accepted for user: %s — demo tokens are insecure, disable ALLOW_DEMO_TOKENS in production", demoClaims.PreferredUsername))
				return demoClaims, nil
			}
		}
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Check token expiration
	if claims.ExpiresAt != nil && time.Now().Unix() > claims.ExpiresAt.Unix() {
		return nil, fmt.Errorf("token has expired")
	}

	claims.applyCompatibility()

	return claims, nil
}

// ExtractBearerToken extracts the JWT token from Authorization header
func ExtractBearerToken(authHeader string) (string, error) {
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", fmt.Errorf("invalid authorization header format")
	}
	return parts[1], nil
}

// decodeRSAPublicKey decodes a JWK to RSA public key
func decodeRSAPublicKey(key JWK) (*rsa.PublicKey, error) {
	// Decode base64url encoded values
	nBytes, err := decodeBase64URL(key.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode N: %w", err)
	}

	eBytes, err := decodeBase64URL(key.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode E: %w", err)
	}

	// Convert bytes to big integers
	n := bytesToBigInt(nBytes)
	e := bytesToInt(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: e,
	}, nil
}

// decodeBase64URL decodes base64url encoded string
func decodeBase64URL(str string) ([]byte, error) {
	// Add padding if needed
	switch len(str) % 4 {
	case 2:
		str += "=="
	case 3:
		str += "="
	}

	// Replace URL-safe characters
	str = strings.ReplaceAll(str, "-", "+")
	str = strings.ReplaceAll(str, "_", "/")

	return decodeBase64Standard(str)
}

// decodeBase64Standard decodes standard base64
func decodeBase64Standard(str string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(str)
}

// bytesToBigInt converts bytes to big.Int
func bytesToBigInt(b []byte) *big.Int {
	return new(big.Int).SetBytes(b)
}

// bytesToInt converts bytes to int
func bytesToInt(b []byte) int {
	result := 0
	for _, byte := range b {
		result = (result << 8) | int(byte)
	}
	return result
}
