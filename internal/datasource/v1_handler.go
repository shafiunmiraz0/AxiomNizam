package datasourceresource

// Store-backed DataSource handler (Phase 3/4 reference implementation).
//
// Thin-handler pattern — no etcd, no mutex, no in-memory map.  All
// persistence and watch fan-out is delegated to
// `store.ResourceStore[*DataSourceV1Resource]`.
//
// Routes:
//
//   POST   /api/v2/datasources            -> Create
//   GET    /api/v2/datasources            -> List
//   GET    /api/v2/datasources/:name      -> Get
//   PUT    /api/v2/datasources/:name      -> Update
//   DELETE /api/v2/datasources/:name      -> Delete

import (
	"errors"
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// V1Handler is a thin Gin handler over a ResourceStore[*DataSourceV1Resource].
type V1Handler struct {
	store store.ResourceStore[*DataSourceV1Resource]
}

// NewV1Handler builds the handler.  `store` must be non-nil.
func NewV1Handler(s store.ResourceStore[*DataSourceV1Resource]) *V1Handler {
	return &V1Handler{store: s}
}

// RegisterRoutes attaches the v2 datasource routes onto the given router group.
func (h *V1Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/datasources", h.Create)
	rg.GET("/datasources", h.List)
	rg.GET("/datasources/:name", h.Get)
	rg.PUT("/datasources/:name", h.Update)
	rg.DELETE("/datasources/:name", h.Delete)
}

func (h *V1Handler) Create(c *gin.Context) {
	var in DataSourceV1Resource
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	if in.Name == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "metadata.name is required"})
		return
	}

	in.TypeMeta = resources.TypeMeta{
		Kind:       DataSourceKind,
		APIVersion: DataSourceAPIVersion,
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
	ds, err := h.store.Get(c.Request.Context(), c.Param("name"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, MessageResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ds)
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

	var patch DataSourceV1Resource
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
	c.JSON(http.StatusOK, MessageResponse{Message: "datasource '" + name + "' deleted"})
}
