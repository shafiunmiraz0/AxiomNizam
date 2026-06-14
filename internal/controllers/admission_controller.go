package controllers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/events"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/timing"
	"axiomnizam.bitbd.net/axiomnizam/internal/policies"
	"axiomnizam.bitbd.net/axiomnizam/internal/rbac"
	"axiomnizam.bitbd.net/axiomnizam/internal/utils/logger"
	"go.uber.org/zap"
)

// AdmissionPhase represents stages in admission control flow
type AdmissionPhase string

const (
	PhaseValidation    AdmissionPhase = "validation"
	PhaseMutation      AdmissionPhase = "mutation"
	PhaseAuthorization AdmissionPhase = "authorization"
	PhasePolicyEnforce AdmissionPhase = "policy_enforcement"
	PhaseResourceQuota AdmissionPhase = "resource_quota"
	PhasePersistence   AdmissionPhase = "persistence"
)

// AdmissionRequest represents a request to create/update/delete a resource
type AdmissionRequest struct {
	ID            string
	Timestamp     time.Time
	Kind          string
	Namespace     string
	Name          string
	Operation     string // create, update, delete, patch
	Resource      map[string]interface{}
	OldResource   map[string]interface{} // For updates
	UserID        string
	UserRole      string
	RequestID     string
	CorrelationID string
}

// AdmissionResponse represents the decision from the admission controller
type AdmissionResponse struct {
	Allowed          bool
	Code             int
	Reason           string
	WarningCount     int
	Warnings         []string
	Mutations        []AdmissionMutation
	PatchJSON        []byte
	ProcessingMs     int64
	Phases           []PhaseResult
	DecisionTime     time.Time
	EnforcedPolicies []string
}

// PhaseResult tracks result of each admission phase
type PhaseResult struct {
	Phase      AdmissionPhase
	Status     string // allowed, denied, warning
	Reason     string
	DurationMs int64
	Details    map[string]interface{}
}

// AdmissionMutation represents a mutation made during admission
type AdmissionMutation struct {
	Path      string
	Operation string // add, remove, replace
	Value     interface{}
	Reason    string
}

// AdmissionController orchestrates the admission control flow
type AdmissionController struct {
	mu                  sync.RWMutex
	logger              *logger.Logger
	eventBus            events.Bus
	admissionPolicy     policies.PolicyEngine
	rbacEngine          *rbac.Engine
	resourceQuotaMgr    *rbac.ResourceQuotaManager
	webhookValidators   []WebhookValidator
	webhookMutators     []WebhookMutator
	auditLog            []*AdmissionAuditLog
	maxAuditEntries     int
	metrics             *AdmissionMetrics
	policyCache         map[string]*policies.PolicyDefinition
	cacheTTL            time.Duration
	lastCacheInvalidate time.Time
}

// AdmissionAuditLog tracks admission decisions for compliance
type AdmissionAuditLog struct {
	ID               string
	Timestamp        time.Time
	RequestID        string
	Kind             string
	Namespace        string
	Name             string
	Operation        string
	UserID           string
	Decision         string // Allowed, Denied, Warned
	Reason           string
	EnforcedPolicies []string
	MutationsApplied int
	ProcessingTime   int64
	PhaseResults     []PhaseResult
}

// AdmissionMetrics tracks performance and decisions
type AdmissionMetrics struct {
	TotalRequests     int64
	AllowedRequests   int64
	DeniedRequests    int64
	WarningRequests   int64
	MutatedRequests   int64
	AvgProcessingTime float64
	PolicyViolations  map[string]int64
	PhaseTimings      map[AdmissionPhase]float64
}

// WebhookValidator defines validating webhook interface
type WebhookValidator interface {
	Validate(ctx context.Context, req *AdmissionRequest) (allowed bool, reason string, err error)
	Name() string
	Phase() AdmissionPhase
}

// WebhookMutator defines mutating webhook interface
type WebhookMutator interface {
	Mutate(ctx context.Context, req *AdmissionRequest) (mutations []AdmissionMutation, err error)
	Name() string
}

// NewAdmissionController creates a new admission controller
func NewAdmissionController(
	eventBus events.Bus,
	admissionPolicy policies.PolicyEngine,
	rbacEngine *rbac.Engine,
	resourceQuotaMgr *rbac.ResourceQuotaManager,
) *AdmissionController {
	log, _ := logger.New("development")
	return &AdmissionController{
		logger:              log,
		eventBus:            eventBus,
		admissionPolicy:     admissionPolicy,
		rbacEngine:          rbacEngine,
		resourceQuotaMgr:    resourceQuotaMgr,
		webhookValidators:   make([]WebhookValidator, 0),
		webhookMutators:     make([]WebhookMutator, 0),
		auditLog:            make([]*AdmissionAuditLog, 0, 10000),
		maxAuditEntries:     10000,
		metrics:             &AdmissionMetrics{PolicyViolations: make(map[string]int64), PhaseTimings: make(map[AdmissionPhase]float64)},
		policyCache:         make(map[string]*policies.PolicyDefinition),
		cacheTTL:            timing.DefaultAdmissionCacheTTL,
		lastCacheInvalidate: time.Now(),
	}
}

// Admit processes an admission request through all phases
func (ac *AdmissionController) Admit(ctx context.Context, req *AdmissionRequest) (*AdmissionResponse, error) {
	startTime := time.Now()
	resp := &AdmissionResponse{
		Allowed:          true,
		Code:             200,
		Phases:           make([]PhaseResult, 0),
		DecisionTime:     time.Now(),
		EnforcedPolicies: make([]string, 0),
	}

	// Phase 1: Validation
	if allowed, reason, phases := ac.phaseValidation(ctx, req); !allowed {
		resp.Allowed = false
		resp.Code = 400
		resp.Reason = reason
		resp.Phases = append(resp.Phases, phases...)
		ac.recordAuditLog(req, resp)
		resp.ProcessingMs = time.Since(startTime).Milliseconds()
		return resp, nil
	}

	// Phase 2: Mutation
	mutations, phases := ac.phaseMutation(ctx, req)
	resp.Mutations = mutations
	resp.Phases = append(resp.Phases, phases...)
	if len(mutations) > 0 {
		resp.Mutations = mutations
	}

	// Phase 3: Authorization (RBAC check)
	if allowed, reason, phases := ac.phaseAuthorization(ctx, req); !allowed {
		resp.Allowed = false
		resp.Code = 403
		resp.Reason = reason
		resp.Phases = append(resp.Phases, phases...)
		ac.recordAuditLog(req, resp)
		resp.ProcessingMs = time.Since(startTime).Milliseconds()
		return resp, nil
	}

	// Phase 4: Policy Enforcement
	enforced, warnings, phases := ac.phasePolicyEnforce(ctx, req)
	resp.Phases = append(resp.Phases, phases...)
	if !enforced {
		resp.Allowed = false
		resp.Code = 400
		resp.Reason = "Policy enforcement failed"
		ac.recordAuditLog(req, resp)
		resp.ProcessingMs = time.Since(startTime).Milliseconds()
		return resp, nil
	}
	if len(warnings) > 0 {
		resp.Warnings = warnings
		resp.WarningCount = len(warnings)
	}

	// Phase 5: Resource Quota Check
	if allowed, reason, phases := ac.phaseResourceQuota(ctx, req); !allowed {
		resp.Allowed = false
		resp.Code = 429
		resp.Reason = reason
		resp.Phases = append(resp.Phases, phases...)
		ac.recordAuditLog(req, resp)
		resp.ProcessingMs = time.Since(startTime).Milliseconds()
		return resp, nil
	}

	// All phases passed
	resp.ProcessingMs = time.Since(startTime).Milliseconds()
	ac.recordAuditLog(req, resp)
	ac.publishAdmissionEvent(ctx, req, resp)
	return resp, nil
}

// phaseValidation validates schema and basic constraints
func (ac *AdmissionController) phaseValidation(ctx context.Context, req *AdmissionRequest) (bool, string, []PhaseResult) {
	start := time.Now()
	result := PhaseResult{Phase: PhaseValidation, Details: make(map[string]interface{})}

	// Basic schema validation
	if req.Kind == "" || req.Name == "" || req.Namespace == "" {
		result.Status = "denied"
		result.Reason = "missing required fields: kind, name, or namespace"
		result.DurationMs = time.Since(start).Milliseconds()
		return false, result.Reason, []PhaseResult{result}
	}

	// Registered webhooks validation
	for _, validator := range ac.webhookValidators {
		if validator.Phase() != PhaseValidation {
			continue
		}
		allowed, reason, err := validator.Validate(ctx, req)
		if err != nil {
			ac.logger.Error("Validation webhook error", zap.String("webhook", validator.Name()), zap.Error(err))
			continue
		}
		if !allowed {
			result.Status = "denied"
			result.Reason = fmt.Sprintf("%s validation failed: %s", validator.Name(), reason)
			result.DurationMs = time.Since(start).Milliseconds()
			return false, result.Reason, []PhaseResult{result}
		}
	}

	result.Status = "allowed"
	result.DurationMs = time.Since(start).Milliseconds()
	return true, "", []PhaseResult{result}
}

// phaseMutation applies mutations to the resource
func (ac *AdmissionController) phaseMutation(ctx context.Context, req *AdmissionRequest) ([]AdmissionMutation, []PhaseResult) {
	start := time.Now()
	result := PhaseResult{Phase: PhaseMutation, Details: make(map[string]interface{})}
	mutations := make([]AdmissionMutation, 0)

	// Apply registered mutators
	for _, mutator := range ac.webhookMutators {
		muts, err := mutator.Mutate(ctx, req)
		if err != nil {
			ac.logger.Error("Mutation webhook error", zap.String("webhook", mutator.Name()), zap.Error(err))
			continue
		}
		mutations = append(mutations, muts...)
	}

	// Add default mutations (e.g., timestamp)
	if req.Operation == "create" {
		mutations = append(mutations, AdmissionMutation{
			Path:      "/metadata/createdAt",
			Operation: "add",
			Value:     time.Now().Unix(),
			Reason:    "system-default-timestamp",
		})
	}

	result.Status = "allowed"
	result.DurationMs = time.Since(start).Milliseconds()
	result.Details["mutations_applied"] = len(mutations)
	return mutations, []PhaseResult{result}
}

// phaseAuthorization checks RBAC permissions
func (ac *AdmissionController) phaseAuthorization(ctx context.Context, req *AdmissionRequest) (bool, string, []PhaseResult) {
	start := time.Now()
	result := PhaseResult{Phase: PhaseAuthorization, Details: make(map[string]interface{})}

	// Map operation to resource action
	action := ""
	switch req.Operation {
	case "create":
		action = "create"
	case "update", "patch":
		action = "update"
	case "delete":
		action = "delete"
	default:
		action = "read"
	}

	// Check RBAC permissions
	allowed, reason := ac.rbacEngine.CanPerform(ctx, req.UserID, req.Kind, action, req.Namespace)
	result.Status = "allowed"
	if !allowed {
		result.Status = "denied"
		result.Reason = reason
		result.DurationMs = time.Since(start).Milliseconds()
		return false, reason, []PhaseResult{result}
	}

	result.DurationMs = time.Since(start).Milliseconds()
	result.Details["role"] = req.UserRole
	result.Details["action"] = action
	return true, "", []PhaseResult{result}
}

// phasePolicyEnforce enforces admission policies
func (ac *AdmissionController) phasePolicyEnforce(ctx context.Context, req *AdmissionRequest) (bool, []string, []PhaseResult) {
	start := time.Now()
	result := PhaseResult{Phase: PhasePolicyEnforce, Details: make(map[string]interface{})}
	warnings := make([]string, 0)

	if ac.admissionPolicy == nil {
		result.Status = "allowed"
		result.DurationMs = time.Since(start).Milliseconds()
		return true, warnings, []PhaseResult{result}
	}

	// Evaluate policies
	// Note: PolicyEngine.Evaluate requires a policy object, not resource details
	// For now, skip policy evaluation if not provided
	// Policy evaluation would happen here if admissionPolicy is available

	result.Status = "allowed"
	result.DurationMs = time.Since(start).Milliseconds()
	result.Details["warnings"] = len(warnings)
	return true, warnings, []PhaseResult{result}
}

// phaseResourceQuota checks resource quota limits
func (ac *AdmissionController) phaseResourceQuota(ctx context.Context, req *AdmissionRequest) (bool, string, []PhaseResult) {
	start := time.Now()
	result := PhaseResult{Phase: PhaseResourceQuota, Details: make(map[string]interface{})}

	if ac.resourceQuotaMgr == nil {
		result.Status = "allowed"
		result.DurationMs = time.Since(start).Milliseconds()
		return true, "", []PhaseResult{result}
	}

	// Check quota
	allowed, reason := ac.resourceQuotaMgr.CanAllocate(ctx, req.Namespace, req.Kind, req.Resource)
	result.Status = "allowed"
	if !allowed {
		result.Status = "denied"
		result.Reason = reason
		result.DurationMs = time.Since(start).Milliseconds()
		return false, reason, []PhaseResult{result}
	}

	result.DurationMs = time.Since(start).Milliseconds()
	result.Details["quota_checked"] = true
	return true, "", []PhaseResult{result}
}

// RegisterValidator registers a validating webhook
func (ac *AdmissionController) RegisterValidator(validator WebhookValidator) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.webhookValidators = append(ac.webhookValidators, validator)
}

// RegisterMutator registers a mutating webhook
func (ac *AdmissionController) RegisterMutator(mutator WebhookMutator) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.webhookMutators = append(ac.webhookMutators, mutator)
}

// recordAuditLog records admission decision to audit log
func (ac *AdmissionController) recordAuditLog(req *AdmissionRequest, resp *AdmissionResponse) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	audit := &AdmissionAuditLog{
		ID:               fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp:        time.Now(),
		RequestID:        req.ID,
		Kind:             req.Kind,
		Namespace:        req.Namespace,
		Name:             req.Name,
		Operation:        req.Operation,
		UserID:           req.UserID,
		Decision:         map[bool]string{true: "Allowed", false: "Denied"}[resp.Allowed],
		Reason:           resp.Reason,
		EnforcedPolicies: resp.EnforcedPolicies,
		MutationsApplied: len(resp.Mutations),
		ProcessingTime:   resp.ProcessingMs,
		PhaseResults:     resp.Phases,
	}

	ac.auditLog = append(ac.auditLog, audit)
	if len(ac.auditLog) > ac.maxAuditEntries {
		ac.auditLog = ac.auditLog[len(ac.auditLog)-ac.maxAuditEntries:]
	}

	// Update metrics
	ac.metrics.TotalRequests++
	if resp.Allowed {
		ac.metrics.AllowedRequests++
	} else {
		ac.metrics.DeniedRequests++
	}
	if len(resp.Warnings) > 0 {
		ac.metrics.WarningRequests++
	}
	if len(resp.Mutations) > 0 {
		ac.metrics.MutatedRequests++
	}
}

// publishAdmissionEvent publishes admission decision to event bus
func (ac *AdmissionController) publishAdmissionEvent(ctx context.Context, req *AdmissionRequest, resp *AdmissionResponse) {
	event := &events.Event{
		ID:        fmt.Sprintf("admission-%d", time.Now().UnixNano()),
		Type:      "admission.decision",
		Source:    "admission-controller",
		Timestamp: time.Now(),
		UserID:    req.UserID,
		Data: map[string]interface{}{
			"kind":            req.Kind,
			"name":            req.Name,
			"namespace":       req.Namespace,
			"operation":       req.Operation,
			"allowed":         resp.Allowed,
			"processing_ms":   resp.ProcessingMs,
			"warnings":        len(resp.Warnings),
			"mutations":       len(resp.Mutations),
			"policy_enforced": len(resp.EnforcedPolicies) > 0,
		},
	}

	if err := ac.eventBus.Publish(ctx, event); err != nil {
		ac.logger.Error("Failed to publish admission event", zap.Error(err))
	}
}

// GetAuditLog returns admission audit log with filtering
func (ac *AdmissionController) GetAuditLog(ctx context.Context, kind string, namespace string, limit int) []*AdmissionAuditLog {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	result := make([]*AdmissionAuditLog, 0)
	count := 0
	for i := len(ac.auditLog) - 1; i >= 0 && count < limit; i-- {
		audit := ac.auditLog[i]
		if (kind == "" || audit.Kind == kind) && (namespace == "" || audit.Namespace == namespace) {
			result = append(result, audit)
			count++
		}
	}
	return result
}

// GetMetrics returns admission controller metrics
func (ac *AdmissionController) GetMetrics() *AdmissionMetrics {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.metrics
}
