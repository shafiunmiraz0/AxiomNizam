package bootstrap

import (
	"context"
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/backupcodes"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/challenge"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/contracts"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/enrollment"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/policy"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/repositories"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/risk"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/trusteddevices"
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
	ch, err := w.svc.BeginChallenge(ctx, &challenge.BeginRequest{
		UserID:   userID,
		FactorID: factorID,
	})
	if err != nil {
		return "", err
	}
	return ch.ID.String(), nil
}

func (w *challengeServiceWrapper) VerifyChallenge(ctx context.Context, challengeID string, code string) (bool, error) {
	id, err := uuid.Parse(challengeID)
	if err != nil {
		return false, err
	}
	ch, err := w.svc.VerifyChallenge(ctx, &challenge.VerifyRequest{
		ChallengeID: id,
		Code:        code,
	})
	if err != nil {
		return false, err
	}
	return ch.Phase == models.ChallengePhaseVerified, nil
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
	svc *policy.Engine
}

func wrapPolicyService(svc *policy.Engine) contracts.PolicyService {
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
	svc *risk.Engine
}

func wrapRiskService(svc *risk.Engine) contracts.RiskService {
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
	svc *trusteddevices.Service
}

func wrapTrustedDeviceService(svc *trusteddevices.Service) contracts.TrustedDeviceService {
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
	svc *backupcodes.Service
}

func wrapBackupCodeService(svc *backupcodes.Service) contracts.BackupCodeService {
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
