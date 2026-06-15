package costing

// =====================================================
// WS-4.4 — Cost Attribution REST API Handlers
//
// Provides usage reports, quota management, and cost policy CRUD.
// =====================================================

import (
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/costing/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/validate"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CostHandlers provides REST API handlers for cost attribution.
type CostHandlers struct {
	policyStore store.ResourceStore[*models.CostPolicyResource]
	usageStore  store.ResourceStore[*models.UsageRecordResource]
}

// NewCostHandlers creates new handlers.
func NewCostHandlers(
	policyStore store.ResourceStore[*models.CostPolicyResource],
	usageStore store.ResourceStore[*models.UsageRecordResource],
) *CostHandlers {
	return &CostHandlers{
		policyStore: policyStore,
		usageStore:  usageStore,
	}
}

// RegisterRoutes registers cost API routes.
func (h *CostHandlers) RegisterRoutes(rg *gin.RouterGroup) {
	costs := rg.Group("/costs")
	{
		costs.GET("/policies", h.ListPolicies)
		costs.GET("/policies/:name", h.GetPolicy)
		costs.POST("/policies", h.CreatePolicy)
		costs.PUT("/policies/:name", h.UpdatePolicy)
		costs.DELETE("/policies/:name", h.DeletePolicy)
		costs.GET("/usage", h.GetUsage)
		costs.GET("/usage/:tenant", h.GetTenantUsage)
		costs.GET("/report", h.GetReport)
	}
}

// ListPolicies returns all cost policies.
func (h *CostHandlers) ListPolicies(c *gin.Context) {
	policies, err := h.policyStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListPolicies"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"policies": policies, "count": len(policies)})
}

// GetPolicy returns a single cost policy.
func (h *CostHandlers) GetPolicy(c *gin.Context) {
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

// CreatePolicy creates a new cost policy.
func (h *CostHandlers) CreatePolicy(c *gin.Context) {
	var policy models.CostPolicyResource
	if err := c.ShouldBindJSON(&policy); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	policy.Kind = models.CostPolicyKind
	policy.APIVersion = models.CostPolicyAPIVersion
	now := time.Now()
	policy.CreatedAt = now
	policy.Generation = 1
	policy.Status.Phase = "Active"

	if err := h.policyStore.Create(c.Request.Context(), &policy); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, policy)
}

// UpdatePolicy updates an existing cost policy.
func (h *CostHandlers) UpdatePolicy(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	existing, err := h.policyStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "policy not found", Name: name})
		return
	}

	var updated models.CostPolicyResource
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

// DeletePolicy deletes a cost policy.
func (h *CostHandlers) DeletePolicy(c *gin.Context) {
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

// GetUsage returns aggregated usage across all tenants.
func (h *CostHandlers) GetUsage(c *gin.Context) {
	records, err := h.usageStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "GetUsage"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	byDimension := make(map[string]float64)
	byTenant := make(map[string]float64)
	var totalCredits float64

	for _, r := range records {
		byDimension[string(r.Spec.Dimension)] += r.Spec.Credits
		byTenant[r.Spec.TenantID] += r.Spec.Credits
		totalCredits += r.Spec.Credits
	}

	c.JSON(http.StatusOK, gin.H{
		"totalCredits":  totalCredits,
		"totalRecords":  len(records),
		"byDimension":   byDimension,
		"byTenant":      byTenant,
	})
}

// GetTenantUsage returns usage for a specific tenant.
func (h *CostHandlers) GetTenantUsage(c *gin.Context) {
	tenant := validate.PathParam(c, "tenant")
	if tenant == "" {
		return
	}

	records, err := h.usageStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "GetTenantUsage"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	byDimension := make(map[string]float64)
	var totalCredits float64
	var recordCount int

	for _, r := range records {
		if r.Spec.TenantID != tenant {
			continue
		}
		byDimension[string(r.Spec.Dimension)] += r.Spec.Credits
		totalCredits += r.Spec.Credits
		recordCount++
	}

	c.JSON(http.StatusOK, gin.H{
		"tenant":       tenant,
		"totalCredits": totalCredits,
		"records":      recordCount,
		"byDimension":  byDimension,
	})
}

// GetReport generates a cost report.
func (h *CostHandlers) GetReport(c *gin.Context) {
	policies, _ := h.policyStore.List(c.Request.Context(), "")

	type tenantReport struct {
		TenantID       string             `json:"tenantId"`
		TotalUsed      float64            `json:"totalUsed"`
		TotalLimit     float64            `json:"totalLimit"`
		UsagePercent   float64            `json:"usagePercent"`
		OverQuota      bool               `json:"overQuota"`
		ByDimension    map[string]float64 `json:"byDimension"`
	}

	var reports []tenantReport
	for _, p := range policies {
		report := tenantReport{
			TenantID:    p.Spec.TenantID,
			TotalUsed:   p.Status.TotalCreditsUsed,
			TotalLimit:  p.Status.TotalCreditsLimit,
			OverQuota:   len(p.Status.QuotaBreaches) > 0,
			ByDimension: p.Status.UsageByDimension,
		}
		if report.TotalLimit > 0 {
			report.UsagePercent = report.TotalUsed / report.TotalLimit * 100
		}
		reports = append(reports, report)
	}

	c.JSON(http.StatusOK, gin.H{
		"report":  reports,
		"tenants": len(reports),
	})
}
