package featurestore

import (
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/validate"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type FeatureStoreHandlers struct {
	store store.ResourceStore[*FeatureGroupResource]
}

func NewFeatureStoreHandlers(s store.ResourceStore[*FeatureGroupResource]) *FeatureStoreHandlers {
	return &FeatureStoreHandlers{store: s}
}

func (h *FeatureStoreHandlers) RegisterRoutes(rg *gin.RouterGroup) {
	fs := rg.Group("/features")
	{
		fs.GET("/groups", h.ListGroups)
		fs.GET("/groups/:name", h.GetGroup)
		fs.POST("/groups", h.CreateGroup)
		fs.PUT("/groups/:name", h.UpdateGroup)
		fs.DELETE("/groups/:name", h.DeleteGroup)
		fs.POST("/groups/:name/materialize", h.TriggerMaterialize)
		fs.POST("/online", h.ServeOnline)
	}
}

func (h *FeatureStoreHandlers) ListGroups(c *gin.Context) {
	groups, err := h.store.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListGroups"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, FeatureGroupListResponse{FeatureGroups: groups, Count: len(groups)})
}

func (h *FeatureStoreHandlers) GetGroup(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	group, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "feature group not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, group)
}

func (h *FeatureStoreHandlers) CreateGroup(c *gin.Context) {
	var group FeatureGroupResource
	if err := c.ShouldBindJSON(&group); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	group.Kind = FeatureGroupKind
	group.APIVersion = FeatureGroupAPIVersion
	now := time.Now()
	group.CreatedAt = now
	group.Generation = 1
	group.Status.Phase = "Pending"
	if err := h.store.Create(c.Request.Context(), &group); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, group)
}

func (h *FeatureStoreHandlers) UpdateGroup(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	existing, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "feature group not found", Name: name})
		return
	}
	var updated FeatureGroupResource
	if err := c.ShouldBindJSON(&updated); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	updated.ObjectMeta = existing.ObjectMeta
	updated.Generation = existing.Generation + 1
	updated.Status = existing.Status
	if err := h.store.Update(c.Request.Context(), &updated); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "UpdateGroup"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *FeatureStoreHandlers) DeleteGroup(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	if err := h.store.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "feature group not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: name})
}

func (h *FeatureStoreHandlers) TriggerMaterialize(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	group, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "feature group not found", Name: name})
		return
	}
	group.Generation++
	_ = h.store.Update(c.Request.Context(), group)
	c.JSON(http.StatusAccepted, MessageResponse{Message: "materialization triggered", Name: name})
}

// ServeOnline handles online feature serving requests.
func (h *FeatureStoreHandlers) ServeOnline(c *gin.Context) {
	var req struct {
		FeatureGroup string              `json:"featureGroup"`
		Entities     []map[string]string `json:"entities"`
		Features     []string            `json:"features,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	group, err := h.store.Get(c.Request.Context(), req.FeatureGroup)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "feature group not found"})
		return
	}

	// In production, this would look up features from the online store.
	c.JSON(http.StatusOK, OnlineServingResponse{
		FeatureGroup: req.FeatureGroup,
		Entities:     len(req.Entities),
		Features:     group.Status.FeatureCount,
		Freshness:    group.Status.FreshnessStatus,
		Message:      "online serving placeholder — connect to online store backend",
	})
}
