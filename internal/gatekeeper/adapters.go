package gatekeeper

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"example.com/axiomnizam/internal/gatekeeper/challenge"
	"example.com/axiomnizam/internal/gatekeeper/contracts"
	"example.com/axiomnizam/internal/gatekeeper/enrollment"
	"example.com/axiomnizam/internal/gatekeeper/models"
	"example.com/axiomnizam/internal/gatekeeper/repositories"
	"example.com/axiomnizam/internal/gatekeeper/trusteddevices"
	gkwebauthn "example.com/axiomnizam/internal/gatekeeper/webauthn"
)

// enrollmentServiceWrapper wraps enrollment.Service to match contracts.EnrollmentService.
type enrollmentServiceWrapper struct {
	svc *enrollment.Service
}

func wrapEnrollmentService(svc *enrollment.Service) contracts.EnrollmentService {
	return &enrollmentServiceWrapper{svc: svc}
}

func (w *enrollmentServiceWrapper) SetupFactor(ctx context.Context, userID models.UserID, factorType models.FactorType, label string) (*contracts.SetupResult, error) {
	resp, err := w.svc.SetupFactor(ctx, &enrollment.SetupRequest{
		UserID:     userID,
		FactorType: factorType,
		Issuer:     "AxiomNizam",
		Label:      label,
	})
	if err != nil {
		return nil, err
	}
	return &contracts.SetupResult{
		FactorID: resp.FactorID,
		Secret:   resp.Secret,
	}, nil
}

func (w *enrollmentServiceWrapper) ActivateFactor(ctx context.Context, factorID models.FactorID, code string) ([]string, error) {
	resp, err := w.svc.ActivateFactor(ctx, &enrollment.ActivateRequest{
		FactorID: factorID,
		Code:     code,
	})
	if err != nil {
		return nil, err
	}
	return resp.BackupCodes, nil
}

func (w *enrollmentServiceWrapper) DisableFactor(ctx context.Context, factorID models.FactorID) error {
	return w.svc.DisableFactor(ctx, factorID)
}

// challengeServiceWrapper wraps challenge.Service to match contracts.ChallengeService.
type challengeServiceWrapper struct {
	svc *challenge.Service
}

func wrapChallengeService(svc *challenge.Service) contracts.ChallengeService {
	return &challengeServiceWrapper{svc: svc}
}

func (w *challengeServiceWrapper) BeginChallenge(ctx context.Context, userID models.UserID, factorID models.FactorID) (string, error) {
	challenge, err := w.svc.BeginChallenge(ctx, &challenge.BeginRequest{
		UserID:   userID,
		FactorID: factorID,
	})
	if err != nil {
		return "", err
	}
	return challenge.ID.String(), nil
}

func (w *challengeServiceWrapper) VerifyChallenge(ctx context.Context, challengeID string, code string) (bool, error) {
	id, err := uuid.Parse(challengeID)
	if err != nil {
		return false, err
	}
	challenge, err := w.svc.VerifyChallenge(ctx, &challenge.VerifyRequest{
		ChallengeID: id,
		Code:        code,
	})
	if err != nil {
		return false, err
	}
	return challenge.Phase == models.ChallengePhaseVerified, nil
}

func (w *challengeServiceWrapper) ExpireChallenge(ctx context.Context, challengeID string) error {
	id, err := uuid.Parse(challengeID)
	if err != nil {
		return err
	}
	return w.svc.ExpireChallenge(ctx, id)
}

// factorServiceWrapper wraps FactorRepository to match contracts.FactorService.
type factorServiceWrapper struct {
	repo repositories.FactorRepository
}

func wrapFactorService(repo repositories.FactorRepository) contracts.FactorService {
	return &factorServiceWrapper{repo: repo}
}

func (w *factorServiceWrapper) GetFactor(ctx context.Context, factorID models.FactorID) (*models.Factor, error) {
	return w.repo.Get(ctx, factorID)
}

func (w *factorServiceWrapper) ListFactors(ctx context.Context, userID models.UserID) ([]*models.Factor, error) {
	return w.repo.GetByUserID(ctx, userID)
}

func (w *factorServiceWrapper) DeleteFactor(ctx context.Context, factorID models.FactorID) error {
	return w.repo.Delete(ctx, factorID)
}

func (w *factorServiceWrapper) GetActiveFactorCount(ctx context.Context, userID models.UserID) (int, error) {
	factors, err := w.repo.GetByUserID(ctx, userID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, f := range factors {
		if f.IsActive() {
			count++
		}
	}
	return count, nil
}

// policyServiceWrapper wraps policy.Engine to match contracts.PolicyService.
type policyServiceWrapper struct {
	svc interface {
		EvaluatePolicy(ctx context.Context, userID models.UserID) (bool, []models.FactorType, error)
		GetPolicy(ctx context.Context, policyID string) (*models.MFAPolicy, error)
	}
}

func wrapPolicyService(svc interface {
	EvaluatePolicy(ctx context.Context, userID models.UserID) (bool, []models.FactorType, error)
	GetPolicy(ctx context.Context, policyID string) (*models.MFAPolicy, error)
}) contracts.PolicyService {
	return &policyServiceWrapper{svc: svc}
}

func (w *policyServiceWrapper) EvaluatePolicy(ctx context.Context, userID models.UserID) (bool, []models.FactorType, error) {
	return w.svc.EvaluatePolicy(ctx, userID)
}

func (w *policyServiceWrapper) GetPolicy(ctx context.Context, policyID uuid.UUID) (*models.MFAPolicy, error) {
	return w.svc.GetPolicy(ctx, policyID.String())
}

// riskServiceWrapper wraps risk.Engine to match contracts.RiskService.
type riskServiceWrapper struct {
	svc interface {
		ScoreAuthentication(ctx context.Context, userID models.UserID, ipAddress string) (int, error)
		IsHighRisk(ctx context.Context, score int) bool
	}
}

func wrapRiskService(svc interface {
	ScoreAuthentication(ctx context.Context, userID models.UserID, ipAddress string) (int, error)
	IsHighRisk(ctx context.Context, score int) bool
}) contracts.RiskService {
	return &riskServiceWrapper{svc: svc}
}

func (w *riskServiceWrapper) ScoreAuthentication(ctx context.Context, userID models.UserID, ipAddress string) (int, error) {
	return w.svc.ScoreAuthentication(ctx, userID, ipAddress)
}

func (w *riskServiceWrapper) IsHighRisk(ctx context.Context, score int) bool {
	return w.svc.IsHighRisk(ctx, score)
}

// trustedDeviceServiceWrapper wraps trusteddevices.Service to match contracts.TrustedDeviceService.
type trustedDeviceServiceWrapper struct {
	svc interface {
		TrustDevice(ctx context.Context, req *trusteddevices.TrustDeviceRequest) (*trusteddevices.TrustDeviceResponse, error)
		VerifyDeviceToken(ctx context.Context, userID models.UserID, fingerprint string, token string) (bool, error)
		ListTrustedDevices(ctx context.Context, userID models.UserID) ([]*models.TrustedDevice, error)
		RevokeTrustedDevice(ctx context.Context, deviceID uuid.UUID) error
		RevokeAllDevices(ctx context.Context, userID models.UserID) error
		DeviceCount(ctx context.Context, userID models.UserID) (int, error)
	}
}

func wrapTrustedDeviceService(svc interface {
	TrustDevice(ctx context.Context, req *trusteddevices.TrustDeviceRequest) (*trusteddevices.TrustDeviceResponse, error)
	VerifyDeviceToken(ctx context.Context, userID models.UserID, fingerprint string, token string) (bool, error)
	ListTrustedDevices(ctx context.Context, userID models.UserID) ([]*models.TrustedDevice, error)
	RevokeTrustedDevice(ctx context.Context, deviceID uuid.UUID) error
	RevokeAllDevices(ctx context.Context, userID models.UserID) error
	DeviceCount(ctx context.Context, userID models.UserID) (int, error)
}) contracts.TrustedDeviceService {
	return &trustedDeviceServiceWrapper{svc: svc}
}

func (w *trustedDeviceServiceWrapper) TrustDevice(ctx context.Context, userID models.UserID, fingerprint, userAgent, ipAddress string) (string, error) {
	resp, err := w.svc.TrustDevice(ctx, &trusteddevices.TrustDeviceRequest{
		UserID:      userID,
		Fingerprint: fingerprint,
		UserAgent:   userAgent,
		IPAddress:   ipAddress,
		TTLDays:     30,
	})
	if err != nil {
		return "", err
	}
	return resp.Token, nil
}

func (w *trustedDeviceServiceWrapper) VerifyDeviceToken(ctx context.Context, userID models.UserID, token string) (bool, error) {
	// Look up all devices for the user and check token hash
	devices, err := w.svc.ListTrustedDevices(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, d := range devices {
		if d.IsExpired(time.Now().UTC()) || d.RevokedAt != nil {
			continue
		}
		// Verify token hash matches
		expectedHash := trusteddevices.HashDeviceToken(token)
		if trusteddevices.BytesEqual(d.TokenHash, expectedHash) {
			return true, nil
		}
	}
	return false, nil
}

func (w *trustedDeviceServiceWrapper) ListTrustedDevices(ctx context.Context, userID models.UserID) ([]*models.TrustedDevice, error) {
	return w.svc.ListTrustedDevices(ctx, userID)
}

func (w *trustedDeviceServiceWrapper) RevokeTrustedDevice(ctx context.Context, deviceID uuid.UUID) error {
	return w.svc.RevokeTrustedDevice(ctx, deviceID)
}

func (w *trustedDeviceServiceWrapper) RevokeAllDevices(ctx context.Context, userID models.UserID) error {
	return w.svc.RevokeAllDevices(ctx, userID)
}

// backupCodeServiceWrapper wraps backupcodes.Service to match contracts.BackupCodeService.
type backupCodeServiceWrapper struct {
	svc interface {
		ConsumeBackupCode(ctx context.Context, code string) (*models.BackupCode, error)
		RemainingCodes(ctx context.Context, userID models.UserID) (int, error)
		RegenerateCodes(ctx context.Context, factorID models.FactorID, count int) ([]string, error)
	}
}

func wrapBackupCodeService(svc interface {
	ConsumeBackupCode(ctx context.Context, code string) (*models.BackupCode, error)
	RemainingCodes(ctx context.Context, userID models.UserID) (int, error)
	RegenerateCodes(ctx context.Context, factorID models.FactorID, count int) ([]string, error)
}) contracts.BackupCodeService {
	return &backupCodeServiceWrapper{svc: svc}
}

func (w *backupCodeServiceWrapper) ConsumeBackupCode(ctx context.Context, userID models.UserID, code string) (bool, error) {
	_, err := w.svc.ConsumeBackupCode(ctx, code)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (w *backupCodeServiceWrapper) GetRemainingBackupCodes(ctx context.Context, userID models.UserID) (int, error) {
	return w.svc.RemainingCodes(ctx, userID)
}

func (w *backupCodeServiceWrapper) RegenerateBackupCodes(ctx context.Context, factorID models.FactorID) ([]string, error) {
	return w.svc.RegenerateCodes(ctx, factorID, 10)
}

// webauthnServiceWrapper wraps gkwebauthn.Service to match contracts.WebAuthnService.
type webauthnServiceWrapper struct {
	svc *gkwebauthn.Service
}

func wrapWebAuthnService(svc *gkwebauthn.Service) contracts.WebAuthnService {
	return &webauthnServiceWrapper{svc: svc}
}

func (w *webauthnServiceWrapper) BeginRegistration(ctx context.Context, userID uuid.UUID) (string, map[string]interface{}, error) {
	sessionID, options, err := w.svc.BeginRegistration(ctx, userID.String(), userID.String(), userID.String())
	if err != nil {
		return "", nil, err
	}
	optMap := map[string]interface{}{
		"challenge":        options.Challenge,
		"rp":               options.RP,
		"user":             options.User,
		"pubKeyCredParams": options.PubKeyCredParams,
		"timeout":          options.Timeout,
		"attestation":      options.Attestation,
	}
	if len(options.ExcludeCredentials) > 0 {
		optMap["excludeCredentials"] = options.ExcludeCredentials
	}
	if options.AuthenticatorSelection.UserVerification != "" {
		optMap["authenticatorSelection"] = options.AuthenticatorSelection
	}
	return sessionID, optMap, nil
}

func (w *webauthnServiceWrapper) FinishRegistration(ctx context.Context, userID uuid.UUID, sessionID string, response []byte) error {
	var resp gkwebauthn.AttestationResponse
	if err := json.Unmarshal(response, &resp); err != nil {
		return fmt.Errorf("parse attestation response: %w", err)
	}
	_, err := w.svc.FinishRegistration(ctx, sessionID, &resp)
	return err
}

func (w *webauthnServiceWrapper) BeginAuthentication(ctx context.Context, userID uuid.UUID) (string, map[string]interface{}, error) {
	sessionID, options, err := w.svc.BeginAuthentication(ctx, userID.String())
	if err != nil {
		return "", nil, err
	}
	optMap := map[string]interface{}{
		"challenge":        options.Challenge,
		"rpId":             options.RPID,
		"timeout":          options.Timeout,
		"userVerification": options.UserVerification,
	}
	if len(options.AllowCredentials) > 0 {
		optMap["allowCredentials"] = options.AllowCredentials
	}
	return sessionID, optMap, nil
}

func (w *webauthnServiceWrapper) FinishAuthentication(ctx context.Context, userID uuid.UUID, sessionID string, response []byte) (bool, error) {
	var resp gkwebauthn.AssertionResponse
	if err := json.Unmarshal(response, &resp); err != nil {
		return false, fmt.Errorf("parse assertion response: %w", err)
	}
	return w.svc.FinishAuthentication(ctx, sessionID, &resp)
}

func (w *webauthnServiceWrapper) ListCredentials(ctx context.Context, userID uuid.UUID) ([]map[string]interface{}, error) {
	creds, err := w.svc.ListCredentials(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	var result []map[string]interface{}
	for _, c := range creds {
		result = append(result, map[string]interface{}{
			"id":               c.ID,
			"user_id":          c.UserID,
			"attestation_type": c.AttestationType,
			"sign_count":       c.SignCount,
			"clone_warning":    c.CloneWarning,
			"created_at":       c.CreatedAt,
		})
	}
	return result, nil
}

func (w *webauthnServiceWrapper) DeleteCredential(ctx context.Context, userID uuid.UUID, credentialID []byte) error {
	return w.svc.DeleteCredential(ctx, credentialID)
}