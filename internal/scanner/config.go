package scanner

import "axiomnizam.bitbd.net/axiomnizam/internal/scanner/config"

// Config re-exports the scanner config type from the config sub-package.
// This preserves backward compatibility for callers using scanner.Config.
type Config = config.Config

// DefaultConfig re-exports the default config constructor.
var DefaultConfig = config.DefaultConfig

// LoadConfigFromEnv re-exports the env loader.
var LoadConfigFromEnv = config.LoadFromEnv
