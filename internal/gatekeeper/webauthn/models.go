package webauthn

import (
	"context"
	"time"
)

// Credential represents a stored WebAuthn credential (public key + metadata).
type Credential struct {
	ID              []byte    `json:"id"`
	UserID          string    `json:"user_id"`
	PublicKey        []byte    `json:"public_key"`
	AttestationType string    `json:"attestation_type"`
	AAGUID          []byte    `json:"aaguid"`
	SignCount       uint32    `json:"sign_count"`
	CloneWarning    bool      `json:"clone_warning"`
	CreatedAt       time.Time `json:"created_at"`
}

// Session represents an in-progress WebAuthn ceremony (registration or authentication).
// Sessions are short-lived (5 min TTL) and stored in-memory.
type Session struct {
	ID             string    `json:"id"`
	Challenge      string    `json:"challenge"`
	UserID         string    `json:"user_id"`
	ExpiresAt      time.Time `json:"expires_at"`
	IsRegistration bool      `json:"is_registration"`
}

// IsExpired returns true if the session has passed its TTL.
func (s *Session) IsExpired() bool {
	return time.Now().UTC().After(s.ExpiresAt)
}

// RegistrationSession is returned to the client after BeginRegistration.
type RegistrationSession struct {
	SessionID  string                           `json:"session_id"`
	Options    *PublicKeyCredentialCreationOptions `json:"options"`
}

// AuthenticationSession is returned to the client after BeginAuthentication.
type AuthenticationSession struct {
	SessionID  string                           `json:"session_id"`
	Options    *PublicKeyCredentialRequestOptions  `json:"options"`
}

// PublicKeyCredentialCreationOptions is the WebAuthn registration options sent to the client.
// See: https://www.w3.org/TR/webauthn/#dictdef-publickeycredentialcreationoptions
type PublicKeyCredentialCreationOptions struct {
	Challenge              string                `json:"challenge"`
	RP                     RelyingParty          `json:"rp"`
	User                   User                  `json:"user"`
	PubKeyCredParams       []PubKeyCredParam     `json:"pubKeyCredParams"`
	Timeout                int                   `json:"timeout,omitempty"`
	ExcludeCredentials     []CredentialDescriptor `json:"excludeCredentials,omitempty"`
	AuthenticatorSelection AuthenticatorSelection `json:"authenticatorSelection,omitempty"`
	Attestation            string                `json:"attestation"`
}

// PublicKeyCredentialRequestOptions is the WebAuthn authentication options sent to the client.
type PublicKeyCredentialRequestOptions struct {
	Challenge        string                `json:"challenge"`
	Timeout          int                   `json:"timeout,omitempty"`
	RPID             string                `json:"rpId"`
	AllowCredentials []CredentialDescriptor `json:"allowCredentials,omitempty"`
	UserVerification string                `json:"userVerification"`
}

// RelyingParty identifies the server.
type RelyingParty struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// User identifies the user account.
type User struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// PubKeyCredParam specifies a supported public key credential type and algorithm.
type PubKeyCredParam struct {
	Type string `json:"type"`
	Alg  int    `json:"alg"`
}

// CredentialDescriptor references an existing credential (for exclude/allow lists).
type CredentialDescriptor struct {
	Type       string   `json:"type"`
	ID         string   `json:"id"`
	Transports []string `json:"transports,omitempty"`
}

// AuthenticatorSelection specifies authenticator attachment and resident key requirements.
type AuthenticatorSelection struct {
	AuthenticatorAttachment string `json:"authenticatorAttachment,omitempty"`
	ResidentKey             string `json:"residentKey,omitempty"`
	UserVerification        string `json:"userVerification"`
}

// AttestationResponse is the client response for registration.
type AttestationResponse struct {
	ID       string                       `json:"id"`
	RawID    string                       `json:"rawId"`
	Type     string                       `json:"type"`
	Response AttestationResponseData      `json:"response"`
}

// AttestationResponseData contains the raw attestation data from the client.
type AttestationResponseData struct {
	ClientDataJSON    string `json:"clientDataJSON"`
	AttestationObject string `json:"attestationObject"`
}

// AssertionResponse is the client response for authentication.
type AssertionResponse struct {
	ID       string                    `json:"id"`
	RawID    string                    `json:"rawId"`
	Type     string                    `json:"type"`
	Response AssertionResponseData     `json:"response"`
}

// AssertionResponseData contains the raw assertion data from the client.
type AssertionResponseData struct {
	ClientDataJSON string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature     string `json:"signature"`
	UserHandle    string `json:"userHandle,omitempty"`
}

// CredentialStore abstracts persistence for WebAuthn credentials.
type CredentialStore interface {
	Create(ctx context.Context, cred *Credential) error
	GetByUserID(ctx context.Context, userID string) ([]*Credential, error)
	GetByCredentialID(ctx context.Context, credentialID []byte) (*Credential, error)
	UpdateSignCount(ctx context.Context, credentialID []byte, newCount uint32) error
	Delete(ctx context.Context, credentialID []byte) error
}
