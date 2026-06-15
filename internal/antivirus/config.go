package antivirus

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"os"
	"strconv"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Default values
// ─────────────────────────────────────────────────────────────────────────────

const (
	// DefaultWorkers is the default number of concurrent scan worker
	// goroutines that process the async scan queue.
	DefaultWorkers = 4

	// DefaultQueueSize is the default capacity of the buffered channel
	// that holds pending scan jobs. When the queue is full, new jobs
	// are dropped (logged) to prevent back-pressure on uploads.
	DefaultQueueSize = 10000

	// DefaultMaxFileSize is the maximum file size (100 MB) that the
	// engine will accept for scanning. Files larger than this are
	// skipped with a log entry.
	DefaultMaxFileSize = 100 * 1024 * 1024 // 100 MB

	// DefaultCacheSize is the maximum number of scan results to keep
	// in the LRU cache (keyed by SHA-256).
	DefaultCacheSize = 100_000

	// DefaultCacheTTL is how long a cached scan result remains valid.
	DefaultCacheTTL = 24 * time.Hour

	// DefaultUpdateInterval is how often the engine checks for new
	// signature database updates.
	DefaultUpdateInterval = 6 * time.Hour

	// DefaultSigDir is the default on-disk directory for signature
	// database files.
	DefaultSigDir = "/data/antivirus"

	// DefaultQuarantineAction is what happens when a threat is detected.
	DefaultQuarantineAction = QuarantineTag
)

// ─────────────────────────────────────────────────────────────────────────────
// Config
// ─────────────────────────────────────────────────────────────────────────────

// Config holds all antivirus engine configuration, loaded from environment
// variables. Each field documents the corresponding env-var name and default.
//
// The configuration is designed so that the engine works out-of-the-box with
// zero env vars set — all defaults are production-safe, erring on the side
// of more detection at minimal resource cost.
type Config struct {
	// Enabled controls whether the antivirus engine is active. When false,
	// Scan() returns VerdictClean immediately with zero overhead.
	// Env: ANTIVIRUS_ENABLED (default: true)
	Enabled bool

	// Workers is the number of concurrent scan goroutines.
	// Env: ANTIVIRUS_WORKERS (default: 4)
	Workers int

	// QueueSize is the capacity of the async scan job channel.
	// Env: ANTIVIRUS_QUEUE_SIZE (default: 10000)
	QueueSize int

	// MaxFileSize is the maximum file size in bytes that will be scanned.
	// Files exceeding this limit are skipped (not blocked).
	// Env: ANTIVIRUS_MAX_FILE_SIZE (default: 104857600 = 100MB)
	MaxFileSize int64

	// CacheSize is the maximum number of entries in the scan result cache.
	// Env: ANTIVIRUS_CACHE_SIZE (default: 100000)
	CacheSize int

	// CacheTTL is how long cached scan results are considered valid.
	// Env: ANTIVIRUS_CACHE_TTL (default: 24h)
	CacheTTL time.Duration

	// UpdateURL is the HTTP(S) endpoint from which to download signature
	// database updates. Empty means no auto-updates (built-in sigs only).
	// Env: ANTIVIRUS_UPDATE_URL (default: "")
	UpdateURL string

	// UpdateInterval is how often to check for signature updates.
	// Env: ANTIVIRUS_UPDATE_INTERVAL (default: 6h)
	UpdateInterval time.Duration

	// SigDir is the on-disk directory containing signature database files.
	// The engine will create this directory if it doesn't exist.
	// Env: ANTIVIRUS_SIG_DIR (default: /data/antivirus)
	SigDir string

	// WebhookURL is an optional HTTP endpoint for threat notifications.
	// When set, a POST is sent for every detected threat.
	// Env: ANTIVIRUS_WEBHOOK_URL (default: "")
	WebhookURL string

	// QuarantineAction defines what happens to objects with detected threats.
	// Env: ANTIVIRUS_QUARANTINE_ACTION (default: "tag")
	QuarantineAction QuarantineAction

	// Layer toggles — allow operators to disable individual layers
	// without recompilation.

	// HashDBEnabled enables the known-malware hash lookup layer.
	// Env: ANTIVIRUS_HASH_DB_ENABLED (default: true)
	HashDBEnabled bool

	// PatternEnabled enables the Aho-Corasick byte-pattern matching layer.
	// Env: ANTIVIRUS_PATTERN_ENABLED (default: true)
	PatternEnabled bool

	// HeuristicEnabled enables the behavioral heuristic analysis layer.
	// Env: ANTIVIRUS_HEURISTIC_ENABLED (default: true)
	HeuristicEnabled bool

	// YARAEnabled enables the YARA rule engine layer.
	// Env: ANTIVIRUS_YARA_ENABLED (default: true)
	YARAEnabled bool

	// EntropyEnabled enables the entropy analysis layer.
	// Env: ANTIVIRUS_ENTROPY_ENABLED (default: true)
	EntropyEnabled bool

	// Engine settings
	MaxThreatLogSize  int           // max threats kept in memory (default: 1000)
	StatsLogInterval  time.Duration // how often to log engine stats (default: 5m)
	ManualScanTimeout time.Duration // HTTP manual scan timeout (default: 2m)

	// Entropy thresholds
	EntropyWindowSize            int     // window size in bytes (default: 256)
	EntropyWholeFileThreshold    float64 // whole-file high entropy threshold (default: 7.5)
	EntropyWindowThreshold       float64 // per-window high entropy threshold (default: 6.5)
	EntropyUniformRatio          float64 // ratio of high-entropy windows to flag as packed (default: 0.80)
	EntropyLowStubMaxWindows     int     // max low-entropy windows for packing indicator (default: 3)
	EntropyMinAnalysisSize       int     // skip entropy for files smaller than this (default: 512)
}

// LoadConfig reads antivirus configuration from environment variables,
// applying sensible defaults for any values not set. This follows the
// same pattern as internal/config.LoadConfig().
func LoadConfig() *Config {
	return &Config{
		Enabled:          envBool("ANTIVIRUS_ENABLED", true),
		Workers:          envInt("ANTIVIRUS_WORKERS", DefaultWorkers),
		QueueSize:        envInt("ANTIVIRUS_QUEUE_SIZE", DefaultQueueSize),
		MaxFileSize:      envInt64("ANTIVIRUS_MAX_FILE_SIZE", DefaultMaxFileSize),
		CacheSize:        envInt("ANTIVIRUS_CACHE_SIZE", DefaultCacheSize),
		CacheTTL:         envDuration("ANTIVIRUS_CACHE_TTL", DefaultCacheTTL),
		UpdateURL:        envStr("ANTIVIRUS_UPDATE_URL", ""),
		UpdateInterval:   envDuration("ANTIVIRUS_UPDATE_INTERVAL", DefaultUpdateInterval),
		SigDir:           envStr("ANTIVIRUS_SIG_DIR", DefaultSigDir),
		WebhookURL:       envStr("ANTIVIRUS_WEBHOOK_URL", ""),
		QuarantineAction: ParseQuarantineAction(envStr("ANTIVIRUS_QUARANTINE_ACTION", string(DefaultQuarantineAction))),
		HashDBEnabled:    envBool("ANTIVIRUS_HASH_DB_ENABLED", true),
		PatternEnabled:   envBool("ANTIVIRUS_PATTERN_ENABLED", true),
		HeuristicEnabled: envBool("ANTIVIRUS_HEURISTIC_ENABLED", true),
		YARAEnabled:      envBool("ANTIVIRUS_YARA_ENABLED", true),
		EntropyEnabled:   envBool("ANTIVIRUS_ENTROPY_ENABLED", true),

		MaxThreatLogSize:  envInt("ANTIVIRUS_MAX_THREAT_LOG", 1000),
		StatsLogInterval:  envDuration("ANTIVIRUS_STATS_INTERVAL", 5*time.Minute),
		ManualScanTimeout: envDuration("ANTIVIRUS_MANUAL_SCAN_TIMEOUT", 2*time.Minute),

		EntropyWindowSize:         envInt("ANTIVIRUS_ENTROPY_WINDOW_SIZE", 256),
		EntropyWholeFileThreshold: envFloat64("ANTIVIRUS_ENTROPY_WHOLE_FILE_THRESHOLD", 7.5),
		EntropyWindowThreshold:    envFloat64("ANTIVIRUS_ENTROPY_WINDOW_THRESHOLD", 6.5),
		EntropyUniformRatio:       envFloat64("ANTIVIRUS_ENTROPY_UNIFORM_RATIO", 0.80),
		EntropyLowStubMaxWindows:  envInt("ANTIVIRUS_ENTROPY_LOW_STUB_MAX_WINDOWS", 3),
		EntropyMinAnalysisSize:    envInt("ANTIVIRUS_ENTROPY_MIN_SIZE", 512),
	}
}

// Validate checks the configuration for logical errors and returns a slice
// of warning messages. An empty return means the configuration is valid.
func (c *Config) Validate() []string {
	var warnings []string

	if c.Workers < 1 {
		warnings = append(warnings, "ANTIVIRUS_WORKERS must be ≥ 1; defaulting to 1")
		c.Workers = 1
	}
	if c.Workers > 64 {
		warnings = append(warnings, "ANTIVIRUS_WORKERS > 64 may cause excessive CPU contention")
	}
	if c.QueueSize < 1 {
		warnings = append(warnings, "ANTIVIRUS_QUEUE_SIZE must be ≥ 1; defaulting to 100")
		c.QueueSize = 100
	}
	if c.MaxFileSize < 1 {
		warnings = append(warnings, "ANTIVIRUS_MAX_FILE_SIZE must be > 0; defaulting to 100MB")
		c.MaxFileSize = DefaultMaxFileSize
	}
	if c.CacheSize < 0 {
		warnings = append(warnings, "ANTIVIRUS_CACHE_SIZE must be ≥ 0; defaulting to 0 (disabled)")
		c.CacheSize = 0
	}
	if c.CacheTTL < 0 {
		warnings = append(warnings, "ANTIVIRUS_CACHE_TTL must be ≥ 0; defaulting to 24h")
		c.CacheTTL = DefaultCacheTTL
	}
	if c.UpdateInterval < time.Minute && c.UpdateURL != "" {
		warnings = append(warnings, "ANTIVIRUS_UPDATE_INTERVAL < 1m is too aggressive; defaulting to 1h")
		c.UpdateInterval = time.Hour
	}

	// Count enabled layers
	enabledCount := 0
	if c.HashDBEnabled {
		enabledCount++
	}
	if c.PatternEnabled {
		enabledCount++
	}
	if c.HeuristicEnabled {
		enabledCount++
	}
	if c.YARAEnabled {
		enabledCount++
	}
	if c.EntropyEnabled {
		enabledCount++
	}
	if c.Enabled && enabledCount == 0 {
		warnings = append(warnings, "antivirus is enabled but all detection layers are disabled")
	}

	return warnings
}

// EnabledLayers returns the names of all detection layers that are currently
// enabled in the configuration.
func (c *Config) EnabledLayers() []string {
	var layers []string
	if c.HashDBEnabled {
		layers = append(layers, string(LayerHashDB))
	}
	if c.PatternEnabled {
		layers = append(layers, string(LayerPattern))
	}
	if c.HeuristicEnabled {
		layers = append(layers, string(LayerHeuristic))
	}
	if c.YARAEnabled {
		layers = append(layers, string(LayerYARA))
	}
	if c.EntropyEnabled {
		layers = append(layers, string(LayerEntropy))
	}
	return layers
}

// ─────────────────────────────────────────────────────────────────────────────
// Environment helpers
//
// These mirror the pattern used in internal/config/config.go (getEnv /
// getEnvInt) but add duration, boolean, and int64 variants that are specific
// to this module.
// ─────────────────────────────────────────────────────────────────────────────

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		logging.Z().Info(fmt.Sprintf("⚠️  antivirus: invalid boolean for %s=%q, using default: %v", key, v, fallback))
		return fallback
	}
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  antivirus: invalid integer for %s=%q: %v, using default: %d", key, v, err, fallback))
		return fallback
	}
	return n
}

func envInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  antivirus: invalid int64 for %s=%q: %v, using default: %d", key, v, err, fallback))
		return fallback
	}
	return n
}

func envFloat64(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  antivirus: invalid float for %s=%q: %v, using default: %f", key, v, err, fallback))
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  antivirus: invalid duration for %s=%q: %v, using default: %s", key, v, err, fallback))
		return fallback
	}
	return d
}
