package iam

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/bootstrapsecrets"
	iamconfig "axiomnizam.bitbd.net/axiomnizam/internal/iam/config"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/admin"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/authn"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/authz"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/identity"
	iammw "axiomnizam.bitbd.net/axiomnizam/internal/iam/middleware"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/pgstore"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/storage"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/token"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"github.com/gin-gonic/gin"
	clientv3 "go.etcd.io/etcd/client/v3"
	"gorm.io/gorm"
)

// System holds the fully initialised IAM system and exposes the router registration.
type System struct {
	Issuer        *token.Issuer
	Authorizer    *authz.Authorizer
	Authenticator *authn.Authenticator
	AdminHandler  *admin.Handler

	// Enhanced PostgreSQL-backed store (realms, groups, scopes, events, IdPs)
	PGStore         *pgstore.Store
	EnhancedHandler *admin.EnhancedHandler

	// Repositories (exported for external callers that need direct access)
	Users        *storage.PostgresUserRepository
	Clients      *storage.EtcdClientRepository
	Roles        *storage.EtcdRoleRepository
	Bindings     *storage.EtcdRoleBindingRepository
	Sessions     *storage.EtcdSessionRepository
	RefreshRepo  *storage.EtcdRefreshTokenRepository
	CodeRepo     *storage.EtcdCodeRepository
	RevokedStore *storage.EtcdRevokedTokenStore
}

// Config re-exports the config type from the config subpackage.
type Config = iamconfig.Config

const (
	issuerPrivateKeyStoreKey = "iam-rsa-private-key-pem"
	issuerPrivateKeyEtcdKey  = "iam:bootstrap:rsa-private-key-pem"
)

func ensureSharedIssuerPrivateKey(pg *gorm.DB, etcd *clientv3.Client) error {
	configuredInline := strings.TrimSpace(os.Getenv("IAM_RSA_PRIVATE_KEY"))
	configuredFile := strings.TrimSpace(os.Getenv("IAM_RSA_PRIVATE_KEY_FILE"))

	if configuredInline != "" || configuredFile != "" {
		resolved := configuredInline
		if resolved == "" {
			pemBytes, err := os.ReadFile(configuredFile)
			if err != nil {
				return fmt.Errorf("read IAM_RSA_PRIVATE_KEY_FILE for bootstrap: %w", err)
			}
			resolved = strings.TrimSpace(string(pemBytes))
			if resolved == "" {
				return errors.New("IAM_RSA_PRIVATE_KEY_FILE is empty")
			}
			if err := os.Setenv("IAM_RSA_PRIVATE_KEY", resolved); err != nil {
				return fmt.Errorf("set IAM_RSA_PRIVATE_KEY from IAM_RSA_PRIVATE_KEY_FILE: %w", err)
			}
		}

		if pg != nil {
			stored, err := bootstrapsecrets.Ensure(pg, issuerPrivateKeyStoreKey, func() (string, error) {
				return resolved, nil
			})
			if err != nil {
				logging.Z().Info(fmt.Sprintf("⚠️  failed to seed IAM RSA key into postgres bootstrap store: %v", err))
			} else if stored != resolved {
				logging.Z().Info(fmt.Sprintf("⚠️  postgres bootstrap IAM RSA key differs from env value; keeping env for current runtime"))
			}
		}

		return nil
	}

	if pg != nil {
		resolved, err := bootstrapsecrets.Ensure(pg, issuerPrivateKeyStoreKey, generateRSAPrivateKeyPEM)
		if err == nil {
			if err := os.Setenv("IAM_RSA_PRIVATE_KEY", resolved); err != nil {
				return fmt.Errorf("set IAM_RSA_PRIVATE_KEY from postgres: %w", err)
			}
			return nil
		}
		logging.Z().Info(fmt.Sprintf("⚠️  postgres bootstrap for IAM RSA key failed, falling back to etcd: %v", err))
	}

	resolved, err := ensureSharedIssuerPrivateKeyFromEtcd(etcd)
	if err != nil {
		return err
	}
	if err := os.Setenv("IAM_RSA_PRIVATE_KEY", resolved); err != nil {
		return fmt.Errorf("set resolved IAM_RSA_PRIVATE_KEY: %w", err)
	}
	return nil
}

func ensureSharedIssuerPrivateKeyFromEtcd(etcd *clientv3.Client) (string, error) {
	if etcd == nil {
		return "", errors.New("IAM_RSA_PRIVATE_KEY is not set and neither postgres nor etcd bootstrap store is available")
	}

	getCtx, getCancel := context.WithTimeout(context.Background(), 5*time.Second)
	resp, err := etcd.Get(getCtx, issuerPrivateKeyEtcdKey)
	getCancel()
	if err != nil {
		return "", fmt.Errorf("read shared IAM RSA key from etcd: %w", err)
	}
	if len(resp.Kvs) > 0 {
		existing := strings.TrimSpace(string(resp.Kvs[0].Value))
		if existing == "" {
			return "", errors.New("shared IAM RSA key exists in etcd but is empty")
		}
		return existing, nil
	}

	candidate, err := generateRSAPrivateKeyPEM()
	if err != nil {
		return "", err
	}

	txnCtx, txnCancel := context.WithTimeout(context.Background(), 5*time.Second)
	txnResp, err := etcd.Txn(txnCtx).
		If(clientv3.Compare(clientv3.Version(issuerPrivateKeyEtcdKey), "=", 0)).
		Then(clientv3.OpPut(issuerPrivateKeyEtcdKey, candidate)).
		Else(clientv3.OpGet(issuerPrivateKeyEtcdKey)).
		Commit()
	txnCancel()
	if err != nil {
		return "", fmt.Errorf("persist shared IAM RSA key in etcd: %w", err)
	}

	resolved := candidate
	if !txnResp.Succeeded {
		resolved = ""
		if len(txnResp.Responses) > 0 {
			rangeResp := txnResp.Responses[0].GetResponseRange()
			if rangeResp != nil && len(rangeResp.Kvs) > 0 {
				resolved = strings.TrimSpace(string(rangeResp.Kvs[0].Value))
			}
		}
		if resolved == "" {
			return "", errors.New("shared IAM RSA key resolution failed after concurrent bootstrap")
		}
	}
	return resolved, nil
}

func generateRSAPrivateKeyPEM() (string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, iamconfig.DefaultRSAKeyBits)
	if err != nil {
		return "", fmt.Errorf("generate IAM RSA key: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	if len(pemBytes) == 0 {
		return "", errors.New("encode IAM RSA key to PEM")
	}
	return string(pemBytes), nil
}

// NewSystem initialises the complete IAM system.
// Requires a PostgreSQL connection (for users).
// For KV storage: uses etcd if available, otherwise KVStore (Raft backend).
func NewSystem(pg *gorm.DB, etcd *clientv3.Client, cfg Config, kvOpt ...platformstore.KVStore) (*System, error) {
	if pg == nil {
		return nil, errors.New("IAM requires a PostgreSQL connection")
	}
	var kv platformstore.KVStore
	if len(kvOpt) > 0 && kvOpt[0] != nil {
		kv = kvOpt[0]
	}
	if etcd == nil && kv == nil {
		return nil, errors.New("IAM requires an etcd connection or KVStore")
	}

	// Token issuer
	issuerURL := cfg.IssuerURL
	if issuerURL == "" {
		issuerURL = os.Getenv("IAM_ISSUER_URL")
	}
	if issuerURL == "" {
		issuerURL = "http://localhost:8080"
	}
	if err := ensureSharedIssuerPrivateKey(pg, etcd); err != nil {
		return nil, fmt.Errorf("IAM shared issuer key bootstrap: %w", err)
	}

	issuer, err := token.NewIssuer(issuerURL)
	if err != nil {
		return nil, fmt.Errorf("IAM token issuer: %w", err)
	}
	if cfg.AccessTokenTTL > 0 {
		issuer.AccessTokenTTL = cfg.AccessTokenTTL
	}
	if cfg.RefreshTokenTTL > 0 {
		issuer.RefreshTokenTTL = cfg.RefreshTokenTTL
	}

	// Repositories
	userRepo, err := storage.NewPostgresUserRepository(pg)
	if err != nil {
		return nil, fmt.Errorf("IAM user repository: %w", err)
	}

	clientRepo := storage.NewEtcdClientRepository(etcd)
	roleRepo := storage.NewEtcdRoleRepository(etcd)
	bindingRepo := storage.NewEtcdRoleBindingRepository(etcd)
	sessionRepo := storage.NewEtcdSessionRepository(etcd)
	refreshRepo := storage.NewEtcdRefreshTokenRepository(etcd)
	codeRepo := storage.NewEtcdCodeRepository(etcd)
	revokedStore := storage.NewEtcdRevokedTokenStore(etcd)

	if etcd == nil && kv != nil {
		// Use KVStore-backed repositories (Raft mode).
		clientRepo = storage.NewKVClientRepository(kv)
		roleRepo = storage.NewKVRoleRepository(kv)
		bindingRepo = storage.NewKVRoleBindingRepository(kv)
		sessionRepo = storage.NewKVSessionRepository(kv)
		refreshRepo = storage.NewKVRefreshTokenRepository(kv)
		codeRepo = storage.NewKVCodeRepository(kv)
		revokedStore = storage.NewKVRevokedTokenStore(kv)
	}

	// Seed system roles
	if err := storage.SeedSystemRoles(roleRepo); err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  IAM: role seeding error (non-fatal): %v", err))
	}

	// Authorizer
	authorizer := authz.NewAuthorizer(roleRepo, bindingRepo)

	// Authenticator
	authenticator := authn.NewAuthenticator(userRepo, sessionRepo)

	// Bootstrap sysadmin
	sysadminEmail := cfg.SysadminEmail
	if sysadminEmail == "" {
		sysadminEmail = os.Getenv("IAM_SYSADMIN_EMAIL")
	}
	sysadminPassword := cfg.SysadminPassword
	if sysadminPassword == "" {
		sysadminPassword = os.Getenv("IAM_SYSADMIN_PASSWORD")
	}

	if sysadminEmail != "" && sysadminPassword != "" {
		if err := bootstrapSysadmin(userRepo, roleRepo, authorizer, sysadminEmail, sysadminPassword); err != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  IAM: sysadmin bootstrap error (non-fatal): %v", err))
		}
	} else {
		logging.Z().Info("⚠️  IAM: No sysadmin credentials configured. Set IAM_SYSADMIN_EMAIL and IAM_SYSADMIN_PASSWORD.")
	}

	adminHandler := admin.NewHandler(
		userRepo, clientRepo, roleRepo, bindingRepo,
		sessionRepo, refreshRepo, revokedStore, codeRepo,
		authorizer, issuer, authenticator,
	)

	// ── Enhanced PostgreSQL-backed IAM store ──
	pgStore, err := pgstore.New(pg)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  IAM: enhanced pgstore init error (non-fatal): %v", err))
	}

	var enhancedHandler *admin.EnhancedHandler
	if pgStore != nil {
		enhancedHandler = admin.NewEnhancedHandler(pgStore)

		// Seed default realm and its roles/scopes
		defaultRealm, seedErr := pgStore.SeedDefaultRealm()
		if seedErr != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  IAM: default realm seed error: %v", seedErr))
		} else if defaultRealm != nil {
			if err := pgStore.SeedDefaultRoles(defaultRealm.ID); err != nil {
				logging.Z().Info(fmt.Sprintf("IAM: seed default roles error: %v", err))
			}
			if err := pgStore.SeedDefaultClientScopes(defaultRealm.ID); err != nil {
				logging.Z().Info(fmt.Sprintf("IAM: seed default scopes error: %v", err))
			}
			logging.Z().Info(fmt.Sprintf("✅ IAM: default realm '%s' ready (id=%s)", defaultRealm.Name, defaultRealm.ID))
		}
	}

	return &System{
		Issuer:          issuer,
		Authorizer:      authorizer,
		Authenticator:   authenticator,
		AdminHandler:    adminHandler,
		PGStore:         pgStore,
		EnhancedHandler: enhancedHandler,
		Users:           userRepo,
		Clients:         clientRepo,
		Roles:           roleRepo,
		Bindings:        bindingRepo,
		Sessions:        sessionRepo,
		RefreshRepo:     refreshRepo,
		CodeRepo:        codeRepo,
		RevokedStore:    revokedStore,
	}, nil
}

// RegisterRoutes mounts all IAM endpoints on the provided Gin router.
func (s *System) RegisterRoutes(router *gin.Engine) {
	h := s.AdminHandler

	// ── OIDC Discovery (public) ──
	router.GET("/.well-known/openid-configuration", h.OpenIDConfiguration)
	router.GET("/.well-known/jwks.json", h.JWKS)
	router.GET("/realms/:realm/.well-known/openid-configuration", h.OpenIDConfigurationRealm)
	router.POST("/realms/:realm/protocol/openid-connect/token", h.RealmToken)
	router.GET("/realms/:realm/protocol/openid-connect/certs", h.RealmJWKS)

	// ── Authentication (public) ──
	router.POST("/iam/auth/login", h.Login)
	router.POST("/iam/auth/refresh", h.RefreshToken)
	if s.EnhancedHandler != nil {
		router.GET("/iam/auth/identity-providers", s.EnhancedHandler.ListPublicIdentityProviders)
	} else {
		router.GET("/iam/auth/identity-providers", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"identity_providers": []gin.H{},
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
		})
	}

	// IAM JWT middleware for all protected IAM routes
	iamAuth := iammw.JWTAuth(s.Issuer, s.RevokedStore)
	sysadminOnly := iammw.RequireSysadmin()

	// ── Self-service (authenticated) ──
	router.GET("/iam/auth/whoami", iamAuth, h.WhoAmI)
	router.POST("/iam/auth/logout", iamAuth, h.Logout)

	// ── OAuth2 Endpoints (authenticated user initiates, public token endpoint) ──
	router.GET("/oauth/authorize", iamAuth, h.Authorize)
	router.GET("/realms/:realm/protocol/openid-connect/auth", iamAuth, h.RealmAuthorize)
	router.POST("/oauth/token", h.Token)

	// ── Sysadmin API (master-realm admin only) ──
	adminAPI := router.Group("/iam/admin", iamAuth, sysadminOnly)
	{
		// Users
		adminAPI.GET("/users", h.ListUsers)
		adminAPI.GET("/users/:id", h.GetUser)
		adminAPI.POST("/users", h.CreateUser)
		adminAPI.PUT("/users/:id", h.UpdateUser)
		adminAPI.PUT("/users/:id/roles", h.SetUserRoles)
		adminAPI.DELETE("/users/:id", h.DeleteUser)

		// OAuth Clients
		adminAPI.GET("/clients", h.ListClients)
		adminAPI.GET("/clients/:id", h.GetClient)
		adminAPI.POST("/clients", h.RegisterClient)
		adminAPI.PUT("/clients/:id", h.UpdateClient)
		adminAPI.PUT("/clients/:id/client-id", h.ChangeClientID)
		adminAPI.POST("/clients/:id/regenerate-secret", h.RegenerateClientSecret)
		adminAPI.DELETE("/clients/:id", h.DeleteClient)
		adminAPI.GET("/service-access-info", h.ServiceAccessInfo)

		// Roles
		adminAPI.GET("/roles", h.ListRoles)
		adminAPI.GET("/roles/:id", h.GetRole)
		adminAPI.POST("/roles", h.CreateRole)
		adminAPI.PUT("/roles/:id", h.UpdateRole)
		adminAPI.DELETE("/roles/:id", h.DeleteRole)

		// Role Assignments
		adminAPI.POST("/role-bindings", h.AssignRole)
		adminAPI.GET("/role-bindings", h.ListBindings)
		adminAPI.DELETE("/role-bindings/:id", h.RevokeBinding)

		// Token Management
		adminAPI.POST("/tokens/revoke", h.RevokeToken)
		adminAPI.POST("/users/:id/revoke-tokens", h.RevokeUserTokens)
	}

	// ── Enhanced Keycloak-style Admin API (PostgreSQL-backed) ──
	if s.EnhancedHandler != nil {
		eh := s.EnhancedHandler

		enhancedAPI := router.Group("/iam/v2", iamAuth, sysadminOnly)
		{
			// Realms
			enhancedAPI.GET("/realms", eh.ListRealms)
			enhancedAPI.GET("/realms/:realmId", eh.GetRealm)
			enhancedAPI.POST("/realms", eh.CreateRealm)
			enhancedAPI.PUT("/realms/:realmId", eh.UpdateRealm)
			enhancedAPI.DELETE("/realms/:realmId", eh.DeleteRealm)
			enhancedAPI.GET("/realms/:realmId/dashboard", eh.RealmDashboard)
			enhancedAPI.GET("/realms/:realmId/info", eh.RealmInfo)

			// Groups
			enhancedAPI.GET("/groups", eh.ListGroups)
			enhancedAPI.GET("/groups/:id", eh.GetGroup)
			enhancedAPI.POST("/groups", eh.CreateGroup)
			enhancedAPI.PUT("/groups/:id", eh.UpdateGroup)
			enhancedAPI.DELETE("/groups/:id", eh.DeleteGroup)
			enhancedAPI.POST("/groups/:id/members", eh.AddGroupMember)
			enhancedAPI.DELETE("/groups/:id/members/:userId", eh.RemoveGroupMember)

			// Client Scopes
			enhancedAPI.GET("/client-scopes", eh.ListClientScopes)
			enhancedAPI.GET("/client-scopes/:id", eh.GetClientScope)
			enhancedAPI.POST("/client-scopes", eh.CreateClientScope)
			enhancedAPI.PUT("/client-scopes/:id", eh.UpdateClientScope)
			enhancedAPI.DELETE("/client-scopes/:id", eh.DeleteClientScope)

			// Identity Providers
			enhancedAPI.GET("/identity-providers", eh.ListIdentityProviders)
			enhancedAPI.GET("/identity-providers/:id", eh.GetIdentityProvider)
			enhancedAPI.POST("/identity-providers", eh.CreateIdentityProvider)
			enhancedAPI.PUT("/identity-providers/:id", eh.UpdateIdentityProvider)
			enhancedAPI.DELETE("/identity-providers/:id", eh.DeleteIdentityProvider)

			// SSO Sessions
			enhancedAPI.GET("/users/:userId/sessions", eh.ListUserSessions)
			enhancedAPI.DELETE("/sessions/:sessionId", eh.RevokeSession)
			enhancedAPI.DELETE("/users/:userId/sessions", eh.RevokeUserSessions)

			// Events / Audit Log
			enhancedAPI.GET("/events", eh.ListEvents)
			enhancedAPI.GET("/users/:userId/events", eh.ListUserEvents)

			// User Attributes
			enhancedAPI.GET("/users/:userId/attributes", eh.GetUserAttributes)
			enhancedAPI.POST("/users/:userId/attributes", eh.SetUserAttribute)
			enhancedAPI.DELETE("/users/:userId/attributes", eh.DeleteUserAttribute)

			// User Groups
			enhancedAPI.GET("/users/:userId/groups", eh.GetUserGroups)
			enhancedAPI.POST("/users/:userId/groups", eh.AddUserToGroup)
			enhancedAPI.DELETE("/users/:userId/groups/:groupId", eh.RemoveUserFromGroup)

			// User Consents
			enhancedAPI.GET("/users/:userId/consents", eh.GetUserConsents)
			enhancedAPI.DELETE("/users/:userId/consents/:clientId", eh.RevokeUserConsent)

			// Required Actions
			enhancedAPI.GET("/users/:userId/required-actions", eh.GetRequiredActions)
			enhancedAPI.POST("/users/:userId/required-actions", eh.AddRequiredAction)
			enhancedAPI.DELETE("/users/:userId/required-actions/:action", eh.RemoveRequiredAction)

			// Realm-scoped Roles (PostgreSQL)
			enhancedAPI.GET("/pg-roles", eh.ListPGRoles)
			enhancedAPI.GET("/pg-roles/:id", eh.GetPGRole)
			enhancedAPI.POST("/pg-roles", eh.CreatePGRole)
			enhancedAPI.PUT("/pg-roles/:id", eh.UpdatePGRole)
			enhancedAPI.DELETE("/pg-roles/:id", eh.DeletePGRole)

			// Role Bindings (PostgreSQL)
			enhancedAPI.POST("/pg-role-bindings", eh.CreateRoleBinding)
			enhancedAPI.GET("/users/:userId/pg-role-bindings", eh.ListUserRoleBindings)
			enhancedAPI.GET("/users/:userId/effective-roles", eh.GetEffectiveRoles)
			enhancedAPI.DELETE("/pg-role-bindings/:id", eh.DeleteRoleBinding)

			// Realm-scoped Clients (PostgreSQL)
			enhancedAPI.GET("/pg-clients", eh.ListPGClients)
			enhancedAPI.GET("/pg-clients/:id", eh.GetPGClient)
			enhancedAPI.POST("/pg-clients", eh.CreatePGClient)
			enhancedAPI.PUT("/pg-clients/:id", eh.UpdatePGClient)
			enhancedAPI.DELETE("/pg-clients/:id", eh.DeletePGClient)
		}

		logging.Z().Info("✅ IAM: enhanced Keycloak-style v2 routes registered")
	}

	logging.Z().Info("✅ IAM: all routes registered")
}

// Name returns the module identifier.
func (s *System) Name() string { return "iam" }

// Start initializes the module.
func (s *System) Start(ctx context.Context) error {
	logging.Z().Info("✅ IAM: module started")
	return nil
}

// Stop gracefully shuts down the module.
func (s *System) Stop() error {
	logging.Z().Info("IAM: stopping")
	return nil
}

// SetKVStore wires the KVStore-backed persistence into IAM.
// Called in Raft mode after the Raft KV becomes available.
func (s *System) SetKVStore(kv platformstore.KVStore) {
	// IAM uses etcd/PostgreSQL directly; KVStore wiring is handled
	// by the individual storage backends (EtcdClientRepository, etc.)
	logging.Z().Info("✅ IAM: KVStore persistence configured (Raft mode)")
}

// bootstrapSysadmin ensures the master-realm admin account exists.
func bootstrapSysadmin(
	userRepo *storage.PostgresUserRepository,
	roleRepo *storage.EtcdRoleRepository,
	authorizer *authz.Authorizer,
	email, password string,
) error {
	email = identity.NormaliseEmail(email)

	existing, _ := userRepo.GetByEmail(email)
	if existing != nil {
		logging.Z().Info(fmt.Sprintf("✅ IAM: sysadmin account already exists (%s)", email))
		// Ensure sysadmin role is bound
		sysRole, _ := roleRepo.GetRoleByName("sysadmin")
		if sysRole != nil {
			_, _ = authorizer.AssignRole(existing.ID, sysRole.ID)
		}
		return nil
	}

	hash, err := identity.HashPassword(password)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}

	user := &identity.User{
		ID:            identity.NewUserID(),
		Email:         email,
		PasswordHash:  hash,
		DisplayName:   "System Administrator",
		Active:        true,
		EmailVerified: true,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	if err := userRepo.Create(user); err != nil {
		return fmt.Errorf("creating sysadmin: %w", err)
	}

	sysRole, _ := roleRepo.GetRoleByName("sysadmin")
	if sysRole != nil {
		if _, err := authorizer.AssignRole(user.ID, sysRole.ID); err != nil {
			return fmt.Errorf("assigning sysadmin role: %w", err)
		}
	}

	logging.Z().Info(fmt.Sprintf("✅ IAM: sysadmin account bootstrapped (%s)", email))
	return nil
}
