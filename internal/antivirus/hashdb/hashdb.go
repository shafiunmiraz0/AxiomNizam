// Package hashdb implements Layer 1 of the AxiomNizam antivirus engine:
// known-malware hash lookup using a bloom filter for fast negative filtering
// and a confirmed-positive map for definitive identification.
//
// # How it works
//
// 99.99%+ of files uploaded to object storage are clean. A naive map lookup
// of every SHA-256 against 500K known hashes would work but wastes memory
// on the map overhead for keys that will almost never match.
//
// Instead we use a two-tier strategy:
//
//  1. Bloom filter (~5 MB for 500K hashes at 0.01% FP rate) — if the hash
//     is NOT in the bloom filter, the file is definitely clean. This handles
//     the >99.99% clean-file fast path in ~1 μs with zero allocations.
//
//  2. Confirmed map (map[string]HashEntry) — only consulted when the bloom
//     filter says "maybe". Maps SHA-256 → malware name + metadata for
//     definitive identification.
//
// # Thread safety
//
// The HashDB is safe for concurrent reads via RWMutex. Write operations
// (Add, Remove, Reload) acquire an exclusive lock and atomically swap the
// internal data structures so that concurrent readers never see a partially-
// updated state.
//
// # ClamAV compatibility
//
// The loader supports ClamAV's .hdb/.hsb file format:
//
//	HashString:FileSize:MalwareName
//
// SHA-256 hashes are identified by their 64-character length.
package hashdb

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Hash Entry
// ─────────────────────────────────────────────────────────────────────────────

// HashEntry stores metadata for a known-malware hash.
type HashEntry struct {
	// MalwareName is the threat name (e.g. "Trojan.Win32.Emotet.A").
	MalwareName string

	// FileSize is the expected file size in bytes. Zero means any size
	// matches (wildcard).
	FileSize int64

	// Category is the threat category (trojan, ransomware, etc.).
	// May be empty if not specified in the source database.
	Category antivirus.ThreatCategory

	// Source indicates where this hash came from (e.g. "builtin",
	// "clamav-main", "custom").
	Source string
}

// ─────────────────────────────────────────────────────────────────────────────
// Bloom Filter
// ─────────────────────────────────────────────────────────────────────────────

// bloomFilter is a space-efficient probabilistic data structure for set
// membership testing. It uses the Kirsch-Mitzenmacher optimisation: two
// independent hash functions (derived from SHA-256) simulate k hash
// functions via g_i(x) = h1(x) + i * h2(x).
//
// We use SHA-256 because the input is already a hex-encoded SHA-256 hash,
// so we just parse the first 16 bytes to extract two 64-bit seeds. No
// additional hashing needed — the input is already uniformly distributed.
type bloomFilter struct {
	bits    []uint64 // bit array packed into 64-bit words
	numBits uint64   // total number of bits (m)
	numHash uint     // number of hash functions (k)
}

// newBloomFilter creates a bloom filter optimally sized for the expected
// number of elements and desired false positive rate.
//
// Formulas:
//
//	m = -n * ln(p) / (ln2)^2
//	k = (m/n) * ln2
func newBloomFilter(expectedItems uint, falsePositiveRate float64) *bloomFilter {
	if expectedItems == 0 {
		expectedItems = 1
	}
	if falsePositiveRate <= 0 || falsePositiveRate >= 1 {
		falsePositiveRate = 0.0001 // default: 0.01%
	}

	// Calculate optimal parameters.
	n := float64(expectedItems)
	ln2 := math.Ln2
	ln2sq := ln2 * ln2

	m := uint64(math.Ceil(-n * math.Log(falsePositiveRate) / ln2sq))
	k := uint(math.Ceil(float64(m) / n * ln2))

	// Minimum sanity.
	if m < 64 {
		m = 64
	}
	if k < 1 {
		k = 1
	}
	if k > 30 {
		k = 30 // practical upper bound
	}

	// Round up to nearest 64-bit word boundary.
	words := (m + 63) / 64

	return &bloomFilter{
		bits:    make([]uint64, words),
		numBits: words * 64,
		numHash: k,
	}
}

// add inserts a hex-encoded SHA-256 hash into the bloom filter.
func (bf *bloomFilter) add(hexHash string) {
	h1, h2 := bf.twoHashes(hexHash)
	for i := uint(0); i < bf.numHash; i++ {
		pos := (h1 + uint64(i)*h2) % bf.numBits
		word := pos / 64
		bit := pos % 64
		bf.bits[word] |= 1 << bit
	}
}

// test returns true if the hash MIGHT be in the set (possible false
// positive), or false if it is DEFINITELY not in the set (no false
// negatives).
func (bf *bloomFilter) test(hexHash string) bool {
	h1, h2 := bf.twoHashes(hexHash)
	for i := uint(0); i < bf.numHash; i++ {
		pos := (h1 + uint64(i)*h2) % bf.numBits
		word := pos / 64
		bit := pos % 64
		if bf.bits[word]&(1<<bit) == 0 {
			return false
		}
	}
	return true
}

// twoHashes extracts two independent 64-bit hash values from a hex-encoded
// SHA-256 string. Since SHA-256 output is already uniformly distributed,
// we hash the hex string with SHA-256 and split the result into two halves.
func (bf *bloomFilter) twoHashes(hexHash string) (uint64, uint64) {
	// Hash the hex string to get uniformly distributed bits.
	digest := sha256.Sum256([]byte(hexHash))
	h1 := binary.LittleEndian.Uint64(digest[0:8])
	h2 := binary.LittleEndian.Uint64(digest[8:16])
	// Ensure h2 is odd for better distribution (avoids degenerate cycles).
	h2 |= 1
	return h1, h2
}

// estimateMemoryBytes returns an approximate memory usage in bytes.
func (bf *bloomFilter) estimateMemoryBytes() int64 {
	return int64(len(bf.bits)) * 8 // 8 bytes per uint64
}

// ─────────────────────────────────────────────────────────────────────────────
// HashDB — the scan layer
// ─────────────────────────────────────────────────────────────────────────────

// DB implements antivirus.ScanLayer for known-malware hash lookups.
// It is the fastest detection layer — a single SHA-256 comparison per file.
type DB struct {
	mu sync.RWMutex

	// bloom is the probabilistic filter for fast negative lookups.
	bloom *bloomFilter

	// hashes maps SHA-256 hex strings → malware metadata for confirmed
	// positives.
	hashes map[string]HashEntry

	// stats tracks lookup performance.
	lookups    atomic.Int64
	hits       atomic.Int64
	bloomFP    atomic.Int64 // bloom filter false positives
	loadedAt   time.Time
	version    string
	hashCount  int
}

// New creates an empty HashDB with a bloom filter sized for the expected
// number of hashes. The falsePositiveRate controls the bloom filter's
// accuracy vs memory tradeoff (recommended: 0.0001 = 0.01%).
func New(expectedHashes uint, falsePositiveRate float64) *DB {
	if expectedHashes == 0 {
		expectedHashes = 500_000 // default: 500K hashes
	}
	if falsePositiveRate <= 0 {
		falsePositiveRate = 0.0001
	}

	return &DB{
		bloom:    newBloomFilter(expectedHashes, falsePositiveRate),
		hashes:   make(map[string]HashEntry, expectedHashes),
		loadedAt: time.Now(),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ScanLayer interface
// ─────────────────────────────────────────────────────────────────────────────

// Name returns the layer identifier.
func (db *DB) Name() string { return "hashdb" }

// Scan checks the file's SHA-256 against the known-malware hash database.
// This is an O(1) operation — bloom filter test + optional map lookup.
func (db *DB) Scan(target *antivirus.ScanTarget) ([]antivirus.ThreatInfo, error) {
	if target.SHA256 == "" {
		return nil, nil
	}

	db.lookups.Add(1)

	db.mu.RLock()
	defer db.mu.RUnlock()

	// Fast path: bloom filter says "definitely not in DB".
	if !db.bloom.test(target.SHA256) {
		return nil, nil
	}

	// Bloom filter says "maybe" — check confirmed map.
	entry, found := db.hashes[target.SHA256]
	if !found {
		// Bloom filter false positive — expected at the configured rate.
		db.bloomFP.Add(1)
		return nil, nil
	}

	// Confirmed match — check file size if specified.
	if entry.FileSize > 0 && entry.FileSize != target.Size {
		// File size mismatch — could be a hash collision or the file
		// was modified. Log but don't flag.
		logging.Z().Info(fmt.Sprintf("🛡️  hashdb: hash match for %q but size mismatch (expected=%d, got=%d)",
			target.Filename, entry.FileSize, target.Size))
		return nil, nil
	}

	db.hits.Add(1)

	// Build threat info.
	category := entry.Category
	if category == "" {
		category = antivirus.CategoryGeneric
	}

	threat := antivirus.ThreatInfo{
		Name:        entry.MalwareName,
		Category:    category,
		Severity:    antivirus.SeverityCritical,
		Layer:       antivirus.LayerHashDB,
		Description: fmt.Sprintf("File SHA-256 matches known malware: %s", entry.MalwareName),
		Signature:   target.SHA256,
		Confidence:  1.0, // Hash match = absolute certainty.
		Offset:      -1,  // N/A for hash-based detection.
		Metadata: map[string]string{
			"source":   entry.Source,
			"fileSize": fmt.Sprintf("%d", entry.FileSize),
		},
	}

	return []antivirus.ThreatInfo{threat}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Mutation methods
// ─────────────────────────────────────────────────────────────────────────────

// Add inserts a single hash entry into the database. Thread-safe.
func (db *DB) Add(sha256Hex string, entry HashEntry) {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.bloom.add(sha256Hex)
	db.hashes[sha256Hex] = entry
	db.hashCount = len(db.hashes)
}

// AddBatch inserts multiple hash entries atomically.
func (db *DB) AddBatch(entries map[string]HashEntry) int {
	db.mu.Lock()
	defer db.mu.Unlock()

	added := 0
	for sha256Hex, entry := range entries {
		if _, exists := db.hashes[sha256Hex]; !exists {
			added++
		}
		db.bloom.add(sha256Hex)
		db.hashes[sha256Hex] = entry
	}
	db.hashCount = len(db.hashes)
	return added
}

// Remove deletes a hash from the confirmed map. Note: bloom filters do not
// support deletion, so the removed hash may still cause bloom filter lookups
// (which will then miss in the map). This is acceptable — it just increases
// the effective false positive rate slightly.
func (db *DB) Remove(sha256Hex string) bool {
	db.mu.Lock()
	defer db.mu.Unlock()

	if _, exists := db.hashes[sha256Hex]; !exists {
		return false
	}
	delete(db.hashes, sha256Hex)
	db.hashCount = len(db.hashes)
	return true
}

// Reload replaces the entire database atomically with new data. A new bloom
// filter is built from scratch, so removed hashes no longer cause false
// positives. This is the recommended way to apply signature database updates.
func (db *DB) Reload(entries map[string]HashEntry, version string) {
	// Build new bloom filter outside the lock.
	expectedSize := uint(len(entries))
	if expectedSize < 1000 {
		expectedSize = 1000
	}
	newBloom := newBloomFilter(expectedSize, 0.0001)
	for sha256Hex := range entries {
		newBloom.add(sha256Hex)
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	db.bloom = newBloom
	db.hashes = entries
	db.hashCount = len(entries)
	db.version = version
	db.loadedAt = time.Now()

	logging.Z().Info(fmt.Sprintf("🛡️  hashdb: reloaded — %d hashes, version=%q, bloom=%.1fKB",
		len(entries), version, float64(newBloom.estimateMemoryBytes())/1024))
}

// Contains checks if a SHA-256 hash exists in the database (without
// generating a ThreatInfo). Useful for administrative queries.
func (db *DB) Contains(sha256Hex string) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if !db.bloom.test(sha256Hex) {
		return false
	}
	_, found := db.hashes[sha256Hex]
	return found
}

// ─────────────────────────────────────────────────────────────────────────────
// Statistics & Info
// ─────────────────────────────────────────────────────────────────────────────

// Stats holds runtime statistics for the hash database.
type Stats struct {
	HashCount      int     `json:"hashCount"`
	BloomSizeBytes int64   `json:"bloomSizeBytes"`
	BloomFPRate    float64 `json:"bloomFalsePositiveRate"`
	TotalLookups   int64   `json:"totalLookups"`
	TotalHits      int64   `json:"totalHits"`
	BloomFalsePos  int64   `json:"bloomFalsePositives"`
	Version        string  `json:"version"`
	LoadedAt       string  `json:"loadedAt"`
}

// Stats returns a snapshot of the database statistics.
func (db *DB) Stats() Stats {
	db.mu.RLock()
	defer db.mu.RUnlock()

	lookups := db.lookups.Load()
	var fpRate float64
	if lookups > 0 {
		fpRate = float64(db.bloomFP.Load()) / float64(lookups)
	}

	return Stats{
		HashCount:      db.hashCount,
		BloomSizeBytes: db.bloom.estimateMemoryBytes(),
		BloomFPRate:    fpRate,
		TotalLookups:   lookups,
		TotalHits:      db.hits.Load(),
		BloomFalsePos:  db.bloomFP.Load(),
		Version:        db.version,
		LoadedAt:       db.loadedAt.UTC().Format(time.RFC3339),
	}
}

// Count returns the number of hashes in the database.
func (db *DB) Count() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.hashCount
}

// Version returns the database version string.
func (db *DB) Version() string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.version
}
