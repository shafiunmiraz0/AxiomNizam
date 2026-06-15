// Package totp implements RFC 6238 Time-based One-Time Password generation and validation.
package totp

import (
	"context"
	"encoding/base32"
	"errors"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// Service generates and validates TOTP (Time-based One-Time Password) OTPs.
type Service struct {
	secretGenerator SecretGenerator
	validator       Validator
	issuer          IssuerProvider
	clock           Clock
}

// NewService creates a new TOTP service.
func NewService(sg SecretGenerator, v Validator, ip IssuerProvider, c Clock) *Service {
	return &Service{
		secretGenerator: sg,
		validator:       v,
		issuer:          ip,
		clock:           c,
	}
}

// GenerateSecret generates a new TOTP secret for a user.
// Returns the base32-encoded secret and OTPAuth URI for QR code generation.
func (s *Service) GenerateSecret(ctx context.Context, userID models.UserID, accountName, issuerName string) (secret, otpAuthURI string, err error) {
	// Generate 32-byte (256-bit) cryptographically secure random secret
	secretBytes, err := s.secretGenerator.Generate()
	if err != nil {
		return "", "", err
	}

	secret = base32.StdEncoding.EncodeToString(secretBytes)

	// Build OTPAuth URI for QR code
	uri := s.issuer.BuildOTPAuthURI(secret, accountName, issuerName)
	return secret, uri, nil
}

// ValidateCode verifies a 6-digit TOTP code against a secret.
// Allows a time window of ±1 time step (default 30 seconds) for clock skew.
func (s *Service) ValidateCode(ctx context.Context, secret, code string) (bool, error) {
	if secret == "" || code == "" {
		return false, errors.New("secret and code are required")
	}

	// Decode the secret from base32
	secretBytes, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return false, errors.New("invalid secret encoding")
	}

	// Get current time
	now := s.clock.Now()

	// Validate against current time step and adjacent time steps (clock skew)
	if s.validator.Validate(secretBytes, code, now, TimeStepSeconds) {
		return true, nil
	}

	// Check previous time step
	if s.validator.Validate(secretBytes, code, now.Add(-time.Duration(TimeStepSeconds)*time.Second), TimeStepSeconds) {
		return true, nil
	}

	// Check next time step
	if s.validator.Validate(secretBytes, code, now.Add(time.Duration(TimeStepSeconds)*time.Second), TimeStepSeconds) {
		return true, nil
	}

	return false, nil
}

// GenerateRecoveryCodes generates a set of one-time backup codes for emergency access.
func (s *Service) GenerateRecoveryCodes(ctx context.Context, count int) (codes []string, err error) {
	recovery := NewRecoveryCodeGenerator()
	return recovery.Generate(count)
}

// ValidateRecoveryCode checks if a recovery code is valid (simple format check).
func (s *Service) ValidateRecoveryCode(code string) bool {
	recovery := NewRecoveryCodeGenerator()
	return recovery.IsValid(code)
}

// Constants for TOTP
const (
	TimeStepSeconds = 30 // RFC 6238 standard time step in seconds
	Digits          = 6  // Standard 6-digit codes
)
