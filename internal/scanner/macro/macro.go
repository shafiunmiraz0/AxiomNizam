// Package macro provides the MacroScanner for the SafeGate pipeline.
// It detects embedded macros and scripts in Office documents (legacy .doc/.xls/.ppt
// and modern .docx/.xlsx/.pptx) and PDF files, including VBA macros,
// auto-execute entries, shell commands, external template references,
// DDE links, and ActiveX controls.
package macro

import (
	"context"
	"regexp"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
)

// Scanner detects embedded macros and scripts in Office documents and PDFs.
type Scanner struct{}

// NewScanner creates a new MacroScanner.
func NewScanner() *Scanner { return &Scanner{} }

func (s *Scanner) Name() string { return "macro_script_scanner" }

func (s *Scanner) Scan(_ context.Context, file *scanner.FileInfo) ([]scanner.Finding, error) {
	ext := strings.ToLower(file.Extension)
	mime := strings.ToLower(file.MIMEType)

	var findings []scanner.Finding

	if ext == ".pdf" || strings.Contains(mime, "pdf") {
		findings = append(findings, s.scanPDF(file.Content)...)
	}

	if isLegacyOffice(ext) || isLegacyOfficeMIME(mime) {
		findings = append(findings, s.scanLegacyOffice(file.Content)...)
	}

	if isModernOffice(ext) || isModernOfficeMIME(mime) {
		findings = append(findings, s.scanModernOffice(file.Content)...)
	}

	// DDE and ActiveX apply to both legacy and modern Office formats
	if isAnyOffice(ext, mime) {
		findings = append(findings, s.scanDDE(file.Content)...)
		findings = append(findings, s.scanActiveX(file.Content)...)
	}

	return findings, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// PDF scanning
// ─────────────────────────────────────────────────────────────────────────────

func (s *Scanner) scanPDF(data []byte) []scanner.Finding {
	var findings []scanner.Finding
	content := string(data)

	checks := []struct {
		pattern *regexp.Regexp
		sev     scanner.Severity
		desc    string
		details string
	}{
		{pdfJSPattern, scanner.SeverityCritical, "PDF contains JavaScript",
			"Embedded JavaScript in PDF can execute malicious code when opened"},
		{pdfAutoActionPattern, scanner.SeverityHigh, "PDF contains auto-execute actions",
			"OpenAction/AA entries can trigger actions automatically when the PDF is opened"},
		{pdfLaunchPattern, scanner.SeverityCritical, "PDF contains launch actions",
			"Launch actions can execute arbitrary system commands"},
		{pdfEmbeddedFilePattern, scanner.SeverityMedium, "PDF contains embedded files",
			"Embedded file streams can carry hidden payloads"},
		{pdfURIPattern, scanner.SeverityLow, "PDF contains URI actions",
			"URI actions can redirect users to malicious websites"},
		{pdfEncryptPattern, scanner.SeverityMedium, "PDF contains encrypted or encoded streams",
			"Encrypted streams may hide malicious content from analysis"},
	}

	for _, c := range checks {
		if c.pattern.MatchString(content) {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: c.sev, Description: c.desc, Details: c.details,
			})
		}
	}
	return findings
}

// ─────────────────────────────────────────────────────────────────────────────
// Legacy Office scanning (OLE2/CFB)
// ─────────────────────────────────────────────────────────────────────────────

func (s *Scanner) scanLegacyOffice(data []byte) []scanner.Finding {
	var findings []scanner.Finding

	// Verify OLE2 Compound File Binary (CFB) header: D0 CF 11 E0 A1 B1 1A E1
	if len(data) < 8 || data[0] != 0xD0 || data[1] != 0xCF || data[2] != 0x11 || data[3] != 0xE0 {
		return nil
	}

	content := string(data)

	// VBA macro detection
	if strings.Contains(content, "VBA") || strings.Contains(content, "_VBA_PROJECT") {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityCritical,
			Description: "Office document contains VBA macros",
			Details:     "VBA macros can execute arbitrary code and are a common malware vector",
		})
	}

	// Auto-execute macro names
	autoExecNames := []string{
		"AutoOpen", "AutoClose", "AutoExec", "AutoNew",
		"Document_Open", "Document_Close",
		"Workbook_Open", "Workbook_Close",
	}
	for _, name := range autoExecNames {
		if strings.Contains(content, name) {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: scanner.SeverityCritical,
				Description: "Office document contains auto-execute macros",
				Details:     "Found auto-execute macro: " + name,
			})
			break
		}
	}

	// Shell execution commands
	if shellPattern.MatchString(content) {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityCritical,
			Description: "Office document contains shell execution commands",
			Details:     "Document references shell/command execution APIs",
		})
	}

	// OLE2 stream names that indicate embedded objects
	oleStreamNames := []string{
		"\x01Ole", "ObjectPool", "\x01CompObj", "ObjInfo",
	}
	for _, stream := range oleStreamNames {
		if strings.Contains(content, stream) {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: scanner.SeverityMedium,
				Description: "Office document contains embedded OLE objects",
				Details:     "OLE embedded objects can carry executable payloads or trigger exploits",
			})
			break
		}
	}

	return findings
}

// ─────────────────────────────────────────────────────────────────────────────
// Modern Office scanning (OOXML/ZIP-based)
// ─────────────────────────────────────────────────────────────────────────────

func (s *Scanner) scanModernOffice(data []byte) []scanner.Finding {
	var findings []scanner.Finding
	content := string(data)

	if strings.Contains(content, "vbaProject.bin") {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityCritical,
			Description: "Office document contains VBA macro project",
			Details:     "vbaProject.bin found — macros are present",
		})
	}

	if externalRelPattern.MatchString(content) {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityHigh,
			Description: "Office document references external content",
			Details:     "External relationships can load remote templates (template injection attack)",
		})
	}

	return findings
}

// ─────────────────────────────────────────────────────────────────────────────
// DDE link detection (applies to both legacy and modern Office)
// ─────────────────────────────────────────────────────────────────────────────

func (s *Scanner) scanDDE(data []byte) []scanner.Finding {
	var findings []scanner.Finding
	content := string(data)

	// DDE (Dynamic Data Exchange) links can execute commands without macros
	for _, check := range ddeChecks {
		if check.pattern.MatchString(content) {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: check.sev, Description: check.desc, Details: check.details,
			})
		}
	}

	return findings
}

var ddeChecks = []struct {
	pattern *regexp.Regexp
	sev     scanner.Severity
	desc    string
	details string
}{
	{ddeFieldPattern, scanner.SeverityCritical,
		"Document contains DDE field codes",
		"DDE (DDEAUTO/DDE) field codes can execute arbitrary commands without VBA macros — a common attack vector"},
	{ddeExecPattern, scanner.SeverityCritical,
		"Document contains DDE command execution",
		"DDE formula with executable reference (cmd, powershell, mshta) — active command execution attack"},
}

// ─────────────────────────────────────────────────────────────────────────────
// ActiveX control detection (applies to both legacy and modern Office)
// ─────────────────────────────────────────────────────────────────────────────

func (s *Scanner) scanActiveX(data []byte) []scanner.Finding {
	var findings []scanner.Finding
	content := string(data)

	// ActiveX controls can execute arbitrary code
	for _, check := range activeXChecks {
		if check.pattern.MatchString(content) {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: check.sev, Description: check.desc, Details: check.details,
			})
		}
	}

	return findings
}

var activeXChecks = []struct {
	pattern *regexp.Regexp
	sev     scanner.Severity
	desc    string
	details string
}{
	{activeXPattern, scanner.SeverityHigh,
		"Document contains ActiveX controls",
		"ActiveX controls (activeX*.xml, CLSID references) can execute native code — a known exploit vector"},
	{clsidPattern, scanner.SeverityHigh,
		"Document contains COM/CLSID references",
		"CLSID references indicate COM objects that can execute arbitrary code via OLE automation"},
	{shellObjPattern, scanner.SeverityCritical,
		"Document contains Shell.Application or WScript.Shell ActiveX",
		"Shell automation objects can execute arbitrary system commands, download files, and modify the registry"},
}

// ─────────────────────────────────────────────────────────────────────────────
// File type helpers
// ─────────────────────────────────────────────────────────────────────────────

func isLegacyOffice(ext string) bool {
	switch ext {
	case ".doc", ".xls", ".ppt":
		return true
	}
	return false
}

func isLegacyOfficeMIME(mime string) bool {
	return strings.Contains(mime, "msword") ||
		strings.Contains(mime, "ms-excel") ||
		strings.Contains(mime, "ms-powerpoint")
}

func isModernOffice(ext string) bool {
	switch ext {
	case ".docx", ".xlsx", ".pptx", ".docm", ".xlsm", ".pptm":
		return true
	}
	return false
}

func isModernOfficeMIME(mime string) bool {
	return strings.Contains(mime, "openxmlformats")
}

func isAnyOffice(ext, mime string) bool {
	return isLegacyOffice(ext) || isLegacyOfficeMIME(mime) ||
		isModernOffice(ext) || isModernOfficeMIME(mime)
}

// ─────────────────────────────────────────────────────────────────────────────
// Compiled regex patterns
// ─────────────────────────────────────────────────────────────────────────────

var (
	// PDF patterns
	pdfJSPattern           = regexp.MustCompile(`(?i)/JavaScript|/JS\s`)
	pdfAutoActionPattern   = regexp.MustCompile(`(?i)/OpenAction|/AA\s`)
	pdfLaunchPattern       = regexp.MustCompile(`(?i)/Launch`)
	pdfEmbeddedFilePattern = regexp.MustCompile(`(?i)/EmbeddedFile`)
	pdfURIPattern          = regexp.MustCompile(`(?i)/URI\s`)
	pdfEncryptPattern      = regexp.MustCompile(`(?i)/Encrypt|/Crypt`)

	// Legacy Office patterns
	shellPattern = regexp.MustCompile(`(?i)(WScript|Shell|PowerShell|cmd\.exe|CreateObject)`)

	// Modern Office patterns
	externalRelPattern = regexp.MustCompile(`(?i)Target\s*=\s*"https?://`)

	// DDE patterns
	ddeFieldPattern = regexp.MustCompile(`(?i)(DDE|DDEAUTO)\s`)
	ddeExecPattern  = regexp.MustCompile(`(?i)DDE(?:AUTO)?\s.*(?:cmd|powershell|mshta|wscript|cscript)`)

	// ActiveX patterns
	activeXPattern = regexp.MustCompile(`(?i)activeX\d*\.xml|ActiveXData`)
	clsidPattern   = regexp.MustCompile(`(?i)CLSID|classid\s*=`)
	shellObjPattern = regexp.MustCompile(`(?i)(Shell\.Application|WScript\.Shell|Scripting\.FileSystemObject)`)
)
