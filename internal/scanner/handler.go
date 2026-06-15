package scanner

import (
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/config"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"github.com/gin-gonic/gin"
)

// ScannerConfigHandler exposes scanner configuration endpoints.
type ScannerConfigHandler struct {
	orchestrator *Orchestrator
	kvStore      platformstore.KVStore
}

// NewScannerConfigHandler creates a new scanner config handler.
func NewScannerConfigHandler(orch *Orchestrator, kv platformstore.KVStore) *ScannerConfigHandler {
	return &ScannerConfigHandler{orchestrator: orch, kvStore: kv}
}

// ScannerConfigResponse is the API response for scanner configuration.
type ScannerConfigResponse struct {
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

// UpdateScannerConfigRequest is the PUT request body.
type UpdateScannerConfigRequest struct {
	MaxFileSize              *int64   `json:"maxFileSize"`
	ArchiveMaxDecompressSize *int64   `json:"archiveMaxDecompressSize"`
	ArchiveMaxDepth          *int     `json:"archiveMaxDepth"`
	ArchiveMaxFiles          *int     `json:"archiveMaxFiles"`
	Timeout                  *string  `json:"timeout"`
	Parallel                 *bool    `json:"parallel"`
	NullByteSampleSize       *int     `json:"nullByteSampleSize"`
	MaxFilenameLength        *int     `json:"maxFilenameLength"`
	ArchiveCompressionRatioLimit *float64 `json:"archiveCompressionRatioLimit"`
}

// GetConfig returns the current scanner configuration.
func (h *ScannerConfigHandler) GetConfig(c *gin.Context) {
	cfg := h.orchestrator.Config()
	c.JSON(http.StatusOK, ScannerConfigResponse{
		MaxFileSize:              cfg.MaxFileSize,
		ArchiveMaxDecompressSize: cfg.ArchiveMaxDecompressedSize,
		ArchiveMaxDepth:          cfg.ArchiveMaxDepth,
		ArchiveMaxFiles:          cfg.ArchiveMaxFiles,
		Timeout:                  cfg.Timeout.String(),
		Parallel:                 cfg.Parallel,
		NullByteSampleSize:       cfg.NullByteSampleSize,
		MaxFilenameLength:        cfg.MaxFilenameLength,
		ArchiveCompressionRatioLimit: cfg.ArchiveCompressionRatioLimit,
	})
}

// UpdateConfig applies a partial config update and persists to KV store.
func (h *ScannerConfigHandler) UpdateConfig(c *gin.Context) {
	var req UpdateScannerConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	current := h.orchestrator.Config()
	updated := applyScannerUpdate(current, &req)

	if err := updated.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.orchestrator.UpdateConfig(updated); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if h.kvStore != nil {
		if err := SaveScannerConfigToKV(c.Request.Context(), h.kvStore, updated); err != nil {
			logging.Z().Error("scanner: failed to persist config to KV: " + err.Error())
		}
	}

	c.JSON(http.StatusOK, ScannerConfigResponse{
		MaxFileSize:              updated.MaxFileSize,
		ArchiveMaxDecompressSize: updated.ArchiveMaxDecompressedSize,
		ArchiveMaxDepth:          updated.ArchiveMaxDepth,
		ArchiveMaxFiles:          updated.ArchiveMaxFiles,
		Timeout:                  updated.Timeout.String(),
		Parallel:                 updated.Parallel,
		NullByteSampleSize:       updated.NullByteSampleSize,
		MaxFilenameLength:        updated.MaxFilenameLength,
		ArchiveCompressionRatioLimit: updated.ArchiveCompressionRatioLimit,
	})
}

func applyScannerUpdate(base config.Config, req *UpdateScannerConfigRequest) config.Config {
	updated := base
	if req.MaxFileSize != nil {
		updated.MaxFileSize = *req.MaxFileSize
	}
	if req.ArchiveMaxDecompressSize != nil {
		updated.ArchiveMaxDecompressedSize = *req.ArchiveMaxDecompressSize
	}
	if req.ArchiveMaxDepth != nil {
		updated.ArchiveMaxDepth = *req.ArchiveMaxDepth
	}
	if req.ArchiveMaxFiles != nil {
		updated.ArchiveMaxFiles = *req.ArchiveMaxFiles
	}
	if req.Timeout != nil {
		if d, err := time.ParseDuration(*req.Timeout); err == nil {
			updated.Timeout = d
		}
	}
	if req.Parallel != nil {
		updated.Parallel = *req.Parallel
	}
	if req.NullByteSampleSize != nil {
		updated.NullByteSampleSize = *req.NullByteSampleSize
	}
	if req.MaxFilenameLength != nil {
		updated.MaxFilenameLength = *req.MaxFilenameLength
	}
	if req.ArchiveCompressionRatioLimit != nil {
		updated.ArchiveCompressionRatioLimit = *req.ArchiveCompressionRatioLimit
	}
	return updated
}
