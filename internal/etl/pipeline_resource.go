package etl

// Re-export domain types from models sub-package for backward compatibility.
import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/etl/models"
)

// --- Constants ---
const (
	PipelineKind       = models.PipelineKind
	PipelineAPIVersion = models.PipelineAPIVersion
)

// --- Type aliases for backward compatibility ---

type PipelineStatus = models.PipelineStatus
type StepType = models.StepType
type Step = models.Step
type OrchestrationConfig = models.OrchestrationConfig
type PipelineSpec = models.PipelineSpec
type PipelineResourceStatus = models.PipelineResourceStatus
type PipelineResource = models.PipelineResource

// --- PipelineStatus constants re-exported for backward compatibility ---
const (
	PipelineCreated = models.PipelineCreated
	PipelineRunning = models.PipelineRunning
	PipelinePaused  = models.PipelinePaused
	PipelineSuccess = models.PipelineSuccess
	PipelineFailed  = models.PipelineFailed
	PipelineStopped = models.PipelineStopped
)

// --- StepType constants re-exported for backward compatibility ---
const (
	StepExtract   = models.StepExtract
	StepTransform = models.StepTransform
	StepLoad      = models.StepLoad
	StepFilter    = models.StepFilter
	StepMap       = models.StepMap
	StepAggregate = models.StepAggregate
	StepJoin      = models.StepJoin
	StepValidate  = models.StepValidate
	StepEnrich    = models.StepEnrich
	StepDedupe    = models.StepDedupe
)

// ToPipeline converts the declarative resource into the imperative
// `*Pipeline` shape consumed by `Engine`.  The returned Pipeline reuses
// the resource UID as its ID so the two stay linked.
func ToPipeline(p *PipelineResource) *Pipeline {
	now := time.Now()
	id := p.UID
	if id == "" {
		id = p.Name
	}
	status := PipelineCreated
	if p.Spec.Paused {
		status = PipelinePaused
	}
	if p.Status.PipelineStatus != "" {
		status = p.Status.PipelineStatus
	}
	return &Pipeline{
		ID:            id,
		Name:          p.Name,
		Description:   p.Spec.Description,
		Steps:         append([]Step(nil), p.Spec.Steps...),
		Schedule:      p.Spec.Schedule,
		Orchestration: p.Spec.Orchestration,
		Config:        p.Spec.Config,
		Status:        status,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     now,
		LastRunAt:     p.Status.LastRunAt,
		RunCount:      p.Status.RunCount,
		Tags:          p.Spec.Tags,
	}
}
