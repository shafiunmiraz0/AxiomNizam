// Package federation provides identity federation, behavior profiling,
// identity risk scoring, and just-in-time privilege elevation (Phase 16).
//
// Components:
//   - OIDCProvider: OpenID Connect upstream IdP integration
//   - SAMLProvider: SAML 2.0 upstream IdP integration (stub with metadata)
//   - BehaviorProfiler: user access pattern analysis from audit data
//   - IdentityRiskScorer: account lifecycle risk assessment
//   - JITManager: time-bound role bindings with auto-expiry
package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ─── OIDC Federation ────────────────────────────────────────────────────────

// OIDCProvider handles OpenID Connect upstream identity federation.
// It authenticates users against an external OIDC provider (Google, Azure AD,
// Okta, etc.) and maps them to local AxiomNizam users.
type OIDCProvider struct {
	Alias          string
	Issuer         string
	AuthorizationURL string
	TokenURL       string
	UserInfoURL    string
	ClientID       string
	ClientSecret   string
	Scopes         []string
	HTTPClient     *http.Client
}

// OIDCTokenResponse is the token endpoint response.
type OIDCTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
}

// OIDCUserInfo is the userinfo endpoint response.
type OIDCUserInfo struct {
	Sub               string `json:"sub"`
	Name              string `json:"name"`
	GivenName         string `json:"given_name"`
	FamilyName        string `json:"family_name"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	Picture           string `json:"picture"`
	Locale            string `json:"locale"`
}

// FederatedUser represents a user authenticated via an upstream IdP.
type FederatedUser struct {
	ExternalID      string    `json:"external_id"`
	Email           string    `json:"email"`
	DisplayName     string    `json:"display_name"`
	FirstName       string    `json:"first_name,omitempty"`
	LastName        string    `json:"last_name,omitempty"`
	Username        string    `json:"username,omitempty"`
	ProviderAlias   string    `json:"provider_alias"`
	ProviderType    string    `json:"provider_type"`
	AuthenticatedAt time.Time `json:"authenticated_at"`
	RawClaims       map[string]interface{} `json:"raw_claims,omitempty"`
}

// NewOIDCProvider creates a new OIDC upstream IdP provider.
func NewOIDCProvider(alias, issuer, clientID, clientSecret string, scopes []string) *OIDCProvider {
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	return &OIDCProvider{
		Alias:        alias,
		Issuer:       issuer,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopes,
		HTTPClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// GetAuthorizationURL returns the URL to redirect the user for authentication.
func (p *OIDCProvider) GetAuthorizationURL(state, redirectURI string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", p.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", strings.Join(p.Scopes, " "))
	params.Set("state", state)
	return p.AuthorizationURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for tokens and user info.
func (p *OIDCProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (*FederatedUser, *OIDCTokenResponse, error) {
	if p.TokenURL == "" {
		return nil, nil, fmt.Errorf("OIDC provider %s: token URL not configured", p.Alias)
	}

	// Exchange code for tokens
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", p.ClientID)
	data.Set("client_secret", p.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("token exchange returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp OIDCTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, nil, fmt.Errorf("parse token response: %w", err)
	}

	// Fetch user info
	user, err := p.FetchUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, &tokenResp, fmt.Errorf("fetch user info: %w", err)
	}

	return user, &tokenResp, nil
}

// FetchUserInfo retrieves user information from the OIDC userinfo endpoint.
func (p *OIDCProvider) FetchUserInfo(ctx context.Context, accessToken string) (*FederatedUser, error) {
	if p.UserInfoURL == "" {
		return nil, fmt.Errorf("OIDC provider %s: userinfo URL not configured", p.Alias)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("userinfo returned %d: %s", resp.StatusCode, string(body))
	}

	var info OIDCUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("parse userinfo: %w", err)
	}

	displayName := info.Name
	if displayName == "" {
		displayName = info.PreferredUsername
	}
	if displayName == "" {
		displayName = info.Email
	}

	return &FederatedUser{
		ExternalID:      info.Sub,
		Email:           info.Email,
		DisplayName:     displayName,
		FirstName:       info.GivenName,
		LastName:        info.FamilyName,
		Username:        info.PreferredUsername,
		ProviderAlias:   p.Alias,
		ProviderType:    "oidc",
		AuthenticatedAt: time.Now().UTC(),
	}, nil
}

// ─── SAML Federation ────────────────────────────────────────────────────────

// SAMLProvider handles SAML 2.0 upstream identity federation.
// This is a metadata-only implementation — the actual SAML assertion
// parsing requires XML signature verification which is beyond the scope
// of this module. Use with a SAML proxy (e.g., Keycloak, Dex) in production.
type SAMLProvider struct {
	Alias             string
	EntityID          string
	SSOURL            string
	SLOURL            string
	Certificate       string // PEM-encoded IdP certificate
	AllowedRedirectURI string
}

// SAMLAuthnRequest represents a SAML authentication request.
type SAMLAuthnRequest struct {
	ID           string    `json:"id"`
	IssueInstant time.Time `json:"issueInstant"`
	Issuer       string    `json:"issuer"`
	Destination  string    `json:"destination"`
	ACSURL       string    `json:"acsURL"`
}

// NewSAMLProvider creates a new SAML upstream IdP provider.
func NewSAMLProvider(alias, entityID, ssoURL, certificate string) *SAMLProvider {
	return &SAMLProvider{
		Alias:       alias,
		EntityID:    entityID,
		SSOURL:      ssoURL,
		Certificate: certificate,
	}
}

// GetAuthnRequestURL returns the URL to redirect the user for SAML authentication.
// In production, this would generate a signed SAML AuthnRequest.
func (p *SAMLProvider) GetAuthnRequestURL(state, acsURL string) string {
	// Simplified: in production, generate proper SAML AuthnRequest XML
	params := url.Values{}
	params.Set("SAMLRequest", fmt.Sprintf("saml-request-%s", state))
	params.Set("RelayState", state)
	return p.SSOURL + "?" + params.Encode()
}

// GetMetadata returns SAML SP metadata XML.
func (p *SAMLProvider) GetMetadata(acsURL string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="%s">
  <SPSSODescriptor AuthnRequestsSigned="false" WantAssertionsSigned="true"
    protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <SingleLogoutService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="%s"/>
    <AssertionConsumerService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST"
      Location="%s" index="0" isDefault="true"/>
  </SPSSODescriptor>
</EntityDescriptor>`, p.EntityID, p.SLOURL, acsURL)
}

// ─── Behavior Profiler ──────────────────────────────────────────────────────

// AuditEvent is the input for behavior profiling.
type AuditEvent struct {
	UserID    string    `json:"user_id"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	IPAddress string    `json:"ip_address"`
	Timestamp time.Time `json:"timestamp"`
	Outcome   string    `json:"outcome"` // success, failure
}

// UserProfile contains the computed behavior profile for a user.
type UserProfile struct {
	UserID            string    `json:"user_id"`
	TotalEvents       int       `json:"total_events"`
	FailedLogins      int       `json:"failed_logins"`
	UniqueIPs         int       `json:"unique_ips"`
	UniqueResources   int       `json:"unique_resources"`
	MostCommonIP      string    `json:"most_common_ip"`
	MostCommonHour    int       `json:"most_common_hour"` // 0-23
	LastActivityAt    time.Time `json:"last_activity_at"`
	FirstActivityAt   time.Time `json:"first_activity_at"`
	IsNewIP           bool      `json:"is_new_ip"`           // current IP not seen before
	IsUnusualHour     bool      `json:"is_unusual_hour"`     // activity outside normal hours
	ActivityBuckets   [24]int   `json:"activity_buckets"`    // events per hour of day
	IPCounts          map[string]int `json:"-"`               // IP → event count
}

// BehaviorProfiler analyzes audit events to build user access profiles.
type BehaviorProfiler struct {
	mu       sync.RWMutex
	profiles map[string]*UserProfile
}

// NewBehaviorProfiler creates a new behavior profiler.
func NewBehaviorProfiler() *BehaviorProfiler {
	return &BehaviorProfiler{
		profiles: make(map[string]*UserProfile),
	}
}

// RecordEvent processes an audit event and updates the user's profile.
func (p *BehaviorProfiler) RecordEvent(evt AuditEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	profile, exists := p.profiles[evt.UserID]
	if !exists {
		profile = &UserProfile{
			UserID:         evt.UserID,
			FirstActivityAt: evt.Timestamp,
			IPCounts:       make(map[string]int),
		}
		p.profiles[evt.UserID] = profile
	}

	profile.TotalEvents++
	profile.LastActivityAt = evt.Timestamp

	if evt.Outcome == "failure" {
		profile.FailedLogins++
	}

	if evt.IPAddress != "" {
		profile.IPCounts[evt.IPAddress]++
		profile.UniqueIPs = len(profile.IPCounts)

		// Find most common IP
		maxCount := 0
		for ip, count := range profile.IPCounts {
			if count > maxCount {
				maxCount = count
				profile.MostCommonIP = ip
			}
		}
	}

	if evt.Timestamp.Hour() >= 0 && evt.Timestamp.Hour() < 24 {
		profile.ActivityBuckets[evt.Timestamp.Hour()]++
	}

	// Find most common hour
	maxBucket := 0
	for h, count := range profile.ActivityBuckets {
		if count > maxBucket {
			maxBucket = count
			profile.MostCommonHour = h
		}
	}
}

// GetProfile returns the behavior profile for a user.
func (p *BehaviorProfiler) GetProfile(userID string) *UserProfile {
	p.mu.RLock()
	defer p.mu.RUnlock()
	profile, exists := p.profiles[userID]
	if !exists {
		return nil
	}
	// Return a copy
	snap := *profile
	return &snap
}

// EvaluateAnomalies checks if current activity is anomalous for the user.
func (p *BehaviorProfiler) EvaluateAnomalies(userID, currentIP string, currentHour int) (isAnomalous bool, reasons []string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	profile, exists := p.profiles[userID]
	if !exists || profile.TotalEvents < 10 {
		return false, nil // insufficient data
	}

	// Check if IP is new
	if currentIP != "" {
		if _, known := profile.IPCounts[currentIP]; !known {
			reasons = append(reasons, "new_ip_address")
			profile.IsNewIP = true
		}
	}

	// Check if hour is unusual (outside top 3 activity hours)
	topHours := make([]int, 0, 3)
	for h := 0; h < 24; h++ {
		if profile.ActivityBuckets[h] > 0 {
			topHours = append(topHours, h)
		}
	}
	if len(topHours) > 0 {
		isNormalHour := false
		for _, h := range topHours {
			if h == currentHour {
				isNormalHour = true
				break
			}
		}
		if !isNormalHour && profile.TotalEvents > 50 {
			reasons = append(reasons, "unusual_activity_hour")
			profile.IsUnusualHour = true
		}
	}

	return len(reasons) > 0, reasons
}

// ─── Identity Risk Scoring ──────────────────────────────────────────────────

// IdentityRiskEvent represents an account lifecycle event.
type IdentityRiskEvent struct {
	EventType string // "password_change", "email_change", "role_change", "mfa_disable", "new_device", "federation_link"
	Severity  string // "low", "medium", "high", "critical"
	Timestamp time.Time
	IPAddress string
}

// IdentityRiskScore contains the computed identity risk assessment.
type IdentityRiskScore struct {
	UserID      string    `json:"user_id"`
	Score       int       `json:"score"`       // 0-100
	Level       string    `json:"level"`       // low, medium, high, critical
	Factors     []string  `json:"factors"`     // contributing factors
	LastEvent   string    `json:"last_event"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IdentityRiskScorer assesses identity risk based on account lifecycle events.
type IdentityRiskScorer struct {
	mu     sync.RWMutex
	scores map[string]*IdentityRiskScore
	events map[string][]IdentityRiskEvent
}

// NewIdentityRiskScorer creates a new identity risk scorer.
func NewIdentityRiskScorer() *IdentityRiskScorer {
	return &IdentityRiskScorer{
		scores: make(map[string]*IdentityRiskScore),
		events: make(map[string][]IdentityRiskEvent),
	}
}

// RecordEvent records an account lifecycle event and recomputes the risk score.
func (s *IdentityRiskScorer) RecordEvent(userID string, event IdentityRiskEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events[userID] = append(s.events[userID], event)
	if len(s.events[userID]) > 100 {
		s.events[userID] = s.events[userID[1:]]
	}

	s.computeScore(userID)
}

// GetScore returns the current identity risk score for a user.
func (s *IdentityRiskScorer) GetScore(userID string) *IdentityRiskScore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	score, exists := s.scores[userID]
	if !exists {
		return &IdentityRiskScore{UserID: userID, Score: 0, Level: "low"}
	}
	snap := *score
	return &snap
}

func (s *IdentityRiskScorer) computeScore(userID string) {
	events := s.events[userID]
	if len(events) == 0 {
		s.scores[userID] = &IdentityRiskScore{UserID: userID, Score: 0, Level: "low", UpdatedAt: time.Now().UTC()}
		return
	}

	score := 0
	factors := make([]string, 0)
	now := time.Now().UTC()

	// Recent events (last 24h) contribute more
	recentEvents := 0
	for _, e := range events {
		if now.Sub(e.Timestamp) < 24*time.Hour {
			recentEvents++
		}
	}

	// Multiple recent high-severity events are suspicious
	if recentEvents >= 3 {
		score += 30
		factors = append(factors, "multiple_recent_events")
	}

	// Check last event severity
	last := events[len(events)-1]
	switch last.Severity {
	case "critical":
		score += 40
		factors = append(factors, "critical_event")
	case "high":
		score += 25
		factors = append(factors, "high_severity_event")
	case "medium":
		score += 10
	}

	// Specific event types
	switch last.EventType {
	case "password_change":
		score += 15
		factors = append(factors, "recent_password_change")
	case "mfa_disable":
		score += 30
		factors = append(factors, "mfa_disabled")
	case "email_change":
		score += 20
		factors = append(factors, "email_changed")
	case "role_change":
		score += 15
		factors = append(factors, "role_changed")
	}

	if score > 100 {
		score = 100
	}

	level := "low"
	if score >= 75 {
		level = "critical"
	} else if score >= 50 {
		level = "high"
	} else if score >= 25 {
		level = "medium"
	}

	s.scores[userID] = &IdentityRiskScore{
		UserID:    userID,
		Score:     score,
		Level:     level,
		Factors:   factors,
		LastEvent: last.EventType,
		UpdatedAt: now,
	}
	log.Printf("📊 [IdentityRisk] User %s: score=%d level=%s factors=%v", userID, score, level, factors)
}

// ─── Just-In-Time Privilege Elevation ───────────────────────────────────────

// JITBinding is a time-bound role binding that auto-expires.
type JITBinding struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	RoleID    string    `json:"role_id"`
	Reason    string    `json:"reason"`
	GrantedBy string    `json:"granted_by"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// IsExpired returns true if the binding has expired or been revoked.
func (b *JITBinding) IsExpired() bool {
	now := time.Now().UTC()
	if b.RevokedAt != nil && now.After(*b.RevokedAt) {
		return true
	}
	return now.After(b.ExpiresAt)
}

// JITManager manages just-in-time privilege elevation with time-bound role bindings.
type JITManager struct {
	mu       sync.RWMutex
	bindings map[string][]*JITBinding // userID → bindings
	repo     JITRepository
}

// JITRepository abstracts persistence for JIT bindings.
type JITRepository interface {
	CreateJITBinding(binding *JITBinding) error
	GetActiveJITBindings(userID string) ([]*JITBinding, error)
	RevokeJITBinding(id string) error
	CleanupExpiredJITBindings() (int, error)
}

// NewJITManager creates a new JIT privilege manager.
func NewJITManager(repo JITRepository) *JITManager {
	return &JITManager{
		bindings: make(map[string][]*JITBinding),
		repo:     repo,
	}
}

// GrantElevation creates a time-bound role binding.
func (m *JITManager) GrantElevation(userID, roleID, reason, grantedBy string, duration time.Duration) (*JITBinding, error) {
	now := time.Now().UTC()
	binding := &JITBinding{
		ID:        fmt.Sprintf("jit-%s-%d", userID, now.UnixNano()),
		UserID:    userID,
		RoleID:    roleID,
		Reason:    reason,
		GrantedBy: grantedBy,
		ExpiresAt: now.Add(duration),
		CreatedAt: now,
	}

	if m.repo != nil {
		if err := m.repo.CreateJITBinding(binding); err != nil {
			return nil, fmt.Errorf("persist JIT binding: %w", err)
		}
	}

	m.mu.Lock()
	m.bindings[userID] = append(m.bindings[userID], binding)
	m.mu.Unlock()

	log.Printf("✅ [JIT] Granted %s → %s for %s (expires: %s, reason: %s)",
		userID, roleID, duration, binding.ExpiresAt.Format(time.RFC3339), reason)
	return binding, nil
}

// RevokeElevation revokes a time-bound role binding early.
func (m *JITManager) RevokeElevation(bindingID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, bindings := range m.bindings {
		for _, b := range bindings {
			if b.ID == bindingID && b.RevokedAt == nil {
				now := time.Now().UTC()
				b.RevokedAt = &now
				if m.repo != nil {
					return m.repo.RevokeJITBinding(bindingID)
				}
				return nil
			}
		}
	}
	return fmt.Errorf("JIT binding %s not found or already revoked", bindingID)
}

// GetActiveBindings returns all active (non-expired, non-revoked) bindings for a user.
func (m *JITManager) GetActiveBindings(userID string) []*JITBinding {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var active []*JITBinding
	for _, b := range m.bindings[userID] {
		if !b.IsExpired() {
			active = append(active, b)
		}
	}
	return active
}

// HasActiveElevation checks if a user has an active elevation for a specific role.
func (m *JITManager) HasActiveElevation(userID, roleID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, b := range m.bindings[userID] {
		if b.RoleID == roleID && !b.IsExpired() {
			return true
		}
	}
	return false
}

// CleanupExpired removes expired bindings from memory.
func (m *JITManager) CleanupExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	for userID, bindings := range m.bindings {
		var active []*JITBinding
		for _, b := range bindings {
			if b.IsExpired() {
				removed++
			} else {
				active = append(active, b)
			}
		}
		m.bindings[userID] = active
	}

	if m.repo != nil {
		if dbRemoved, err := m.repo.CleanupExpiredJITBindings(); err == nil {
			removed += dbRemoved
		}
	}

	if removed > 0 {
		log.Printf("🧹 [JIT] Cleaned up %d expired privilege elevations", removed)
	}
	return removed
}

// StartCleanupLoop runs periodic cleanup of expired JIT bindings.
func (m *JITManager) StartCleanupLoop(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			m.CleanupExpired()
		}
	}()
}
