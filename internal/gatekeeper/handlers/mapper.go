package handlers

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// FactorToResponse converts a Factor model to an API response DTO.
func FactorToResponse(f *models.Factor) *FactorResponse {
	resp := &FactorResponse{
		ID:         f.ID,
		UserID:     f.UserID,
		FactorType: string(f.Spec.Type),
		Label:      f.Spec.Label,
		Phase:      string(f.Status.Phase),
		Issuer:     f.Spec.Issuer,
		CreatedAt:  f.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  f.UpdatedAt.Format(time.RFC3339),
	}
	if f.Status.ActivatedAt != nil {
		t := f.Status.ActivatedAt.Format(time.RFC3339)
		resp.ActivatedAt = &t
	}
	return resp
}

// FactorsToResponse converts a slice of Factor models to API response DTOs.
func FactorsToResponse(factors []*models.Factor) []*FactorResponse {
	result := make([]*FactorResponse, len(factors))
	for i, f := range factors {
		result[i] = FactorToResponse(f)
	}
	return result
}

// ChallengeToBeginResponse converts a Challenge model to a BeginChallengeResponse.
func ChallengeToBeginResponse(ch *models.Challenge) *BeginChallengeResponse {
	return &BeginChallengeResponse{
		ChallengeID: ch.ID,
		ExpiresAt:   ch.ExpiresAt.Format(time.RFC3339),
	}
}

// DeviceToResponse converts a TrustedDevice model to a TrustDeviceResponse.
func DeviceToResponse(d *models.TrustedDevice) *TrustDeviceResponse {
	return &TrustDeviceResponse{
		DeviceID:  d.ID,
		Token:     "", // Token only returned on creation
		ExpiresAt: d.ExpiresAt.Format(time.RFC3339),
	}
}

// RiskLevelForScore converts a numeric score to a risk level string.
func RiskLevelForScore(score int) string {
	switch {
	case score >= 81:
		return "critical"
	case score >= 61:
		return "high"
	case score >= 31:
		return "medium"
	default:
		return "low"
	}
}
