package models

import (
	"time"

	"github.com/lib/pq"
)

// ──────────────────────────────────────────────
// Realm — Keycloak-style multi-tenant isolation
// ──────────────────────────────────────────────

// Realm represents a security domain (equivalent to a Keycloak realm).
type Realm struct {
	ID                     string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name                   string         `gorm:"uniqueIndex;type:varchar(64);not null" json:"name"`
	DisplayName            string         `gorm:"type:varchar(255)" json:"display_name"`
	Enabled                bool           `gorm:"default:true" json:"enabled"`
	RegistrationAllowed    bool           `gorm:"default:false" json:"registration_allowed"`
	ResetPasswordAllowed   bool           `gorm:"default:true" json:"reset_password_allowed"`
	RememberMe             bool           `gorm:"default:true" json:"remember_me"`
	VerifyEmail            bool           `gorm:"default:false" json:"verify_email"`
	LoginWithEmail         bool           `gorm:"default:true" json:"login_with_email"`
	DuplicateEmailsAllowed bool           `gorm:"default:false" json:"duplicate_emails_allowed"`
	SSLRequired            string         `gorm:"type:varchar(20);default:'external'" json:"ssl_required"` // none, external, all
	DefaultRoles           pq.StringArray `gorm:"type:text[]" json:"default_roles"`
	AccessTokenLifespan    int            `gorm:"default:300" json:"access_token_lifespan"`      // seconds
	RefreshTokenLifespan   int            `gorm:"default:1800" json:"refresh_token_lifespan"`    // seconds
	SSOSessionIdleTimeout  int            `gorm:"default:1800" json:"sso_session_idle_timeout"`  // seconds
	SSOSessionMaxLifespan  int            `gorm:"default:36000" json:"sso_session_max_lifespan"` // seconds
	BruteForceProtected    bool           `gorm:"default:true" json:"brute_force_protected"`
	MaxFailureWaitSeconds  int            `gorm:"default:900" json:"max_failure_wait_seconds"`
	MaxLoginFailures       int            `gorm:"default:30" json:"max_login_failures"`
	PasswordMinLength      int            `gorm:"default:8" json:"password_min_length"`
	PasswordRequireUpper   bool           `gorm:"default:true" json:"password_require_upper"`
	PasswordRequireDigit   bool           `gorm:"default:true" json:"password_require_digit"`
	PasswordRequireSpecial bool           `gorm:"default:false" json:"password_require_special"`
	CreatedAt              time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt              time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Realm) TableName() string { return "iam_realms" }

// ──────────────────────────────────────────────
// Client — enriched OAuth client (Keycloak-style)
// ──────────────────────────────────────────────

// Client is the PostgreSQL-backed OAuth/OIDC client.
type Client struct {
	ID                    string         `gorm:"primaryKey;type:varchar(128)" json:"id"`
	RealmID               string         `gorm:"type:varchar(36);index;not null" json:"realm_id"`
	Secret                string         `gorm:"type:varchar(255)" json:"secret,omitempty" classification:"Confidential"`
	Name                  string         `gorm:"type:varchar(255);not null" json:"name"`
	Description           string         `gorm:"type:text" json:"description,omitempty"`
	Enabled               bool           `gorm:"default:true" json:"enabled"`
	Public                bool           `gorm:"default:false" json:"public"`
	Protocol              string         `gorm:"type:varchar(20);default:'openid-connect'" json:"protocol"` // openid-connect, saml
	RootURL               string         `gorm:"type:varchar(512)" json:"root_url,omitempty"`
	BaseURL               string         `gorm:"type:varchar(512)" json:"base_url,omitempty"`
	RedirectURIs          pq.StringArray `gorm:"type:text[]" json:"redirect_uris"`
	WebOrigins            pq.StringArray `gorm:"type:text[]" json:"web_origins,omitempty"`
	Scopes                pq.StringArray `gorm:"type:text[]" json:"scopes"`
	GrantTypes            pq.StringArray `gorm:"type:text[]" json:"grant_types"`
	ServiceRoles          pq.StringArray `gorm:"type:text[]" json:"service_roles,omitempty"`
	ConsentRequired       bool           `gorm:"default:false" json:"consent_required"`
	StandardFlowEnabled   bool           `gorm:"default:true" json:"standard_flow_enabled"`
	ImplicitFlowEnabled   bool           `gorm:"default:false" json:"implicit_flow_enabled"`
	DirectAccessEnabled   bool           `gorm:"default:false" json:"direct_access_enabled"`
	ServiceAccountEnabled bool           `gorm:"default:false" json:"service_account_enabled"`
	RateLimitMaxCalls     int64          `gorm:"default:500" json:"rate_limit_max_calls"`
	TokenValidityMinutes  int            `gorm:"default:15" json:"token_validity_minutes"`
	Active                bool           `gorm:"default:true" json:"active"`
	CreatedAt             time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt             time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Client) TableName() string { return "iam_clients" }

// ──────────────────────────────────────────────
// User — enriched identity user
// ──────────────────────────────────────────────

// User is the PostgreSQL-backed IAM user.
type User struct {
	ID                     string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	RealmID                string     `gorm:"type:varchar(36);index;not null" json:"realm_id"`
	Username               string     `gorm:"type:varchar(255)" json:"username"`
	Email                  string     `gorm:"type:varchar(255)" json:"email" classification:"PII"`
	PasswordHash           string     `gorm:"type:varchar(255);not null" json:"-" classification:"Confidential"`
	FirstName              string     `gorm:"type:varchar(255)" json:"first_name,omitempty"`
	LastName               string     `gorm:"type:varchar(255)" json:"last_name,omitempty"`
	DisplayName            string     `gorm:"type:varchar(255)" json:"display_name"`
	Active                 bool       `gorm:"default:true" json:"active"`
	EmailVerified          bool       `gorm:"default:false" json:"email_verified"`
	PhoneNumber            string     `gorm:"type:varchar(50)" json:"phone_number,omitempty" classification:"PII"`
	PhoneVerified          bool       `gorm:"default:false" json:"phone_verified"`
	TOTPEnabled            bool       `gorm:"default:false" json:"totp_enabled"`
	TOTPSecret             string     `gorm:"type:varchar(64)" json:"-" classification:"Confidential"`
	FederatedIdentity      string     `gorm:"type:varchar(255)" json:"federated_identity,omitempty"`
	FederationLink         string     `gorm:"type:varchar(36)" json:"federation_link,omitempty"`
	ServiceAccountClientID string     `gorm:"type:varchar(128)" json:"service_account_client_id,omitempty"`
	Locale                 string     `gorm:"type:varchar(10)" json:"locale,omitempty"`
	LoginFailures          int        `gorm:"default:0" json:"-"`
	LastFailedLogin        *time.Time `gorm:"" json:"-"`
	LockedUntil            *time.Time `gorm:"" json:"locked_until,omitempty"`
	CreatedAt              time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt              time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (User) TableName() string { return "iam_users_v2" }

// ──────────────────────────────────────────────
// User Attributes — key-value store per user
// ──────────────────────────────────────────────

type UserAttribute struct {
	ID     string `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserID string `gorm:"type:varchar(36);index;not null" json:"user_id"`
	Key    string `gorm:"type:varchar(255);not null" json:"key"`
	Value  string `gorm:"type:text" json:"value"`
}

func (UserAttribute) TableName() string { return "iam_user_attributes" }

// ──────────────────────────────────────────────
// Group — hierarchical user groups (Keycloak-style)
// ──────────────────────────────────────────────

type Group struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	RealmID   string    `gorm:"type:varchar(36);index;not null" json:"realm_id"`
	Name      string    `gorm:"type:varchar(255);not null" json:"name"`
	ParentID  string    `gorm:"type:varchar(36);index" json:"parent_id,omitempty"`
	Path      string    `gorm:"type:varchar(1024)" json:"path"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Group) TableName() string { return "iam_groups" }

// ──────────────────────────────────────────────
// Group Membership
// ──────────────────────────────────────────────

type GroupMembership struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserID    string    `gorm:"type:varchar(36);index;not null" json:"user_id"`
	GroupID   string    `gorm:"type:varchar(36);index;not null" json:"group_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (GroupMembership) TableName() string { return "iam_group_memberships" }

// ──────────────────────────────────────────────
// Role — realm & client roles
// ──────────────────────────────────────────────

type Role struct {
	ID             string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	RealmID        string         `gorm:"type:varchar(36);index;not null" json:"realm_id"`
	ClientID       string         `gorm:"type:varchar(128);index" json:"client_id,omitempty"` // empty = realm role
	Name           string         `gorm:"type:varchar(255);not null" json:"name"`
	Description    string         `gorm:"type:text" json:"description,omitempty"`
	Composite      bool           `gorm:"default:false" json:"composite"`
	CompositeRoles pq.StringArray `gorm:"type:text[]" json:"composite_roles,omitempty"` // IDs of child roles
	Permissions    pq.StringArray `gorm:"type:text[]" json:"permissions,omitempty"`     // resource:action pairs
	System         bool           `gorm:"default:false" json:"system"`
	CreatedAt      time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Role) TableName() string { return "iam_roles_v2" }

// ──────────────────────────────────────────────
// Role Binding — user/group → role assignment
// ──────────────────────────────────────────────

type RoleBinding struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	RealmID   string    `gorm:"type:varchar(36);index;not null" json:"realm_id"`
	UserID    string    `gorm:"type:varchar(36);index" json:"user_id,omitempty"`
	GroupID   string    `gorm:"type:varchar(36);index" json:"group_id,omitempty"`
	RoleID    string    `gorm:"type:varchar(36);index;not null" json:"role_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (RoleBinding) TableName() string { return "iam_role_bindings_v2" }

// ──────────────────────────────────────────────
// Client Scope — protocol mapper scope definitions
// ──────────────────────────────────────────────

type ClientScope struct {
	ID               string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	RealmID          string    `gorm:"type:varchar(36);index;not null" json:"realm_id"`
	Name             string    `gorm:"type:varchar(255);not null" json:"name"`
	Description      string    `gorm:"type:text" json:"description,omitempty"`
	Protocol         string    `gorm:"type:varchar(20);default:'openid-connect'" json:"protocol"`
	ClaimName        string    `gorm:"type:varchar(255)" json:"claim_name,omitempty"`
	ClaimType        string    `gorm:"type:varchar(20)" json:"claim_type,omitempty"` // String, long, int, boolean, JSON
	AddToIDToken     bool      `gorm:"default:true" json:"add_to_id_token"`
	AddToAccessToken bool      `gorm:"default:true" json:"add_to_access_token"`
	AddToUserInfo    bool      `gorm:"default:true" json:"add_to_userinfo"`
	BuiltIn          bool      `gorm:"default:false" json:"built_in"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (ClientScope) TableName() string { return "iam_client_scopes" }

// ──────────────────────────────────────────────
// Identity Provider — external SSO federation
// ──────────────────────────────────────────────

type IdentityProvider struct {
	ID               string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	RealmID          string    `gorm:"type:varchar(36);index;not null" json:"realm_id"`
	Alias            string    `gorm:"type:varchar(255);not null" json:"alias"`
	DisplayName      string    `gorm:"type:varchar(255)" json:"display_name"`
	ProviderType     string    `gorm:"type:varchar(50);not null" json:"provider_type"` // oidc, saml, github, google, ldap
	Enabled          bool      `gorm:"default:true" json:"enabled"`
	TrustEmail       bool      `gorm:"default:false" json:"trust_email"`
	StoreToken       bool      `gorm:"default:false" json:"store_token"`
	FirstBrokerLogin string    `gorm:"type:varchar(255)" json:"first_broker_login,omitempty"` // flow alias
	AuthorizationURL string    `gorm:"type:varchar(512)" json:"authorization_url,omitempty"`
	TokenURL         string    `gorm:"type:varchar(512)" json:"token_url,omitempty"`
	UserInfoURL      string    `gorm:"type:varchar(512)" json:"userinfo_url,omitempty"`
	ClientID         string    `gorm:"type:varchar(255)" json:"client_id,omitempty"`
	ClientSecret     string    `gorm:"type:varchar(512)" json:"client_secret,omitempty" classification:"Confidential"`
	Issuer           string    `gorm:"type:varchar(512)" json:"issuer,omitempty"`
	DefaultScopes    string    `gorm:"type:text" json:"default_scopes,omitempty"`
	SyncMode         string    `gorm:"type:varchar(20);default:'import'" json:"sync_mode"` // import, force, legacy
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (IdentityProvider) TableName() string { return "iam_identity_providers" }

// ──────────────────────────────────────────────
// SSO Session — user login sessions
// ──────────────────────────────────────────────

type SSOSession struct {
	ID           string    `gorm:"primaryKey;type:varchar(128)" json:"id"`
	RealmID      string    `gorm:"type:varchar(36);index;not null" json:"realm_id"`
	UserID       string    `gorm:"type:varchar(36);index;not null" json:"user_id"`
	IPAddress    string    `gorm:"type:varchar(45)" json:"ip_address" classification:"PII"`
	UserAgent    string    `gorm:"type:text" json:"user_agent,omitempty" classification:"Sensitive"`
	AuthMethod   string    `gorm:"type:varchar(50)" json:"auth_method,omitempty"` // password, otp, sso, federated
	RememberMe   bool      `gorm:"default:false" json:"remember_me"`
	State        string    `gorm:"type:varchar(20);default:'active'" json:"state"` // active, expired, revoked
	StartedAt    time.Time `gorm:"autoCreateTime" json:"started_at"`
	LastAccessAt time.Time `gorm:"" json:"last_access_at"`
	ExpiresAt    time.Time `gorm:"" json:"expires_at"`
}

func (SSOSession) TableName() string { return "iam_sso_sessions" }

// ──────────────────────────────────────────────
// Client Session — per-client session tracking
// ──────────────────────────────────────────────

type ClientSession struct {
	ID           string    `gorm:"primaryKey;type:varchar(128)" json:"id"`
	SSOSessionID string    `gorm:"type:varchar(128);index;not null" json:"sso_session_id"`
	ClientID     string    `gorm:"type:varchar(128);index;not null" json:"client_id"`
	RealmID      string    `gorm:"type:varchar(36);index;not null" json:"realm_id"`
	UserID       string    `gorm:"type:varchar(36)" json:"user_id"`
	RedirectURI  string    `gorm:"type:varchar(512)" json:"redirect_uri,omitempty"`
	Action       string    `gorm:"type:varchar(50)" json:"action,omitempty"` // code, token
	Scope        string    `gorm:"type:text" json:"scope,omitempty"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	ExpiresAt    time.Time `gorm:"" json:"expires_at"`
}

func (ClientSession) TableName() string { return "iam_client_sessions" }

// ──────────────────────────────────────────────
// Event — audit log for login & admin events
// ──────────────────────────────────────────────

type Event struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	RealmID   string    `gorm:"type:varchar(36);index;not null" json:"realm_id"`
	Type      string    `gorm:"type:varchar(50);index;not null" json:"type"` // LOGIN, LOGIN_ERROR, LOGOUT, REGISTER, TOKEN, ADMIN
	UserID    string    `gorm:"type:varchar(36);index" json:"user_id,omitempty"`
	ClientID  string    `gorm:"type:varchar(128)" json:"client_id,omitempty"`
	SessionID string    `gorm:"type:varchar(128)" json:"session_id,omitempty"`
	IPAddress string    `gorm:"type:varchar(45)" json:"ip_address,omitempty" classification:"PII"`
	Details   string    `gorm:"type:text" json:"details,omitempty"` // JSON details
	Error     string    `gorm:"type:text" json:"error,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (Event) TableName() string { return "iam_events" }

// ──────────────────────────────────────────────
// Required Action — user-required actions on login
// ──────────────────────────────────────────────

type RequiredAction struct {
	ID       string `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserID   string `gorm:"type:varchar(36);index;not null" json:"user_id"`
	Action   string `gorm:"type:varchar(50);not null" json:"action"` // VERIFY_EMAIL, UPDATE_PASSWORD, CONFIGURE_TOTP, UPDATE_PROFILE, TERMS_AND_CONDITIONS
	Priority int    `gorm:"default:0" json:"priority"`
}

func (RequiredAction) TableName() string { return "iam_required_actions" }

// ──────────────────────────────────────────────
// Credential — multi-credential support (password, OTP, WebAuthn)
// ──────────────────────────────────────────────

type Credential struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserID    string    `gorm:"type:varchar(36);index;not null" json:"user_id"`
	Type      string    `gorm:"type:varchar(50);not null" json:"type"` // password, otp, webauthn
	Value     string    `gorm:"type:text;not null" json:"-" classification:"Confidential"`
	Device    string    `gorm:"type:varchar(255)" json:"device,omitempty"`
	Priority  int       `gorm:"default:0" json:"priority"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (Credential) TableName() string { return "iam_credentials" }

// ──────────────────────────────────────────────
// Consent — user consent per client
// ──────────────────────────────────────────────

type UserConsent struct {
	ID            string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserID        string         `gorm:"type:varchar(36);index;not null" json:"user_id"`
	ClientID      string         `gorm:"type:varchar(128);index;not null" json:"client_id"`
	RealmID       string         `gorm:"type:varchar(36);index;not null" json:"realm_id"`
	GrantedScopes pq.StringArray `gorm:"type:text[]" json:"granted_scopes"`
	CreatedAt     time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (UserConsent) TableName() string { return "iam_user_consents" }
