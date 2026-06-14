package jobs

// Store-backed Job handler (Phase 3/4).
//
// Thin-handler pattern — no etcd, no mutex, no in-memory map, no
// private cron scheduler.  All persistence and watch fan-out is
// delegated to `store.ResourceStore[*JobResource]`.  Dispatching,
// retries and scheduling live in the job controller/reconciler
// downstream of the workqueue.
//
// Routes (v2):
//
//   POST   /api/v2/jobs            -> Create
//   GET    /api/v2/jobs            -> List
//   GET    /api/v2/jobs/:name      -> Get
//   PUT    /api/v2/jobs/:name      -> Update (patch spec)
//   DELETE /api/v2/jobs/:name      -> Delete

import (
	"errors"
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// V1Handler is a thin Gin handler over a ResourceStore[*JobResource].
type V1Handler struct {
	store store.ResourceStore[*JobResource]
}

// NewV1Handler builds the handler.  `store` must be non-nil.
func NewV1Handler(s store.ResourceStore[*JobResource]) *V1Handler {
	return &V1Handler{store: s}
}

// RegisterRoutes attaches the v2 job routes onto the given router group.
func (h *V1Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/jobs", h.Create)
	rg.GET("/jobs", h.List)
	rg.GET("/jobs/:name", h.Get)
	rg.PUT("/jobs/:name", h.Update)
	rg.DELETE("/jobs/:name", h.Delete)
}

func (h *V1Handler) Create(c *gin.Context) {
	var in JobResource
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	if in.Name == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "metadata.name is required"})
		return
	}

	in.TypeMeta = resources.TypeMeta{
		Kind:       JobKind,
		APIVersion: JobAPIVersion,
	}
	if in.UID == "" {
		in.UID = uuid.New().String()
	}
	now := time.Now()
	in.CreatedAt = now
	in.UpdatedAt = now
	in.Generation = 1
	in.Status.Phase = "Pending"
	in.Status.LastTransitionTime = now

	if err := h.store.Create(c.Request.Context(), &in); err != nil {
		if errors.Is(err, store.ErrConflict) {
			c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, &in)
}

func (h *V1Handler) List(c *gin.Context) {
	items, err := h.store.List(c.Request.Context(), "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *V1Handler) Get(c *gin.Context) {
	j, err := h.store.Get(c.Request.Context(), c.Param("name"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, j)
}

func (h *V1Handler) Update(c *gin.Context) {
	name := c.Param("name")
	existing, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	var patch JobResource
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	existing.Spec = patch.Spec
	existing.UpdatedAt = time.Now()
	existing.Generation++

	if err := h.store.Update(c.Request.Context(), existing); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func (h *V1Handler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.store.Delete(c.Request.Context(), name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "job '" + name + "' deleted"})
}
