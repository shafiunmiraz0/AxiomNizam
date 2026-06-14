package iam

import "axiomnizam.bitbd.net/axiomnizam/internal/errors"

// IAM-specific sentinel errors.
var (
	// ErrUserNotFound indicates the requested user does not exist.
	ErrUserNotFound = errors.WrapNotFound("user", "")

	// ErrClientNotFound indicates the requested OAuth client does not exist.
	ErrClientNotFound = errors.WrapNotFound("client", "")

	// ErrRealmNotFound indicates the requested realm does not exist.
	ErrRealmNotFound = errors.WrapNotFound("realm", "")

	// ErrRoleNotFound indicates the requested role does not exist.
	ErrRoleNotFound = errors.WrapNotFound("role", "")

	// ErrSessionExpired indicates the session has expired.
	ErrSessionExpired = errors.WrapUnauthorized("session expired")

	// ErrInvalidCredentials indicates wrong username/password.
	ErrInvalidCredentials = errors.WrapUnauthorized("invalid credentials")

	// ErrTokenExpired indicates the JWT token has expired.
	ErrTokenExpired = errors.WrapUnauthorized("token expired")

	// ErrTokenRevoked indicates the JWT token has been revoked.
	ErrTokenRevoked = errors.WrapUnauthorized("token revoked")

	// ErrInsufficientScope indicates the token lacks required scope.
	ErrInsufficientScope = errors.WrapForbidden("token", "insufficient scope")

	// ErrDuplicateUser indicates a user creation conflict.
	ErrDuplicateUser = errors.WrapConflict("user", "", "already exists")
)
