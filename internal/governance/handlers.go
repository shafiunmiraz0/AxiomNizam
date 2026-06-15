package governance

// =====================================================
// WS-6 — Governance REST API Handlers
//
// Provides CRUD for compliance policies, retention policies,
// access requests, and erasure workflows.
// =====================================================

import (
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/governance/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/validate"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GovernanceHandlers provides REST API handlers for governance.
type GovernanceHandlers struct {
	policyStore    store.ResourceStore[*models.CompliancePolicyResource]
	retentionStore store.ResourceStore[*models.RetentionPolicyResource]
	accessStore    store.ResourceStore[*models.AccessRequestResource]
}

// NewGovernanceHandlers creates new handlers.
func NewGovernanceHandlers(
	policyStore store.ResourceStore[*models.CompliancePolicyResource],
	retentionStore store.ResourceStore[*models.RetentionPolicyResource],
	accessStore store.ResourceStore[*models.AccessRequestResource],
) *GovernanceHandlers {
	return &GovernanceHandlers{
		policyStore:    policyStore,
		retentionStore: retentionStore,
		accessStore:    accessStore,
	}
}

// RegisterRoutes registers governance API routes.
func (h *GovernanceHandlers) RegisterRoutes(rg *gin.RouterGroup) {
	gov := rg.Group("/governance")
	{
		// Compliance policies
		gov.GET("/policies", h.ListPolicies)
		gov.GET("/policies/:name", h.GetPolicy)
		gov.POST("/policies", h.CreatePolicy)
		gov.PUT("/policies/:name", h.UpdatePolicy)
		gov.DELETE("/policies/:name", h.DeletePolicy)
		gov.POST("/policies/:name/audit", h.TriggerAudit)

		// Retention policies
		gov.GET("/retention", h.ListRetentionPolicies)
		gov.GET("/retention/:name", h.GetRetentionPolicy)
		gov.POST("/retention", h.CreateRetentionPolicy)
		gov.DELETE("/retention/:name", h.DeleteRetentionPolicy)

		// Access requests
		gov.GET("/access", h.ListAccessRequests)
		gov.GET("/access/:name", h.GetAccessRequest)
		gov.POST("/access", h.CreateAccessRequest)
		gov.POST("/access/:name/approve", h.ApproveAccessRequest)
		gov.POST("/access/:name/deny", h.DenyAccessRequest)
		gov.POST("/access/:name/revoke", h.RevokeAccessRequest)

		// Summary
		gov.GET("/summary", h.GetSummary)
	}
}

// --- Compliance Policy Handlers ---

func (h *GovernanceHandlers) ListPolicies(c *gin.Context) {
	policies, err := h.policyStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListPolicies"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	frameworkFilter := c.Query("framework")
	var filtered []*models.CompliancePolicyResource
	for _, p := range policies {
		if frameworkFilter != "" && string(p.Spec.Framework) != frameworkFilter {
			continue
		}
		filtered = append(filtered, p)
	}

	c.JSON(http.StatusOK, PolicyListResponse{Policies: filtered, Count: len(filtered)})
}

func (h *GovernanceHandlers) GetPolicy(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	policy, err := h.policyStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "policy not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, policy)
}

func (h *GovernanceHandlers) CreatePolicy(c *gin.Context) {
	var policy models.CompliancePolicyResource
	if err := c.ShouldBindJSON(&policy); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	policy.Kind = models.CompliancePolicyKind
	policy.APIVersion = models.CompliancePolicyAPIVersion
	now := time.Now()
	policy.CreatedAt = now
	policy.Generation = 1
	policy.Status.Phase = "Pending"

	if err := h.policyStore.Create(c.Request.Context(), &policy); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, policy)
}

func (h *GovernanceHandlers) UpdatePolicy(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	existing, err := h.policyStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "policy not found", Name: name})
		return
	}

	var updated models.CompliancePolicyResource
	if err := c.ShouldBindJSON(&updated); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	updated.ObjectMeta = existing.ObjectMeta
	updated.Generation = existing.Generation + 1
	updated.Status = existing.Status

	if err := h.policyStore.Update(c.Request.Context(), &updated); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "UpdatePolicy"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *GovernanceHandlers) DeletePolicy(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	if err := h.policyStore.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "policy not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: name})
}

func (h *GovernanceHandlers) TriggerAudit(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	policy, err := h.policyStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "policy not found", Name: name})
		return
	}

	policy.Generation++
	if err := h.policyStore.Update(c.Request.Context(), policy); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "TriggerAudit"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, MessageResponse{Message: "audit triggered", Name: name})
}

// --- Retention Policy Handlers ---

func (h *GovernanceHandlers) ListRetentionPolicies(c *gin.Context) {
	policies, err := h.retentionStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListRetentionPolicies"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, RetentionPolicyListResponse{RetentionPolicies: policies, Count: len(policies)})
}

func (h *GovernanceHandlers) GetRetentionPolicy(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	policy, err := h.retentionStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "retention policy not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, policy)
}

func (h *GovernanceHandlers) CreateRetentionPolicy(c *gin.Context) {
	var policy models.RetentionPolicyResource
	if err := c.ShouldBindJSON(&policy); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	policy.Kind = models.RetentionPolicyKind
	policy.APIVersion = models.RetentionPolicyAPIVersion
	now := time.Now()
	policy.CreatedAt = now
	policy.Generation = 1
	policy.Status.Phase = "Pending"

	if err := h.retentionStore.Create(c.Request.Context(), &policy); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, policy)
}

func (h *GovernanceHandlers) DeleteRetentionPolicy(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	if err := h.retentionStore.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "retention policy not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: name})
}

// --- Access Request Handlers ---

func (h *GovernanceHandlers) ListAccessRequests(c *gin.Context) {
	requests, err := h.accessStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListAccessRequests"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	statusFilter := c.Query("status")
	var filtered []*models.AccessRequestResource
	for _, req := range requests {
		if statusFilter != "" && req.Status.ApprovalStatus != statusFilter {
			continue
		}
		filtered = append(filtered, req)
	}

	c.JSON(http.StatusOK, AccessRequestListResponse{AccessRequests: filtered, Count: len(filtered)})
}

func (h *GovernanceHandlers) GetAccessRequest(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	req, err := h.accessStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "access request not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, req)
}

func (h *GovernanceHandlers) CreateAccessRequest(c *gin.Context) {
	var req models.AccessRequestResource
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	req.Kind = models.AccessRequestKind
	req.APIVersion = models.AccessRequestAPIVersion
	now := time.Now()
	req.CreatedAt = now
	req.Generation = 1
	req.Status.Phase = "Pending"
	req.Status.ApprovalStatus = "pending"

	if err := h.accessStore.Create(c.Request.Context(), &req); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, req)
}

func (h *GovernanceHandlers) ApproveAccessRequest(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	req, err := h.accessStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "access request not found", Name: name})
		return
	}

	var body struct {
		ApprovedBy string `json:"approvedBy"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	now := time.Now()
	req.Status.ApprovalStatus = "approved"
	req.Status.ApprovedBy = append(req.Status.ApprovedBy, body.ApprovedBy)
	req.Status.GrantedAt = &now
	req.Status.Phase = "Approved"
	req.Status.LastTransitionTime = now

	// Calculate expiry.
	if req.Spec.Duration != "" && req.Spec.Duration != "permanent" {
		if d, err := time.ParseDuration(req.Spec.Duration); err == nil {
			expires := now.Add(d)
			req.Status.ExpiresAt = &expires
		}
	}

	if err := h.accessStore.Update(c.Request.Context(), req); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ApproveAccessRequest"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "approved"})
}

func (h *GovernanceHandlers) DenyAccessRequest(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	req, err := h.accessStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "access request not found", Name: name})
		return
	}

	var body struct {
		DeniedBy string `json:"deniedBy"`
		Reason   string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	now := time.Now()
	req.Status.ApprovalStatus = "denied"
	req.Status.DeniedBy = body.DeniedBy
	req.Status.DenyReason = body.Reason
	req.Status.Phase = "Denied"
	req.Status.LastTransitionTime = now

	if err := h.accessStore.Update(c.Request.Context(), req); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "DenyAccessRequest"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "denied"})
}

func (h *GovernanceHandlers) RevokeAccessRequest(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	req, err := h.accessStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "access request not found", Name: name})
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	now := time.Now()
	req.Status.ApprovalStatus = "revoked"
	req.Status.RevokedAt = &now
	req.Status.RevokeReason = body.Reason
	req.Status.Phase = "Revoked"
	req.Status.LastTransitionTime = now

	if err := h.accessStore.Update(c.Request.Context(), req); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "RevokeAccessRequest"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "revoked"})
}

// --- Summary ---

func (h *GovernanceHandlers) GetSummary(c *gin.Context) {
	policies, _ := h.policyStore.List(c.Request.Context(), "")
	requests, _ := h.accessStore.List(c.Request.Context(), "")

	var totalPolicies, compliantPolicies, nonCompliantPolicies int
	var totalViolations int
	var avgScore float64

	for _, p := range policies {
		totalPolicies++
		if p.Status.Compliant {
			compliantPolicies++
		} else {
			nonCompliantPolicies++
		}
		totalViolations += len(p.Status.Violations)
		avgScore += p.Status.ComplianceScore
	}
	if totalPolicies > 0 {
		avgScore /= float64(totalPolicies)
	}

	var pendingRequests, approvedRequests int
	for _, r := range requests {
		switch r.Status.ApprovalStatus {
		case "pending":
			pendingRequests++
		case "approved":
			approvedRequests++
		}
	}

	c.JSON(http.StatusOK, GovernanceSummaryResponse{
		TotalPolicies:         totalPolicies,
		CompliantPolicies:     compliantPolicies,
		NonCompliantPolicies:  nonCompliantPolicies,
		TotalViolations:       totalViolations,
		AvgComplianceScore:    avgScore,
		PendingAccessRequests: pendingRequests,
		ActiveAccessGrants:    approvedRequests,
	})
}
