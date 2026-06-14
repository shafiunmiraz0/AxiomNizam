package svg

import (
	"context"
	"strings"
	"testing"

	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
)

var ctx = context.Background()

func TestNewScanner(t *testing.T) {
	s := NewScanner()
	if s.Name() != "svg_xss_scanner" {
		t.Errorf("expected name svg_xss_scanner, got %s", s.Name())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Non-SVG files should be skipped
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_NonSVG_NoFindings(t *testing.T) {
	s := NewScanner()
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "test.png", Extension: ".png", MIMEType: "image/png",
		Content: []byte("<script>alert('xss')</script>"), Size: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for non-SVG, got %d", len(findings))
	}
}

func TestScan_SVG_ByExtension(t *testing.T) {
	s := NewScanner()
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "icon.svg", Extension: ".svg",
		Content: []byte("<svg><script>alert(1)</script></svg>"), Size: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "<script>", scanner.SeverityCritical)
}

func TestScan_SVG_ByMIME(t *testing.T) {
	s := NewScanner()
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "icon", Extension: "", MIMEType: "image/svg+xml",
		Content: []byte("<svg><script>alert(1)</script></svg>"), Size: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, "<script>", scanner.SeverityCritical)
}

// ─────────────────────────────────────────────────────────────────────────────
// Original patterns — critical
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_ScriptTag(t *testing.T) {
	scanSVG(t, `<svg><script>alert(1)</script></svg>`, "<script>", scanner.SeverityCritical)
}

func TestScan_EventHandler(t *testing.T) {
	scanSVG(t, `<svg onload="alert(1)"></svg>`, "event handler", scanner.SeverityCritical)
}

func TestScan_JavascriptURI(t *testing.T) {
	scanSVG(t, `<svg><a href="javascript:alert(1)">click</a></svg>`, "javascript:", scanner.SeverityCritical)
}

func TestScan_DataURI(t *testing.T) {
	scanSVG(t, `<svg><image href="data:image/png;base64,abc"/></svg>`, "data:", scanner.SeverityHigh)
}

func TestScan_ForeignObject(t *testing.T) {
	scanSVG(t, `<svg><foreignObject><body>test</body></foreignObject></svg>`, "foreignObject", scanner.SeverityHigh)
}

func TestScan_ExternalXlink(t *testing.T) {
	scanSVG(t, `<svg><image xlink:href="https://evil.com/payload.png"/></svg>`, "external resource", scanner.SeverityMedium)
}

func TestScan_Base64Embed(t *testing.T) {
	b64 := strings.Repeat("AAAA", 30) // 120 chars of base64
	scanSVG(t, `<svg><image href="data:image/png;base64,`+b64+`"/></svg>`, "base64", scanner.SeverityMedium)
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 4: CSS injection patterns
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_CSSImport(t *testing.T) {
	scanSVG(t, `<svg><style>@import url("https://evil.com/steal.css")</style></svg>`, "@import", scanner.SeverityHigh)
}

func TestScan_CSSUrl(t *testing.T) {
	scanSVG(t, `<svg><style>div { background: url(https://evil.com/track.png) }</style></svg>`, "url()", scanner.SeverityHigh)
}

func TestScan_StyleTag(t *testing.T) {
	scanSVG(t, `<svg><style>.cls { fill: red }</style></svg>`, "<style>", scanner.SeverityMedium)
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 4: External <use> references
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_UseExternalHref(t *testing.T) {
	scanSVG(t, `<svg><use href="https://evil.com/fragment.svg#icon"/></svg>`, "<use>", scanner.SeverityHigh)
}

func TestScan_UseDataURI(t *testing.T) {
	scanSVG(t, `<svg><use href="data:image/svg+xml,<svg/>"/></svg>`, "<use> element with data:", scanner.SeverityHigh)
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 4: Embedded HTML elements
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_Iframe(t *testing.T) {
	scanSVG(t, `<svg><foreignObject><iframe src="https://evil.com"></iframe></foreignObject></svg>`, "iframe", scanner.SeverityCritical)
}

func TestScan_EmbedObject(t *testing.T) {
	scanSVG(t, `<svg><foreignObject><embed src="flash.swf"/></foreignObject></svg>`, "embed", scanner.SeverityHigh)
}

// ─────────────────────────────────────────────────────────────────────────────
// Clean SVG — no findings
// ─────────────────────────────────────────────────────────────────────────────

func TestScan_CleanSVG(t *testing.T) {
	s := NewScanner()
	clean := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
		<circle cx="50" cy="50" r="40" fill="blue"/>
		<rect x="10" y="10" width="80" height="80" fill="none" stroke="red"/>
	</svg>`
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "icon.svg", Extension: ".svg",
		Content: []byte(clean), Size: int64(len(clean)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for clean SVG, got %d: %v", len(findings), findings)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func scanSVG(t *testing.T, content, descContains string, sev scanner.Severity) {
	t.Helper()
	s := NewScanner()
	findings, err := s.Scan(ctx, &scanner.FileInfo{
		Filename: "test.svg", Extension: ".svg",
		Content: []byte(content), Size: int64(len(content)),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, findings, descContains, sev)
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
