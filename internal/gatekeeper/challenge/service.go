package challenge

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/repositories"
)

// Service manages the lifecycle of MFA challenges (authentication events).
type Service struct {
	challengeRepo repositories.ChallengeRepository
	factorRepo    repositories.FactorRepository
	totpSvc       TOTPValidator
	clock         Clock
	encryptionKey []byte
}

// TOTPValidator validates TOTP codes against a secret.
type TOTPValidator interface {
	ValidateCode(ctx context.Context, secret, code string) (bool, error)
}

// Clock abstracts time for testability.
type Clock interface {
	Now() time.Time
}

// realClock implements Clock using system time.
type realClock struct{}

func (c *realClock) Now() time.Time {
	return time.Now().UTC()
}

// NewRealClock creates a clock that returns the actual time.
func NewRealClock() Clock {
	return &realClock{}
}

// NewService creates a new challenge service.
func NewService(cr repositories.ChallengeRepository, fr repositories.FactorRepository, tv TOTPValidator, c Clock, encKey []byte) *Service {
	return &Service{
		challengeRepo: cr,
		factorRepo:    fr,
		totpSvc:       tv,
		clock:         c,
		encryptionKey: encKey,
	}
}

// BeginRequest contains parameters for starting a challenge.
type BeginRequest struct {
	UserID    models.UserID
	FactorID  models.FactorID
	IPAddress string
	UserAgent string
}

// BeginChallenge creates a new authentication challenge.
// Returns a challenge that the user must respond to within the TTL.
func (s *Service) BeginChallenge(ctx context.Context, req *BeginRequest) (*models.Challenge, error) {
	// Verify the factor exists and is active
	factor, err := s.factorRepo.Get(ctx, req.FactorID)
	if err != nil {
		return nil, err
	}
	if factor == nil || !factor.IsActive() {
		return nil, errors.New("factor not found or not active")
	}

	// Generate OTP nonce for TOTP/SMS/Email
	nonce, err := generateNonce()
	if err != nil {
		return nil, err
	}

	// Create challenge with default 5-minute TTL
	now := s.clock.Now()
	challenge := &models.Challenge{
		ID:        uuid.New(),
		UserID:    req.UserID,
		FactorID:  req.FactorID,
		Phase:     models.ChallengePhaseWaiting,
		Nonce:     nonce,
		Attempts:  0,
		ExpiresAt: now.Add(5 * time.Minute),
		IPAddress: req.IPAddress,
		UserAgent: req.UserAgent,
		CreatedAt: now,
	}

	// Persist the challenge
	challenge, err = s.challengeRepo.Create(ctx, challenge)
	if err != nil {
		return nil, err
	}

	return challenge, nil
}

// VerifyRequest contains parameters for verifying a challenge.
type VerifyRequest struct {
	ChallengeID models.ChallengeID
	Code        string // OTP code (6 digits for TOTP)
}

// VerifyChallenge verifies a user's response to a challenge.
func (s *Service) VerifyChallenge(ctx context.Context, req *VerifyRequest) (*models.Challenge, error) {
	// Retrieve the challenge
	ch, err := s.challengeRepo.Get(ctx, req.ChallengeID)
	if err != nil {
		return nil, err
	}
	if ch == nil {
		return nil, errors.New("challenge not found")
	}

	// Check if challenge is in terminal state
	if ch.IsTerminal() {
		return nil, errors.New("challenge is in terminal state")
	}

	// Check if challenge has expired
	if ch.IsExpired(s.clock.Now()) {
		ch.Phase = models.ChallengePhaseExpired
		ch.ResolvedAt = ptrTime(s.clock.Now())
		_, _ = s.challengeRepo.Update(ctx, ch)
		return nil, errors.New("challenge expired")
	}

	// Increment attempt counter
	ch.Attempts++

	// Retrieve the factor to get its secret for TOTP validation
	factor, err := s.factorRepo.Get(ctx, ch.FactorID)
	if err != nil || factor == nil {
		ch.Phase = models.ChallengePhaseFailed
		ch.ResolvedAt = ptrTime(s.clock.Now())
		_, _ = s.challengeRepo.Update(ctx, ch)
		return nil, errors.New("factor not found")
	}

	// Decrypt the stored TOTP secret
	secret, decErr := s.decryptSecret(factor.Spec.EncryptedSecret)
	if decErr != nil {
		ch.Phase = models.ChallengePhaseFailed
		ch.ResolvedAt = ptrTime(s.clock.Now())
		_, _ = s.challengeRepo.Update(ctx, ch)
		return nil, errors.New("failed to decrypt factor secret")
	}

	// Validate OTP code against the factor's TOTP secret
	valid, err := s.totpSvc.ValidateCode(ctx, secret, req.Code)
	if err != nil || !valid {
		// Max 3 attempts before lockout
		if ch.Attempts >= 3 {
			ch.Phase = models.ChallengePhaseFailed
			ch.ResolvedAt = ptrTime(s.clock.Now())
		}
		_, _ = s.challengeRepo.Update(ctx, ch)
		return nil, errors.New("invalid code")
	}

	// Code verified successfully
	ch.Phase = models.ChallengePhaseVerified
	ch.ResolvedAt = ptrTime(s.clock.Now())

	ch, err = s.challengeRepo.Update(ctx, ch)
	if err != nil {
		return nil, err
	}

	return ch, nil
}

// ExpireChallenge marks a challenge as expired (for garbage collection).
func (s *Service) ExpireChallenge(ctx context.Context, challengeID models.ChallengeID) error {
	challenge, err := s.challengeRepo.Get(ctx, challengeID)
	if err != nil {
		return err
	}
	if challenge == nil {
		return errors.New("challenge not found")
	}

	if !challenge.IsTerminal() {
		challenge.Phase = models.ChallengePhaseExpired
		challenge.ResolvedAt = ptrTime(s.clock.Now())
		_, err = s.challengeRepo.Update(ctx, challenge)
	}

	return err
}

// generateNonce generates a random 6-digit code.
func generateNonce() (string, error) {
	bytes := make([]byte, 3)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	// Convert 3 random bytes to a number between 0-999999
	// Take modulo 1000000 to stay within 6 digits
	num := uint32(bytes[0])<<16 | uint32(bytes[1])<<8 | uint32(bytes[2])
	num = num % 1000000
	return hex.EncodeToString(bytes), nil
}

// decryptSecret decrypts an AES-GCM encrypted TOTP secret.
func (s *Service) decryptSecret(ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// ptrTime returns a pointer to time.Time.
func ptrTime(t time.Time) *time.Time {
	return &t
}
