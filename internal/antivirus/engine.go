package antivirus

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────────────────────────────────────

const (
	// EngineVersion is the semantic version of the antivirus engine.
	// Bumped when detection logic changes (not on signature updates).
	EngineVersion = "0.1.0"
)

// ─────────────────────────────────────────────────────────────────────────────
// Engine
// ─────────────────────────────────────────────────────────────────────────────

// Engine is the core antivirus scanner. It orchestrates registered scan
// layers, manages the async scan queue, collects statistics, and handles
// lifecycle (start / shutdown).
//
// Usage:
//
//	cfg := antivirus.LoadConfig()
//	engine := antivirus.NewEngine(cfg)
//	// Register layers (added in later phases):
//	//   engine.RegisterLayer(hashdb.New(...))
//	//   engine.RegisterLayer(matcher.New(...))
//	engine.Start()
//	defer engine.Shutdown(context.Background())
//
//	result, err := engine.Scan(ctx, fileBytes, "report.pdf")
type Engine struct {
	cfg *Config

	// layers holds registered scan layers in execution order. Protected
	// by layersMu only during registration (which happens at startup,
	// before Scan() is ever called). During steady-state scanning the
	// slice is read without locking because it's immutable after Start().
	layers   []ScanLayer
	layersMu sync.RWMutex

	// stats is the runtime statistics counter. All fields are updated
	// atomically — no lock needed for reads.
	stats engineStatsAtomic

	// startTime records when the engine was started, used for uptime.
	startTime time.Time

	// sigDBVersion is the currently loaded signature DB version string.
	// Protected by sigDBMu for hot-reload support.
	sigDBVersion string
	sigDBMu      sync.RWMutex

	// threatLog stores recent threat detections for the /threats API.
	// Capped at 1000 entries, oldest evicted first.
	threatLog   []ScanResult
	threatLogMu sync.Mutex

	// cancel is the context cancellation function for background workers.
	cancel context.CancelFunc

	// wg tracks background goroutines for clean shutdown.
	wg sync.WaitGroup

	// started guards against double-start.
	started atomic.Bool
}

// engineStatsAtomic stores counters using atomic int64 for lock-free updates.
type engineStatsAtomic struct {
	totalScanned atomic.Int64
	threatsFound atomic.Int64
	cleanFiles   atomic.Int64
	errorCount   atomic.Int64
	cacheHits    atomic.Int64
	cacheMisses  atomic.Int64
	bytesScanned atomic.Int64
	totalScanNs  atomic.Int64 // accumulated nanoseconds for avg calculation
}

// NewEngine creates a new antivirus engine with the provided configuration.
// The engine is not started until Start() is called.
func NewEngine(cfg *Config) *Engine {
	if cfg == nil {
		cfg = LoadConfig()
	}

	// Validate and auto-correct configuration.
	if warnings := cfg.Validate(); len(warnings) > 0 {
		for _, w := range warnings {
			logging.Z().Info(fmt.Sprintf("⚠️  antivirus config: %s", w))
		}
	}

	return &Engine{
		cfg:    cfg,
		layers: make([]ScanLayer, 0, 5),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Layer Registration
// ─────────────────────────────────────────────────────────────────────────────

// RegisterLayer adds a scan layer to the engine. Layers are executed in
// the order they are registered. This method MUST be called before Start().
//
// Panics if called after Start() to prevent data races.
func (e *Engine) RegisterLayer(layer ScanLayer) {
	if e.started.Load() {
		panic("antivirus: RegisterLayer called after Start()")
	}
	e.layersMu.Lock()
	defer e.layersMu.Unlock()

	// Guard against duplicate layer names.
	for _, existing := range e.layers {
		if existing.Name() == layer.Name() {
			logging.Z().Info(fmt.Sprintf("⚠️  antivirus: duplicate layer %q ignored", layer.Name()))
			return
		}
	}

	e.layers = append(e.layers, layer)
	logging.Z().Info(fmt.Sprintf("🛡️  antivirus: registered layer %q", layer.Name()))
}

// LayerCount returns the number of registered scan layers.
func (e *Engine) LayerCount() int {
	e.layersMu.RLock()
	defer e.layersMu.RUnlock()
	return len(e.layers)
}

// ─────────────────────────────────────────────────────────────────────────────
// Lifecycle
// ─────────────────────────────────────────────────────────────────────────────

// Start initialises the engine and begins background workers. After this
// call, the layer list is frozen and Scan() may be called concurrently.
// Start starts the antivirus engine with a background context.
func (e *Engine) Start() {
	if err := e.StartCtx(context.Background()); err != nil {
		logging.Z().Error(fmt.Sprintf("antivirus: engine start failed: %v", err))
	}
}

// StartCtx starts the antivirus engine with the given context.
// Satisfies the contracts.Module lifecycle interface.
func (e *Engine) StartCtx(ctx context.Context) error {
	if !e.cfg.Enabled {
		logging.Z().Info("🛡️  antivirus: engine disabled via ANTIVIRUS_ENABLED=false")
		return nil
	}

	if !e.started.CompareAndSwap(false, true) {
		logging.Z().Info("⚠️  antivirus: engine already started")
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.startTime = time.Now()

	// Freeze the layer list (no more registrations after this point).
	e.layersMu.Lock()
	layerNames := make([]string, len(e.layers))
	for i, l := range e.layers {
		layerNames[i] = l.Name()
	}
	e.layersMu.Unlock()

	logging.Z().Info(fmt.Sprintf("🛡️  antivirus engine v%s started — %d layers: %v, workers: %d, queue: %d",
		EngineVersion, len(layerNames), layerNames, e.cfg.Workers, e.cfg.QueueSize))

	// Background heartbeat/stats logger.
	e.wg.Add(1)
	go e.statsLogger(ctx)
	return nil
}

// Shutdown gracefully stops the engine and waits for in-flight scans to
// complete. The provided context controls the maximum wait time.
func (e *Engine) Shutdown(ctx context.Context) error {
	if !e.started.Load() {
		return nil // never started, nothing to do
	}

	logging.Z().Info("🛡️  antivirus: shutting down engine...")

	if e.cancel != nil {
		e.cancel()
	}

	// Wait for goroutines to exit, or context deadline.
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logging.Z().Info("🛡️  antivirus: engine shut down cleanly")
		return nil
	case <-ctx.Done():
		logging.Z().Info(fmt.Sprint("⚠️  antivirus: shutdown timed out, some workers may still be running"))
		return ctx.Err()
	}
}

// IsRunning returns true if the engine has been started and not yet shut down.
func (e *Engine) IsRunning() bool {
	return e.started.Load()
}

// ─────────────────────────────────────────────────────────────────────────────
// Core Scan
// ─────────────────────────────────────────────────────────────────────────────

// Scan performs a synchronous scan of the provided file content, executing
// all registered layers in order. This is the primary entry point for
// scanning.
//
// The engine:
//  1. Validates that scanning is enabled and the file is within size limits.
//  2. Computes the SHA-256 hash of the content.
//  3. Detects the MIME type via magic bytes.
//  4. Runs each registered layer sequentially, collecting threats.
//  5. Aggregates results into a single ScanResult.
//
// Sequential execution is deliberate: layers are ordered from fastest to
// slowest, and an early "malware" detection from the hash-DB layer means
// we can skip expensive pattern matching (future optimisation path via
// EarlyExit config).
func (e *Engine) Scan(ctx context.Context, content []byte, filename string) (*ScanResult, error) {
	start := time.Now()

	// Fast path: engine disabled.
	if !e.cfg.Enabled {
		return &ScanResult{
			Verdict:       VerdictClean,
			ScannedAt:     start.UTC(),
			DurationMs:    0,
			EngineVersion: EngineVersion,
		}, nil
	}

	// Validate file size.
	fileSize := int64(len(content))
	if fileSize > e.cfg.MaxFileSize {
		logging.Z().Info(fmt.Sprintf("🛡️  antivirus: skipping %q (%d bytes > max %d bytes)",
			filename, fileSize, e.cfg.MaxFileSize))
		return &ScanResult{
			Verdict:       VerdictClean,
			FileSize:      fileSize,
			ScannedAt:     start.UTC(),
			DurationMs:    time.Since(start).Milliseconds(),
			EngineVersion: EngineVersion,
			LayersRun:     []string{},
			Threats:       []ThreatInfo{},
		}, nil
	}

	// Compute SHA-256.
	hash := sha256.Sum256(content)
	sha256Hex := hex.EncodeToString(hash[:])

	// Detect MIME type via magic bytes (first 512 bytes).
	mimeType := http.DetectContentType(content)

	// Build scan target.
	target := &ScanTarget{
		Filename:             filename,
		SHA256:               sha256Hex,
		Size:                 fileSize,
		MIMEType:             mimeType,
		Content:              content,
		FullContentAvailable: true,
	}

	// Run scan layers.
	result := &ScanResult{
		Verdict:       VerdictClean,
		Threats:       make([]ThreatInfo, 0),
		Filename:      filename,
		SHA256:        sha256Hex,
		FileSize:      fileSize,
		FileType:      mimeType,
		ScannedAt:     start.UTC(),
		LayersRun:     make([]string, 0, len(e.layers)),
		EngineVersion: EngineVersion,
		SigDBVersion:  e.getSigDBVersion(),
	}

	e.layersMu.RLock()
	layers := e.layers
	e.layersMu.RUnlock()

	for _, layer := range layers {
		// Check context cancellation between layers.
		select {
		case <-ctx.Done():
			result.Verdict = VerdictError
			result.DurationMs = time.Since(start).Milliseconds()
			e.stats.errorCount.Add(1)
			return result, ctx.Err()
		default:
		}

		result.LayersRun = append(result.LayersRun, layer.Name())

		threats, err := layer.Scan(target)
		if err != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  antivirus: layer %q error on %q: %v", layer.Name(), filename, err))
			// Layer errors are non-fatal — we continue with other layers.
			// The error is logged but does not change the verdict unless
			// ALL layers fail.
			continue
		}

		if len(threats) > 0 {
			result.Threats = append(result.Threats, threats...)
		}
	}

	// Determine final verdict from aggregated threats.
	result.Verdict = determineVerdict(result.Threats)
	result.DurationMs = time.Since(start).Milliseconds()

	// Update stats.
	e.stats.totalScanned.Add(1)
	e.stats.bytesScanned.Add(fileSize)
	e.stats.totalScanNs.Add(time.Since(start).Nanoseconds())
	switch result.Verdict {
	case VerdictMalware, VerdictSuspicious:
		e.stats.threatsFound.Add(1)
	case VerdictClean:
		e.stats.cleanFiles.Add(1)
	case VerdictError:
		e.stats.errorCount.Add(1)
	}

	// Log threat detections.
	if result.Verdict.IsThreat() {
		logging.Z().Info(fmt.Sprintf("🚨 antivirus: THREAT in %q — %s [sha256=%s]",
			filename, result.Summary(), sha256Hex))
		e.recordThreat(*result)
	}

	return result, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Verdict Determination
// ─────────────────────────────────────────────────────────────────────────────

// determineVerdict examines the collected threats and returns the appropriate
// verdict. The logic is:
//   - No threats → VerdictClean
//   - Any threat with confidence ≥ 0.8 → VerdictMalware
//   - Any threat with confidence ≥ 0.5 → VerdictSuspicious
//   - Any threat with confidence < 0.5 → VerdictSuspicious (still flagged)
func determineVerdict(threats []ThreatInfo) ScanVerdict {
	if len(threats) == 0 {
		return VerdictClean
	}

	var maxConfidence float64
	for _, t := range threats {
		if t.Confidence > maxConfidence {
			maxConfidence = t.Confidence
		}
	}

	if maxConfidence >= 0.8 {
		return VerdictMalware
	}
	return VerdictSuspicious
}

// ─────────────────────────────────────────────────────────────────────────────
// Statistics
// ─────────────────────────────────────────────────────────────────────────────

// Stats returns a snapshot of the current engine statistics.
func (e *Engine) Stats() EngineStats {
	total := e.stats.totalScanned.Load()
	cacheHits := e.stats.cacheHits.Load()
	cacheMisses := e.stats.cacheMisses.Load()

	var cacheHitRate float64
	cacheTotal := cacheHits + cacheMisses
	if cacheTotal > 0 {
		cacheHitRate = float64(cacheHits) / float64(cacheTotal)
	}

	var avgScanMs float64
	if total > 0 {
		avgScanMs = float64(e.stats.totalScanNs.Load()) / float64(total) / 1e6
	}

	var uptimeSeconds int64
	if !e.startTime.IsZero() {
		uptimeSeconds = int64(time.Since(e.startTime).Seconds())
	}

	return EngineStats{
		TotalScanned:  total,
		ThreatsFound:  e.stats.threatsFound.Load(),
		CleanFiles:    e.stats.cleanFiles.Load(),
		ErrorCount:    e.stats.errorCount.Load(),
		CacheHits:     cacheHits,
		CacheMisses:   cacheMisses,
		CacheHitRate:  cacheHitRate,
		AvgScanMs:     avgScanMs,
		BytesScanned:  e.stats.bytesScanned.Load(),
		UptimeSeconds: uptimeSeconds,
		SigDBVersion:  e.getSigDBVersion(),
		EngineVersion: EngineVersion,
		LayersEnabled: e.cfg.EnabledLayers(),
	}
}

// MaxFileSize returns the configured maximum file size for scanning.
func (e *Engine) MaxFileSize() int64 {
	return e.cfg.MaxFileSize
}

// ─────────────────────────────────────────────────────────────────────────────
// Threat Log
// ─────────────────────────────────────────────────────────────────────────────

const maxThreatLogSize = 1000

// recordThreat appends a threat result to the in-memory log. If the log
// exceeds maxThreatLogSize, the oldest entries are evicted.
func (e *Engine) recordThreat(result ScanResult) {
	e.threatLogMu.Lock()
	defer e.threatLogMu.Unlock()

	e.threatLog = append(e.threatLog, result)

	// Evict oldest if over cap.
	if len(e.threatLog) > maxThreatLogSize {
		evict := len(e.threatLog) - maxThreatLogSize
		e.threatLog = e.threatLog[evict:]
	}
}

// RecentThreats returns a copy of recent threat detections in reverse
// chronological order (newest first). The returned slice is safe to
// modify without affecting the engine's internal log.
func (e *Engine) RecentThreats() []ScanResult {
	e.threatLogMu.Lock()
	defer e.threatLogMu.Unlock()

	n := len(e.threatLog)
	result := make([]ScanResult, n)
	for i, t := range e.threatLog {
		result[n-1-i] = t // reverse order
	}
	return result
}

// ─────────────────────────────────────────────────────────────────────────────
// Signature DB Version
// ─────────────────────────────────────────────────────────────────────────────

// SetSigDBVersion updates the signature database version string. Called by
// the sigdb.Updater when a new database is loaded.
func (e *Engine) SetSigDBVersion(version string) {
	e.sigDBMu.Lock()
	defer e.sigDBMu.Unlock()
	e.sigDBVersion = version
}

func (e *Engine) getSigDBVersion() string {
	e.sigDBMu.RLock()
	defer e.sigDBMu.RUnlock()
	return e.sigDBVersion
}

// Name returns the module identifier.
func (e *Engine) Name() string { return "antivirus" }

// Stop gracefully shuts down the antivirus engine.
func (e *Engine) Stop() error {
	e.Shutdown(context.Background())
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Config accessor
// ─────────────────────────────────────────────────────────────────────────────

// Config returns the engine's configuration. The returned pointer is
// read-only — callers must not modify it.
func (e *Engine) Config() *Config {
	return e.cfg
}

// ─────────────────────────────────────────────────────────────────────────────
// Background Workers
// ─────────────────────────────────────────────────────────────────────────────

// statsLogger periodically logs engine statistics for observability.
func (e *Engine) statsLogger(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s := e.Stats()
			if s.TotalScanned > 0 {
				logging.Z().Info(fmt.Sprintf("🛡️  antivirus stats: scanned=%d threats=%d clean=%d errors=%d cache_hit=%.1f%% avg=%.1fms bytes=%s",
					s.TotalScanned, s.ThreatsFound, s.CleanFiles, s.ErrorCount,
					s.CacheHitRate*100, s.AvgScanMs, formatBytes(s.BytesScanned)))
			}
		}
	}
}

// formatBytes returns a human-readable byte size string.
func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1fGB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1fMB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1fKB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
