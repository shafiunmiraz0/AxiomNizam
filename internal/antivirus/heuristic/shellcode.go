package heuristic

import (
	"bytes"
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Shellcode / NOP Sled Detection
//
// Detects common shellcode patterns in non-executable files:
//   - NOP sleds (long runs of 0x90)
//   - Known syscall instruction sequences (x86/x64)
//   - Common shellcode prologue patterns
//   - INT 0x80 / SYSCALL instruction sequences in non-ELF files
// ─────────────────────────────────────────────────────────────────────────────

// Minimum NOP sled length to trigger detection.
const minNOPSledLength = 16

// Known shellcode byte sequences.
var shellcodePatterns = []struct {
	pattern []byte
	name    string
	desc    string
}{
	// x86 NOP
	{bytes.Repeat([]byte{0x90}, minNOPSledLength), "x86 NOP sled", "16+ consecutive NOP (0x90) instructions"},

	// INT 0x80 (Linux x86 syscall)
	{[]byte{0xCD, 0x80}, "INT 0x80 syscall", "Linux x86 interrupt-based syscall"},

	// SYSCALL (x86_64)
	{[]byte{0x0F, 0x05}, "SYSCALL instruction", "x86_64 syscall instruction"},

	// x86 shellcode prologue: XOR ECX,ECX; MUL ECX (zero registers)
	{[]byte{0x31, 0xC9, 0xF7, 0xE1}, "register zeroing", "XOR ECX,ECX; MUL ECX — common shellcode prologue"},

	// x86 shellcode: PUSH imm32; POP ESI (loading address)
	{[]byte{0x31, 0xC0, 0x50, 0x68}, "stack string setup", "XOR EAX,EAX; PUSH EAX; PUSH imm — shellcode string construction"},

	// XOR-decode loop: XOR [ESI], BL; INC ESI; LOOP
	{[]byte{0x30, 0x1E, 0x46, 0xE2, 0xFB}, "XOR decode loop", "XOR-based shellcode decoder loop"},

	// Windows API hashing: ROR EDX,13; ADD EDX,[ESI]
	{[]byte{0xC1, 0xCA, 0x0D, 0x03}, "API hash rotation", "ROR EDX,13 — Windows API name hashing (used by Metasploit)"},
}

func analyzeShellcode(target *antivirus.ScanTarget) []Finding {
	data := target.Content
	if len(data) < 8 {
		return nil
	}

	// Skip known executable formats — shellcode detection targets
	// non-executable files where embedded shellcode is suspicious.
	if isKnownExecutable(data) {
		return nil
	}

	var findings []Finding

	// Check for NOP sleds (variable length).
	if nopLen := longestNOPSled(data); nopLen >= minNOPSledLength {
		confidence := 0.65
		severity := antivirus.SeverityMedium
		if nopLen >= 64 {
			confidence = 0.85
			severity = antivirus.SeverityHigh
		} else if nopLen >= 32 {
			confidence = 0.75
			severity = antivirus.SeverityHigh
		}

		findings = append(findings, Finding{
			Name:        "Heuristic.Shellcode.NOPSled",
			Description: fmt.Sprintf("NOP sled detected: %d consecutive 0x90 bytes", nopLen),
			Category:    antivirus.CategoryExploit,
			Severity:    severity,
			Confidence:  confidence,
			Offset:      -1,
			Metadata:    map[string]string{"nopLength": fmt.Sprintf("%d", nopLen)},
		})
	}

	// Check for known shellcode patterns.
	patternScore := 0
	var matchedPatterns []string

	for _, sp := range shellcodePatterns {
		if sp.name == "x86 NOP sled" {
			continue // handled above with variable-length check
		}
		idx := bytes.Index(data, sp.pattern)
		if idx >= 0 {
			patternScore++
			matchedPatterns = append(matchedPatterns, sp.name)
		}
	}

	// Single syscall instruction is common in legitimate files; require
	// multiple indicators for detection.
	if patternScore >= 2 {
		confidence := 0.60 + float64(patternScore)*0.08
		if confidence > 0.90 {
			confidence = 0.90
		}

		findings = append(findings, Finding{
			Name:        "Heuristic.Shellcode.Patterns",
			Description: fmt.Sprintf("%d shellcode indicator(s) found: %s", patternScore, joinStrings(matchedPatterns)),
			Category:    antivirus.CategoryExploit,
			Severity:    antivirus.SeverityHigh,
			Confidence:  confidence,
			Offset:      -1,
			Metadata: map[string]string{
				"indicators": fmt.Sprintf("%d", patternScore),
				"patterns":   joinStrings(matchedPatterns),
			},
		})
	}

	return findings
}

// longestNOPSled finds the longest consecutive run of 0x90 bytes.
func longestNOPSled(data []byte) int {
	maxRun := 0
	currentRun := 0

	for _, b := range data {
		if b == 0x90 {
			currentRun++
			if currentRun > maxRun {
				maxRun = currentRun
			}
		} else {
			currentRun = 0
		}
	}

	return maxRun
}

// isKnownExecutable checks if the file is a PE or ELF binary.
// Shellcode detection is skipped for these (they have their own analyzers).
func isKnownExecutable(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	// PE: "MZ"
	if data[0] == 0x4D && data[1] == 0x5A {
		return true
	}
	// ELF: "\x7fELF"
	if data[0] == 0x7F && data[1] == 0x45 && data[2] == 0x4C && data[3] == 0x46 {
		return true
	}
	return false
}

func joinStrings(s []string) string {
	result := ""
	for i, str := range s {
		if i > 0 {
			result += ", "
		}
		result += str
	}
	return result
}
