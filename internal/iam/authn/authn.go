package authn

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"example.com/axiomnizam/internal/iam/identity"
)

// Session represents an authenticated session.
type Session struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	AccessToken  string    `json:"access_token,omitempty"` // set after token issuance
	RefreshToken string    `json:"refresh_token,omitempty"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	CreatedAt    time.Time `json:"created_at"`
	LastAccessAt time.Time `json:"last_access_at"` // Phase 11: updated on each authenticated request
	ExpiresAt    time.Time `json:"expires_at"`
	Active       bool      `json:"active"`
}

// LoginRequest is the credential payload.
type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse returned upon successful authentication.
type LoginResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
	User         UserInfo  `json:"user"`
}

// UserInfo is a safe projection of identity.User.
type UserInfo struct {
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	DisplayName   string   `json:"display_name"`
	Roles         []string `json:"roles"`
	EmailVerified bool     `json:"email_verified"`
}

// Authenticator validates credentials and produces sessions.
type Authenticator struct {
	userRepo    UserRepository
	sessionRepo SessionRepository
}

// UserRepository is the minimum user lookup contract.
type UserRepository interface {
	GetByLoginIdentifier(identifier string) (*identity.User, error)
	GetByEmail(email string) (*identity.User, error)
	GetByID(id string) (*identity.User, error)
	Create(user *identity.User) error
	Update(user *identity.User) error
}

// SessionRepository persists sessions.
type SessionRepository interface {
	Create(s *Session) error
	GetByID(id string) (*Session, error)
	RevokeByUserID(userID string) error
	Revoke(sessionID string) error
}

// NewAuthenticator wires dependencies.
func NewAuthenticator(users UserRepository, sessions SessionRepository) *Authenticator {
	return &Authenticator{userRepo: users, sessionRepo: sessions}
}

// Authenticate validates identifier/password and returns the user on success.
func (a *Authenticator) Authenticate(identifier, password string) (*identity.User, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || password == "" {
		return nil, errors.New("email and password are required")
	}

	user, err := a.userRepo.GetByLoginIdentifier(identifier)
	if err != nil {
		return nil, errors.New("invalid credentials")
	}
	if user == nil {
		return nil, errors.New("invalid credentials")
	}
	if !user.Active {
		return nil, errors.New("account is disabled")
	}
	if !identity.CheckPassword(password, user.PasswordHash) {
		return nil, errors.New("invalid credentials")
	}
	return user, nil
}

// CreateSession persists a new session record.
func (a *Authenticator) CreateSession(userID, ip, userAgent string, ttl time.Duration) (*Session, error) {
	sid, err := generateSessionID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	s := &Session{
		ID:           sid,
		UserID:       userID,
		IPAddress:    ip,
		UserAgent:    userAgent,
		CreatedAt:    now,
		LastAccessAt: now,
		ExpiresAt:    now.Add(ttl),
		Active:       true,
	}
	if err := a.sessionRepo.Create(s); err != nil {
		return nil, err
	}
	return s, nil
}

func generateSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
