package admin

import (
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/iam/authz"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/identity"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/oauth"
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/token"
)

// ──────────────────────────────────────────────
// User Mappers
// ──────────────────────────────────────────────

// UserToResponse converts an identity.User to a UserResponse.
func UserToResponse(u *identity.User) UserResponse {
	return UserResponse{
		ID:            u.ID,
		Email:         u.Email,
		DisplayName:   u.DisplayName,
		Roles:         u.Roles,
		Active:        u.Active,
		EmailVerified: u.EmailVerified,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
	}
}

// UsersToResponse converts a slice of identity.User to ListUsersResponse.
func UsersToResponse(users []*identity.User) ListUsersResponse {
	result := make([]UserResponse, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		result = append(result, UserToResponse(u))
	}
	return ListUsersResponse{
		Users: result,
		Count: len(result),
	}
}

// ──────────────────────────────────────────────
// Client Mappers
// ──────────────────────────────────────────────

// ClientToResponse converts an OAuthClient to a ClientResponse (no secret).
func ClientToResponse(cl *oauth.OAuthClient) ClientResponse {
	return ClientResponse{
		ID:                   cl.ID,
		Name:                 cl.Name,
		RedirectURIs:         cl.RedirectURIs,
		Scopes:               cl.Scopes,
		GrantTypes:           cl.GrantTypes,
		ServiceRoles:         cl.ServiceRoles,
		RateLimitMaxCalls:    resolvedClientRateLimitMaxCalls(cl),
		TokenValidityMinutes: resolvedClientTokenValidityMinutes(cl, 15*time.Minute),
		Public:               cl.Public,
		Active:               cl.Active,
		CreatedAt:            cl.CreatedAt,
	}
}

// ClientsToResponse converts a slice of OAuthClient to ListClientsResponse.
func ClientsToResponse(clients []*oauth.OAuthClient, fallbackTTL time.Duration) ListClientsResponse {
	result := make([]ClientResponse, 0, len(clients))
	for _, cl := range clients {
		if cl == nil {
			continue
		}
		resp := ClientResponse{
			ID:                   cl.ID,
			Name:                 cl.Name,
			RedirectURIs:         cl.RedirectURIs,
			Scopes:               cl.Scopes,
			GrantTypes:           cl.GrantTypes,
			ServiceRoles:         cl.ServiceRoles,
			RateLimitMaxCalls:    resolvedClientRateLimitMaxCalls(cl),
			TokenValidityMinutes: resolvedClientTokenValidityMinutes(cl, fallbackTTL),
			Public:               cl.Public,
			Active:               cl.Active,
			CreatedAt:            cl.CreatedAt,
		}
		result = append(result, resp)
	}
	return ListClientsResponse{
		Clients: result,
		Count:   len(result),
	}
}

// ClientToCreatedResponse converts an OAuthClient to a ClientCreatedResponse (with optional secret).
func ClientToCreatedResponse(cl *oauth.OAuthClient, secret string) ClientCreatedResponse {
	resp := ClientCreatedResponse{
		ID:                   cl.ID,
		Name:                 cl.Name,
		RedirectURIs:         cl.RedirectURIs,
		Scopes:               cl.Scopes,
		GrantTypes:           cl.GrantTypes,
		ServiceRoles:         cl.ServiceRoles,
		RateLimitMaxCalls:    cl.RateLimitMaxCalls,
		TokenValidityMinutes: cl.TokenValidityMinutes,
		Public:               cl.Public,
		CreatedAt:            cl.CreatedAt,
	}
	if secret != "" {
		resp.ClientSecret = secret
		resp.Warning = "Store the client_secret securely. It will not be shown again."
	}
	return resp
}

// ClientToRegenerateSecretResponse builds the response after secret regeneration.
func ClientToRegenerateSecretResponse(cl *oauth.OAuthClient, newSecret string) RegenerateSecretResponse {
	return RegenerateSecretResponse{
		ID:                   cl.ID,
		ClientID:             cl.ID,
		ClientSecret:         newSecret,
		Scopes:               cl.Scopes,
		GrantTypes:           cl.GrantTypes,
		RateLimitMaxCalls:    resolvedClientRateLimitMaxCalls(cl),
		TokenValidityMinutes: resolvedClientTokenValidityMinutes(cl, 15*time.Minute),
		Warning:              "Store the client_secret securely. It will not be shown again.",
	}
}

// ClientToChangeIDResponse builds the response after changing a client ID.
func ClientToChangeIDResponse(oldID string, cl *oauth.OAuthClient) ChangeClientIDResponse {
	return ChangeClientIDResponse{
		Message:               "client id updated",
		OldClientID:           oldID,
		NewClientID:           cl.ID,
		RedirectURIs:          cl.RedirectURIs,
		Scopes:                cl.Scopes,
		GrantTypes:            cl.GrantTypes,
		ServiceRoles:          cl.ServiceRoles,
		RateLimitMaxCalls:     resolvedClientRateLimitMaxCalls(cl),
		TokenValidityMinutes:  resolvedClientTokenValidityMinutes(cl, 15*time.Minute),
		Public:                cl.Public,
		Active:                cl.Active,
		CreatedAt:             cl.CreatedAt,
	}
}

// ──────────────────────────────────────────────
// Role Mappers
// ──────────────────────────────────────────────

// RolesToListResponse converts a slice of authz.Role to ListRolesResponse.
func RolesToListResponse(roles []*authz.Role) ListRolesResponse {
	return ListRolesResponse{
		Roles: roles,
		Count: len(roles),
	}
}

// ──────────────────────────────────────────────
// Binding Mappers
// ──────────────────────────────────────────────

// BindingsToListResponse converts a slice of authz.RoleBinding to ListBindingsResponse.
func BindingsToListResponse(bindings []*authz.RoleBinding) ListBindingsResponse {
	return ListBindingsResponse{
		Bindings: bindings,
		Count:    len(bindings),
	}
}

// ──────────────────────────────────────────────
// Token Mappers
// ──────────────────────────────────────────────

// WhoAmIFromClaims creates a WhoAmIResponse from JWT claims.
func WhoAmIFromClaims(claims *token.IAMClaims) WhoAmIResponse {
	return WhoAmIResponse{
		UserID:      claims.Sub,
		Email:       claims.Email,
		DisplayName: claims.DisplayName,
		Roles:       claims.Roles,
	}
}

// LogoutResponseFromState creates a LogoutResponse from revocation states.
func LogoutResponseFromState(accessRevoked, sessionRevoked, refreshRevoked bool) LogoutResponse {
	return LogoutResponse{
		Status:               "ok",
		AccessTokenRevoked:   accessRevoked,
		SessionRevoked:       sessionRevoked,
		RefreshTokensRevoked: refreshRevoked,
	}
}

// ClientCredentialsToResponse builds a ClientCredentialsResponse.
func ClientCredentialsToResponse(accessToken *token.AccessTokenResponse, client *oauth.OAuthClient, fallbackTTL time.Duration) ClientCredentialsResponse {
	return ClientCredentialsResponse{
		AccessToken:          accessToken.AccessToken,
		TokenType:            accessToken.TokenType,
		ExpiresIn:            accessToken.ExpiresIn,
		Scope:                accessToken.Scope,
		RateLimitMaxCalls:    resolvedClientRateLimitMaxCalls(client),
		TokenValidityMinutes: resolvedClientTokenValidityMinutes(client, fallbackTTL),
	}
}

// ──────────────────────────────────────────────
// Enhanced (v2) Mappers
// ──────────────────────────────────────────────

// GroupToDetailResponse creates a GroupDetailResponse.
func GroupToDetailResponse(group *models.Group, subGroups []*models.Group, members []string) GroupDetailResponse {
	return GroupDetailResponse{
		Group:     group,
		SubGroups: subGroups,
		Members:   members,
	}
}

// IdPToPublicResponse converts an IdentityProvider to a PublicIdPResponse.
func IdPToPublicResponse(idp *models.IdentityProvider) PublicIdPResponse {
	return PublicIdPResponse{
		ID:               idp.ID,
		RealmID:          idp.RealmID,
		Alias:            idp.Alias,
		DisplayName:      idp.DisplayName,
		ProviderType:     strings.ToLower(strings.TrimSpace(idp.ProviderType)),
		Enabled:          idp.Enabled,
		AuthorizationURL: strings.TrimSpace(idp.AuthorizationURL),
		DefaultScopes:    strings.TrimSpace(idp.DefaultScopes),
		ClientID:         strings.TrimSpace(idp.ClientID),
		Issuer:           strings.TrimSpace(idp.Issuer),
	}
}

// IdPsToPublicResponse converts a slice of IdentityProvider to ListPublicIdPsResponse.
func IdPsToPublicResponse(idps []*models.IdentityProvider) ListPublicIdPsResponse {
	providers := make([]PublicIdPResponse, 0, len(idps))
	for _, idp := range idps {
		if idp == nil || !idp.Enabled {
			continue
		}
		providers = append(providers, IdPToPublicResponse(idp))
	}
	return ListPublicIdPsResponse{
		IdentityProviders: providers,
		SupportedProviderTypes: []string{
			"oidc", "saml", "github", "google", "ldap", "microsoft", "gitlab", "facebook",
		},
	}
}

// PGClientToGetResponse creates a GetPGClientResponse.
func PGClientToGetResponse(client *models.Client, roles []*models.Role) GetPGClientResponse {
	return GetPGClientResponse{
		Client: client,
		Roles:  roles,
	}
}

// EffectiveRolesToResponse creates a GetEffectiveRolesResponse.
func EffectiveRolesToResponse(userID, realmID string, roles []string) GetEffectiveRolesResponse {
	return GetEffectiveRolesResponse{
		UserID:         userID,
		RealmID:        realmID,
		EffectiveRoles: roles,
	}
}

// RealmDashboardToResponse creates a RealmDashboardResponse.
func RealmDashboardToResponse(
	realm *models.Realm,
	clientCount, roleCount, groupCount, scopeCount, idpCount int,
	userCount, activeSessionCount int64,
	recentEvents []models.Event,
) RealmDashboardResponse {
	return RealmDashboardResponse{
		Realm:              realm,
		ClientCount:        clientCount,
		RoleCount:          roleCount,
		GroupCount:         groupCount,
		ScopeCount:         scopeCount,
		IDPCount:           idpCount,
		UserCount:          userCount,
		ActiveSessionCount: activeSessionCount,
		RecentEvents:       recentEvents,
	}
}

// RealmInfoToResponse creates a RealmInfoResponse.
func RealmInfoToResponse(realm *models.Realm, realmBase string) RealmInfoResponse {
	return RealmInfoResponse{
		Realm:         realm.Name,
		DisplayName:   realm.DisplayName,
		PublicKey:     "(available at JWKS endpoint)",
		TokenService:  "/realms/" + realm.Name + "/protocol/openid-connect/token",
		Authorization: "/realms/" + realm.Name + "/protocol/openid-connect/auth",
		JWKS:          "/realms/" + realm.Name + "/protocol/openid-connect/certs",
		Discovery:     "/realms/" + realm.Name + "/.well-known/openid-configuration",
		TokenSettings: RealmTokenSettings{
			AccessTokenLifespan:   realm.AccessTokenLifespan,
			RefreshTokenLifespan:  realm.RefreshTokenLifespan,
			SSOSessionIdleTimeout: realm.SSOSessionIdleTimeout,
			SSOSessionMaxLifespan: realm.SSOSessionMaxLifespan,
		},
		LoginSettings: RealmLoginSettings{
			RegistrationAllowed:    realm.RegistrationAllowed,
			ResetPasswordAllowed:   realm.ResetPasswordAllowed,
			RememberMe:             realm.RememberMe,
			VerifyEmail:            realm.VerifyEmail,
			LoginWithEmail:         realm.LoginWithEmail,
			DuplicateEmailsAllowed: realm.DuplicateEmailsAllowed,
		},
		SecuritySettings: RealmSecuritySettings{
			BruteForceProtected:    realm.BruteForceProtected,
			MaxLoginFailures:       realm.MaxLoginFailures,
			MaxFailureWaitSeconds:  realm.MaxFailureWaitSeconds,
			PasswordMinLength:      realm.PasswordMinLength,
			PasswordRequireUpper:   realm.PasswordRequireUpper,
			PasswordRequireDigit:   realm.PasswordRequireDigit,
			PasswordRequireSpecial: realm.PasswordRequireSpecial,
		},
	}
}

// MaskClientSecret masks the secret field for list responses.
func MaskClientSecret(client *models.Client) {
	if client.Secret != "" {
		client.Secret = "**********"
	}
}
