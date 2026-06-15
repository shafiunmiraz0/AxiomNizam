// Package errs re-exports the canonical error sentinels from internal/errors.
//
// DEPRECATED: New code should import internal/errors directly.
// This package exists for backward compatibility with existing code
// that imports platform/errs. All sentinels and helpers are identical
// to those in internal/errors.
package errs

import (
	axiomnizamerrors "axiomnizam.bitbd.net/axiomnizam/internal/errors"
)

// Re-export all sentinels from internal/errors.
var (
	ErrNotFound           = axiomnizamerrors.ErrNotFound
	ErrAlreadyExists      = axiomnizamerrors.ErrAlreadyExists
	ErrConflict           = axiomnizamerrors.ErrConflict
	ErrInvalidInput       = axiomnizamerrors.ErrInvalidInput
	ErrUnauthorized       = axiomnizamerrors.ErrUnauthorized
	ErrForbidden          = axiomnizamerrors.ErrForbidden
	ErrUnavailable        = axiomnizamerrors.ErrUnavailable
	ErrInternal           = axiomnizamerrors.ErrInternal
	ErrTimeout            = axiomnizamerrors.ErrTimeout
	ErrNotImplemented     = axiomnizamerrors.ErrNotImplemented
	ErrPreconditionFailed = axiomnizamerrors.ErrPreconditionFailed
	ErrRateLimited        = axiomnizamerrors.ErrRateLimited
)

// Re-export helper functions.
var (
	Is = axiomnizamerrors.Is
	As = axiomnizamerrors.As
)

// Re-export constructor helpers.
var (
	NotFoundf      = axiomnizamerrors.NotFoundf
	AlreadyExistsf = axiomnizamerrors.AlreadyExistsf
	Conflictf      = axiomnizamerrors.Conflictf
	InvalidInputf  = axiomnizamerrors.InvalidInputf
)
