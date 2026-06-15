package macro

import (
	"context"
	"strings"
	"testing"

	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
)

var ctx = context.Background()

func TestNewScanner(t *testing.T) {
	s := NewScanner()
	if s.Name() != "macro_script_scanner" {
		t.Errorf("expected name macro_script_scanner, got %s", s.Name())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PDF scanning
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_PDF_JavaScript(t *testing.T) {
	scanFile(t, ".pdf", "application/pdf", []byte("/JavaScript (alert)"), "PDF contains JavaScript", scanner.SeverityCritical)
}

func TestScan_PDF_AutoAction(t *testing.T) {
	scanFile(t, ".pdf", "application/pdf", []byte("/OpenAction /URI"), "auto-execute actions", scanner.SeverityHigh)
}

func TestScan_PDF_LaunchAction(t *testing.T) {
	scanFile(t, ".pdf", "application/pdf", []byte("/Launch /Win cmd.exe"), "launch actions", scanner.SeverityCritical)
}

func TestScan_PDF_EmbeddedFile(t *testing.T) {
	scanFile(t, ".pdf", "application/pdf", []byte("/EmbeddedFile stream"), "embedded files", scanner.SeverityMedium)
}

func TestScan_PDF_URI(t *testing.T) {
	scanFile(t, ".pdf", "application/pdf", []byte("/URI (https://evil.com)"), "URI actions", scanner.SeverityLow)
}

func TestScan_PDF_Encrypt(t *testing.T) {
	scanFile(t, ".pdf", "application/pdf", []byte("/Encrypt /Length 128"), "encrypted", scanner.SeverityMedium)
}

func TestScan_PDF_Clean(t *testing.T) {
	s := NewScanner()
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "doc.pdf", Extension: ".pdf", MIMEType: "application/pdf",
		Content: []byte("%PDF-1.4 clean content"), Size: 22,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for clean PDF, got %d", len(findings))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Legacy Office scanning (OLE2)
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_LegacyOffice_VBA(t *testing.T) {
	data := ole2Header()
	data = append(data, []byte("some data _VBA_PROJECT more data")...)
	scanFile(t, ".doc", "application/msword", data, "VBA macros", scanner.SeverityCritical)
}

func TestScan_LegacyOffice_AutoExec(t *testing.T) {
	data := ole2Header()
	data = append(data, []byte("macro code AutoOpen sub end")...)
	scanFile(t, ".xls", "application/vnd.ms-excel", data, "auto-execute macros", scanner.SeverityCritical)
}

func TestScan_LegacyOffice_Shell(t *testing.T) {
	data := ole2Header()
	data = append(data, []byte("CreateObject(\"WScript.Shell\")")...)
	scanFile(t, ".ppt", "application/vnd.ms-powerpoint", data, "shell execution", scanner.SeverityCritical)
}

func TestScan_LegacyOffice_OLEStreams(t *testing.T) {
	data := ole2Header()
	data = append(data, []byte("\x01Ole something ObjectPool data")...)
	scanFile(t, ".doc", "application/msword", data, "OLE objects", scanner.SeverityMedium)
}

func TestScan_LegacyOffice_NoOLE2Header(t *testing.T) {
	s := NewScanner()
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "fake.doc", Extension: ".doc", MIMEType: "application/msword",
		Content: []byte("not an OLE2 document"), Size: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoFindingWith(t, findings, "VBA")
}

// ─────────────────────────────────────────────────────────────────────────────
// Modern Office scanning (OOXML)
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_ModernOffice_VBAProject(t *testing.T) {
	scanFile(t, ".docm", "", []byte("PK\x03\x04 vbaProject.bin xl/"), "VBA macro project", scanner.SeverityCritical)
}

func TestScan_ModernOffice_ExternalTemplate(t *testing.T) {
	scanFile(t, ".docx", "", []byte(`Target="https://evil.com/template.dotx"`), "external content", scanner.SeverityHigh)
}

func TestScan_ModernOffice_Clean(t *testing.T) {
	s := NewScanner()
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "doc.xlsx", Extension: ".xlsx",
		Content: []byte("PK\x03\x04 normal content without macros"), Size: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for clean OOXML, got %d: %v", len(findings), findings)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// DDE detection
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_DDE_FieldCode(t *testing.T) {
	scanFile(t, ".doc", "application/msword", buildOLE("DDEAUTO c:\\windows\\system32\\cmd.exe"),
		"DDE field codes", scanner.SeverityCritical)
}

func TestScan_DDE_CommandExecution(t *testing.T) {
	scanFile(t, ".doc", "application/msword", buildOLE("DDEAUTO powershell -exec bypass"),
		"DDE command execution", scanner.SeverityCritical)
}

// ─────────────────────────────────────────────────────────────────────────────
// ActiveX detection
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_ActiveX_Control(t *testing.T) {
	scanFile(t, ".docx", "", []byte("activeX1.xml data"), "ActiveX controls", scanner.SeverityHigh)
}

func TestScan_ActiveX_CLSID(t *testing.T) {
	scanFile(t, ".doc", "application/msword", buildOLE("CLSID {00000000-0000-0000-0000-000000000000}"),
		"CLSID", scanner.SeverityHigh)
}

func TestScan_ActiveX_ShellApp(t *testing.T) {
	scanFile(t, ".xls", "application/vnd.ms-excel", buildOLE("Shell.Application execute cmd"),
		"Shell.Application", scanner.SeverityCritical)
}

// ─────────────────────────────────────────────────────────────────────────────
// Non-Office file — no findings
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_NonOffice_NoFindings(t *testing.T) {
	s := NewScanner()
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "image.png", Extension: ".png", MIMEType: "image/png",
		Content: []byte{0x89, 'P', 'N', 'G'}, Size: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for non-Office file, got %d", len(findings))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// File type helpers
// ─────────────────────────────────────────────────────────────────────────────

func TestIsLegacyOffice(t *testing.T) {
	for _, ext := range []string{".doc", ".xls", ".ppt"} {
		if !isLegacyOffice(ext) {
			t.Errorf("expected %s to be legacy Office", ext)
		}
	}
	if isLegacyOffice(".docx") {
		t.Error(".docx should not be legacy Office")
	}
}

func TestIsModernOffice(t *testing.T) {
	for _, ext := range []string{".docx", ".xlsx", ".pptx", ".docm", ".xlsm", ".pptm"} {
		if !isModernOffice(ext) {
			t.Errorf("expected %s to be modern Office", ext)
		}
	}
	if isModernOffice(".doc") {
		t.Error(".doc should not be modern Office")
	}
}

func TestIsAnyOffice(t *testing.T) {
	if !isAnyOffice(".doc", "") {
		t.Error("expected .doc to be any Office")
	}
	if !isAnyOffice(".docx", "") {
		t.Error("expected .docx to be any Office")
	}
	if isAnyOffice(".png", "image/png") {
		t.Error("expected .png to NOT be any Office")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func ole2Header() []byte {
	h := make([]byte, 512)
	h[0], h[1], h[2], h[3] = 0xD0, 0xCF, 0x11, 0xE0
	h[4], h[5], h[6], h[7] = 0xA1, 0xB1, 0x1A, 0xE1
	return h
}

func buildOLE(payload string) []byte {
	h := ole2Header()
	return append(h, []byte(payload)...)
}

func scanFile(t *testing.T, ext, mime string, content []byte, desc string, sev scanner.Severity) {
	t.Helper()
	s := NewScanner()
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "test" + ext, Extension: ext, MIMEType: mime,
		Content: content, Size: int64(len(content)),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, desc, sev)
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

func assertNoFindingWith(t *testing.T, findings []scanner.Finding, desc string) {
	t.Helper()
	for _, f := range findings {
		if strings.Contains(strings.ToLower(f.Description), strings.ToLower(desc)) {
			t.Errorf("unexpected finding containing %q: %v", desc, f)
		}
	}
}
