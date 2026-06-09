package policy

import (
	"context"
	"errors"

	"example.com/axiomnizam/internal/gatekeeper/models"
)

// Engine evaluates MFA policies to determine enforcement requirements.
type Engine struct {
	evaluator Evaluator
	rules     []Rule
}

// Evaluator defines policy evaluation logic.
type Evaluator interface {
	Evaluate(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error)
}

// Rule represents a single policy enforcement rule.
type Rule interface {
	Match(ctx context.Context, req *EvaluationRequest) bool
	Action(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error)
}

// EvaluationRequest contains context for policy evaluation.
type EvaluationRequest struct {
	UserID               string            // User identifier
	UserLabels           map[string]string // K8s-style labels for targeting
	ResourceType         string            // e.g., "api", "database", "ui"
	ResourcePath         string            // e.g., "/api/v1/sensitive-data"
	AuthenticationMethod string            // Current authentication method
	IPAddress            string
	Geo                  string
	Browser              string
	IsNewDevice          bool
	LastMFAAt            int64 // Unix timestamp
	RiskScore            int   // 0-100
}

// EvaluationResult indicates what action to take.
type EvaluationResult struct {
	RequiresMFA        bool                // Whether MFA is needed
	AllowedFactors     []models.FactorType // Acceptable factor types
	ChallengeTTL       int                 // Seconds
	MaxAttempts        int
	TrustDeviceAllowed bool
	RiskAction         string // "allow", "challenge", "block"
	Reason             string
	Conditions         []string
}

// NewEngine creates a new policy evaluation engine.
func NewEngine(e Evaluator, rules []Rule) *Engine {
	return &Engine{
		evaluator: e,
		rules:     rules,
	}
}

// Evaluate determines if MFA is required and what factors are allowed.
func (e *Engine) Evaluate(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error) {
	if req == nil {
		return nil, errors.New("evaluation request is required")
	}

	// Start with default result: MFA optional
	result := &EvaluationResult{
		RequiresMFA:        false,
		AllowedFactors:     []models.FactorType{models.FactorTypeTOTP, models.FactorTypeSMS, models.FactorTypeEmail},
		ChallengeTTL:       300,
		MaxAttempts:        3,
		TrustDeviceAllowed: true,
		RiskAction:         "allow",
		Reason:             "default policy",
	}

	// Apply evaluation engine first
	if e.evaluator != nil {
		engineResult, err := e.evaluator.Evaluate(ctx, req)
		if err != nil {
			return nil, err
		}
		result = engineResult
	}

	// Apply rules in order (first match wins)
	for _, rule := range e.rules {
		if rule.Match(ctx, req) {
			ruleResult, err := rule.Action(ctx, req)
			if err != nil {
				return nil, err
			}
			result = ruleResult
			break
		}
	}

	return result, nil
}

// EvaluatePolicy determines if MFA is required for a user.
// Implements contracts.PolicyService.
// NOTE: This method uses zero values for risk/context — prefer EvaluateHTTPRequest
// when request context is available.
func (e *Engine) EvaluatePolicy(ctx context.Context, userID models.UserID) (bool, []models.FactorType, error) {
	req := &EvaluationRequest{
		UserID:    userID.String(),
		LastMFAAt: 0,
		RiskScore: 0,
	}
	result, err := e.Evaluate(ctx, req)
	if err != nil {
		return false, nil, err
	}
	return result.RequiresMFA, result.AllowedFactors, nil
}

// EvaluateHTTPRequest evaluates policy with actual request context.
// This is the Zero Trust–aware entry point that passes real risk scores,
// IP address, resource path, and device signals into the policy engine.
func (e *Engine) EvaluateHTTPRequest(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error) {
	return e.Evaluate(ctx, req)
}

// GetPolicy retrieves a policy by ID.
// Currently returns default policy as MFA policies are not yet implemented.
func (e *Engine) GetPolicy(ctx context.Context, policyID string) (*models.MFAPolicy, error) {
	// Placeholder - return default policy
	return &models.MFAPolicy{
		Enforcement: models.PolicyEnforcementOptional,
	}, nil
}

// DefaultEvaluator provides basic policy evaluation logic.
type DefaultEvaluator struct{}

// Evaluate returns a default evaluation result based on policy rules.
func (d *DefaultEvaluator) Evaluate(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error) {
	result := &EvaluationResult{
		AllowedFactors:     []models.FactorType{models.FactorTypeTOTP, models.FactorTypeSMS, models.FactorTypeEmail},
		ChallengeTTL:       300,
		MaxAttempts:        3,
		TrustDeviceAllowed: true,
	}

	// Adaptive risk-based logic
	if req.RiskScore > 75 {
		result.RequiresMFA = true
		result.RiskAction = "require"
		result.Reason = "High risk score detected"
		result.TrustDeviceAllowed = false
		return result, nil
	}

	if req.RiskScore > 50 {
		result.RequiresMFA = true
		result.RiskAction = "challenge"
		result.Reason = "Elevated risk score"
		return result, nil
	}

	// Privileged operations always require MFA
	if req.ResourceType == "sensitive-operation" {
		result.RequiresMFA = true
		result.Reason = "Resource requires MFA"
		return result, nil
	}

	// New devices may require challenge
	if req.IsNewDevice {
		result.RequiresMFA = true
		result.RiskAction = "challenge"
		result.Reason = "New device detected"
		result.TrustDeviceAllowed = true
		return result, nil
	}

	// Default: MFA optional
	result.RequiresMFA = false
	result.Reason = "Policy allows optional MFA"

	return result, nil
}

// IsFactorAllowed checks if a factor type is permitted by the policy.
func (r *EvaluationResult) IsFactorAllowed(ft models.FactorType) bool {
	for _, allowed := range r.AllowedFactors {
		if allowed == ft {
			return true
		}
	}
	return false
}

// ShouldBlock returns true if the risk action is "block".
func (r *EvaluationResult) ShouldBlock() bool {
	return r.RiskAction == "block"
}

// ShouldChallenge returns true if MFA challenge is required.
func (r *EvaluationResult) ShouldChallenge() bool {
	return r.RequiresMFA || r.RiskAction == "challenge"
}
