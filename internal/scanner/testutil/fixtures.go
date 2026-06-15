package testutil

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
)

// TestFilename is a fixed filename for testing.
const TestFilename = "test-file.txt"

// NewTestFileInfo creates a test FileInfo with sensible defaults.
func NewTestFileInfo() *scanner.FileInfo {
	return &scanner.FileInfo{
		Filename:  TestFilename,
		Size:      1024,
		MIMEType:  "text/plain",
		Extension: ".txt",
	}
}

// NewTestFileInfoLarge creates a test FileInfo with a large file size.
func NewTestFileInfoLarge() *scanner.FileInfo {
	return &scanner.FileInfo{
		Filename:  "large-file.bin",
		Size:      100 * 1024 * 1024, // 100MB
		MIMEType:  "application/octet-stream",
		Extension: ".bin",
	}
}

// NewTestFinding creates a test Finding with sensible defaults.
func NewTestFinding() scanner.Finding {
	return scanner.Finding{
		Scanner:     "test-scanner",
		Severity:    scanner.SeverityMedium,
		Description: "Test finding",
		Details:     "Test details for unit test",
		Category:    "test",
	}
}

// NewTestScanResult creates a clean test ScanResult.
func NewTestScanResult() scanner.ScanResult {
	return scanner.ScanResult{
		Safe:       true,
		Filename:   TestFilename,
		Findings:   nil,
		Timings:    nil,
		ScannedAt:  time.Now().UTC(),
		DurationMs: 10,
	}
}
