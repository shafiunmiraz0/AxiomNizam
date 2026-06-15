package repositories

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/jobs"
)

// JobRepository defines persistence operations for background jobs.
// Implemented by JobManagerImpl.
type JobRepository interface {
	// Submit adds a new job to the queue.
	Submit(ctx context.Context, job *jobs.Job) error

	// GetJob retrieves a job by ID.
	GetJob(ctx context.Context, jobID string) (*jobs.Job, error)
}
