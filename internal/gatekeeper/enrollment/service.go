package enrollment

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/repositories"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/totp"
)

// Service manages the 2FA enrollment workflow.
type Service struct {
	factorRepo     repositories.FactorRepository
	backupCodeRepo repositories.BackupCodeRepository
	totpSvc        *totp.Service
	encryptionKey  []byte
}

// NewService creates a new enrollment service.
func NewService(
	fr repositories.FactorRepository,
	bcr repositories.BackupCodeRepository,
	ts *totp.Service,
	encKey []byte,
) *Service {
	return &Service{
		factorRepo:     fr,
		backupCodeRepo: bcr,
		totpSvc:        ts,
		encryptionKey:  encKey,
	}
}

// SetupRequest contains parameters for starting factor setup.
type SetupRequest struct {
	UserID     models.UserID
	FactorType models.FactorType
	Label      string // User-friendly name for the factor
	Email      string // For email OTP
	Phone      string // For SMS OTP
	Issuer     string
}

// SetupResponse contains setup instructions for the user.
type SetupResponse struct {
	FactorID  models.FactorID
	Secret    string // Base32-encoded TOTP secret (for TOTP only)
	QRCodeURI string // otpauth:// URI for QR code generation
}

// SetupFactor initiates factor enrollment by creating a Pending factor and generating a secret.
// If the user already has an active or pending factor of the same type, it is disabled first.
func (s *Service) SetupFactor(ctx context.Context, req *SetupRequest) (*SetupResponse, error) {
	if req.UserID == uuid.Nil {
		return nil, errors.New("user_id is required")
	}

	if req.FactorType == "" {
		return nil, errors.New("factor_type is required")
	}

	// Deactivate any existing active/pending factor of the same type for this user
	existing, err := s.factorRepo.GetByUserID(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing factors: %w", err)
	}
	now := time.Now().UTC()
	for _, f := range existing {
		if f.Spec.Type == req.FactorType && (f.Status.Phase == models.FactorPhaseActive || f.Status.Phase == models.FactorPhasePending) {
			f.Status.Phase = models.FactorPhaseDisabled
			f.Status.DisabledAt = &now
			if _, err := s.factorRepo.Update(ctx, f); err != nil {
				return nil, fmt.Errorf("failed to disable existing factor %s: %w", f.ID, err)
			}
			// Clean up backup codes for the old factor
			_ = s.backupCodeRepo.DeleteByFactorID(ctx, f.ID)
		}
	}

	// Generate secret for TOTP
	var secret, qrCodeURI string

	if req.FactorType == models.FactorTypeTOTP {
		secret, qrCodeURI, err = s.totpSvc.GenerateSecret(ctx, req.UserID, req.UserID.String(), req.Issuer)
		if err != nil {
			return nil, err
		}
	}

	// Create factor in Pending phase
	factor := &models.Factor{
		ID:     uuid.New(),
		UserID: req.UserID,
		Spec: models.FactorSpec{
			Type:            req.FactorType,
			Label:           req.Label,
			PhoneNumber:     req.Phone,
			Email:           req.Email,
			EncryptedSecret: mustEncryptSecret(s.encryptionKey, secret),
			Issuer:          req.Issuer,
		},
		Status: models.FactorStatus{
			Phase:      models.FactorPhasePending,
			Conditions: []models.Condition{},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	factor, err = s.factorRepo.Create(ctx, factor)
	if err != nil {
		return nil, err
	}

	return &SetupResponse{
		FactorID:  factor.ID,
		Secret:    secret,
		QRCodeURI: qrCodeURI,
	}, nil
}

// ActivateRequest contains parameters for completing factor activation.
type ActivateRequest struct {
	FactorID models.FactorID
	Code     string // OTP code to verify
}

// ActivateResponse contains confirmation and backup codes.
type ActivateResponse struct {
	FactorID    models.FactorID
	BackupCodes []string // Plaintext backup codes (shown once)
}

// ActivateFactor completes enrollment by verifying the OTP and transitioning to Active phase.
// Generates backup codes as fallback recovery mechanism.
func (s *Service) ActivateFactor(ctx context.Context, req *ActivateRequest) (*ActivateResponse, error) {
	// Retrieve the factor
	factor, err := s.factorRepo.Get(ctx, req.FactorID)
	if err != nil {
		return nil, err
	}
	if factor == nil {
		return nil, errors.New("factor not found")
	}

	if factor.Status.Phase != models.FactorPhasePending {
		return nil, errors.New("factor is not in Pending phase")
	}

	// Verify the OTP code
	var valid bool
	switch factor.Spec.Type {
	case models.FactorTypeTOTP:
		secret, decErr := decryptSecret(s.encryptionKey, factor.Spec.EncryptedSecret)
		if decErr != nil {
			return nil, fmt.Errorf("failed to decrypt secret: %w", decErr)
		}
		valid, err = s.totpSvc.ValidateCode(ctx, secret, req.Code)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported factor type for OTP validation")
	}

	if !valid {
		return nil, errors.New("invalid OTP code")
	}

	// Generate backup codes
	plainCodes, err := s.totpSvc.GenerateRecoveryCodes(ctx, 10)
	if err != nil {
		return nil, err
	}

	// Hash and persist backup codes
	now := time.Now().UTC()
	backupCodes := make([]*models.BackupCode, len(plainCodes))
	for i, code := range plainCodes {
		backupCodes[i] = &models.BackupCode{
			ID:        uuid.New(),
			UserID:    factor.UserID,
			FactorID:  req.FactorID,
			CodeHash:  hashCode(code), // TODO: Use bcrypt or argon2
			CreatedAt: now,
		}
	}

	err = s.backupCodeRepo.Create(ctx, backupCodes)
	if err != nil {
		return nil, err
	}

	// Transition factor to Active phase
	factor.Status.Phase = models.FactorPhaseActive
	factor.Status.ActivatedAt = &now
	factor.Status.LastVerifiedAt = &now

	_, err = s.factorRepo.Update(ctx, factor)
	if err != nil {
		return nil, err
	}

	return &ActivateResponse{
		FactorID:    req.FactorID,
		BackupCodes: plainCodes,
	}, nil
}

// DisableFactor removes an active factor (user-initiated).
func (s *Service) DisableFactor(ctx context.Context, factorID models.FactorID) error {
	factor, err := s.factorRepo.Get(ctx, factorID)
	if err != nil {
		return err
	}
	if factor == nil {
		return errors.New("factor not found")
	}

	now := time.Now().UTC()
	factor.Status.Phase = models.FactorPhaseDisabled
	factor.Status.DisabledAt = &now

	// Clean up backup codes
	_ = s.backupCodeRepo.DeleteByFactorID(ctx, factorID)

	_, err = s.factorRepo.Update(ctx, factor)
	return err
}

// hashCode creates a SHA-256 hash of a backup code.
func hashCode(code string) []byte {
	normalized := strings.ToLower(strings.ReplaceAll(code, "-", ""))
	hash := sha256.Sum256([]byte(normalized))
	return hash[:]
}

// encryptSecret encrypts a TOTP secret using AES-GCM.
func encryptSecret(key []byte, plaintext string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return ciphertext, nil
}

// decryptSecret decrypts a TOTP secret using AES-GCM.
func decryptSecret(key []byte, ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(key)
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

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// mustEncryptSecret encrypts a secret and panics on error (used during setup).
func mustEncryptSecret(key []byte, plaintext string) []byte {
	encrypted, err := encryptSecret(key, plaintext)
	if err != nil {
		panic(fmt.Sprintf("failed to encrypt secret: %v", err))
	}
	return encrypted
}
