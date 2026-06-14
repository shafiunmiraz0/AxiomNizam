// Package mimetype provides the MIMEScanner for the SafeGate pipeline.
// It detects file type spoofing by comparing actual MIME type (from content
// magic bytes) against the claimed extension, checks for executable
// signatures hidden in non-executable files, and detects dangerous
// file formats (WebAssembly, Java class files, shell scripts).
package mimetype

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
)

// Scanner detects file type spoofing by comparing the actual MIME type
// (detected from file content magic bytes) against the claimed extension.
type Scanner struct {
	allowedTypes map[string]bool
}

// NewScanner creates a MIMEScanner with the given list of allowed MIME types.
func NewScanner(allowedTypes []string) *Scanner {
	m := make(map[string]bool, len(allowedTypes))
	for _, t := range allowedTypes {
		m[t] = true
	}
	return &Scanner{allowedTypes: m}
}

func (s *Scanner) Name() string { return "mime_type_validator" }

func (s *Scanner) Scan(_ context.Context, file *scanner.FileInfo) ([]scanner.Finding, error) {
	var findings []scanner.Finding

	// Detect actual MIME type from content bytes
	detected := http.DetectContentType(file.Content)
	detected = strings.Split(detected, ";")[0]
	detected = strings.TrimSpace(detected)

	// Check if the detected type is in the allowed list
	if !s.allowedTypes[detected] && detected != "application/octet-stream" {
		findings = append(findings, scanner.Finding{
			Scanner:     s.Name(),
			Severity:    scanner.SeverityHigh,
			Description: "Disallowed file type detected",
			Details:     fmt.Sprintf("Detected MIME type %q is not in the allowed list", detected),
		})
	}

	// Check for type spoofing: extension says one thing, content says another
	if file.MIMEType != "" && detected != "application/octet-stream" {
		claimedBase := strings.Split(file.MIMEType, ";")[0]
		claimedBase = strings.TrimSpace(claimedBase)
		if !typesCompatible(claimedBase, detected) {
			findings = append(findings, scanner.Finding{
				Scanner:     s.Name(),
				Severity:    scanner.SeverityCritical,
				Description: "File type spoofing detected",
				Details: fmt.Sprintf(
					"File claims to be %q but content detected as %q — possible disguised payload",
					claimedBase, detected,
				),
			})
		}
	}

	// ── Dangerous file format detection (magic bytes) ────────────────────
	findings = append(findings, checkDangerousFormats(file.Content, detected)...)

	return findings, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Dangerous format detection via magic bytes
// ─────────────────────────────────────────────────────────────────────────────

// checkDangerousFormats inspects raw file content for magic byte signatures
// of executable, compiled, or otherwise dangerous file formats that Go's
// http.DetectContentType does not identify.
func checkDangerousFormats(data []byte, detectedMIME string) []scanner.Finding {
	var findings []scanner.Finding

	for _, check := range dangerousFormatChecks {
		if check.matchFn(data) && !strings.Contains(detectedMIME, "executable") {
			findings = append(findings, scanner.Finding{
				Scanner:     "mime_type_validator",
				Severity:    check.severity,
				Description: check.desc,
				Details:     check.details,
			})
		}
	}

	return findings
}

type formatCheck struct {
	matchFn  func([]byte) bool
	severity scanner.Severity
	desc     string
	details  string
}

var dangerousFormatChecks = []formatCheck{
	// ── Native executables ───────────────────────────────────────────────
	{isPE, scanner.SeverityCritical,
		"PE executable detected (Windows .exe/.dll)",
		"File contains MZ header — Windows Portable Executable format"},
	{isELF, scanner.SeverityCritical,
		"ELF executable detected (Linux/Unix binary)",
		"File contains ELF magic bytes — Linux/Unix executable or shared library"},
	{isMachO, scanner.SeverityCritical,
		"Mach-O executable detected (macOS binary)",
		"File contains Mach-O magic bytes — macOS executable or library"},

	// ── Compiled/intermediate formats ────────────────────────────────────
	{isWebAssembly, scanner.SeverityCritical,
		"WebAssembly binary detected",
		"File contains WASM magic bytes (\\x00asm) — WebAssembly modules can execute native-speed code in browsers"},
	{isJavaClass, scanner.SeverityHigh,
		"Java class file detected",
		"File contains Java class magic bytes (0xCAFEBABE) — compiled Java bytecode can execute arbitrary code via JVM"},

	// ── Script-based threats ─────────────────────────────────────────────
	{isShellScript, scanner.SeverityHigh,
		"Shell script detected",
		"File begins with a shebang (#!) indicating a script — can execute system commands if invoked"},
}

// ─────────────────────────────────────────────────────────────────────────────
// Magic byte matchers
// ─────────────────────────────────────────────────────────────────────────────

// isPE checks for Windows PE (MZ) header.
func isPE(data []byte) bool {
	return len(data) >= 2 && data[0] == 'M' && data[1] == 'Z'
}

// isELF checks for Linux/Unix ELF header (0x7F ELF).
func isELF(data []byte) bool {
	return len(data) >= 4 &&
		data[0] == 0x7F && data[1] == 'E' && data[2] == 'L' && data[3] == 'F'
}

// isMachO checks for macOS Mach-O magic bytes (all four variants).
func isMachO(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	return (data[0] == 0xFE && data[1] == 0xED && data[2] == 0xFA && data[3] == 0xCE) || // 32-bit
		(data[0] == 0xFE && data[1] == 0xED && data[2] == 0xFA && data[3] == 0xCF) || // 64-bit
		(data[0] == 0xCE && data[1] == 0xFA && data[2] == 0xED && data[3] == 0xFE) || // 32-bit reversed
		(data[0] == 0xCF && data[1] == 0xFA && data[2] == 0xED && data[3] == 0xFE) // 64-bit reversed
}

// isWebAssembly checks for WASM magic bytes: \x00asm (0x00 0x61 0x73 0x6D).
func isWebAssembly(data []byte) bool {
	return len(data) >= 4 &&
		data[0] == 0x00 && data[1] == 0x61 && data[2] == 0x73 && data[3] == 0x6D
}

// isJavaClass checks for Java class file magic bytes: 0xCAFEBABE.
func isJavaClass(data []byte) bool {
	return len(data) >= 4 &&
		data[0] == 0xCA && data[1] == 0xFE && data[2] == 0xBA && data[3] == 0xBE
}

// isShellScript checks for shebang (#!) at the start of the file.
func isShellScript(data []byte) bool {
	return len(data) >= 2 && data[0] == '#' && data[1] == '!'
}

// ─────────────────────────────────────────────────────────────────────────────
// MIME type compatibility
// ─────────────────────────────────────────────────────────────────────────────

// CompatMap defines which MIME types are compatible with each other.
// This is exported so callers can inspect or extend the map.
var CompatMap = map[string][]string{
	"application/msword": {"application/zip", "application/x-cfbf", "application/octet-stream"},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": {"application/zip", "application/octet-stream"},
	"application/vnd.ms-excel": {"application/zip", "application/x-cfbf", "application/octet-stream"},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         {"application/zip", "application/octet-stream"},
	"application/vnd.ms-powerpoint":                                             {"application/zip", "application/x-cfbf", "application/octet-stream"},
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": {"application/zip", "application/octet-stream"},
	"image/svg+xml":                {"text/xml", "application/xml", "text/plain", "text/html"},
	"text/csv":                     {"text/plain", "application/octet-stream"},
	"application/json":             {"text/plain"},
	"application/x-rar-compressed": {"application/octet-stream"},
	"application/x-7z-compressed":  {"application/octet-stream"},
	"application/wasm":             {"application/octet-stream"},
	"application/java-archive":     {"application/zip", "application/octet-stream"},
}

func typesCompatible(claimed, detected string) bool {
	claimed = strings.ToLower(claimed)
	detected = strings.ToLower(detected)

	if claimed == detected {
		return true
	}

	if compatible, ok := CompatMap[claimed]; ok {
		for _, c := range compatible {
			if detected == c {
				return true
			}
		}
	}

	if detected == "application/octet-stream" {
		return true
	}

	return false
}
