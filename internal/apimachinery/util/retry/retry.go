// Package retry implements the "retry on conflict" pattern that k8s
// controllers use when two writers race on the same resource version.
// The retry budget is expressed as a wait.Backoff so callers choose
// between tight inner loops (no backoff) and outer RPC layers
// (exponential).
//
// Typical usage in a reconciler:
//
//	err := retry.OnConflict(retry.DefaultRetry, func() error {
//	    cur, err := store.Get(ctx, key)
//	    if err != nil { return err }
//	    cur.Status.Phase = "Ready"
//	    return store.Update(ctx, cur)  // may return IsConflict
//	})
package retry

import (
	"context"
	"errors"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apimachinery/util/wait"
)

// DefaultRetry is the budget controllers use for optimistic-locking
// conflicts: 5 tries, 10ms → 20ms → 40ms → 80ms → 160ms with jitter.
var DefaultRetry = wait.Backoff{
	Steps:    5,
	Duration: 10 * time.Millisecond,
	Factor:   2.0,
	Jitter:   0.1,
}

// DefaultBackoff is the budget for API call retries at the client
// layer: longer baseline, more steps, suitable for transient 5xx.
var DefaultBackoff = wait.Backoff{
	Steps:    4,
	Duration: 100 * time.Millisecond,
	Factor:   2.0,
	Jitter:   0.1,
}

// ErrConflict is the sentinel that retryable operations return when
// their underlying Update failed because the target moved under them.
// Callers wrap domain-specific conflict errors with errors.Join or
// fmt.Errorf("%w", ErrConflict) to make them classifiable.
var ErrConflict = errors.New("retry: optimistic-lock conflict")

// IsRetryable is the default retryable-error predicate — identifies
// errors that wrap ErrConflict.  Other callers can pass their own
// predicate via RetryOnError.
func IsRetryable(err error) bool { return errors.Is(err, ErrConflict) }

// OnConflict retries fn while it returns a retryable conflict.  Non-
// conflict errors abort immediately; nil success returns immediately.
func OnConflict(backoff wait.Backoff, fn func() error) error {
	return RetryOnError(context.Background(), backoff, IsRetryable, fn)
}

// OnConflictContext is OnConflict with an explicit context for
// cancellation.
func OnConflictContext(ctx context.Context, backoff wait.Backoff, fn func() error) error {
	return RetryOnError(ctx, backoff, IsRetryable, fn)
}

// RetryOnError is the general variant: retryable is a predicate that
// decides which errors warrant another attempt.  Any non-retryable
// error from fn is returned immediately.
func RetryOnError(ctx context.Context, backoff wait.Backoff, retryable func(error) bool, fn func() error) error {
	return wait.ExponentialBackoff(ctx, backoff, func() (bool, error) {
		err := fn()
		switch {
		case err == nil:
			return true, nil
		case retryable(err):
			return false, nil
		default:
			return false, err
		}
	})
}
