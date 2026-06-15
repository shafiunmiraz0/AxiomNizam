// Package sigdb implements Layer 7 of the AxiomNizam antivirus engine:
// a unified signature database that coordinates loading, versioning,
// and hot-reloading of all signature types across layers.
//
// The database manages three signature categories:
//
//   - Hash signatures (SHA-256) for the hashdb layer
//   - Byte patterns (hex) for the matcher/Aho-Corasick layer
//   - YARA rules for the yara layer
//
// Signatures come from two sources:
//
//  1. Built-in: compiled into the Go binary (always available)
//  2. On-disk: loaded from the SigDir directory (updated by the Updater)
//
// The Database provides a single Init() that loads all sources, and a
// Reload() that re-reads disk files and hot-swaps into the running layers.
package sigdb

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/hashdb"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/matcher"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/yara"
)

// ─────────────────────────────────────────────────────────────────────────────
// Version Manifest
// ─────────────────────────────────────────────────────────────────────────────

// Version tracks the current state of the signature database.
type Version struct {
	Version     string    `json:"version"`
	UpdatedAt   time.Time `json:"updatedAt"`
	HashCount   int       `json:"hashCount"`
	PatternCount int      `json:"patternCount"`
	YARACount   int       `json:"yaraCount"`
	Source      string    `json:"source"` // "builtin", "disk", "remote"
}

// ─────────────────────────────────────────────────────────────────────────────
// Database
// ─────────────────────────────────────────────────────────────────────────────

// Database is the unified signature database coordinator. It loads
// signatures from built-in and on-disk sources, and provides hot-reload
// to running layers.
type Database struct {
	mu     sync.RWMutex
	sigDir string

	// Layer references for hot-reload.
	hashDB       *hashdb.DB
	matcherLayer *matcher.Layer
	yaraLayer    *yara.Layer

	// Current version.
	version Version

	// Stats
	lastReload   time.Time
	reloadCount  int
	loadErrors   []string
}

// New creates a new signature database coordinator.
func New(sigDir string) *Database {
	return &Database{
		sigDir: sigDir,
	}
}

// SetLayers registers the scan layers for hot-reload.
// Must be called before Init().
func (db *Database) SetLayers(h *hashdb.DB, m *matcher.Layer, y *yara.Layer) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.hashDB = h
	db.matcherLayer = m
	db.yaraLayer = y
}

// Init loads all signatures from built-in and on-disk sources.
// It populates the layers registered via SetLayers().
// Returns the loaded version info.
func (db *Database) Init() (*Version, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.loadErrors = nil

	// ── 1. Load built-in signatures ──────────────────────────────────
	hashCount := 0
	patternCount := 0
	yaraCount := 0

	// Built-in hash database (none compiled in — loaded from disk only).
	// hashdb.DB entries are loaded by loadDiskSignatures.

	// Built-in patterns.
	if db.matcherLayer != nil {
		b := matcher.NewBuilder()
		n := matcher.RegisterBuiltinPatterns(b)
		patternCount += n
		db.matcherLayer.Reload(b.Build())
		logging.Z().Info(fmt.Sprintf("🛡️  sigdb: loaded %d built-in patterns", n))
	}

	// Built-in YARA rules.
	if db.yaraLayer != nil {
		rs := yara.NewRuleSet()
		n := yara.RegisterBuiltinRules(rs)
		yaraCount += n
		db.yaraLayer.Reload(rs)
		logging.Z().Info(fmt.Sprintf("🛡️  sigdb: loaded %d built-in YARA rules", n))
	}

	// ── 2. Load on-disk signatures ───────────────────────────────────
	if db.sigDir != "" {
		dh, dp, dy := db.loadDiskSignatures()
		hashCount += dh
		patternCount += dp
		yaraCount += dy
	}

	// ── 3. Set version ───────────────────────────────────────────────
	db.version = Version{
		Version:      db.readVersionFromDisk(),
		UpdatedAt:    time.Now(),
		HashCount:    hashCount,
		PatternCount: patternCount,
		YARACount:    yaraCount,
		Source:       "builtin+disk",
	}
	db.lastReload = time.Now()

	logging.Z().Info(fmt.Sprintf("🛡️  sigdb: initialized — hashes=%d patterns=%d yara=%d (v%s)",
		hashCount, patternCount, yaraCount, db.version.Version))

	return &db.version, nil
}

// loadDiskSignatures loads signatures from the on-disk directory.
// Returns counts of loaded items per type.
func (db *Database) loadDiskSignatures() (hashes, patterns, yaraRules int) {
	if _, err := os.Stat(db.sigDir); os.IsNotExist(err) {
		logging.Z().Info(fmt.Sprintf("🛡️  sigdb: signature directory %q does not exist, using built-in only", db.sigDir))
		return 0, 0, 0
	}

	// Hash databases (.hdb, .hsb, .txt, .json).
	hashDir := filepath.Join(db.sigDir, "hashes")
	if db.hashDB != nil {
		h, errs := db.loadHashDir(hashDir)
		hashes = h
		for _, e := range errs {
			db.loadErrors = append(db.loadErrors, e.Error())
		}
	}

	// Pattern databases (.ndb, .json).
	patternDir := filepath.Join(db.sigDir, "patterns")
	if db.matcherLayer != nil {
		p, errs := db.loadPatternDir(patternDir)
		patterns = p
		for _, e := range errs {
			db.loadErrors = append(db.loadErrors, e.Error())
		}
	}

	// YARA rules (.yar, .yara).
	yaraDir := filepath.Join(db.sigDir, "yara")
	if db.yaraLayer != nil {
		y, errs := db.loadYARADir(yaraDir)
		yaraRules = y
		for _, e := range errs {
			db.loadErrors = append(db.loadErrors, e.Error())
		}
	}

	// Also check for custom rules.
	customDir := filepath.Join(db.sigDir, "custom")
	if _, err := os.Stat(customDir); err == nil {
		ch, cp, cy := db.loadCustomDir(customDir)
		hashes += ch
		patterns += cp
		yaraRules += cy
	}

	return hashes, patterns, yaraRules
}

// loadHashDir loads hash databases from a directory into the hash layer.
func (db *Database) loadHashDir(dir string) (int, []error) {
	if db.hashDB == nil {
		return 0, nil
	}
	loaded, errs := hashdb.LoadFromDir(db.hashDB, dir)
	if loaded > 0 {
		logging.Z().Info(fmt.Sprintf("🛡️  sigdb: loaded %d hashes from %s", loaded, dir))
	}
	return loaded, errs
}

// loadPatternDir loads pattern databases and rebuilds the matcher automaton.
func (db *Database) loadPatternDir(dir string) (int, []error) {
	if db.matcherLayer == nil {
		return 0, nil
	}
	b := matcher.NewBuilder()
	matcher.RegisterBuiltinPatterns(b)
	loaded, errs := matcher.LoadFromDir(b, dir)
	if loaded > 0 {
		db.matcherLayer.Reload(b.Build())
		logging.Z().Info(fmt.Sprintf("🛡️  sigdb: loaded %d patterns from %s", loaded, dir))
	}
	return loaded, errs
}

// loadYARADir loads YARA rules and rebuilds the rule set.
func (db *Database) loadYARADir(dir string) (int, []error) {
	if db.yaraLayer == nil {
		return 0, nil
	}
	rs := yara.NewRuleSet()
	yara.RegisterBuiltinRules(rs)
	loaded, errs := yara.LoadFromDir(rs, dir)
	if loaded > 0 {
		db.yaraLayer.Reload(rs)
		logging.Z().Info(fmt.Sprintf("🛡️  sigdb: loaded %d YARA rules from %s", loaded, dir))
	}
	return loaded, errs
}

// loadCustomDir loads user-provided custom signatures.
func (db *Database) loadCustomDir(dir string) (hashes, patterns, yaraRules int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, 0
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		path := filepath.Join(dir, entry.Name())

		switch ext {
		case ".hdb", ".hsb":
			if db.hashDB != nil {
				n, _ := hashdb.LoadFromDir(db.hashDB, filepath.Dir(path))
				hashes += n
			}
		case ".ndb":
			if db.matcherLayer != nil {
				b := matcher.NewBuilder()
				n, _ := matcher.LoadFromDir(b, filepath.Dir(path))
				if n > 0 {
					db.matcherLayer.Reload(b.Build())
				}
				patterns += n
			}
		case ".yar", ".yara":
			if db.yaraLayer != nil {
				rs := yara.NewRuleSet()
				n, _ := yara.LoadFromDir(rs, filepath.Dir(path))
				if n > 0 {
					db.yaraLayer.Reload(rs)
				}
				yaraRules += n
			}
		}
	}

	if hashes+patterns+yaraRules > 0 {
		logging.Z().Info(fmt.Sprintf("🛡️  sigdb: loaded custom sigs — hashes=%d patterns=%d yara=%d",
			hashes, patterns, yaraRules))
	}
	return hashes, patterns, yaraRules
}

// readVersionFromDisk reads the version.json file from the sig directory.
func (db *Database) readVersionFromDisk() string {
	path := filepath.Join(db.sigDir, "version.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "builtin-only"
	}
	var v struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &v) != nil || v.Version == "" {
		return "builtin-only"
	}
	return v.Version
}

// Reload re-reads all on-disk signature files and hot-swaps into layers.
func (db *Database) Reload() (*Version, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.loadErrors = nil

	// Re-initialize with built-ins + disk.
	hashCount := 0
	patternCount := 0
	yaraCount := 0

	if db.matcherLayer != nil {
		b := matcher.NewBuilder()
		n := matcher.RegisterBuiltinPatterns(b)
		patternCount += n
		db.matcherLayer.Reload(b.Build())
	}

	if db.yaraLayer != nil {
		rs := yara.NewRuleSet()
		n := yara.RegisterBuiltinRules(rs)
		yaraCount += n
		db.yaraLayer.Reload(rs)
	}

	if db.sigDir != "" {
		dh, dp, dy := db.loadDiskSignatures()
		hashCount += dh
		patternCount += dp
		yaraCount += dy
	}

	db.version = Version{
		Version:      db.readVersionFromDisk(),
		UpdatedAt:    time.Now(),
		HashCount:    hashCount,
		PatternCount: patternCount,
		YARACount:    yaraCount,
		Source:       "reload",
	}
	db.lastReload = time.Now()
	db.reloadCount++

	logging.Z().Info(fmt.Sprintf("🛡️  sigdb: reloaded — hashes=%d patterns=%d yara=%d (reload #%d)",
		hashCount, patternCount, yaraCount, db.reloadCount))

	return &db.version, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Stats
// ─────────────────────────────────────────────────────────────────────────────

// Stats returns a snapshot of database statistics.
type Stats struct {
	Version      Version   `json:"version"`
	LastReload   time.Time `json:"lastReload"`
	ReloadCount  int       `json:"reloadCount"`
	LoadErrors   []string  `json:"loadErrors,omitempty"`
}

// Stats returns current database statistics.
func (db *Database) Stats() Stats {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return Stats{
		Version:     db.version,
		LastReload:  db.lastReload,
		ReloadCount: db.reloadCount,
		LoadErrors:  db.loadErrors,
	}
}

// SigDir returns the configured signature directory path.
func (db *Database) SigDir() string {
	return db.sigDir
}

// WriteVersion writes a version.json file to the signature directory.
func (db *Database) WriteVersion(ver Version) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.sigDir == "" {
		return fmt.Errorf("no signature directory configured")
	}

	if err := os.MkdirAll(db.sigDir, 0755); err != nil {
		return fmt.Errorf("create sig dir: %w", err)
	}

	data, err := json.MarshalIndent(ver, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal version: %w", err)
	}

	path := filepath.Join(db.sigDir, "version.json")
	return os.WriteFile(path, data, 0644)
}
