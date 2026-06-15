package cdc

// Re-export domain types from models sub-package for backward compatibility.
import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/cdc/models"
)

// --- Constants ---
const (
	CDCPipelineKind       = models.CDCPipelineKind
	CDCPipelineAPIVersion = models.CDCPipelineAPIVersion
)

// --- Type aliases for backward compatibility ---

type PipelineStatus = models.PipelineStatus
type CDCSource = models.CDCSource
type CDCSink = models.CDCSink
type CDCFilters = models.CDCFilters
type CDCPipelineSpec = models.CDCPipelineSpec
type CDCPipelineResourceStatus = models.CDCPipelineResourceStatus
type CDCPipelineResource = models.CDCPipelineResource

// --- PipelineStatus constants re-exported for backward compatibility ---
const (
	CDCActive  = models.CDCActive
	CDCPaused  = models.CDCPaused
	CDCStopped = models.CDCStopped
	CDCFailed  = models.CDCFailed
	CDCCreated = models.CDCCreated
)

// ToCDCPipeline projects the declarative resource onto the imperative
// `*CDCPipeline` shape consumed by `PipelineEngine`.
func ToCDCPipeline(c *CDCPipelineResource) *CDCPipeline {
	id := c.UID
	if id == "" {
		id = c.Name
	}
	status := CDCCreated
	if c.Spec.Paused {
		status = CDCPaused
	}
	if c.Status.CDCStatus != "" {
		status = c.Status.CDCStatus
	}
	return &CDCPipeline{
		ID:          id,
		Name:        c.Name,
		Description: c.Spec.Description,
		Source:      c.Spec.Source,
		Sink:        c.Spec.Sink,
		Filters:     c.Spec.Filters,
		Status:      status,
		Config:      c.Spec.Config,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   time.Now(),
		EventCount:  c.Status.EventCount,
		ErrorCount:  c.Status.ErrorCount,
		LastEventAt: c.Status.LastEventAt,
		Lag:         c.Status.Lag,
		Tags:        c.Spec.Tags,
	}
}
