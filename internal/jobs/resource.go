package jobs

// Re-export domain types from models sub-package for backward compatibility.
import (
	"axiomnizam.bitbd.net/axiomnizam/internal/jobs/models"
)

// --- Constants ---
const (
	JobKind       = models.JobKind
	JobAPIVersion = models.JobAPIVersion
)

// --- Type aliases for backward compatibility ---

type JobStatus = models.JobStatus
type JobPriority = models.JobPriority
type JobType = models.JobType
type JobSpec = models.JobSpec
type JobResourceStatus = models.JobResourceStatus
type JobResource = models.JobResource

// --- JobStatus constants re-exported for backward compatibility ---
const (
	JobStatusPending   = models.JobStatusPending
	JobStatusRunning   = models.JobStatusRunning
	JobStatusCompleted = models.JobStatusCompleted
	JobStatusFailed    = models.JobStatusFailed
	JobStatusCancelled = models.JobStatusCancelled
	JobStatusRetrying  = models.JobStatusRetrying
)

// --- JobPriority constants re-exported for backward compatibility ---
const (
	PriorityLow      = models.PriorityLow
	PriorityNormal   = models.PriorityNormal
	PriorityHigh     = models.PriorityHigh
	PriorityCritical = models.PriorityCritical
)

// --- JobType constants re-exported for backward compatibility ---
const (
	JobTypeEmail           = models.JobTypeEmail
	JobTypeReport          = models.JobTypeReport
	JobTypeDataCleanup     = models.JobTypeDataCleanup
	JobTypeDataMigration   = models.JobTypeDataMigration
	JobTypeNotification    = models.JobTypeNotification
	JobTypeWebhook         = models.JobTypeWebhook
	JobTypeImageProcessing = models.JobTypeImageProcessing
	JobTypeBackup          = models.JobTypeBackup
	JobTypeExport          = models.JobTypeExport
	JobTypeImport          = models.JobTypeImport
)

// ToJob converts the declarative resource into the imperative `*Job`
// shape accepted by `JobManager.Submit`.
func ToJob(j *JobResource) *Job {
	id := j.UID
	if id == "" {
		id = j.Name
	}
	return &Job{
		ID:          id,
		Type:        j.Spec.Type,
		Status:      JobStatusPending,
		Priority:    j.Spec.Priority,
		Data:        j.Spec.Data,
		MaxRetries:  j.Spec.MaxRetries,
		CreatedAt:   j.CreatedAt,
		Timeout:     j.Spec.Timeout,
		Tags:        j.Spec.Tags,
		CallbackURL: j.Spec.CallbackURL,
		DeadlineAt:  j.Spec.DeadlineAt,
	}
}
