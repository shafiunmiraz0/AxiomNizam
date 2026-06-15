package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/config"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

const (
	scannerConfigKVKey   = "scanner:config"
	scannerConfigTimeout = 3 * time.Second
)

// scannerConfigPersist is the JSON-serializable form of scanner config.
type scannerConfigPersist struct {
	MaxFileSize              int64   `json:"maxFileSize"`
	ArchiveMaxDecompressSize int64   `json:"archiveMaxDecompressSize"`
	ArchiveMaxDepth          int     `json:"archiveMaxDepth"`
	ArchiveMaxFiles          int     `json:"archiveMaxFiles"`
	Timeout                  string  `json:"timeout"`
	Parallel                 bool    `json:"parallel"`
	NullByteSampleSize       int     `json:"nullByteSampleSize"`
	MaxFilenameLength        int     `json:"maxFilenameLength"`
	ArchiveCompressionRatioLimit float64 `json:"archiveCompressionRatioLimit"`
}

// LoadScannerConfigFromKV loads scanner config from the KV store.
// Returns nil if no persisted config exists.
func LoadScannerConfigFromKV(ctx context.Context, kv platformstore.KVStore) *config.Config {
	if kv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, scannerConfigTimeout)
	defer cancel()

	val, err := kv.Get(ctx, scannerConfigKVKey)
	if err != nil || val == "" {
		return nil
	}

	var persist scannerConfigPersist
	if err := json.Unmarshal([]byte(val), &persist); err != nil {
		logging.Z().Warn(fmt.Sprintf("scanner: failed to unmarshal persisted config: %v", err))
		return nil
	}

	cfg := config.DefaultConfig()
	cfg.MaxFileSize = persist.MaxFileSize
	cfg.ArchiveMaxDecompressedSize = persist.ArchiveMaxDecompressSize
	cfg.ArchiveMaxDepth = persist.ArchiveMaxDepth
	cfg.ArchiveMaxFiles = persist.ArchiveMaxFiles
	cfg.Parallel = persist.Parallel
	cfg.NullByteSampleSize = persist.NullByteSampleSize
	cfg.MaxFilenameLength = persist.MaxFilenameLength
	cfg.ArchiveCompressionRatioLimit = persist.ArchiveCompressionRatioLimit

	if persist.Timeout != "" {
		if d, err := time.ParseDuration(persist.Timeout); err == nil {
			cfg.Timeout = d
		}
	}

	return &cfg
}

// SaveScannerConfigToKV persists scanner config to the KV store.
func SaveScannerConfigToKV(ctx context.Context, kv platformstore.KVStore, cfg config.Config) error {
	if kv == nil {
		return nil
	}

	persist := scannerConfigPersist{
		MaxFileSize:              cfg.MaxFileSize,
		ArchiveMaxDecompressSize: cfg.ArchiveMaxDecompressedSize,
		ArchiveMaxDepth:          cfg.ArchiveMaxDepth,
		ArchiveMaxFiles:          cfg.ArchiveMaxFiles,
		Timeout:                  cfg.Timeout.String(),
		Parallel:                 cfg.Parallel,
		NullByteSampleSize:       cfg.NullByteSampleSize,
		MaxFilenameLength:        cfg.MaxFilenameLength,
		ArchiveCompressionRatioLimit: cfg.ArchiveCompressionRatioLimit,
	}

	data, err := json.Marshal(persist)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, scannerConfigTimeout)
	defer cancel()

	return kv.Put(ctx, scannerConfigKVKey, string(data))
}
