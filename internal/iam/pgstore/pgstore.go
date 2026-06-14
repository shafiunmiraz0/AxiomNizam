package pgstore

import (
	"errors"
	"fmt"
	"strings"
	"time"

	iamconfig "axiomnizam.bitbd.net/axiomnizam/internal/iam/config"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Store bundles all PostgreSQL-backed IAM repositories.
type Store struct {
	db *gorm.DB
}

// New creates a new IAM PostgreSQL store and auto-migrates all tables.
func New(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("postgresql connection is required")
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("iam pg migration: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	return s.db.AutoMigrate(
		&models.Realm{},
		&models.Client{},
		&models.User{},
		&models.UserAttribute{},
		&models.Group{},
		&models.GroupMembership{},
		&models.Role{},
		&models.RoleBinding{},
		&models.ClientScope{},
		&models.IdentityProvider{},
		&models.SSOSession{},
		&models.ClientSession{},
		&models.Event{},
		&models.RequiredAction{},
		&models.Credential{},
		&models.UserConsent{},
	)
}

func newID() string { return uuid.New().String() }

// ═══════════════════════════════════════════════
// Realm Repository
// ═══════════════════════════════════════════════

func (s *Store) CreateRealm(r *models.Realm) error {
	if r.ID == "" {
		r.ID = newID()
	}
	r.Name = strings.ToLower(strings.TrimSpace(r.Name))
	return s.db.Create(r).Error
}

func (s *Store) GetRealm(id string) (*models.Realm, error) {
	var r models.Realm
	if err := s.db.Where("id = ?", id).First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) GetRealmByName(name string) (*models.Realm, error) {
	var r models.Realm
	if err := s.db.Where("name = ?", strings.ToLower(strings.TrimSpace(name))).First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) ListRealms() ([]models.Realm, error) {
	var realms []models.Realm
	if err := s.db.Order("name ASC").Find(&realms).Error; err != nil {
		return nil, err
	}
	return realms, nil
}

func (s *Store) UpdateRealm(r *models.Realm) error {
	return s.db.Save(r).Error
}

func (s *Store) DeleteRealm(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.Realm{}).Error
}

// SeedDefaultRealm ensures the default realm exists.
func (s *Store) SeedDefaultRealm() (*models.Realm, error) {
	existing, err := s.GetRealmByName(iamconfig.DefaultRealm)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	r := &models.Realm{
		ID:                    newID(),
		Name:                  iamconfig.DefaultRealm,
		DisplayName:           "AxiomNizam",
		Enabled:               true,
		LoginWithEmail:        true,
		ResetPasswordAllowed:  true,
		RememberMe:            true,
		DefaultRoles:          []string{"user"},
		AccessTokenLifespan:   900,
		RefreshTokenLifespan:  604800,
		SSOSessionIdleTimeout: 1800,
		SSOSessionMaxLifespan: 36000,
		BruteForceProtected:   true,
		MaxLoginFailures:      30,
		PasswordMinLength:     8,
		PasswordRequireUpper:  true,
		PasswordRequireDigit:  true,
	}
	if err := s.db.Create(r).Error; err != nil {
		return nil, err
	}
	return r, nil
}

// ═══════════════════════════════════════════════
// Client Repository (PostgreSQL)
// ═══════════════════════════════════════════════

func (s *Store) CreateClient(c *models.Client) error {
	if c.ID == "" {
		c.ID = newID()
	}
	return s.db.Create(c).Error
}

func (s *Store) GetClient(id string) (*models.Client, error) {
	var c models.Client
	if err := s.db.Where("id = ?", id).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (s *Store) ListClients(realmID string) ([]models.Client, error) {
	var clients []models.Client
	q := s.db.Order("name ASC")
	if realmID != "" {
		q = q.Where("realm_id = ?", realmID)
	}
	if err := q.Find(&clients).Error; err != nil {
		return nil, err
	}
	return clients, nil
}

func (s *Store) UpdateClient(c *models.Client) error {
	return s.db.Save(c).Error
}

func (s *Store) DeleteClient(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.Client{}).Error
}

// ═══════════════════════════════════════════════
// Group Repository
// ═══════════════════════════════════════════════

func (s *Store) CreateGroup(g *models.Group) error {
	if g.ID == "" {
		g.ID = newID()
	}
	if g.Path == "" {
		if g.ParentID != "" {
			parent, _ := s.GetGroup(g.ParentID)
			if parent != nil {
				g.Path = parent.Path + "/" + g.Name
			} else {
				g.Path = "/" + g.Name
			}
		} else {
			g.Path = "/" + g.Name
		}
	}
	return s.db.Create(g).Error
}

func (s *Store) GetGroup(id string) (*models.Group, error) {
	var g models.Group
	if err := s.db.Where("id = ?", id).First(&g).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &g, nil
}

func (s *Store) ListGroups(realmID string) ([]models.Group, error) {
	var groups []models.Group
	q := s.db.Order("path ASC")
	if realmID != "" {
		q = q.Where("realm_id = ?", realmID)
	}
	if err := q.Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *Store) ListSubGroups(parentID string) ([]models.Group, error) {
	var groups []models.Group
	if err := s.db.Where("parent_id = ?", parentID).Order("name ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *Store) UpdateGroup(g *models.Group) error {
	return s.db.Save(g).Error
}

func (s *Store) DeleteGroup(id string) error {
	// Delete memberships first
	s.db.Where("group_id = ?", id).Delete(&models.GroupMembership{})
	// Delete child groups recursively
	var children []models.Group
	s.db.Where("parent_id = ?", id).Find(&children)
	for _, child := range children {
		_ = s.DeleteGroup(child.ID)
	}
	return s.db.Where("id = ?", id).Delete(&models.Group{}).Error
}

// ═══════════════════════════════════════════════
// Group Membership
// ═══════════════════════════════════════════════

func (s *Store) AddUserToGroup(userID, groupID string) error {
	// Check duplicate
	var count int64
	s.db.Model(&models.GroupMembership{}).Where("user_id = ? AND group_id = ?", userID, groupID).Count(&count)
	if count > 0 {
		return nil // already a member
	}
	return s.db.Create(&models.GroupMembership{
		ID:      newID(),
		UserID:  userID,
		GroupID: groupID,
	}).Error
}

func (s *Store) RemoveUserFromGroup(userID, groupID string) error {
	return s.db.Where("user_id = ? AND group_id = ?", userID, groupID).Delete(&models.GroupMembership{}).Error
}

func (s *Store) GetUserGroups(userID string) ([]models.Group, error) {
	var groups []models.Group
	if err := s.db.Raw(`
		SELECT g.* FROM iam_groups g
		JOIN iam_group_memberships gm ON gm.group_id = g.id
		WHERE gm.user_id = ? ORDER BY g.path ASC
	`, userID).Scan(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *Store) GetGroupMembers(groupID string) ([]models.User, error) {
	var users []models.User
	if err := s.db.Raw(`
		SELECT u.* FROM iam_users_v2 u
		JOIN iam_group_memberships gm ON gm.user_id = u.id
		WHERE gm.group_id = ? ORDER BY u.email ASC
	`, groupID).Scan(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// ═══════════════════════════════════════════════
// Role Repository (PostgreSQL)
// ═══════════════════════════════════════════════

func (s *Store) CreateRole(r *models.Role) error {
	if r.ID == "" {
		r.ID = newID()
	}
	return s.db.Create(r).Error
}

func (s *Store) GetRole(id string) (*models.Role, error) {
	var r models.Role
	if err := s.db.Where("id = ?", id).First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) GetRoleByName(realmID, name string) (*models.Role, error) {
	var r models.Role
	q := s.db.Where("name = ?", strings.ToLower(strings.TrimSpace(name)))
	if realmID != "" {
		q = q.Where("realm_id = ?", realmID)
	}
	q = q.Where("client_id = '' OR client_id IS NULL") // realm roles only
	if err := q.First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) ListRoles(realmID string) ([]models.Role, error) {
	var roles []models.Role
	q := s.db.Order("name ASC")
	if realmID != "" {
		q = q.Where("realm_id = ?", realmID)
	}
	if err := q.Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

func (s *Store) ListClientRoles(clientID string) ([]models.Role, error) {
	var roles []models.Role
	if err := s.db.Where("client_id = ?", clientID).Order("name ASC").Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

func (s *Store) UpdateRole(r *models.Role) error {
	return s.db.Save(r).Error
}

func (s *Store) DeleteRole(id string) error {
	// Remove bindings first
	s.db.Where("role_id = ?", id).Delete(&models.RoleBinding{})
	return s.db.Where("id = ?", id).Delete(&models.Role{}).Error
}

// ═══════════════════════════════════════════════
// Role Binding Repository (PostgreSQL)
// ═══════════════════════════════════════════════

func (s *Store) CreateRoleBinding(rb *models.RoleBinding) error {
	if rb.ID == "" {
		rb.ID = newID()
	}
	// Prevent duplicates
	var count int64
	q := s.db.Model(&models.RoleBinding{}).Where("role_id = ? AND realm_id = ?", rb.RoleID, rb.RealmID)
	if rb.UserID != "" {
		q = q.Where("user_id = ?", rb.UserID)
	}
	if rb.GroupID != "" {
		q = q.Where("group_id = ?", rb.GroupID)
	}
	q.Count(&count)
	if count > 0 {
		return nil // already bound
	}
	return s.db.Create(rb).Error
}

func (s *Store) ListUserRoleBindings(userID string) ([]models.RoleBinding, error) {
	var bindings []models.RoleBinding
	if err := s.db.Where("user_id = ?", userID).Find(&bindings).Error; err != nil {
		return nil, err
	}
	return bindings, nil
}

func (s *Store) ListGroupRoleBindings(groupID string) ([]models.RoleBinding, error) {
	var bindings []models.RoleBinding
	if err := s.db.Where("group_id = ?", groupID).Find(&bindings).Error; err != nil {
		return nil, err
	}
	return bindings, nil
}

func (s *Store) DeleteRoleBinding(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.RoleBinding{}).Error
}

// GetEffectiveRoles returns all role names for a user (direct + group-inherited).
func (s *Store) GetEffectiveRoles(userID, realmID string) ([]string, error) {
	var roleNames []string
	if err := s.db.Raw(`
		SELECT DISTINCT r.name FROM iam_roles_v2 r
		JOIN iam_role_bindings_v2 rb ON rb.role_id = r.id
		WHERE rb.realm_id = ? AND (
			rb.user_id = ?
			OR rb.group_id IN (
				SELECT gm.group_id FROM iam_group_memberships gm WHERE gm.user_id = ?
			)
		)
	`, realmID, userID, userID).Scan(&roleNames).Error; err != nil {
		return nil, err
	}
	return roleNames, nil
}

// ═══════════════════════════════════════════════
// Client Scope Repository
// ═══════════════════════════════════════════════

func (s *Store) CreateClientScope(cs *models.ClientScope) error {
	if cs.ID == "" {
		cs.ID = newID()
	}
	return s.db.Create(cs).Error
}

func (s *Store) GetClientScope(id string) (*models.ClientScope, error) {
	var cs models.ClientScope
	if err := s.db.Where("id = ?", id).First(&cs).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &cs, nil
}

func (s *Store) ListClientScopes(realmID string) ([]models.ClientScope, error) {
	var scopes []models.ClientScope
	q := s.db.Order("name ASC")
	if realmID != "" {
		q = q.Where("realm_id = ?", realmID)
	}
	if err := q.Find(&scopes).Error; err != nil {
		return nil, err
	}
	return scopes, nil
}

func (s *Store) UpdateClientScope(cs *models.ClientScope) error {
	return s.db.Save(cs).Error
}

func (s *Store) DeleteClientScope(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.ClientScope{}).Error
}

// ═══════════════════════════════════════════════
// Identity Provider Repository
// ═══════════════════════════════════════════════

func (s *Store) CreateIdentityProvider(idp *models.IdentityProvider) error {
	if idp.ID == "" {
		idp.ID = newID()
	}
	return s.db.Create(idp).Error
}

func (s *Store) GetIdentityProvider(id string) (*models.IdentityProvider, error) {
	var idp models.IdentityProvider
	if err := s.db.Where("id = ?", id).First(&idp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &idp, nil
}

func (s *Store) GetIdentityProviderByAlias(realmID, alias string) (*models.IdentityProvider, error) {
	var idp models.IdentityProvider
	if err := s.db.Where("realm_id = ? AND alias = ?", realmID, alias).First(&idp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &idp, nil
}

func (s *Store) ListIdentityProviders(realmID string) ([]models.IdentityProvider, error) {
	var idps []models.IdentityProvider
	q := s.db.Order("alias ASC")
	if realmID != "" {
		q = q.Where("realm_id = ?", realmID)
	}
	if err := q.Find(&idps).Error; err != nil {
		return nil, err
	}
	return idps, nil
}

func (s *Store) UpdateIdentityProvider(idp *models.IdentityProvider) error {
	return s.db.Save(idp).Error
}

func (s *Store) DeleteIdentityProvider(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.IdentityProvider{}).Error
}

// ═══════════════════════════════════════════════
// SSO Session Repository
// ═══════════════════════════════════════════════

func (s *Store) CreateSSOSession(sess *models.SSOSession) error {
	if sess.ID == "" {
		sess.ID = uuid.New().String()
	}
	return s.db.Create(sess).Error
}

func (s *Store) GetSSOSession(id string) (*models.SSOSession, error) {
	var sess models.SSOSession
	if err := s.db.Where("id = ?", id).First(&sess).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &sess, nil
}

func (s *Store) ListUserSSOSessions(userID string) ([]models.SSOSession, error) {
	var sessions []models.SSOSession
	if err := s.db.Where("user_id = ? AND state = 'active'", userID).Order("started_at DESC").Find(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}

func (s *Store) UpdateSSOSession(sess *models.SSOSession) error {
	return s.db.Save(sess).Error
}

func (s *Store) RevokeSSOSession(id string) error {
	return s.db.Model(&models.SSOSession{}).Where("id = ?", id).Update("state", "revoked").Error
}

func (s *Store) RevokeUserSSOSessions(userID string) error {
	return s.db.Model(&models.SSOSession{}).Where("user_id = ? AND state = 'active'", userID).Update("state", "revoked").Error
}

func (s *Store) CleanupExpiredSessions() error {
	return s.db.Where("expires_at < ? AND state = 'active'", time.Now().UTC()).Model(&models.SSOSession{}).Update("state", "expired").Error
}

// ═══════════════════════════════════════════════
// Event Repository
// ═══════════════════════════════════════════════

func (s *Store) RecordEvent(evt *models.Event) error {
	if evt.ID == "" {
		evt.ID = newID()
	}
	return s.db.Create(evt).Error
}

func (s *Store) ListEvents(realmID string, eventType string, limit int) ([]models.Event, error) {
	var events []models.Event
	q := s.db.Order("created_at DESC")
	if realmID != "" {
		q = q.Where("realm_id = ?", realmID)
	}
	if eventType != "" {
		q = q.Where("type = ?", eventType)
	}
	if limit <= 0 {
		limit = 100
	}
	if err := q.Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) ListUserEvents(userID string, limit int) ([]models.Event, error) {
	var events []models.Event
	if limit <= 0 {
		limit = 50
	}
	if err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) CleanupOldEvents(olderThan time.Duration) error {
	cutoff := time.Now().UTC().Add(-olderThan)
	return s.db.Where("created_at < ?", cutoff).Delete(&models.Event{}).Error
}

// ═══════════════════════════════════════════════
// User Attribute Repository
// ═══════════════════════════════════════════════

func (s *Store) SetUserAttribute(userID, key, value string) error {
	var existing models.UserAttribute
	err := s.db.Where("user_id = ? AND key = ?", userID, key).First(&existing).Error
	if err == nil {
		existing.Value = value
		return s.db.Save(&existing).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.Create(&models.UserAttribute{
			ID:     newID(),
			UserID: userID,
			Key:    key,
			Value:  value,
		}).Error
	}
	return err
}

func (s *Store) GetUserAttributes(userID string) ([]models.UserAttribute, error) {
	var attrs []models.UserAttribute
	if err := s.db.Where("user_id = ?", userID).Order("key ASC").Find(&attrs).Error; err != nil {
		return nil, err
	}
	return attrs, nil
}

func (s *Store) DeleteUserAttribute(userID, key string) error {
	return s.db.Where("user_id = ? AND key = ?", userID, key).Delete(&models.UserAttribute{}).Error
}

// ═══════════════════════════════════════════════
// User Consent Repository
// ═══════════════════════════════════════════════

func (s *Store) CreateOrUpdateConsent(consent *models.UserConsent) error {
	var existing models.UserConsent
	err := s.db.Where("user_id = ? AND client_id = ? AND realm_id = ?", consent.UserID, consent.ClientID, consent.RealmID).First(&existing).Error
	if err == nil {
		existing.GrantedScopes = consent.GrantedScopes
		return s.db.Save(&existing).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if consent.ID == "" {
			consent.ID = newID()
		}
		return s.db.Create(consent).Error
	}
	return err
}

func (s *Store) GetUserConsents(userID string) ([]models.UserConsent, error) {
	var consents []models.UserConsent
	if err := s.db.Where("user_id = ?", userID).Find(&consents).Error; err != nil {
		return nil, err
	}
	return consents, nil
}

func (s *Store) RevokeConsent(userID, clientID string) error {
	return s.db.Where("user_id = ? AND client_id = ?", userID, clientID).Delete(&models.UserConsent{}).Error
}

// ═══════════════════════════════════════════════
// Required Actions
// ═══════════════════════════════════════════════

func (s *Store) AddRequiredAction(userID, action string) error {
	var count int64
	s.db.Model(&models.RequiredAction{}).Where("user_id = ? AND action = ?", userID, action).Count(&count)
	if count > 0 {
		return nil
	}
	return s.db.Create(&models.RequiredAction{
		ID:     newID(),
		UserID: userID,
		Action: action,
	}).Error
}

func (s *Store) GetRequiredActions(userID string) ([]models.RequiredAction, error) {
	var actions []models.RequiredAction
	if err := s.db.Where("user_id = ?", userID).Order("priority ASC").Find(&actions).Error; err != nil {
		return nil, err
	}
	return actions, nil
}

func (s *Store) RemoveRequiredAction(userID, action string) error {
	return s.db.Where("user_id = ? AND action = ?", userID, action).Delete(&models.RequiredAction{}).Error
}

// ═══════════════════════════════════════════════
// Credential Repository
// ═══════════════════════════════════════════════

func (s *Store) AddCredential(c *models.Credential) error {
	if c.ID == "" {
		c.ID = newID()
	}
	return s.db.Create(c).Error
}

func (s *Store) GetCredentials(userID, credType string) ([]models.Credential, error) {
	var creds []models.Credential
	q := s.db.Where("user_id = ?", userID)
	if credType != "" {
		q = q.Where("type = ?", credType)
	}
	if err := q.Order("priority ASC").Find(&creds).Error; err != nil {
		return nil, err
	}
	return creds, nil
}

func (s *Store) DeleteCredential(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.Credential{}).Error
}

// ═══════════════════════════════════════════════
// Seed Helpers
// ═══════════════════════════════════════════════

// SeedDefaultClientScopes creates built-in OIDC scopes for a realm.
func (s *Store) SeedDefaultClientScopes(realmID string) error {
	defaults := []models.ClientScope{
		{Name: "openid", Description: "OpenID Connect scope", Protocol: "openid-connect", BuiltIn: true, AddToIDToken: true, AddToAccessToken: true},
		{Name: "profile", Description: "User profile information", Protocol: "openid-connect", ClaimName: "profile", BuiltIn: true, AddToIDToken: true, AddToUserInfo: true},
		{Name: "email", Description: "User email address", Protocol: "openid-connect", ClaimName: "email", BuiltIn: true, AddToIDToken: true, AddToUserInfo: true},
		{Name: "roles", Description: "User roles", Protocol: "openid-connect", ClaimName: "roles", ClaimType: "JSON", BuiltIn: true, AddToIDToken: true, AddToAccessToken: true},
		{Name: "groups", Description: "User groups", Protocol: "openid-connect", ClaimName: "groups", ClaimType: "JSON", BuiltIn: true, AddToIDToken: false, AddToAccessToken: true},
		{Name: "phone", Description: "User phone number", Protocol: "openid-connect", ClaimName: "phone_number", BuiltIn: true, AddToIDToken: false, AddToUserInfo: true},
		{Name: "address", Description: "User address", Protocol: "openid-connect", ClaimName: "address", BuiltIn: true, AddToIDToken: false, AddToUserInfo: true},
		{Name: "offline_access", Description: "Offline access (long-lived refresh tokens)", Protocol: "openid-connect", BuiltIn: true},
	}

	for _, cs := range defaults {
		existing, _ := s.getClientScopeByName(realmID, cs.Name)
		if existing != nil {
			continue
		}
		cs.ID = newID()
		cs.RealmID = realmID
		if err := s.db.Create(&cs).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) getClientScopeByName(realmID, name string) (*models.ClientScope, error) {
	var cs models.ClientScope
	if err := s.db.Where("realm_id = ? AND name = ?", realmID, name).First(&cs).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &cs, nil
}

// SeedDefaultRoles seeds system-level realm roles.
func (s *Store) SeedDefaultRoles(realmID string) error {
	systemRoles := []struct {
		Name        string
		Description string
		Permissions []string
	}{
		{"sysadmin", "Full system administrator access", []string{"*:*"}},
		{"admin", "Administrative access", []string{"users:read", "users:create", "users:update", "clients:read", "clients:create", "clients:update", "roles:read"}},
		{"manager", "Manager-level access", []string{"users:read", "jobs:read", "jobs:execute", "datasources:read"}},
		{"user", "Standard user access", []string{"profile:read", "profile:update"}},
	}

	for _, sr := range systemRoles {
		existing, _ := s.GetRoleByName(realmID, sr.Name)
		if existing != nil {
			continue
		}
		role := &models.Role{
			ID:          newID(),
			RealmID:     realmID,
			Name:        sr.Name,
			Description: sr.Description,
			Permissions: sr.Permissions,
			System:      true,
		}
		if err := s.db.Create(role).Error; err != nil {
			return err
		}
	}
	return nil
}

// DB exposes the underlying gorm.DB connection for external use.
func (s *Store) DB() *gorm.DB {
	return s.db
}
