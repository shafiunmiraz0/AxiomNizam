package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/pgstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// EnhancedHandler exposes Keycloak-style admin endpoints backed by PostgreSQL.
type EnhancedHandler struct {
	store *pgstore.Store
}

// NewEnhancedHandler creates the enhanced admin handler.
func NewEnhancedHandler(store *pgstore.Store) *EnhancedHandler {
	return &EnhancedHandler{store: store}
}

// ═══════════════════════════════════════════════
// Realm Endpoints
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) ListRealms(c *gin.Context) {
	realms, err := h.store.ListRealms()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, realms)
}

func (h *EnhancedHandler) GetRealm(c *gin.Context) {
	id := c.Param("realmId")
	realm, err := h.store.GetRealm(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if realm == nil {
		// Try by name
		realm, err = h.store.GetRealmByName(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if realm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "realm not found"})
		return
	}
	c.JSON(http.StatusOK, realm)
}

func (h *EnhancedHandler) CreateRealm(c *gin.Context) {
	var realm models.Realm
	if err := c.ShouldBindJSON(&realm); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if realm.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	realm.Name = strings.ToLower(strings.TrimSpace(realm.Name))
	existing, _ := h.store.GetRealmByName(realm.Name)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "realm already exists"})
		return
	}

	if realm.ID == "" {
		realm.ID = uuid.New().String()
	}
	if realm.AccessTokenLifespan <= 0 {
		realm.AccessTokenLifespan = 900
	}
	if realm.RefreshTokenLifespan <= 0 {
		realm.RefreshTokenLifespan = 604800
	}
	if realm.SSOSessionIdleTimeout <= 0 {
		realm.SSOSessionIdleTimeout = 1800
	}
	if realm.SSOSessionMaxLifespan <= 0 {
		realm.SSOSessionMaxLifespan = 36000
	}
	if realm.PasswordMinLength <= 0 {
		realm.PasswordMinLength = 8
	}

	if err := h.store.CreateRealm(&realm); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Seed defaults
	_ = h.store.SeedDefaultRoles(realm.ID)
	_ = h.store.SeedDefaultClientScopes(realm.ID)

	c.JSON(http.StatusCreated, realm)
}

func (h *EnhancedHandler) UpdateRealm(c *gin.Context) {
	id := c.Param("realmId")
	existing, err := h.store.GetRealm(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "realm not found"})
		return
	}
	if err := c.ShouldBindJSON(existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing.ID = id // preserve
	if err := h.store.UpdateRealm(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func (h *EnhancedHandler) DeleteRealm(c *gin.Context) {
	id := c.Param("realmId")
	if err := h.store.DeleteRealm(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// ═══════════════════════════════════════════════
// Group Endpoints
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) ListGroups(c *gin.Context) {
	realmID := c.Query("realm_id")
	groups, err := h.store.ListGroups(realmID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, groups)
}

func (h *EnhancedHandler) GetGroup(c *gin.Context) {
	id := c.Param("id")
	group, err := h.store.GetGroup(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if group == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}

	subGroups, _ := h.store.ListSubGroups(id)
	members, _ := h.store.GetGroupMembers(id)

	c.JSON(http.StatusOK, gin.H{
		"group":      group,
		"sub_groups": subGroups,
		"members":    members,
	})
}

func (h *EnhancedHandler) CreateGroup(c *gin.Context) {
	var group models.Group
	if err := c.ShouldBindJSON(&group); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if group.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if group.RealmID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "realm_id is required"})
		return
	}
	if err := h.store.CreateGroup(&group); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, group)
}

func (h *EnhancedHandler) UpdateGroup(c *gin.Context) {
	id := c.Param("id")
	existing, err := h.store.GetGroup(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	if err := c.ShouldBindJSON(existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing.ID = id
	if err := h.store.UpdateGroup(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func (h *EnhancedHandler) DeleteGroup(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteGroup(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (h *EnhancedHandler) AddGroupMember(c *gin.Context) {
	groupID := c.Param("id")
	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.AddUserToGroup(req.UserID, groupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "added"})
}

func (h *EnhancedHandler) RemoveGroupMember(c *gin.Context) {
	groupID := c.Param("id")
	userID := c.Param("userId")
	if err := h.store.RemoveUserFromGroup(userID, groupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

// ═══════════════════════════════════════════════
// Client Scope Endpoints
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) ListClientScopes(c *gin.Context) {
	realmID := c.Query("realm_id")
	scopes, err := h.store.ListClientScopes(realmID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, scopes)
}

func (h *EnhancedHandler) GetClientScope(c *gin.Context) {
	id := c.Param("id")
	scope, err := h.store.GetClientScope(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if scope == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client scope not found"})
		return
	}
	c.JSON(http.StatusOK, scope)
}

func (h *EnhancedHandler) CreateClientScope(c *gin.Context) {
	var scope models.ClientScope
	if err := c.ShouldBindJSON(&scope); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if scope.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if scope.RealmID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "realm_id is required"})
		return
	}
	if err := h.store.CreateClientScope(&scope); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, scope)
}

func (h *EnhancedHandler) UpdateClientScope(c *gin.Context) {
	id := c.Param("id")
	existing, err := h.store.GetClientScope(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client scope not found"})
		return
	}
	if err := c.ShouldBindJSON(existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing.ID = id
	if err := h.store.UpdateClientScope(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func (h *EnhancedHandler) DeleteClientScope(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteClientScope(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// ═══════════════════════════════════════════════
// Identity Provider Endpoints
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) ListPublicIdentityProviders(c *gin.Context) {
	realmID := strings.TrimSpace(c.Query("realm_id"))
	idps, err := h.store.ListIdentityProviders(realmID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	providers := make([]gin.H, 0, len(idps))
	for _, idp := range idps {
		if !idp.Enabled {
			continue
		}

		providers = append(providers, gin.H{
			"id":                idp.ID,
			"realm_id":          idp.RealmID,
			"alias":             idp.Alias,
			"display_name":      idp.DisplayName,
			"provider_type":     strings.ToLower(strings.TrimSpace(idp.ProviderType)),
			"enabled":           true,
			"authorization_url": strings.TrimSpace(idp.AuthorizationURL),
			"default_scopes":    strings.TrimSpace(idp.DefaultScopes),
			"client_id":         strings.TrimSpace(idp.ClientID),
			"issuer":            strings.TrimSpace(idp.Issuer),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"identity_providers": providers,
		"supported_provider_types": []string{
			"oidc",
			"saml",
			"github",
			"google",
			"ldap",
			"microsoft",
			"gitlab",
			"facebook",
		},
	})
}

func (h *EnhancedHandler) ListIdentityProviders(c *gin.Context) {
	realmID := c.Query("realm_id")
	idps, err := h.store.ListIdentityProviders(realmID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, idps)
}

func (h *EnhancedHandler) GetIdentityProvider(c *gin.Context) {
	id := c.Param("id")
	idp, err := h.store.GetIdentityProvider(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if idp == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "identity provider not found"})
		return
	}
	c.JSON(http.StatusOK, idp)
}

func (h *EnhancedHandler) CreateIdentityProvider(c *gin.Context) {
	var idp models.IdentityProvider
	if err := c.ShouldBindJSON(&idp); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if idp.Alias == "" || idp.ProviderType == "" || idp.RealmID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alias, provider_type, and realm_id are required"})
		return
	}
	validTypes := map[string]bool{"oidc": true, "saml": true, "github": true, "google": true, "ldap": true, "microsoft": true, "gitlab": true, "facebook": true}
	if !validTypes[strings.ToLower(idp.ProviderType)] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider_type; must be one of: oidc, saml, github, google, ldap, microsoft, gitlab, facebook"})
		return
	}
	existing, _ := h.store.GetIdentityProviderByAlias(idp.RealmID, idp.Alias)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "identity provider alias already exists in realm"})
		return
	}
	if err := h.store.CreateIdentityProvider(&idp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, idp)
}

func (h *EnhancedHandler) UpdateIdentityProvider(c *gin.Context) {
	id := c.Param("id")
	existing, err := h.store.GetIdentityProvider(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "identity provider not found"})
		return
	}
	if err := c.ShouldBindJSON(existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing.ID = id
	if err := h.store.UpdateIdentityProvider(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func (h *EnhancedHandler) DeleteIdentityProvider(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteIdentityProvider(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// ═══════════════════════════════════════════════
// SSO Session Endpoints
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) ListUserSessions(c *gin.Context) {
	userID := c.Param("userId")
	sessions, err := h.store.ListUserSSOSessions(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sessions)
}

func (h *EnhancedHandler) RevokeSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if err := h.store.RevokeSSOSession(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"revoked": sessionID})
}

func (h *EnhancedHandler) RevokeUserSessions(c *gin.Context) {
	userID := c.Param("userId")
	if err := h.store.RevokeUserSSOSessions(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "all sessions revoked"})
}

// ═══════════════════════════════════════════════
// Event / Audit Log Endpoints
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) ListEvents(c *gin.Context) {
	realmID := c.Query("realm_id")
	eventType := c.Query("type")
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)
	events, err := h.store.ListEvents(realmID, eventType, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, events)
}

func (h *EnhancedHandler) ListUserEvents(c *gin.Context) {
	userID := c.Param("userId")
	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.Atoi(limitStr)
	events, err := h.store.ListUserEvents(userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, events)
}

// ═══════════════════════════════════════════════
// User Attributes Endpoints
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) GetUserAttributes(c *gin.Context) {
	userID := c.Param("userId")
	attrs, err := h.store.GetUserAttributes(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, attrs)
}

func (h *EnhancedHandler) SetUserAttribute(c *gin.Context) {
	userID := c.Param("userId")
	var req struct {
		Key   string `json:"key" binding:"required"`
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.SetUserAttribute(userID, req.Key, req.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "set"})
}

func (h *EnhancedHandler) DeleteUserAttribute(c *gin.Context) {
	userID := c.Param("userId")
	key := c.Query("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key query param is required"})
		return
	}
	if err := h.store.DeleteUserAttribute(userID, key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ═══════════════════════════════════════════════
// User Groups (for a specific user)
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) GetUserGroups(c *gin.Context) {
	userID := c.Param("userId")
	groups, err := h.store.GetUserGroups(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, groups)
}

func (h *EnhancedHandler) AddUserToGroup(c *gin.Context) {
	userID := c.Param("userId")
	var req struct {
		GroupID string `json:"group_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.AddUserToGroup(userID, req.GroupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "added"})
}

func (h *EnhancedHandler) RemoveUserFromGroup(c *gin.Context) {
	userID := c.Param("userId")
	groupID := c.Param("groupId")
	if err := h.store.RemoveUserFromGroup(userID, groupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

// ═══════════════════════════════════════════════
// User Consents
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) GetUserConsents(c *gin.Context) {
	userID := c.Param("userId")
	consents, err := h.store.GetUserConsents(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, consents)
}

func (h *EnhancedHandler) RevokeUserConsent(c *gin.Context) {
	userID := c.Param("userId")
	clientID := c.Param("clientId")
	if err := h.store.RevokeConsent(userID, clientID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "revoked"})
}

// ═══════════════════════════════════════════════
// Required Actions
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) GetRequiredActions(c *gin.Context) {
	userID := c.Param("userId")
	actions, err := h.store.GetRequiredActions(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, actions)
}

func (h *EnhancedHandler) AddRequiredAction(c *gin.Context) {
	userID := c.Param("userId")
	var req struct {
		Action string `json:"action" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	validActions := map[string]bool{
		"VERIFY_EMAIL": true, "UPDATE_PASSWORD": true, "CONFIGURE_TOTP": true,
		"UPDATE_PROFILE": true, "TERMS_AND_CONDITIONS": true,
	}
	if !validActions[strings.ToUpper(req.Action)] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action"})
		return
	}
	if err := h.store.AddRequiredAction(userID, strings.ToUpper(req.Action)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "added"})
}

func (h *EnhancedHandler) RemoveRequiredAction(c *gin.Context) {
	userID := c.Param("userId")
	action := c.Param("action")
	if err := h.store.RemoveRequiredAction(userID, strings.ToUpper(action)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

// ═══════════════════════════════════════════════
// Enhanced Roles (PostgreSQL-backed, realm-scoped)
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) ListPGRoles(c *gin.Context) {
	realmID := c.Query("realm_id")
	roles, err := h.store.ListRoles(realmID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, roles)
}

func (h *EnhancedHandler) GetPGRole(c *gin.Context) {
	id := c.Param("id")
	role, err := h.store.GetRole(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if role == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	c.JSON(http.StatusOK, role)
}

func (h *EnhancedHandler) CreatePGRole(c *gin.Context) {
	var role models.Role
	if err := c.ShouldBindJSON(&role); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if role.Name == "" || role.RealmID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and realm_id are required"})
		return
	}
	role.Name = strings.ToLower(strings.TrimSpace(role.Name))
	if err := h.store.CreateRole(&role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, role)
}

func (h *EnhancedHandler) UpdatePGRole(c *gin.Context) {
	id := c.Param("id")
	existing, err := h.store.GetRole(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	if existing.System {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot modify system role"})
		return
	}
	if err := c.ShouldBindJSON(existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing.ID = id
	if err := h.store.UpdateRole(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func (h *EnhancedHandler) DeletePGRole(c *gin.Context) {
	id := c.Param("id")
	existing, err := h.store.GetRole(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil && existing.System {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete system role"})
		return
	}
	if err := h.store.DeleteRole(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// ═══════════════════════════════════════════════
// Role Bindings (PostgreSQL-backed)
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) CreateRoleBinding(c *gin.Context) {
	var rb models.RoleBinding
	if err := c.ShouldBindJSON(&rb); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if rb.RoleID == "" || rb.RealmID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role_id and realm_id are required"})
		return
	}
	if rb.UserID == "" && rb.GroupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id or group_id is required"})
		return
	}
	if err := h.store.CreateRoleBinding(&rb); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, rb)
}

func (h *EnhancedHandler) ListUserRoleBindings(c *gin.Context) {
	userID := c.Param("userId")
	bindings, err := h.store.ListUserRoleBindings(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, bindings)
}

func (h *EnhancedHandler) DeleteRoleBinding(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteRoleBinding(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (h *EnhancedHandler) GetEffectiveRoles(c *gin.Context) {
	userID := c.Param("userId")
	realmID := c.Query("realm_id")
	if realmID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "realm_id query param is required"})
		return
	}
	roles, err := h.store.GetEffectiveRoles(userID, realmID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_id": userID, "realm_id": realmID, "effective_roles": roles})
}

// ═══════════════════════════════════════════════
// Enhanced Clients (PostgreSQL-backed)
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) ListPGClients(c *gin.Context) {
	realmID := c.Query("realm_id")
	clients, err := h.store.ListClients(realmID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// mask secrets
	for i := range clients {
		if clients[i].Secret != "" {
			clients[i].Secret = "**********"
		}
	}
	c.JSON(http.StatusOK, clients)
}

func (h *EnhancedHandler) GetPGClient(c *gin.Context) {
	id := c.Param("id")
	client, err := h.store.GetClient(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}
	roles, _ := h.store.ListClientRoles(id)
	output := gin.H{
		"client": client,
		"roles":  roles,
	}
	c.JSON(http.StatusOK, output)
}

func (h *EnhancedHandler) CreatePGClient(c *gin.Context) {
	var client models.Client
	if err := c.ShouldBindJSON(&client); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if client.Name == "" || client.RealmID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and realm_id are required"})
		return
	}
	if client.RateLimitMaxCalls <= 0 {
		client.RateLimitMaxCalls = 500
	}
	if client.TokenValidityMinutes <= 0 {
		client.TokenValidityMinutes = 15
	}
	if client.Protocol == "" {
		client.Protocol = "openid-connect"
	}
	if err := h.store.CreateClient(&client); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, client)
}

func (h *EnhancedHandler) UpdatePGClient(c *gin.Context) {
	id := c.Param("id")
	existing, err := h.store.GetClient(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}
	if err := c.ShouldBindJSON(existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing.ID = id
	if err := h.store.UpdateClient(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func (h *EnhancedHandler) DeletePGClient(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteClient(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// ═══════════════════════════════════════════════
// Dashboard Summary
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) RealmDashboard(c *gin.Context) {
	realmID := c.Param("realmId")
	realm, err := h.store.GetRealm(realmID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if realm == nil {
		realm, _ = h.store.GetRealmByName(realmID)
	}
	if realm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "realm not found"})
		return
	}

	clients, _ := h.store.ListClients(realm.ID)
	roles, _ := h.store.ListRoles(realm.ID)
	groups, _ := h.store.ListGroups(realm.ID)
	scopes, _ := h.store.ListClientScopes(realm.ID)
	idps, _ := h.store.ListIdentityProviders(realm.ID)
	recentEvents, _ := h.store.ListEvents(realm.ID, "", 10)

	var counts struct {
		Users          int64
		ActiveSessions int64
	}
	h.store.DB().Model(&models.User{}).Where("realm_id = ?", realm.ID).Count(&counts.Users)
	h.store.DB().Model(&models.SSOSession{}).Where("realm_id = ? AND state = 'active'", realm.ID).Count(&counts.ActiveSessions)

	c.JSON(http.StatusOK, gin.H{
		"realm":                realm,
		"client_count":         len(clients),
		"role_count":           len(roles),
		"group_count":          len(groups),
		"scope_count":          len(scopes),
		"idp_count":            len(idps),
		"user_count":           counts.Users,
		"active_session_count": counts.ActiveSessions,
		"recent_events":        recentEvents,
	})
}

// ═══════════════════════════════════════════════
// Realm Key Info
// ═══════════════════════════════════════════════

func (h *EnhancedHandler) RealmInfo(c *gin.Context) {
	realmID := c.Param("realmId")
	realm, _ := h.store.GetRealm(realmID)
	if realm == nil {
		realm, _ = h.store.GetRealmByName(realmID)
	}
	if realm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "realm not found"})
		return
	}

	info := gin.H{
		"realm":         realm.Name,
		"display_name":  realm.DisplayName,
		"public_key":    "(available at JWKS endpoint)",
		"token_service": "/realms/" + realm.Name + "/protocol/openid-connect/token",
		"authorization": "/realms/" + realm.Name + "/protocol/openid-connect/auth",
		"jwks":          "/realms/" + realm.Name + "/protocol/openid-connect/certs",
		"discovery":     "/realms/" + realm.Name + "/.well-known/openid-configuration",
		"token_settings": gin.H{
			"access_token_lifespan":    realm.AccessTokenLifespan,
			"refresh_token_lifespan":   realm.RefreshTokenLifespan,
			"sso_session_idle_timeout": realm.SSOSessionIdleTimeout,
			"sso_session_max_lifespan": realm.SSOSessionMaxLifespan,
		},
		"login_settings": gin.H{
			"registration_allowed":     realm.RegistrationAllowed,
			"reset_password_allowed":   realm.ResetPasswordAllowed,
			"remember_me":              realm.RememberMe,
			"verify_email":             realm.VerifyEmail,
			"login_with_email":         realm.LoginWithEmail,
			"duplicate_emails_allowed": realm.DuplicateEmailsAllowed,
		},
		"security_settings": gin.H{
			"brute_force_protected":    realm.BruteForceProtected,
			"max_login_failures":       realm.MaxLoginFailures,
			"max_failure_wait_seconds": realm.MaxFailureWaitSeconds,
			"password_min_length":      realm.PasswordMinLength,
			"password_require_upper":   realm.PasswordRequireUpper,
			"password_require_digit":   realm.PasswordRequireDigit,
			"password_require_special": realm.PasswordRequireSpecial,
		},
	}
	c.JSON(http.StatusOK, info)
}

// unused import guard
var _ = json.Marshal
