// Package utils contains general-purpose utilities.
//
// This file re-exports a small set of types from the canonical control-plane
// package (internal/platform/controlplane) so that legacy references inside
// utils/ (e.g. input_validation.go, projection.go) keep compiling after the
// P0.2 move. New code MUST import internal/platform/controlplane directly
// instead of relying on these aliases.
package utils

import "axiomnizam.bitbd.net/axiomnizam/internal/platform/controlplane"

// ValidationError is re-exported from controlplane for backwards compatibility.
type ValidationError = controlplane.ValidationError

// WatchEvent is re-exported from controlplane for backwards compatibility.
type WatchEvent = controlplane.WatchEvent
