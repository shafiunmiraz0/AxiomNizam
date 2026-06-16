// Package errors provides shared, typed error types for all AxiomNizam modules.
// These replace string-based error comparisons with structured, matchable errors.
//
// Usage:
//
//	if errors.Is(err, errors.ErrNotFound) { ... }
//	return errors.WrapNotFound("bucket", name)
package errors

import (
	"errors"
	"fmt"
)

// Re-export standard library functions for convenience.
var (
	Is     = errors.Is
	As     = errors.As
	Unwrap = errors.Unwrap
	New    = errors.New
)

// ─────────────────────────────────────────────────────────────────────────────
// Sentinel errors — shared across all modules
// ─────────────────────────────────────────────────────────────────────────────

var (
	// ErrNotFound indicates the requested resource does not exist.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists indicates a resource creation conflict.
	ErrAlreadyExists = errors.New("already exists")

	// ErrUnauthorized indicates missing or invalid authentication.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates insufficient permissions.
	ErrForbidden = errors.New("forbidden")

	// ErrInvalidInput indicates invalid request parameters.
	ErrInvalidInput = errors.New("invalid input")

	// ErrConflict indicates a state conflict (e.g., optimistic lock).
	ErrConflict = errors.New("conflict")

	// ErrInternal indicates an unexpected internal error.
	ErrInternal = errors.New("internal error")

	// ErrTimeout indicates an operation timed out.
	ErrTimeout = errors.New("timeout")

	// ErrUnavailable indicates the service is temporarily unavailable.
	ErrUnavailable = errors.New("unavailable")

	// ErrNotImplemented indicates a feature is not yet implemented.
	ErrNotImplemented = errors.New("not implemented")

	// ErrPreconditionFailed indicates a precondition check failed.
	ErrPreconditionFailed = errors.New("precondition failed")

	// ErrRateLimited indicates the request was rate-limited.
	ErrRateLimited = errors.New("rate limited")
)

// ─────────────────────────────────────────────────────────────────────────────
// Typed errors — structured context for error reporting
// ─────────────────────────────────────────────────────────────────────────────

// NotFoundError wraps ErrNotFound with resource context.
type NotFoundError struct {
	Resource string
	Name     string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Resource, e.Name)
}

func (e *NotFoundError) Unwrap() error { return ErrNotFound }

// ValidationError wraps ErrInvalidInput with field-level detail.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

// ConflictError wraps ErrConflict with resource context.
type ConflictError struct {
	Resource string
	Name     string
	Message  string
}

func (e *ConflictError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s conflict %s: %s", e.Resource, e.Name, e.Message)
	}
	return fmt.Sprintf("%s conflict: %s", e.Resource, e.Name)
}

func (e *ConflictError) Unwrap() error { return ErrConflict }

// UnauthorizedError wraps ErrUnauthorized with a reason.
type UnauthorizedError struct {
	Reason string
}

func (e *UnauthorizedError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("unauthorized: %s", e.Reason)
	}
	return "unauthorized"
}

func (e *UnauthorizedError) Unwrap() error { return ErrUnauthorized }

// ForbiddenError wraps ErrForbidden with resource/action context.
type ForbiddenError struct {
	Resource string
	Action   string
}

func (e *ForbiddenError) Error() string {
	return fmt.Sprintf("forbidden: cannot %s %s", e.Action, e.Resource)
}

func (e *ForbiddenError) Unwrap() error { return ErrForbidden }

// ─────────────────────────────────────────────────────────────────────────────
// Constructor helpers — concise error creation
// ─────────────────────────────────────────────────────────────────────────────

// WrapNotFound creates a NotFoundError.
func WrapNotFound(resource, name string) error {
	return &NotFoundError{Resource: resource, Name: name}
}

// WrapValidation creates a ValidationError.
func WrapValidation(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}

// WrapConflict creates a ConflictError.
func WrapConflict(resource, name, message string) error {
	return &ConflictError{Resource: resource, Name: name, Message: message}
}

// WrapUnauthorized creates an UnauthorizedError.
func WrapUnauthorized(reason string) error {
	return &UnauthorizedError{Reason: reason}
}

// WrapForbidden creates a ForbiddenError.
func WrapForbidden(resource, action string) error {
	return &ForbiddenError{Resource: resource, Action: action}
}

// WrapInternal wraps an internal error with context.
func WrapInternal(msg string, cause error) error {
	return fmt.Errorf("%s: %w", msg, cause)
}

// WrapPrecondition creates a precondition failed error.
func WrapPrecondition(msg string) error {
	return fmt.Errorf("%s: %w", msg, ErrPreconditionFailed)
}

// WrapUnavailable creates an unavailable error.
func WrapUnavailable(msg string) error {
	return fmt.Errorf("%s: %w", msg, ErrUnavailable)
}

// ─────────────────────────────────────────────────────────────────────────────
// Format helpers — concise error creation with formatted messages
// ─────────────────────────────────────────────────────────────────────────────

// NotFoundf wraps ErrNotFound with a formatted message.
func NotFoundf(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), ErrNotFound)
}

// AlreadyExistsf wraps ErrAlreadyExists with a formatted message.
func AlreadyExistsf(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), ErrAlreadyExists)
}

// Conflictf wraps ErrConflict with a formatted message.
func Conflictf(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), ErrConflict)
}

// InvalidInputf wraps ErrInvalidInput with a formatted message.
func InvalidInputf(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), ErrInvalidInput)
}
