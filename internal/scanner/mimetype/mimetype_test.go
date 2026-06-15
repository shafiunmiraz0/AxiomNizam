package mimetype

import (
	"context"
	"strings"
	"testing"

	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
)

var ctx = context.Background()

// ─────────────────────────────────────────────────────────────────────────────
// Constructor & name
// ─────────────────────────────────────────────────────────────────────────────

func TestNewScanner(t *testing.T) {
	s := NewScanner([]string{"text/plain", "image/png"})
	if s.Name() != "mime_type_validator" {
		t.Errorf("expected name mime_type_validator, got %s", s.Name())
	}
	if !s.allowedTypes["text/plain"] || !s.allowedTypes["image/png"] {
		t.Error("allowed types not properly set")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Allowed/disallowed type detection
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_AllowedType(t *testing.T) {
	s := NewScanner([]string{"text/plain"})
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "test.txt", Extension: ".txt",
		Content: []byte("Hello, World!"), Size: 13,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoFindingWith(t, findings, "Disallowed file type")
}

func TestScan_DisallowedType(t *testing.T) {
	s := NewScanner([]string{"image/png"}) // only allow PNG
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "test.txt", Extension: ".txt",
		Content: []byte("Hello, World!"), Size: 13, // text/plain, not in allowed
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Disallowed file type", scanner.SeverityHigh)
}

// ─────────────────────────────────────────────────────────────────────────────
// Type spoofing detection
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_TypeSpoofing(t *testing.T) {
	s := NewScanner([]string{"text/plain", "text/html"})
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "test.txt", Extension: ".txt",
		MIMEType: "image/png",                // claims to be PNG
		Content:  []byte("Hello, World!"),     // but content is text/plain
		Size:     13,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "File type spoofing detected", scanner.SeverityCritical)
}

func TestScan_NoSpoofing_CompatibleTypes(t *testing.T) {
	s := NewScanner([]string{"text/plain"})
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "data.csv", Extension: ".csv",
		MIMEType: "text/csv",             // CSV claims
		Content:  []byte("a,b,c\n1,2,3"), // detected as text/plain (compatible)
		Size:     10,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoFindingWith(t, findings, "spoofing")
}

// ─────────────────────────────────────────────────────────────────────────────
// Dangerous format detection — PE/ELF/Mach-O
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_PE_Executable(t *testing.T) {
	s := NewScanner([]string{"application/octet-stream"})
	// MZ header = PE executable
	pe := []byte{'M', 'Z', 0x90, 0x00, 0x03, 0x00, 0x00, 0x00}
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "payload.txt", Extension: ".txt",
		Content: pe, Size: int64(len(pe)),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "PE executable", scanner.SeverityCritical)
}

func TestScan_ELF_Executable(t *testing.T) {
	s := NewScanner([]string{"application/octet-stream"})
	elf := []byte{0x7F, 'E', 'L', 'F', 0x02, 0x01}
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "binary", Extension: "",
		Content: elf, Size: int64(len(elf)),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "ELF executable", scanner.SeverityCritical)
}

func TestScan_MachO_Executable(t *testing.T) {
	s := NewScanner([]string{"application/octet-stream"})
	macho := []byte{0xCF, 0xFA, 0xED, 0xFE, 0x07, 0x00}
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "app", Extension: "",
		Content: macho, Size: int64(len(macho)),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Mach-O executable", scanner.SeverityCritical)
}

// ─────────────────────────────────────────────────────────────────────────────
// Dangerous format detection — WebAssembly, Java, Shell scripts
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_WebAssembly(t *testing.T) {
	s := NewScanner([]string{"application/octet-stream"})
	wasm := []byte{0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00}
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "module.wasm", Extension: ".wasm",
		Content: wasm, Size: int64(len(wasm)),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "WebAssembly", scanner.SeverityCritical)
}

func TestScan_JavaClass(t *testing.T) {
	s := NewScanner([]string{"application/octet-stream"})
	java := []byte{0xCA, 0xFE, 0xBA, 0xBE, 0x00, 0x00}
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "Exploit.class", Extension: ".class",
		Content: java, Size: int64(len(java)),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Java class", scanner.SeverityHigh)
}

func TestScan_ShellScript(t *testing.T) {
	s := NewScanner([]string{"text/plain"})
	script := []byte("#!/bin/bash\nrm -rf /\n")
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "update.sh", Extension: ".sh",
		Content: script, Size: int64(len(script)),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "Shell script", scanner.SeverityHigh)
}

// ─────────────────────────────────────────────────────────────────────────────
// Magic byte matchers (unit-level)
// ─────────────────────────────────────────────────────────────────────────────

func TestIsPE(t *testing.T)          { check(t, isPE, []byte{'M', 'Z'}, true) }
func TestIsELF(t *testing.T)         { check(t, isELF, []byte{0x7F, 'E', 'L', 'F'}, true) }
func TestIsMachO(t *testing.T)       { check(t, isMachO, []byte{0xFE, 0xED, 0xFA, 0xCE}, true) }
func TestIsWebAssembly(t *testing.T) { check(t, isWebAssembly, []byte{0x00, 0x61, 0x73, 0x6D}, true) }
func TestIsJavaClass(t *testing.T)   { check(t, isJavaClass, []byte{0xCA, 0xFE, 0xBA, 0xBE}, true) }
func TestIsShellScript(t *testing.T) { check(t, isShellScript, []byte{'#', '!'}, true) }

func TestMagicBytes_TooShort(t *testing.T) {
	short := []byte{0x00}
	if isPE(short) || isELF(short) || isMachO(short) || isWebAssembly(short) || isJavaClass(short) {
		t.Error("magic byte check should return false for too-short data")
	}
}

func TestMagicBytes_PlainText(t *testing.T) {
	plain := []byte("Hello, World!")
	if isPE(plain) || isELF(plain) || isMachO(plain) || isWebAssembly(plain) || isJavaClass(plain) {
		t.Error("magic byte check should return false for plain text")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MIME compatibility
// ─────────────────────────────────────────────────────────────────────────────

func TestTypesCompatible_Exact(t *testing.T) {
	if !typesCompatible("text/plain", "text/plain") {
		t.Error("identical types should be compatible")
	}
}

func TestTypesCompatible_OctetStream(t *testing.T) {
	if !typesCompatible("image/png", "application/octet-stream") {
		t.Error("octet-stream should be compatible with anything")
	}
}

func TestTypesCompatible_CompatMap(t *testing.T) {
	if !typesCompatible("text/csv", "text/plain") {
		t.Error("CSV→text/plain should be compatible via CompatMap")
	}
}

func TestTypesCompatible_Incompatible(t *testing.T) {
	if typesCompatible("image/png", "text/html") {
		t.Error("image/png and text/html should NOT be compatible")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Clean file
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_CleanFile(t *testing.T) {
	s := NewScanner([]string{"text/plain"})
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "readme.txt", Extension: ".txt",
		Content: []byte("This is safe content."), Size: 21,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for clean file, got %d", len(findings))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func check(t *testing.T, fn func([]byte) bool, data []byte, want bool) {
	t.Helper()
	if fn(data) != want {
		t.Errorf("expected %v, got %v for %v", want, !want, data)
	}
}

func assertFinding(t *testing.T, findings []scanner.Finding, desc string, sev scanner.Severity) {
	t.Helper()
	for _, f := range findings {
		if strings.Contains(f.Description, desc) {
			if f.Severity != sev {
				t.Errorf("finding %q severity=%s, want %s", desc, f.Severity, sev)
			}
			return
		}
	}
	t.Errorf("expected finding %q not found in %d findings", desc, len(findings))
}

func assertNoFindingWith(t *testing.T, findings []scanner.Finding, desc string) {
	t.Helper()
	for _, f := range findings {
		if strings.Contains(strings.ToLower(f.Description), strings.ToLower(desc)) {
			t.Errorf("unexpected finding containing %q: %v", desc, f)
		}
	}
}
