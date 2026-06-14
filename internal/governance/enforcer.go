package governance

// =====================================================
// WS-6.1 — Real-time Compliance Policy Enforcer
//
// Enforces compliance policies at request time by intercepting
// data access operations and checking them against active policies.
// Supports audit, warn, and block enforcement modes.
// =====================================================

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/governance/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
)

// EnforcementDecision represents the outcome of a policy check.
type EnforcementDecision struct {
	Allowed     bool                  `json:"allowed"`
	Mode        models.EnforcementMode       `json:"mode"`
	Violations  []EnforcementViolation `json:"violations,omitempty"`
	Message     string                `json:"message"`
	EvaluatedAt time.Time             `json:"evaluatedAt"`
}

// EnforcementViolation describes a single policy violation at enforcement time.
type EnforcementViolation struct {
	PolicyName  string `json:"policyName"`
	RuleID      string `json:"ruleId"`
	RuleName    string `json:"ruleName"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// AccessContext describes the context of a data access operation being enforced.
type AccessContext struct {
	UserID          string            `json:"userId"`
	Roles           []string          `json:"roles"`
	AssetRef        string            `json:"assetRef"`
	Operation       string            `json:"operation"` // read, write, delete, export
	DataSourceRef   string            `json:"dataSourceRef,omitempty"`
	Columns         []string          `json:"columns,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Classification  string            `json:"classification,omitempty"`
}

// PolicyProvider abstracts retrieval of active compliance policies.
type PolicyProvider interface {
	ListActivePolicies(ctx context.Context) ([]*models.CompliancePolicyResource, error)
}

// EnforcementLogger abstracts audit logging for enforcement decisions.
type EnforcementLogger interface {
	LogDecision(ctx context.Context, access AccessContext, decision EnforcementDecision) error
}

// Enforcer evaluates data access operations against compliance policies in real time.
type Enforcer struct {
	mu       sync.RWMutex
	provider PolicyProvider
	logger   EnforcementLogger

	// Cached policies (refreshed periodically).
	policies    []*models.CompliancePolicyResource
	lastRefresh time.Time
	cacheTTL    time.Duration
}

// NewEnforcer creates a new compliance policy enforcer.
func NewEnforcer(provider PolicyProvider, logger EnforcementLogger) *Enforcer {
	return &Enforcer{
		provider: provider,
		logger:   logger,
		cacheTTL: 30 * time.Second,
	}
}

// Enforce evaluates an access operation against all active policies.
func (e *Enforcer) Enforce(ctx context.Context, access AccessContext) (*EnforcementDecision, error) {
	policies, err := e.getActivePolicies(ctx)
	if err != nil {
		return nil, fmt.Errorf("enforcer: failed to load policies: %w", err)
	}

	decision := &EnforcementDecision{
		Allowed:     true,
		Mode:        models.EnforcementAudit,
		EvaluatedAt: time.Now(),
	}

	for _, policy := range policies {
		if !policy.Spec.Enabled {
			continue
		}

		// Check if this policy applies to the accessed asset.
		if !e.policyApplies(policy, access) {
			continue
		}

		// Evaluate each rule.
		for _, rule := range policy.Spec.Rules {
			if violation := e.evaluateRule(rule, access); violation != nil {
				violation.PolicyName = policy.Spec.DisplayName
				decision.Violations = append(decision.Violations, *violation)

				// Determine enforcement mode — strictest mode wins.
				if isStricterMode(policy.Spec.Enforcement, decision.Mode) {
					decision.Mode = policy.Spec.Enforcement
				}
			}
		}
	}

	if len(decision.Violations) > 0 {
		switch decision.Mode {
		case models.EnforcementBlock:
			decision.Allowed = false
			decision.Message = fmt.Sprintf("access blocked: %d policy violation(s)", len(decision.Violations))
		case models.EnforcementWarn:
			decision.Allowed = true
			decision.Message = fmt.Sprintf("access allowed with warning: %d policy violation(s)", len(decision.Violations))
		default: // audit
			decision.Allowed = true
			decision.Message = fmt.Sprintf("access allowed (audit): %d policy violation(s) logged", len(decision.Violations))
		}
	} else {
		decision.Message = "access allowed: no policy violations"
	}

	// Log the decision.
	if e.logger != nil {
		if logErr := e.logger.LogDecision(ctx, access, *decision); logErr != nil {
			logging.Z().Info(fmt.Sprintf("governance: failed to log enforcement decision: %v", logErr))
		}
	}

	return decision, nil
}

// --- Internal ---

func (e *Enforcer) getActivePolicies(ctx context.Context) ([]*models.CompliancePolicyResource, error) {
	e.mu.RLock()
	if time.Since(e.lastRefresh) < e.cacheTTL && e.policies != nil {
		policies := e.policies
		e.mu.RUnlock()
		return policies, nil
	}
	e.mu.RUnlock()

	// Refresh cache.
	if e.provider == nil {
		return nil, nil
	}

	policies, err := e.provider.ListActivePolicies(ctx)
	if err != nil {
		return nil, err
	}

	e.mu.Lock()
	e.policies = policies
	e.lastRefresh = time.Now()
	e.mu.Unlock()

	return policies, nil
}

func (e *Enforcer) policyApplies(policy *models.CompliancePolicyResource, access AccessContext) bool {
	scope := policy.Spec.Scope

	if scope.AllAssets {
		return true
	}

	// Check domain match.
	if len(scope.Domains) > 0 {
		for _, domain := range scope.Domains {
			if strings.Contains(access.AssetRef, domain) {
				return true
			}
		}
	}

	// Check datasource match.
	if len(scope.DataSources) > 0 {
		for _, ds := range scope.DataSources {
			if strings.EqualFold(ds, access.DataSourceRef) {
				return true
			}
		}
	}

	// Check classification match.
	if len(scope.Classifications) > 0 {
		for _, cls := range scope.Classifications {
			if strings.EqualFold(cls, access.Classification) {
				return true
			}
		}
	}

	// Check tag match.
	if len(scope.Tags) > 0 {
		for _, tag := range scope.Tags {
			if v, ok := access.Labels[tag]; ok && v != "" {
				return true
			}
		}
	}

	return false
}

func (e *Enforcer) evaluateRule(rule models.ComplianceRule, access AccessContext) *EnforcementViolation {
	switch rule.Type {
	case "access":
		return e.enforceAccessRule(rule, access)
	case "encryption":
		return e.enforceEncryptionRule(rule, access)
	case "masking":
		return e.enforceMaskingRule(rule, access)
	default:
		return nil // Other rule types are evaluated by the batch reconciler
	}
}

func (e *Enforcer) enforceAccessRule(rule models.ComplianceRule, access AccessContext) *EnforcementViolation {
	if rule.Access == nil {
		return nil
	}

	// Check forbidden roles.
	for _, forbidden := range rule.Access.ForbiddenRoles {
		for _, role := range access.Roles {
			if strings.EqualFold(role, forbidden) {
				return &EnforcementViolation{
					RuleID:      rule.ID,
					RuleName:    rule.Name,
					Severity:    "critical",
					Description: fmt.Sprintf("user has forbidden role '%s' for asset '%s'", forbidden, access.AssetRef),
				}
			}
		}
	}

	// Check max access level.
	if rule.Access.MaxAccessLevel != "" {
		if accessLevelExceeds(access.Operation, rule.Access.MaxAccessLevel) {
			return &EnforcementViolation{
				RuleID:      rule.ID,
				RuleName:    rule.Name,
				Severity:    "warning",
				Description: fmt.Sprintf("operation '%s' exceeds max access level '%s'", access.Operation, rule.Access.MaxAccessLevel),
			}
		}
	}

	return nil
}

func (e *Enforcer) enforceEncryptionRule(rule models.ComplianceRule, access AccessContext) *EnforcementViolation {
	if rule.Encryption == nil {
		return nil
	}
	// Encryption enforcement is primarily handled by the batch reconciler.
	// Real-time enforcement only applies to export operations.
	if access.Operation == "export" && rule.Encryption.RequireInTransit {
		return &EnforcementViolation{
			RuleID:      rule.ID,
			RuleName:    rule.Name,
			Severity:    "warning",
			Description: fmt.Sprintf("export of '%s' requires in-transit encryption", access.AssetRef),
		}
	}
	return nil
}

func (e *Enforcer) enforceMaskingRule(rule models.ComplianceRule, access AccessContext) *EnforcementViolation {
	if rule.Masking == nil {
		return nil
	}

	// Check if user is exempt.
	for _, exempt := range rule.Masking.ExemptRoles {
		for _, role := range access.Roles {
			if strings.EqualFold(role, exempt) {
				return nil
			}
		}
	}

	// Check if accessed columns match masking patterns.
	for _, pattern := range rule.Masking.ColumnPatterns {
		for _, col := range access.Columns {
			if matchesPattern(col, pattern) {
				return &EnforcementViolation{
					RuleID:      rule.ID,
					RuleName:    rule.Name,
					Severity:    "warning",
					Description: fmt.Sprintf("column '%s' requires masking (pattern: %s)", col, pattern),
				}
			}
		}
	}

	return nil
}

// --- Helpers ---

func isStricterMode(a, b models.EnforcementMode) bool {
	order := map[models.EnforcementMode]int{models.EnforcementAudit: 0, models.EnforcementWarn: 1, models.EnforcementBlock: 2}
	return order[a] > order[b]
}

func accessLevelExceeds(operation, maxLevel string) bool {
	levels := map[string]int{"read": 0, "write": 1, "delete": 2, "admin": 3, "export": 1}
	maxLevels := map[string]int{"read": 0, "write": 1, "admin": 3}
	return levels[operation] > maxLevels[maxLevel]
}

func matchesPattern(value, pattern string) bool {
	// Simple wildcard: "*email*" matches "user_email", "email_address", etc.
	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		for _, part := range parts {
			if part != "" && !strings.Contains(strings.ToLower(value), strings.ToLower(part)) {
				return false
			}
		}
		return true
	}
	return strings.EqualFold(value, pattern)
}
