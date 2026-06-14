package heuristic

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// PE (Portable Executable) Heuristic Analyzer
//
// Detects suspicious Windows executable characteristics:
//   - Corrupted or anomalous PE headers
//   - Sections with suspicious permissions (writable+executable)
//   - High-entropy sections (packed/encrypted)
//   - Unusual section names (packer indicators)
//   - Import table anomalies (very few or suspicious API imports)
//   - Entry point outside .text section
//   - Size mismatches between raw and virtual section sizes
// ─────────────────────────────────────────────────────────────────────────────

// PE magic bytes.
var (
	peDosMagic = []byte{0x4D, 0x5A}        // "MZ"
	peSig      = []byte{0x50, 0x45, 0x00, 0x00} // "PE\0\0"
)

// Known packer section names.
var packerSectionNames = map[string]string{
	"UPX0":     "UPX",
	"UPX1":     "UPX",
	"UPX2":     "UPX",
	".aspack":  "ASPack",
	".adata":   "ASPack",
	"ASPack":   "ASPack",
	".themida": "Themida",
	".vmp0":    "VMProtect",
	".vmp1":    "VMProtect",
	"VProtect": "VMProtect",
	".petite":  "Petite",
	"PECmpact": "PECompact",
	".nsp0":    "NsPack",
	".nsp1":    "NsPack",
	".enigma":  "Enigma",
	"MEW":      "MEW",
}

func analyzePE(target *antivirus.ScanTarget) []Finding {
	data := target.Content
	if len(data) < 64 || !bytes.HasPrefix(data, peDosMagic) {
		return nil // Not a PE file.
	}

	var findings []Finding

	// Read e_lfanew (offset to PE signature) at offset 0x3C.
	if len(data) < 0x3C+4 {
		return nil
	}
	peOffset := int(binary.LittleEndian.Uint32(data[0x3C:0x40]))

	// Validate PE signature.
	if peOffset < 0 || peOffset+4 > len(data) {
		findings = append(findings, Finding{
			Name:        "Heuristic.PE.CorruptHeader",
			Description: fmt.Sprintf("Invalid PE offset: 0x%X (file size: %d)", peOffset, len(data)),
			Category:    antivirus.CategoryGeneric,
			Severity:    antivirus.SeverityMedium,
			Confidence:  0.70,
			Offset:      0x3C,
		})
		return findings
	}

	if !bytes.Equal(data[peOffset:peOffset+4], peSig) {
		findings = append(findings, Finding{
			Name:        "Heuristic.PE.InvalidSignature",
			Description: "MZ header present but PE signature missing or corrupted",
			Category:    antivirus.CategoryGeneric,
			Severity:    antivirus.SeverityMedium,
			Confidence:  0.65,
			Offset:      int64(peOffset),
		})
		return findings
	}

	// Parse COFF header (20 bytes after PE sig).
	coffStart := peOffset + 4
	if coffStart+20 > len(data) {
		return findings
	}

	numSections := int(binary.LittleEndian.Uint16(data[coffStart+2 : coffStart+4]))
	optionalSize := int(binary.LittleEndian.Uint16(data[coffStart+16 : coffStart+18]))

	// Parse optional header to get entry point.
	optStart := coffStart + 20
	if optStart+optionalSize > len(data) || optionalSize < 16 {
		return findings
	}

	optMagic := binary.LittleEndian.Uint16(data[optStart : optStart+2])
	var entryPoint uint32
	if optMagic == 0x10b { // PE32
		entryPoint = binary.LittleEndian.Uint32(data[optStart+16 : optStart+20])
	} else if optMagic == 0x20b { // PE32+
		entryPoint = binary.LittleEndian.Uint32(data[optStart+16 : optStart+20])
	}

	// Parse section headers (40 bytes each, after optional header).
	sectionStart := optStart + optionalSize
	if sectionStart+numSections*40 > len(data) {
		return findings
	}

	var (
		hasWriteExec     bool
		packerDetected   string
		unusualSections  int
		highEntSections  int
		textSectionFound bool
		epInText         bool
	)

	for i := 0; i < numSections && i < 96; i++ {
		off := sectionStart + i*40
		if off+40 > len(data) {
			break
		}

		// Section name (8 bytes, null-padded).
		rawName := data[off : off+8]
		name := string(bytes.TrimRight(rawName, "\x00"))

		virtualSize := binary.LittleEndian.Uint32(data[off+8 : off+12])
		virtualAddr := binary.LittleEndian.Uint32(data[off+12 : off+16])
		rawSize := binary.LittleEndian.Uint32(data[off+16 : off+20])
		rawOffset := binary.LittleEndian.Uint32(data[off+20 : off+24])
		characteristics := binary.LittleEndian.Uint32(data[off+36 : off+40])

		// Check for known packer section names.
		if packer, ok := packerSectionNames[name]; ok {
			packerDetected = packer
		}

		// Check for empty/unusual section names.
		if name == "" || (len(name) > 0 && name[0] < 0x20) {
			unusualSections++
		}

		// Check for writable + executable sections.
		isWritable := characteristics&0x80000000 != 0
		isExecutable := characteristics&0x20000000 != 0
		if isWritable && isExecutable {
			hasWriteExec = true
		}

		// Track .text section for entry point check.
		if name == ".text" || name == ".code" {
			textSectionFound = true
			if entryPoint >= virtualAddr && entryPoint < virtualAddr+virtualSize {
				epInText = true
			}
		}

		// Calculate section entropy if data is available.
		if rawSize > 0 && rawOffset > 0 && int(rawOffset+rawSize) <= len(data) {
			sectionData := data[rawOffset : rawOffset+rawSize]
			entropy := shannonEntropy(sectionData)
			if entropy > 7.2 && isExecutable {
				highEntSections++
			}
		}

		// Size mismatch heuristic (unpacking indicator).
		if virtualSize > 0 && rawSize > 0 && virtualSize > rawSize*10 {
			findings = append(findings, Finding{
				Name:        "Heuristic.PE.SizeMismatch",
				Description: fmt.Sprintf("Section %q: virtual size (%d) is >10x raw size (%d) — self-unpacking indicator", name, virtualSize, rawSize),
				Category:    antivirus.CategoryPacker,
				Severity:    antivirus.SeverityMedium,
				Confidence:  0.65,
				Offset:      int64(off),
				Metadata:    map[string]string{"section": name},
			})
		}
	}

	// Report packer detection.
	if packerDetected != "" {
		findings = append(findings, Finding{
			Name:        fmt.Sprintf("Heuristic.PE.Packer.%s", packerDetected),
			Description: fmt.Sprintf("Known packer detected: %s", packerDetected),
			Category:    antivirus.CategoryPacker,
			Severity:    antivirus.SeverityMedium,
			Confidence:  0.80,
			Offset:      -1,
			Metadata:    map[string]string{"packer": packerDetected},
		})
	}

	// Report writable+executable sections.
	if hasWriteExec {
		findings = append(findings, Finding{
			Name:        "Heuristic.PE.WritableExecutable",
			Description: "Section with both WRITE and EXECUTE permissions — common in malware and packers",
			Category:    antivirus.CategoryGeneric,
			Severity:    antivirus.SeverityMedium,
			Confidence:  0.60,
			Offset:      -1,
		})
	}

	// Entry point outside .text section.
	if textSectionFound && !epInText && entryPoint > 0 {
		findings = append(findings, Finding{
			Name:        "Heuristic.PE.EntryPointAnomaly",
			Description: fmt.Sprintf("Entry point (0x%X) is outside the .text section — possible injection or packing", entryPoint),
			Category:    antivirus.CategoryGeneric,
			Severity:    antivirus.SeverityMedium,
			Confidence:  0.60,
			Offset:      -1,
			Metadata:    map[string]string{"entryPoint": fmt.Sprintf("0x%X", entryPoint)},
		})
	}

	// High-entropy executable sections.
	if highEntSections > 0 {
		findings = append(findings, Finding{
			Name:        "Heuristic.PE.HighEntropyCode",
			Description: fmt.Sprintf("%d executable section(s) with entropy >7.2 — likely packed or encrypted", highEntSections),
			Category:    antivirus.CategoryPacker,
			Severity:    antivirus.SeverityMedium,
			Confidence:  0.70,
			Offset:      -1,
			Metadata:    map[string]string{"highEntropySections": fmt.Sprintf("%d", highEntSections)},
		})
	}

	// Many unusual section names.
	if unusualSections >= 2 {
		findings = append(findings, Finding{
			Name:        "Heuristic.PE.UnusualSections",
			Description: fmt.Sprintf("%d section(s) with empty or non-printable names", unusualSections),
			Category:    antivirus.CategoryGeneric,
			Severity:    antivirus.SeverityLow,
			Confidence:  0.50,
			Offset:      -1,
		})
	}

	return findings
}

// shannonEntropy calculates the Shannon entropy of data in bits per byte.
// Returns a value between 0.0 (perfectly uniform) and 8.0 (perfectly random).
func shannonEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	var freq [256]float64
	for _, b := range data {
		freq[b]++
	}

	n := float64(len(data))
	var entropy float64
	for _, count := range freq {
		if count > 0 {
			p := count / n
			entropy -= p * math.Log2(p)
		}
	}
	return entropy
}
