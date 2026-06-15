package antivirus

import "fmt"

// StatusToResponse builds a StatusResponse from engine state.
func StatusToResponse(status string, version string, stats EngineStats, cfg *Config) StatusResponse {
	return StatusResponse{
		Status:        status,
		EngineVersion: version,
		SigDBVersion:  stats.SigDBVersion,
		LayersEnabled: stats.LayersEnabled,
		LayerCount:    len(stats.LayersEnabled),
		UptimeSeconds: stats.UptimeSeconds,
		ScanCapacity: ScanCapacity{
			Workers:     cfg.Workers,
			QueueSize:   cfg.QueueSize,
			MaxFileSize: cfg.MaxFileSize,
		},
		Features: FeaturesConfig{
			HashDB:    cfg.HashDBEnabled,
			Pattern:   cfg.PatternEnabled,
			Heuristic: cfg.HeuristicEnabled,
			Entropy:   cfg.EntropyEnabled,
			YARA:      cfg.YARAEnabled,
		},
	}
}

// StatsToResponse builds a StatsResponse from engine stats.
func StatsToResponse(stats EngineStats) StatsResponse {
	return StatsResponse{
		TotalScanned:  stats.TotalScanned,
		ThreatsFound:  stats.ThreatsFound,
		CleanFiles:    stats.CleanFiles,
		ErrorCount:    stats.ErrorCount,
		BytesScanned:  stats.BytesScanned,
		AvgScanMs:     fmt.Sprintf("%.2f", stats.AvgScanMs),
		Cache: CacheStats{
			Hits:    stats.CacheHits,
			Misses:  stats.CacheMisses,
			HitRate: fmt.Sprintf("%.4f", stats.CacheHitRate),
		},
		UptimeSeconds: stats.UptimeSeconds,
		EngineVersion: stats.EngineVersion,
		SigDBVersion:  stats.SigDBVersion,
	}
}

// ConfigToResponse builds a ConfigResponse from engine config.
func ConfigToResponse(cfg *Config) ConfigResponse {
	return ConfigResponse{
		Enabled:          cfg.Enabled,
		Workers:          cfg.Workers,
		QueueSize:        cfg.QueueSize,
		MaxFileSize:      cfg.MaxFileSize,
		CacheSize:        cfg.CacheSize,
		CacheTTL:         cfg.CacheTTL.String(),
		UpdateURL:        redactURL(cfg.UpdateURL),
		UpdateInterval:   cfg.UpdateInterval.String(),
		SigDir:           cfg.SigDir,
		QuarantineAction: string(cfg.QuarantineAction),
		Layers: FeaturesConfig{
			HashDB:    cfg.HashDBEnabled,
			Pattern:   cfg.PatternEnabled,
			Heuristic: cfg.HeuristicEnabled,
			Entropy:   cfg.EntropyEnabled,
			YARA:      cfg.YARAEnabled,
		},
	}
}

// ThreatsToResponse converts engine threats to ThreatListResponse.
func ThreatsToResponse(threats []ScanResult) ThreatListResponse {
	records := make([]ThreatRecord, 0, len(threats))
	for _, r := range threats {
		names := make([]string, 0, len(r.Threats))
		for _, t := range r.Threats {
			names = append(names, t.Name)
		}
		severity := "unknown"
		if hs := r.HighestSeverity(); hs != "" {
			severity = string(hs)
		}
		records = append(records, ThreatRecord{
			Filename:   r.Filename,
			SHA256:     r.SHA256,
			Verdict:    r.Verdict,
			Threats:    names,
			Severity:   severity,
			ScannedAt:  r.ScannedAt,
			DurationMs: r.DurationMs,
		})
	}
	return ThreatListResponse{Threats: records, Count: len(records)}
}

// ApplyUpdateRequest merges a partial update request onto an existing Config,
// returning a new Config with the changes applied. Only non-nil fields are updated.
func ApplyUpdateRequest(base *Config, req *UpdateConfigRequest) *Config {
	updated := *base // copy

	if req.Enabled != nil {
		updated.Enabled = *req.Enabled
	}
	if req.Workers != nil {
		updated.Workers = *req.Workers
	}
	if req.QueueSize != nil {
		updated.QueueSize = *req.QueueSize
	}
	if req.MaxFileSize != nil {
		updated.MaxFileSize = *req.MaxFileSize
	}
	if req.CacheSize != nil {
		updated.CacheSize = *req.CacheSize
	}
	if req.CacheTTL != nil {
		updated.CacheTTL = parseDurationOrDefault(*req.CacheTTL, base.CacheTTL)
	}
	if req.QuarantineAction != nil {
		updated.QuarantineAction = ParseQuarantineAction(*req.QuarantineAction)
	}
	if req.HashDBEnabled != nil {
		updated.HashDBEnabled = *req.HashDBEnabled
	}
	if req.PatternEnabled != nil {
		updated.PatternEnabled = *req.PatternEnabled
	}
	if req.HeuristicEnabled != nil {
		updated.HeuristicEnabled = *req.HeuristicEnabled
	}
	if req.YARAEnabled != nil {
		updated.YARAEnabled = *req.YARAEnabled
	}
	if req.EntropyEnabled != nil {
		updated.EntropyEnabled = *req.EntropyEnabled
	}

	return &updated
}
