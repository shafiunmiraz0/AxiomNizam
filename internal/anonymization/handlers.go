package anonymization

import (
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/validate"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AnonymizationHandlers struct {
	store store.ResourceStore[*AnonymizationPolicyResource]
}

func NewAnonymizationHandlers(s store.ResourceStore[*AnonymizationPolicyResource]) *AnonymizationHandlers {
	return &AnonymizationHandlers{store: s}
}

func (h *AnonymizationHandlers) RegisterRoutes(rg *gin.RouterGroup) {
	anon := rg.Group("/anonymization")
	{
		anon.GET("/policies", h.ListPolicies)
		anon.GET("/policies/:name", h.GetPolicy)
		anon.POST("/policies", h.CreatePolicy)
		anon.PUT("/policies/:name", h.UpdatePolicy)
		anon.DELETE("/policies/:name", h.DeletePolicy)
		anon.POST("/policies/:name/run", h.TriggerRun)
	}
}

func (h *AnonymizationHandlers) ListPolicies(c *gin.Context) {
	policies, err := h.store.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListPolicies"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ListPoliciesResponse{Policies: policies, Count: len(policies)})
}

func (h *AnonymizationHandlers) GetPolicy(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	policy, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "policy not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, policy)
}

func (h *AnonymizationHandlers) CreatePolicy(c *gin.Context) {
	var policy AnonymizationPolicyResource
	if err := c.ShouldBindJSON(&policy); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	policy.Kind = AnonymizationPolicyKind
	policy.APIVersion = AnonymizationPolicyAPIVersion
	now := time.Now()
	policy.CreatedAt = now
	policy.Generation = 1
	policy.Status.Phase = "Pending"
	if err := h.store.Create(c.Request.Context(), &policy); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, policy)
}

func (h *AnonymizationHandlers) UpdatePolicy(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	existing, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "policy not found", Name: name})
		return
	}
	var updated AnonymizationPolicyResource
	if err := c.ShouldBindJSON(&updated); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	updated.ObjectMeta = existing.ObjectMeta
	updated.Generation = existing.Generation + 1
	updated.Status = existing.Status
	if err := h.store.Update(c.Request.Context(), &updated); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "UpdatePolicy"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *AnonymizationHandlers) DeletePolicy(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	if err := h.store.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "policy not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: name})
}

func (h *AnonymizationHandlers) TriggerRun(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	policy, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "policy not found", Name: name})
		return
	}
	policy.Generation++
	_ = h.store.Update(c.Request.Context(), policy)
	c.JSON(http.StatusAccepted, MessageResponse{Message: "anonymization run triggered", Policy: name})
}
