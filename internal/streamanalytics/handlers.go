package streamanalytics

import (
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/validate"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type StreamAnalyticsHandlers struct {
	store store.ResourceStore[*StreamJobResource]
}

func NewStreamAnalyticsHandlers(s store.ResourceStore[*StreamJobResource]) *StreamAnalyticsHandlers {
	return &StreamAnalyticsHandlers{store: s}
}

func (h *StreamAnalyticsHandlers) RegisterRoutes(rg *gin.RouterGroup) {
	sa := rg.Group("/stream-analytics")
	{
		sa.GET("/jobs", h.ListJobs)
		sa.GET("/jobs/:name", h.GetJob)
		sa.POST("/jobs", h.CreateJob)
		sa.PUT("/jobs/:name", h.UpdateJob)
		sa.DELETE("/jobs/:name", h.DeleteJob)
		sa.POST("/jobs/:name/start", h.StartJob)
		sa.POST("/jobs/:name/stop", h.StopJob)
	}
}

func (h *StreamAnalyticsHandlers) ListJobs(c *gin.Context) {
	jobs, err := h.store.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListJobs"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ListJobsResponse{StreamJobs: jobs, Count: len(jobs)})
}

func (h *StreamAnalyticsHandlers) GetJob(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	job, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "stream job not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *StreamAnalyticsHandlers) CreateJob(c *gin.Context) {
	var job StreamJobResource
	if err := c.ShouldBindJSON(&job); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	job.Kind = StreamJobKind
	job.APIVersion = StreamJobAPIVersion
	now := time.Now()
	job.CreatedAt = now
	job.Generation = 1
	job.Status.Phase = "Pending"
	job.Status.JobStatus = "pending"
	if err := h.store.Create(c.Request.Context(), &job); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, job)
}

func (h *StreamAnalyticsHandlers) UpdateJob(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	existing, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "stream job not found", Name: name})
		return
	}
	var updated StreamJobResource
	if err := c.ShouldBindJSON(&updated); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	updated.ObjectMeta = existing.ObjectMeta
	updated.Generation = existing.Generation + 1
	updated.Status = existing.Status
	if err := h.store.Update(c.Request.Context(), &updated); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "UpdateJob"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *StreamAnalyticsHandlers) DeleteJob(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	if err := h.store.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "stream job not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: name})
}

func (h *StreamAnalyticsHandlers) StartJob(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	job, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "stream job not found", Name: name})
		return
	}
	job.Spec.Enabled = true
	job.Generation++
	_ = h.store.Update(c.Request.Context(), job)
	c.JSON(http.StatusAccepted, MessageResponse{Message: "job start triggered", Name: name})
}

func (h *StreamAnalyticsHandlers) StopJob(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	job, err := h.store.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "stream job not found", Name: name})
		return
	}
	job.Spec.Enabled = false
	job.Generation++
	_ = h.store.Update(c.Request.Context(), job)
	c.JSON(http.StatusAccepted, MessageResponse{Message: "job stop triggered", Name: name})
}
