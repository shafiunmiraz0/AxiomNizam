package scanner

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Scanner Interface
// ─────────────────────────────────────────────────────────────────────────────

// Scanner is the interface that all individual scanners in the SafeGate
// pipeline must implement. Each scanner inspects a file and returns zero
// or more findings.
//
// The ctx parameter enables:
//   - Deadline propagation from the orchestrator's global timeout.
//   - Caller-initiated cancellation (e.g. HTTP request cancelled).
//   - Per-scanner timeout enforcement.
//
// Implementations:
//   - MetadataScanner  — file size, empty files, null bytes, suspicious filenames
//   - MIMEScanner      — content-type validation, type spoofing, executable detection
//   - SVGScanner       — XSS vectors in SVG files (script, event handlers, JS URIs)
//   - MacroScanner     — VBA macros, auto-exec, shell commands in Office/PDF files
//   - ArchiveScanner   — zip bombs, path traversal, executable entries in archives
//   - NativeAVScanner  — malware detection via internal antivirus engine
type Scanner interface {
	// Name returns a unique, stable identifier for this scanner.
	Name() string

	// Scan inspects the file and returns any findings.
	// Implementations must be safe to call concurrently.
	// Return (nil, nil) to indicate "no findings, no error".
	// Respect ctx.Done() for cancellation and deadline propagation.
	Scan(ctx context.Context, file *FileInfo) ([]Finding, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// Orchestrator
// ─────────────────────────────────────────────────────────────────────────────

// Orchestrator runs all registered scanners against a file and aggregates
// their findings into a single ScanResult.
//
// Behavior:
//   - When Config.Parallel is true (default), scanners run concurrently.
//     Findings are collected thread-safely and merged in registration order.
//   - When Config.Parallel is false, scanners run sequentially in
//     registration order (useful for debugging and deterministic output).
//   - Config.Timeout is enforced as a context deadline on the entire scan.
//   - If a scanner returns an error, an info-level finding is recorded and
//     other scanners continue — the pipeline never aborts.
//   - If a scanner's context is cancelled (timeout), a timed-out finding
//     is recorded for that scanner.
//   - After all scanners complete, the result is marked as unsafe if any
//     finding has severity ≥ medium (critical, high, or medium).
//   - Per-scanner timing is recorded in ScanResult.Timings.
type Orchestrator struct {
	scanners []Scanner
	config   Config
	configMu sync.RWMutex // protects config for hot-reload
	metrics  *Metrics
}

// NewOrchestrator creates an orchestrator with the given scanners.
// Uses DefaultConfig(). For custom configuration, use NewOrchestratorWithConfig.
func NewOrchestrator(scanners ...Scanner) *Orchestrator {
	return &Orchestrator{
		scanners: scanners,
		config:   DefaultConfig(),
		metrics:  NewMetrics(),
	}
}

// NewOrchestratorWithConfig creates an orchestrator with the given scanners
// and explicit configuration.
func NewOrchestratorWithConfig(cfg Config, scanners ...Scanner) *Orchestrator {
	return &Orchestrator{
		scanners: scanners,
		config:   cfg,
		metrics:  NewMetrics(),
	}
}

// Config returns the orchestrator's active configuration.
func (o *Orchestrator) Config() Config {
	o.configMu.RLock()
	defer o.configMu.RUnlock()
	return o.config
}

// UpdateConfig atomically replaces the orchestrator's configuration.
func (o *Orchestrator) UpdateConfig(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	o.configMu.Lock()
	o.config = cfg
	o.configMu.Unlock()
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Scan Methods
// ─────────────────────────────────────────────────────────────────────────────

// Scan runs all registered scanners and returns aggregated results.
// Uses an internal context with Config.Timeout as the deadline.
// This is the backward-compatible entry point.
func (o *Orchestrator) Scan(file *FileInfo) *ScanResult {
	o.configMu.RLock()
	cfg := o.config
	o.configMu.RUnlock()

	ctx := context.Background()
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}
	return o.ScanWithContext(ctx, file)
}

// ScanWithContext runs all registered scanners with the provided context.
// If Config.Timeout is shorter than the context's existing deadline,
// Config.Timeout takes precedence.
func (o *Orchestrator) ScanWithContext(ctx context.Context, file *FileInfo) *ScanResult {
	start := time.Now()

	o.configMu.RLock()
	cfg := o.config
	o.configMu.RUnlock()

	// Apply config timeout if shorter than existing deadline.
	if cfg.Timeout > 0 {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > cfg.Timeout {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
			defer cancel()
		}
	}

	result := &ScanResult{
		Safe:      true,
		Filename:  file.Filename,
		FileSize:  file.Size,
		MIMEType:  file.MIMEType,
		SHA256:    file.SHA256,
		ScannedAt: start.UTC(),
		Findings:  make([]Finding, 0),
		Scanners:  make([]string, 0, len(o.scanners)),
		Timings:   make([]ScannerTiming, len(o.scanners)),
	}

	// Record scanner names upfront (preserves registration order).
	for i, s := range o.scanners {
		result.Scanners = append(result.Scanners, s.Name())
		result.Timings[i].Scanner = s.Name()
	}

	if cfg.Parallel && len(o.scanners) > 1 {
		o.scanParallel(ctx, file, result)
	} else {
		o.scanSequential(ctx, file, result)
	}

	// Determine safety: any medium+ finding makes the result unsafe.
	for _, f := range result.Findings {
		if f.Severity.IsThreat() {
			result.Safe = false
			break
		}
	}

	result.DurationMs = time.Since(start).Milliseconds()

	// Record metrics for observability.
	o.metrics.Record(result)

	return result
}

// ─────────────────────────────────────────────────────────────────────────────
// Sequential Execution
// ─────────────────────────────────────────────────────────────────────────────

func (o *Orchestrator) scanSequential(ctx context.Context, file *FileInfo, result *ScanResult) {
	for i, s := range o.scanners {
		scanStart := time.Now()
		findings, err := s.Scan(ctx, file)
		elapsed := time.Since(scanStart).Milliseconds()

		result.Timings[i].DurationMs = elapsed

		if err != nil {
			timedOut := ctx.Err() == context.DeadlineExceeded
			result.Timings[i].Error = true
			result.Timings[i].TimedOut = timedOut
			detail := fmt.Sprintf("Scanner %q could not complete: %v", s.Name(), err)
			if timedOut {
				detail = fmt.Sprintf("Scanner %q timed out after %dms", s.Name(), elapsed)
			}
			result.Findings = append(result.Findings, Finding{
				Scanner:     s.Name(),
				Severity:    SeverityInfo,
				Description: "Scanner unavailable",
				Details:     detail,
			})
			result.Timings[i].FindingCount = 1
			if ctx.Err() != nil {
				break
			}
			continue
		}

		result.Timings[i].FindingCount = len(findings)
		result.Findings = append(result.Findings, findings...)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Parallel Execution
// ─────────────────────────────────────────────────────────────────────────────

// scannerOutput holds the output of a single scanner execution.
type scannerOutput struct {
	index    int
	findings []Finding
	err      error
	elapsed  int64
}

func (o *Orchestrator) scanParallel(ctx context.Context, file *FileInfo, result *ScanResult) {
	outputs := make([]scannerOutput, len(o.scanners))
	var wg sync.WaitGroup

	for i, s := range o.scanners {
		wg.Add(1)
		go func(idx int, sc Scanner) {
			defer wg.Done()
			scanStart := time.Now()
			findings, err := sc.Scan(ctx, file)
			outputs[idx] = scannerOutput{
				index:    idx,
				findings: findings,
				err:      err,
				elapsed:  time.Since(scanStart).Milliseconds(),
			}
		}(i, s)
	}

	wg.Wait()

	// Merge in registration order for deterministic output.
	for i, out := range outputs {
		result.Timings[i].DurationMs = out.elapsed

		if out.err != nil {
			timedOut := ctx.Err() == context.DeadlineExceeded
			result.Timings[i].Error = true
			result.Timings[i].TimedOut = timedOut
			detail := fmt.Sprintf("Scanner %q could not complete: %v", o.scanners[i].Name(), out.err)
			if timedOut {
				detail = fmt.Sprintf("Scanner %q timed out after %dms", o.scanners[i].Name(), out.elapsed)
			}
			result.Findings = append(result.Findings, Finding{
				Scanner:     o.scanners[i].Name(),
				Severity:    SeverityInfo,
				Description: "Scanner unavailable",
				Details:     detail,
			})
			result.Timings[i].FindingCount = 1
			continue
		}

		result.Timings[i].FindingCount = len(out.findings)
		result.Findings = append(result.Findings, out.findings...)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Accessors
// ─────────────────────────────────────────────────────────────────────────────

// ScannerNames returns the names of all registered scanners.
func (o *Orchestrator) ScannerNames() []string {
	names := make([]string, len(o.scanners))
	for i, s := range o.scanners {
		names[i] = s.Name()
	}
	return names
}

// ScannerCount returns the number of registered scanners.
func (o *Orchestrator) ScannerCount() int {
	return len(o.scanners)
}

// Metrics returns the orchestrator's metrics collector.
// Use Metrics().Snapshot() to obtain a serializable copy.
func (o *Orchestrator) Metrics() *Metrics {
	return o.metrics
}

// Health returns the current health status of the scanner pipeline.
// Pass includeMetrics=true to embed the full MetricsSnapshot in the response.
func (o *Orchestrator) Health(includeMetrics bool) HealthStatus {
	snap := o.metrics.Snapshot()

	// Determine status based on error rate.
	status := "healthy"
	errorRate := "0.0%"
	if snap.TotalScans > 0 {
		var totalErrors int64
		for _, sm := range snap.Scanners {
			totalErrors += sm.TotalErrors
		}
		totalRuns := snap.TotalScans * int64(len(o.scanners))
		if totalRuns > 0 {
			rate := float64(totalErrors) / float64(totalRuns) * 100
			errorRate = fmt.Sprintf("%.1f%%", rate)
			if rate > 50 {
				status = "degraded"
			}
			if rate > 90 {
				status = "unavailable"
			}
		}
	}

	if len(o.scanners) == 0 {
		status = "unavailable"
	}

	h := HealthStatus{
		Status:        status,
		ScannerCount:  len(o.scanners),
		Scanners:      o.ScannerNames(),
		TotalScans:    snap.TotalScans,
		UptimeSeconds: snap.UptimeSeconds,
		LastScanAt:    snap.LastScanAt,
		ErrorRate:     errorRate,
	}

	if includeMetrics {
		h.Metrics = &snap
	}

	return h
}

// Name returns the module identifier.
func (o *Orchestrator) Name() string { return "scanner" }

// Start is a no-op — the scanner is stateless and requires no background workers.
func (o *Orchestrator) Start(ctx context.Context) error { return nil }

// Stop is a no-op — the scanner has no resources to release.
func (o *Orchestrator) Stop() error { return nil }
