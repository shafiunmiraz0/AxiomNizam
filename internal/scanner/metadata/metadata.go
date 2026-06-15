// Package metadata provides the MetadataScanner for the SafeGate pipeline.
// It checks file size limits, empty files, null byte injection in text files,
// suspicious filename patterns (excessive length, double extensions, path
// traversal), and dangerous Unicode/control characters in filenames.
package metadata

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
)

// Scanner checks file size, empty files, null bytes, control characters,
// and suspicious filenames including path traversal attempts.
type Scanner struct {
	maxFileSize        int64
	nullByteSampleSize int
	maxFilenameLength  int
}

// NewScanner creates a MetadataScanner with the given maximum file size.
// Uses default values for sample size (8192) and filename length (255).
func NewScanner(maxFileSize int64) *Scanner {
	return &Scanner{
		maxFileSize:        maxFileSize,
		nullByteSampleSize: 8192,
		maxFilenameLength:  255,
	}
}

// NewScannerWithConfig creates a MetadataScanner from full configuration.
func NewScannerWithConfig(maxFileSize int64, nullByteSampleSize, maxFilenameLength int) *Scanner {
	return &Scanner{
		maxFileSize:        maxFileSize,
		nullByteSampleSize: nullByteSampleSize,
		maxFilenameLength:  maxFilenameLength,
	}
}

func (s *Scanner) Name() string { return "metadata_scanner" }

func (s *Scanner) Scan(_ context.Context, file *scanner.FileInfo) ([]scanner.Finding, error) {
	var findings []scanner.Finding

	// ── File size checks ─────────────────────────────────────────────────
	if file.Size > s.maxFileSize {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityHigh,
			Description: "File exceeds maximum allowed size",
			Details:     fmt.Sprintf("Size %d bytes exceeds %d byte limit", file.Size, s.maxFileSize),
		})
	}

	if file.Size == 0 {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityLow,
			Description: "Empty file uploaded", Details: "File has zero bytes",
		})
	}

	// ── Null byte detection (sample-based for efficiency) ────────────────
	if isTextFile(file) {
		findings = append(findings, s.checkNullBytes(file.Content)...)
	}

	// ── Filename analysis ────────────────────────────────────────────────
	findings = append(findings, s.checkFilename(file.Filename)...)

	return findings, nil
}

// checkNullBytes detects null bytes in text file content using sample-based
// scanning for efficiency. If nullByteSampleSize is 0, scans entire file.
func (s *Scanner) checkNullBytes(content []byte) []scanner.Finding {
	sample := content
	if s.nullByteSampleSize > 0 && len(content) > s.nullByteSampleSize {
		sample = content[:s.nullByteSampleSize]
	}

	nullCount := 0
	for _, b := range sample {
		if b == 0x00 {
			nullCount++
		}
	}
	if nullCount == 0 {
		return nil
	}

	detail := fmt.Sprintf("Found %d null bytes", nullCount)
	if len(sample) < len(content) {
		detail += fmt.Sprintf(" in first %d bytes (sampled)", s.nullByteSampleSize)
	}
	detail += " — may indicate binary injection"

	return []scanner.Finding{{
		Scanner: s.Name(), Severity: scanner.SeverityMedium,
		Description: "Text file contains null bytes",
		Details:     detail,
	}}
}

// checkFilename performs comprehensive filename analysis:
// - Length limits
// - Double/multiple extension spoofing
// - Path traversal sequences
// - Dangerous Unicode and control characters
// - Null bytes in filename
func (s *Scanner) checkFilename(name string) []scanner.Finding {
	var findings []scanner.Finding

	// ── Length check ─────────────────────────────────────────────────────
	if len(name) > s.maxFilenameLength {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityMedium,
			Description: "Filename exceeds maximum length",
			Details:     fmt.Sprintf("Filename is %d characters (max %d)", len(name), s.maxFilenameLength),
		})
	}

	// ── Double extensions: e.g. "report.pdf.exe" ─────────────────────────
	dots := 0
	for _, c := range name {
		if c == '.' {
			dots++
		}
	}
	if dots > 2 {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityMedium,
			Description: "File has many extensions",
			Details:     fmt.Sprintf("Filename %q has %d dots — possible extension spoofing", name, dots),
		})
	}

	// ── Path traversal detection ─────────────────────────────────────────
	if containsPathTraversal(name) {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityCritical,
			Description: "Filename contains path traversal sequence",
			Details:     fmt.Sprintf("Filename %q contains directory traversal — file write attack", name),
		})
	}

	// ── Unicode / control character detection ────────────────────────────
	if issues := checkUnicodeIssues(name); len(issues) > 0 {
		for _, issue := range issues {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: issue.severity,
				Description: issue.desc,
				Details:     issue.details,
			})
		}
	}

	// ── Null bytes in filename ────────────────────────────────────────────
	if strings.ContainsRune(name, 0x00) {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityCritical,
			Description: "Filename contains null byte",
			Details:     "Null bytes in filenames can truncate paths and bypass extension checks",
		})
	}

	return findings
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper functions
// ─────────────────────────────────────────────────────────────────────────────

func isTextFile(file *scanner.FileInfo) bool {
	switch file.Extension {
	case ".txt", ".csv", ".json", ".xml", ".svg", ".html", ".htm",
		".css", ".js", ".ts", ".md", ".yaml", ".yml", ".toml", ".ini",
		".log", ".sh", ".bat", ".ps1", ".py", ".go", ".rs":
		return true
	}
	return false
}

// containsPathTraversal checks for directory traversal patterns.
func containsPathTraversal(name string) bool {
	// Normalize separators for cross-platform detection.
	normalized := strings.ReplaceAll(name, "\\", "/")

	// Direct traversal sequences
	if strings.Contains(normalized, "../") || strings.Contains(normalized, "/..") {
		return true
	}
	// Starts with absolute path
	if strings.HasPrefix(normalized, "/") {
		return true
	}
	// Windows absolute paths (C:\, D:\)
	if len(name) >= 3 && name[1] == ':' && (name[2] == '\\' || name[2] == '/') {
		return true
	}
	// Bare ".." component
	if name == ".." || strings.HasPrefix(name, "..") {
		return true
	}

	return false
}

type unicodeIssue struct {
	severity scanner.Severity
	desc     string
	details  string
}

// checkUnicodeIssues detects dangerous Unicode patterns in filenames.
func checkUnicodeIssues(name string) []unicodeIssue {
	var issues []unicodeIssue

	hasControl := false
	hasBidi := false
	hasHomoglyph := false

	for _, r := range name {
		// Control characters (except common whitespace)
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' && r != 0x00 {
			hasControl = true
		}

		// Bidirectional override characters (used to reverse displayed filename)
		// These can make "exe.doc" display as "cod.exe"
		if r == 0x202A || r == 0x202B || r == 0x202C || r == 0x202D || r == 0x202E ||
			r == 0x2066 || r == 0x2067 || r == 0x2068 || r == 0x2069 {
			hasBidi = true
		}

		// Zero-width characters (can hide content or bypass filters)
		if r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF {
			hasHomoglyph = true
		}
	}

	if hasBidi {
		issues = append(issues, unicodeIssue{
			severity: scanner.SeverityCritical,
			desc:     "Filename contains bidirectional override characters",
			details:  "Bidi overrides (U+202A-E, U+2066-9) can reverse displayed text — an attacker can make 'exe.doc' appear as 'cod.exe'",
		})
	}

	if hasControl {
		issues = append(issues, unicodeIssue{
			severity: scanner.SeverityHigh,
			desc:     "Filename contains control characters",
			details:  "Control characters in filenames can cause parsing issues and may indicate an attack",
		})
	}

	if hasHomoglyph {
		issues = append(issues, unicodeIssue{
			severity: scanner.SeverityMedium,
			desc:     "Filename contains zero-width characters",
			details:  "Zero-width spaces/joiners (U+200B-D, U+FEFF) can hide content and bypass filename filters",
		})
	}

	return issues
}
