package testutil

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/jobs"
)

// TestJobID is a fixed job ID for testing.
const TestJobID = "test-job-001"

// NewTestJob creates a test Job with sensible defaults.
func NewTestJob() *jobs.Job {
	return &jobs.Job{
		ID:         TestJobID,
		Type:       "test",
		Status:     jobs.JobStatusPending,
		Priority:   jobs.PriorityNormal,
		MaxRetries: 3,
		Timeout:    30 * time.Second,
		Data:       map[string]interface{}{"key": "value"},
		CreatedAt:  time.Now().UTC(),
	}
}

// NewTestJobWithType creates a test job with a specific type.
func NewTestJobWithType(jobType jobs.JobType) *jobs.Job {
	j := NewTestJob()
	j.Type = jobType
	return j
}

// NewTestJobRunning creates a test job in running state.
func NewTestJobRunning() *jobs.Job {
	j := NewTestJob()
	j.Status = jobs.JobStatusRunning
	j.StartedAt = time.Now().UTC()
	return j
}
