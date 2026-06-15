// Package heuristic implements Layer 3 of the AxiomNizam antivirus engine:
// behavioral heuristic analysis that detects malware by examining file
// structure and behavior patterns rather than relying on signatures.
//
// This layer provides zero-day detection capabilities — it can flag threats
// that have never been seen before, based on structural anomalies:
//
//   - PE (Windows executable) header analysis
//   - ELF (Linux executable) header analysis
//   - Script obfuscation detection (JS, PS1, VBS, PHP, Bash)
//   - Shellcode / NOP sled detection
//
// Each sub-analyzer produces findings with confidence scores. The Layer
// aggregates them into ThreatInfo results. All analysis is pure logic —
// no data structures, negligible RAM.
package heuristic

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"sync/atomic"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Finding — internal heuristic result
// ─────────────────────────────────────────────────────────────────────────────

// Finding represents a single heuristic detection from a sub-analyzer.
type Finding struct {
	Name        string
	Description string
	Category    antivirus.ThreatCategory
	Severity    antivirus.ThreatSeverity
	Confidence  float64
	Offset      int64
	Metadata    map[string]string
}

// ─────────────────────────────────────────────────────────────────────────────
// Analyzer — sub-analyzer interface
// ─────────────────────────────────────────────────────────────────────────────

// analyzer is a function that examines file content and returns findings.
type analyzer func(target *antivirus.ScanTarget) []Finding

// ─────────────────────────────────────────────────────────────────────────────
// Layer — ScanLayer implementation
// ─────────────────────────────────────────────────────────────────────────────

// Layer implements antivirus.ScanLayer for behavioral heuristic analysis.
type Layer struct {
	analyzers []namedAnalyzer
	scans     atomic.Int64
	findings  atomic.Int64
}

type namedAnalyzer struct {
	name string
	fn   analyzer
}

// New creates a new heuristic scan layer with all sub-analyzers enabled.
func New() *Layer {
	l := &Layer{}
	l.analyzers = []namedAnalyzer{
		{name: "pe", fn: analyzePE},
		{name: "elf", fn: analyzeELF},
		{name: "script", fn: analyzeScript},
		{name: "shellcode", fn: analyzeShellcode},
	}
	return l
}

// Name returns the layer identifier.
func (l *Layer) Name() string { return "heuristic" }

// Scan runs all sub-analyzers against the target and returns aggregated
// threats. Safe for concurrent use — all analyzers are pure functions.
func (l *Layer) Scan(target *antivirus.ScanTarget) ([]antivirus.ThreatInfo, error) {
	if len(target.Content) == 0 {
		return nil, nil
	}

	l.scans.Add(1)

	var allFindings []Finding
	for _, a := range l.analyzers {
		findings := a.fn(target)
		if len(findings) > 0 {
			allFindings = append(allFindings, findings...)
		}
	}

	if len(allFindings) == 0 {
		return nil, nil
	}

	l.findings.Add(int64(len(allFindings)))

	threats := make([]antivirus.ThreatInfo, 0, len(allFindings))
	for _, f := range allFindings {
		threats = append(threats, antivirus.ThreatInfo{
			Name:        f.Name,
			Category:    f.Category,
			Severity:    f.Severity,
			Layer:       antivirus.LayerHeuristic,
			Description: f.Description,
			Confidence:  f.Confidence,
			Offset:      f.Offset,
			Metadata:    f.Metadata,
		})
	}

	logging.Z().Info(fmt.Sprintf("🛡️  heuristic: %d finding(s) in %q", len(threats), target.Filename))
	return threats, nil
}

// Stats returns runtime statistics.
type Stats struct {
	TotalScans    int64 `json:"totalScans"`
	TotalFindings int64 `json:"totalFindings"`
}

// Stats returns a snapshot of heuristic layer statistics.
func (l *Layer) Stats() Stats {
	return Stats{
		TotalScans:    l.scans.Load(),
		TotalFindings: l.findings.Load(),
	}
}
