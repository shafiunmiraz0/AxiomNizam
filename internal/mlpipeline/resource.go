package mlpipeline

// Re-export domain types from models sub-package for backward compatibility.
import "axiomnizam.bitbd.net/axiomnizam/internal/mlpipeline/models"

// --- Constants ---
const (
	MLPipelineKind       = models.MLPipelineKind
	MLPipelineAPIVersion = models.MLPipelineAPIVersion

	ModelDeploymentKind       = models.ModelDeploymentKind
	ModelDeploymentAPIVersion = models.ModelDeploymentAPIVersion
)

// --- Type aliases for backward compatibility ---
type MLStep = models.MLStep
type MLPipelineSpec = models.MLPipelineSpec
type StepStatus = models.StepStatus
type MLPipelineResourceStatus = models.MLPipelineResourceStatus
type MLPipelineResource = models.MLPipelineResource
type AutoScaleConfig = models.AutoScaleConfig
type CanaryConfig = models.CanaryConfig
type ModelDeploymentSpec = models.ModelDeploymentSpec
type ModelDeploymentResourceStatus = models.ModelDeploymentResourceStatus
type ModelDeploymentResource = models.ModelDeploymentResource
