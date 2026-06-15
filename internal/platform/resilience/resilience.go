// Package resilience provides retry-with-backoff and circuit breaker
// utilities for the platform's reconcilers and external call sites.
//
// The platform already has backoff strategies in internal/utils/backoff
// and a circuit breaker in internal/utils/cncf_kubernetes.go. This
// package provides higher-level wrappers that combine both patterns
// into a single call site, with structured logging and context awareness.
//
// Usage:
//
//	result, err := resilience.Do(ctx, resilience.Config{
//	    MaxAttempts: 3,
//	    InitialDelay: 100 * time.Millisecond,
//	    MaxDelay: 5 * time.Second,
//	}, func(ctx context.Context) (T, error) {
//	    return callExternalService(ctx)
//	})
package resilience

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"go.uber.org/zap"
)

// Config controls retry and backoff behavior.
type Config struct {
	// MaxAttempts is the total number of attempts (1 = no retry).
	MaxAttempts int

	// InitialDelay is the delay before the first retry.
	InitialDelay time.Duration

	// MaxDelay caps the backoff duration.
	MaxDelay time.Duration

	// Multiplier is the backoff multiplier (default: 2.0).
	Multiplier float64

	// Jitter adds randomness to prevent thundering herd (default: true).
	Jitter bool

	// RetryableCheck determines if an error is retryable. If nil, all errors are retried.
	RetryableCheck func(error) bool

	// Name is used in log messages to identify the operation.
	Name string
}

// DefaultConfig returns a sensible default retry configuration.
func DefaultConfig(name string) Config {
	return Config{
		MaxAttempts:  3,
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		Name:         name,
	}
}

// Do executes fn with retry and exponential backoff.
// Returns the result of the first successful call, or the last error
// after all attempts are exhausted.
func Do[T any](ctx context.Context, cfg Config, fn func(ctx context.Context) (T, error)) (T, error) {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.Multiplier <= 0 {
		cfg.Multiplier = 2.0
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 200 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 10 * time.Second
	}

	var lastErr error
	var zero T

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		// Check context before each attempt.
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		default:
		}

		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if error is retryable.
		if cfg.RetryableCheck != nil && !cfg.RetryableCheck(err) {
			return zero, err
		}

		// Don't sleep after the last attempt.
		if attempt == cfg.MaxAttempts {
			break
		}

		// Calculate backoff delay.
		delay := backoffDelay(cfg.InitialDelay, cfg.MaxDelay, cfg.Multiplier, attempt, cfg.Jitter)

		if cfg.Name != "" {
			logging.Z().Warn("operation failed, retrying",
				zap.String("op", cfg.Name),
				zap.Int("attempt", attempt),
				zap.Int("maxAttempts", cfg.MaxAttempts),
				zap.Duration("nextDelay", delay),
				zap.Error(err),
			)
		}

		// Wait for backoff delay or context cancellation.
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	}

	return zero, fmt.Errorf("%s: all %d attempts failed: %w", cfg.Name, cfg.MaxAttempts, lastErr)
}

// DoVoid executes a void function with retry and exponential backoff.
func DoVoid(ctx context.Context, cfg Config, fn func(ctx context.Context) error) error {
	_, err := Do(ctx, cfg, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}

// backoffDelay calculates the delay for a given attempt.
func backoffDelay(initial, max time.Duration, multiplier float64, attempt int, jitter bool) time.Duration {
	delay := time.Duration(float64(initial) * math.Pow(multiplier, float64(attempt-1)))
	if delay > max {
		delay = max
	}
	if jitter && delay > 0 {
		// Add ±25% jitter.
		jitterRange := int64(float64(delay) * 0.25)
		if jitterRange > 0 {
			delay += time.Duration(rand.Int63n(jitterRange*2) - jitterRange)
		}
	}
	if delay < 0 {
		delay = 0
	}
	return delay
}

// ReconcileBackoff calculates the requeue delay for a reconciler based
// on consecutive failure count. This replaces fixed requeue intervals
// with exponential backoff that prevents hammering a failing dependency.
//
// Usage in reconcilers:
//
//	return reconciler.ReconcileResult{
//	    Requeue:      true,
//	    RequeueAfter: resilience.ReconcileBackoff(status.ConsecutiveFailures),
//	}
func ReconcileBackoff(failures int) time.Duration {
	if failures <= 0 {
		return 5 * time.Second
	}
	delay := time.Duration(float64(5*time.Second) * math.Pow(2, float64(failures-1)))
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	// Add jitter.
	jitterRange := int64(float64(delay) * 0.2)
	if jitterRange > 0 {
		delay += time.Duration(rand.Int63n(jitterRange))
	}
	return delay
}

// =====================================================
// Circuit Breaker
// =====================================================

// CircuitState represents the state of a circuit breaker.
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"    // Normal operation
	CircuitOpen     CircuitState = "open"      // Failing, reject calls
	CircuitHalfOpen CircuitState = "half-open" // Testing recovery
)

// CircuitBreaker prevents cascading failures by short-circuiting
// calls to a failing dependency.
type CircuitBreaker struct {
	mu               sync.Mutex
	name             string
	state            CircuitState
	failureCount     int
	successCount     int
	failureThreshold int
	successThreshold int // Successes needed in half-open to close
	timeout          time.Duration
	lastFailTime     time.Time
	lastStateChange  time.Time
}

// NewCircuitBreaker creates a circuit breaker.
//
//	cb := resilience.NewCircuitBreaker("notifications", 5, 30*time.Second)
//	err := cb.Execute(ctx, func(ctx context.Context) error {
//	    return sendNotification(ctx, msg)
//	})
func NewCircuitBreaker(name string, failureThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:             name,
		state:            CircuitClosed,
		failureThreshold: failureThreshold,
		successThreshold: 2,
		timeout:          timeout,
		lastStateChange:  time.Now(),
	}
}

// Execute runs fn through the circuit breaker.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	cb.mu.Lock()

	switch cb.state {
	case CircuitOpen:
		if time.Since(cb.lastFailTime) > cb.timeout {
			// Transition to half-open.
			cb.state = CircuitHalfOpen
			cb.successCount = 0
			cb.lastStateChange = time.Now()
			logging.Z().Info("circuit breaker half-open",
				zap.String("breaker", cb.name),
			)
		} else {
			cb.mu.Unlock()
			return fmt.Errorf("circuit breaker '%s' is open (resets in %s)",
				cb.name, cb.timeout-time.Since(cb.lastFailTime))
		}
	}

	cb.mu.Unlock()

	// Execute the function.
	err := fn(ctx)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failureCount++
		cb.lastFailTime = time.Now()

		if cb.failureCount >= cb.failureThreshold {
			cb.state = CircuitOpen
			cb.lastStateChange = time.Now()
			logging.Z().Warn("circuit breaker opened",
				zap.String("breaker", cb.name),
				zap.Int("failures", cb.failureCount),
				zap.Duration("timeout", cb.timeout),
				zap.Error(err),
			)
		}
		return err
	}

	// Success.
	cb.successCount++
	if cb.state == CircuitHalfOpen && cb.successCount >= cb.successThreshold {
		cb.state = CircuitClosed
		cb.failureCount = 0
		cb.lastStateChange = time.Now()
		logging.Z().Info("circuit breaker closed",
			zap.String("breaker", cb.name),
			zap.Int("successes", cb.successCount),
		)
	} else if cb.state == CircuitClosed {
		// Reset failure count on success in closed state.
		cb.failureCount = 0
	}

	return nil
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Stats returns circuit breaker statistics.
func (cb *CircuitBreaker) Stats() map[string]interface{} {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return map[string]interface{}{
		"name":            cb.name,
		"state":           cb.state,
		"failureCount":    cb.failureCount,
		"successCount":    cb.successCount,
		"lastFailTime":    cb.lastFailTime,
		"lastStateChange": cb.lastStateChange,
	}
}
