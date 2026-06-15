package antivirus

// StatusResponse is the API response for engine status.
type StatusResponse struct {
	Status        string         `json:"status"`
	EngineVersion string         `json:"engineVersion"`
	SigDBVersion  string         `json:"sigDbVersion"`
	LayersEnabled []string       `json:"layersEnabled"`
	LayerCount    int            `json:"layerCount"`
	UptimeSeconds int64          `json:"uptimeSeconds"`
	ScanCapacity  ScanCapacity   `json:"scanCapacity"`
	Features      FeaturesConfig `json:"features"`
}

// ScanCapacity holds scan capacity info.
type ScanCapacity struct {
	Workers     int   `json:"workers"`
	QueueSize   int   `json:"queueSize"`
	MaxFileSize int64 `json:"maxFileSize"`
}

// FeaturesConfig holds enabled feature flags.
type FeaturesConfig struct {
	HashDB    bool `json:"hashDB"`
	Pattern   bool `json:"pattern"`
	Heuristic bool `json:"heuristic"`
	Entropy   bool `json:"entropy"`
	YARA      bool `json:"yara"`
}

// StatsResponse is the API response for scan statistics.
type StatsResponse struct {
	TotalScanned  int64      `json:"totalScanned"`
	ThreatsFound  int64      `json:"threatsFound"`
	CleanFiles    int64      `json:"cleanFiles"`
	ErrorCount    int64      `json:"errorCount"`
	BytesScanned  int64      `json:"bytesScanned"`
	AvgScanMs     string     `json:"avgScanMs"`
	Cache         CacheStats `json:"cache"`
	UptimeSeconds int64      `json:"uptimeSeconds"`
	EngineVersion string     `json:"engineVersion"`
	SigDBVersion  string     `json:"sigDbVersion"`
}

// CacheStats holds cache performance stats.
type CacheStats struct {
	Hits    int64  `json:"hits"`
	Misses  int64  `json:"misses"`
	HitRate string `json:"hitRate"`
}

// ConfigResponse is the API response for engine configuration.
type ConfigResponse struct {
	Enabled          bool           `json:"enabled"`
	Workers          int            `json:"workers"`
	QueueSize        int            `json:"queueSize"`
	MaxFileSize      int64          `json:"maxFileSize"`
	CacheSize        int            `json:"cacheSize"`
	CacheTTL         string         `json:"cacheTTL"`
	UpdateURL        string         `json:"updateURL"`
	UpdateInterval   string         `json:"updateInterval"`
	SigDir           string         `json:"sigDir"`
	QuarantineAction string         `json:"quarantineAction"`
	Layers           FeaturesConfig `json:"layers"`
}

// ThreatListResponse is the API response for listing threats.
type ThreatListResponse struct {
	Threats []ThreatRecord `json:"threats"`
	Count   int            `json:"count"`
}

// ErrorResponse is a standard error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Detail  string `json:"detail,omitempty"`
	MaxSize int64  `json:"maxSize,omitempty"`
}

// UpdateConfigRequest is the PUT request body for updating antivirus config.
type UpdateConfigRequest struct {
	Enabled          *bool   `json:"enabled"`
	Workers          *int    `json:"workers"`
	QueueSize        *int    `json:"queueSize"`
	MaxFileSize      *int64  `json:"maxFileSize"`
	CacheSize        *int    `json:"cacheSize"`
	CacheTTL         *string `json:"cacheTTL"`
	QuarantineAction *string `json:"quarantineAction"`
	HashDBEnabled    *bool   `json:"hashDBEnabled"`
	PatternEnabled   *bool   `json:"patternEnabled"`
	HeuristicEnabled *bool   `json:"heuristicEnabled"`
	YARAEnabled      *bool   `json:"yaraEnabled"`
	EntropyEnabled   *bool   `json:"entropyEnabled"`
}
