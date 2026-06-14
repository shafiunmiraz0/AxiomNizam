// Package config provides a consistent config sub-package for the antivirus module.
// The actual Config struct and LoadConfig() live in the root antivirus package
// (to avoid circular dependencies with QuarantineAction and Layer types).
// This package re-exports them for import consistency.
package config

import "axiomnizam.bitbd.net/axiomnizam/internal/antivirus"

// Config is an alias for antivirus.Config.
type Config = antivirus.Config

// DefaultConfig returns a Config loaded from environment variables.
var DefaultConfig = antivirus.LoadConfig

// LoadFromEnv is an alias for LoadConfig.
var LoadFromEnv = antivirus.LoadConfig
