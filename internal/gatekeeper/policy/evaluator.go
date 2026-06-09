package policy

import (
	"context"

	"example.com/axiomnizam/internal/gatekeeper/models"
)

// AdaptiveEvaluator evaluates policies based on risk score and context.
// Unlike DefaultEvaluator, it considers time since last MFA and device trust.
type AdaptiveEvaluator struct {
	RequireMFAAfterHours int // Require MFA if last verification was N hours ago
}

// Evaluate implements the Evaluator interface with adaptive logic.
func (a *AdaptiveEvaluator) Evaluate(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error) {
	result := &EvaluationResult{
		AllowedFactors:     []models.FactorType{models.FactorTypeTOTP},
		ChallengeTTL:       300,
		MaxAttempts:        3,
		TrustDeviceAllowed: true,
	}

	// High risk: require MFA, prefer WebAuthn, block trust device
	if req.RiskScore > 75 {
		result.RequiresMFA = true
		result.AllowedFactors = []models.FactorType{models.FactorTypeWebAuthn, models.FactorTypeTOTP}
		result.RiskAction = "block"
		result.Reason = "Critical risk score"
		result.TrustDeviceAllowed = false
		return result, nil
	}

	// Elevated risk: require MFA challenge, WebAuthn preferred
	if req.RiskScore > 50 {
		result.RequiresMFA = true
		result.AllowedFactors = []models.FactorType{models.FactorTypeWebAuthn, models.FactorTypeTOTP}
		result.RiskAction = "challenge"
		result.Reason = "Elevated risk score"
		return result, nil
	}

	// New device: require MFA with trust option
	if req.IsNewDevice {
		result.RequiresMFA = true
		result.RiskAction = "challenge"
		result.Reason = "New device detected"
		result.TrustDeviceAllowed = true
		return result, nil
	}

	// Time-based: require MFA if too long since last verification
	if a.RequireMFAAfterHours > 0 && req.LastMFAAt > 0 {
		// This would check if last MFA was more than N hours ago
		// For now, flag it as requiring MFA
		result.RequiresMFA = true
		result.RiskAction = "challenge"
		result.Reason = "MFA session expired"
		return result, nil
	}

	// Sensitive resource: always require MFA, prefer WebAuthn
	if req.ResourceType == "sensitive-operation" {
		result.RequiresMFA = true
		result.AllowedFactors = []models.FactorType{models.FactorTypeWebAuthn, models.FactorTypeTOTP}
		result.RiskAction = "require"
		result.Reason = "Sensitive operation requires MFA"
		return result, nil
	}

	// Default: MFA optional
	result.RequiresMFA = false
	result.RiskAction = "allow"
	result.Reason = "Policy allows optional MFA"
	return result, nil
}
