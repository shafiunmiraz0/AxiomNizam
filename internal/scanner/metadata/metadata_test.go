package metadata

import (
	"context"
	"strings"
	"testing"

	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
)

var ctx = context.Background()

// ─────────────────────────────────────────────────────────────────────────────
// Constructor tests
// ─────────────────────────────────────────────────────────────────────────────

func TestNewScanner(t *testing.T) {
	s := NewScanner(1024)
	if s.Name() != "metadata_scanner" {
		t.Errorf("expected name metadata_scanner, got %s", s.Name())
	}
	if s.maxFileSize != 1024 {
		t.Errorf("expected maxFileSize=1024, got %d", s.maxFileSize)
	}
	if s.nullByteSampleSize != 8192 {
		t.Errorf("expected default nullByteSampleSize=8192, got %d", s.nullByteSampleSize)
	}
}

func TestNewScannerWithConfig(t *testing.T) {
	s := NewScannerWithConfig(2048, 4096, 128)
	if s.maxFileSize != 2048 {
		t.Errorf("expected maxFileSize=2048, got %d", s.maxFileSize)
	}
	if s.nullByteSampleSize != 4096 {
		t.Errorf("expected nullByteSampleSize=4096, got %d", s.nullByteSampleSize)
	}
	if s.maxFilenameLength != 128 {
		t.Errorf("expected maxFilenameLength=128, got %d", s.maxFilenameLength)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// File size tests
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_FileSizeExceeded(t *testing.T) {
	s := NewScanner(100)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "big.txt", Size: 200, Content: make([]byte, 200),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "File exceeds maximum allowed size", scanner.SeverityHigh)
}

func TestScan_FileSizeWithinLimit(t *testing.T) {
	s := NewScanner(1000)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "ok.txt", Size: 500, Content: make([]byte, 500),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoFindingWith(t, findings, "File exceeds maximum allowed size")
}

func TestScan_EmptyFile(t *testing.T) {
	s := NewScanner(1000)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "empty.txt", Size: 0, Content: []byte{},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Empty file uploaded", scanner.SeverityLow)
}

// ─────────────────────────────────────────────────────────────────────────────
// Null byte detection tests
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_NullBytesInTextFile(t *testing.T) {
	s := NewScanner(10000)
	content := []byte("hello\x00world\x00test")
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "test.txt", Extension: ".txt", Size: int64(len(content)), Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Text file contains null bytes", scanner.SeverityMedium)
}

func TestScan_NullBytesInNonTextFile(t *testing.T) {
	s := NewScanner(10000)
	content := []byte("hello\x00world")
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "binary.bin", Extension: ".bin", Size: int64(len(content)), Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoFindingWith(t, findings, "Text file contains null bytes")
}

func TestScan_NullBytesSampling(t *testing.T) {
	s := NewScannerWithConfig(1000000, 10, 255) // sample only first 10 bytes
	// Null byte at position 5 (within sample)
	content := make([]byte, 100)
	content[5] = 0x00
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "test.csv", Extension: ".csv", Size: int64(len(content)), Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Text file contains null bytes", scanner.SeverityMedium)
}

func TestScan_NullBytesBeyondSample(t *testing.T) {
	s := NewScannerWithConfig(1000000, 10, 255) // sample only first 10 bytes
	// Fill with non-null data, then place null byte beyond sample window
	content := make([]byte, 100)
	for i := range content {
		content[i] = 'A'
	}
	content[50] = 0x00 // beyond 10-byte sample window
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "test.csv", Extension: ".csv", Size: int64(len(content)), Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoFindingWith(t, findings, "Text file contains null bytes")
}

func TestScan_NullBytesFullScan(t *testing.T) {
	s := NewScannerWithConfig(1000000, 0, 255) // sampleSize=0 → full scan
	content := make([]byte, 100)
	content[99] = 0x00
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "test.json", Extension: ".json", Size: int64(len(content)), Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Text file contains null bytes", scanner.SeverityMedium)
}

// ─────────────────────────────────────────────────────────────────────────────
// Filename tests — length and double extensions
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_FilenameTooLong(t *testing.T) {
	s := NewScannerWithConfig(10000, 8192, 50)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: strings.Repeat("a", 60) + ".txt", Size: 10, Content: []byte("hi"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Filename exceeds maximum length", scanner.SeverityMedium)
}

func TestScan_DoubleExtension(t *testing.T) {
	s := NewScanner(10000)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "report.pdf.exe.bat", Size: 10, Content: []byte("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "File has many extensions", scanner.SeverityMedium)
}

func TestScan_NormalFilename(t *testing.T) {
	s := NewScanner(10000)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "report.pdf", Size: 10, Content: []byte("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoFindingWith(t, findings, "File has many extensions")
}

// ─────────────────────────────────────────────────────────────────────────────
// Path traversal tests
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_PathTraversal_DotDotSlash(t *testing.T) {
	s := NewScanner(10000)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "../../../etc/passwd", Size: 10, Content: []byte("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Filename contains path traversal sequence", scanner.SeverityCritical)
}

func TestScan_PathTraversal_Backslash(t *testing.T) {
	s := NewScanner(10000)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "..\\..\\windows\\system32\\cmd.exe", Size: 10, Content: []byte("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Filename contains path traversal sequence", scanner.SeverityCritical)
}

func TestScan_PathTraversal_AbsolutePath(t *testing.T) {
	s := NewScanner(10000)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "/etc/shadow", Size: 10, Content: []byte("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Filename contains path traversal sequence", scanner.SeverityCritical)
}

func TestScan_PathTraversal_WindowsDrive(t *testing.T) {
	s := NewScanner(10000)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "C:\\Windows\\System32\\calc.exe", Size: 10, Content: []byte("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Filename contains path traversal sequence", scanner.SeverityCritical)
}

func TestScan_PathTraversal_SafeFilename(t *testing.T) {
	s := NewScanner(10000)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "my-report-2024.pdf", Size: 10, Content: []byte("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoFindingWith(t, findings, "path traversal")
}

// ─────────────────────────────────────────────────────────────────────────────
// Unicode / control character tests
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_BidiOverride(t *testing.T) {
	s := NewScanner(10000)
	// U+202E = Right-to-Left Override (makes "exe.doc" look like "cod.exe")
	name := "invoice\u202Eexe.doc"
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: name, Size: 10, Content: []byte("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "bidirectional override characters", scanner.SeverityCritical)
}

func TestScan_ZeroWidthChars(t *testing.T) {
	s := NewScanner(10000)
	// U+200B = Zero-Width Space
	name := "invoice\u200Btest.pdf"
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: name, Size: 10, Content: []byte("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "zero-width characters", scanner.SeverityMedium)
}

func TestScan_ControlChars(t *testing.T) {
	s := NewScanner(10000)
	// BEL character (0x07)
	name := "test\x07file.txt"
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: name, Size: 10, Content: []byte("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "control characters", scanner.SeverityHigh)
}

func TestScan_NullByteInFilename(t *testing.T) {
	s := NewScanner(10000)
	name := "test\x00.exe"
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: name, Size: 10, Content: []byte("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "null byte", scanner.SeverityCritical)
}

// ─────────────────────────────────────────────────────────────────────────────
// Text file detection
// ─────────────────────────────────────────────────────────────────────────────

func TestIsTextFile(t *testing.T) {
	textExts := []string{".txt", ".csv", ".json", ".xml", ".svg", ".html", ".htm",
		".css", ".js", ".ts", ".md", ".yaml", ".yml", ".py", ".go", ".rs"}
	for _, ext := range textExts {
		f := &scanner.FileInfo{Extension: ext}
		if !isTextFile(f) {
			t.Errorf("expected %s to be text file", ext)
		}
	}

	nonTextExts := []string{".exe", ".zip", ".png", ".pdf", ".bin", ".dll"}
	for _, ext := range nonTextExts {
		f := &scanner.FileInfo{Extension: ext}
		if isTextFile(f) {
			t.Errorf("expected %s to NOT be text file", ext)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Clean file — no findings
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_CleanFile(t *testing.T) {
	s := NewScanner(100 * 1024 * 1024)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename:  "report.pdf",
		Extension: ".pdf",
		Size:      1024,
		Content:   make([]byte, 1024),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for clean file, got %d: %v", len(findings), findings)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func assertFinding(t *testing.T, findings []scanner.Finding, descContains string, sev scanner.Severity) {
	t.Helper()
	for _, f := range findings {
		if strings.Contains(f.Description, descContains) {
			if f.Severity != sev {
				t.Errorf("finding %q has severity %s, want %s", descContains, f.Severity, sev)
			}
			return
		}
	}
	t.Errorf("expected finding containing %q, got %d findings: %v", descContains, len(findings), findings)
}

func assertNoFindingWith(t *testing.T, findings []scanner.Finding, descContains string) {
	t.Helper()
	for _, f := range findings {
		if strings.Contains(strings.ToLower(f.Description), strings.ToLower(descContains)) {
			t.Errorf("unexpected finding containing %q: %v", descContains, f)
		}
	}
}
