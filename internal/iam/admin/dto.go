package admin

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/iam/authz"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
)

// ──────────────────────────────────────────────
// Auth DTOs
// ──────────────────────────────────────────────

// RefreshTokenRequest is the API request for refreshing tokens.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// LogoutRequest is the API request for logging out.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// LogoutResponse is the API response for logout.
type LogoutResponse struct {
	Status              string `json:"status"`
	AccessTokenRevoked  bool   `json:"access_token_revoked"`
	SessionRevoked      bool   `json:"session_revoked"`
	RefreshTokensRevoked bool  `json:"refresh_tokens_revoked"`
}

// WhoAmIResponse is the API response for the whoami endpoint.
type WhoAmIResponse struct {
	UserID      string   `json:"user_id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Roles       []string `json:"roles"`
}

// ──────────────────────────────────────────────
// User DTOs
// ──────────────────────────────────────────────

// CreateUserRequest is the API request for creating a user.
type CreateUserRequest struct {
	Email         string   `json:"email" binding:"required,email"`
	Password      string   `json:"password" binding:"required,min=8"`
	DisplayName   string   `json:"display_name"`
	Active        *bool    `json:"active"`
	EmailVerified *bool    `json:"email_verified"`
	RoleNames     []string `json:"role_names"`
}

// UserResponse is the API response for a user (no password hash).
type UserResponse struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	DisplayName   string    `json:"display_name"`
	Roles         []string  `json:"roles"`
	Active        bool      `json:"active"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ListUsersResponse is the API response for listing users.
type ListUsersResponse struct {
	Users []UserResponse `json:"users"`
	Count int            `json:"count"`
}

// SetUserRolesRequest is the API request for replacing user roles.
type SetUserRolesRequest struct {
	RoleNames []string `json:"role_names" binding:"required"`
}

// SetUserRolesResponse is the API response after setting user roles.
type SetUserRolesResponse struct {
	UserID string   `json:"user_id"`
	Roles  []string `json:"roles"`
	Count  int      `json:"count"`
}

// ──────────────────────────────────────────────
// Client DTOs
// ──────────────────────────────────────────────

// RegisterClientRequest is the API request for creating an OAuth client.
type RegisterClientRequest struct {
	Name                 string   `json:"name" binding:"required"`
	RedirectURIs         []string `json:"redirect_uris"`
	Scopes               []string `json:"scopes"`
	GrantTypes           []string `json:"grant_types"`
	ServiceRoles         []string `json:"service_roles"`
	RateLimitMaxCalls    int64    `json:"rate_limit_max_calls"`
	TokenValidityMinutes int      `json:"token_validity_minutes"`
	Public               bool     `json:"public"`
}

// ClientResponse is the API response for an OAuth client (no secret).
type ClientResponse struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	RedirectURIs         []string  `json:"redirect_uris"`
	Scopes               []string  `json:"scopes"`
	GrantTypes           []string  `json:"grant_types"`
	ServiceRoles         []string  `json:"service_roles,omitempty"`
	RateLimitMaxCalls    int64     `json:"rate_limit_max_calls"`
	TokenValidityMinutes int       `json:"token_validity_minutes"`
	Public               bool      `json:"public"`
	Active               bool      `json:"active"`
	CreatedAt            time.Time `json:"created_at"`
}

// ClientCreatedResponse is the API response for a newly created client (includes secret).
type ClientCreatedResponse struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	RedirectURIs         []string  `json:"redirect_uris"`
	Scopes               []string  `json:"scopes"`
	GrantTypes           []string  `json:"grant_types"`
	ServiceRoles         []string  `json:"service_roles,omitempty"`
	RateLimitMaxCalls    int64     `json:"rate_limit_max_calls"`
	TokenValidityMinutes int       `json:"token_validity_minutes"`
	Public               bool      `json:"public"`
	CreatedAt            time.Time `json:"created_at"`
	ClientSecret         string    `json:"client_secret,omitempty"`
	Warning              string    `json:"warning,omitempty"`
}

// ListClientsResponse is the API response for listing clients.
type ListClientsResponse struct {
	Clients []ClientResponse `json:"clients"`
	Count   int              `json:"count"`
}

// UpdateClientRequest is the API request for updating a client.
type UpdateClientRequest struct {
	RedirectURIs         *[]string `json:"redirect_uris"`
	Scopes               *[]string `json:"scopes"`
	GrantTypes           *[]string `json:"grant_types"`
	ServiceRoles         *[]string `json:"service_roles"`
	RateLimitMaxCalls    *int64    `json:"rate_limit_max_calls"`
	TokenValidityMinutes *int      `json:"token_validity_minutes"`
	Active               *bool     `json:"active"`
	Name                 *string   `json:"name"`
}

// RegenerateSecretResponse is the API response after regenerating a client secret.
type RegenerateSecretResponse struct {
	ID                   string   `json:"id"`
	ClientID             string   `json:"client_id"`
	ClientSecret         string   `json:"client_secret"`
	Scopes               []string `json:"scopes"`
	GrantTypes           []string `json:"grant_types"`
	RateLimitMaxCalls    int64    `json:"rate_limit_max_calls"`
	TokenValidityMinutes int      `json:"token_validity_minutes"`
	Warning              string   `json:"warning"`
}

// ChangeClientIDRequest is the API request for changing a client ID.
type ChangeClientIDRequest struct {
	NewClientID string `json:"new_client_id" binding:"required"`
}

// ChangeClientIDResponse is the API response after changing a client ID.
type ChangeClientIDResponse struct {
	Message               string    `json:"message"`
	OldClientID           string    `json:"old_client_id"`
	NewClientID           string    `json:"new_client_id"`
	RedirectURIs          []string  `json:"redirect_uris"`
	Scopes                []string  `json:"scopes"`
	GrantTypes            []string  `json:"grant_types"`
	ServiceRoles          []string  `json:"service_roles,omitempty"`
	RateLimitMaxCalls     int64     `json:"rate_limit_max_calls"`
	TokenValidityMinutes  int       `json:"token_validity_minutes"`
	Public                bool      `json:"public"`
	Active                bool      `json:"active"`
	CreatedAt             time.Time `json:"created_at"`
}

// ──────────────────────────────────────────────
// Role DTOs
// ──────────────────────────────────────────────

// UpdateRoleRequest is the API request for updating a role.
type UpdateRoleRequest struct {
	Description *string             `json:"description"`
	Permissions *[]authz.Permission `json:"permissions"`
}

// ListRolesResponse is the API response for listing roles.
type ListRolesResponse struct {
	Roles []*authz.Role `json:"roles"`
	Count int           `json:"count"`
}

// ──────────────────────────────────────────────
// Role Binding DTOs
// ──────────────────────────────────────────────

// ListBindingsResponse is the API response for listing role bindings.
type ListBindingsResponse struct {
	Bindings []*authz.RoleBinding `json:"bindings"`
	Count    int                  `json:"count"`
}

// ──────────────────────────────────────────────
// Token DTOs
// ──────────────────────────────────────────────

// RevokeTokenRequest is the API request for revoking a token.
type RevokeTokenRequest struct {
	JTI        string `json:"jti" binding:"required"`
	TTLSeconds int    `json:"ttl_seconds"`
}

// RevokeTokenResponse is the API response after revoking a token.
type RevokeTokenResponse struct {
	Message string `json:"message"`
	JTI     string `json:"jti"`
}

// RevokeUserTokensResponse is the API response after revoking all user tokens.
type RevokeUserTokensResponse struct {
	Message string `json:"message"`
	UserID  string `json:"user_id"`
}

// ──────────────────────────────────────────────
// OAuth DTOs
// ──────────────────────────────────────────────

// AuthorizeResponse is the API response for the authorize endpoint.
type AuthorizeResponse struct {
	Code        string `json:"code"`
	State       string `json:"state"`
	RedirectURI string `json:"redirect_uri"`
}

// ClientCredentialsResponse is the API response for client_credentials grant.
type ClientCredentialsResponse struct {
	AccessToken           string `json:"access_token"`
	TokenType             string `json:"token_type"`
	ExpiresIn             int    `json:"expires_in"`
	Scope                 string `json:"scope"`
	RateLimitMaxCalls     int64  `json:"rate_limit_max_calls"`
	TokenValidityMinutes  int    `json:"token_validity_minutes"`
}

// ServiceAccessInfoResponse is the API response for service access info.
type ServiceAccessInfoResponse struct {
	Realm           string                    `json:"realm"`
	ConfiguredRealm string                    `json:"configured_realm"`
	Issuer          string                    `json:"issuer"`
	Endpoints       ServiceAccessEndpoints    `json:"endpoints"`
	GrantTypes      []string                  `json:"grant_types_supported"`
	AuthMethods     []string                  `json:"token_endpoint_auth_methods_supported"`
}

// ServiceAccessEndpoints holds the endpoint URLs for service access info.
type ServiceAccessEndpoints struct {
	IAMOpenIDConfiguration     string `json:"iam_openid_configuration"`
	IAMToken                   string `json:"iam_token"`
	IAMAuthorize               string `json:"iam_authorize"`
	IAMJWKS                    string `json:"iam_jwks"`
	KeycloakOpenIDConfiguration string `json:"keycloak_openid_configuration"`
	KeycloakToken              string `json:"keycloak_token"`
	KeycloakAuthorize          string `json:"keycloak_authorize"`
	KeycloakCerts              string `json:"keycloak_certs"`
}

// ──────────────────────────────────────────────
// Enhanced (v2) DTOs
// ──────────────────────────────────────────────

// CreateRealmRequest is the API request for creating a realm.
type CreateRealmRequest struct {
	Name                   string   `json:"name" binding:"required"`
	DisplayName            string   `json:"display_name"`
	Enabled                *bool    `json:"enabled"`
	RegistrationAllowed    *bool    `json:"registration_allowed"`
	ResetPasswordAllowed   *bool    `json:"reset_password_allowed"`
	RememberMe             *bool    `json:"remember_me"`
	VerifyEmail            *bool    `json:"verify_email"`
	LoginWithEmail         *bool    `json:"login_with_email"`
	DuplicateEmailsAllowed *bool    `json:"duplicate_emails_allowed"`
	SSLRequired            string   `json:"ssl_required"`
	DefaultRoles           []string `json:"default_roles"`
	AccessTokenLifespan    *int     `json:"access_token_lifespan"`
	RefreshTokenLifespan   *int     `json:"refresh_token_lifespan"`
	SSOSessionIdleTimeout  *int     `json:"sso_session_idle_timeout"`
	SSOSessionMaxLifespan  *int     `json:"sso_session_max_lifespan"`
	BruteForceProtected    *bool    `json:"brute_force_protected"`
	MaxFailureWaitSeconds  *int     `json:"max_failure_wait_seconds"`
	MaxLoginFailures       *int     `json:"max_login_failures"`
	PasswordMinLength      *int     `json:"password_min_length"`
	PasswordRequireUpper   *bool    `json:"password_require_upper"`
	PasswordRequireDigit   *bool    `json:"password_require_digit"`
	PasswordRequireSpecial *bool    `json:"password_require_special"`
}

// CreateGroupRequest is the API request for creating a group.
type CreateGroupRequest struct {
	RealmID  string `json:"realm_id" binding:"required"`
	Name     string `json:"name" binding:"required"`
	ParentID string `json:"parent_id"`
}

// GroupDetailResponse is the API response for a group with sub-groups and members.
type GroupDetailResponse struct {
	Group     *models.Group `json:"group"`
	SubGroups []*models.Group `json:"sub_groups"`
	Members   []string      `json:"members"`
}

// GroupMemberRequest is the API request for adding a member to a group.
type GroupMemberRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

// CreateClientScopeRequest is the API request for creating a client scope.
type CreateClientScopeRequest struct {
	RealmID          string `json:"realm_id" binding:"required"`
	Name             string `json:"name" binding:"required"`
	Description      string `json:"description"`
	Protocol         string `json:"protocol"`
	ClaimName        string `json:"claim_name"`
	ClaimType        string `json:"claim_type"`
	AddToIDToken     *bool  `json:"add_to_id_token"`
	AddToAccessToken *bool  `json:"add_to_access_token"`
	AddToUserInfo    *bool  `json:"add_to_userinfo"`
	BuiltIn          *bool  `json:"built_in"`
}

// CreateIdentityProviderRequest is the API request for creating an identity provider.
type CreateIdentityProviderRequest struct {
	RealmID          string `json:"realm_id" binding:"required"`
	Alias            string `json:"alias" binding:"required"`
	DisplayName      string `json:"display_name"`
	ProviderType     string `json:"provider_type" binding:"required"`
	Enabled          *bool  `json:"enabled"`
	TrustEmail       *bool  `json:"trust_email"`
	StoreToken       *bool  `json:"store_token"`
	AuthorizationURL string `json:"authorization_url"`
	TokenURL         string `json:"token_url"`
	UserInfoURL      string `json:"userinfo_url"`
	ClientID         string `json:"client_id"`
	ClientSecret     string `json:"client_secret"`
	Issuer           string `json:"issuer"`
	DefaultScopes    string `json:"default_scopes"`
	SyncMode         string `json:"sync_mode"`
}

// PublicIdPResponse is the public-facing identity provider response.
type PublicIdPResponse struct {
	ID               string `json:"id"`
	RealmID          string `json:"realm_id"`
	Alias            string `json:"alias"`
	DisplayName      string `json:"display_name"`
	ProviderType     string `json:"provider_type"`
	Enabled          bool   `json:"enabled"`
	AuthorizationURL string `json:"authorization_url,omitempty"`
	DefaultScopes    string `json:"default_scopes,omitempty"`
	ClientID         string `json:"client_id,omitempty"`
	Issuer           string `json:"issuer,omitempty"`
}

// ListPublicIdPsResponse is the API response for listing public identity providers.
type ListPublicIdPsResponse struct {
	IdentityProviders     []PublicIdPResponse `json:"identity_providers"`
	SupportedProviderTypes []string           `json:"supported_provider_types"`
}

// SetUserAttributeRequest is the API request for setting a user attribute.
type SetUserAttributeRequest struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value"`
}

// AddUserToGroupRequest is the API request for adding a user to a group.
type AddUserToGroupRequest struct {
	GroupID string `json:"group_id" binding:"required"`
}

// AddRequiredActionRequest is the API request for adding a required action.
type AddRequiredActionRequest struct {
	Action string `json:"action" binding:"required"`
}

// GetPGClientResponse is the API response for a PG client with roles.
type GetPGClientResponse struct {
	Client *models.Client `json:"client"`
	Roles  []*models.Role `json:"roles"`
}

// GetEffectiveRolesResponse is the API response for effective roles.
type GetEffectiveRolesResponse struct {
	UserID         string   `json:"user_id"`
	RealmID        string   `json:"realm_id"`
	EffectiveRoles []string `json:"effective_roles"`
}

// RealmDashboardResponse is the API response for the realm dashboard.
type RealmDashboardResponse struct {
	Realm              *models.Realm  `json:"realm"`
	ClientCount        int            `json:"client_count"`
	RoleCount          int            `json:"role_count"`
	GroupCount         int            `json:"group_count"`
	ScopeCount         int            `json:"scope_count"`
	IDPCount           int            `json:"idp_count"`
	UserCount          int64          `json:"user_count"`
	ActiveSessionCount int64          `json:"active_session_count"`
	RecentEvents       []models.Event `json:"recent_events"`
}

// RealmInfoResponse is the API response for realm info.
type RealmInfoResponse struct {
	Realm          string                   `json:"realm"`
	DisplayName    string                   `json:"display_name"`
	PublicKey      string                   `json:"public_key"`
	TokenService   string                   `json:"token_service"`
	Authorization  string                   `json:"authorization"`
	JWKS           string                   `json:"jwks"`
	Discovery      string                   `json:"discovery"`
	TokenSettings  RealmTokenSettings       `json:"token_settings"`
	LoginSettings  RealmLoginSettings       `json:"login_settings"`
	SecuritySettings RealmSecuritySettings  `json:"security_settings"`
}

// RealmTokenSettings holds realm token configuration.
type RealmTokenSettings struct {
	AccessTokenLifespan   int `json:"access_token_lifespan"`
	RefreshTokenLifespan  int `json:"refresh_token_lifespan"`
	SSOSessionIdleTimeout int `json:"sso_session_idle_timeout"`
	SSOSessionMaxLifespan int `json:"sso_session_max_lifespan"`
}

// RealmLoginSettings holds realm login configuration.
type RealmLoginSettings struct {
	RegistrationAllowed    bool `json:"registration_allowed"`
	ResetPasswordAllowed   bool `json:"reset_password_allowed"`
	RememberMe             bool `json:"remember_me"`
	VerifyEmail            bool `json:"verify_email"`
	LoginWithEmail         bool `json:"login_with_email"`
	DuplicateEmailsAllowed bool `json:"duplicate_emails_allowed"`
}

// RealmSecuritySettings holds realm security configuration.
type RealmSecuritySettings struct {
	BruteForceProtected    bool `json:"brute_force_protected"`
	MaxLoginFailures       int  `json:"max_login_failures"`
	MaxFailureWaitSeconds  int  `json:"max_failure_wait_seconds"`
	PasswordMinLength      int  `json:"password_min_length"`
	PasswordRequireUpper   bool `json:"password_require_upper"`
	PasswordRequireDigit   bool `json:"password_require_digit"`
	PasswordRequireSpecial bool `json:"password_require_special"`
}
