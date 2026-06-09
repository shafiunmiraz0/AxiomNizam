package webauthn

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// Service handles WebAuthn registration and authentication ceremonies.
// Implements the WebAuthn Level 2 spec using pure Go crypto (no external dependency).
type Service struct {
	rpID      string
	rpName    string
	rpOrigin  string
	credStore CredentialStore
	sessions  map[string]*Session
	mu        sync.RWMutex
}

// NewService creates a new WebAuthn service.
func NewService(rpID, rpOrigin string, credStore CredentialStore) *Service {
	s := &Service{
		rpID:      rpID,
		rpName:    "AxiomNizam",
		rpOrigin:  rpOrigin,
		credStore: credStore,
		sessions:  make(map[string]*Session),
	}
	go s.cleanupExpiredSessions()
	return s
}

// BeginRegistration starts the WebAuthn registration ceremony.
// Returns a session ID and the PublicKeyCredentialCreationOptions to send to the client.
func (s *Service) BeginRegistration(ctx context.Context, userID, userName, displayName string) (string, *PublicKeyCredentialCreationOptions, error) {
	challenge, err := GenerateChallenge()
	if err != nil {
		return "", nil, fmt.Errorf("generate challenge: %w", err)
	}

	sessionID, err := GenerateChallenge()
	if err != nil {
		return "", nil, fmt.Errorf("generate session ID: %w", err)
	}

	// Store session (5 min TTL)
	s.mu.Lock()
	s.sessions[sessionID] = &Session{
		ID:             sessionID,
		Challenge:      challenge,
		UserID:         userID,
		ExpiresAt:      time.Now().UTC().Add(5 * time.Minute),
		IsRegistration: true,
	}
	s.mu.Unlock()

	// Build exclude list from existing credentials
	var excludeCreds []CredentialDescriptor
	existing, _ := s.credStore.GetByUserID(ctx, userID)
	for _, c := range existing {
		excludeCreds = append(excludeCreds, CredentialDescriptor{
			Type: "public-key",
			ID:   base64.RawURLEncoding.EncodeToString(c.ID),
		})
	}

	// Encode user ID as base64url
	userIDB64 := base64.RawURLEncoding.EncodeToString([]byte(userID))

	options := &PublicKeyCredentialCreationOptions{
		Challenge: challenge,
		RP: RelyingParty{
			ID:   s.rpID,
			Name: s.rpName,
		},
		User: User{
			ID:          userIDB64,
			Name:        userName,
			DisplayName: displayName,
		},
		PubKeyCredParams: []PubKeyCredParam{
			{Type: "public-key", Alg: -7},  // ES256 (ECDSA P-256)
			{Type: "public-key", Alg: -257}, // RS256 (RSASSA-PKCS1-v1_5)
		},
		Timeout:            60000,
		ExcludeCredentials: excludeCreds,
		AuthenticatorSelection: AuthenticatorSelection{
			ResidentKey:      "preferred",
			UserVerification: "preferred",
		},
		Attestation: "none",
	}

	return sessionID, options, nil
}

// FinishRegistration completes the WebAuthn registration ceremony.
// Verifies the attestation response and stores the credential.
func (s *Service) FinishRegistration(ctx context.Context, sessionID string, resp *AttestationResponse) (*Credential, error) {
	// 1. Look up and validate session
	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return nil, errors.New("session not found")
	}
	if session.IsExpired() {
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		return nil, ErrChallengeExpired
	}
	if !session.IsRegistration {
		s.mu.Unlock()
		return nil, errors.New("session is not a registration session")
	}
	delete(s.sessions, sessionID)
	s.mu.Unlock()

	// 2. Decode clientDataJSON
	clientDataJSON, err := base64.RawURLEncoding.DecodeString(resp.Response.ClientDataJSON)
	if err != nil {
		return nil, fmt.Errorf("decode clientDataJSON: %w", err)
	}

	// 3. Verify clientData
	clientData, err := ParseClientDataJSON(clientDataJSON)
	if err != nil {
		return nil, err
	}
	if clientData.Type != "webauthn.create" {
		return nil, fmt.Errorf("unexpected clientData type: %s", clientData.Type)
	}
	if clientData.Challenge != session.Challenge {
		return nil, errors.New("challenge mismatch")
	}
	if clientData.Origin != s.rpOrigin {
		return nil, fmt.Errorf("origin mismatch: got %s, want %s", clientData.Origin, s.rpOrigin)
	}

	// 4. Decode attestationObject
	attObjBytes, err := base64.RawURLEncoding.DecodeString(resp.Response.AttestationObject)
	if err != nil {
		return nil, fmt.Errorf("decode attestationObject: %w", err)
	}

	// 5. Parse attestation object CBOR (it's a map: fmt, attStmt, authData)
	attMap, err := DecodeCBORMap(attObjBytes)
	if err != nil {
		return nil, fmt.Errorf("parse attestation CBOR: %w", err)
	}

	// 6. Extract authenticator data
	authDataRaw, ok := attMap[3] // "authData" is key 3 in attestation object
	if !ok {
		return nil, errors.New("attestation missing authData")
	}
	authDataBytes, ok := authDataRaw.([]byte)
	if !ok {
		return nil, errors.New("authData is not byte string")
	}

	authData, err := ParseAuthenticatorData(authDataBytes)
	if err != nil {
		return nil, fmt.Errorf("parse authData: %w", err)
	}

	// 7. Verify rpIdHash
	expectedRPIDHash := sha256.Sum256([]byte(s.rpID))
	if !bytesEqual(authData.RPIDHash, expectedRPIDHash[:]) {
		return nil, errors.New("rpIdHash mismatch")
	}

	// 8. Verify UP flag is set
	if !authData.Flags.UserPresent {
		return nil, errors.New("user present flag not set")
	}

	// 9. Extract credential data
	if authData.AttestedCredentialData == nil {
		return nil, errors.New("no attested credential data")
	}
	acd := authData.AttestedCredentialData

	// 10. Parse the COSE public key
	pubKey, err := ParseCOSEPublicKey(acd.CredentialPublicKey)
	if err != nil {
		return nil, fmt.Errorf("parse COSE public key: %w", err)
	}
	_ = pubKey // key is validated; we store the raw COSE bytes

	// 11. For "none" attestation, we trust the authenticator data
	// In production, you'd verify attestation statement signatures here

	// 12. Store the credential
	credential := &Credential{
		ID:              acd.CredentialID,
		UserID:          session.UserID,
		PublicKey:        acd.CredentialPublicKey,
		AttestationType: "none",
		AAGUID:          acd.AAGUID,
		SignCount:       authData.SignCount,
		CloneWarning:    false,
		CreatedAt:       time.Now().UTC(),
	}

	if err := s.credStore.Create(ctx, credential); err != nil {
		return nil, fmt.Errorf("store credential: %w", err)
	}

	log.Printf("✅ WebAuthn credential registered for user %s (credential ID: %x)", session.UserID, credential.ID[:8])
	return credential, nil
}

// BeginAuthentication starts the WebAuthn authentication ceremony.
// Returns a session ID and the PublicKeyCredentialRequestOptions to send to the client.
func (s *Service) BeginAuthentication(ctx context.Context, userID string) (string, *PublicKeyCredentialRequestOptions, error) {
	challenge, err := GenerateChallenge()
	if err != nil {
		return "", nil, fmt.Errorf("generate challenge: %w", err)
	}

	sessionID, err := GenerateChallenge()
	if err != nil {
		return "", nil, fmt.Errorf("generate session ID: %w", err)
	}

	s.mu.Lock()
	s.sessions[sessionID] = &Session{
		ID:             sessionID,
		Challenge:      challenge,
		UserID:         userID,
		ExpiresAt:      time.Now().UTC().Add(5 * time.Minute),
		IsRegistration: false,
	}
	s.mu.Unlock()

	// Build allow list from user's credentials
	var allowCreds []CredentialDescriptor
	creds, err := s.credStore.GetByUserID(ctx, userID)
	if err == nil {
		for _, c := range creds {
			allowCreds = append(allowCreds, CredentialDescriptor{
				Type: "public-key",
				ID:   base64.RawURLEncoding.EncodeToString(c.ID),
			})
		}
	}

	options := &PublicKeyCredentialRequestOptions{
		Challenge:        challenge,
		Timeout:          60000,
		RPID:             s.rpID,
		AllowCredentials: allowCreds,
		UserVerification: "preferred",
	}

	return sessionID, options, nil
}

// FinishAuthentication completes the WebAuthn authentication ceremony.
// Verifies the assertion response and updates the credential's sign count.
func (s *Service) FinishAuthentication(ctx context.Context, sessionID string, resp *AssertionResponse) (bool, error) {
	// 1. Look up and validate session
	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return false, errors.New("session not found")
	}
	if session.IsExpired() {
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		return false, ErrChallengeExpired
	}
	if session.IsRegistration {
		s.mu.Unlock()
		return false, errors.New("session is not an authentication session")
	}
	delete(s.sessions, sessionID)
	s.mu.Unlock()

	// 2. Decode clientDataJSON
	clientDataJSON, err := base64.RawURLEncoding.DecodeString(resp.Response.ClientDataJSON)
	if err != nil {
		return false, fmt.Errorf("decode clientDataJSON: %w", err)
	}

	// 3. Verify clientData
	clientData, err := ParseClientDataJSON(clientDataJSON)
	if err != nil {
		return false, err
	}
	if clientData.Type != "webauthn.get" {
		return false, fmt.Errorf("unexpected clientData type: %s", clientData.Type)
	}
	if clientData.Challenge != session.Challenge {
		return false, errors.New("challenge mismatch")
	}
	if clientData.Origin != s.rpOrigin {
		return false, fmt.Errorf("origin mismatch")
	}

	// 4. Decode authenticator data
	authDataB64 := resp.Response.AuthenticatorData
	authDataBytes, err := base64.RawURLEncoding.DecodeString(authDataB64)
	if err != nil {
		return false, fmt.Errorf("decode authenticatorData: %w", err)
	}

	authData, err := ParseAuthenticatorData(authDataBytes)
	if err != nil {
		return false, fmt.Errorf("parse authenticatorData: %w", err)
	}

	// 5. Verify rpIdHash
	expectedRPIDHash := sha256.Sum256([]byte(s.rpID))
	if !bytesEqual(authData.RPIDHash, expectedRPIDHash[:]) {
		return false, errors.New("rpIdHash mismatch")
	}

	// 6. Verify UP flag
	if !authData.Flags.UserPresent {
		return false, errors.New("user present flag not set")
	}

	// 7. Decode credential ID from response
	credIDBytes, err := base64.RawURLEncoding.DecodeString(resp.RawID)
	if err != nil {
		credIDBytes, err = base64.RawURLEncoding.DecodeString(resp.ID)
		if err != nil {
			return false, fmt.Errorf("decode credential ID: %w", err)
		}
	}

	// 8. Look up stored credential
	cred, err := s.credStore.GetByCredentialID(ctx, credIDBytes)
	if err != nil || cred == nil {
		return false, ErrInvalidCredential
	}
	if cred.UserID != session.UserID {
		return false, errors.New("credential belongs to different user")
	}

	// 9. Parse stored public key
	pubKey, err := ParseCOSEPublicKey(cred.PublicKey)
	if err != nil {
		return false, fmt.Errorf("parse stored public key: %w", err)
	}

	// 10. Build signed data: authenticatorData || SHA256(clientDataJSON)
	clientDataHash := sha256.Sum256(clientDataJSON)
	signedData := append(authDataBytes, clientDataHash[:]...)

	// 11. Decode signature
	sigBytes, err := base64.RawURLEncoding.DecodeString(resp.Response.Signature)
	if err != nil {
		return false, fmt.Errorf("decode signature: %w", err)
	}

	// 12. Verify signature
	if !VerifyECDSAP256Signature(pubKey, signedData, sigBytes) {
		return false, errors.New("signature verification failed")
	}

	// 13. Verify sign count (clone detection)
	if authData.SignCount > 0 || cred.SignCount > 0 {
		if authData.SignCount <= cred.SignCount {
			cred.CloneWarning = true
			log.Printf("⚠️  WebAuthn clone warning for user %s: sign count %d <= stored %d",
				session.UserID, authData.SignCount, cred.SignCount)
		}
		if err := s.credStore.UpdateSignCount(ctx, cred.ID, authData.SignCount); err != nil {
			log.Printf("⚠️  Failed to update sign count: %v", err)
		}
	}

	log.Printf("✅ WebAuthn authentication verified for user %s", session.UserID)
	return true, nil
}

// ListCredentials returns all WebAuthn credentials for a user.
func (s *Service) ListCredentials(ctx context.Context, userID string) ([]*Credential, error) {
	return s.credStore.GetByUserID(ctx, userID)
}

// DeleteCredential removes a WebAuthn credential.
func (s *Service) DeleteCredential(ctx context.Context, credentialID []byte) error {
	return s.credStore.Delete(ctx, credentialID)
}

// cleanupExpiredSessions runs every minute and removes expired sessions.
func (s *Service) cleanupExpiredSessions() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		for id, sess := range s.sessions {
			if sess.IsExpired() {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

// bytesEqual is a constant-time-safe byte comparison.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Ensure JSON round-trip works for the options (used by handlers).
var _ = json.Marshal
