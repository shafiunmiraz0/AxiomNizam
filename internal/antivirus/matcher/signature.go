package matcher

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// ClamAV .ndb format
//
// Format: MalwareName:TargetType:Offset:HexSig[:MinFL:[MaxFL]]
//
//   - MalwareName: The malware name string
//   - TargetType:  0=any, 1=PE, 2=OLE2, 3=HTML, 4=Mail, etc.
//   - Offset:      "*" (any), "0" (start), "n" (exact), "EOF-n" (from end)
//   - HexSig:      Hex-encoded byte pattern to match
//
// We parse the MalwareName, ignore TargetType/Offset (scan full content),
// and decode HexSig into a raw byte pattern.
// ─────────────────────────────────────────────────────────────────────────────

// LoadNDBFile parses a ClamAV .ndb signature file and adds all valid
// patterns to the builder.
func LoadNDBFile(b *Builder, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	return LoadNDB(b, f, filepath.Base(path))
}

// LoadNDB parses ClamAV .ndb signatures from a reader.
func LoadNDB(b *Builder, r io.Reader, source string) (int, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4096), 65536) // NDB lines can be long

	loaded := 0
	lineNum := 0
	skipped := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 4)
		if len(parts) < 4 {
			skipped++
			continue
		}

		malwareName := strings.TrimSpace(parts[0])
		// parts[1] = target type (ignored)
		// parts[2] = offset (ignored — we scan full content)
		hexSig := strings.TrimSpace(parts[3])

		// Remove any trailing fields after the hex signature.
		if colonIdx := strings.IndexByte(hexSig, ':'); colonIdx >= 0 {
			hexSig = hexSig[:colonIdx]
		}

		// ClamAV hex signatures may contain wildcards (??), alternations
		// ({n}, (a|b)), etc. We only support exact hex patterns for now.
		// Skip signatures with wildcard characters.
		if containsWildcard(hexSig) {
			skipped++
			continue
		}

		// Decode hex string to bytes.
		patternBytes, err := hex.DecodeString(hexSig)
		if err != nil {
			skipped++
			continue
		}

		// Skip very short patterns (high false-positive risk).
		if len(patternBytes) < 4 {
			skipped++
			continue
		}

		// Infer category from malware name.
		category := inferCategory(malwareName)

		b.AddPattern(SignatureInfo{
			ID:          fmt.Sprintf("ndb:%s", malwareName),
			Name:        malwareName,
			Pattern:     patternBytes,
			Category:    category,
			Severity:    inferSeverity(category),
			Confidence:  0.90,
			Description: fmt.Sprintf("ClamAV NDB signature match: %s", malwareName),
			Source:      source,
		})
		loaded++
	}

	if err := scanner.Err(); err != nil {
		return loaded, fmt.Errorf("scan error at line %d: %w", lineNum, err)
	}

	if skipped > 0 {
		logging.Z().Info(fmt.Sprintf("🛡️  matcher: skipped %d incompatible signatures in %s (wildcards/too-short/invalid-hex)",
			skipped, source))
	}

	return loaded, nil
}

// containsWildcard returns true if the hex signature contains ClamAV
// wildcard syntax that we don't support.
func containsWildcard(hexSig string) bool {
	return strings.ContainsAny(hexSig, "?*{}()|![]")
}

// ─────────────────────────────────────────────────────────────────────────────
// JSON format
//
// Our native format — an array of SignatureRecord objects, more expressive
// than ClamAV's line-based format.
// ─────────────────────────────────────────────────────────────────────────────

// SignatureRecord is the JSON representation of a pattern signature.
type SignatureRecord struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	HexPattern  string  `json:"hexPattern"`
	Category    string  `json:"category,omitempty"`
	Severity    string  `json:"severity,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
	Description string  `json:"description,omitempty"`
	Source      string  `json:"source,omitempty"`
}

// LoadJSONFile parses an AxiomNizam JSON signature file.
func LoadJSONFile(b *Builder, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	return LoadJSON(b, f, filepath.Base(path))
}

// LoadJSON parses JSON signature records from a reader.
func LoadJSON(b *Builder, r io.Reader, source string) (int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, fmt.Errorf("read JSON: %w", err)
	}

	var records []SignatureRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return 0, fmt.Errorf("parse JSON: %w", err)
	}

	loaded := 0
	for _, rec := range records {
		hexPattern := strings.TrimSpace(rec.HexPattern)
		if hexPattern == "" {
			continue
		}

		patternBytes, err := hex.DecodeString(hexPattern)
		if err != nil {
			continue
		}
		if len(patternBytes) < 4 {
			continue
		}

		category := antivirus.ThreatCategory(rec.Category)
		if category == "" {
			category = inferCategory(rec.Name)
		}
		severity := antivirus.ThreatSeverity(rec.Severity)
		if severity == "" {
			severity = inferSeverity(category)
		}
		confidence := rec.Confidence
		if confidence == 0 {
			confidence = 0.85
		}
		sigSource := source
		if rec.Source != "" {
			sigSource = rec.Source
		}

		b.AddPattern(SignatureInfo{
			ID:          rec.ID,
			Name:        rec.Name,
			Pattern:     patternBytes,
			Category:    category,
			Severity:    severity,
			Confidence:  confidence,
			Description: rec.Description,
			Source:      sigSource,
		})
		loaded++
	}

	return loaded, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Directory loader
// ─────────────────────────────────────────────────────────────────────────────

// LoadFromDir scans a directory for pattern signature files and loads them
// into the builder. Supported extensions: .ndb, .json
func LoadFromDir(b *Builder, dir string) (loaded int, errs []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Z().Info(fmt.Sprintf("🛡️  matcher: signature directory %q does not exist, skipping", dir))
			return 0, nil
		}
		return 0, []error{fmt.Errorf("read directory %q: %w", dir, err)}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		path := filepath.Join(dir, entry.Name())

		var count int
		var loadErr error

		switch ext {
		case ".ndb":
			count, loadErr = LoadNDBFile(b, path)
		case ".json":
			count, loadErr = LoadJSONFile(b, path)
		default:
			continue
		}

		if loadErr != nil {
			errs = append(errs, fmt.Errorf("load %q: %w", entry.Name(), loadErr))
			continue
		}

		loaded += count
		logging.Z().Info(fmt.Sprintf("🛡️  matcher: loaded %d patterns from %s", count, entry.Name()))
	}

	return loaded, errs
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// inferCategory determines threat category from a malware name using ClamAV
// naming conventions.
func inferCategory(name string) antivirus.ThreatCategory {
	lower := strings.ToLower(name)
	checks := []struct {
		substr   string
		category antivirus.ThreatCategory
	}{
		{"trojan", antivirus.CategoryTrojan},
		{"ransom", antivirus.CategoryRansomware},
		{"worm", antivirus.CategoryWorm},
		{"exploit", antivirus.CategoryExploit},
		{"backdoor", antivirus.CategoryBackdoor},
		{"coinminer", antivirus.CategoryCryptominer},
		{"miner", antivirus.CategoryCryptominer},
		{"webshell", antivirus.CategoryWebshell},
		{"dropper", antivirus.CategoryDropper},
		{"rootkit", antivirus.CategoryRootkit},
		{"adware", antivirus.CategoryAdware},
		{"spyware", antivirus.CategorySpyware},
		{"packed", antivirus.CategoryPacker},
	}
	for _, c := range checks {
		if strings.Contains(lower, c.substr) {
			return c.category
		}
	}
	return antivirus.CategoryGeneric
}

// inferSeverity returns an appropriate severity for a given category.
func inferSeverity(cat antivirus.ThreatCategory) antivirus.ThreatSeverity {
	switch cat {
	case antivirus.CategoryRansomware, antivirus.CategoryRootkit,
		antivirus.CategoryExploit, antivirus.CategoryBackdoor:
		return antivirus.SeverityCritical
	case antivirus.CategoryTrojan, antivirus.CategoryWorm,
		antivirus.CategoryDropper, antivirus.CategoryWebshell:
		return antivirus.SeverityHigh
	case antivirus.CategoryCryptominer, antivirus.CategorySpyware:
		return antivirus.SeverityMedium
	case antivirus.CategoryAdware, antivirus.CategoryPacker:
		return antivirus.SeverityLow
	default:
		return antivirus.SeverityMedium
	}
}
