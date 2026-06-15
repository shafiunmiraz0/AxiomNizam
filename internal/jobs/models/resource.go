package models

// =====================================================
// Job domain types and declarative resource
//
// Contains the job resource envelope (Spec / Status / Conditions)
// plus the supporting domain types (JobStatus, JobPriority, JobType)
// that the resource structs reference.
//
// The parent jobs package re-exports all types via aliases so existing
// code continues to compile unchanged.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// --- Domain types used by the resource structs ---

// JobStatus represents the status of a job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
	JobStatusRetrying  JobStatus = "retrying"
)

// JobPriority represents job priority level.
type JobPriority int

const (
	PriorityLow      JobPriority = 1
	PriorityNormal   JobPriority = 5
	PriorityHigh     JobPriority = 10
	PriorityCritical JobPriority = 20
)

// JobType represents the type of job.
type JobType string

const (
	JobTypeEmail           JobType = "email"
	JobTypeReport          JobType = "report"
	JobTypeDataCleanup     JobType = "data_cleanup"
	JobTypeDataMigration   JobType = "data_migration"
	JobTypeNotification    JobType = "notification"
	JobTypeWebhook         JobType = "webhook"
	JobTypeImageProcessing JobType = "image_processing"
	JobTypeBackup          JobType = "backup"
	JobTypeExport          JobType = "export"
	JobTypeImport          JobType = "import"
)

// --- Resource constants ---

const (
	JobKind       = "Job"
	JobAPIVersion = "jobs.axiomnizam.io/v1"
)

// --- Resource types ---

// JobSpec is the *desired* state of a background job.
type JobSpec struct {
	Type        JobType                `json:"type"`
	Priority    JobPriority            `json:"priority"`
	Data        map[string]interface{} `json:"data"`
	MaxRetries  int                    `json:"maxRetries"`
	Timeout     time.Duration          `json:"timeout"`
	Tags        []string               `json:"tags,omitempty"`
	CallbackURL string                 `json:"callbackUrl,omitempty"`
	DeadlineAt  time.Time              `json:"deadlineAt,omitempty"`

	// Schedule, if non-empty, asks the controller to register this job
	// with a cron-style scheduler instead of submitting it once.
	Schedule string `json:"schedule,omitempty"`

	// Suspend, when true, prevents the controller from dispatching new
	// runs of a scheduled job.
	Suspend bool `json:"suspend,omitempty"`
}

// JobResourceStatus extends the canonical ObjectStatus with job-specific
// lifecycle telemetry.
type JobResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	JobStatus   JobStatus              `json:"jobStatus,omitempty"`
	Retries     int                    `json:"retries"`
	Error       string                 `json:"error,omitempty"`
	Result      map[string]interface{} `json:"result,omitempty"`
	StartedAt   time.Time              `json:"startedAt,omitempty"`
	CompletedAt time.Time              `json:"completedAt,omitempty"`
}

// JobResource is the declarative resource for a background job.
type JobResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   JobSpec           `json:"spec"`
	Status JobResourceStatus `json:"status"`
}

// --- resources.Resource ---

func (j *JobResource) GetObjectMeta() *resources.ObjectMeta { return &j.ObjectMeta }
func (j *JobResource) GetTypeMeta() *resources.TypeMeta     { return &j.TypeMeta }
func (j *JobResource) GetStatus() *resources.ObjectStatus   { return &j.Status.ObjectStatus }
func (j *JobResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		j.Status.ObjectStatus = *s
	}
}
func (j *JobResource) DeepCopy() resources.Resource {
	cp := *j
	return &cp
}

// --- reconciler.Resource ---

func (j *JobResource) GetKey() string {
	if j.Namespace == "" {
		return j.Name
	}
	return j.Namespace + "/" + j.Name
}
func (j *JobResource) GetGeneration() int64         { return j.Generation }
func (j *JobResource) GetObservedGeneration() int64 { return j.Status.ObservedGeneration }
