package heuristic

import (
	"bytes"
	"fmt"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Script Obfuscation Heuristic Analyzer
//
// Detects obfuscated scripts across multiple languages:
//   - JavaScript: eval chains, String.fromCharCode arrays, hex escapes
//   - PowerShell: -EncodedCommand, IEX chains, char-code concatenation
//   - VBScript: Chr() concatenation, Execute/ExecuteGlobal
//   - PHP: eval(base64_decode()), preg_replace /e modifier
//   - Bash: eval with base64, hex-encoded commands
// ─────────────────────────────────────────────────────────────────────────────

// Script detection thresholds.
const (
	maxScriptSize       = 2 * 1024 * 1024 // 2 MB — skip very large files
	minObfuscationScore = 3               // at least 3 indicators needed
)

// scriptIndicator defines a single obfuscation pattern to look for.
type scriptIndicator struct {
	pattern    []byte
	name       string
	weight     int // how many "score points" this adds
	confidence float64
}

var jsIndicators = []scriptIndicator{
	{[]byte("eval("), "eval()", 1, 0.50},
	{[]byte("String.fromCharCode"), "String.fromCharCode", 2, 0.70},
	{[]byte("\\x"), "hex escape sequences", 1, 0.40},
	{[]byte("unescape("), "unescape()", 1, 0.50},
	{[]byte("document.write(unescape"), "document.write+unescape", 2, 0.75},
	{[]byte("atob("), "atob() base64 decode", 1, 0.50},
	{[]byte("Function("), "Function constructor", 2, 0.65},
	{[]byte("setTimeout("), "setTimeout for deferred eval", 1, 0.30},
	{[]byte("\\u00"), "unicode escapes", 1, 0.40},
}

var psIndicators = []scriptIndicator{
	{[]byte("-EncodedCommand"), "EncodedCommand", 3, 0.80},
	{[]byte("-enc "), "Encoded shorthand", 3, 0.80},
	{[]byte("IEX"), "Invoke-Expression", 2, 0.65},
	{[]byte("Invoke-Expression"), "Invoke-Expression (full)", 2, 0.65},
	{[]byte("[Convert]::FromBase64String"), "Base64 decode", 2, 0.70},
	{[]byte("DownloadString"), "Web download + exec", 2, 0.75},
	{[]byte("New-Object Net.WebClient"), "WebClient creation", 2, 0.60},
	{[]byte("[char]"), "Char-code obfuscation", 1, 0.50},
	{[]byte("bypass"), "Execution policy bypass", 1, 0.40},
	{[]byte("-nop"), "No profile flag", 1, 0.35},
	{[]byte("-w hidden"), "Hidden window", 2, 0.60},
}

var vbsIndicators = []scriptIndicator{
	{[]byte("Execute("), "Execute()", 2, 0.70},
	{[]byte("ExecuteGlobal"), "ExecuteGlobal()", 2, 0.75},
	{[]byte("Chr("), "Chr() concatenation", 1, 0.50},
	{[]byte("WScript.Shell"), "Shell execution", 2, 0.60},
	{[]byte("Scripting.FileSystemObject"), "Filesystem access", 1, 0.40},
}

var bashIndicators = []scriptIndicator{
	{[]byte("eval "), "eval command", 1, 0.40},
	{[]byte("base64 -d"), "base64 decode pipe", 2, 0.65},
	{[]byte("base64 --decode"), "base64 decode pipe", 2, 0.65},
	{[]byte("$(echo"), "command substitution with echo", 1, 0.40},
	{[]byte("\\x"), "hex escape in bash", 1, 0.40},
	{[]byte("/dev/tcp/"), "TCP redirect (reverse shell)", 3, 0.85},
	{[]byte("curl "), "curl download", 1, 0.30},
	{[]byte("wget "), "wget download", 1, 0.30},
}

func analyzeScript(target *antivirus.ScanTarget) []Finding {
	data := target.Content
	if len(data) == 0 || int64(len(data)) > maxScriptSize {
		return nil
	}

	// Determine script type from MIME type and content.
	mime := strings.ToLower(target.MIMEType)
	lower := bytes.ToLower(data)

	var findings []Finding

	// JavaScript detection.
	if isJavaScript(mime, data, lower) {
		if f := checkIndicators(lower, jsIndicators, "JavaScript", antivirus.CategoryGeneric); f != nil {
			findings = append(findings, *f)
		}
		if f := checkCharCodeDensity(lower); f != nil {
			findings = append(findings, *f)
		}
	}

	// PowerShell detection.
	if isPowerShell(mime, target.Filename, lower) {
		if f := checkIndicators(lower, psIndicators, "PowerShell", antivirus.CategoryGeneric); f != nil {
			findings = append(findings, *f)
		}
	}

	// VBScript detection.
	if isVBScript(target.Filename, lower) {
		if f := checkIndicators(lower, vbsIndicators, "VBScript", antivirus.CategoryGeneric); f != nil {
			findings = append(findings, *f)
		}
	}

	// Bash detection.
	if isBashScript(data, lower) {
		if f := checkIndicators(lower, bashIndicators, "Bash", antivirus.CategoryGeneric); f != nil {
			findings = append(findings, *f)
		}
	}

	// Generic: excessive base64 content.
	if f := checkExcessiveBase64(data); f != nil {
		findings = append(findings, *f)
	}

	return findings
}

// checkIndicators scores a file against a set of script indicators.
func checkIndicators(lower []byte, indicators []scriptIndicator, lang string, cat antivirus.ThreatCategory) *Finding {
	totalScore := 0
	maxConf := 0.0
	matched := 0
	var details []string

	for _, ind := range indicators {
		count := bytes.Count(lower, bytes.ToLower(ind.pattern))
		if count > 0 {
			totalScore += ind.weight * count
			matched++
			if ind.confidence > maxConf {
				maxConf = ind.confidence
			}
			if count > 3 {
				details = append(details, fmt.Sprintf("%s (×%d)", ind.name, count))
			} else {
				details = append(details, ind.name)
			}
		}
	}

	if totalScore < minObfuscationScore || matched < 2 {
		return nil
	}

	// Scale confidence by match density.
	confidence := maxConf
	if totalScore >= 10 {
		confidence = min64(confidence+0.15, 0.95)
	}

	return &Finding{
		Name:        fmt.Sprintf("Heuristic.Script.%s.Obfuscation", lang),
		Description: fmt.Sprintf("Obfuscated %s detected (score=%d): %s", lang, totalScore, strings.Join(details, ", ")),
		Category:    cat,
		Severity:    antivirus.SeverityHigh,
		Confidence:  confidence,
		Offset:      -1,
		Metadata: map[string]string{
			"language":   lang,
			"score":      fmt.Sprintf("%d", totalScore),
			"indicators": fmt.Sprintf("%d", matched),
		},
	}
}

// checkCharCodeDensity flags JavaScript with excessive String.fromCharCode usage.
func checkCharCodeDensity(lower []byte) *Finding {
	count := bytes.Count(lower, []byte("fromcharcode"))
	if count < 5 {
		return nil
	}
	return &Finding{
		Name:        "Heuristic.Script.CharCodeArray",
		Description: fmt.Sprintf("Excessive String.fromCharCode usage (%d occurrences) — character-code obfuscation", count),
		Category:    antivirus.CategoryGeneric,
		Severity:    antivirus.SeverityHigh,
		Confidence:  0.80,
		Offset:      -1,
		Metadata:    map[string]string{"count": fmt.Sprintf("%d", count)},
	}
}

// checkExcessiveBase64 detects files that are mostly base64-encoded content.
func checkExcessiveBase64(data []byte) *Finding {
	if len(data) < 500 {
		return nil
	}
	b64Chars := 0
	for _, b := range data {
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') ||
			(b >= '0' && b <= '9') || b == '+' || b == '/' || b == '=' {
			b64Chars++
		}
	}
	ratio := float64(b64Chars) / float64(len(data))
	if ratio > 0.85 && len(data) > 1000 {
		return &Finding{
			Name:        "Heuristic.Script.ExcessiveBase64",
			Description: fmt.Sprintf("%.0f%% of file content is base64-compatible characters — possible encoded payload", ratio*100),
			Category:    antivirus.CategoryGeneric,
			Severity:    antivirus.SeverityMedium,
			Confidence:  0.55,
			Offset:      -1,
			Metadata:    map[string]string{"ratio": fmt.Sprintf("%.2f", ratio)},
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Script type detection
// ─────────────────────────────────────────────────────────────────────────────

func isJavaScript(mime string, data, lower []byte) bool {
	if strings.Contains(mime, "javascript") || strings.Contains(mime, "ecmascript") {
		return true
	}
	if bytes.HasPrefix(lower, []byte("<script")) || bytes.Contains(lower[:min(len(lower), 512)], []byte("function ")) {
		return true
	}
	return false
}

func isPowerShell(mime, filename string, lower []byte) bool {
	fn := strings.ToLower(filename)
	if strings.HasSuffix(fn, ".ps1") || strings.HasSuffix(fn, ".psm1") || strings.HasSuffix(fn, ".psd1") {
		return true
	}
	if bytes.Contains(lower[:min(len(lower), 256)], []byte("powershell")) {
		return true
	}
	return false
}

func isVBScript(filename string, lower []byte) bool {
	fn := strings.ToLower(filename)
	if strings.HasSuffix(fn, ".vbs") || strings.HasSuffix(fn, ".vbe") || strings.HasSuffix(fn, ".wsf") {
		return true
	}
	if bytes.Contains(lower[:min(len(lower), 256)], []byte("wscript")) {
		return true
	}
	return false
}

func isBashScript(data, lower []byte) bool {
	if bytes.HasPrefix(data, []byte("#!/bin/bash")) || bytes.HasPrefix(data, []byte("#!/bin/sh")) {
		return true
	}
	if bytes.HasPrefix(data, []byte("#!/usr/bin/env bash")) || bytes.HasPrefix(data, []byte("#!/usr/bin/env sh")) {
		return true
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
