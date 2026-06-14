package policy

import (
	"context"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// IPRestrictionRule requires MFA for requests from specific IP ranges or patterns.
type IPRestrictionRule struct {
	BlockedPrefixes []string // IP prefixes that require MFA
}

// Match returns true if the request IP matches a blocked prefix.
func (r *IPRestrictionRule) Match(ctx context.Context, req *EvaluationRequest) bool {
	for _, prefix := range r.BlockedPrefixes {
		if strings.HasPrefix(req.IPAddress, prefix) {
			return true
		}
	}
	return false
}

// Action requires MFA for the matched IP.
func (r *IPRestrictionRule) Action(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error) {
	return &EvaluationResult{
		RequiresMFA:    true,
		AllowedFactors: []models.FactorType{models.FactorTypeTOTP},
		ChallengeTTL:   300,
		MaxAttempts:    3,
		RiskAction:     "challenge",
		Reason:         "IP requires MFA verification",
	}, nil
}

// SensitiveResourceRule requires MFA for sensitive resource types.
type SensitiveResourceRule struct {
	ResourceTypes []string
}

// Match returns true if the resource type is in the sensitive list.
func (r *SensitiveResourceRule) Match(ctx context.Context, req *EvaluationRequest) bool {
	for _, rt := range r.ResourceTypes {
		if req.ResourceType == rt || req.ResourcePath == rt {
			return true
		}
	}
	return false
}

// Action requires MFA for the sensitive resource.
func (r *SensitiveResourceRule) Action(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error) {
	return &EvaluationResult{
		RequiresMFA:        true,
		AllowedFactors:     []models.FactorType{models.FactorTypeTOTP, models.FactorTypeEmail},
		ChallengeTTL:       300,
		MaxAttempts:        3,
		TrustDeviceAllowed: false,
		RiskAction:         "require",
		Reason:             "Resource requires MFA",
	}, nil
}

// HighRiskBlockRule blocks requests with extremely high risk scores.
type HighRiskBlockRule struct {
	Threshold int
}

// Match returns true if risk score exceeds the threshold.
func (r *HighRiskBlockRule) Match(ctx context.Context, req *EvaluationRequest) bool {
	return req.RiskScore >= r.Threshold
}

// Action blocks the request entirely.
func (r *HighRiskBlockRule) Action(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error) {
	return &EvaluationResult{
		RequiresMFA:        true,
		AllowedFactors:     []models.FactorType{},
		ChallengeTTL:       0,
		MaxAttempts:        0,
		TrustDeviceAllowed: false,
		RiskAction:         "block",
		Reason:             "Risk score too high, access blocked",
	}, nil
}

// LabelBasedRule applies MFA based on user labels (K8s-style targeting).
type LabelBasedRule struct {
	RequiredLabels map[string]string
}

// Match returns true if the user has all required labels.
func (r *LabelBasedRule) Match(ctx context.Context, req *EvaluationRequest) bool {
	for k, v := range r.RequiredLabels {
		if req.UserLabels[k] != v {
			return false
		}
	}
	return len(r.RequiredLabels) > 0
}

// Action requires MFA for users matching the label criteria.
func (r *LabelBasedRule) Action(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error) {
	return &EvaluationResult{
		RequiresMFA:        true,
		AllowedFactors:     []models.FactorType{models.FactorTypeTOTP},
		ChallengeTTL:       300,
		MaxAttempts:        3,
		TrustDeviceAllowed: true,
		RiskAction:         "challenge",
		Reason:             "User label policy requires MFA",
	}, nil
}
