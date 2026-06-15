// Package svg provides the SVGScanner for the SafeGate pipeline.
// It detects XSS attacks and malicious content embedded in SVG files,
// including script tags, event handlers, javascript: URIs, data: URIs,
// foreignObject elements, external references, CSS injection vectors,
// and external SVG <use> references.
package svg

import (
	"context"
	"regexp"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
)

// Scanner detects XSS attacks and malicious content embedded in SVG files.
type Scanner struct{}

// NewScanner creates a new SVGScanner.
func NewScanner() *Scanner { return &Scanner{} }

func (s *Scanner) Name() string { return "svg_xss_scanner" }

func (s *Scanner) Scan(_ context.Context, file *scanner.FileInfo) ([]scanner.Finding, error) {
	if !isSVG(file) {
		return nil, nil
	}

	var findings []scanner.Finding
	content := strings.ToLower(string(file.Content))

	for _, c := range patternChecks {
		if c.pattern.MatchString(content) {
			findings = append(findings, scanner.Finding{
				Scanner:     s.Name(),
				Severity:    c.severity,
				Description: c.desc,
				Details:     c.details,
			})
		}
	}

	return findings, nil
}

func isSVG(file *scanner.FileInfo) bool {
	if strings.EqualFold(file.Extension, ".svg") {
		return true
	}
	mime := strings.ToLower(file.MIMEType)
	return strings.Contains(mime, "svg")
}

// ─────────────────────────────────────────────────────────────────────────────
// Pattern definitions — table-driven for maintainability
// ─────────────────────────────────────────────────────────────────────────────

type check struct {
	pattern  *regexp.Regexp
	severity scanner.Severity
	desc     string
	details  string
}

var patternChecks = []check{
	// ── Critical: Direct code execution ──────────────────────────────────
	{scriptTagPattern, scanner.SeverityCritical,
		"SVG contains <script> tags",
		"Inline JavaScript in SVG files can execute XSS attacks when rendered in a browser"},
	{eventHandlerPattern, scanner.SeverityCritical,
		"SVG contains event handler attributes",
		"Event handlers like onload/onclick can execute arbitrary JavaScript"},
	{jsURIPattern, scanner.SeverityCritical,
		"SVG contains javascript: URI",
		"javascript: URIs in SVG attributes can execute arbitrary code"},

	// ── High: Content embedding / HTML injection ─────────────────────────
	{dataURIPattern, scanner.SeverityHigh,
		"SVG contains data: URI",
		"data: URIs in SVG can embed executable content or exfiltrate data"},
	{foreignObjectPattern, scanner.SeverityHigh,
		"SVG contains <foreignObject> element",
		"foreignObject can embed arbitrary HTML/XHTML inside SVG, enabling XSS"},

	// ── Medium: External resource loading ────────────────────────────────
	{xlinkPattern, scanner.SeverityMedium,
		"SVG contains external resource references",
		"External xlink:href references can load malicious content or leak data"},
	{base64EmbedPattern, scanner.SeverityMedium,
		"SVG contains embedded base64 data",
		"Base64-encoded content in SVG may hide malicious payloads"},

	// ── Phase 4: CSS injection vectors ───────────────────────────────────
	{cssImportPattern, scanner.SeverityHigh,
		"SVG contains CSS @import rule",
		"CSS @import in SVG can load external stylesheets, enabling data exfiltration and CSS injection attacks"},
	{cssURLPattern, scanner.SeverityHigh,
		"SVG contains CSS url() function with external reference",
		"CSS url() in SVG can load external resources, enabling tracking, data exfiltration, or font-based attacks"},
	{styleTagPattern, scanner.SeverityMedium,
		"SVG contains <style> element",
		"Inline <style> blocks in SVG can contain CSS injection vectors including @import and url() calls"},

	// ── Phase 4: External <use> references ───────────────────────────────
	{useExternalPattern, scanner.SeverityHigh,
		"SVG contains <use> element with external reference",
		"<use> with external href can load arbitrary SVG fragments from remote servers, enabling content injection"},
	{useDataURIPattern, scanner.SeverityHigh,
		"SVG contains <use> element with data: URI",
		"<use> with data: URI can embed arbitrary SVG content inline, bypassing content filters"},

	// ── Phase 4: Additional injection vectors ────────────────────────────
	{iframePattern, scanner.SeverityCritical,
		"SVG contains embedded <iframe> element",
		"Iframes embedded in SVG via foreignObject can load arbitrary external pages"},
	{embedObjectPattern, scanner.SeverityHigh,
		"SVG contains <embed> or <object> element",
		"Embed/object elements in SVG can load plugins, Flash, or arbitrary external content"},
}

// ─────────────────────────────────────────────────────────────────────────────
// Compiled regex patterns
// ─────────────────────────────────────────────────────────────────────────────

var (
	// Original patterns
	scriptTagPattern     = regexp.MustCompile(`<\s*script[\s>]`)
	eventHandlerPattern  = regexp.MustCompile(`\bon\w+\s*=`)
	jsURIPattern         = regexp.MustCompile(`javascript\s*:`)
	dataURIPattern       = regexp.MustCompile(`data\s*:\s*[^,]*;`)
	foreignObjectPattern = regexp.MustCompile(`<\s*foreignobject[\s>]`)
	xlinkPattern         = regexp.MustCompile(`xlink:href\s*=\s*["']https?://`)
	base64EmbedPattern   = regexp.MustCompile(`base64\s*,\s*[A-Za-z0-9+/]{100,}`)

	// Phase 4: CSS injection
	cssImportPattern = regexp.MustCompile(`@import\s+(?:url\s*\()?["']?https?://`)
	cssURLPattern    = regexp.MustCompile(`url\s*\(\s*["']?https?://`)
	styleTagPattern  = regexp.MustCompile(`<\s*style[\s>]`)

	// Phase 4: External <use> references
	useExternalPattern = regexp.MustCompile(`<\s*use[^>]+(?:href|xlink:href)\s*=\s*["']https?://`)
	useDataURIPattern  = regexp.MustCompile(`<\s*use[^>]+(?:href|xlink:href)\s*=\s*["']data:`)

	// Phase 4: Embedded HTML elements
	iframePattern     = regexp.MustCompile(`<\s*iframe[\s>]`)
	embedObjectPattern = regexp.MustCompile(`<\s*(?:embed|object)[\s>]`)
)
