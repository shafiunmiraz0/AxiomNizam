package authn

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"example.com/axiomnizam/internal/auth"
	iamidentity "example.com/axiomnizam/internal/iam/identity"
	iammodels "example.com/axiomnizam/internal/iam/models"
	"example.com/axiomnizam/internal/logging"
	"example.com/axiomnizam/internal/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	gojwt "github.com/golang-jwt/jwt/v5"
)

// PlatformUserStore is the interface AuthHandler needs from the platform user
// handler. It avoids a circular import between authn and users packages.
type PlatformUserStore interface {
	ValidateCredentials(username, password string) (*PlatformUser, bool)
	EnsureFederatedUser(username, email, defaultRole string) (*PlatformUser, error)
}

// PlatformUser is a minimal projection of the platform user type needed by
// AuthHandler. The full type lives in the users package.
type PlatformUser struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

// IdentityProviderStore is the interface for looking up configured identity providers.
type IdentityProviderStore interface {
	ListIdentityProviders(realmID string) ([]iammodels.IdentityProvider, error)
}

// IAMRoleResolver resolves role names for a given user ID.
type IAMRoleResolver interface {
	GetUserRoleNames(userID string) ([]string, error)
}

// AuthHandler handles authentication requests
type AuthHandler struct {
	iamBaseURL    string
	rateLimiter   *auth.RateLimiter
	platformUsers PlatformUserStore
	idpStore      IdentityProviderStore
	iamUsers      UserRepository
	iamAuthorizer IAMRoleResolver
	httpClient    *http.Client
}

const (
	headerContentType  = "Content-Type"
	oauthStateCookie   = "axiomnizam_oauth_state"
	oauthStateMaxAge   = 600
	oauthStateLifetime = 10 * time.Minute
)

// NewAuthHandler creates a new auth handler
func NewAuthHandler() *AuthHandler {
	iamBaseURL := strings.TrimSpace(getEnv("IAM_INTERNAL_BASE_URL", ""))
	if iamBaseURL == "" {
		iamBaseURL = defaultIAMInternalBaseURL()
	}
	if iamBaseURL == "" {
		iamBaseURL = strings.TrimSpace(getEnv("IAM_ISSUER_URL", ""))
	}
	iamBaseURL = normalizeIAMBaseURL(iamBaseURL)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TLS_ENABLED")), "true") {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return &AuthHandler{
		iamBaseURL:  iamBaseURL,
		rateLimiter: nil, // Will be set via SetRateLimiter
		httpClient: &http.Client{
			Timeout:   iamAuthProxyTimeout(),
			Transport: transport,
		},
	}
}

// SetRateLimiter sets the rate limiter for the auth handler
func (h *AuthHandler) SetRateLimiter(limiter *auth.RateLimiter) {
	h.rateLimiter = limiter
}

// SetPlatformUserHandler wires the platform user store into the auth handler
// so that users created via the sysadmin UI can log in.
func (h *AuthHandler) SetPlatformUserHandler(puh PlatformUserStore) {
	h.platformUsers = puh
}

// SetIdentityProviderStore wires IAM identity provider persistence into auth flows.
func (h *AuthHandler) SetIdentityProviderStore(store IdentityProviderStore) {
	h.idpStore = store
}

// SetIAMUserRepository wires IAM user repository into OAuth provisioning flow.
func (h *AuthHandler) SetIAMUserRepository(repo UserRepository) {
	h.iamUsers = repo
}

// SetIAMAuthorizer wires IAM role resolver into OAuth provisioning flow.
func (h *AuthHandler) SetIAMAuthorizer(authorizer IAMRoleResolver) {
	h.iamAuthorizer = authorizer
}

// getEnv gets environment variable with fallback
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func iamScheme() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TLS_ENABLED")), "true") {
		return "https"
	}
	return "http"
}

func defaultIAMInternalBaseURL() string {
	host := strings.TrimSpace(getEnv("API_HOST", "localhost"))
	port := apiPortOrDefault()
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return fmt.Sprintf("%s://%s:%s", iamScheme(), host, port)
}

func apiPortOrDefault() string {
	port := strings.TrimSpace(getEnv("API_PORT", "8000"))
	if port == "" {
		return "8000"
	}
	return port
}

func defaultIAMLoopbackBaseURL() string {
	return fmt.Sprintf("%s://127.0.0.1:%s", iamScheme(), apiPortOrDefault())
}

func defaultIAMServiceBaseURL() string {
	host := strings.TrimSpace(getEnv("IAM_INTERNAL_SERVICE_HOST", "axiomnizam"))
	if host == "" {
		host = "axiomnizam"
	}
	return fmt.Sprintf("%s://%s:%s", iamScheme(), host, apiPortOrDefault())
}

func iamAuthProxyTimeout() time.Duration {
	raw := strings.TrimSpace(getEnv("IAM_AUTH_PROXY_TIMEOUT", "25s"))
	if raw == "" {
		return 25 * time.Second
	}

	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return 25 * time.Second
	}
	if timeout > 2*time.Minute {
		return 2 * time.Minute
	}
	return timeout
}

func iamLoginBaseCandidates(primaryBase string) []string {
	rawCandidates := []string{
		primaryBase,
		defaultIAMLoopbackBaseURL(),
		defaultIAMInternalBaseURL(),
		defaultIAMServiceBaseURL(),
		strings.TrimSpace(getEnv("IAM_ISSUER_URL", "")),
	}

	seen := map[string]struct{}{}
	candidates := make([]string, 0, len(rawCandidates))
	for _, raw := range rawCandidates {
		normalized := normalizeIAMBaseURL(raw)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		candidates = append(candidates, normalized)
	}

	return candidates
}

func normalizeIAMBaseURL(raw string) string {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return ""
	}

	parsed, err := url.Parse(candidate)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/")
	}

	return strings.TrimRight(candidate, "/")
}

func summarizeIAMBody(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "empty response body"
	}

	compact := strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(text, "\r", " "), "\n", " ")), " ")
	if strings.HasPrefix(strings.ToLower(compact), "<!doctype") || strings.HasPrefix(strings.ToLower(compact), "<html") || strings.HasPrefix(compact, "<") {
		return "upstream returned HTML instead of JSON"
	}
	if len(compact) > 220 {
		return compact[:220] + "..."
	}
	return compact
}

func shouldRetryIAMLogin(resp *http.Response, responseBody []byte, reqErr error) bool {
	if reqErr != nil {
		return true
	}
	if resp == nil {
		return true
	}

	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get(headerContentType)))
	bodySummary := summarizeIAMBody(responseBody)
	htmlResponse := strings.Contains(contentType, "text/html") || strings.Contains(bodySummary, "HTML instead of JSON")

	if htmlResponse {
		return true
	}

	if resp.StatusCode == http.StatusOK && !json.Valid(responseBody) {
		return true
	}

	return false
}

type oauthStatePayload struct {
	State          string `json:"state"`
	ProviderKey    string `json:"provider_key"`
	ReturnTo       string `json:"return_to"`
	FrontendOrigin string `json:"frontend_origin"`
	CodeVerifier   string `json:"code_verifier"`
	IssuedAt       int64  `json:"issued_at"`
}

type oauthProviderTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type oauthIdentityProfile struct {
	Subject           string
	Email             string
	Name              string
	DisplayName       string
	PreferredUsername string
}

func oauthStateSecret() string {
	if secret := strings.TrimSpace(getEnv("OAUTH_STATE_SECRET", "")); secret != "" {
		return secret
	}
	return auth.DemoJWTSecret()
}

func isHTTPSRequest(c *gin.Context) bool {
	if c != nil && c.Request != nil && c.Request.TLS != nil {
		return true
	}
	proto := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")))
	if strings.Contains(proto, "https") {
		return true
	}
	return false
}

func requestScheme(c *gin.Context) string {
	if isHTTPSRequest(c) {
		return "https"
	}
	proto := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")))
	if proto != "" {
		if comma := strings.Index(proto, ","); comma > 0 {
			proto = strings.TrimSpace(proto[:comma])
		}
		if proto == "http" || proto == "https" {
			return proto
		}
	}
	return "http"
}

func requestHost(c *gin.Context) string {
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host != "" {
		if comma := strings.Index(host, ","); comma > 0 {
			host = strings.TrimSpace(host[:comma])
		}
		if host != "" {
			return host
		}
	}
	if c != nil && c.Request != nil {
		return strings.TrimSpace(c.Request.Host)
	}
	return ""
}

func requestBaseURL(c *gin.Context) string {
	host := requestHost(c)
	if host == "" {
		return ""
	}
	return requestScheme(c) + "://" + host
}

func oauthCallbackURL(c *gin.Context) string {
	base := strings.TrimRight(requestBaseURL(c), "/")
	if base == "" {
		base = strings.TrimRight(normalizeIAMBaseURL(getEnv("IAM_ISSUER_URL", "")), "/")
	}
	if base == "" {
		base = strings.TrimRight(normalizeIAMBaseURL(defaultIAMInternalBaseURL()), "/")
	}
	return base + "/auth/oauth/callback"
}

func normalizeOrigin(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return scheme + "://" + strings.TrimSpace(parsed.Host)
}

func resolveFrontendOrigin(c *gin.Context, requested string) string {
	if origin := normalizeOrigin(requested); origin != "" {
		return origin
	}

	if origin := normalizeOrigin(strings.TrimSpace(getEnv("PUBLIC_FRONTEND_URL", ""))); origin != "" {
		return origin
	}

	if host := strings.TrimSpace(getEnv("PUBLIC_FRONTEND_HOSTNAME", "")); host != "" {
		scheme := requestScheme(c)
		if strings.Contains(host, "localhost") || strings.HasPrefix(host, "127.") {
			scheme = "http"
		}
		if origin := normalizeOrigin(scheme + "://" + host); origin != "" {
			return origin
		}
	}

	if ref := normalizeOrigin(strings.TrimSpace(c.GetHeader("Referer"))); ref != "" {
		return ref
	}

	return normalizeOrigin(requestBaseURL(c))
}

func sanitizeReturnPath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "/"
	}
	if strings.Contains(value, "://") || strings.HasPrefix(value, "//") {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func defaultPathForRole(role string) string {
	r := strings.ToLower(strings.TrimSpace(role))
	switch {
	case r == "system-manager" || r == "system_manager" || r == "system-admin" || r == "sysadmin":
		return "/system-manager"
	case strings.Contains(r, "admin") && !strings.Contains(r, "account"):
		return "/admin"
	case r == "manager" || r == "api-manager" || r == "api_manager":
		return "/manager"
	default:
		return "/"
	}
}

func sanitizeUsername(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	builder := strings.Builder{}
	builder.Grow(len(value))
	for _, ch := range value {
		switch {
		case ch >= 'a' && ch <= 'z':
			builder.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			builder.WriteRune(ch)
		case ch == '.' || ch == '-' || ch == '_' || ch == '@':
			builder.WriteRune(ch)
		case ch == ' ':
			builder.WriteRune('.')
		}
	}
	clean := strings.Trim(builder.String(), ".")
	return clean
}

func deriveOAuthRole(username, email, displayName string) string {
	candidates := []string{username, email, displayName}
	for _, candidate := range candidates {
		value := strings.ToLower(strings.TrimSpace(candidate))
		if value == "" {
			continue
		}
		local := value
		if at := strings.Index(local, "@"); at > 0 {
			local = local[:at]
		}
		compact := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(local, "-", ""), "_", ""), " ", "")
		switch compact {
		case "sysadmin", "systemadmin", "systemadministrator", "systemmanager":
			return "system-manager"
		case "admin", "administrator", "superadmin":
			return "admin"
		case "manager", "apimanager", "mgr":
			return "manager"
		}
	}
	return "user"
}

func randomURLSafeString(length int) (string, error) {
	if length < 32 {
		length = 32
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	value := base64.RawURLEncoding.EncodeToString(buf)
	if len(value) >= length {
		return value[:length], nil
	}
	for len(value) < length {
		value += "A"
	}
	return value, nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func signOAuthPayload(raw []byte) string {
	mac := hmac.New(sha256.New, []byte(oauthStateSecret()))
	_, _ = mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func encodeOAuthStateCookie(payload oauthStatePayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	signature := signOAuthPayload(raw)
	return encoded + "." + signature, nil
}

func decodeOAuthStateCookie(value string) (*oauthStatePayload, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid oauth state cookie format")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode oauth state payload: %w", err)
	}
	providedSig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode oauth state signature: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(oauthStateSecret()))
	_, _ = mac.Write(raw)
	expectedSig := mac.Sum(nil)
	if !hmac.Equal(providedSig, expectedSig) {
		return nil, errors.New("oauth state signature mismatch")
	}

	var payload oauthStatePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse oauth state payload: %w", err)
	}
	if payload.State == "" {
		return nil, errors.New("oauth state payload missing state")
	}
	return &payload, nil
}

func clearOAuthStateCookie(c *gin.Context) {
	c.SetCookie(oauthStateCookie, "", -1, "/", "", isHTTPSRequest(c), true)
}

func redirectOAuthError(c *gin.Context, frontendOrigin, message string) {
	cleanOrigin := normalizeOrigin(frontendOrigin)
	if cleanOrigin == "" {
		cleanOrigin = normalizeOrigin(requestBaseURL(c))
	}
	if cleanOrigin == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": message})
		return
	}
	qs := url.Values{}
	qs.Set("oauth_error", message)
	target := strings.TrimRight(cleanOrigin, "/") + "/login?" + qs.Encode()
	c.Redirect(http.StatusFound, target)
}

func redirectOAuthSuccess(c *gin.Context, frontendOrigin, returnPath, accessToken, refreshToken, username, role string) {
	cleanOrigin := normalizeOrigin(frontendOrigin)
	if cleanOrigin == "" {
		cleanOrigin = normalizeOrigin(requestBaseURL(c))
	}
	if cleanOrigin == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unable to resolve frontend origin"})
		return
	}

	values := url.Values{}
	values.Set("oauth_access_token", accessToken)
	values.Set("oauth_username", username)
	values.Set("oauth_role", role)
	values.Set("oauth_return_to", sanitizeReturnPath(returnPath))
	if refreshToken != "" {
		values.Set("oauth_refresh_token", refreshToken)
	}

	target := strings.TrimRight(cleanOrigin, "/") + "/login#" + values.Encode()
	c.Redirect(http.StatusFound, target)
}

func defaultIdentityProviderAuthorizationURL(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "google":
		return "https://accounts.google.com/o/oauth2/v2/auth"
	case "github":
		return "https://github.com/login/oauth/authorize"
	case "gitlab":
		return "https://gitlab.com/oauth/authorize"
	case "microsoft":
		return "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
	case "facebook":
		return "https://www.facebook.com/v19.0/dialog/oauth"
	default:
		return ""
	}
}

func defaultIdentityProviderTokenURL(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "google":
		return "https://oauth2.googleapis.com/token"
	case "github":
		return "https://github.com/login/oauth/access_token"
	case "gitlab":
		return "https://gitlab.com/oauth/token"
	case "microsoft":
		return "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	case "facebook":
		return "https://graph.facebook.com/v19.0/oauth/access_token"
	default:
		return ""
	}
}

func defaultIdentityProviderUserInfoURL(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "google":
		return "https://openidconnect.googleapis.com/v1/userinfo"
	case "github":
		return "https://api.github.com/user"
	case "gitlab":
		return "https://gitlab.com/api/v4/user"
	case "microsoft":
		return "https://graph.microsoft.com/oidc/userinfo"
	case "facebook":
		return "https://graph.facebook.com/me?fields=id,name,email"
	default:
		return ""
	}
}

func defaultIdentityProviderScopes(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "github":
		return "read:user user:email"
	case "gitlab":
		return "read_user"
	case "microsoft":
		return "openid profile email User.Read"
	case "facebook":
		return "email public_profile"
	default:
		return "openid profile email"
	}
}

func (h *AuthHandler) resolveIdentityProvider(providerKey string) (*iammodels.IdentityProvider, error) {
	if h.idpStore == nil {
		return nil, errors.New("identity provider store is not configured")
	}
	key := strings.ToLower(strings.TrimSpace(providerKey))
	if key == "" {
		return nil, errors.New("provider identifier is required")
	}

	idps, err := h.idpStore.ListIdentityProviders("")
	if err != nil {
		return nil, fmt.Errorf("list identity providers: %w", err)
	}

	var fallback *iammodels.IdentityProvider
	for i := range idps {
		idp := idps[i]
		if !idp.Enabled {
			continue
		}

		if strings.EqualFold(strings.TrimSpace(idp.ID), key) || strings.EqualFold(strings.TrimSpace(idp.Alias), key) {
			copyIDP := idp
			return &copyIDP, nil
		}

		if strings.EqualFold(strings.TrimSpace(idp.ProviderType), key) && fallback == nil {
			copyIDP := idp
			fallback = &copyIDP
		}
	}

	if fallback != nil {
		return fallback, nil
	}

	return nil, fmt.Errorf("identity provider %q is not configured or enabled", providerKey)
}

func buildOAuthAuthorizationURL(idp *iammodels.IdentityProvider, callbackURL, state, codeChallenge string) (string, error) {
	if idp == nil {
		return "", errors.New("identity provider is required")
	}
	providerType := strings.ToLower(strings.TrimSpace(idp.ProviderType))
	authorizeURL := strings.TrimSpace(idp.AuthorizationURL)
	if authorizeURL == "" {
		authorizeURL = defaultIdentityProviderAuthorizationURL(providerType)
	}
	if authorizeURL == "" {
		return "", fmt.Errorf("identity provider %q requires authorization_url", idp.Alias)
	}

	parsed, err := url.Parse(authorizeURL)
	if err != nil {
		return "", fmt.Errorf("invalid authorization_url: %w", err)
	}

	q := parsed.Query()
	if q.Get("response_type") == "" {
		q.Set("response_type", "code")
	}
	if q.Get("client_id") == "" {
		q.Set("client_id", strings.TrimSpace(idp.ClientID))
	}
	if strings.TrimSpace(q.Get("client_id")) == "" {
		return "", fmt.Errorf("identity provider %q requires client_id", idp.Alias)
	}
	if q.Get("redirect_uri") == "" {
		q.Set("redirect_uri", callbackURL)
	}
	if q.Get("scope") == "" {
		scopes := strings.TrimSpace(idp.DefaultScopes)
		if scopes == "" {
			scopes = defaultIdentityProviderScopes(providerType)
		}
		if scopes != "" {
			q.Set("scope", scopes)
		}
	}
	q.Set("state", state)
	if codeChallenge != "" {
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
	}

	if providerType == "google" {
		if q.Get("access_type") == "" {
			q.Set("access_type", "offline")
		}
		if q.Get("prompt") == "" {
			q.Set("prompt", "select_account")
		}
	}

	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

func parseOAuthJWTClaims(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, errors.New("invalid JWT payload")
	}
	payload := parts[1]
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, err
	}
	claims := map[string]interface{}{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func stringValueFromMap(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case fmt.Stringer:
			value := strings.TrimSpace(v.String())
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func (h *AuthHandler) exchangeOAuthAuthorizationCode(c *gin.Context, idp *iammodels.IdentityProvider, code, codeVerifier string) (*oauthProviderTokenResponse, error) {
	tokenURL := strings.TrimSpace(idp.TokenURL)
	if tokenURL == "" {
		tokenURL = defaultIdentityProviderTokenURL(idp.ProviderType)
	}
	if tokenURL == "" {
		return nil, fmt.Errorf("identity provider %q requires token_url", idp.Alias)
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", strings.TrimSpace(code))
	form.Set("redirect_uri", oauthCallbackURL(c))
	if strings.TrimSpace(idp.ClientID) != "" {
		form.Set("client_id", strings.TrimSpace(idp.ClientID))
	}
	if strings.TrimSpace(idp.ClientSecret) != "" {
		form.Set("client_secret", strings.TrimSpace(idp.ClientSecret))
	}
	if strings.TrimSpace(codeVerifier) != "" {
		form.Set("code_verifier", strings.TrimSpace(codeVerifier))
	}

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("invalid token response from provider: %s", summarizeIAMBody(body))
	}

	var tokenResp oauthProviderTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errText := strings.TrimSpace(tokenResp.ErrorDescription)
		if errText == "" {
			errText = strings.TrimSpace(tokenResp.Error)
		}
		if errText == "" {
			errText = summarizeIAMBody(body)
		}
		return nil, fmt.Errorf("provider token exchange failed: %s", errText)
	}

	if strings.TrimSpace(tokenResp.Error) != "" {
		errText := strings.TrimSpace(tokenResp.ErrorDescription)
		if errText == "" {
			errText = strings.TrimSpace(tokenResp.Error)
		}
		return nil, fmt.Errorf("provider token exchange failed: %s", errText)
	}

	if strings.TrimSpace(tokenResp.AccessToken) == "" && strings.TrimSpace(tokenResp.IDToken) == "" {
		return nil, errors.New("provider token exchange returned no access_token or id_token")
	}

	if strings.TrimSpace(tokenResp.TokenType) == "" {
		tokenResp.TokenType = "Bearer"
	}

	return &tokenResp, nil
}

func (h *AuthHandler) fetchOAuthIdentityProfile(idp *iammodels.IdentityProvider, tokenResp *oauthProviderTokenResponse) (*oauthIdentityProfile, error) {
	profile := &oauthIdentityProfile{}

	if idToken := strings.TrimSpace(tokenResp.IDToken); idToken != "" {
		if claims, err := parseOAuthJWTClaims(idToken); err == nil {
			profile.Subject = stringValueFromMap(claims, "sub", "id")
			profile.Email = stringValueFromMap(claims, "email", "upn")
			profile.Name = stringValueFromMap(claims, "name")
			profile.DisplayName = stringValueFromMap(claims, "display_name", "name")
			profile.PreferredUsername = stringValueFromMap(claims, "preferred_username", "nickname")
		}
	}

	needUserInfo := strings.TrimSpace(profile.Email) == "" || strings.TrimSpace(profile.PreferredUsername) == "" || strings.TrimSpace(profile.DisplayName) == ""
	if needUserInfo && strings.TrimSpace(tokenResp.AccessToken) != "" {
		userInfoURL := strings.TrimSpace(idp.UserInfoURL)
		if userInfoURL == "" {
			userInfoURL = defaultIdentityProviderUserInfoURL(idp.ProviderType)
		}

		if userInfoURL != "" {
			req, err := http.NewRequest(http.MethodGet, userInfoURL, nil)
			if err == nil {
				req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(tokenResp.AccessToken))
				req.Header.Set("Accept", "application/json")

				resp, callErr := h.httpClient.Do(req)
				if callErr == nil {
					defer resp.Body.Close()
					if body, readErr := io.ReadAll(resp.Body); readErr == nil && json.Valid(body) {
						data := map[string]interface{}{}
						if unmarshalErr := json.Unmarshal(body, &data); unmarshalErr == nil {
							if profile.Subject == "" {
								profile.Subject = stringValueFromMap(data, "sub", "id")
							}
							if profile.Email == "" {
								profile.Email = stringValueFromMap(data, "email", "upn")
							}
							if profile.Name == "" {
								profile.Name = stringValueFromMap(data, "name")
							}
							if profile.DisplayName == "" {
								profile.DisplayName = stringValueFromMap(data, "display_name", "name")
							}
							if profile.PreferredUsername == "" {
								profile.PreferredUsername = stringValueFromMap(data, "preferred_username", "username", "login", "nickname")
							}
						}
					}
				}
			}
		}
	}

	if profile.DisplayName == "" {
		switch {
		case profile.Name != "":
			profile.DisplayName = profile.Name
		case profile.PreferredUsername != "":
			profile.DisplayName = profile.PreferredUsername
		case profile.Email != "":
			profile.DisplayName = profile.Email
		}
	}

	if profile.PreferredUsername == "" {
		if profile.Email != "" {
			profile.PreferredUsername = profile.Email
		} else {
			profile.PreferredUsername = profile.DisplayName
		}
	}

	if profile.Subject == "" {
		profile.Subject = sanitizeUsername(profile.PreferredUsername)
	}

	if strings.TrimSpace(profile.PreferredUsername) == "" && strings.TrimSpace(profile.Email) == "" && strings.TrimSpace(profile.Subject) == "" {
		return nil, errors.New("unable to determine identity from OAuth provider response")
	}

	return profile, nil
}

func resolveFederatedEmail(username, email string) string {
	resolved := strings.TrimSpace(email)
	if resolved == "" {
		if strings.Contains(strings.TrimSpace(username), "@") {
			resolved = strings.TrimSpace(username)
		} else {
			resolved = strings.TrimSpace(username) + "@federated.local"
		}
	}
	return iamidentity.NormaliseEmail(resolved)
}

func (h *AuthHandler) ensureIAMFederatedUser(username, email, displayName string) error {
	if h.iamUsers == nil {
		return nil
	}

	resolvedEmail := resolveFederatedEmail(username, email)
	if resolvedEmail == "" {
		return errors.New("unable to resolve IAM email for federated user")
	}

	existing, err := h.iamUsers.GetByEmail(resolvedEmail)
	if err != nil {
		return fmt.Errorf("lookup IAM user by email: %w", err)
	}

	if existing != nil {
		changed := false
		if strings.TrimSpace(existing.DisplayName) == "" && strings.TrimSpace(displayName) != "" {
			existing.DisplayName = strings.TrimSpace(displayName)
			changed = true
		}
		if !existing.EmailVerified {
			existing.EmailVerified = true
			changed = true
		}
		if changed {
			existing.UpdatedAt = time.Now().UTC()
			if updateErr := h.iamUsers.Update(existing); updateErr != nil {
				return fmt.Errorf("update IAM federated user: %w", updateErr)
			}
		}
		return nil
	}

	randomPass, err := randomURLSafeString(40)
	if err != nil {
		return fmt.Errorf("generate federated password seed: %w", err)
	}
	passwordHash, err := iamidentity.HashPassword(randomPass)
	if err != nil {
		return fmt.Errorf("hash federated password: %w", err)
	}

	now := time.Now().UTC()
	user := &iamidentity.User{
		ID:            iamidentity.NewUserID(),
		Email:         resolvedEmail,
		PasswordHash:  passwordHash,
		DisplayName:   strings.TrimSpace(displayName),
		Active:        true,
		EmailVerified: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := h.iamUsers.Create(user); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil
		}
		return fmt.Errorf("create IAM federated user: %w", err)
	}

	return nil
}

func (h *AuthHandler) resolveIAMFederatedRole(username, email string) string {
	if h.iamUsers == nil || h.iamAuthorizer == nil {
		return ""
	}

	resolvedEmail := resolveFederatedEmail(username, email)
	if resolvedEmail == "" {
		return ""
	}

	user, err := h.iamUsers.GetByEmail(resolvedEmail)
	if err != nil || user == nil {
		return ""
	}

	roleNames, err := h.iamAuthorizer.GetUserRoleNames(user.ID)
	if err != nil || len(roleNames) == 0 {
		return ""
	}

	resolved := strings.ToLower(strings.TrimSpace(resolvePrimaryRole(roleNames)))
	return resolved
}

// AuthProxyLoginRequest is the request payload for the auth proxy login endpoint.
// This is distinct from the LoginRequest in authn.go which uses email-based auth.
type AuthProxyLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// demoAccounts maps username -> {password, role} for local dev fallback
var demoAccounts = map[string]struct {
	password string
	role     string
}{
	"admin":    {password: "admin", role: "admin"},
	"sysadmin": {password: "sysadmin", role: "system-manager"},
	"manager":  {password: "manager", role: "manager"},
	"user":     {password: "user", role: "user"},
}

// generateDemoToken creates an HMAC-HS256 JWT for a demo account
func generateDemoToken(username, role string) (string, error) {
	now := time.Now()
	claims := gojwt.MapClaims{
		"preferred_username": username,
		"email":              username + "@demo.local",
		"realm_access": map[string]interface{}{
			"roles": []string{role, "uma_authorization"},
		},
		"demo": true,
		"iat":  now.Unix(),
		"exp":  now.Add(8 * time.Hour).Unix(),
	}
	token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(auth.DemoJWTSecret()))
}

// generateFederatedToken creates an HMAC-HS256 JWT for OAuth-based sessions.
func generateFederatedToken(username, email, role string) (string, error) {
	now := time.Now()
	resolvedEmail := strings.TrimSpace(email)
	if resolvedEmail == "" {
		resolvedEmail = strings.TrimSpace(username) + "@federated.local"
	}
	claims := gojwt.MapClaims{
		"preferred_username": strings.TrimSpace(username),
		"email":              resolvedEmail,
		"realm_access": map[string]interface{}{
			"roles": []string{role, "uma_authorization"},
		},
		"federated": true,
		"iat":       now.Unix(),
		"exp":       now.Add(8 * time.Hour).Unix(),
	}
	token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(auth.DemoJWTSecret()))
}

// extractRoleFromToken decodes the JWT payload and determines the user role
func extractRoleFromToken(tokenString string) string {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return "user"
	}
	payload := parts[1]
	// Add padding
	padded := payload + strings.Repeat("=", (4-len(payload)%4)%4)
	padded = strings.ReplaceAll(padded, "-", "+")
	padded = strings.ReplaceAll(padded, "_", "/")
	decoded, err := base64.StdEncoding.DecodeString(padded)
	if err != nil {
		return "user"
	}
	var payload2 struct {
		Roles       []string `json:"roles"`
		RealmAccess struct {
			Roles []string `json:"roles"`
		} `json:"realm_access"`
		ResourceAccess map[string]struct {
			Roles []string `json:"roles"`
		} `json:"resource_access"`
	}
	if err := json.Unmarshal(decoded, &payload2); err != nil {
		return "user"
	}

	allRoles := make([]string, 0, len(payload2.Roles)+len(payload2.RealmAccess.Roles)+8)
	allRoles = append(allRoles, payload2.Roles...)
	allRoles = append(allRoles, payload2.RealmAccess.Roles...)
	for _, access := range payload2.ResourceAccess {
		allRoles = append(allRoles, access.Roles...)
	}

	for _, r := range allRoles {
		rl := strings.ToLower(strings.TrimSpace(r))
		if rl == "system-manager" || rl == "system_manager" || rl == "system-admin" || rl == "sysadmin" {
			return "system-manager"
		}
	}
	for _, r := range allRoles {
		rl := strings.ToLower(strings.TrimSpace(r))
		if strings.Contains(rl, "admin") && !strings.Contains(rl, "account") {
			return "admin"
		}
	}
	for _, r := range allRoles {
		rl := strings.ToLower(strings.TrimSpace(r))
		if rl == "manager" || rl == "api-manager" || rl == "api_manager" {
			return "manager"
		}
	}
	return "user"
}

func resolvePrimaryRole(roles []string) string {
	resolved := "user"
	for _, role := range roles {
		r := strings.ToLower(strings.TrimSpace(role))
		switch {
		case r == "system-manager" || r == "system_manager" || r == "system-admin" || r == "sysadmin":
			return "system-manager"
		case strings.Contains(r, "admin") && !strings.Contains(r, "account"):
			resolved = "admin"
		case (r == "manager" || r == "api-manager" || r == "api_manager") && resolved == "user":
			resolved = "manager"
		}
	}
	return resolved
}

// TokenResponse is the response payload used by IAM token endpoints.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
	User         struct {
		ID          string   `json:"id,omitempty"`
		Email       string   `json:"email,omitempty"`
		DisplayName string   `json:"display_name,omitempty"`
		Roles       []string `json:"roles,omitempty"`
	} `json:"user,omitempty"`
}

// OAuthStart handles GET /auth/oauth/start and redirects to the selected identity provider.
func (h *AuthHandler) OAuthStart(c *gin.Context) {
	frontendOrigin := resolveFrontendOrigin(c, c.Query("frontend_origin"))
	providerKey := strings.TrimSpace(c.Query("provider"))
	returnTo := sanitizeReturnPath(c.Query("return_to"))

	if providerKey == "" {
		redirectOAuthError(c, frontendOrigin, "Missing provider identifier")
		return
	}

	idp, err := h.resolveIdentityProvider(providerKey)
	if err != nil {
		redirectOAuthError(c, frontendOrigin, err.Error())
		return
	}

	stateValue, err := randomURLSafeString(48)
	if err != nil {
		redirectOAuthError(c, frontendOrigin, "Failed to initialize OAuth state")
		return
	}

	codeVerifier, err := randomURLSafeString(96)
	if err != nil {
		redirectOAuthError(c, frontendOrigin, "Failed to initialize PKCE verifier")
		return
	}

	payload := oauthStatePayload{
		State:          stateValue,
		ProviderKey:    strings.TrimSpace(idp.ID),
		ReturnTo:       returnTo,
		FrontendOrigin: frontendOrigin,
		CodeVerifier:   codeVerifier,
		IssuedAt:       time.Now().UTC().Unix(),
	}

	encodedState, err := encodeOAuthStateCookie(payload)
	if err != nil {
		redirectOAuthError(c, frontendOrigin, "Failed to encode OAuth state")
		return
	}

	c.SetCookie(oauthStateCookie, encodedState, oauthStateMaxAge, "/", "", isHTTPSRequest(c), true)

	authorizeURL, err := buildOAuthAuthorizationURL(idp, oauthCallbackURL(c), stateValue, pkceChallenge(codeVerifier))
	if err != nil {
		clearOAuthStateCookie(c)
		redirectOAuthError(c, frontendOrigin, err.Error())
		return
	}

	c.Redirect(http.StatusFound, authorizeURL)
}

// OAuthCallback handles GET /auth/oauth/callback and finalizes a platform session.
func (h *AuthHandler) OAuthCallback(c *gin.Context) {
	frontendOrigin := resolveFrontendOrigin(c, "")

	rawStateCookie, cookieErr := c.Cookie(oauthStateCookie)
	if cookieErr != nil {
		redirectOAuthError(c, frontendOrigin, "OAuth session state is missing. Please retry login.")
		return
	}

	statePayload, err := decodeOAuthStateCookie(rawStateCookie)
	if err != nil {
		clearOAuthStateCookie(c)
		redirectOAuthError(c, frontendOrigin, "OAuth session state is invalid. Please retry login.")
		return
	}

	if statePayload.FrontendOrigin != "" {
		frontendOrigin = statePayload.FrontendOrigin
	}

	if oauthErr := strings.TrimSpace(c.Query("error")); oauthErr != "" {
		detail := strings.TrimSpace(c.Query("error_description"))
		clearOAuthStateCookie(c)
		if detail != "" {
			redirectOAuthError(c, frontendOrigin, oauthErr+": "+detail)
		} else {
			redirectOAuthError(c, frontendOrigin, oauthErr)
		}
		return
	}

	if statePayload.IssuedAt <= 0 || time.Since(time.Unix(statePayload.IssuedAt, 0).UTC()) > oauthStateLifetime {
		clearOAuthStateCookie(c)
		redirectOAuthError(c, frontendOrigin, "OAuth session expired. Please retry login.")
		return
	}

	requestState := strings.TrimSpace(c.Query("state"))
	if requestState == "" || requestState != statePayload.State {
		clearOAuthStateCookie(c)
		redirectOAuthError(c, frontendOrigin, "OAuth state mismatch. Please retry login.")
		return
	}

	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		clearOAuthStateCookie(c)
		redirectOAuthError(c, frontendOrigin, "OAuth provider did not return an authorization code")
		return
	}

	idp, err := h.resolveIdentityProvider(statePayload.ProviderKey)
	if err != nil {
		clearOAuthStateCookie(c)
		redirectOAuthError(c, frontendOrigin, err.Error())
		return
	}

	tokenResp, err := h.exchangeOAuthAuthorizationCode(c, idp, code, statePayload.CodeVerifier)
	if err != nil {
		clearOAuthStateCookie(c)
		redirectOAuthError(c, frontendOrigin, err.Error())
		return
	}

	identity, err := h.fetchOAuthIdentityProfile(idp, tokenResp)
	if err != nil {
		clearOAuthStateCookie(c)
		redirectOAuthError(c, frontendOrigin, err.Error())
		return
	}

	username := sanitizeUsername(identity.PreferredUsername)
	if username == "" {
		username = sanitizeUsername(identity.Email)
	}
	if username == "" {
		username = sanitizeUsername(identity.DisplayName)
	}
	if username == "" {
		username = "oauth-user"
	}

	if err := h.ensureIAMFederatedUser(username, identity.Email, identity.DisplayName); err != nil {
		clearOAuthStateCookie(c)
		redirectOAuthError(c, frontendOrigin, "Failed to register IAM user profile: "+err.Error())
		return
	}

	role := strings.ToLower(strings.TrimSpace(h.resolveIAMFederatedRole(username, identity.Email)))
	if h.platformUsers != nil {
		persistedUser, persistErr := h.platformUsers.EnsureFederatedUser(username, identity.Email, "user")
		if persistErr != nil {
			clearOAuthStateCookie(c)
			redirectOAuthError(c, frontendOrigin, "Failed to register platform user profile: "+persistErr.Error())
			return
		} else if persistedUser != nil {
			if strings.ToLower(strings.TrimSpace(persistedUser.Status)) != "active" {
				clearOAuthStateCookie(c)
				redirectOAuthError(c, frontendOrigin, "Account is disabled. Please contact administrator.")
				return
			}
			persistedRole := strings.ToLower(strings.TrimSpace(persistedUser.Role))
			if (role == "" || role == "user") && persistedRole != "" {
				role = persistedRole
			}
		}
	}

	if role == "" || role == "user" {
		role = deriveOAuthRole(username, identity.Email, identity.DisplayName)
	}
	if role != "system-manager" && role != "admin" && role != "manager" && role != "user" {
		role = "user"
	}

	platformToken, err := generateFederatedToken(username, identity.Email, role)
	if err != nil {
		clearOAuthStateCookie(c)
		redirectOAuthError(c, frontendOrigin, "Failed to create platform session token")
		return
	}

	if h.rateLimiter != nil {
		h.rateLimiter.RegisterToken(platformToken, username)
	}

	returnTo := sanitizeReturnPath(statePayload.ReturnTo)
	if returnTo == "/login" {
		returnTo = defaultPathForRole(role)
	}

	clearOAuthStateCookie(c)
	redirectOAuthSuccess(c, frontendOrigin, returnTo, platformToken, "", username, role)
}

// Login handles POST /auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req AuthProxyLoginRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request: " + err.Error(),
		})
		return
	}

	iamOnlyAuth := strings.EqualFold(getEnv("IAM_ONLY_AUTH", "true"), "true")

	if !iamOnlyAuth {
		// Optional local demo login path for development.
		demoEnabled := getEnv("ENABLE_DEMO_ACCOUNTS", "false") == "true"
		if demo, ok := demoAccounts[req.Username]; ok && demo.password == req.Password {
			if demoEnabled {
				demoToken, err := generateDemoToken(req.Username, demo.role)
				if err == nil {
					if h.rateLimiter != nil {
						h.rateLimiter.RegisterToken(demoToken, req.Username)
					}
					c.JSON(http.StatusOK, gin.H{
						"status":        "ok",
						"access_token":  demoToken,
						"expires_in":    28800,
						"refresh_token": "",
						"token_type":    "Bearer",
						"username":      req.Username,
						"role":          demo.role,
						"demo_mode":     true,
					})
					return
				}
			}
		}

		// Optional platform user fallback.
		if h.platformUsers != nil {
			if platformUser, ok := h.platformUsers.ValidateCredentials(req.Username, req.Password); ok {
				platformToken, err := generateDemoToken(platformUser.Username, platformUser.Role)
				if err == nil {
					if h.rateLimiter != nil {
						h.rateLimiter.RegisterToken(platformToken, platformUser.Username)
					}
					c.JSON(http.StatusOK, gin.H{
						"status":        "ok",
						"access_token":  platformToken,
						"expires_in":    28800,
						"refresh_token": "",
						"token_type":    "Bearer",
						"username":      platformUser.Username,
						"role":          platformUser.Role,
						"demo_mode":     true,
					})
					return
				}
			}
		}
	}

	loginID := resolveIAMLoginIdentifier(req.Username)
	if loginID == "" {
		c.JSON(http.StatusBadRequest, models.Response{Status: "error", Error: "username is required"})
		return
	}

	body, _ := json.Marshal(map[string]string{
		"email":    loginID,
		"password": req.Password,
	})

	postLogin := func(targetURL string) (*http.Response, []byte, error) {
		resp, err := h.httpClient.Post(targetURL, "application/json", strings.NewReader(string(body)))
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()

		responseBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return resp, nil, readErr
		}
		return resp, responseBody, nil
	}

	var (
		resp         *http.Response
		responseBody []byte
		err          error
	)

	loginBases := iamLoginBaseCandidates(h.iamBaseURL)
	if len(loginBases) == 0 {
		loginBases = []string{normalizeIAMBaseURL(defaultIAMInternalBaseURL())}
	}

	attemptErrors := make([]string, 0, len(loginBases))
	for idx, base := range loginBases {
		targetURL := base + "/iam/auth/login"
		if idx == 0 {
			logging.Z().Debug("IAM login attempt", zap.String("identifier", loginID), zap.String("base", base))
		} else {
			logging.Z().Debug("IAM login retrying with alternate base", zap.String("base", base))
		}

		attemptResp, attemptBody, attemptErr := postLogin(targetURL)
		resp = attemptResp
		responseBody = attemptBody
		err = attemptErr

		if !shouldRetryIAMLogin(resp, responseBody, err) {
			break
		}

		if attemptErr != nil {
			attemptErrors = append(attemptErrors, fmt.Sprintf("Post %q: %v", targetURL, attemptErr))
			continue
		}

		statusCode := 0
		contentType := ""
		if attemptResp != nil {
			statusCode = attemptResp.StatusCode
			contentType = strings.TrimSpace(attemptResp.Header.Get(headerContentType))
		}
		attemptErrors = append(attemptErrors, fmt.Sprintf("%s returned status=%d content-type=%q body=%q", targetURL, statusCode, contentType, summarizeIAMBody(attemptBody)))
	}

	if shouldRetryIAMLogin(resp, responseBody, err) && len(attemptErrors) > 0 {
		err = errors.New(strings.Join(attemptErrors, "; "))
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to connect to IAM authentication service: " + err.Error(),
		})
		return
	}

	contentType := ""
	if resp != nil {
		contentType = strings.TrimSpace(resp.Header.Get(headerContentType))
	}

	if !json.Valid(responseBody) {
		summary := summarizeIAMBody(responseBody)
		logging.Z().Warn("IAM login non-JSON response", zap.Int("status", resp.StatusCode), zap.String("contentType", contentType), zap.String("body", summary))
		c.JSON(http.StatusBadGateway, models.Response{
			Status: "error",
			Error:  "authentication service temporarily unavailable",
		})
		return
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(responseBody, &tokenResp); err != nil {
		summary := summarizeIAMBody(responseBody)
		logging.Z().Warn("IAM login malformed JSON payload", zap.Int("status", resp.StatusCode), zap.String("contentType", contentType), zap.String("body", summary), zap.Error(err))
		c.JSON(http.StatusBadGateway, models.Response{
			Status: "error",
			Error:  "authentication service returned an invalid response",
		})
		return
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := strings.TrimSpace(tokenResp.Error)
		if errMsg == "" {
			errMsg = strings.TrimSpace(tokenResp.ErrorDesc)
		}
		if errMsg == "" {
			errMsg = summarizeIAMBody(responseBody)
		}
		if errMsg == "" {
			errMsg = "authentication failed"
		}

		statusCode := http.StatusUnauthorized
		switch {
		case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			statusCode = resp.StatusCode
		case resp.StatusCode == http.StatusTooManyRequests:
			statusCode = http.StatusTooManyRequests
		case resp.StatusCode >= 500:
			statusCode = http.StatusBadGateway
		}

		c.JSON(statusCode, models.Response{
			Status: "error",
			Error:  errMsg,
		})
		return
	}

	resolvedRole := resolvePrimaryRole(tokenResp.User.Roles)
	if resolvedRole == "user" {
		resolvedRole = extractRoleFromToken(tokenResp.AccessToken)
	}

	resolvedUsername := strings.TrimSpace(tokenResp.User.DisplayName)
	if resolvedUsername == "" {
		resolvedUsername = strings.TrimSpace(tokenResp.User.Email)
	}
	if resolvedUsername == "" {
		resolvedUsername = req.Username
	}

	if h.rateLimiter != nil {
		h.rateLimiter.RegisterToken(tokenResp.AccessToken, resolvedUsername)
		logging.Z().Info("token registered in rate limiter", zap.String("user", resolvedUsername))
	}

	c.JSON(http.StatusOK, gin.H{
		"role":          resolvedRole,
		"status":        "ok",
		"access_token":  tokenResp.AccessToken,
		"expires_in":    tokenResp.ExpiresIn,
		"refresh_token": tokenResp.RefreshToken,
		"token_type":    tokenResp.TokenType,
		"username":      resolvedUsername,
		"user":          tokenResp.User,
		"rate_limit": gin.H{
			"max_calls":    500,
			"validity_min": tokenResp.ExpiresIn / 60,
			"expires_at":   time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format("2006-01-02 15:04:05"),
			"message":      fmt.Sprintf("You have 500 API calls available with this token. Token expires in %d minutes.", tokenResp.ExpiresIn/60),
		},
	})
}

// RefreshToken handles POST /auth/refresh
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request: " + err.Error(),
		})
		return
	}

	// Try all IAM base candidates (same resilience as login).
	loginBases := iamLoginBaseCandidates(h.iamBaseURL)
	if len(loginBases) == 0 {
		loginBases = []string{normalizeIAMBaseURL(defaultIAMInternalBaseURL())}
	}

	body, _ := json.Marshal(map[string]string{
		"refresh_token": req.RefreshToken,
	})

	var (
		resp         *http.Response
		responseBody []byte
		err          error
	)

	for _, base := range loginBases {
		tokenURL := base + "/iam/auth/refresh"
		resp, err = h.httpClient.Post(
			tokenURL,
			"application/json",
			strings.NewReader(string(body)),
		)
		if err != nil {
			continue
		}
		responseBody, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		if !shouldRetryIAMLogin(resp, responseBody, nil) {
			break
		}
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to connect to IAM authentication service: " + err.Error(),
		})
		return
	}

	contentType := strings.TrimSpace(resp.Header.Get(headerContentType))
	if !json.Valid(responseBody) {
		summary := summarizeIAMBody(responseBody)
		logging.Z().Warn("IAM refresh non-JSON response", zap.Int("status", resp.StatusCode), zap.String("contentType", contentType), zap.String("body", summary))
		c.JSON(http.StatusBadGateway, models.Response{
			Status: "error",
			Error:  "token refresh service temporarily unavailable",
		})
		return
	}

	// Parse token response
	var tokenResp TokenResponse
	if err := json.Unmarshal(responseBody, &tokenResp); err != nil {
		summary := summarizeIAMBody(responseBody)
		logging.Z().Warn("IAM refresh malformed JSON payload", zap.Int("status", resp.StatusCode), zap.String("contentType", contentType), zap.String("body", summary), zap.Error(err))
		c.JSON(http.StatusBadGateway, models.Response{
			Status: "error",
			Error:  "token refresh service returned an invalid response",
		})
		return
	}

	// Check if IAM returned an error
	if tokenResp.Error != "" {
		errText := tokenResp.ErrorDesc
		if strings.TrimSpace(errText) == "" {
			errText = tokenResp.Error
		}
		c.JSON(http.StatusUnauthorized, models.Response{
			Status: "error",
			Error:  "Token refresh failed: " + errText,
		})
		return
	}

	// Check response status code
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusUnauthorized, models.Response{
			Status: "error",
			Error:  "Token refresh failed with status: " + resp.Status,
		})
		return
	}

	// Resolve user identity from the new access token so the frontend
	// can keep its session state consistent.
	resolvedRole := resolvePrimaryRole(tokenResp.User.Roles)
	if resolvedRole == "user" {
		resolvedRole = extractRoleFromToken(tokenResp.AccessToken)
	}

	resolvedUsername := strings.TrimSpace(tokenResp.User.DisplayName)
	if resolvedUsername == "" {
		resolvedUsername = strings.TrimSpace(tokenResp.User.Email)
	}

	// Register the freshly issued access token in the rate limiter so that
	// subsequent API requests are accepted without the user having to re-login.
	if h.rateLimiter != nil && strings.TrimSpace(tokenResp.AccessToken) != "" {
		h.rateLimiter.RegisterToken(tokenResp.AccessToken, resolvedUsername)
		logging.Z().Info("refreshed token registered in rate limiter", zap.String("user", resolvedUsername))
	}

	// Success - return new token info with role/username for frontend.
	c.JSON(http.StatusOK, gin.H{
		"status":        "ok",
		"access_token":  tokenResp.AccessToken,
		"expires_in":    tokenResp.ExpiresIn,
		"refresh_token": tokenResp.RefreshToken,
		"token_type":    tokenResp.TokenType,
		"role":          resolvedRole,
		"username":      resolvedUsername,
	})
}

// Logout handles POST /auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, models.Response{
			Status: "error",
			Error:  "No token provided",
		})
		return
	}

	requestBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request body",
		})
		return
	}

	trimmedRequestBody := strings.TrimSpace(string(requestBody))
	if trimmedRequestBody != "" && !json.Valid([]byte(trimmedRequestBody)) {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid JSON body",
		})
		return
	}

	tokenURL := h.iamBaseURL + "/iam/auth/logout"
	bodyReader := io.Reader(http.NoBody)
	if trimmedRequestBody != "" {
		bodyReader = strings.NewReader(trimmedRequestBody)
	}

	req, err := http.NewRequest(http.MethodPost, tokenURL, bodyReader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to build logout request",
		})
		return
	}
	req.Header.Set("Authorization", authHeader)
	if trimmedRequestBody != "" {
		req.Header.Set(headerContentType, "application/json")
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to connect to IAM authentication service: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to read logout response: " + err.Error(),
		})
		return
	}

	trimmedResponseBody := strings.TrimSpace(string(responseBody))
	if trimmedResponseBody == "" {
		c.Status(resp.StatusCode)
		return
	}

	if !json.Valid([]byte(trimmedResponseBody)) {
		contentType := strings.TrimSpace(resp.Header.Get(headerContentType))
		summary := summarizeIAMBody(responseBody)
		logging.Z().Warn("IAM logout non-JSON response", zap.Int("status", resp.StatusCode), zap.String("contentType", contentType), zap.String("body", summary))
		c.JSON(http.StatusBadGateway, models.Response{
			Status: "error",
			Error:  "logout service temporarily unavailable",
		})
		return
	}

	contentType := strings.TrimSpace(resp.Header.Get(headerContentType))
	if contentType == "" {
		contentType = "application/json"
	}

	c.Data(resp.StatusCode, contentType, []byte(trimmedResponseBody))
}

// ValidateToken handles GET /auth/validate
func (h *AuthHandler) ValidateToken(c *gin.Context) {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, models.Response{
			Status: "error",
			Error:  "No token provided",
		})
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	req, err := http.NewRequest(http.MethodGet, h.iamBaseURL+"/iam/auth/whoami", nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{Status: "error", Error: "Failed to build validation request"})
		return
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))

	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, models.Response{Status: "error", Error: "IAM validation endpoint unreachable"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusUnauthorized, models.Response{Status: "error", Error: "Invalid token"})
		return
	}

	c.JSON(http.StatusOK, models.Response{Status: "ok", Message: "Token is valid"})
}

// GetTokenStatus handles GET /auth/token-status
func (h *AuthHandler) GetTokenStatus(c *gin.Context) {
	if h.rateLimiter == nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Rate limiter not initialized",
		})
		return
	}

	// Get token from header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, models.Response{
			Status: "error",
			Error:  "missing authorization header",
		})
		return
	}

	// Extract Bearer token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.Response{
			Status: "error",
			Error:  "invalid authorization header",
		})
		return
	}

	// Get token stats
	stats, err := h.rateLimiter.GetTokenStats(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "token not found or invalid",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"data":   stats,
	})
}

// GetAllTokensStatus handles GET /auth/admin/tokens-status (admin only)
func (h *AuthHandler) GetAllTokensStatus(c *gin.Context) {
	if h.rateLimiter == nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Rate limiter not initialized",
		})
		return
	}

	stats := h.rateLimiter.GetAllTokenStats()

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"data":   stats,
	})
}
