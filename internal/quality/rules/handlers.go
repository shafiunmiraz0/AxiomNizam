package rules

// =====================================================
// WS-2.1 — Quality Rules REST API Handlers
//
// Provides CRUD operations for quality rules and check results,
// plus manual trigger and score endpoints.
// =====================================================

import (
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/validate"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// QualityRulesHandlers provides REST API handlers for quality rules.
type QualityRulesHandlers struct {
	ruleStore  store.ResourceStore[*QualityRuleResource]
	checkStore store.ResourceStore[*QualityCheckResource]
	engine     *RuleEngine
}

// NewQualityRulesHandlers creates new handlers.
func NewQualityRulesHandlers(
	ruleStore store.ResourceStore[*QualityRuleResource],
	checkStore store.ResourceStore[*QualityCheckResource],
	engine *RuleEngine,
) *QualityRulesHandlers {
	return &QualityRulesHandlers{
		ruleStore:  ruleStore,
		checkStore: checkStore,
		engine:     engine,
	}
}

// RegisterRoutes registers quality rule API routes.
func (h *QualityRulesHandlers) RegisterRoutes(rg *gin.RouterGroup) {
	quality := rg.Group("/quality")
	{
		// Rules CRUD
		quality.GET("/rules", h.ListRules)
		quality.GET("/rules/:name", h.GetRule)
		quality.POST("/rules", h.CreateRule)
		quality.PUT("/rules/:name", h.UpdateRule)
		quality.DELETE("/rules/:name", h.DeleteRule)

		// Manual trigger
		quality.POST("/rules/:name/run", h.RunRule)

		// Check results
		quality.GET("/checks", h.ListChecks)
		quality.GET("/checks/:name", h.GetCheck)

		// Score
		quality.GET("/score/:asset", h.GetAssetScore)

		// Summary
		quality.GET("/summary", h.GetSummary)
	}
}

// ListRules returns all quality rules, optionally filtered.
func (h *QualityRulesHandlers) ListRules(c *gin.Context) {
	rules, err := h.ruleStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListRules"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	// Apply filters.
	assetFilter := c.Query("asset")
	severityFilter := c.Query("severity")
	statusFilter := c.Query("status")

	var filtered []*QualityRuleResource
	for _, rule := range rules {
		if assetFilter != "" && rule.Spec.AssetRef != assetFilter {
			continue
		}
		if severityFilter != "" && rule.Spec.Severity != severityFilter {
			continue
		}
		if statusFilter != "" {
			if statusFilter == "passing" && rule.Status.LastResult != CheckResultPass {
				continue
			}
			if statusFilter == "failing" && rule.Status.LastResult != CheckResultFail {
				continue
			}
		}
		filtered = append(filtered, rule)
	}

	c.JSON(http.StatusOK, RuleListResponse{
		Rules: filtered,
		Count: len(filtered),
	})
}

// GetRule returns a single quality rule by name.
func (h *QualityRulesHandlers) GetRule(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	rule, err := h.ruleStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "rule not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, rule)
}

// CreateRule creates a new quality rule.
func (h *QualityRulesHandlers) CreateRule(c *gin.Context) {
	var rule QualityRuleResource
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	// Set defaults.
	rule.Kind = QualityRuleKind
	rule.APIVersion = QualityRuleAPIVersion
	if rule.Spec.Severity == "" {
		rule.Spec.Severity = "warning"
	}
	if !rule.Spec.Enabled {
		rule.Spec.Enabled = true
	}
	now := time.Now()
	rule.CreatedAt = now
	rule.Generation = 1
	rule.Status.Phase = "Pending"

	if err := h.ruleStore.Create(c.Request.Context(), &rule); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, rule)
}

// UpdateRule updates an existing quality rule.
func (h *QualityRulesHandlers) UpdateRule(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	existing, err := h.ruleStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "rule not found", Name: name})
		return
	}

	var updated QualityRuleResource
	if err := c.ShouldBindJSON(&updated); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	// Preserve metadata.
	updated.ObjectMeta = existing.ObjectMeta
	updated.Generation = existing.Generation + 1
	updated.Status = existing.Status

	if err := h.ruleStore.Update(c.Request.Context(), &updated); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "UpdateRule"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteRule deletes a quality rule.
func (h *QualityRulesHandlers) DeleteRule(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	if err := h.ruleStore.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "rule not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: name})
}

// RunRule manually triggers a quality rule evaluation.
func (h *QualityRulesHandlers) RunRule(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	rule, err := h.ruleStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "rule not found", Name: name})
		return
	}

	if h.engine == nil {
		c.JSON(http.StatusServiceUnavailable, MessageResponse{Error: "quality engine not available"})
		return
	}

	output, err := h.engine.Evaluate(c.Request.Context(), rule)
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "RunRule"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, RunRuleResponse{
		Rule:   name,
		Result: output,
	})
}

// ListChecks returns historical check results.
func (h *QualityRulesHandlers) ListChecks(c *gin.Context) {
	checks, err := h.checkStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListChecks"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	// Filter by asset or rule.
	assetFilter := c.Query("asset")
	ruleFilter := c.Query("rule")

	var filtered []*QualityCheckResource
	for _, check := range checks {
		if assetFilter != "" && check.Spec.AssetRef != assetFilter {
			continue
		}
		if ruleFilter != "" && check.Spec.RuleRef != ruleFilter {
			continue
		}
		filtered = append(filtered, check)
	}

	c.JSON(http.StatusOK, CheckListResponse{
		Checks: filtered,
		Count:  len(filtered),
	})
}

// GetCheck returns a single check result.
func (h *QualityRulesHandlers) GetCheck(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	check, err := h.checkStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "check not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, check)
}

// GetAssetScore returns the quality score for a catalog asset.
func (h *QualityRulesHandlers) GetAssetScore(c *gin.Context) {
	asset := validate.PathParam(c, "asset")
	if asset == "" {
		return
	}

	rules, err := h.ruleStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "GetAssetScore"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	var totalRules, passingRules int
	var criticalFails, warningFails int

	for _, rule := range rules {
		if rule.Spec.AssetRef != asset {
			continue
		}
		totalRules++
		switch rule.Status.LastResult {
		case CheckResultPass:
			passingRules++
		case CheckResultFail:
			if rule.Spec.Severity == "critical" {
				criticalFails++
			} else {
				warningFails++
			}
		}
	}

	var score float64
	if totalRules > 0 {
		score = float64(passingRules) / float64(totalRules) * 100.0
	}

	c.JSON(http.StatusOK, AssetScoreResponse{
		Asset:         asset,
		Score:         score,
		TotalRules:    totalRules,
		PassingRules:  passingRules,
		CriticalFails: criticalFails,
		WarningFails:  warningFails,
	})
}

// GetSummary returns a platform-wide quality summary.
func (h *QualityRulesHandlers) GetSummary(c *gin.Context) {
	rules, err := h.ruleStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "GetSummary"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	var total, passing, failing, errors, pending int
	for _, rule := range rules {
		total++
		switch rule.Status.LastResult {
		case CheckResultPass:
			passing++
		case CheckResultFail:
			failing++
		case CheckResultError:
			errors++
		default:
			pending++
		}
	}

	var overallScore float64
	if total > 0 {
		overallScore = float64(passing) / float64(total) * 100.0
	}

	c.JSON(http.StatusOK, QualitySummaryResponse{
		TotalRules:   total,
		Passing:      passing,
		Failing:      failing,
		Errors:       errors,
		Pending:      pending,
		OverallScore: overallScore,
	})
}
