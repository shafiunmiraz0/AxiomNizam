package antivirus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

const (
	antivirusConfigKVKey   = "antivirus:config"
	antivirusConfigTimeout = 3 * time.Second
)

// configPersist is the JSON-serializable form stored in the KV store.
type configPersist struct {
	Enabled          bool   `json:"enabled"`
	Workers          int    `json:"workers"`
	QueueSize        int    `json:"queueSize"`
	MaxFileSize      int64  `json:"maxFileSize"`
	CacheSize        int    `json:"cacheSize"`
	CacheTTL         string `json:"cacheTTL"`
	QuarantineAction string `json:"quarantineAction"`
	HashDBEnabled    bool   `json:"hashDBEnabled"`
	PatternEnabled   bool   `json:"patternEnabled"`
	HeuristicEnabled bool   `json:"heuristicEnabled"`
	YARAEnabled      bool   `json:"yaraEnabled"`
	EntropyEnabled   bool   `json:"entropyEnabled"`
}

// LoadConfigFromKV loads antivirus config from the KV store.
// Returns nil if no persisted config exists (caller should fall back to env).
func LoadConfigFromKV(ctx context.Context, kv platformstore.KVStore) *Config {
	if kv == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, antivirusConfigTimeout)
	defer cancel()

	val, err := kv.Get(ctx, antivirusConfigKVKey)
	if err != nil || val == "" {
		return nil
	}

	var persist configPersist
	if err := json.Unmarshal([]byte(val), &persist); err != nil {
		logging.Z().Warn(fmt.Sprintf("antivirus: failed to unmarshal persisted config, falling back to env: %v", err))
		return nil
	}

	return &Config{
		Enabled:          persist.Enabled,
		Workers:          persist.Workers,
		QueueSize:        persist.QueueSize,
		MaxFileSize:      persist.MaxFileSize,
		CacheSize:        persist.CacheSize,
		CacheTTL:         parseDurationOrDefault(persist.CacheTTL, 24*time.Hour),
		QuarantineAction: ParseQuarantineAction(persist.QuarantineAction),
		HashDBEnabled:    persist.HashDBEnabled,
		PatternEnabled:   persist.PatternEnabled,
		HeuristicEnabled: persist.HeuristicEnabled,
		YARAEnabled:      persist.YARAEnabled,
		EntropyEnabled:   persist.EntropyEnabled,
		// Fields not exposed to admin UI retain env-var defaults.
		UpdateURL:         envStr("ANTIVIRUS_UPDATE_URL", ""),
		UpdateInterval:    envDuration("ANTIVIRUS_UPDATE_INTERVAL", 6*time.Hour),
		SigDir:            envStr("ANTIVIRUS_SIG_DIR", "/data/antivirus"),
		WebhookURL:        envStr("ANTIVIRUS_WEBHOOK_URL", ""),
		MaxThreatLogSize:  envInt("ANTIVIRUS_MAX_THREAT_LOG", 1000),
		StatsLogInterval:  envDuration("ANTIVIRUS_STATS_INTERVAL", 5*time.Minute),
		ManualScanTimeout: envDuration("ANTIVIRUS_MANUAL_SCAN_TIMEOUT", 2*time.Minute),
	}
}

// SaveConfigToKV persists the antivirus config to the KV store.
func SaveConfigToKV(ctx context.Context, kv platformstore.KVStore, cfg *Config) error {
	if kv == nil {
		return nil
	}

	persist := configPersist{
		Enabled:          cfg.Enabled,
		Workers:          cfg.Workers,
		QueueSize:        cfg.QueueSize,
		MaxFileSize:      cfg.MaxFileSize,
		CacheSize:        cfg.CacheSize,
		CacheTTL:         cfg.CacheTTL.String(),
		QuarantineAction: string(cfg.QuarantineAction),
		HashDBEnabled:    cfg.HashDBEnabled,
		PatternEnabled:   cfg.PatternEnabled,
		HeuristicEnabled: cfg.HeuristicEnabled,
		YARAEnabled:      cfg.YARAEnabled,
		EntropyEnabled:   cfg.EntropyEnabled,
	}

	data, err := json.Marshal(persist)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, antivirusConfigTimeout)
	defer cancel()

	return kv.Put(ctx, antivirusConfigKVKey, string(data))
}

func parseDurationOrDefault(s string, dflt time.Duration) time.Duration {
	if s == "" {
		return dflt
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return dflt
	}
	return d
}
