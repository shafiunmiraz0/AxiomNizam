package admin

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	iamconfig "axiomnizam.bitbd.net/axiomnizam/internal/iam/config"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/authn"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/authz"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/identity"
	iammw "axiomnizam.bitbd.net/axiomnizam/internal/iam/middleware"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/oauth"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/storage"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/token"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var (
	realmNamePattern = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)
	clientIDPattern  = regexp.MustCompile(`^[A-Za-z0-9._:-]{3,128}$`)
)

const (
	defaultClientRateLimitMaxCalls int64 = 500
	minClientRateLimitMaxCalls     int64 = 1
	maxClientRateLimitMaxCalls     int64 = 100000000

	minClientTokenValidityMinutes = 1
	maxClientTokenValidityMinutes = 10080 // 7 days
)

type userRepository interface {
	Create(user *identity.User) error
	GetByID(id string) (*identity.User, error)
	GetByEmail(email string) (*identity.User, error)
	Update(user *identity.User) error
	Delete(id string) error
	List() ([]*identity.User, error)
}

type sessionRepository interface {
	Create(s *authn.Session) error
	GetByID(id string) (*authn.Session, error)
	RevokeByUserID(userID string) error
	Revoke(sessionID string) error
}

type refreshTokenRepository interface {
	StoreRefreshToken(rt *oauth.RefreshTokenRecord) error
	GetRefreshToken(jti string) (*oauth.RefreshTokenRecord, error)
	RevokeRefreshToken(jti string) error
	RevokeAllForUser(userID string) error
	RevokeBySessionID(sessionID string) error
}

type authorizerService interface {
	GetUserRoleNames(userID string) ([]string, error)
	AssignRole(userID, roleID string) (*authz.RoleBinding, error)
	RevokeRole(bindingID string) error
}

type authenticatorService interface {
	Authenticate(identifier, password string) (*identity.User, error)
	CreateSession(userID, ip, userAgent string, ttl time.Duration) (*authn.Session, error)
}

// Handler bundles all sysadmin (master-realm) API endpoints.
type Handler struct {
	users        userRepository
	clients      *storage.EtcdClientRepository
	roles        *storage.EtcdRoleRepository
	bindings     *storage.EtcdRoleBindingRepository
	sessions     sessionRepository
	refreshRepo  refreshTokenRepository
	revokedStore *storage.EtcdRevokedTokenStore
	codeRepo     *storage.EtcdCodeRepository
	authorizer   authorizerService
	issuer       *token.Issuer
	authn        authenticatorService
}

// NewHandler creates the admin handler.
func NewHandler(
	users *storage.PostgresUserRepository,
	clients *storage.EtcdClientRepository,
	roles *storage.EtcdRoleRepository,
	bindings *storage.EtcdRoleBindingRepository,
	sessions *storage.EtcdSessionRepository,
	refreshRepo *storage.EtcdRefreshTokenRepository,
	revokedStore *storage.EtcdRevokedTokenStore,
	codeRepo *storage.EtcdCodeRepository,
	authorizer *authz.Authorizer,
	issuer *token.Issuer,
	authenticator *authn.Authenticator,
) *Handler {
	return &Handler{
		users:        users,
		clients:      clients,
		roles:        roles,
		bindings:     bindings,
		sessions:     sessions,
		refreshRepo:  refreshRepo,
		revokedStore: revokedStore,
		codeRepo:     codeRepo,
		authorizer:   authorizer,
		issuer:       issuer,
		authn:        authenticator,
	}
}

func normalizeRoleNames(roleNames []string) []string {
	seen := make(map[string]struct{}, len(roleNames))
	out := make([]string, 0, len(roleNames))

	for _, roleName := range roleNames {
		normalized := strings.ToLower(strings.TrimSpace(roleName))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	if len(out) == 0 {
		out = []string{"user"}
	}

	return out
}

func hasRoleName(roleNames []string, roleName string) bool {
	target := strings.ToLower(strings.TrimSpace(roleName))
	if target == "" {
		return false
	}

	for _, candidate := range roleNames {
		if strings.ToLower(strings.TrimSpace(candidate)) == target {
			return true
		}
	}

	return false
}

func isBootstrapSysadminEmail(email string) bool {
	normalizedEmail := identity.NormaliseEmail(email)
	if normalizedEmail == "" {
		return false
	}

	configured := identity.NormaliseEmail(strings.TrimSpace(os.Getenv("IAM_SYSADMIN_EMAIL")))
	if configured != "" {
		return normalizedEmail == configured
	}

	parts := strings.SplitN(normalizedEmail, "@", 2)
	return len(parts) > 0 && strings.EqualFold(parts[0], "sysadmin")
}

func (h *Handler) ensureUserHasSysadminRole(userID string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("user id is required")
	}

	sysRole, err := h.roles.GetRoleByName("sysadmin")
	if err != nil {
		return err
	}

	if sysRole == nil {
		for _, role := range authz.DefaultSystemRoles() {
			if strings.EqualFold(role.Name, "sysadmin") {
				sysRole = role
				break
			}
		}

		if sysRole == nil {
			return fmt.Errorf("sysadmin role definition is unavailable")
		}

		if err := h.roles.CreateRole(sysRole); err != nil {
			reloadedRole, reloadErr := h.roles.GetRoleByName("sysadmin")
			if reloadErr != nil {
				return reloadErr
			}
			if reloadedRole == nil {
				return err
			}
			sysRole = reloadedRole
		}
	}

	_, err = h.authorizer.AssignRole(userID, sysRole.ID)
	return err
}

func (h *Handler) resolveRoleIDsByName(roleNames []string) (map[string]struct{}, error) {
	desiredRoleIDs := make(map[string]struct{}, len(roleNames))

	for _, roleName := range roleNames {
		role, err := h.roles.GetRoleByName(roleName)
		if err != nil {
			return nil, err
		}
		if role == nil {
			return nil, fmt.Errorf("unknown role: %s", roleName)
		}
		desiredRoleIDs[role.ID] = struct{}{}
	}

	return desiredRoleIDs, nil
}

func (h *Handler) setUserRoles(userID string, roleNames []string) error {
	normalizedRoleNames := normalizeRoleNames(roleNames)
	desiredRoleIDs, err := h.resolveRoleIDsByName(normalizedRoleNames)
	if err != nil {
		return err
	}

	existingBindings, err := h.bindings.ListBindingsForUser(userID)
	if err != nil {
		return err
	}

	for _, binding := range existingBindings {
		if _, keep := desiredRoleIDs[binding.RoleID]; keep {
			delete(desiredRoleIDs, binding.RoleID)
			continue
		}
		if err := h.authorizer.RevokeRole(binding.ID); err != nil {
			return err
		}
	}

	for roleID := range desiredRoleIDs {
		if _, err := h.authorizer.AssignRole(userID, roleID); err != nil {
			return err
		}
	}

	return nil
}

func containsGrantType(grantTypes []string, target string) bool {
	t := strings.ToLower(strings.TrimSpace(target))
	if t == "" {
		return false
	}
	for _, grantType := range grantTypes {
		if strings.ToLower(strings.TrimSpace(grantType)) == t {
			return true
		}
	}
	return false
}

func configuredRealm() string {
	realm := strings.ToLower(strings.TrimSpace(os.Getenv("IAM_REALM")))
	if realm == "" {
		realm = iamconfig.DefaultRealm
	}
	return strings.ToLower(realm)
}

func resolveRealmName(raw string) (string, error) {
	realm := strings.ToLower(strings.TrimSpace(raw))
	if realm == "" {
		realm = configuredRealm()
	}
	if !realmNamePattern.MatchString(realm) {
		return "", fmt.Errorf("invalid realm name")
	}
	return realm, nil
}

func validateClientID(id string) error {
	clientID := strings.TrimSpace(id)
	if !clientIDPattern.MatchString(clientID) {
		return fmt.Errorf("new_client_id must match %s", clientIDPattern.String())
	}
	return nil
}

func defaultClientTokenValidityMinutes(fallbackTTL time.Duration) int {
	minutes := int((fallbackTTL + time.Minute - 1) / time.Minute)
	if minutes < minClientTokenValidityMinutes {
		minutes = 15
	}
	return minutes
}

func validateClientRateLimitMaxCalls(v int64) error {
	if v < minClientRateLimitMaxCalls || v > maxClientRateLimitMaxCalls {
		return fmt.Errorf("rate_limit_max_calls must be between %d and %d", minClientRateLimitMaxCalls, maxClientRateLimitMaxCalls)
	}
	return nil
}

func validateClientTokenValidityMinutes(v int) error {
	if v < minClientTokenValidityMinutes || v > maxClientTokenValidityMinutes {
		return fmt.Errorf("token_validity_minutes must be between %d and %d", minClientTokenValidityMinutes, maxClientTokenValidityMinutes)
	}
	return nil
}

func resolvedClientRateLimitMaxCalls(client *oauth.OAuthClient) int64 {
	if client == nil || client.RateLimitMaxCalls <= 0 {
		return defaultClientRateLimitMaxCalls
	}
	return client.RateLimitMaxCalls
}

func resolvedClientTokenValidityMinutes(client *oauth.OAuthClient, fallbackTTL time.Duration) int {
	if client != nil && client.TokenValidityMinutes > 0 {
		return client.TokenValidityMinutes
	}
	return defaultClientTokenValidityMinutes(fallbackTTL)
}

func resolvedClientAccessTokenTTL(client *oauth.OAuthClient, fallbackTTL time.Duration) time.Duration {
	minutes := resolvedClientTokenValidityMinutes(client, fallbackTTL)
	return time.Duration(minutes) * time.Minute
}

func (h *Handler) realmBaseURL(realm string) string {
	base := strings.TrimRight(h.issuer.IssuerURL(), "/")
	return base + "/realms/" + strings.ToLower(strings.TrimSpace(realm))
}

func extractClientCredentials(c *gin.Context, req *oauth.TokenRequest) (string, string, error) {
	clientID := strings.TrimSpace(req.ClientID)
	clientSecret := strings.TrimSpace(req.ClientSecret)

	if clientID != "" && clientSecret != "" {
		return clientID, clientSecret, nil
	}

	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader == "" || !strings.HasPrefix(strings.ToLower(authHeader), "basic ") {
		if clientID == "" {
			return "", "", fmt.Errorf("client_id is required")
		}
		if clientSecret == "" {
			return "", "", fmt.Errorf("client_secret is required")
		}
		return clientID, clientSecret, nil
	}

	encoded := strings.TrimSpace(authHeader[6:])
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", fmt.Errorf("invalid basic authorization header")
	}

	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid basic authorization header")
	}

	if clientID == "" {
		clientID = strings.TrimSpace(parts[0])
	}
	if clientSecret == "" {
		clientSecret = strings.TrimSpace(parts[1])
	}

	if clientID == "" {
		return "", "", fmt.Errorf("client_id is required")
	}
	if clientSecret == "" {
		return "", "", fmt.Errorf("client_secret is required")
	}

	return clientID, clientSecret, nil
}

func firstNonEmptyAudience(aud jwt.ClaimStrings) string {
	for _, candidate := range aud {
		trimmed := strings.TrimSpace(candidate)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (h *Handler) persistRefreshToken(rawToken, userID, clientID, scope, sessionID string) error {
	if h.refreshRepo == nil {
		return fmt.Errorf("refresh token storage not initialised")
	}

	claims, err := h.issuer.ValidateRefreshToken(rawToken)
	if err != nil {
		return fmt.Errorf("refresh token validation failed: %w", err)
	}

	jti := strings.TrimSpace(claims.ID)
	if jti == "" {
		return fmt.Errorf("refresh token is missing jti")
	}

	subject := strings.TrimSpace(userID)
	tokenSubject := strings.TrimSpace(claims.Subject)
	if subject == "" {
		subject = tokenSubject
	}
	if subject == "" {
		return fmt.Errorf("refresh token subject is missing")
	}
	if tokenSubject != "" && subject != tokenSubject {
		return fmt.Errorf("refresh token subject mismatch")
	}

	storedClientID := strings.TrimSpace(clientID)
	tokenClientID := firstNonEmptyAudience(claims.Audience)
	if storedClientID != tokenClientID {
		return fmt.Errorf("refresh token client mismatch")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(h.issuer.RefreshTokenTTL)
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time.UTC()
	}
	if expiresAt.Before(now) {
		return fmt.Errorf("refresh token already expired")
	}

	return h.refreshRepo.StoreRefreshToken(&oauth.RefreshTokenRecord{
		ID:        jti,
		UserID:    subject,
		ClientID:  storedClientID,
		SessionID: strings.TrimSpace(sessionID),
		Scope:     strings.TrimSpace(scope),
		ExpiresAt: expiresAt,
		Revoked:   false,
	})
}

func (h *Handler) issueAndStoreTokenPair(input token.IssueInput, accessTTL time.Duration) (*token.TokenPair, error) {
	pair, err := h.issuer.IssueTokenPairWithAccessTTL(input, accessTTL)
	if err != nil {
		return nil, err
	}

	if err := h.persistRefreshToken(pair.RefreshToken, input.Sub, input.ClientID, input.Scope, input.SessionID); err != nil {
		return nil, err
	}

	return pair, nil
}

func (h *Handler) validateRefreshTokenRecord(rawToken, expectedClientID string) (*jwt.RegisteredClaims, *oauth.RefreshTokenRecord, error) {
	claims, err := h.issuer.ValidateRefreshToken(rawToken)
	if err != nil {
		return nil, nil, err
	}

	if h.refreshRepo == nil {
		return nil, nil, fmt.Errorf("refresh token storage not initialised")
	}

	jti := strings.TrimSpace(claims.ID)
	if jti == "" {
		return nil, nil, fmt.Errorf("refresh token is missing jti")
	}

	record, err := h.refreshRepo.GetRefreshToken(jti)
	if err != nil {
		return nil, nil, err
	}
	if record == nil {
		return nil, nil, fmt.Errorf("refresh token not found")
	}
	if record.Revoked {
		return nil, nil, fmt.Errorf("refresh token revoked")
	}

	now := time.Now().UTC()
	if claims.ExpiresAt == nil || now.After(claims.ExpiresAt.Time.UTC()) {
		return nil, nil, fmt.Errorf("refresh token expired")
	}
	if now.After(record.ExpiresAt.UTC()) {
		return nil, nil, fmt.Errorf("refresh token record expired")
	}

	storedSubject := strings.TrimSpace(record.UserID)
	tokenSubject := strings.TrimSpace(claims.Subject)
	if storedSubject == "" || tokenSubject == "" || storedSubject != tokenSubject {
		return nil, nil, fmt.Errorf("refresh token subject mismatch")
	}

	storedClientID := strings.TrimSpace(record.ClientID)
	tokenClientID := firstNonEmptyAudience(claims.Audience)
	if storedClientID != tokenClientID {
		return nil, nil, fmt.Errorf("refresh token audience mismatch")
	}

	expected := strings.TrimSpace(expectedClientID)
	if expected != "" && storedClientID != expected {
		return nil, nil, fmt.Errorf("refresh token client mismatch")
	}

	return claims, record, nil
}

func (h *Handler) ensureSessionIsActive(sessionID string) error {
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID == "" {
		return nil
	}

	if h.sessions == nil {
		return fmt.Errorf("session storage not initialised")
	}

	session, err := h.sessions.GetByID(trimmedSessionID)
	if err != nil {
		return err
	}
	if session == nil || !session.Active {
		return fmt.Errorf("session not active")
	}

	now := time.Now().UTC()
	if now.After(session.ExpiresAt.UTC()) {
		_ = h.sessions.Revoke(trimmedSessionID)
		return fmt.Errorf("session expired")
	}

	if h.issuer.RefreshTokenTTL > 0 {
		session.ExpiresAt = now.Add(h.issuer.RefreshTokenTTL)
		if err := h.sessions.Create(session); err != nil {
			return fmt.Errorf("failed to extend session: %w", err)
		}
	}

	return nil
}

// ═══════════════════════════════════════════════
// AUTH ENDPOINTS (public)
// ═══════════════════════════════════════════════

// Login authenticates a user and issues tokens.
func (h *Handler) Login(c *gin.Context) {
	var req authn.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	user, err := h.authn.Authenticate(req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Resolve roles for the user
	roleNames, _ := h.authorizer.GetUserRoleNames(user.ID)
	if isBootstrapSysadminEmail(user.Email) && !hasRoleName(roleNames, "sysadmin") {
		if ensureErr := h.ensureUserHasSysadminRole(user.ID); ensureErr != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  IAM: failed to auto-heal sysadmin role binding for %s: %v", user.Email, ensureErr))
		} else if refreshedRoleNames, refreshedErr := h.authorizer.GetUserRoleNames(user.ID); refreshedErr == nil {
			roleNames = refreshedRoleNames
		}
	}
	roleNames = normalizeRoleNames(roleNames)

	// Bind refresh token lifecycle to a server-side login session.
	sessionTTL := h.issuer.RefreshTokenTTL
	if sessionTTL <= 0 {
		sessionTTL = h.issuer.AccessTokenTTL + time.Hour
	}

	session, err := h.authn.CreateSession(user.ID, c.ClientIP(), c.GetHeader("User-Agent"), sessionTTL)
	if err != nil || session == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create login session"})
		return
	}

	sessionID := session.ID
	pair, err := h.issueAndStoreTokenPair(token.IssueInput{
		Sub:         user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Scope:       "openid profile email roles",
		ClientID:    "",
		SessionID:   sessionID,
		Roles:       roleNames,
	}, h.issuer.AccessTokenTTL)
	if err != nil {
		_ = h.sessions.Revoke(sessionID)
		logging.Z().Info(fmt.Sprintf("⚠️  IAM: login token issue/store failed for user=%s: %v", user.ID, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token issuance failed"})
		return
	}

	c.JSON(http.StatusOK, authn.LoginResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		TokenType:    pair.TokenType,
		ExpiresIn:    pair.ExpiresIn,
		ExpiresAt:    pair.ExpiresAt,
		User: authn.UserInfo{
			ID:            user.ID,
			Email:         user.Email,
			DisplayName:   user.DisplayName,
			Roles:         roleNames,
			EmailVerified: user.EmailVerified,
		},
	})
}

// RefreshToken issues new tokens from a valid refresh token.
func (h *Handler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token is required"})
		return
	}

	claims, record, err := h.validateRefreshTokenRecord(req.RefreshToken, "")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}
	if err := h.ensureSessionIsActive(record.SessionID); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login session has expired or been revoked"})
		return
	}

	// Look up user to ensure still active
	userID := strings.TrimSpace(claims.Subject)
	if userID == "" {
		userID = strings.TrimSpace(record.UserID)
	}
	user, err := h.users.GetByID(userID)
	if err != nil || user == nil || !user.Active {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user account not found or disabled"})
		return
	}

	roleNames, _ := h.authorizer.GetUserRoleNames(user.ID)
	scope := strings.TrimSpace(record.Scope)
	if scope == "" {
		scope = "openid profile email roles"
	}

	if err := h.refreshRepo.RevokeRefreshToken(claims.ID); err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  IAM: refresh token revoke failed for user=%s jti=%s: %v", user.ID, claims.ID, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token refresh failed"})
		return
	}

	pair, err := h.issueAndStoreTokenPair(token.IssueInput{
		Sub:         user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Scope:       scope,
		ClientID:    strings.TrimSpace(record.ClientID),
		SessionID:   strings.TrimSpace(record.SessionID),
		Roles:       roleNames,
	}, h.issuer.AccessTokenTTL)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  IAM: refresh token rotation failed for user=%s: %v", user.ID, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token re-issuance failed"})
		return
	}

	c.JSON(http.StatusOK, pair)
}

// Logout revokes the current access token, bound session, and active refresh token(s).
func (h *Handler) Logout(c *gin.Context) {
	claims := iammw.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	accessTokenRevoked := false
	if h.revokedStore != nil && strings.TrimSpace(claims.ID) != "" {
		ttl := time.Hour
		if claims.ExpiresAt != nil {
			ttl = time.Until(claims.ExpiresAt.Time)
			if ttl <= 0 {
				ttl = time.Minute
			}
		}
		if err := h.revokedStore.Revoke(strings.TrimSpace(claims.ID), ttl); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke access token"})
			return
		}
		accessTokenRevoked = true
	}

	sessionID := strings.TrimSpace(claims.SessionID)
	sessionRevoked := false
	if sessionID != "" {
		if h.sessions == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "session storage not initialised"})
			return
		}
		if err := h.sessions.Revoke(sessionID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke session"})
			return
		}
		sessionRevoked = true
	}

	refreshRevoked := false
	providedRefreshToken := strings.TrimSpace(req.RefreshToken)
	if providedRefreshToken != "" {
		refreshClaims, record, err := h.validateRefreshTokenRecord(providedRefreshToken, claims.ClientID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
			return
		}

		if strings.TrimSpace(refreshClaims.Subject) != strings.TrimSpace(claims.Sub) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token does not belong to current user"})
			return
		}

		if sessionID != "" {
			recordSessionID := strings.TrimSpace(record.SessionID)
			if recordSessionID != "" && recordSessionID != sessionID {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token does not belong to current session"})
				return
			}
		}

		if err := h.refreshRepo.RevokeRefreshToken(strings.TrimSpace(refreshClaims.ID)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke refresh token"})
			return
		}
		refreshRevoked = true
	}

	if !refreshRevoked && sessionID != "" {
		if h.refreshRepo == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "refresh token storage not initialised"})
			return
		}
		if err := h.refreshRepo.RevokeBySessionID(sessionID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke session refresh tokens"})
			return
		}
		refreshRevoked = true
	}

	c.JSON(http.StatusOK, gin.H{
		"status":                 "ok",
		"access_token_revoked":   accessTokenRevoked,
		"session_revoked":        sessionRevoked,
		"refresh_tokens_revoked": refreshRevoked,
	})
}

// WhoAmI returns the currently authenticated user's info.
func (h *Handler) WhoAmI(c *gin.Context) {
	claims := iammw.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":      claims.Sub,
		"email":        claims.Email,
		"display_name": claims.DisplayName,
		"roles":        claims.Roles,
	})
}

// ═══════════════════════════════════════════════
// USER MANAGEMENT (sysadmin only)
// ═══════════════════════════════════════════════

// ListUsers returns all IAM users.
func (h *Handler) ListUsers(c *gin.Context) {
	users, err := h.users.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}

	for _, user := range users {
		if user == nil {
			continue
		}
		roleNames, _ := h.authorizer.GetUserRoleNames(user.ID)
		user.Roles = roleNames
	}

	c.JSON(http.StatusOK, gin.H{"users": users, "count": len(users)})
}

// GetUser returns a single user.
func (h *Handler) GetUser(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id required"})
		return
	}
	user, err := h.users.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	roleNames, _ := h.authorizer.GetUserRoleNames(user.ID)
	user.Roles = roleNames
	c.JSON(http.StatusOK, user)
}

// CreateUser registers a new IAM user.
func (h *Handler) CreateUser(c *gin.Context) {
	var req struct {
		Email         string   `json:"email" binding:"required,email"`
		Password      string   `json:"password" binding:"required,min=8"`
		DisplayName   string   `json:"display_name"`
		Active        *bool    `json:"active"`
		EmailVerified *bool    `json:"email_verified"`
		RoleNames     []string `json:"role_names"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	desiredRoleNames := normalizeRoleNames(req.RoleNames)
	if _, err := h.resolveRoleIDsByName(desiredRoleNames); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := identity.NormaliseEmail(req.Email)
	existing, _ := h.users.GetByEmail(email)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
		return
	}

	hash, err := identity.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user := &identity.User{
		ID:           identity.NewUserID(),
		Email:        email,
		PasswordHash: hash,
		DisplayName:  strings.TrimSpace(req.DisplayName),
		Active:       true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if req.Active != nil {
		user.Active = *req.Active
	}
	if req.EmailVerified != nil {
		user.EmailVerified = *req.EmailVerified
	}

	if err := h.users.Create(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	if err := h.setUserRoles(user.ID, desiredRoleNames); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user created but role assignment failed: " + err.Error()})
		return
	}

	roleNames, _ := h.authorizer.GetUserRoleNames(user.ID)
	user.Roles = roleNames

	c.JSON(http.StatusCreated, user)
}

// UpdateUser modifies a user record.
func (h *Handler) UpdateUser(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id required"})
		return
	}

	user, err := h.users.GetByID(id)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var req identity.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Email != nil {
		user.Email = identity.NormaliseEmail(*req.Email)
	}
	if req.Password != nil {
		hash, err := identity.HashPassword(*req.Password)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		user.PasswordHash = hash
	}
	if req.DisplayName != nil {
		user.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.Active != nil {
		user.Active = *req.Active
	}
	if req.EmailVerified != nil {
		user.EmailVerified = *req.EmailVerified
	}
	user.UpdatedAt = time.Now().UTC()

	if err := h.users.Update(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}
	c.JSON(http.StatusOK, user)
}

// SetUserRoles replaces the role mapping for a user.
func (h *Handler) SetUserRoles(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id required"})
		return
	}

	user, err := h.users.GetByID(id)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var req struct {
		RoleNames []string `json:"role_names" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	normalizedRoleNames := normalizeRoleNames(req.RoleNames)
	actorUserID := iammw.GetUserID(c)
	if actorUserID == user.ID && !hasRoleName(normalizedRoleNames, "sysadmin") {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot remove your own sysadmin role"})
		return
	}

	if err := h.setUserRoles(user.ID, normalizedRoleNames); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unknown role") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user roles"})
		return
	}

	roleNames, _ := h.authorizer.GetUserRoleNames(user.ID)
	c.JSON(http.StatusOK, gin.H{
		"user_id": user.ID,
		"roles":   roleNames,
		"count":   len(roleNames),
	})
}

// DeleteUser removes a user.
func (h *Handler) DeleteUser(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id required"})
		return
	}
	if err := h.users.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

// ═══════════════════════════════════════════════
// OAUTH CLIENT MANAGEMENT (sysadmin only)
// ═══════════════════════════════════════════════

// RegisterClient creates a new OAuth2 client.
func (h *Handler) RegisterClient(c *gin.Context) {
	var req struct {
		Name                 string   `json:"name" binding:"required"`
		RedirectURIs         []string `json:"redirect_uris"`
		Scopes               []string `json:"scopes"`
		GrantTypes           []string `json:"grant_types"`
		ServiceRoles         []string `json:"service_roles"`
		RateLimitMaxCalls    int64    `json:"rate_limit_max_calls"`
		TokenValidityMinutes int      `json:"token_validity_minutes"`
		Public               bool     `json:"public"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.GrantTypes) == 0 {
		req.GrantTypes = []string{"authorization_code", "refresh_token", "client_credentials"}
	}
	if containsGrantType(req.GrantTypes, "authorization_code") && len(req.RedirectURIs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "redirect_uris are required when authorization_code grant is enabled"})
		return
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{"openid", "profile", "email"}
	}

	rateLimitMaxCalls := req.RateLimitMaxCalls
	if rateLimitMaxCalls <= 0 {
		rateLimitMaxCalls = defaultClientRateLimitMaxCalls
	}
	if err := validateClientRateLimitMaxCalls(rateLimitMaxCalls); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokenValidityMinutes := req.TokenValidityMinutes
	if tokenValidityMinutes <= 0 {
		tokenValidityMinutes = defaultClientTokenValidityMinutes(h.issuer.AccessTokenTTL)
	}
	if err := validateClientTokenValidityMinutes(tokenValidityMinutes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	serviceRoles := normalizeRoleNames(req.ServiceRoles)
	if _, err := h.resolveRoleIDsByName(serviceRoles); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	secret := ""
	secretHash := ""
	if !req.Public {
		secret = oauth.GenerateClientSecret()
		var err error
		secretHash, err = identity.HashPassword(secret)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "secret generation failed"})
			return
		}
	}

	client := &oauth.OAuthClient{
		ID:                   oauth.GenerateClientID(),
		Secret:               secretHash,
		Name:                 strings.TrimSpace(req.Name),
		RedirectURIs:         req.RedirectURIs,
		Scopes:               req.Scopes,
		GrantTypes:           req.GrantTypes,
		ServiceRoles:         serviceRoles,
		RateLimitMaxCalls:    rateLimitMaxCalls,
		TokenValidityMinutes: tokenValidityMinutes,
		Public:               req.Public,
		CreatedAt:            time.Now().UTC(),
		Active:               true,
	}

	if err := h.clients.CreateClient(client); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create client"})
		return
	}

	// Return secret in plaintext only during creation
	resp := gin.H{
		"id":                     client.ID,
		"name":                   client.Name,
		"redirect_uris":          client.RedirectURIs,
		"scopes":                 client.Scopes,
		"grant_types":            client.GrantTypes,
		"service_roles":          client.ServiceRoles,
		"rate_limit_max_calls":   client.RateLimitMaxCalls,
		"token_validity_minutes": client.TokenValidityMinutes,
		"public":                 client.Public,
		"created_at":             client.CreatedAt,
	}
	if secret != "" {
		resp["client_secret"] = secret
		resp["warning"] = "Store the client_secret securely. It will not be shown again."
	}

	c.JSON(http.StatusCreated, resp)
}

// ListClients returns all OAuth clients.
func (h *Handler) ListClients(c *gin.Context) {
	clients, err := h.clients.ListClients()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list clients"})
		return
	}
	// Never expose secrets in list
	safe := make([]gin.H, 0, len(clients))
	for _, cl := range clients {
		safe = append(safe, gin.H{
			"id":                     cl.ID,
			"name":                   cl.Name,
			"redirect_uris":          cl.RedirectURIs,
			"scopes":                 cl.Scopes,
			"grant_types":            cl.GrantTypes,
			"service_roles":          cl.ServiceRoles,
			"rate_limit_max_calls":   resolvedClientRateLimitMaxCalls(cl),
			"token_validity_minutes": resolvedClientTokenValidityMinutes(cl, h.issuer.AccessTokenTTL),
			"public":                 cl.Public,
			"active":                 cl.Active,
			"created_at":             cl.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"clients": safe, "count": len(safe)})
}

// GetClient returns a single client (secret hidden).
func (h *Handler) GetClient(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	client, err := h.clients.GetClient(id)
	if err != nil || client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":                     client.ID,
		"name":                   client.Name,
		"redirect_uris":          client.RedirectURIs,
		"scopes":                 client.Scopes,
		"grant_types":            client.GrantTypes,
		"service_roles":          client.ServiceRoles,
		"rate_limit_max_calls":   resolvedClientRateLimitMaxCalls(client),
		"token_validity_minutes": resolvedClientTokenValidityMinutes(client, h.issuer.AccessTokenTTL),
		"public":                 client.Public,
		"active":                 client.Active,
		"created_at":             client.CreatedAt,
	})
}

// UpdateClient modifies a client's redirect URIs, scopes, or active status.
func (h *Handler) UpdateClient(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	client, err := h.clients.GetClient(id)
	if err != nil || client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	var req struct {
		RedirectURIs         *[]string `json:"redirect_uris"`
		Scopes               *[]string `json:"scopes"`
		GrantTypes           *[]string `json:"grant_types"`
		ServiceRoles         *[]string `json:"service_roles"`
		RateLimitMaxCalls    *int64    `json:"rate_limit_max_calls"`
		TokenValidityMinutes *int      `json:"token_validity_minutes"`
		Active               *bool     `json:"active"`
		Name                 *string   `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.RedirectURIs != nil {
		client.RedirectURIs = *req.RedirectURIs
	}
	if req.Scopes != nil {
		client.Scopes = *req.Scopes
	}
	if req.GrantTypes != nil {
		client.GrantTypes = *req.GrantTypes
	}
	if containsGrantType(client.GrantTypes, "authorization_code") && len(client.RedirectURIs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "redirect_uris are required when authorization_code grant is enabled"})
		return
	}
	if req.ServiceRoles != nil {
		normalized := normalizeRoleNames(*req.ServiceRoles)
		if _, err := h.resolveRoleIDsByName(normalized); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		client.ServiceRoles = normalized
	}
	if req.RateLimitMaxCalls != nil {
		if err := validateClientRateLimitMaxCalls(*req.RateLimitMaxCalls); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		client.RateLimitMaxCalls = *req.RateLimitMaxCalls
	}
	if req.TokenValidityMinutes != nil {
		if err := validateClientTokenValidityMinutes(*req.TokenValidityMinutes); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		client.TokenValidityMinutes = *req.TokenValidityMinutes
	}
	if req.Active != nil {
		client.Active = *req.Active
	}
	if req.Name != nil {
		client.Name = strings.TrimSpace(*req.Name)
	}

	if err := h.clients.UpdateClient(client); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "client updated", "id": client.ID})
}

// DeleteClient removes a client.
func (h *Handler) DeleteClient(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if err := h.clients.DeleteClient(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "client deleted"})
}

// RegenerateClientSecret creates a new secret for a confidential client.
func (h *Handler) RegenerateClientSecret(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	client, err := h.clients.GetClient(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load client"})
		return
	}
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}
	if client.Public {
		c.JSON(http.StatusBadRequest, gin.H{"error": "public clients do not have secrets"})
		return
	}

	newSecret := oauth.GenerateClientSecret()
	hash, err := identity.HashPassword(newSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "secret generation failed"})
		return
	}

	client.Secret = hash
	if err := h.clients.UpdateClient(client); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update client secret"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                     client.ID,
		"client_id":              client.ID,
		"client_secret":          newSecret,
		"scopes":                 client.Scopes,
		"grant_types":            client.GrantTypes,
		"rate_limit_max_calls":   resolvedClientRateLimitMaxCalls(client),
		"token_validity_minutes": resolvedClientTokenValidityMinutes(client, h.issuer.AccessTokenTTL),
		"warning":                "Store the client_secret securely. It will not be shown again.",
	})
}

// ChangeClientID changes a client identifier in a Keycloak-like way.
func (h *Handler) ChangeClientID(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	client, err := h.clients.GetClient(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load client"})
		return
	}
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	var req struct {
		NewClientID string `json:"new_client_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newClientID := strings.TrimSpace(req.NewClientID)
	if newClientID == id {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new_client_id must be different"})
		return
	}
	if err := validateClientID(newClientID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, err := h.clients.GetClient(newClientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate new client id"})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "new_client_id already exists"})
		return
	}

	replacement := *client
	replacement.ID = newClientID

	if err := h.clients.CreateClient(&replacement); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create client with new id"})
		return
	}

	if err := h.clients.DeleteClient(id); err != nil {
		_ = h.clients.DeleteClient(newClientID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize client id change"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":                "client id updated",
		"old_client_id":          id,
		"new_client_id":          newClientID,
		"redirect_uris":          replacement.RedirectURIs,
		"scopes":                 replacement.Scopes,
		"grant_types":            replacement.GrantTypes,
		"service_roles":          replacement.ServiceRoles,
		"rate_limit_max_calls":   resolvedClientRateLimitMaxCalls(&replacement),
		"token_validity_minutes": resolvedClientTokenValidityMinutes(&replacement, h.issuer.AccessTokenTTL),
		"public":                 replacement.Public,
		"active":                 replacement.Active,
		"created_at":             replacement.CreatedAt,
	})
}

// ═══════════════════════════════════════════════
// ROLE MANAGEMENT (sysadmin only)
// ═══════════════════════════════════════════════

// CreateRole creates a new IAM role.
func (h *Handler) CreateRole(c *gin.Context) {
	var req authz.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, _ := h.roles.GetRoleByName(req.Name)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "role name already exists"})
		return
	}

	now := time.Now().UTC()
	role := &authz.Role{
		ID:          "role-" + strings.ToLower(strings.ReplaceAll(req.Name, " ", "-")) + "-" + now.Format("20060102150405"),
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
		System:      false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.roles.CreateRole(role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create role"})
		return
	}
	c.JSON(http.StatusCreated, role)
}

// ListRoles returns all IAM roles.
func (h *Handler) ListRoles(c *gin.Context) {
	roles, err := h.roles.ListRoles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list roles"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"roles": roles, "count": len(roles)})
}

// GetRole returns a single role.
func (h *Handler) GetRole(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	role, err := h.roles.GetRole(id)
	if err != nil || role == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	c.JSON(http.StatusOK, role)
}

// UpdateRole modifies a role.
func (h *Handler) UpdateRole(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	role, err := h.roles.GetRole(id)
	if err != nil || role == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	if role.System {
		c.JSON(http.StatusForbidden, gin.H{"error": "system roles cannot be modified"})
		return
	}

	var req struct {
		Description *string             `json:"description"`
		Permissions *[]authz.Permission `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Description != nil {
		role.Description = *req.Description
	}
	if req.Permissions != nil {
		role.Permissions = *req.Permissions
	}
	role.UpdatedAt = time.Now().UTC()

	if err := h.roles.UpdateRole(role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}
	c.JSON(http.StatusOK, role)
}

// DeleteRole removes a non-system role.
func (h *Handler) DeleteRole(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	role, err := h.roles.GetRole(id)
	if err != nil || role == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	if role.System {
		c.JSON(http.StatusForbidden, gin.H{"error": "system roles cannot be deleted"})
		return
	}
	if err := h.roles.DeleteRole(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "role deleted"})
}

// ═══════════════════════════════════════════════
// ROLE ASSIGNMENT (sysadmin only)
// ═══════════════════════════════════════════════

// AssignRole binds a user to a role.
func (h *Handler) AssignRole(c *gin.Context) {
	var req authz.AssignRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	binding, err := h.authorizer.AssignRole(req.UserID, req.RoleID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, binding)
}

// ListBindings returns role bindings, optionally filtered by user_id query param.
func (h *Handler) ListBindings(c *gin.Context) {
	userID := strings.TrimSpace(c.Query("user_id"))

	var bindings []*authz.RoleBinding
	var err error
	if userID != "" {
		bindings, err = h.bindings.ListBindingsForUser(userID)
	} else {
		bindings, err = h.bindings.ListAllBindings()
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list bindings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"bindings": bindings, "count": len(bindings)})
}

// RevokeBinding removes a role binding.
func (h *Handler) RevokeBinding(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if err := h.authorizer.RevokeRole(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revoke failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "role binding revoked"})
}

// ═══════════════════════════════════════════════
// TOKEN MANAGEMENT (sysadmin only)
// ═══════════════════════════════════════════════

// RevokeToken marks a token JTI as revoked.
func (h *Handler) RevokeToken(c *gin.Context) {
	var req struct {
		JTI string `json:"jti" binding:"required"`
		TTL int    `json:"ttl_seconds"` // how long to remember the revocation
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ttl := time.Duration(req.TTL) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}

	if err := h.revokedStore.Revoke(req.JTI, ttl); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revocation failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "token revoked", "jti": req.JTI})
}

// RevokeUserTokens revokes all refresh tokens for a user.
func (h *Handler) RevokeUserTokens(c *gin.Context) {
	userID := strings.TrimSpace(c.Param("id"))
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id required"})
		return
	}

	if err := h.refreshRepo.RevokeAllForUser(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revocation failed"})
		return
	}
	// Also revoke sessions
	_ = h.sessions.RevokeByUserID(userID)

	c.JSON(http.StatusOK, gin.H{"message": "all tokens revoked for user", "user_id": userID})
}

// ═══════════════════════════════════════════════
// OAUTH2 ENDPOINTS (Authorization Code + PKCE)
// ═══════════════════════════════════════════════

// Authorize handles GET /oauth/authorize (Authorization Code + PKCE).
// In a real deployment this would render a consent screen; for API-first usage
// it validates parameters and issues the code directly to authenticated users.
func (h *Handler) Authorize(c *gin.Context) {
	var req oauth.AuthorizeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid authorize request: " + err.Error()})
		return
	}

	if req.ResponseType != "code" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only response_type=code is supported"})
		return
	}
	if req.CodeChallengeMethod != "S256" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only S256 code_challenge_method is supported (PKCE required)"})
		return
	}

	client, err := h.clients.GetClient(req.ClientID)
	if err != nil || client == nil || !client.Active {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown or inactive client_id"})
		return
	}

	if err := oauth.ValidateRedirectURI(client.RedirectURIs, req.RedirectURI); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	scopes := oauth.ParseScopes(req.Scope)
	if len(scopes) > 0 {
		if err := oauth.ValidateScopes(client.Scopes, scopes); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// The user must be authenticated (via IAM JWT)
	userID := iammw.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user must be authenticated to authorize"})
		return
	}

	code, err := oauth.GenerateAuthorizationCode()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "code generation failed"})
		return
	}

	codeRecord := &oauth.AuthorizationCode{
		Code:                code,
		ClientID:            req.ClientID,
		UserID:              userID,
		RedirectURI:         req.RedirectURI,
		Scope:               req.Scope,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		ExpiresAt:           time.Now().UTC().Add(5 * time.Minute),
	}

	if h.codeRepo == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "code storage not initialised"})
		return
	}
	if err := h.codeRepo.StoreCode(codeRecord); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store authorization code"})
		return
	}

	// Redirect back with code
	c.JSON(http.StatusOK, gin.H{
		"code":         code,
		"state":        req.State,
		"redirect_uri": req.RedirectURI,
	})
}

// Token handles POST /oauth/token (code exchange + refresh).
func (h *Handler) Token(c *gin.Context) {
	var req oauth.TokenRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token request"})
		return
	}

	switch req.GrantType {
	case "authorization_code":
		h.handleAuthorizationCodeGrant(c, &req)
	case "refresh_token":
		h.handleRefreshTokenGrant(c, &req)
	case "client_credentials":
		h.handleClientCredentialsGrant(c, &req)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported grant_type"})
	}
}

func (h *Handler) handleAuthorizationCodeGrant(c *gin.Context, req *oauth.TokenRequest) {
	if h.codeRepo == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "code storage not initialised"})
		return
	}

	if strings.TrimSpace(req.ClientID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client_id is required"})
		return
	}

	if req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	codeRecord, err := h.codeRepo.GetCode(req.Code)
	if err != nil || codeRecord == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired authorization code"})
		return
	}

	// Invalidate the code immediately (single-use)
	_ = h.codeRepo.InvalidateCode(req.Code)

	if codeRecord.Used {
		c.JSON(http.StatusBadRequest, gin.H{"error": "authorization code already used"})
		return
	}

	if time.Now().UTC().After(codeRecord.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "authorization code expired"})
		return
	}

	if codeRecord.ClientID != req.ClientID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client_id mismatch"})
		return
	}

	if codeRecord.RedirectURI != req.RedirectURI {
		c.JSON(http.StatusBadRequest, gin.H{"error": "redirect_uri mismatch"})
		return
	}

	// Verify PKCE
	if err := oauth.VerifyPKCE(codeRecord.CodeChallenge, req.CodeVerifier, codeRecord.CodeChallengeMethod); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate client secret (for confidential clients)
	client, err := h.clients.GetClient(req.ClientID)
	if err != nil || client == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown client"})
		return
	}
	if !client.Public && client.Secret != "" {
		if !identity.CheckPassword(req.ClientSecret, client.Secret) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid client_secret"})
			return
		}
	}

	// Look up user
	user, err := h.users.GetByID(codeRecord.UserID)
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
		return
	}

	roleNames, _ := h.authorizer.GetUserRoleNames(user.ID)
	accessTTL := resolvedClientAccessTokenTTL(client, h.issuer.AccessTokenTTL)
	grantedScope := strings.TrimSpace(codeRecord.Scope)
	if grantedScope == "" {
		grantedScope = "openid profile email roles"
	}

	pair, err := h.issueAndStoreTokenPair(token.IssueInput{
		Sub:         user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Scope:       grantedScope,
		ClientID:    req.ClientID,
		SessionID:   "",
		Roles:       roleNames,
	}, accessTTL)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  IAM: auth code token issue/store failed for user=%s client=%s: %v", user.ID, req.ClientID, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token issuance failed"})
		return
	}

	c.JSON(http.StatusOK, pair)
}

func (h *Handler) handleRefreshTokenGrant(c *gin.Context, req *oauth.TokenRequest) {
	if strings.TrimSpace(req.ClientID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client_id is required"})
		return
	}

	if req.RefreshToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token is required"})
		return
	}

	client, err := h.clients.GetClient(req.ClientID)
	if err != nil || client == nil || !client.Active {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unknown or inactive client"})
		return
	}
	if !containsGrantType(client.GrantTypes, "refresh_token") {
		c.JSON(http.StatusForbidden, gin.H{"error": "client is not allowed to use refresh_token grant"})
		return
	}

	claims, record, err := h.validateRefreshTokenRecord(req.RefreshToken, strings.TrimSpace(req.ClientID))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}
	if err := h.ensureSessionIsActive(record.SessionID); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login session has expired or been revoked"})
		return
	}

	userID := strings.TrimSpace(claims.Subject)
	if userID == "" {
		userID = strings.TrimSpace(record.UserID)
	}
	user, err := h.users.GetByID(userID)
	if err != nil || user == nil || !user.Active {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found or disabled"})
		return
	}

	roleNames, _ := h.authorizer.GetUserRoleNames(user.ID)

	scope := strings.TrimSpace(record.Scope)
	if scope == "" {
		scope = "openid profile email roles"
	}
	requestedScope := strings.TrimSpace(req.Scope)
	if requestedScope != "" {
		if err := oauth.ValidateScopes(oauth.ParseScopes(scope), oauth.ParseScopes(requestedScope)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		scope = requestedScope
	}
	accessTTL := resolvedClientAccessTokenTTL(client, h.issuer.AccessTokenTTL)

	if err := h.refreshRepo.RevokeRefreshToken(claims.ID); err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  IAM: oauth refresh token revoke failed for user=%s client=%s jti=%s: %v", user.ID, req.ClientID, claims.ID, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token refresh failed"})
		return
	}

	pair, err := h.issueAndStoreTokenPair(token.IssueInput{
		Sub:         user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Scope:       scope,
		ClientID:    strings.TrimSpace(req.ClientID),
		SessionID:   strings.TrimSpace(record.SessionID),
		Roles:       roleNames,
	}, accessTTL)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  IAM: oauth refresh token rotation failed for user=%s client=%s: %v", user.ID, req.ClientID, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token re-issuance failed"})
		return
	}

	c.JSON(http.StatusOK, pair)
}

func (h *Handler) handleClientCredentialsGrant(c *gin.Context, req *oauth.TokenRequest) {
	clientID, clientSecret, err := extractClientCredentials(c, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	client, err := h.clients.GetClient(clientID)
	if err != nil || client == nil || !client.Active {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unknown or inactive client"})
		return
	}

	if !containsGrantType(client.GrantTypes, "client_credentials") {
		c.JSON(http.StatusForbidden, gin.H{"error": "client is not allowed to use client_credentials grant"})
		return
	}

	if client.Public || strings.TrimSpace(client.Secret) == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "public clients cannot use client_credentials grant"})
		return
	}

	if !identity.CheckPassword(clientSecret, client.Secret) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid client_secret"})
		return
	}

	requestedScopes := oauth.ParseScopes(req.Scope)
	if len(requestedScopes) > 0 {
		if err := oauth.ValidateScopes(client.Scopes, requestedScopes); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else {
		requestedScopes = client.Scopes
	}

	grantedScope := strings.Join(requestedScopes, " ")
	serviceRoles := normalizeRoleNames(client.ServiceRoles)
	accessTTL := resolvedClientAccessTokenTTL(client, h.issuer.AccessTokenTTL)

	accessToken, err := h.issuer.IssueAccessTokenWithTTL(token.IssueInput{
		Sub:         "service:" + client.ID,
		Email:       "",
		DisplayName: client.Name,
		Scope:       grantedScope,
		ClientID:    client.ID,
		SessionID:   "",
		Roles:       serviceRoles,
	}, accessTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token issuance failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":           accessToken.AccessToken,
		"token_type":             accessToken.TokenType,
		"expires_in":             accessToken.ExpiresIn,
		"scope":                  accessToken.Scope,
		"rate_limit_max_calls":   resolvedClientRateLimitMaxCalls(client),
		"token_validity_minutes": resolvedClientTokenValidityMinutes(client, h.issuer.AccessTokenTTL),
	})
}

// ═══════════════════════════════════════════════
// OIDC DISCOVERY
// ═══════════════════════════════════════════════

// OpenIDConfiguration returns the OIDC discovery document.
func (h *Handler) OpenIDConfiguration(c *gin.Context) {
	c.JSON(http.StatusOK, h.issuer.OpenIDConfiguration())
}

// OpenIDConfigurationRealm returns OIDC discovery in Keycloak-compatible realm path format.
func (h *Handler) OpenIDConfigurationRealm(c *gin.Context) {
	realm, err := resolveRealmName(c.Param("realm"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	realmBase := h.realmBaseURL(realm)
	c.JSON(http.StatusOK, h.issuer.OpenIDConfigurationWithEndpoints(
		realmBase,
		realmBase+"/protocol/openid-connect/auth",
		realmBase+"/protocol/openid-connect/token",
		realmBase+"/protocol/openid-connect/certs",
	))
}

// RealmToken provides a Keycloak-compatible token endpoint path.
func (h *Handler) RealmToken(c *gin.Context) {
	if _, err := resolveRealmName(c.Param("realm")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.Token(c)
}

// RealmAuthorize provides a Keycloak-compatible authorize endpoint path.
func (h *Handler) RealmAuthorize(c *gin.Context) {
	if _, err := resolveRealmName(c.Param("realm")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.Authorize(c)
}

// RealmJWKS returns Keycloak-compatible realm certs endpoint.
func (h *Handler) RealmJWKS(c *gin.Context) {
	if _, err := resolveRealmName(c.Param("realm")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(http.StatusOK, h.issuer.JWKS())
}

// ServiceAccessInfo returns shareable auth endpoints for external service integrations.
func (h *Handler) ServiceAccessInfo(c *gin.Context) {
	realm, err := resolveRealmName(c.Query("realm"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	configured := configuredRealm()
	base := strings.TrimRight(h.issuer.IssuerURL(), "/")
	realmBase := h.realmBaseURL(realm)

	c.JSON(http.StatusOK, gin.H{
		"realm":            realm,
		"configured_realm": configured,
		"issuer":           realmBase,
		"endpoints": gin.H{
			"iam_openid_configuration":      base + "/.well-known/openid-configuration",
			"iam_token":                     base + "/oauth/token",
			"iam_authorize":                 base + "/oauth/authorize",
			"iam_jwks":                      base + "/.well-known/jwks.json",
			"keycloak_openid_configuration": realmBase + "/.well-known/openid-configuration",
			"keycloak_token":                realmBase + "/protocol/openid-connect/token",
			"keycloak_authorize":            realmBase + "/protocol/openid-connect/auth",
			"keycloak_certs":                realmBase + "/protocol/openid-connect/certs",
		},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token", "client_credentials"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
	})
}

// JWKS returns the JSON Web Key Set.
func (h *Handler) JWKS(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(http.StatusOK, h.issuer.JWKS())
}
