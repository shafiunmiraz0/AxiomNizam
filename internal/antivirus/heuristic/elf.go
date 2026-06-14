package heuristic

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// ELF (Executable and Linkable Format) Heuristic Analyzer
//
// Detects suspicious Linux executable characteristics:
//   - Corrupted or anomalous ELF headers
//   - Executable stack (common in exploits and shellcode)
//   - Stripped binaries with executable stack (evasion indicator)
//   - Suspicious section names or missing standard sections
//   - UPX packing on Linux binaries
//   - High-entropy LOAD segments
// ─────────────────────────────────────────────────────────────────────────────

var elfMagic = []byte{0x7F, 0x45, 0x4C, 0x46} // "\x7fELF"

// ELF constants.
const (
	elfClass32      = 1
	elfClass64      = 2
	elfDataLSB      = 1 // little-endian
	elfDataMSB      = 2 // big-endian
	elfTypeExec     = 2
	elfTypeDyn      = 3
	phTypeLoad      = 1
	phTypeGnuStack  = 0x6474e551
	pfX             = 0x1 // PF_X: executable
	pfW             = 0x2 // PF_W: writable
	shTypeStrTab    = 3
)

func analyzeELF(target *antivirus.ScanTarget) []Finding {
	data := target.Content
	if len(data) < 64 || !bytes.HasPrefix(data, elfMagic) {
		return nil // Not an ELF file.
	}

	var findings []Finding

	class := data[4] // 1=32-bit, 2=64-bit
	if class != elfClass32 && class != elfClass64 {
		findings = append(findings, Finding{
			Name:        "Heuristic.ELF.InvalidClass",
			Description: fmt.Sprintf("Invalid ELF class byte: 0x%02X", class),
			Category:    antivirus.CategoryGeneric,
			Severity:    antivirus.SeverityMedium,
			Confidence:  0.65,
			Offset:      4,
		})
		return findings
	}

	encoding := data[5]
	if encoding != elfDataLSB && encoding != elfDataMSB {
		return findings
	}

	// We only parse little-endian for now (covers x86/x86_64/ARM LE).
	if encoding != elfDataLSB {
		return nil
	}

	is64 := class == elfClass64

	var (
		elfType   uint16
		phOff     uint64
		phEntSize uint16
		phNum     uint16
		shOff     uint64
		shEntSize uint16
		shNum     uint16
		shStrIdx  uint16
	)

	if is64 {
		if len(data) < 64 {
			return findings
		}
		elfType = binary.LittleEndian.Uint16(data[16:18])
		phOff = binary.LittleEndian.Uint64(data[32:40])
		phEntSize = binary.LittleEndian.Uint16(data[54:56])
		phNum = binary.LittleEndian.Uint16(data[56:58])
		shOff = binary.LittleEndian.Uint64(data[40:48])
		shEntSize = binary.LittleEndian.Uint16(data[58:60])
		shNum = binary.LittleEndian.Uint16(data[60:62])
		shStrIdx = binary.LittleEndian.Uint16(data[62:64])
	} else {
		if len(data) < 52 {
			return findings
		}
		elfType = binary.LittleEndian.Uint16(data[16:18])
		phOff = uint64(binary.LittleEndian.Uint32(data[28:32]))
		phEntSize = binary.LittleEndian.Uint16(data[42:44])
		phNum = binary.LittleEndian.Uint16(data[44:46])
		shOff = uint64(binary.LittleEndian.Uint32(data[32:36]))
		shEntSize = binary.LittleEndian.Uint16(data[46:48])
		shNum = binary.LittleEndian.Uint16(data[48:50])
		shStrIdx = binary.LittleEndian.Uint16(data[50:52])
	}

	// Only analyze executables and shared objects.
	if elfType != elfTypeExec && elfType != elfTypeDyn {
		return nil
	}

	// ── Program headers: check for executable stack ──────────────────
	execStack := false
	highEntSegments := 0

	for i := 0; i < int(phNum) && i < 128; i++ {
		off := int(phOff) + i*int(phEntSize)
		if is64 {
			if off+56 > len(data) {
				break
			}
			phType := binary.LittleEndian.Uint32(data[off : off+4])
			phFlags := binary.LittleEndian.Uint32(data[off+4 : off+8])
			phFileOff := binary.LittleEndian.Uint64(data[off+8 : off+16])
			phFileSize := binary.LittleEndian.Uint64(data[off+32 : off+40])

			if phType == phTypeGnuStack && (phFlags&pfX) != 0 {
				execStack = true
			}
			if phType == phTypeLoad && phFileSize > 256 && int(phFileOff+phFileSize) <= len(data) {
				ent := shannonEntropy(data[phFileOff : phFileOff+phFileSize])
				if ent > 7.2 {
					highEntSegments++
				}
			}
		} else {
			if off+32 > len(data) {
				break
			}
			phType := binary.LittleEndian.Uint32(data[off : off+4])
			phFileOff := binary.LittleEndian.Uint32(data[off+4 : off+8])
			phFileSize := binary.LittleEndian.Uint32(data[off+16 : off+20])
			phFlags := binary.LittleEndian.Uint32(data[off+24 : off+28])

			if phType == phTypeGnuStack && (phFlags&pfX) != 0 {
				execStack = true
			}
			if phType == phTypeLoad && phFileSize > 256 && int(phFileOff+phFileSize) <= len(data) {
				ent := shannonEntropy(data[phFileOff : phFileOff+phFileSize])
				if ent > 7.2 {
					highEntSegments++
				}
			}
		}
	}

	if execStack {
		findings = append(findings, Finding{
			Name:        "Heuristic.ELF.ExecutableStack",
			Description: "GNU_STACK segment is executable — common in exploits and shellcode",
			Category:    antivirus.CategoryExploit,
			Severity:    antivirus.SeverityHigh,
			Confidence:  0.70,
			Offset:      -1,
		})
	}

	if highEntSegments > 0 {
		findings = append(findings, Finding{
			Name:        "Heuristic.ELF.HighEntropySegment",
			Description: fmt.Sprintf("%d LOAD segment(s) with entropy >7.2 — possible packed binary", highEntSegments),
			Category:    antivirus.CategoryPacker,
			Severity:    antivirus.SeverityMedium,
			Confidence:  0.65,
			Offset:      -1,
		})
	}

	// ── Section headers: check names for packer indicators ───────────
	sectionNames := extractELFSectionNames(data, shOff, shEntSize, shNum, shStrIdx, is64)

	hasSymtab := false
	hasStrtab := false
	for _, name := range sectionNames {
		switch name {
		case ".symtab":
			hasSymtab = true
		case ".strtab":
			hasStrtab = true
		}

		// Check for UPX on Linux.
		if name == "UPX!" || name == "UPX0" || name == "UPX1" {
			findings = append(findings, Finding{
				Name:        "Heuristic.ELF.Packer.UPX",
				Description: "UPX packer detected on Linux ELF binary",
				Category:    antivirus.CategoryPacker,
				Severity:    antivirus.SeverityMedium,
				Confidence:  0.80,
				Offset:      -1,
				Metadata:    map[string]string{"packer": "UPX"},
			})
			break
		}
	}

	// Stripped binary + executable stack = evasion indicator.
	if !hasSymtab && !hasStrtab && execStack {
		findings = append(findings, Finding{
			Name:        "Heuristic.ELF.StrippedExecStack",
			Description: "Stripped binary (no symbol tables) with executable stack — evasion indicator",
			Category:    antivirus.CategoryGeneric,
			Severity:    antivirus.SeverityHigh,
			Confidence:  0.72,
			Offset:      -1,
		})
	}

	return findings
}

// extractELFSectionNames parses the section header string table to get
// section names. Returns an empty slice on any parse error.
func extractELFSectionNames(data []byte, shOff uint64, shEntSize, shNum, shStrIdx uint16, is64 bool) []string {
	if shNum == 0 || shStrIdx >= shNum || shEntSize == 0 {
		return nil
	}

	// Find the string table section header.
	strSecOff := int(shOff) + int(shStrIdx)*int(shEntSize)

	var strTabOff, strTabSize uint64
	if is64 {
		if strSecOff+64 > len(data) {
			return nil
		}
		strTabOff = binary.LittleEndian.Uint64(data[strSecOff+24 : strSecOff+32])
		strTabSize = binary.LittleEndian.Uint64(data[strSecOff+32 : strSecOff+40])
	} else {
		if strSecOff+40 > len(data) {
			return nil
		}
		strTabOff = uint64(binary.LittleEndian.Uint32(data[strSecOff+16 : strSecOff+20]))
		strTabSize = uint64(binary.LittleEndian.Uint32(data[strSecOff+20 : strSecOff+24]))
	}

	if int(strTabOff+strTabSize) > len(data) {
		return nil
	}
	strTab := data[strTabOff : strTabOff+strTabSize]

	names := make([]string, 0, int(shNum))
	for i := 0; i < int(shNum); i++ {
		secOff := int(shOff) + i*int(shEntSize)
		if is64 {
			if secOff+8 > len(data) {
				break
			}
		} else {
			if secOff+8 > len(data) {
				break
			}
		}
		nameIdx := binary.LittleEndian.Uint32(data[secOff : secOff+4])
		if int(nameIdx) < len(strTab) {
			end := bytes.IndexByte(strTab[nameIdx:], 0)
			if end < 0 {
				end = len(strTab) - int(nameIdx)
			}
			names = append(names, string(strTab[nameIdx:nameIdx+uint32(end)]))
		}
	}
	return names
}
