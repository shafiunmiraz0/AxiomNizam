// Package-level note (P2.3): this file was moved from internal/controllers/rbac_engine.go
// so the two RBAC surfaces live together.  Colliding type names were
// prefixed with `Engine` (EngineRole, EngineRoleBinding, EngineSubject, etc.)
// to coexist with the tenant-oriented types already defined in this package.

package rbac

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

// RequestMetadataKey is the context key for passing HTTP request metadata
// into the RBAC engine for condition evaluation (IP restrictions, time windows).
const RequestMetadataKey contextKey = "rbac_request_metadata"

// RequestMetadata carries HTTP request context into the RBAC engine
// so that EngineRuleCondition types can be evaluated.
type RequestMetadata struct {
	IPAddress   string
	RequestTime time.Time
	UserAgent   string
}

// Engine provides Kubernetes-style RBAC (Role-Based Access Control)
// Supports resource-level permissions with verb-based actions
type Engine struct {
	mu           sync.RWMutex
	roles        map[string]*EngineRole
	roleBindings map[string][]*EngineRoleBinding
	clusterRoles map[string]*EngineClusterRole
	clusterRbacs map[string][]*EngineClusterRoleBinding
	auditLog     []*EngineAuditLog
	maxAuditLog  int
	apiGroups    map[string]bool // Track allowed API groups
}

// Role defines permissions for a namespace scope
type EngineRole struct {
	Name        string
	Namespace   string
	Rules       []*EnginePolicyRule
	CreatedAt   time.Time
	Labels      map[string]string
	Annotations map[string]string
}

// RoleBinding binds a Role to subjects (users/groups/serviceaccounts)
type EngineRoleBinding struct {
	Name      string
	Namespace string
	Role      string
	Subjects  []*EngineSubject
	Labels    map[string]string
}

// ClusterRole defines cluster-wide permissions
type EngineClusterRole struct {
	Name        string
	Rules       []*EnginePolicyRule
	CreatedAt   time.Time
	Labels      map[string]string
	Annotations map[string]string
}

// ClusterRoleBinding binds a ClusterRole to subjects cluster-wide
type EngineClusterRoleBinding struct {
	Name     string
	Role     string
	Subjects []*EngineSubject
	Labels   map[string]string
}

// PolicyRule defines what verbs can be performed on which resources
type EnginePolicyRule struct {
	Verbs           []string               // create, read, update, delete, list, watch, patch
	APIGroups       []string               // e.g., "", "batch", "apps"
	Resources       []string               // e.g., "pods", "deployments", "jobs"
	ResourceNames   []string               // specific resource names (optional)
	NonResourceURLs []string               // for non-resource endpoints
	Conditions      []*EngineRuleCondition // additional conditions
}

// RuleCondition adds fine-grained control to policy rules
type EngineRuleCondition struct {
	Type  string      // "OwnershipRequired", "LabelSelector", "TimeWindow", "IPRestriction"
	Value interface{} // varies by condition type
}

// Subject represents a user, group, or service account
type EngineSubject struct {
	Type      string // User, Group, ServiceAccount
	Name      string
	Namespace string // only for ServiceAccount
}

// RBACAuditLog tracks RBAC decisions for compliance
type EngineAuditLog struct {
	ID          string
	Timestamp   time.Time
	UserID      string
	Subject     *EngineSubject
	Action      string // create, read, update, delete, list, watch, patch
	Resource    string
	APIGroup    string
	Namespace   string
	Allowed     bool
	Reason      string
	MatchedRule *EnginePolicyRule
}

// RBACDecision represents the outcome of an RBAC check
type EngineDecision struct {
	Allowed      bool
	Reason       string
	MatchedRules []*EnginePolicyRule
	DecisionTime time.Time
}

// NewEngine creates a new RBAC engine
func NewEngine() *Engine {
	return &Engine{
		roles:        make(map[string]*EngineRole),
		roleBindings: make(map[string][]*EngineRoleBinding),
		clusterRoles: make(map[string]*EngineClusterRole),
		clusterRbacs: make(map[string][]*EngineClusterRoleBinding),
		auditLog:     make([]*EngineAuditLog, 0, 10000),
		maxAuditLog:  10000,
		apiGroups:    make(map[string]bool),
	}
}

// CreateRole creates a new namespaced role
func (re *Engine) CreateRole(ctx context.Context, role *EngineRole) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	if role.Name == "" || role.Namespace == "" {
		return fmt.Errorf("role name and namespace required")
	}

	key := fmt.Sprintf("%s/%s", role.Namespace, role.Name)
	if _, exists := re.roles[key]; exists {
		return fmt.Errorf("role already exists")
	}

	role.CreatedAt = time.Now()
	re.roles[key] = role
	return nil
}

// CreateClusterRole creates a cluster-wide role
func (re *Engine) CreateClusterRole(ctx context.Context, role *EngineClusterRole) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	if role.Name == "" {
		return fmt.Errorf("cluster role name required")
	}

	if _, exists := re.clusterRoles[role.Name]; exists {
		return fmt.Errorf("cluster role already exists")
	}

	role.CreatedAt = time.Now()
	re.clusterRoles[role.Name] = role
	return nil
}

// CreateRoleBinding binds a role to subjects
func (re *Engine) CreateRoleBinding(ctx context.Context, binding *EngineRoleBinding) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	if binding.Name == "" || binding.Namespace == "" || binding.Role == "" {
		return fmt.Errorf("binding name, namespace, and role required")
	}

	key := fmt.Sprintf("%s/%s", binding.Namespace, binding.Name)
	re.roleBindings[key] = append(re.roleBindings[key], binding)
	return nil
}

// CreateClusterRoleBinding binds a cluster role to subjects
func (re *Engine) CreateClusterRoleBinding(ctx context.Context, binding *EngineClusterRoleBinding) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	if binding.Name == "" || binding.Role == "" {
		return fmt.Errorf("binding name and role required")
	}

	re.clusterRbacs[binding.Name] = append(re.clusterRbacs[binding.Name], binding)
	return nil
}

// CanPerform checks if a subject can perform an action on a resource
func (re *Engine) CanPerform(ctx context.Context, userID string, resourceKind string, verb string, namespace string) (bool, string) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	subject := &EngineSubject{Type: "User", Name: userID}
	decision := &EngineDecision{
		Allowed:      false,
		Reason:       "no matching rules",
		MatchedRules: make([]*EnginePolicyRule, 0),
		DecisionTime: time.Now(),
	}

	// Check cluster-wide roles first
	allowed, rules := re.checkClusterRoles(ctx, subject, verb, resourceKind)
	if allowed {
		decision.Allowed = true
		decision.MatchedRules = append(decision.MatchedRules, rules...)
	}

	// Check namespaced roles
	if !allowed && namespace != "" {
		allowed, rules := re.checkNamespacedRoles(ctx, subject, verb, resourceKind, namespace)
		if allowed {
			decision.Allowed = true
			decision.MatchedRules = append(decision.MatchedRules, rules...)
		}
	}

	if !decision.Allowed {
		decision.Reason = fmt.Sprintf("no permissions for %s on %s", verb, resourceKind)
	} else {
		decision.Reason = "allowed by matching rules"
	}

	// Record audit log
	re.recordAuditLog(&EngineAuditLog{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		UserID:    userID,
		Subject:   subject,
		Action:    verb,
		Resource:  resourceKind,
		Namespace: namespace,
		Allowed:   decision.Allowed,
		Reason:    decision.Reason,
	})

	return decision.Allowed, decision.Reason
}

// checkClusterRoles checks if subject has permission via cluster roles
func (re *Engine) checkClusterRoles(ctx context.Context, subject *EngineSubject, verb string, resourceKind string) (bool, []*EnginePolicyRule) {
	matchedRules := make([]*EnginePolicyRule, 0)

	// Find bindings for this subject
	for _, bindings := range re.clusterRbacs {
		for _, binding := range bindings {
			if re.subjectMatches(subject, binding.Subjects) {
				// Get the cluster role
				if role, exists := re.clusterRoles[binding.Role]; exists {
					// Check if role has the required permission
					if rules := re.checkRuleMatch(ctx, role.Rules, verb, resourceKind); len(rules) > 0 {
						matchedRules = append(matchedRules, rules...)
						return true, matchedRules
					}
				}
			}
		}
	}

	return len(matchedRules) > 0, matchedRules
}

// checkNamespacedRoles checks if subject has permission via namespaced roles
func (re *Engine) checkNamespacedRoles(ctx context.Context, subject *EngineSubject, verb string, resourceKind string, namespace string) (bool, []*EnginePolicyRule) {
	matchedRules := make([]*EnginePolicyRule, 0)

	// Find bindings in the namespace
	for key, bindings := range re.roleBindings {
		parts := strings.Split(key, "/")
		if len(parts) == 2 && parts[0] != namespace {
			continue // Different namespace
		}

		for _, binding := range bindings {
			if re.subjectMatches(subject, binding.Subjects) {
				// Get the role
				roleKey := fmt.Sprintf("%s/%s", binding.Namespace, binding.Role)
				if role, exists := re.roles[roleKey]; exists {
					// Check if role has the required permission
					if rules := re.checkRuleMatch(ctx, role.Rules, verb, resourceKind); len(rules) > 0 {
						matchedRules = append(matchedRules, rules...)
						return true, matchedRules
					}
				}
			}
		}
	}

	return len(matchedRules) > 0, matchedRules
}

// checkRuleMatch checks if any rule matches the verb and resource.
// Phase 3: conditions are now evaluated — a rule matches only if verb+resource
// match AND all conditions pass.
func (re *Engine) checkRuleMatch(ctx context.Context, rules []*EnginePolicyRule, verb string, resourceKind string) []*EnginePolicyRule {
	matched := make([]*EnginePolicyRule, 0)

	for _, rule := range rules {
		if re.verbMatches(rule.Verbs, verb) && re.resourceMatches(rule.Resources, resourceKind) {
			if re.evaluateConditions(ctx, rule.Conditions) {
				matched = append(matched, rule)
			}
		}
	}

	return matched
}

// evaluateConditions checks all conditions on a rule against the request metadata
// carried in context. Returns true if all conditions pass (AND logic).
// If no conditions are defined, returns true (unconditional match).
func (re *Engine) evaluateConditions(ctx context.Context, conditions []*EngineRuleCondition) bool {
	if len(conditions) == 0 {
		return true
	}

	meta, _ := ctx.Value(RequestMetadataKey).(*RequestMetadata)
	if meta == nil {
		// No metadata available — deny if conditions exist (secure default)
		return false
	}

	for _, cond := range conditions {
		if !re.evaluateOneCondition(meta, cond) {
			return false
		}
	}
	return true
}

// evaluateOneCondition evaluates a single EngineRuleCondition.
func (re *Engine) evaluateOneCondition(meta *RequestMetadata, cond *EngineRuleCondition) bool {
	switch cond.Type {
	case "IPRestriction":
		return re.evaluateIPRestriction(meta.IPAddress, cond.Value)
	case "TimeWindow":
		return re.evaluateTimeWindow(meta.RequestTime, cond.Value)
	default:
		// Unknown condition type — pass (don't block on unrecognized conditions)
		return true
	}
}

// evaluateIPRestriction checks if the client IP is within allowed CIDR ranges.
// Value should be []string of CIDR notation (e.g., ["10.0.0.0/8", "192.168.0.0/16"]).
func (re *Engine) evaluateIPRestriction(clientIP string, value interface{}) bool {
	if clientIP == "" {
		return false
	}

	cidrs, ok := value.([]string)
	if !ok {
		// Try []interface{} (JSON deserialization produces this)
		if ifaceSlice, ok := value.([]interface{}); ok {
			cidrs = make([]string, 0, len(ifaceSlice))
			for _, v := range ifaceSlice {
				if s, ok := v.(string); ok {
					cidrs = append(cidrs, s)
				}
			}
		} else {
			return false
		}
	}

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}

	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// evaluateTimeWindow checks if the request time falls within an allowed window.
// Value should be a map with "start" and "end" as "HH:MM" strings (24h format).
func (re *Engine) evaluateTimeWindow(requestTime time.Time, value interface{}) bool {
	windowMap, ok := value.(map[string]interface{})
	if !ok {
		return false
	}

	startStr, _ := windowMap["start"].(string)
	endStr, _ := windowMap["end"].(string)
	if startStr == "" || endStr == "" {
		return false
	}

	startParts := strings.SplitN(startStr, ":", 2)
	endParts := strings.SplitN(endStr, ":", 2)
	if len(startParts) != 2 || len(endParts) != 2 {
		return false
	}

	var startH, startM, endH, endM int
	fmt.Sscanf(startParts[0], "%d", &startH)
	fmt.Sscanf(startParts[1], "%d", &startM)
	fmt.Sscanf(endParts[0], "%d", &endH)
	fmt.Sscanf(endParts[1], "%d", &endM)

	currentMinutes := requestTime.Hour()*60 + requestTime.Minute()
	startMinutes := startH*60 + startM
	endMinutes := endH*60 + endM

	if startMinutes <= endMinutes {
		// Same-day window (e.g., 09:00-17:00)
		return currentMinutes >= startMinutes && currentMinutes <= endMinutes
	}
	// Overnight window (e.g., 22:00-06:00)
	return currentMinutes >= startMinutes || currentMinutes <= endMinutes
}

// verbMatches checks if verb is in the allowed verbs (supports wildcards)
func (re *Engine) verbMatches(verbs []string, verb string) bool {
	for _, v := range verbs {
		if v == "*" || v == verb {
			return true
		}
	}
	return false
}

// resourceMatches checks if resource is in the allowed resources (supports wildcards)
func (re *Engine) resourceMatches(resources []string, resource string) bool {
	for _, r := range resources {
		if r == "*" || r == resource {
			return true
		}
	}
	return false
}

// subjectMatches checks if subject matches any in the list
func (re *Engine) subjectMatches(subject *EngineSubject, subjects []*EngineSubject) bool {
	for _, s := range subjects {
		if s.Type == subject.Type && s.Name == subject.Name {
			if s.Type == "ServiceAccount" {
				if s.Namespace == "" || s.Namespace == subject.Namespace {
					return true
				}
			} else {
				return true
			}
		}
	}
	return false
}

// recordAuditLog records RBAC decision
func (re *Engine) recordAuditLog(audit *EngineAuditLog) {
	re.auditLog = append(re.auditLog, audit)
	if len(re.auditLog) > re.maxAuditLog {
		re.auditLog = re.auditLog[len(re.auditLog)-re.maxAuditLog:]
	}
}

// GetAuditLog returns RBAC audit log with filtering
func (re *Engine) GetAuditLog(ctx context.Context, userID string, allowed *bool, limit int) []*EngineAuditLog {
	re.mu.RLock()
	defer re.mu.RUnlock()

	result := make([]*EngineAuditLog, 0)
	count := 0

	for i := len(re.auditLog) - 1; i >= 0 && count < limit; i-- {
		audit := re.auditLog[i]
		if (userID == "" || audit.UserID == userID) &&
			(allowed == nil || audit.Allowed == *allowed) {
			result = append(result, audit)
			count++
		}
	}

	return result
}

// ListRoles returns all roles in a namespace
func (re *Engine) ListRoles(ctx context.Context, namespace string) []*EngineRole {
	re.mu.RLock()
	defer re.mu.RUnlock()

	result := make([]*EngineRole, 0)
	prefix := fmt.Sprintf("%s/", namespace)

	for key, role := range re.roles {
		if strings.HasPrefix(key, prefix) {
			result = append(result, role)
		}
	}

	return result
}

// ListClusterRoles returns all cluster roles
func (re *Engine) ListClusterRoles(ctx context.Context) []*EngineClusterRole {
	re.mu.RLock()
	defer re.mu.RUnlock()

	result := make([]*EngineClusterRole, 0)
	for _, role := range re.clusterRoles {
		result = append(result, role)
	}

	return result
}

// GetRole retrieves a specific role
func (re *Engine) GetRole(ctx context.Context, name string, namespace string) (*EngineRole, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", namespace, name)
	if role, exists := re.roles[key]; exists {
		return role, nil
	}

	return nil, fmt.Errorf("role not found")
}

// GetClusterRole retrieves a specific cluster role
func (re *Engine) GetClusterRole(ctx context.Context, name string) (*EngineClusterRole, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	if role, exists := re.clusterRoles[name]; exists {
		return role, nil
	}

	return nil, fmt.Errorf("cluster role not found")
}

// DeleteRole deletes a role
func (re *Engine) DeleteRole(ctx context.Context, name string, namespace string) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	key := fmt.Sprintf("%s/%s", namespace, name)
	if _, exists := re.roles[key]; !exists {
		return fmt.Errorf("role not found")
	}

	delete(re.roles, key)

	// Clean up associated bindings
	for k, bindings := range re.roleBindings {
		filtered := make([]*EngineRoleBinding, 0)
		for _, b := range bindings {
			if b.Role != name {
				filtered = append(filtered, b)
			}
		}
		if len(filtered) == 0 {
			delete(re.roleBindings, k)
		} else {
			re.roleBindings[k] = filtered
		}
	}

	return nil
}

// DeleteClusterRole deletes a cluster role
func (re *Engine) DeleteClusterRole(ctx context.Context, name string) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	if _, exists := re.clusterRoles[name]; !exists {
		return fmt.Errorf("cluster role not found")
	}

	delete(re.clusterRoles, name)

	// Clean up associated bindings
	for k, bindings := range re.clusterRbacs {
		filtered := make([]*EngineClusterRoleBinding, 0)
		for _, b := range bindings {
			if b.Role != name {
				filtered = append(filtered, b)
			}
		}
		if len(filtered) == 0 {
			delete(re.clusterRbacs, k)
		} else {
			re.clusterRbacs[k] = filtered
		}
	}

	return nil
}

// GetRBACStats returns RBAC statistics
func (re *Engine) GetRBACStats(ctx context.Context) map[string]interface{} {
	re.mu.RLock()
	defer re.mu.RUnlock()

	return map[string]interface{}{
		"roles":                 len(re.roles),
		"cluster_roles":         len(re.clusterRoles),
		"role_bindings":         len(re.roleBindings),
		"cluster_role_bindings": len(re.clusterRbacs),
		"audit_log_entries":     len(re.auditLog),
	}
}

// ResourceQuotaManager manages resource quotas per namespace
type ResourceQuotaManager struct {
	mu     sync.RWMutex
	quotas map[string]*Quota
}

// Quota defines resource limits for a namespace
type Quota struct {
	Namespace string
	Resources map[string]*ResourceLimit
	Used      map[string]int64
	CreatedAt time.Time
}

// ResourceLimit defines limits for a specific resource
type ResourceLimit struct {
	Kind        string
	MaxCount    int64
	MaxCPU      string
	MaxMemory   string
	Description string
}

// NewResourceQuotaManager creates a new quota manager
func NewResourceQuotaManager() *ResourceQuotaManager {
	return &ResourceQuotaManager{
		quotas: make(map[string]*Quota),
	}
}

// CreateQuota creates a new quota
func (rq *ResourceQuotaManager) CreateQuota(ctx context.Context, quota *Quota) error {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	if quota.Namespace == "" {
		return fmt.Errorf("namespace required")
	}

	if _, exists := rq.quotas[quota.Namespace]; exists {
		return fmt.Errorf("quota already exists")
	}

	quota.CreatedAt = time.Now()
	if quota.Used == nil {
		quota.Used = make(map[string]int64)
	}

	rq.quotas[quota.Namespace] = quota
	return nil
}

// CanAllocate checks if resource can be allocated
func (rq *ResourceQuotaManager) CanAllocate(ctx context.Context, namespace string, kind string, resource map[string]interface{}) (bool, string) {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	quota, exists := rq.quotas[namespace]
	if !exists {
		return true, "" // No quota defined
	}

	limit, hasLimit := quota.Resources[kind]
	if !hasLimit {
		return true, "" // No limit for this resource kind
	}

	used := quota.Used[kind]
	if used >= limit.MaxCount {
		return false, fmt.Sprintf("quota exceeded for %s: %d/%d", kind, used, limit.MaxCount)
	}

	return true, ""
}

// RecordUsage records resource usage
func (rq *ResourceQuotaManager) RecordUsage(ctx context.Context, namespace string, kind string, count int64) error {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	quota, exists := rq.quotas[namespace]
	if !exists {
		return fmt.Errorf("quota not found")
	}

	quota.Used[kind] += count
	return nil
}

// GetQuota retrieves quota for a namespace
func (rq *ResourceQuotaManager) GetQuota(ctx context.Context, namespace string) (*Quota, error) {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	if quota, exists := rq.quotas[namespace]; exists {
		return quota, nil
	}

	return nil, fmt.Errorf("quota not found")
}
