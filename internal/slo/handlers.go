package slo

// =====================================================
// WS-4.3 — SLO REST API Handlers
//
// Provides CRUD for SLOs plus status, budget, and summary endpoints.
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

// SLOHandlers provides REST API handlers for SLOs.
type SLOHandlers struct {
	store store.ResourceStore[*SLOResource]
}

// NewSLOHandlers creates new handlers.
func NewSLOHandlers(s store.ResourceStore[*SLOResource]) *SLOHandlers {
	return &SLOHandlers{store: s}
}

// RegisterRoutes registers SLO API routes.
func (h *SLOHandlers) RegisterRoutes(rg *gin.RouterGroup) {
	slos := rg.Group("/slos")
	{
		slos.GET("", h.ListSLOs)
		slos.GET("/:name", h.GetSLO)
		slos.POST("", h.CreateSLO)
		slos.PUT("/:name", h.UpdateSLO)
		slos.DELETE("/:name", h.DeleteSLO)
		slos.GET("/:name/budget", h.GetBudget)
		slos.GET("/status", h.GetAllStatus)
	}
}

// ListSLOs returns all SLOs.
func (h *SLOHandlers) ListSLOs(c *gin.Context) {
	slos, err := h.store.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListSLOs"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	serviceFilter := c.Query("service")
	var filtered []*SLOResource
	for _, s := range slos {
		if serviceFilter != "" && s.Spec.Service != serviceFilter {
			continue
		}
		filtered = append(filtered, s)
	}

	c.JSON(http.StatusOK, SLOListResponse{SLOs: filtered, Count: len(filtered)})
}

// GetSLO returns a single SLO.
func (h *SLOHandlers) GetSLO(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	s, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "SLO not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, s)
}

// CreateSLO creates a new SLO.
func (h *SLOHandlers) CreateSLO(c *gin.Context) {
	var s SLOResource
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	s.Kind = SLOKind
	s.APIVersion = SLOAPIVersion
	now := time.Now()
	s.CreatedAt = now
	s.Generation = 1
	s.Status.Phase = "Pending"

	if err := h.store.Create(c.Request.Context(), &s); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, s)
}

// UpdateSLO updates an existing SLO.
func (h *SLOHandlers) UpdateSLO(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	existing, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "SLO not found", Name: name})
		return
	}

	var updated SLOResource
	if err := c.ShouldBindJSON(&updated); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	updated.ObjectMeta = existing.ObjectMeta
	updated.Generation = existing.Generation + 1
	updated.Status = existing.Status

	if err := h.store.Update(c.Request.Context(), &updated); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "UpdateSLO"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteSLO deletes an SLO.
func (h *SLOHandlers) DeleteSLO(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	if err := h.store.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "SLO not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: name})
}

// GetBudget returns the error budget details for an SLO.
func (h *SLOHandlers) GetBudget(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	s, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "SLO not found", Name: name})
		return
	}

	c.JSON(http.StatusOK, BudgetResponse{
		Name:           name,
		Target:         s.Spec.Target,
		CurrentSLI:     s.Status.CurrentSLI,
		ErrorBudget:    s.Status.ErrorBudget,
		BudgetConsumed: s.Status.BudgetConsumed,
		BurnRate:       s.Status.BurnRate,
		IsBreaching:    s.Status.IsBreaching,
		TimeToExhaust:  s.Status.TimeToExhaust,
		GoodEvents:     s.Status.GoodEvents,
		TotalEvents:    s.Status.TotalEvents,
		Window:         s.Spec.Window,
	})
}

// GetAllStatus returns a summary of all SLO statuses.
func (h *SLOHandlers) GetAllStatus(c *gin.Context) {
	slos, err := h.store.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "GetAllStatus"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	var healthy, atRisk, breaching int
	for _, s := range slos {
		switch s.Status.Phase {
		case "Healthy":
			healthy++
		case "AtRisk":
			atRisk++
		case "Breaching":
			breaching++
		}
	}

	c.JSON(http.StatusOK, AllStatusResponse{
		Total:     len(slos),
		Healthy:   healthy,
		AtRisk:    atRisk,
		Breaching: breaching,
	})
}
