package token

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	iamconfig "example.com/axiomnizam/internal/iam/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// IAMClaims are the JWT claims issued by the IAM system.
// Phase 9: Added continuous verification claims for risk delta, IP change, device change detection.
type IAMClaims struct {
	Sub             string   `json:"sub"`
	Email           string   `json:"email"`
	DisplayName     string   `json:"display_name,omitempty"`
	Roles           []string `json:"roles,omitempty"`
	Scope           string   `json:"scope,omitempty"`
	ClientID        string   `json:"client_id,omitempty"`
	SessionID       string   `json:"sid,omitempty"`
	RiskScore       int      `json:"risk_score,omitempty"`        // 0-100, current risk at token issue
	LastRiskScore   int      `json:"last_risk_score,omitempty"`   // 0-100, previous risk for delta comparison
	LastVerifiedAt  int64    `json:"last_verified_at,omitempty"`  // unix timestamp of last MFA verification
	LastIPAddress   string   `json:"last_ip,omitempty"`           // IP from previous request for change detection
	LastDeviceFP    string   `json:"last_fp,omitempty"`           // device fingerprint for change detection
	jwt.RegisteredClaims
}

// TokenPair holds both access and refresh tokens.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
}

// AccessTokenResponse is used for client_credentials and other access-token-only flows.
type AccessTokenResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int       `json:"expires_in"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	Scope       string    `json:"scope,omitempty"`
}

// IssueInput carries common token issuance arguments.
// Phase 9: Added LastRiskScore and LastVerifiedAt for continuous verification.
type IssueInput struct {
	Sub            string
	Email          string
	DisplayName    string
	Scope          string
	ClientID       string
	SessionID      string
	Roles          []string
	RiskScore      int // 0-100, embedded in JWT claims when > 0
	LastRiskScore  int // 0-100, previous risk score for delta comparison
	LastVerifiedAt int64 // unix timestamp of last MFA verification
}

// JWKSResponse is the /.well-known/jwks.json payload.
type JWKSResponse struct {
	Keys []JWKEntry `json:"keys"`
}

// JWKEntry represents a single JWK public key.
type JWKEntry struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// Issuer handles JWT signing, verification and JWKS exposure.
type Issuer struct {
	mu         sync.RWMutex
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	kid        string
	issuerURL  string

	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	// ExpectedAudience, when set, enables aud claim validation.
	// Tokens whose aud claim does not contain this value are rejected.
	ExpectedAudience string
}

// Default constants re-exported from config package.
const (
	defaultAccessTTL  = iamconfig.DefaultAccessTokenTTL
	defaultRefreshTTL = iamconfig.DefaultRefreshTokenTTL
	rsaKeyBits        = iamconfig.DefaultRSAKeyBits
)

// NewIssuer creates a token issuer. It loads an RSA key from the environment
// or generates an ephemeral one for development.
func NewIssuer(issuerURL string) (*Issuer, error) {
	iss := &Issuer{
		issuerURL:       issuerURL,
		AccessTokenTTL:  defaultAccessTTL,
		RefreshTokenTTL: defaultRefreshTTL,
	}

	if keyPEM := normalizeInlinePEM(os.Getenv("IAM_RSA_PRIVATE_KEY")); strings.TrimSpace(keyPEM) != "" {
		block, _ := pem.Decode([]byte(keyPEM))
		if block == nil {
			return nil, errors.New("IAM_RSA_PRIVATE_KEY: invalid PEM")
		}
		priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("IAM_RSA_PRIVATE_KEY: %w", err)
		}
		iss.privateKey = priv
		iss.publicKey = &priv.PublicKey
	} else if keyPath := os.Getenv("IAM_RSA_PRIVATE_KEY_FILE"); keyPath != "" {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("IAM_RSA_PRIVATE_KEY_FILE: %w", err)
		}
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, errors.New("IAM_RSA_PRIVATE_KEY_FILE: invalid PEM")
		}
		priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("IAM_RSA_PRIVATE_KEY_FILE: %w", err)
		}
		iss.privateKey = priv
		iss.publicKey = &priv.PublicKey
	} else {
		// Ephemeral key for development
		priv, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
		if err != nil {
			return nil, fmt.Errorf("failed to generate RSA key: %w", err)
		}
		iss.privateKey = priv
		iss.publicKey = &priv.PublicKey
	}

	iss.kid = deriveKID(iss.publicKey)
	if iss.kid == "" {
		iss.kid = uuid.New().String()[:8]
	}
	return iss, nil
}

func normalizeInlinePEM(raw string) string {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return ""
	}
	normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\\n", "\n")
	if !strings.HasSuffix(normalized, "\n") {
		normalized += "\n"
	}
	return normalized
}

func deriveKID(pub *rsa.PublicKey) string {
	if pub == nil || pub.N == nil {
		return ""
	}

	material := append([]byte{}, pub.N.Bytes()...)
	material = append(material, byte(pub.E>>24), byte(pub.E>>16), byte(pub.E>>8), byte(pub.E))
	sum := sha256.Sum256(material)
	return hex.EncodeToString(sum[:8])
}

// IssueTokenPair creates a signed access and refresh token pair.
func (iss *Issuer) IssueTokenPair(sub, email, displayName, scope, clientID, sessionID string, roles []string) (*TokenPair, error) {
	return iss.IssueTokenPairWithAccessTTL(IssueInput{
		Sub:         sub,
		Email:       email,
		DisplayName: displayName,
		Scope:       scope,
		ClientID:    clientID,
		SessionID:   sessionID,
		Roles:       roles,
	}, iss.AccessTokenTTL)
}

// IssueTokenPairWithAccessTTL creates a signed access and refresh token pair with custom access token TTL.
func (iss *Issuer) IssueTokenPairWithAccessTTL(input IssueInput, accessTokenTTL time.Duration) (*TokenPair, error) {
	now := time.Now().UTC()
	if accessTokenTTL <= 0 {
		accessTokenTTL = iss.AccessTokenTTL
	}
	accessExp := now.Add(accessTokenTTL)
	refreshExp := now.Add(iss.RefreshTokenTTL)

	accessClaims := IAMClaims{
		Sub:         input.Sub,
		Email:       input.Email,
		DisplayName: input.DisplayName,
		Roles:       input.Roles,
		Scope:       input.Scope,
		ClientID:    input.ClientID,
		SessionID:   input.SessionID,
		RiskScore:      input.RiskScore,
		LastRiskScore:  input.LastRiskScore,
		LastVerifiedAt: input.LastVerifiedAt,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    iss.issuerURL,
			Subject:   input.Sub,
			Audience:  jwt.ClaimStrings{input.ClientID},
			ExpiresAt: jwt.NewNumericDate(accessExp),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        uuid.New().String(),
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodRS256, accessClaims)
	accessToken.Header["kid"] = iss.kid

	signedAccess, err := accessToken.SignedString(iss.privateKey)
	if err != nil {
		return nil, fmt.Errorf("signing access token: %w", err)
	}

	refreshClaims := jwt.RegisteredClaims{
		Issuer:    iss.issuerURL,
		Subject:   input.Sub,
		Audience:  jwt.ClaimStrings{input.ClientID},
		ExpiresAt: jwt.NewNumericDate(refreshExp),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        uuid.New().String(),
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodRS256, refreshClaims)
	refreshToken.Header["kid"] = iss.kid

	signedRefresh, err := refreshToken.SignedString(iss.privateKey)
	if err != nil {
		return nil, fmt.Errorf("signing refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  signedAccess,
		RefreshToken: signedRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessTokenTTL.Seconds()),
		ExpiresAt:    accessExp,
		Scope:        input.Scope,
	}, nil
}

// IssueAccessToken creates a signed access token without issuing a refresh token.
func (iss *Issuer) IssueAccessToken(sub, email, displayName, scope, clientID, sessionID string, roles []string) (*AccessTokenResponse, error) {
	return iss.IssueAccessTokenWithTTL(IssueInput{
		Sub:         sub,
		Email:       email,
		DisplayName: displayName,
		Scope:       scope,
		ClientID:    clientID,
		SessionID:   sessionID,
		Roles:       roles,
	}, iss.AccessTokenTTL)
}

// IssueAccessTokenWithTTL creates a signed access token with a custom TTL and without issuing a refresh token.
func (iss *Issuer) IssueAccessTokenWithTTL(input IssueInput, accessTokenTTL time.Duration) (*AccessTokenResponse, error) {
	now := time.Now().UTC()
	if accessTokenTTL <= 0 {
		accessTokenTTL = iss.AccessTokenTTL
	}
	accessExp := now.Add(accessTokenTTL)

	accessClaims := IAMClaims{
		Sub:         input.Sub,
		Email:       input.Email,
		DisplayName: input.DisplayName,
		Roles:       input.Roles,
		Scope:       input.Scope,
		ClientID:    input.ClientID,
		SessionID:   input.SessionID,
		RiskScore:      input.RiskScore,
		LastRiskScore:  input.LastRiskScore,
		LastVerifiedAt: input.LastVerifiedAt,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    iss.issuerURL,
			Subject:   input.Sub,
			Audience:  jwt.ClaimStrings{input.ClientID},
			ExpiresAt: jwt.NewNumericDate(accessExp),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        uuid.New().String(),
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodRS256, accessClaims)
	accessToken.Header["kid"] = iss.kid

	signedAccess, err := accessToken.SignedString(iss.privateKey)
	if err != nil {
		return nil, fmt.Errorf("signing access token: %w", err)
	}

	return &AccessTokenResponse{
		AccessToken: signedAccess,
		TokenType:   "Bearer",
		ExpiresIn:   int(accessTokenTTL.Seconds()),
		ExpiresAt:   accessExp,
		Scope:       input.Scope,
	}, nil
}

// ValidateAccessToken parses and validates an access token.
// When ExpectedAudience is set on the Issuer, the token's aud claim must contain it.
func (iss *Issuer) ValidateAccessToken(raw string) (*IAMClaims, error) {
	claims := &IAMClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return iss.publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	// Audience validation — reject tokens that don't carry the expected audience.
	if expected := strings.TrimSpace(iss.ExpectedAudience); expected != "" {
		aud := claims.Audience
		if len(aud) == 0 {
			return nil, errors.New("token missing aud claim")
		}
		found := false
		for _, a := range aud {
			if a == expected {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("token audience mismatch: expected %q", expected)
		}
	}

	return claims, nil
}

// ValidateRefreshToken parses a refresh token to extract the subject.
func (iss *Issuer) ValidateRefreshToken(raw string) (*jwt.RegisteredClaims, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return iss.publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("refresh token validation failed: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid refresh token")
	}
	return claims, nil
}

// JWKS returns the JSON Web Key Set containing the public key.
func (iss *Issuer) JWKS() *JWKSResponse {
	iss.mu.RLock()
	defer iss.mu.RUnlock()

	return &JWKSResponse{
		Keys: []JWKEntry{
			{
				Kty: "RSA",
				Use: "sig",
				Kid: iss.kid,
				Alg: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(iss.publicKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(iss.publicKey.E)).Bytes()),
			},
		},
	}
}

// ServeJWKS is an http.HandlerFunc for /.well-known/jwks.json
func (iss *Issuer) ServeJWKS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(iss.JWKS())
}

// IssuerURL returns the configured issuer base URL.
func (iss *Issuer) IssuerURL() string {
	return iss.issuerURL
}

// OpenIDConfigurationWithEndpoints builds an OIDC discovery document for custom endpoint paths.
func (iss *Issuer) OpenIDConfigurationWithEndpoints(issuerURL, authorizationEndpoint, tokenEndpoint, jwksURI string) map[string]interface{} {
	return map[string]interface{}{
		"issuer":                                issuerURL,
		"authorization_endpoint":                authorizationEndpoint,
		"token_endpoint":                        tokenEndpoint,
		"jwks_uri":                              jwksURI,
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email", "roles"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token", "client_credentials"},
		"code_challenge_methods_supported":      []string{"S256"},
	}
}

// OpenIDConfiguration returns the OIDC discovery document.
func (iss *Issuer) OpenIDConfiguration() map[string]interface{} {
	base := strings.TrimRight(iss.issuerURL, "/")
	return iss.OpenIDConfigurationWithEndpoints(
		base,
		base+"/oauth/authorize",
		base+"/oauth/token",
		base+"/.well-known/jwks.json",
	)
}
