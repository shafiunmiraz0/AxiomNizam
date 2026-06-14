package mlpipeline

import (
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/validate"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type MLPipelineHandlers struct {
	pipelineStore   store.ResourceStore[*MLPipelineResource]
	deploymentStore store.ResourceStore[*ModelDeploymentResource]
}

func NewMLPipelineHandlers(
	pipelineStore store.ResourceStore[*MLPipelineResource],
	deploymentStore store.ResourceStore[*ModelDeploymentResource],
) *MLPipelineHandlers {
	return &MLPipelineHandlers{pipelineStore: pipelineStore, deploymentStore: deploymentStore}
}

func (h *MLPipelineHandlers) RegisterRoutes(rg *gin.RouterGroup) {
	ml := rg.Group("/ml")
	{
		// Pipelines
		ml.GET("/pipelines", h.ListPipelines)
		ml.GET("/pipelines/:name", h.GetPipeline)
		ml.POST("/pipelines", h.CreatePipeline)
		ml.PUT("/pipelines/:name", h.UpdatePipeline)
		ml.DELETE("/pipelines/:name", h.DeletePipeline)
		ml.POST("/pipelines/:name/run", h.TriggerRun)

		// Deployments
		ml.GET("/deployments", h.ListDeployments)
		ml.GET("/deployments/:name", h.GetDeployment)
		ml.POST("/deployments", h.CreateDeployment)
		ml.DELETE("/deployments/:name", h.DeleteDeployment)
	}
}

func (h *MLPipelineHandlers) ListPipelines(c *gin.Context) {
	pipelines, err := h.pipelineStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListPipelines"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"pipelines": pipelines, "count": len(pipelines)})
}

func (h *MLPipelineHandlers) GetPipeline(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	pipeline, err := h.pipelineStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "pipeline not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, pipeline)
}

func (h *MLPipelineHandlers) CreatePipeline(c *gin.Context) {
	var pipeline MLPipelineResource
	if err := c.ShouldBindJSON(&pipeline); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	pipeline.Kind = MLPipelineKind
	pipeline.APIVersion = MLPipelineAPIVersion
	now := time.Now()
	pipeline.CreatedAt = now
	pipeline.Generation = 1
	pipeline.Status.Phase = "Pending"
	pipeline.Status.PipelineStatus = "pending"
	if err := h.pipelineStore.Create(c.Request.Context(), &pipeline); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, pipeline)
}

func (h *MLPipelineHandlers) UpdatePipeline(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	existing, err := h.pipelineStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "pipeline not found", Name: name})
		return
	}
	var updated MLPipelineResource
	if err := c.ShouldBindJSON(&updated); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	updated.ObjectMeta = existing.ObjectMeta
	updated.Generation = existing.Generation + 1
	updated.Status = existing.Status
	if err := h.pipelineStore.Update(c.Request.Context(), &updated); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "UpdatePipeline"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *MLPipelineHandlers) DeletePipeline(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	if err := h.pipelineStore.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "pipeline not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: name})
}

func (h *MLPipelineHandlers) TriggerRun(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	pipeline, err := h.pipelineStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "pipeline not found", Name: name})
		return
	}
	pipeline.Generation++
	// Reset step statuses for a fresh run.
	pipeline.Status.StepStatuses = nil
	pipeline.Status.PipelineStatus = "pending"
	_ = h.pipelineStore.Update(c.Request.Context(), pipeline)
	c.JSON(http.StatusAccepted, MessageResponse{Message: "pipeline run triggered", Name: name})
}

// --- Deployment Handlers ---

func (h *MLPipelineHandlers) ListDeployments(c *gin.Context) {
	deployments, err := h.deploymentStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListDeployments"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deployments": deployments, "count": len(deployments)})
}

func (h *MLPipelineHandlers) GetDeployment(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	deployment, err := h.deploymentStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "deployment not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, deployment)
}

func (h *MLPipelineHandlers) CreateDeployment(c *gin.Context) {
	var deployment ModelDeploymentResource
	if err := c.ShouldBindJSON(&deployment); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}
	deployment.Kind = ModelDeploymentKind
	deployment.APIVersion = ModelDeploymentAPIVersion
	now := time.Now()
	deployment.CreatedAt = now
	deployment.Generation = 1
	deployment.Status.Phase = "Pending"
	deployment.Status.DeploymentStatus = "pending"
	if err := h.deploymentStore.Create(c.Request.Context(), &deployment); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, deployment)
}

func (h *MLPipelineHandlers) DeleteDeployment(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	if err := h.deploymentStore.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "deployment not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: name})
}
