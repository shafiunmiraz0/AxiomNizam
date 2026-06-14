// Package native provides the NativeAVScanner bridge for the SafeGate pipeline.
// It wraps the internal antivirus.Engine as a scanner.Scanner implementation,
// translating antivirus threats into scanner.Finding results.
package native

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
)

// Scanner wraps the internal antivirus.Engine as a scanner.Scanner
// implementation. This is the drop-in replacement for ClamAVScanner in
// the SafeGate scanner orchestrator pipeline.
type Scanner struct {
	engine *antivirus.Engine
}

// NewScanner creates a scanner.Scanner backed by the internal
// antivirus engine. If engine is nil, all scans return clean (no-op).
func NewScanner(engine *antivirus.Engine) *Scanner {
	return &Scanner{engine: engine}
}

func (s *Scanner) Name() string { return "native_antivirus" }

func (s *Scanner) Scan(ctx context.Context, file *scanner.FileInfo) ([]scanner.Finding, error) {
	if s.engine == nil || !s.engine.IsRunning() {
		return nil, nil // engine not available — skip
	}

	// Safety-net timeout when caller provides no deadline.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
	}

	result, err := s.engine.Scan(ctx, file.Content, file.Filename)
	if err != nil {
		return nil, fmt.Errorf("native antivirus scan failed: %w", err)
	}

	var findings []scanner.Finding
	if result.Verdict.IsThreat() {
		for _, t := range result.Threats {
			sev := scanner.SeverityMedium
			switch t.Severity {
			case antivirus.SeverityCritical:
				sev = scanner.SeverityCritical
			case antivirus.SeverityHigh:
				sev = scanner.SeverityHigh
			case antivirus.SeverityMedium:
				sev = scanner.SeverityMedium
			case antivirus.SeverityLow:
				sev = scanner.SeverityLow
			}

			findings = append(findings, scanner.Finding{
				Scanner:     s.Name(),
				Severity:    sev,
				Description: fmt.Sprintf("Malware detected: %s", t.Name),
				Details:     fmt.Sprintf("[%s] %s — category=%s, confidence=%.2f, sha256=%s", t.Severity, t.Description, t.Category, t.Confidence, result.SHA256),
			})
		}
	}

	return findings, nil
}
