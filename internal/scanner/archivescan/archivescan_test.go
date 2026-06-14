package archivescan

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
)

var ctx = context.Background()

func TestNewScanner(t *testing.T) {
	s := NewScanner(5, 1<<30)
	if s.Name() != "archive_bomb_scanner" {
		t.Errorf("expected name archive_bomb_scanner, got %s", s.Name())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Non-archive skip
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_NonArchive_NoFindings(t *testing.T) {
	s := NewScanner(5, 1<<30)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "test.txt", Extension: ".txt",
		Content: []byte("hello world"), Size: 11,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for non-archive, got %d", len(findings))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ZIP — clean archive
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_ZIP_Clean(t *testing.T) {
	s := NewScanner(5, 1<<30)
	zipData := createZip(t, map[string][]byte{
		"readme.txt":  []byte("Hello, World!"),
		"data.csv":    []byte("a,b,c"),
	})
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "data.zip", Extension: ".zip",
		Content: zipData, Size: int64(len(zipData)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for clean zip, got %d: %v", len(findings), findings)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ZIP — path traversal
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_ZIP_PathTraversal(t *testing.T) {
	s := NewScanner(5, 1<<30)
	zipData := createZip(t, map[string][]byte{
		"../../../etc/passwd": []byte("root:x:0:0"),
	})
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "evil.zip", Extension: ".zip",
		Content: zipData, Size: int64(len(zipData)),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "path traversal", scanner.SeverityCritical)
}

// ─────────────────────────────────────────────────────────────────────────────
// ZIP — executable inside
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_ZIP_Executable(t *testing.T) {
	s := NewScanner(5, 1<<30)
	zipData := createZip(t, map[string][]byte{
		"payload.exe": []byte("MZ fake executable"),
	})
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "tools.zip", Extension: ".zip",
		Content: zipData, Size: int64(len(zipData)),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "executable file", scanner.SeverityHigh)
}

// ─────────────────────────────────────────────────────────────────────────────
// ZIP — excessive files
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_ZIP_ExcessiveFiles(t *testing.T) {
	s := NewScanner(5, 1<<30)
	files := make(map[string][]byte)
	for i := 0; i < 10001; i++ {
		name := fmt.Sprintf("file_%05d.txt", i)
		files[name] = []byte("x")
	}
	zipData := createZip(t, files)
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "bomb.zip", Extension: ".zip",
		Content: zipData, Size: int64(len(zipData)),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "excessive files", scanner.SeverityHigh)
}

// ─────────────────────────────────────────────────────────────────────────────
// Archive detection helpers
// ─────────────────────────────────────────────────────────────────────────────

func TestIsArchive(t *testing.T) {
	cases := []struct {
		ext  string
		want bool
	}{
		{".zip", true}, {".rar", true}, {".7z", true}, {".tar", true},
		{".gz", true}, {".tgz", true}, {".bz2", true}, {".xz", true},
		{".docx", true}, {".jar", true},
		{".txt", false}, {".png", false}, {".pdf", false},
	}
	for _, tc := range cases {
		f := &scanner.FileInfo{Extension: tc.ext}
		got := isArchive(f)
		if got != tc.want {
			t.Errorf("isArchive(%s) = %v, want %v", tc.ext, got, tc.want)
		}
	}
}

func TestIsZipArchive_MagicBytes(t *testing.T) {
	f := &scanner.FileInfo{
		Extension: ".bin",
		Content:   []byte{0x50, 0x4B, 0x03, 0x04, 0x00},
	}
	if !isZipArchive(f) {
		t.Error("expected PK magic bytes to be detected as zip")
	}
}

func TestIsTarData(t *testing.T) {
	data := make([]byte, 263)
	copy(data[257:], "ustar")
	if !isTarData(data) {
		t.Error("expected ustar magic to be detected as tar")
	}
	if isTarData([]byte("short")) {
		t.Error("short data should not be tar")
	}
}

func TestIsGzipArchive(t *testing.T) {
	f := &scanner.FileInfo{
		Extension: ".bin",
		Content:   []byte{0x1F, 0x8B, 0x08},
	}
	if !isGzipArchive(f) {
		t.Error("expected gzip magic bytes to be detected")
	}
}

func TestIsBzip2Archive(t *testing.T) {
	f := &scanner.FileInfo{
		Extension: ".bin",
		Content:   []byte{0x42, 0x5A, 0x68, 0x39},
	}
	if !isBzip2Archive(f) {
		t.Error("expected bzip2 magic bytes to be detected")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Executable extension detection
// ─────────────────────────────────────────────────────────────────────────────

func TestIsExecutableExtension(t *testing.T) {
	execs := []string{
		"payload.exe", "run.bat", "script.cmd", "tool.msi",
		"deploy.sh", "update.ps1", "macro.vbs", "app.dll", "install.hta",
	}
	for _, name := range execs {
		if !isExecutableExtension(name) {
			t.Errorf("expected %s to be executable", name)
		}
	}

	safe := []string{"readme.txt", "image.png", "data.csv", "doc.pdf"}
	for _, name := range safe {
		if isExecutableExtension(name) {
			t.Errorf("expected %s to NOT be executable", name)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Path traversal detection
// ─────────────────────────────────────────────────────────────────────────────

func TestContainsPathTraversal(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"../etc/passwd", true},
		{"foo/../../bar", true},
		{"/absolute/path", true},
		{"..", true},
		{"safe/file.txt", false},
		{"no-dots-here", false},
	}
	for _, tc := range cases {
		if containsPathTraversal(tc.path) != tc.want {
			t.Errorf("containsPathTraversal(%q) = %v, want %v", tc.path, !tc.want, tc.want)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func createZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry %q: %v", name, err)
		}
		if _, err := f.Write(content); err != nil {
			t.Fatalf("failed to write zip entry %q: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}
	return buf.Bytes()
}

func assertFinding(t *testing.T, findings []scanner.Finding, desc string, sev scanner.Severity) {
	t.Helper()
	for _, f := range findings {
		if strings.Contains(strings.ToLower(f.Description), strings.ToLower(desc)) {
			if f.Severity != sev {
				t.Errorf("finding %q severity=%s, want %s", desc, f.Severity, sev)
			}
			return
		}
	}
	t.Errorf("expected finding containing %q not found in %d findings", desc, len(findings))
}
