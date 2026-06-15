package users

// Store-backed Platform User handler (Phase 3/4).
//
// Thin-handler pattern — no etcd, no mutex, no in-memory map.  All
// persistence and watch fan-out is delegated to
// `store.ResourceStore[*UserResource]`; reconciliation runs on
// the shared workqueue via `UserReconciler`.
//
// Routes (v2):
//
//   POST   /api/v2/platform/users            -> Create
//   GET    /api/v2/platform/users            -> List
//   GET    /api/v2/platform/users/:name      -> Get
//   PUT    /api/v2/platform/users/:name      -> Update
//   DELETE /api/v2/platform/users/:name      -> Delete

import (
	"errors"
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// V1Handler is a thin Gin handler over a ResourceStore[*UserResource].
type V1Handler struct {
	store store.ResourceStore[*UserResource]
}

// NewV1Handler builds the handler.  `store` must be non-nil.
func NewV1Handler(s store.ResourceStore[*UserResource]) *V1Handler {
	return &V1Handler{store: s}
}

// RegisterRoutes attaches the v2 user routes onto the given router group.
// The group is expected to already carry platform-admin auth middleware.
func (h *V1Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/platform/users", h.Create)
	rg.GET("/platform/users", h.List)
	rg.GET("/platform/users/:name", h.Get)
	rg.PUT("/platform/users/:name", h.Update)
	rg.DELETE("/platform/users/:name", h.Delete)
}

// createUserV1Body is the JSON body accepted by Create.
type createUserV1Body struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role" binding:"required"`
}

func (h *V1Handler) Create(c *gin.Context) {
	var in createUserV1Body
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	now := time.Now()
	u := &UserResource{
		TypeMeta: resources.TypeMeta{
			Kind:       UserKind,
			APIVersion: UserAPIVersion,
		},
		ObjectMeta: resources.ObjectMeta{
			Name:       in.Username,
			UID:        uuid.New().String(),
			Generation: 1,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		Spec: UserSpec{
			Username:     in.Username,
			Email:        in.Email,
			PasswordHash: string(hashed),
			Role:         in.Role,
		},
	}
	u.Status.Phase = "Pending"
	u.Status.LastTransitionTime = now

	if err := h.store.Create(c.Request.Context(), u); err != nil {
		if errors.Is(err, store.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, u)
}

func (h *V1Handler) List(c *gin.Context) {
	items, err := h.store.List(c.Request.Context(), "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *V1Handler) Get(c *gin.Context) {
	u, err := h.store.Get(c.Request.Context(), c.Param("name"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, u)
}

// updateUserV1Body is the JSON body accepted by Update.
type updateUserV1Body struct {
	Email     string `json:"email,omitempty"`
	Role      string `json:"role,omitempty"`
	Password  string `json:"password,omitempty"`
	Suspended *bool  `json:"suspended,omitempty"`
}

func (h *V1Handler) Update(c *gin.Context) {
	name := c.Param("name")
	existing, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var patch updateUserV1Body
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if patch.Email != "" {
		existing.Spec.Email = patch.Email
	}
	if patch.Role != "" {
		existing.Spec.Role = patch.Role
	}
	if patch.Suspended != nil {
		existing.Spec.Suspended = *patch.Suspended
	}
	if patch.Password != "" {
		hashed, err := bcrypt.GenerateFromPassword([]byte(patch.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}
		existing.Spec.PasswordHash = string(hashed)
	}
	existing.UpdatedAt = time.Now()
	existing.Generation++

	if err := h.store.Update(c.Request.Context(), existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func (h *V1Handler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.store.Delete(c.Request.Context(), name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "user '" + name + "' deleted"})
}
