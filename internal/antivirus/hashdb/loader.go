package hashdb

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Loader — file format parsers
// ─────────────────────────────────────────────────────────────────────────────

// LoadFromDir scans a directory for hash database files and loads them all
// into the provided DB. Supported formats:
//
//   - .hdb / .hsb  — ClamAV hash signature format (HashString:FileSize:MalwareName)
//   - .json        — AxiomNizam JSON format (array of HashRecord objects)
//   - .txt         — Plain text, one SHA-256 hash per line (optionally with malware name)
//
// Returns the total number of hashes loaded and any non-fatal errors encountered.
func LoadFromDir(db *DB, dir string) (loaded int, errs []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Z().Info(fmt.Sprintf("🛡️  hashdb: signature directory %q does not exist, skipping", dir))
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
		case ".hdb", ".hsb":
			count, loadErr = LoadClamAVFile(db, path)
		case ".json":
			count, loadErr = LoadJSONFile(db, path)
		case ".txt":
			count, loadErr = LoadTextFile(db, path)
		default:
			continue // skip unrecognised extensions
		}

		if loadErr != nil {
			errs = append(errs, fmt.Errorf("load %q: %w", entry.Name(), loadErr))
			continue
		}

		loaded += count
		logging.Z().Info(fmt.Sprintf("🛡️  hashdb: loaded %d hashes from %s", count, entry.Name()))
	}

	return loaded, errs
}

// ─────────────────────────────────────────────────────────────────────────────
// ClamAV format (.hdb / .hsb)
//
// Format: HashString:FileSize:MalwareName
//
// SHA-256 hashes are identified by 64-character hex strings.
// MD5 (32 chars) and SHA-1 (40 chars) are skipped — we only use SHA-256.
// FileSize may be "*" (wildcard = 0).
// ─────────────────────────────────────────────────────────────────────────────

// LoadClamAVFile parses a ClamAV .hdb/.hsb file and adds SHA-256 hashes
// to the database. Non-SHA-256 hashes (MD5, SHA-1) are silently skipped.
func LoadClamAVFile(db *DB, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	return LoadClamAV(db, f, filepath.Base(path))
}

// LoadClamAV parses ClamAV hash signatures from a reader.
func LoadClamAV(db *DB, r io.Reader, source string) (int, error) {
	scanner := bufio.NewScanner(r)
	// ClamAV files can have very long lines in some signature types,
	// but hash files are typically short. Set a generous limit.
	scanner.Buffer(make([]byte, 0, 1024), 4096)

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

		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			skipped++
			continue
		}

		hashStr := strings.TrimSpace(parts[0])
		sizeStr := strings.TrimSpace(parts[1])
		malwareName := strings.TrimSpace(parts[2])

		// Only accept SHA-256 hashes (64 hex characters).
		if len(hashStr) != 64 {
			continue // MD5 (32) or SHA-1 (40) — skip
		}

		// Validate hex characters.
		if !isHexString(hashStr) {
			skipped++
			continue
		}

		// Parse file size ("*" means wildcard).
		var fileSize int64
		if sizeStr != "*" && sizeStr != "" {
			parsed, err := strconv.ParseInt(sizeStr, 10, 64)
			if err != nil {
				skipped++
				continue
			}
			fileSize = parsed
		}

		// Normalise hash to lowercase.
		hashStr = strings.ToLower(hashStr)

		// Infer threat category from ClamAV naming convention.
		category := inferCategoryFromName(malwareName)

		db.Add(hashStr, HashEntry{
			MalwareName: malwareName,
			FileSize:    fileSize,
			Category:    category,
			Source:      source,
		})
		loaded++
	}

	if err := scanner.Err(); err != nil {
		return loaded, fmt.Errorf("scan error at line %d: %w", lineNum, err)
	}

	if skipped > 0 {
		logging.Z().Info(fmt.Sprintf("🛡️  hashdb: skipped %d invalid lines in %s", skipped, source))
	}

	return loaded, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// JSON format (.json)
//
// Array of HashRecord objects. This is AxiomNizam's native format, more
// expressive than ClamAV's line-based format.
// ─────────────────────────────────────────────────────────────────────────────

// HashRecord is the JSON representation of a hash database entry.
type HashRecord struct {
	SHA256      string `json:"sha256"`
	MalwareName string `json:"malwareName"`
	FileSize    int64  `json:"fileSize,omitempty"`
	Category    string `json:"category,omitempty"`
	Source      string `json:"source,omitempty"`
}

// LoadJSONFile parses an AxiomNizam JSON hash database file.
func LoadJSONFile(db *DB, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	return LoadJSON(db, f, filepath.Base(path))
}

// LoadJSON parses AxiomNizam JSON hash records from a reader.
func LoadJSON(db *DB, r io.Reader, source string) (int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, fmt.Errorf("read JSON: %w", err)
	}

	var records []HashRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return 0, fmt.Errorf("parse JSON: %w", err)
	}

	loaded := 0
	for _, rec := range records {
		sha256Hex := strings.ToLower(strings.TrimSpace(rec.SHA256))
		if len(sha256Hex) != 64 || !isHexString(sha256Hex) {
			continue
		}

		entrySource := source
		if rec.Source != "" {
			entrySource = rec.Source
		}

		category := antivirus.ThreatCategory(rec.Category)
		if category == "" {
			category = inferCategoryFromName(rec.MalwareName)
		}

		db.Add(sha256Hex, HashEntry{
			MalwareName: rec.MalwareName,
			FileSize:    rec.FileSize,
			Category:    category,
			Source:      entrySource,
		})
		loaded++
	}

	return loaded, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Plain text format (.txt)
//
// One entry per line. Supports two formats:
//   - SHA256_HEX                    (malware name defaults to "Unknown")
//   - SHA256_HEX  MALWARE_NAME      (space or tab separated)
// ─────────────────────────────────────────────────────────────────────────────

// LoadTextFile parses a plain text hash file.
func LoadTextFile(db *DB, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	return LoadText(db, f, filepath.Base(path))
}

// LoadText parses plain text hash entries from a reader.
func LoadText(db *DB, r io.Reader, source string) (int, error) {
	scanner := bufio.NewScanner(r)
	loaded := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on whitespace (space or tab).
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		sha256Hex := strings.ToLower(strings.TrimSpace(fields[0]))
		if len(sha256Hex) != 64 || !isHexString(sha256Hex) {
			continue
		}

		malwareName := "Unknown"
		if len(fields) > 1 {
			malwareName = strings.Join(fields[1:], " ")
		}

		db.Add(sha256Hex, HashEntry{
			MalwareName: malwareName,
			FileSize:    0,
			Category:    inferCategoryFromName(malwareName),
			Source:      source,
		})
		loaded++
	}

	return loaded, scanner.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper functions
// ─────────────────────────────────────────────────────────────────────────────

// isHexString returns true if s consists entirely of hexadecimal characters.
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}

// inferCategoryFromName attempts to determine the threat category from a
// ClamAV-style malware name. ClamAV names typically follow the pattern:
//
//	Category.Platform.Family.Variant
//
// Examples:
//
//	Trojan.Win32.Emotet.A     → CategoryTrojan
//	Ransom.Linux.WannaCry     → CategoryRansomware
//	CoinMiner.Multi.XMRig     → CategoryCryptominer
//	Backdoor.PHP.WebShell     → CategoryWebshell
func inferCategoryFromName(name string) antivirus.ThreatCategory {
	lower := strings.ToLower(name)

	// Check prefixes first (most specific).
	prefixMap := []struct {
		prefix   string
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
		{"packer", antivirus.CategoryPacker},
		{"packed", antivirus.CategoryPacker},
	}

	for _, pm := range prefixMap {
		if strings.HasPrefix(lower, pm.prefix) {
			return pm.category
		}
	}

	// Substring checks for names that embed the category mid-string.
	if strings.Contains(lower, "ransom") {
		return antivirus.CategoryRansomware
	}
	if strings.Contains(lower, "miner") || strings.Contains(lower, "coinminer") {
		return antivirus.CategoryCryptominer
	}
	if strings.Contains(lower, "webshell") {
		return antivirus.CategoryWebshell
	}

	return antivirus.CategoryGeneric
}
