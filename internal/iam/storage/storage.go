package storage

import (
	"example.com/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"example.com/axiomnizam/internal/iam/authn"
	"example.com/axiomnizam/internal/iam/authz"
	"example.com/axiomnizam/internal/iam/identity"
	"example.com/axiomnizam/internal/iam/oauth"
	clientv3 "go.etcd.io/etcd/client/v3"
	"gorm.io/gorm"
)

// ──────────────────────────────────────────────
// PostgreSQL repositories (users, roles)
// ──────────────────────────────────────────────

// IAMUser is the GORM model for the iam_users table.
type IAMUser struct {
	ID            string    `gorm:"primaryKey;type:varchar(36)"`
	Email         string    `gorm:"uniqueIndex;type:varchar(255);not null"`
	PasswordHash  string    `gorm:"type:varchar(255);not null"`
	DisplayName   string    `gorm:"type:varchar(255)"`
	Active        bool      `gorm:"default:true"`
	EmailVerified bool      `gorm:"default:false"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
}

func (IAMUser) TableName() string { return "iam_users" }

func toModel(u *IAMUser) *identity.User {
	if u == nil {
		return nil
	}
	return &identity.User{
		ID:            u.ID,
		Email:         u.Email,
		PasswordHash:  u.PasswordHash,
		DisplayName:   u.DisplayName,
		Active:        u.Active,
		EmailVerified: u.EmailVerified,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
	}
}

func fromModel(u *identity.User) *IAMUser {
	if u == nil {
		return nil
	}
	return &IAMUser{
		ID:            u.ID,
		Email:         u.Email,
		PasswordHash:  u.PasswordHash,
		DisplayName:   u.DisplayName,
		Active:        u.Active,
		EmailVerified: u.EmailVerified,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
	}
}

// PostgresUserRepository stores users in PostgreSQL via GORM.
type PostgresUserRepository struct {
	db *gorm.DB
}

// NewPostgresUserRepository creates the repository and ensures the table exists.
func NewPostgresUserRepository(db *gorm.DB) (*PostgresUserRepository, error) {
	if db == nil {
		return nil, errors.New("postgresql connection is required for IAM user storage")
	}
	if err := db.AutoMigrate(&IAMUser{}); err != nil {
		return nil, fmt.Errorf("iam_users migration: %w", err)
	}
	return &PostgresUserRepository{db: db}, nil
}

func (r *PostgresUserRepository) Create(user *identity.User) error {
	return r.db.Create(fromModel(user)).Error
}

func (r *PostgresUserRepository) GetByID(id string) (*identity.User, error) {
	var row IAMUser
	if err := r.db.Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toModel(&row), nil
}

func (r *PostgresUserRepository) GetByEmail(email string) (*identity.User, error) {
	var row IAMUser
	if err := r.db.Where("email = ?", strings.ToLower(strings.TrimSpace(email))).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toModel(&row), nil
}

// GetByLoginIdentifier resolves an explicit email or a unique email local-part.
func (r *PostgresUserRepository) GetByLoginIdentifier(identifier string) (*identity.User, error) {
	normalized := strings.ToLower(strings.TrimSpace(identifier))
	if normalized == "" {
		return nil, nil
	}

	if strings.Contains(normalized, "@") {
		return r.GetByEmail(normalized)
	}

	var rows []IAMUser
	if err := r.db.Where("split_part(email, '@', 1) = ?", normalized).Order("created_at DESC").Limit(2).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) > 1 {
		logging.Z().Info(fmt.Sprintf("⚠️  IAM: login identifier %q is ambiguous; use full email", normalized))
		return nil, nil
	}

	return toModel(&rows[0]), nil
}

func (r *PostgresUserRepository) Update(user *identity.User) error {
	return r.db.Save(fromModel(user)).Error
}

func (r *PostgresUserRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&IAMUser{}).Error
}

func (r *PostgresUserRepository) List() ([]*identity.User, error) {
	var rows []IAMUser
	if err := r.db.Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	users := make([]*identity.User, len(rows))
	for i := range rows {
		users[i] = toModel(&rows[i])
	}
	return users, nil
}

// ──────────────────────────────────────────────
// etcd repositories (codes, tokens, clients, roles, bindings, sessions)
// ──────────────────────────────────────────────

const (
	etcdTimeout      = 3 * time.Second
	prefixClients    = "iam:clients:"
	prefixCodes      = "iam:codes:"
	prefixRefresh    = "iam:refresh:"
	prefixRoles      = "iam:roles:"
	prefixBindings   = "iam:bindings:"
	prefixSessions   = "iam:sessions:"
	prefixRevokedJTI = "iam:revoked:"
)

// etcdStore wraps the etcd client for IAM storage.
type etcdStore struct {
	client *clientv3.Client
}

func newEtcdStore(c *clientv3.Client) *etcdStore {
	return &etcdStore{client: c}
}

func (s *etcdStore) put(key string, val []byte, ttl time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
	defer cancel()

	if ttl > 0 {
		lease, err := s.client.Grant(ctx, int64(ttl.Seconds()))
		if err != nil {
			return err
		}
		_, err = s.client.Put(ctx, key, string(val), clientv3.WithLease(lease.ID))
		return err
	}

	_, err := s.client.Put(ctx, key, string(val))
	return err
}

func (s *etcdStore) get(key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
	defer cancel()

	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, nil
	}
	return resp.Kvs[0].Value, nil
}

func (s *etcdStore) del(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
	defer cancel()
	_, err := s.client.Delete(ctx, key)
	return err
}

func (s *etcdStore) list(prefix string) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
	defer cancel()

	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	results := make([][]byte, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		results = append(results, kv.Value)
	}
	return results, nil
}

func (s *etcdStore) delPrefix(prefix string) error {
	ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
	defer cancel()
	_, err := s.client.Delete(ctx, prefix, clientv3.WithPrefix())
	return err
}

// ── OAuth Client Repository ──

// EtcdClientRepository stores OAuth clients in etcd.
type EtcdClientRepository struct {
	store iamBackend
}

func NewEtcdClientRepository(c *clientv3.Client) *EtcdClientRepository {
	return &EtcdClientRepository{store: newEtcdStore(c)}
}

func (r *EtcdClientRepository) CreateClient(client *oauth.OAuthClient) error {
	data, err := json.Marshal(client)
	if err != nil {
		return err
	}
	return r.store.put(prefixClients+client.ID, data, 0)
}

func (r *EtcdClientRepository) GetClient(clientID string) (*oauth.OAuthClient, error) {
	data, err := r.store.get(prefixClients + clientID)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var client oauth.OAuthClient
	if err := json.Unmarshal(data, &client); err != nil {
		return nil, err
	}
	return &client, nil
}

func (r *EtcdClientRepository) UpdateClient(client *oauth.OAuthClient) error {
	return r.CreateClient(client)
}

func (r *EtcdClientRepository) DeleteClient(clientID string) error {
	return r.store.del(prefixClients + clientID)
}

func (r *EtcdClientRepository) ListClients() ([]*oauth.OAuthClient, error) {
	entries, err := r.store.list(prefixClients)
	if err != nil {
		return nil, err
	}
	clients := make([]*oauth.OAuthClient, 0, len(entries))
	for _, data := range entries {
		var c oauth.OAuthClient
		if err := json.Unmarshal(data, &c); err != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  IAM: skipping corrupt client entry: %v", err))
			continue
		}
		clients = append(clients, &c)
	}
	return clients, nil
}

// ── Authorization Code Repository ──

// EtcdCodeRepository stores short-lived auth codes in etcd with TTL.
type EtcdCodeRepository struct {
	store iamBackend
}

func NewEtcdCodeRepository(c *clientv3.Client) *EtcdCodeRepository {
	return &EtcdCodeRepository{store: newEtcdStore(c)}
}

func (r *EtcdCodeRepository) StoreCode(code *oauth.AuthorizationCode) error {
	data, err := json.Marshal(code)
	if err != nil {
		return err
	}
	ttl := time.Until(code.ExpiresAt)
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return r.store.put(prefixCodes+code.Code, data, ttl)
}

func (r *EtcdCodeRepository) GetCode(code string) (*oauth.AuthorizationCode, error) {
	data, err := r.store.get(prefixCodes + code)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var ac oauth.AuthorizationCode
	if err := json.Unmarshal(data, &ac); err != nil {
		return nil, err
	}
	return &ac, nil
}

func (r *EtcdCodeRepository) InvalidateCode(code string) error {
	return r.store.del(prefixCodes + code)
}

// ── Refresh Token Repository ──

// EtcdRefreshTokenRepository manages refresh tokens in etcd.
type EtcdRefreshTokenRepository struct {
	store iamBackend
}

func NewEtcdRefreshTokenRepository(c *clientv3.Client) *EtcdRefreshTokenRepository {
	return &EtcdRefreshTokenRepository{store: newEtcdStore(c)}
}

func (r *EtcdRefreshTokenRepository) StoreRefreshToken(rt *oauth.RefreshTokenRecord) error {
	data, err := json.Marshal(rt)
	if err != nil {
		return err
	}
	ttl := time.Until(rt.ExpiresAt)
	if ttl <= 0 {
		return errors.New("refresh token already expired")
	}
	return r.store.put(prefixRefresh+rt.ID, data, ttl)
}

func (r *EtcdRefreshTokenRepository) GetRefreshToken(jti string) (*oauth.RefreshTokenRecord, error) {
	data, err := r.store.get(prefixRefresh + jti)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var rt oauth.RefreshTokenRecord
	if err := json.Unmarshal(data, &rt); err != nil {
		return nil, err
	}
	return &rt, nil
}

func (r *EtcdRefreshTokenRepository) RevokeRefreshToken(jti string) error {
	rt, err := r.GetRefreshToken(jti)
	if err != nil || rt == nil {
		return err
	}
	rt.Revoked = true
	data, _ := json.Marshal(rt)
	return r.store.put(prefixRefresh+jti, data, time.Until(rt.ExpiresAt))
}

func (r *EtcdRefreshTokenRepository) RevokeAllForUser(userID string) error {
	entries, err := r.store.list(prefixRefresh)
	if err != nil {
		return err
	}
	for _, data := range entries {
		var rt oauth.RefreshTokenRecord
		if err := json.Unmarshal(data, &rt); err != nil {
			continue
		}
		if rt.UserID == userID && !rt.Revoked {
			rt.Revoked = true
			updated, _ := json.Marshal(rt)
			_ = r.store.put(prefixRefresh+rt.ID, updated, time.Until(rt.ExpiresAt))
		}
	}
	return nil
}

func (r *EtcdRefreshTokenRepository) RevokeBySessionID(sessionID string) error {
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID == "" {
		return nil
	}

	entries, err := r.store.list(prefixRefresh)
	if err != nil {
		return err
	}

	for _, data := range entries {
		var rt oauth.RefreshTokenRecord
		if err := json.Unmarshal(data, &rt); err != nil {
			continue
		}
		if strings.TrimSpace(rt.SessionID) == trimmedSessionID && !rt.Revoked {
			rt.Revoked = true
			updated, _ := json.Marshal(rt)
			_ = r.store.put(prefixRefresh+rt.ID, updated, time.Until(rt.ExpiresAt))
		}
	}

	return nil
}

// ── Role Repository (etcd) ──

// EtcdRoleRepository persists IAM roles in etcd.
type EtcdRoleRepository struct {
	mu    sync.RWMutex
	store iamBackend
}

func NewEtcdRoleRepository(c *clientv3.Client) *EtcdRoleRepository {
	return &EtcdRoleRepository{store: newEtcdStore(c)}
}

func (r *EtcdRoleRepository) CreateRole(role *authz.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, err := json.Marshal(role)
	if err != nil {
		return err
	}
	return r.store.put(prefixRoles+role.ID, data, 0)
}

func (r *EtcdRoleRepository) GetRole(id string) (*authz.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	data, err := r.store.get(prefixRoles + id)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var role authz.Role
	if err := json.Unmarshal(data, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *EtcdRoleRepository) GetRoleByName(name string) (*authz.Role, error) {
	roles, err := r.ListRoles()
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, role := range roles {
		if strings.ToLower(role.Name) == lower {
			return role, nil
		}
	}
	return nil, nil
}

func (r *EtcdRoleRepository) ListRoles() ([]*authz.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries, err := r.store.list(prefixRoles)
	if err != nil {
		return nil, err
	}
	roles := make([]*authz.Role, 0, len(entries))
	for _, data := range entries {
		var role authz.Role
		if err := json.Unmarshal(data, &role); err != nil {
			continue
		}
		roles = append(roles, &role)
	}
	return roles, nil
}

func (r *EtcdRoleRepository) UpdateRole(role *authz.Role) error {
	return r.CreateRole(role)
}

func (r *EtcdRoleRepository) DeleteRole(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.store.del(prefixRoles + id)
}

// ── Role Binding Repository (etcd) ──

// EtcdRoleBindingRepository stores user-role bindings in etcd.
type EtcdRoleBindingRepository struct {
	mu    sync.RWMutex
	store iamBackend
}

func NewEtcdRoleBindingRepository(c *clientv3.Client) *EtcdRoleBindingRepository {
	return &EtcdRoleBindingRepository{store: newEtcdStore(c)}
}

func (r *EtcdRoleBindingRepository) CreateBinding(b *authz.RoleBinding) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	return r.store.put(prefixBindings+b.ID, data, 0)
}

func (r *EtcdRoleBindingRepository) DeleteBinding(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.store.del(prefixBindings + id)
}

func (r *EtcdRoleBindingRepository) ListBindingsForUser(userID string) ([]*authz.RoleBinding, error) {
	all, err := r.ListAllBindings()
	if err != nil {
		return nil, err
	}
	filtered := make([]*authz.RoleBinding, 0)
	for _, b := range all {
		if b.UserID == userID {
			filtered = append(filtered, b)
		}
	}
	return filtered, nil
}

func (r *EtcdRoleBindingRepository) ListAllBindings() ([]*authz.RoleBinding, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries, err := r.store.list(prefixBindings)
	if err != nil {
		return nil, err
	}
	bindings := make([]*authz.RoleBinding, 0, len(entries))
	for _, data := range entries {
		var b authz.RoleBinding
		if err := json.Unmarshal(data, &b); err != nil {
			continue
		}
		bindings = append(bindings, &b)
	}
	return bindings, nil
}

// ── Session Repository (etcd) ──

// EtcdSessionRepository stores sessions with TTL.
type EtcdSessionRepository struct {
	store iamBackend
}

func NewEtcdSessionRepository(c *clientv3.Client) *EtcdSessionRepository {
	return &EtcdSessionRepository{store: newEtcdStore(c)}
}

func (r *EtcdSessionRepository) Create(s *authn.Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	ttl := time.Until(s.ExpiresAt)
	if ttl <= 0 {
		ttl = time.Hour
	}
	return r.store.put(prefixSessions+s.ID, data, ttl)
}

func (r *EtcdSessionRepository) GetByID(id string) (*authn.Session, error) {
	data, err := r.store.get(prefixSessions + id)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var s authn.Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *EtcdSessionRepository) Revoke(sessionID string) error {
	return r.store.del(prefixSessions + sessionID)
}

// Touch updates only the LastAccessAt timestamp of a session (Phase 11).
// Returns nil if the session doesn't exist (idempotent).
func (r *EtcdSessionRepository) Touch(sessionID string) error {
	data, err := r.store.get(prefixSessions + sessionID)
	if err != nil {
		return err
	}
	if data == nil {
		return nil
	}
	var s authn.Session
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	s.LastAccessAt = time.Now().UTC()
	updated, err := json.Marshal(s)
	if err != nil {
		return err
	}
	ttl := time.Until(s.ExpiresAt)
	if ttl <= 0 {
		ttl = time.Hour
	}
	return r.store.put(prefixSessions+sessionID, updated, ttl)
}

func (r *EtcdSessionRepository) RevokeByUserID(userID string) error {
	entries, err := r.store.list(prefixSessions)
	if err != nil {
		return err
	}
	for _, data := range entries {
		var s authn.Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		if s.UserID == userID {
			_ = r.store.del(prefixSessions + s.ID)
		}
	}
	return nil
}

// ── Revoked Token JTI Registry ──

// EtcdRevokedTokenStore tracks revoked JTIs to prevent replay.
type EtcdRevokedTokenStore struct {
	store iamBackend
}

func NewEtcdRevokedTokenStore(c *clientv3.Client) *EtcdRevokedTokenStore {
	return &EtcdRevokedTokenStore{store: newEtcdStore(c)}
}

// Revoke marks a JTI as revoked with a TTL matching the token's remaining life.
func (r *EtcdRevokedTokenStore) Revoke(jti string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return r.store.put(prefixRevokedJTI+jti, []byte("1"), ttl)
}

// IsRevoked checks if a JTI was explicitly revoked.
func (r *EtcdRevokedTokenStore) IsRevoked(jti string) (bool, error) {
	data, err := r.store.get(prefixRevokedJTI + jti)
	if err != nil {
		return false, err
	}
	return data != nil, nil
}

// ── Bootstrap ──

// SeedSystemRoles writes the default IAM roles if they don't exist.
func SeedSystemRoles(repo *EtcdRoleRepository) error {
	for _, role := range authz.DefaultSystemRoles() {
		existing, _ := repo.GetRole(role.ID)
		if existing == nil {
			if err := repo.CreateRole(role); err != nil {
				return fmt.Errorf("seeding role %s: %w", role.Name, err)
			}
			logging.Z().Info(fmt.Sprintf("✅ IAM: seeded system role: %s", role.Name))
		}
	}
	return nil
}
