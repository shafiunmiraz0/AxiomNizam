package handlers

import (
	"encoding/json"

	"github.com/google/uuid"
)

// EnrollRequest is the API request for starting factor enrollment.
type EnrollRequest struct {
	UserID     uuid.UUID `json:"user_id" binding:"required"`
	FactorType string    `json:"factor_type" binding:"required"`
	Label      string    `json:"label"`
	Email      string    `json:"email"`
	Phone      string    `json:"phone"`
	Issuer     string    `json:"issuer"`
}

// EnrollResponse is the API response for enrollment.
type EnrollResponse struct {
	FactorID  uuid.UUID `json:"factor_id"`
	Secret    string    `json:"secret,omitempty"`
	QRCodeURI string    `json:"qr_code_uri,omitempty"`
}

// ActivateRequest is the API request for completing factor activation.
type ActivateRequest struct {
	FactorID uuid.UUID `json:"factor_id" binding:"required"`
	Code     string    `json:"code" binding:"required"`
}

// ActivateResponse is the API response for activation.
type ActivateResponse struct {
	FactorID    uuid.UUID `json:"factor_id"`
	BackupCodes []string  `json:"backup_codes"`
}

// BeginChallengeRequest is the API request for starting an MFA challenge.
type BeginChallengeRequest struct {
	UserID   uuid.UUID `json:"user_id" binding:"required"`
	FactorID uuid.UUID `json:"factor_id" binding:"required"`
}

// BeginChallengeResponse is the API response for challenge creation.
type BeginChallengeResponse struct {
	ChallengeID uuid.UUID `json:"challenge_id"`
	ExpiresAt   string    `json:"expires_at"`
}

// VerifyChallengeRequest is the API request for verifying an MFA challenge.
type VerifyChallengeRequest struct {
	ChallengeID string `json:"challenge_id" binding:"required"`
	Code        string `json:"code" binding:"required"`
}

// VerifyChallengeResponse is the API response for challenge verification.
type VerifyChallengeResponse struct {
	Verified bool   `json:"verified"`
	Message  string `json:"message"`
}

// FactorResponse is the API response for a factor (without sensitive fields).
type FactorResponse struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	FactorType  string    `json:"factor_type"`
	Label       string    `json:"label"`
	Phase       string    `json:"phase"`
	Issuer      string    `json:"issuer"`
	ActivatedAt *string   `json:"activated_at,omitempty"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

// TrustDeviceRequest is the API request for trusting a device.
type TrustDeviceRequest struct {
	UserID      uuid.UUID `json:"user_id" binding:"required"`
	Fingerprint string    `json:"fingerprint" binding:"required"`
	UserAgent   string    `json:"user_agent"`
	IPAddress   string    `json:"ip_address"`
}

// TrustDeviceResponse is the API response for device trust.
type TrustDeviceResponse struct {
	DeviceID  uuid.UUID `json:"device_id"`
	Token     string    `json:"token"`
	ExpiresAt string    `json:"expires_at"`
}

// ScoreRiskRequest is the API request for risk scoring.
type ScoreRiskRequest struct {
	UserID    uuid.UUID `json:"user_id" binding:"required"`
	IPAddress string    `json:"ip_address" binding:"required"`
}

// ScoreRiskResponse is the API response for risk scoring.
type ScoreRiskResponse struct {
	Score     int    `json:"score"`
	Level     string `json:"level"`
	IsHigh    bool   `json:"is_high_risk"`
	IPAddress string `json:"ip_address"`
}

// WebAuthnBeginRegistrationRequest is the API request for starting WebAuthn registration.
type WebAuthnBeginRegistrationRequest struct {
	UserID      uuid.UUID `json:"user_id" binding:"required"`
	UserName    string    `json:"user_name" binding:"required"`
	DisplayName string    `json:"display_name"`
}

// WebAuthnFinishRegistrationRequest is the API request for completing WebAuthn registration.
type WebAuthnFinishRegistrationRequest struct {
	UserID    uuid.UUID       `json:"user_id" binding:"required"`
	SessionID string          `json:"session_id" binding:"required"`
	Response  json.RawMessage `json:"response" binding:"required"`
}

// WebAuthnBeginAuthenticationRequest is the API request for starting WebAuthn authentication.
type WebAuthnBeginAuthenticationRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
}

// WebAuthnFinishAuthenticationRequest is the API request for completing WebAuthn authentication.
type WebAuthnFinishAuthenticationRequest struct {
	UserID    uuid.UUID       `json:"user_id" binding:"required"`
	SessionID string          `json:"session_id" binding:"required"`
	Response  json.RawMessage `json:"response" binding:"required"`
}
