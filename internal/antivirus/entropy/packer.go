package entropy

import (
	"bytes"
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Packer / Crypter Detection via Magic Bytes
//
// Known packers and protectors insert identifiable headers or markers
// into the packed binary. We check for these independently of entropy
// analysis because they are high-confidence indicators.
// ─────────────────────────────────────────────────────────────────────────────

// packerSignature defines a magic-byte pattern for a known packer.
type packerSignature struct {
	name       string
	magic      []byte
	offset     int  // fixed offset to check (-1 = anywhere in first 4KB)
	confidence float64
	severity   antivirus.ThreatSeverity
	desc       string
}

// packerSignatures contains known packer/protector magic bytes.
// Order: check fixed-offset patterns first (faster), then search patterns.
var packerSignatures = []packerSignature{
	// ── UPX ──────────────────────────────────────────────────────────
	{
		name:       "UPX",
		magic:      []byte("UPX!"),
		offset:     -1,
		confidence: 0.85,
		severity:   antivirus.SeverityMedium,
		desc:       "UPX packer header detected",
	},
	{
		name:       "UPX",
		magic:      []byte("UPX0"),
		offset:     -1,
		confidence: 0.82,
		severity:   antivirus.SeverityMedium,
		desc:       "UPX section marker detected",
	},

	// ── Themida / WinLicense ─────────────────────────────────────────
	{
		name:       "Themida",
		magic:      []byte(".themida"),
		offset:     -1,
		confidence: 0.88,
		severity:   antivirus.SeverityHigh,
		desc:       "Themida/WinLicense protector section detected",
	},

	// ── VMProtect ────────────────────────────────────────────────────
	{
		name:       "VMProtect",
		magic:      []byte(".vmp0"),
		offset:     -1,
		confidence: 0.88,
		severity:   antivirus.SeverityHigh,
		desc:       "VMProtect protector section detected",
	},
	{
		name:       "VMProtect",
		magic:      []byte(".vmp1"),
		offset:     -1,
		confidence: 0.88,
		severity:   antivirus.SeverityHigh,
		desc:       "VMProtect protector section detected",
	},

	// ── ASPack ───────────────────────────────────────────────────────
	{
		name:       "ASPack",
		magic:      []byte(".aspack"),
		offset:     -1,
		confidence: 0.82,
		severity:   antivirus.SeverityMedium,
		desc:       "ASPack packer section detected",
	},
	{
		name:       "ASPack",
		magic:      []byte(".adata"),
		offset:     -1,
		confidence: 0.70,
		severity:   antivirus.SeverityMedium,
		desc:       "ASPack data section detected",
	},

	// ── PECompact ────────────────────────────────────────────────────
	{
		name:       "PECompact",
		magic:      []byte("PECompact2"),
		offset:     -1,
		confidence: 0.85,
		severity:   antivirus.SeverityMedium,
		desc:       "PECompact packer marker detected",
	},

	// ── Petite ───────────────────────────────────────────────────────
	{
		name:       "Petite",
		magic:      []byte(".petite"),
		offset:     -1,
		confidence: 0.82,
		severity:   antivirus.SeverityMedium,
		desc:       "Petite packer section detected",
	},

	// ── NsPack ───────────────────────────────────────────────────────
	{
		name:       "NsPack",
		magic:      []byte(".nsp0"),
		offset:     -1,
		confidence: 0.80,
		severity:   antivirus.SeverityMedium,
		desc:       "NsPack packer section detected",
	},

	// ── Enigma Protector ─────────────────────────────────────────────
	{
		name:       "Enigma",
		magic:      []byte(".enigma"),
		offset:     -1,
		confidence: 0.85,
		severity:   antivirus.SeverityHigh,
		desc:       "Enigma Protector section detected",
	},

	// ── MPRESS ───────────────────────────────────────────────────────
	{
		name:       "MPRESS",
		magic:      []byte(".MPRESS1"),
		offset:     -1,
		confidence: 0.82,
		severity:   antivirus.SeverityMedium,
		desc:       "MPRESS packer section detected",
	},
	{
		name:       "MPRESS",
		magic:      []byte(".MPRESS2"),
		offset:     -1,
		confidence: 0.82,
		severity:   antivirus.SeverityMedium,
		desc:       "MPRESS packer section detected",
	},

	// ── ConfuserEx (.NET obfuscator) ─────────────────────────────────
	{
		name:       "ConfuserEx",
		magic:      []byte("ConfuserEx"),
		offset:     -1,
		confidence: 0.80,
		severity:   antivirus.SeverityMedium,
		desc:       "ConfuserEx .NET obfuscator marker detected",
	},
}

// maxScanRange limits the packer magic byte search to the first N bytes
// for performance. Most packer headers appear in the first 4KB.
const maxScanRange = 4096

// detectPackers scans for known packer magic bytes in the file data.
func detectPackers(data []byte) []antivirus.ThreatInfo {
	if len(data) < 4 {
		return nil
	}

	scanRange := data
	if len(scanRange) > maxScanRange {
		scanRange = scanRange[:maxScanRange]
	}

	// Deduplicate: report each packer name only once.
	seen := make(map[string]struct{})
	var threats []antivirus.ThreatInfo

	for _, sig := range packerSignatures {
		if _, dup := seen[sig.name]; dup {
			continue
		}

		var found bool
		if sig.offset >= 0 {
			// Fixed offset check.
			end := sig.offset + len(sig.magic)
			if end <= len(data) {
				found = bytes.Equal(data[sig.offset:end], sig.magic)
			}
		} else {
			// Search in scan range.
			found = bytes.Contains(scanRange, sig.magic)
		}

		if found {
			seen[sig.name] = struct{}{}
			threats = append(threats, antivirus.ThreatInfo{
				Name:        fmt.Sprintf("Entropy.Packer.%s", sig.name),
				Category:    antivirus.CategoryPacker,
				Severity:    sig.severity,
				Layer:       antivirus.LayerEntropy,
				Description: sig.desc,
				Confidence:  sig.confidence,
				Metadata: map[string]string{
					"packer": sig.name,
				},
			})
		}
	}

	return threats
}

// PackerCount returns the number of packer signatures available.
func PackerCount() int {
	return len(packerSignatures)
}
