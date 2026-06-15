// Package entropy implements Layer 5 of the AxiomNizam antivirus engine:
// Shannon entropy analysis for detecting packed, encrypted, or compressed
// malware.
//
// # Theory
//
// Shannon entropy measures the information density of data, expressed in
// bits per byte. The theoretical range is [0.0, 8.0]:
//
//   - 0.0 — perfectly uniform (e.g. all zeros)
//   - ~4.5 — typical English text
//   - ~5.5 — typical source code
//   - ~6.5 — compiled native code (.text section)
//   - ~7.0 — JPEG/PNG images (already compressed)
//   - ~7.5 — ZIP/GZIP archives
//   - ~7.9 — AES-encrypted or random data
//
// Packed/encrypted malware (UPX, Themida, custom XOR stubs) produces
// uniformly high entropy (>7.5) across the entire file. Legitimate
// encrypted files (ZIP, HTTPS payloads) also have high entropy, but the
// entropy distribution pattern differs:
//
//   - Packed malware: uniformly high entropy with a small low-entropy stub
//   - Legitimate archives: mixed entropy (headers + compressed streams)
//   - Legitimate executables: varied entropy per section
//
// This layer uses **windowed entropy analysis** — scanning the file in
// fixed-size windows and building an entropy profile — to distinguish
// between these cases.
package entropy

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"bytes"
	"fmt"
	"math"
	"sync/atomic"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Configuration
// ─────────────────────────────────────────────────────────────────────────────

const (
	// windowSize is the size of each entropy analysis window (bytes).
	// 256 bytes gives fine-grained entropy distribution without excessive
	// computation.
	windowSize = 256

	// wholeFileHighEntropyThreshold triggers a finding when the entire
	// file's entropy exceeds this value. Only applies to executable and
	// script MIME types — archives/images are expected to be high-entropy.
	wholeFileHighEntropyThreshold = 7.5

	// windowHighEntropyThreshold is the per-window threshold for counting
	// high-entropy windows. Set to 6.5 because 256-byte windows of truly
	// random data typically produce entropy of ~6.5-7.2 due to the small
	// sample size effect. This is still well above normal compiled code
	// (~5.5) and text (~4.5).
	windowHighEntropyThreshold = 6.5

	// uniformHighEntropyRatio — if this fraction of windows exceeds the
	// high-entropy threshold, the file is flagged as "uniformly packed".
	uniformHighEntropyRatio = 0.80

	// lowEntropyStubMaxWindows — packed malware typically has a small
	// low-entropy loader stub followed by a high-entropy payload. If
	// the number of low-entropy windows is fewer than this, it's a
	// packing indicator.
	lowEntropyStubMaxWindows = 3

	// minAnalysisSize — skip entropy analysis for very small files.
	minAnalysisSize = 512
)

// ─────────────────────────────────────────────────────────────────────────────
// Entropy Profile
// ─────────────────────────────────────────────────────────────────────────────

// Profile holds the complete entropy analysis of a file.
type Profile struct {
	// WholeFileEntropy is the Shannon entropy of the entire file content.
	WholeFileEntropy float64 `json:"wholeFileEntropy"`

	// WindowEntropies holds the entropy of each fixed-size window.
	WindowEntropies []float64 `json:"-"`

	// HighEntropyWindows is the count of windows exceeding the threshold.
	HighEntropyWindows int `json:"highEntropyWindows"`

	// TotalWindows is the total number of analysis windows.
	TotalWindows int `json:"totalWindows"`

	// HighEntropyRatio is HighEntropyWindows / TotalWindows.
	HighEntropyRatio float64 `json:"highEntropyRatio"`

	// MeanWindowEntropy is the arithmetic mean of all window entropies.
	MeanWindowEntropy float64 `json:"meanWindowEntropy"`

	// EntropyStdDev is the standard deviation of window entropies.
	// Low stddev + high mean = uniformly packed data.
	EntropyStdDev float64 `json:"entropyStdDev"`

	// LowEntropyWindows is the count of windows with entropy < 4.0.
	LowEntropyWindows int `json:"lowEntropyWindows"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Shannon Entropy Calculation
// ─────────────────────────────────────────────────────────────────────────────

// Shannon calculates the Shannon entropy of data in bits per byte.
// Returns [0.0, 8.0]. Empty data returns 0.
func Shannon(data []byte) float64 {
	n := len(data)
	if n == 0 {
		return 0
	}

	var freq [256]int
	for _, b := range data {
		freq[b]++
	}

	total := float64(n)
	var entropy float64
	for _, count := range freq {
		if count > 0 {
			p := float64(count) / total
			entropy -= p * math.Log2(p)
		}
	}
	return entropy
}

// ─────────────────────────────────────────────────────────────────────────────
// Profiling
// ─────────────────────────────────────────────────────────────────────────────

// Analyze computes a full entropy profile for the given data.
func Analyze(data []byte) Profile {
	p := Profile{
		WholeFileEntropy: Shannon(data),
	}

	if len(data) < windowSize {
		p.TotalWindows = 1
		p.WindowEntropies = []float64{p.WholeFileEntropy}
		p.MeanWindowEntropy = p.WholeFileEntropy
		if p.WholeFileEntropy > windowHighEntropyThreshold {
			p.HighEntropyWindows = 1
		}
		if p.WholeFileEntropy < 4.0 {
			p.LowEntropyWindows = 1
		}
		p.HighEntropyRatio = float64(p.HighEntropyWindows) / float64(p.TotalWindows)
		return p
	}

	numWindows := len(data) / windowSize
	p.WindowEntropies = make([]float64, 0, numWindows)

	var sum float64
	for i := 0; i < numWindows; i++ {
		start := i * windowSize
		end := start + windowSize
		if end > len(data) {
			end = len(data)
		}
		w := data[start:end]
		if len(w) < 64 {
			continue // skip very small trailing windows
		}

		ent := Shannon(w)
		p.WindowEntropies = append(p.WindowEntropies, ent)
		sum += ent

		if ent > windowHighEntropyThreshold {
			p.HighEntropyWindows++
		}
		if ent < 4.0 {
			p.LowEntropyWindows++
		}
	}

	p.TotalWindows = len(p.WindowEntropies)
	if p.TotalWindows > 0 {
		p.MeanWindowEntropy = sum / float64(p.TotalWindows)
		p.HighEntropyRatio = float64(p.HighEntropyWindows) / float64(p.TotalWindows)

		// Standard deviation.
		var variance float64
		for _, ent := range p.WindowEntropies {
			diff := ent - p.MeanWindowEntropy
			variance += diff * diff
		}
		p.EntropyStdDev = math.Sqrt(variance / float64(p.TotalWindows))
	}

	return p
}

// ─────────────────────────────────────────────────────────────────────────────
// MIME type classification
// ─────────────────────────────────────────────────────────────────────────────

// isExpectedHighEntropy returns true for MIME types that are naturally
// high-entropy (compressed, encrypted, media). We don't flag these.
func isExpectedHighEntropy(mime string) bool {
	highEntropyPrefixes := []string{
		"image/jpeg", "image/png", "image/webp", "image/gif",
		"audio/", "video/",
		"application/zip", "application/gzip", "application/x-7z",
		"application/x-rar", "application/x-tar",
		"application/pdf",
		"application/x-bzip",
		"application/octet-stream", // too generic to judge
	}
	for _, prefix := range highEntropyPrefixes {
		if len(mime) >= len(prefix) && mime[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// isExecutableMIME returns true for file types where high entropy is
// suspicious (executables, scripts, documents).
func isExecutableMIME(mime string) bool {
	suspects := []string{
		"application/x-executable", "application/x-dosexec",
		"application/x-msdos-program", "application/x-elf",
		"application/x-mach-binary",
		"text/x-script", "text/x-shellscript",
		"application/javascript", "text/javascript",
	}
	for _, s := range suspects {
		if mime == s {
			return true
		}
	}
	return false
}

// isExecutableByMagic checks the file header for PE/ELF magic bytes.
func isExecutableByMagic(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	if data[0] == 0x4D && data[1] == 0x5A { // MZ (PE)
		return true
	}
	if bytes.HasPrefix(data, []byte{0x7F, 0x45, 0x4C, 0x46}) { // ELF
		return true
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// Layer — ScanLayer implementation
// ─────────────────────────────────────────────────────────────────────────────

// Layer implements antivirus.ScanLayer for entropy-based detection.
type Layer struct {
	scans    atomic.Int64
	findings atomic.Int64
}

// New creates a new entropy analysis layer.
func New() *Layer {
	return &Layer{}
}

// Name returns the layer identifier.
func (l *Layer) Name() string { return "entropy" }

// Scan performs entropy analysis on the target file.
func (l *Layer) Scan(target *antivirus.ScanTarget) ([]antivirus.ThreatInfo, error) {
	if len(target.Content) < minAnalysisSize {
		return nil, nil
	}

	l.scans.Add(1)

	profile := Analyze(target.Content)
	var threats []antivirus.ThreatInfo

	// Check for known packer magic bytes (regardless of entropy).
	if packerThreats := detectPackers(target.Content); len(packerThreats) > 0 {
		threats = append(threats, packerThreats...)
	}

	// Skip entropy flagging for naturally high-entropy formats.
	if isExpectedHighEntropy(target.MIMEType) {
		if len(threats) > 0 {
			l.findings.Add(int64(len(threats)))
			logging.Z().Info(fmt.Sprintf("🛡️  entropy: %d finding(s) in %q (packer only, high-entropy MIME skipped)",
				len(threats), target.Filename))
		}
		return threats, nil
	}

	isExec := isExecutableMIME(target.MIMEType) || isExecutableByMagic(target.Content)

	// 1. Whole-file high entropy for executables.
	if isExec && profile.WholeFileEntropy > wholeFileHighEntropyThreshold {
		threats = append(threats, antivirus.ThreatInfo{
			Name:        "Entropy.HighOverall",
			Category:    antivirus.CategoryPacker,
			Severity:    antivirus.SeverityMedium,
			Layer:       antivirus.LayerEntropy,
			Description: fmt.Sprintf("Executable with high overall entropy (%.2f bits/byte) — likely packed or encrypted", profile.WholeFileEntropy),
			Confidence:  0.65,
			Metadata: map[string]string{
				"wholeFileEntropy": fmt.Sprintf("%.4f", profile.WholeFileEntropy),
			},
		})
	}

	// 2. Uniformly high entropy across windows — strong packing signal.
	if profile.TotalWindows >= 4 &&
		profile.HighEntropyRatio > uniformHighEntropyRatio &&
		profile.EntropyStdDev < 0.5 {

		confidence := 0.70
		if profile.HighEntropyRatio > 0.95 {
			confidence = 0.85
		}

		threats = append(threats, antivirus.ThreatInfo{
			Name:        "Entropy.UniformlyPacked",
			Category:    antivirus.CategoryPacker,
			Severity:    antivirus.SeverityHigh,
			Layer:       antivirus.LayerEntropy,
			Description: fmt.Sprintf("%.0f%% of file has entropy >%.1f with low variance (σ=%.2f) — uniformly packed/encrypted", profile.HighEntropyRatio*100, windowHighEntropyThreshold, profile.EntropyStdDev),
			Confidence:  confidence,
			Metadata: map[string]string{
				"highEntropyRatio": fmt.Sprintf("%.4f", profile.HighEntropyRatio),
				"stdDev":           fmt.Sprintf("%.4f", profile.EntropyStdDev),
				"meanEntropy":      fmt.Sprintf("%.4f", profile.MeanWindowEntropy),
			},
		})
	}

	// 3. Stub + payload pattern: few low-entropy windows + majority
	//    high-entropy = loader stub followed by packed payload.
	if isExec && profile.TotalWindows >= 8 &&
		profile.LowEntropyWindows > 0 &&
		profile.LowEntropyWindows <= lowEntropyStubMaxWindows &&
		profile.HighEntropyRatio > 0.70 {

		threats = append(threats, antivirus.ThreatInfo{
			Name:        "Entropy.StubPayloadPattern",
			Category:    antivirus.CategoryPacker,
			Severity:    antivirus.SeverityMedium,
			Layer:       antivirus.LayerEntropy,
			Description: fmt.Sprintf("Executable with %d low-entropy stub window(s) and %d%% high-entropy payload — packed executable pattern", profile.LowEntropyWindows, int(profile.HighEntropyRatio*100)),
			Confidence:  0.60,
			Metadata: map[string]string{
				"lowEntropyWindows":  fmt.Sprintf("%d", profile.LowEntropyWindows),
				"highEntropyWindows": fmt.Sprintf("%d", profile.HighEntropyWindows),
				"totalWindows":       fmt.Sprintf("%d", profile.TotalWindows),
			},
		})
	}

	if len(threats) > 0 {
		l.findings.Add(int64(len(threats)))
		logging.Z().Info(fmt.Sprintf("🛡️  entropy: %d finding(s) in %q (overall=%.2f, high-ratio=%.0f%%)",
			len(threats), target.Filename, profile.WholeFileEntropy, profile.HighEntropyRatio*100))
	}

	return threats, nil
}

// Stats returns runtime statistics.
type Stats struct {
	TotalScans    int64 `json:"totalScans"`
	TotalFindings int64 `json:"totalFindings"`
}

// Stats returns a snapshot of entropy layer statistics.
func (l *Layer) Stats() Stats {
	return Stats{
		TotalScans:    l.scans.Load(),
		TotalFindings: l.findings.Load(),
	}
}
