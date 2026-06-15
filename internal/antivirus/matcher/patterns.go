package matcher

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"encoding/hex"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Built-in Patterns
//
// These critical patterns are compiled directly into the binary so the
// engine provides baseline detection even without any external signature
// database. They cover the most impactful threat categories for object
// storage security:
//
//   - EICAR test file (standard AV testing)
//   - PE executable markers (dropper/embedded EXE detection)
//   - ELF executable markers
//   - Ransomware markers
//   - Cryptominer pool URLs / identifiers
//   - Webshell indicators
//   - Common exploit payloads
//   - Reverse shell patterns
// ─────────────────────────────────────────────────────────────────────────────

// builtinPattern is a compact definition for built-in signatures.
type builtinPattern struct {
	id          string
	name        string
	hexPattern  string
	category    antivirus.ThreatCategory
	severity    antivirus.ThreatSeverity
	confidence  float64
	description string
}

// builtinPatterns contains curated, high-value signatures.
// Each hex string is decoded at registration time.
var builtinPatterns = []builtinPattern{

	// ─────────────────────────────────────────────────────────────────────
	// EICAR Test File
	// The EICAR Anti-Malware Test File is a standard test string used by
	// every AV engine. It is NOT malware — it simply triggers detection
	// to verify the scanner is working.
	// ─────────────────────────────────────────────────────────────────────
	{
		id:         "builtin:eicar-test",
		name:       "EICAR-Test-File",
		hexPattern: "58354f2150254041505b345c505a5835" + "3428505e2937434329377d2445494341" + "522d5354414e444152442d414e544956" + "495255532d544553542d46494c4521",
		category:   antivirus.CategoryGeneric,
		severity:   antivirus.SeverityLow,
		confidence: 1.0,
		description: "EICAR anti-malware test file (not real malware, used for AV testing)",
	},

	// ─────────────────────────────────────────────────────────────────────
	// Ransomware Indicators
	// ─────────────────────────────────────────────────────────────────────
	{
		id:          "builtin:ransom-wannacry-marker",
		name:        "Ransom.WannaCry.Marker",
		hexPattern:  "57616e6e614372797074",  // "WannaCrypt"
		category:    antivirus.CategoryRansomware,
		severity:    antivirus.SeverityCritical,
		confidence:  0.90,
		description: "WannaCry ransomware identifier string",
	},
	{
		id:          "builtin:ransom-wannacry-killswitch",
		name:        "Ransom.WannaCry.KillSwitch",
		hexPattern:  "69757172657277696473666e726d616c73656d69",  // "iuqerwidfnrmalssemi" (part of kill switch domain)
		category:    antivirus.CategoryRansomware,
		severity:    antivirus.SeverityCritical,
		confidence:  0.95,
		description: "WannaCry kill switch domain fragment",
	},
	{
		id:          "builtin:ransom-note-generic",
		name:        "Ransom.Note.Generic",
		hexPattern:  "596f75722066696c6573206861766520" + "6265656e20656e637279707465",  // "Your files have been encrypte"
		category:    antivirus.CategoryRansomware,
		severity:    antivirus.SeverityCritical,
		confidence:  0.85,
		description: "Generic ransomware note text pattern",
	},
	{
		id:          "builtin:ransom-bitcoin-demand",
		name:        "Ransom.BitcoinDemand",
		hexPattern:  "73656e6420626974636f696e",  // "send bitcoin"
		category:    antivirus.CategoryRansomware,
		severity:    antivirus.SeverityHigh,
		confidence:  0.75,
		description: "Bitcoin ransom demand text",
	},

	// ─────────────────────────────────────────────────────────────────────
	// Cryptominer Indicators
	// ─────────────────────────────────────────────────────────────────────
	{
		id:          "builtin:miner-stratum-protocol",
		name:        "CoinMiner.Stratum.Protocol",
		hexPattern:  "7374726174756d2b746370",  // "stratum+tcp"
		category:    antivirus.CategoryCryptominer,
		severity:    antivirus.SeverityHigh,
		confidence:  0.90,
		description: "Stratum mining protocol URL scheme",
	},
	{
		id:          "builtin:miner-xmrig-id",
		name:        "CoinMiner.XMRig.Identifier",
		hexPattern:  "786d7269672f",  // "xmrig/"
		category:    antivirus.CategoryCryptominer,
		severity:    antivirus.SeverityHigh,
		confidence:  0.92,
		description: "XMRig cryptocurrency miner identifier",
	},
	{
		id:          "builtin:miner-moneropool",
		name:        "CoinMiner.MoneroPool",
		hexPattern:  "6d696e652e6d6f6e65726f706f6f6c",  // "mine.moneropool"
		category:    antivirus.CategoryCryptominer,
		severity:    antivirus.SeverityHigh,
		confidence:  0.92,
		description: "Monero mining pool URL",
	},
	{
		id:          "builtin:miner-coinhive",
		name:        "CoinMiner.CoinHive",
		hexPattern:  "436f696e486976652e416e6f6e796d6f7573",  // "CoinHive.Anonymous"
		category:    antivirus.CategoryCryptominer,
		severity:    antivirus.SeverityMedium,
		confidence:  0.92,
		description: "CoinHive browser-based cryptocurrency miner",
	},

	// ─────────────────────────────────────────────────────────────────────
	// Webshell Indicators
	// ─────────────────────────────────────────────────────────────────────
	{
		id:          "builtin:webshell-php-system",
		name:        "Webshell.PHP.System",
		hexPattern:  "3c3f70687020" + "73797374656d28245f474554",  // "<?php system($_GET"
		category:    antivirus.CategoryWebshell,
		severity:    antivirus.SeverityCritical,
		confidence:  0.92,
		description: "PHP webshell using system() with GET parameter",
	},
	{
		id:          "builtin:webshell-php-eval-post",
		name:        "Webshell.PHP.EvalPost",
		hexPattern:  "6576616c28626173653634" + "5f6465636f646528",  // "eval(base64_decode("
		category:    antivirus.CategoryWebshell,
		severity:    antivirus.SeverityCritical,
		confidence:  0.90,
		description: "PHP eval with base64 decode — common webshell pattern",
	},
	{
		id:          "builtin:webshell-php-passthru",
		name:        "Webshell.PHP.Passthru",
		hexPattern:  "706173737468727528245f504f5354",  // "passthru($_POST"
		category:    antivirus.CategoryWebshell,
		severity:    antivirus.SeverityCritical,
		confidence:  0.92,
		description: "PHP webshell using passthru() with POST parameter",
	},
	{
		id:          "builtin:webshell-jsp-runtime",
		name:        "Webshell.JSP.Runtime",
		hexPattern:  "52756e74696d652e676574" + "52756e74696d6528292e65786563",  // "Runtime.getRuntime().exec"
		category:    antivirus.CategoryWebshell,
		severity:    antivirus.SeverityCritical,
		confidence:  0.88,
		description: "JSP webshell using Runtime.exec()",
	},

	// ─────────────────────────────────────────────────────────────────────
	// Reverse Shell Indicators
	// ─────────────────────────────────────────────────────────────────────
	{
		id:          "builtin:revshell-bash-tcp",
		name:        "Backdoor.BashReverseShell",
		hexPattern:  "2f6465762f7463702f",  // "/dev/tcp/"
		category:    antivirus.CategoryBackdoor,
		severity:    antivirus.SeverityCritical,
		confidence:  0.85,
		description: "Bash reverse shell using /dev/tcp",
	},
	{
		id:          "builtin:revshell-mkfifo",
		name:        "Backdoor.MkfifoShell",
		hexPattern:  "6d6b6669666f202f746d702f",  // "mkfifo /tmp/"
		category:    antivirus.CategoryBackdoor,
		severity:    antivirus.SeverityHigh,
		confidence:  0.82,
		description: "Reverse shell using mkfifo named pipe",
	},
	{
		id:          "builtin:revshell-python-socket",
		name:        "Backdoor.PythonSocket",
		hexPattern:  "696d706f727420736f636b6574" + "2c7375627072",  // "import socket,subpr"
		category:    antivirus.CategoryBackdoor,
		severity:    antivirus.SeverityCritical,
		confidence:  0.82,
		description: "Python reverse shell using socket+subprocess",
	},

	// ─────────────────────────────────────────────────────────────────────
	// Common Exploit Payloads
	// ─────────────────────────────────────────────────────────────────────
	{
		id:          "builtin:exploit-log4shell",
		name:        "Exploit.Log4Shell",
		hexPattern:  "247b6a6e64693a6c6461703a2f2f",  // "${jndi:ldap://"
		category:    antivirus.CategoryExploit,
		severity:    antivirus.SeverityCritical,
		confidence:  0.95,
		description: "Log4Shell (CVE-2021-44228) JNDI injection payload",
	},
	{
		id:          "builtin:exploit-log4shell-rmi",
		name:        "Exploit.Log4Shell.RMI",
		hexPattern:  "247b6a6e64693a726d693a2f2f",  // "${jndi:rmi://"
		category:    antivirus.CategoryExploit,
		severity:    antivirus.SeverityCritical,
		confidence:  0.95,
		description: "Log4Shell RMI variant payload",
	},
	{
		id:          "builtin:exploit-shellshock",
		name:        "Exploit.ShellShock",
		hexPattern:  "2829207b203a3b207d3b",  // "() { :; };"
		category:    antivirus.CategoryExploit,
		severity:    antivirus.SeverityCritical,
		confidence:  0.92,
		description: "Shellshock (CVE-2014-6271) bash exploit payload",
	},

	// ─────────────────────────────────────────────────────────────────────
	// PE/ELF Embedded Executable Detection
	// (Flags executables hidden inside non-executable uploads)
	// ─────────────────────────────────────────────────────────────────────
	{
		id:          "builtin:suspicious-embedded-pe",
		name:        "Suspicious.EmbeddedPE",
		hexPattern:  "4d5a90000300000004000000ffff",  // MZ header + standard PE stub
		category:    antivirus.CategoryDropper,
		severity:    antivirus.SeverityMedium,
		confidence:  0.70,
		description: "Windows PE executable header embedded in file",
	},
	{
		id:          "builtin:suspicious-embedded-elf",
		name:        "Suspicious.EmbeddedELF",
		hexPattern:  "7f454c4602",  // ELF magic + 64-bit class
		category:    antivirus.CategoryDropper,
		severity:    antivirus.SeverityMedium,
		confidence:  0.65,
		description: "Linux ELF 64-bit executable header embedded in file",
	},

	// ─────────────────────────────────────────────────────────────────────
	// Metasploit / Cobalt Strike Markers
	// ─────────────────────────────────────────────────────────────────────
	{
		id:          "builtin:exploit-meterpreter",
		name:        "Exploit.Meterpreter.Marker",
		hexPattern:  "6d657465727072657465725f",  // "meterpreter_"
		category:    antivirus.CategoryExploit,
		severity:    antivirus.SeverityCritical,
		confidence:  0.92,
		description: "Metasploit Meterpreter payload marker",
	},
	{
		id:          "builtin:exploit-cobaltstrike",
		name:        "Exploit.CobaltStrike.Beacon",
		hexPattern:  "253732732530307325",  // "%72s%00s%" — Cobalt Strike beacon config marker
		category:    antivirus.CategoryExploit,
		severity:    antivirus.SeverityCritical,
		confidence:  0.88,
		description: "Cobalt Strike beacon configuration marker",
	},
}

// ─────────────────────────────────────────────────────────────────────────────
// Registration
// ─────────────────────────────────────────────────────────────────────────────

// RegisterBuiltinPatterns adds all built-in patterns to the builder.
// Returns the number of patterns successfully registered.
func RegisterBuiltinPatterns(b *Builder) int {
	loaded := 0

	for _, bp := range builtinPatterns {
		patternBytes, err := hex.DecodeString(bp.hexPattern)
		if err != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  matcher: invalid builtin pattern %q: %v", bp.id, err))
			continue
		}
		if len(patternBytes) < 4 {
			logging.Z().Info(fmt.Sprintf("⚠️  matcher: builtin pattern %q too short (%d bytes)", bp.id, len(patternBytes)))
			continue
		}

		b.AddPattern(SignatureInfo{
			ID:          bp.id,
			Name:        bp.name,
			Pattern:     patternBytes,
			Category:    bp.category,
			Severity:    bp.severity,
			Confidence:  bp.confidence,
			Description: bp.description,
			Source:      "builtin",
		})
		loaded++
	}

	return loaded
}

// BuiltinPatternCount returns the number of built-in patterns available.
func BuiltinPatternCount() int {
	return len(builtinPatterns)
}
