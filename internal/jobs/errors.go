package jobs

import "axiomnizam.bitbd.net/axiomnizam/internal/errors"

// Additional jobs-specific sentinel errors.
// Core errors (ErrJobNotFound, ErrQueueFull, etc.) are defined in job.go.
var (
	// ErrJobAlreadyComplete indicates the job is already in a terminal state.
	ErrJobAlreadyComplete = errors.WrapConflict("job", "", "already complete")

	// ErrScheduleConflict indicates a scheduling conflict.
	ErrScheduleConflict = errors.WrapConflict("schedule", "", "conflict with existing schedule")
)
