package trusteddevices

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/repositories"
)

// Service manages trusted device tokens for "remember this device" functionality.
type Service struct {
	deviceRepo repositories.TrustedDeviceRepository
	clock      Clock
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

// NewService creates a new trusted device service.
func NewService(dr repositories.TrustedDeviceRepository, c Clock) *Service {
	return &Service{
		deviceRepo: dr,
		clock:      c,
	}
}

// TrustDeviceRequest contains parameters for trusting a device.
type TrustDeviceRequest struct {
	UserID      models.UserID
	Fingerprint string
	UserAgent   string
	IPAddress   string
	TTLDays     int // Time-to-live in days (configurable per policy)
}

// TrustDeviceResponse contains the device token to store in a persistent cookie.
type TrustDeviceResponse struct {
	DeviceID    uuid.UUID
	Token       string // Random token (store in browser localStorage or cookie)
	ExpiresAt   time.Time
	CookieName  string
	CookieValue string
}

// TrustDevice registers a new trusted device after successful MFA verification.
func (s *Service) TrustDevice(ctx context.Context, req *TrustDeviceRequest) (*TrustDeviceResponse, error) {
	if req.UserID == uuid.Nil {
		return nil, errors.New("user_id is required")
	}

	if req.Fingerprint == "" {
		return nil, errors.New("fingerprint is required")
	}

	// Generate random device token
	token, err := generateDeviceToken()
	if err != nil {
		return nil, err
	}

	// Hash the token for storage
	tokenHash := HashDeviceToken(token)

	// Calculate expiration
	now := s.clock.Now()
	if req.TTLDays == 0 {
		req.TTLDays = 90 // Default 90 days
	}
	expiresAt := now.AddDate(0, 0, req.TTLDays)

	// Create trusted device record
	device := &models.TrustedDevice{
		ID:          uuid.New(),
		UserID:      req.UserID,
		TokenHash:   tokenHash,
		Fingerprint: req.Fingerprint,
		UserAgent:   req.UserAgent,
		IPAddress:   req.IPAddress,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}

	device, err = s.deviceRepo.Create(ctx, device)
	if err != nil {
		return nil, err
	}

	// Return token to client (only shown once)
	return &TrustDeviceResponse{
		DeviceID:    device.ID,
		Token:       token,
		ExpiresAt:   expiresAt,
		CookieName:  "x-trusted-device",
		CookieValue: token,
	}, nil
}

// VerifyDeviceToken checks if a device token is valid and active.
func (s *Service) VerifyDeviceToken(ctx context.Context, userID models.UserID, fingerprint string, token string) (bool, error) {
	// Look up device by fingerprint
	device, err := s.deviceRepo.GetByFingerprint(ctx, userID, fingerprint)
	if err != nil {
		return false, err
	}

	if device == nil {
		return false, nil
	}

	// Check if device is still active
	if device.IsExpired(s.clock.Now()) || device.RevokedAt != nil {
		return false, nil
	}

	// Verify token hash matches
	expectedHash := HashDeviceToken(token)
	if !BytesEqual(device.TokenHash, expectedHash) {
		return false, nil
	}

	return true, nil
}

// ListTrustedDevices returns all active trusted devices for a user.
func (s *Service) ListTrustedDevices(ctx context.Context, userID models.UserID) ([]*models.TrustedDevice, error) {
	return s.deviceRepo.GetByUserID(ctx, userID)
}

// RevokeTrustedDevice marks a specific device as revoked.
func (s *Service) RevokeTrustedDevice(ctx context.Context, deviceID uuid.UUID) error {
	return s.deviceRepo.Revoke(ctx, deviceID)
}

// RevokeAllDevices revokes all trusted devices for a user (e.g., on password change).
func (s *Service) RevokeAllDevices(ctx context.Context, userID models.UserID) error {
	return s.deviceRepo.RevokeByUserID(ctx, userID)
}

// CleanupExpired removes expired device records (run periodically).
func (s *Service) CleanupExpired(ctx context.Context) error {
	return s.deviceRepo.DeleteExpired(ctx)
}

// DeviceCount returns the number of active trusted devices for a user.
func (s *Service) DeviceCount(ctx context.Context, userID models.UserID) (int, error) {
	return s.deviceRepo.Count(ctx, userID)
}

// generateDeviceToken generates a random 32-byte base64-encoded device token.
func generateDeviceToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// HashDeviceToken creates a SHA-256 hash of the device token for secure storage.
func HashDeviceToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

// BytesEqual compares two byte slices in constant time.
func BytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	result := 0
	for i := range a {
		result |= int(a[i]) ^ int(b[i])
	}
	return result == 0
}
