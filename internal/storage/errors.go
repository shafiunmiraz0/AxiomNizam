package storage

import "axiomnizam.bitbd.net/axiomnizam/internal/errors"

// Storage-specific sentinel errors.
var (
	// ErrBucketNotFound indicates the requested bucket does not exist.
	ErrBucketNotFound = errors.WrapNotFound("bucket", "")

	// ErrObjectNotFound indicates the requested object does not exist.
	ErrObjectNotFound = errors.WrapNotFound("object", "")

	// ErrBucketAlreadyExists indicates a bucket creation conflict.
	ErrBucketAlreadyExists = errors.WrapConflict("bucket", "", "already exists")

	// ErrObjectTooLarge indicates the object exceeds size limits.
	ErrObjectTooLarge = errors.WrapValidation("size", "exceeds maximum allowed size")

	// ErrInvalidBucketName indicates the bucket name is invalid.
	ErrInvalidBucketName = errors.WrapValidation("bucket name", "must be 3-63 characters, lowercase alphanumeric and hyphens")

	// ErrAccessDenied indicates insufficient permissions for the operation.
	ErrAccessDenied = errors.WrapForbidden("storage object", "access")
)
