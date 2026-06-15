package hashdb

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

// testHash returns a deterministic SHA-256 hex string for a given input.
func testHash(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}

// ─────────────────────────────────────────────────────────────────────────────
// Bloom Filter Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestBloomFilter_Basic(t *testing.T) {
	bf := newBloomFilter(1000, 0.0001)

	hash1 := testHash("malware1")
	hash2 := testHash("malware2")
	hashClean := testHash("clean_file")

	bf.add(hash1)
	bf.add(hash2)

	if !bf.test(hash1) {
		t.Error("bloom filter should contain hash1 (no false negatives)")
	}
	if !bf.test(hash2) {
		t.Error("bloom filter should contain hash2 (no false negatives)")
	}

	// hashClean was never added — bloom filter may or may not match
	// (false positive possible but very unlikely at 0.01% FP rate).
	// We don't assert on this — just verify no panic.
	_ = bf.test(hashClean)
}

func TestBloomFilter_NoFalseNegatives(t *testing.T) {
	bf := newBloomFilter(10000, 0.001)

	// Insert 1000 known hashes.
	hashes := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		hashes[i] = testHash(fmt.Sprintf("malware_%d", i))
		bf.add(hashes[i])
	}

	// All must be found — zero false negatives is guaranteed.
	for i, h := range hashes {
		if !bf.test(h) {
			t.Fatalf("false negative at index %d: hash %s not found in bloom filter", i, h[:16])
		}
	}
}

func TestBloomFilter_FalsePositiveRate(t *testing.T) {
	n := uint(10000)
	bf := newBloomFilter(n, 0.01) // 1% target FP rate

	// Insert n hashes.
	for i := uint(0); i < n; i++ {
		bf.add(testHash(fmt.Sprintf("insert_%d", i)))
	}

	// Test n hashes that were NOT inserted.
	falsePositives := 0
	testCount := 100000
	for i := 0; i < testCount; i++ {
		if bf.test(testHash(fmt.Sprintf("lookup_%d", i))) {
			falsePositives++
		}
	}

	actualRate := float64(falsePositives) / float64(testCount)
	// Allow up to 3x the target rate (statistical variance).
	if actualRate > 0.03 {
		t.Errorf("false positive rate %.4f exceeds acceptable threshold (target=0.01, max=0.03)", actualRate)
	}
	t.Logf("bloom filter FP rate: %.4f%% (%d/%d), memory: %d bytes",
		actualRate*100, falsePositives, testCount, bf.estimateMemoryBytes())
}

func TestBloomFilter_MemorySize(t *testing.T) {
	bf := newBloomFilter(500_000, 0.0001)
	memBytes := bf.estimateMemoryBytes()
	memMB := float64(memBytes) / (1024 * 1024)

	// For 500K elements at 0.01% FP rate, expect ~1-2 MB.
	if memMB > 5 {
		t.Errorf("bloom filter memory %.1f MB exceeds 5 MB budget for 500K hashes", memMB)
	}
	t.Logf("bloom filter for 500K hashes at 0.01%% FP: %.2f MB, k=%d", memMB, bf.numHash)
}

// ─────────────────────────────────────────────────────────────────────────────
// HashDB Core Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestDB_NewEmpty(t *testing.T) {
	db := New(1000, 0.0001)
	if db.Count() != 0 {
		t.Errorf("new DB should be empty, got %d", db.Count())
	}
	if db.Name() != "hashdb" {
		t.Errorf("expected layer name 'hashdb', got %q", db.Name())
	}
}

func TestDB_AddAndContains(t *testing.T) {
	db := New(1000, 0.0001)

	hash := testHash("evil.exe")
	db.Add(hash, HashEntry{
		MalwareName: "Trojan.Win32.Emotet.A",
		FileSize:    12345,
		Category:    antivirus.CategoryTrojan,
		Source:      "test",
	})

	if !db.Contains(hash) {
		t.Error("DB should contain the added hash")
	}
	if db.Contains(testHash("clean.txt")) {
		t.Error("DB should not contain a hash that was never added")
	}
	if db.Count() != 1 {
		t.Errorf("expected count 1, got %d", db.Count())
	}
}

func TestDB_Scan_CleanFile(t *testing.T) {
	db := New(1000, 0.0001)
	db.Add(testHash("evil"), HashEntry{MalwareName: "Malware.Test"})

	target := &antivirus.ScanTarget{
		Filename: "clean.txt",
		SHA256:   testHash("clean_content"),
		Size:     100,
	}

	threats, err := db.Scan(target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(threats) != 0 {
		t.Errorf("expected no threats for clean file, got %d", len(threats))
	}
}

func TestDB_Scan_MalwareDetected(t *testing.T) {
	db := New(1000, 0.0001)
	malwareHash := testHash("evil_payload")
	db.Add(malwareHash, HashEntry{
		MalwareName: "Trojan.Win32.Emotet.A",
		FileSize:    0, // wildcard
		Category:    antivirus.CategoryTrojan,
		Source:      "test-db",
	})

	target := &antivirus.ScanTarget{
		Filename: "report.pdf",
		SHA256:   malwareHash,
		Size:     5000,
	}

	threats, err := db.Scan(target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(threats) != 1 {
		t.Fatalf("expected 1 threat, got %d", len(threats))
	}

	threat := threats[0]
	if threat.Name != "Trojan.Win32.Emotet.A" {
		t.Errorf("expected malware name 'Trojan.Win32.Emotet.A', got %q", threat.Name)
	}
	if threat.Confidence != 1.0 {
		t.Errorf("hash match should have confidence 1.0, got %f", threat.Confidence)
	}
	if threat.Layer != antivirus.LayerHashDB {
		t.Errorf("expected layer hashdb, got %s", threat.Layer)
	}
	if threat.Severity != antivirus.SeverityCritical {
		t.Errorf("expected severity critical, got %s", threat.Severity)
	}
	if threat.Category != antivirus.CategoryTrojan {
		t.Errorf("expected category trojan, got %s", threat.Category)
	}
}

func TestDB_Scan_SizeMismatch(t *testing.T) {
	db := New(1000, 0.0001)
	hash := testHash("size_check")
	db.Add(hash, HashEntry{
		MalwareName: "Malware.Test",
		FileSize:    1000, // specific size
	})

	target := &antivirus.ScanTarget{
		Filename: "file.bin",
		SHA256:   hash,
		Size:     9999, // different size
	}

	threats, err := db.Scan(target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(threats) != 0 {
		t.Error("size mismatch should result in no threat (possible hash collision)")
	}
}

func TestDB_Scan_EmptySHA256(t *testing.T) {
	db := New(1000, 0.0001)
	target := &antivirus.ScanTarget{SHA256: ""}
	threats, err := db.Scan(target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(threats) != 0 {
		t.Error("empty SHA256 should return no threats")
	}
}

func TestDB_AddBatch(t *testing.T) {
	db := New(1000, 0.0001)

	batch := map[string]HashEntry{
		testHash("m1"): {MalwareName: "Malware.1"},
		testHash("m2"): {MalwareName: "Malware.2"},
		testHash("m3"): {MalwareName: "Malware.3"},
	}

	added := db.AddBatch(batch)
	if added != 3 {
		t.Errorf("expected 3 added, got %d", added)
	}
	if db.Count() != 3 {
		t.Errorf("expected count 3, got %d", db.Count())
	}

	// Adding duplicates should not increase count.
	added = db.AddBatch(batch)
	if added != 0 {
		t.Errorf("expected 0 new additions, got %d", added)
	}
}

func TestDB_Remove(t *testing.T) {
	db := New(1000, 0.0001)
	hash := testHash("removable")
	db.Add(hash, HashEntry{MalwareName: "Malware.Temp"})

	if !db.Remove(hash) {
		t.Error("Remove should return true for existing hash")
	}
	if db.Count() != 0 {
		t.Errorf("expected count 0 after removal, got %d", db.Count())
	}
	if db.Remove(hash) {
		t.Error("Remove should return false for non-existing hash")
	}
}

func TestDB_Reload(t *testing.T) {
	db := New(1000, 0.0001)
	db.Add(testHash("old1"), HashEntry{MalwareName: "Old.1"})
	db.Add(testHash("old2"), HashEntry{MalwareName: "Old.2"})

	newEntries := map[string]HashEntry{
		testHash("new1"): {MalwareName: "New.1"},
		testHash("new2"): {MalwareName: "New.2"},
		testHash("new3"): {MalwareName: "New.3"},
	}

	db.Reload(newEntries, "v2.0")

	if db.Count() != 3 {
		t.Errorf("expected 3 after reload, got %d", db.Count())
	}
	if db.Version() != "v2.0" {
		t.Errorf("expected version v2.0, got %q", db.Version())
	}
	if db.Contains(testHash("old1")) {
		t.Error("old hashes should be gone after reload")
	}
	if !db.Contains(testHash("new1")) {
		t.Error("new hashes should be present after reload")
	}
}

func TestDB_Stats(t *testing.T) {
	db := New(1000, 0.0001)
	hash := testHash("stats_test")
	db.Add(hash, HashEntry{MalwareName: "Malware.Stats"})

	// Perform some lookups.
	db.Scan(&antivirus.ScanTarget{SHA256: testHash("clean1"), Size: 1})
	db.Scan(&antivirus.ScanTarget{SHA256: testHash("clean2"), Size: 1})
	db.Scan(&antivirus.ScanTarget{SHA256: hash, Size: 1})

	stats := db.Stats()
	if stats.TotalLookups != 3 {
		t.Errorf("expected 3 lookups, got %d", stats.TotalLookups)
	}
	if stats.TotalHits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.TotalHits)
	}
	if stats.HashCount != 1 {
		t.Errorf("expected 1 hash, got %d", stats.HashCount)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Concurrency Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestDB_ConcurrentReads(t *testing.T) {
	db := New(10000, 0.0001)

	// Load some hashes.
	for i := 0; i < 100; i++ {
		db.Add(testHash(fmt.Sprintf("concurrent_%d", i)), HashEntry{
			MalwareName: fmt.Sprintf("Malware.Concurrent.%d", i),
		})
	}

	// Hammer with concurrent reads.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				target := &antivirus.ScanTarget{
					SHA256: testHash(fmt.Sprintf("concurrent_%d", j%100)),
					Size:   1,
				}
				_, err := db.Scan(target)
				if err != nil {
					t.Errorf("concurrent scan error: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()
}

// ─────────────────────────────────────────────────────────────────────────────
// Loader Tests — ClamAV format
// ─────────────────────────────────────────────────────────────────────────────

func TestLoadClamAV_ValidSHA256(t *testing.T) {
	db := New(100, 0.01)

	// Create ClamAV-format content with SHA-256 hashes.
	hash1 := testHash("clamav_test_1")
	hash2 := testHash("clamav_test_2")
	content := fmt.Sprintf("%s:12345:Trojan.Win32.TestVirus.A\n%s:*:Ransom.Linux.WannaCry\n", hash1, hash2)

	loaded, err := LoadClamAV(db, strings.NewReader(content), "test.hsb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != 2 {
		t.Errorf("expected 2 loaded, got %d", loaded)
	}
	if !db.Contains(hash1) {
		t.Error("hash1 should be in DB")
	}
	if !db.Contains(hash2) {
		t.Error("hash2 should be in DB")
	}
}

func TestLoadClamAV_SkipsMD5AndSHA1(t *testing.T) {
	db := New(100, 0.01)

	content := "d41d8cd98f00b204e9800998ecf8427e:0:MD5.Test\n" + // MD5 (32 chars) — skip
		"da39a3ee5e6b4b0d3255bfef95601890afd80709:0:SHA1.Test\n" + // SHA-1 (40 chars) — skip
		testHash("real") + ":100:SHA256.Test\n" // SHA-256 (64 chars) — load

	loaded, err := LoadClamAV(db, strings.NewReader(content), "test.hdb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != 1 {
		t.Errorf("expected 1 SHA-256 loaded (MD5 and SHA-1 skipped), got %d", loaded)
	}
}

func TestLoadClamAV_SkipsCommentsAndEmpty(t *testing.T) {
	db := New(100, 0.01)

	hash := testHash("comment_test")
	content := fmt.Sprintf("# This is a comment\n\n%s:100:Valid.Sig\n\n# Another comment\n", hash)

	loaded, err := LoadClamAV(db, strings.NewReader(content), "test.hsb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != 1 {
		t.Errorf("expected 1 loaded, got %d", loaded)
	}
}

func TestLoadClamAV_CategoryInference(t *testing.T) {
	db := New(100, 0.01)

	hash := testHash("trojan_category")
	content := fmt.Sprintf("%s:100:Trojan.Win32.Emotet.A\n", hash)

	LoadClamAV(db, strings.NewReader(content), "test.hsb")

	// Scan to get the threat info and verify category.
	threats, _ := db.Scan(&antivirus.ScanTarget{SHA256: hash, Size: 100})
	if len(threats) != 1 {
		t.Fatalf("expected 1 threat, got %d", len(threats))
	}
	if threats[0].Category != antivirus.CategoryTrojan {
		t.Errorf("expected category trojan, got %s", threats[0].Category)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Loader Tests — JSON format
// ─────────────────────────────────────────────────────────────────────────────

func TestLoadJSON_Valid(t *testing.T) {
	db := New(100, 0.01)

	hash1 := testHash("json_test_1")
	hash2 := testHash("json_test_2")
	jsonContent := fmt.Sprintf(`[
		{"sha256": "%s", "malwareName": "Ransom.WannaCry", "fileSize": 1000, "category": "ransomware"},
		{"sha256": "%s", "malwareName": "Trojan.Generic", "category": "trojan"}
	]`, hash1, hash2)

	loaded, err := LoadJSON(db, strings.NewReader(jsonContent), "test.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != 2 {
		t.Errorf("expected 2 loaded, got %d", loaded)
	}
	if !db.Contains(hash1) {
		t.Error("hash1 should be in DB")
	}
}

func TestLoadJSON_InvalidHash(t *testing.T) {
	db := New(100, 0.01)

	jsonContent := `[{"sha256": "not_a_valid_hash", "malwareName": "Bad.Hash"}]`
	loaded, err := LoadJSON(db, strings.NewReader(jsonContent), "test.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != 0 {
		t.Errorf("invalid hashes should be skipped, got %d loaded", loaded)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Loader Tests — Plain text format
// ─────────────────────────────────────────────────────────────────────────────

func TestLoadText_HashOnly(t *testing.T) {
	db := New(100, 0.01)

	hash := testHash("text_test")
	content := fmt.Sprintf("# Comment line\n%s\n", hash)

	loaded, err := LoadText(db, strings.NewReader(content), "test.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != 1 {
		t.Errorf("expected 1 loaded, got %d", loaded)
	}
}

func TestLoadText_HashWithName(t *testing.T) {
	db := New(100, 0.01)

	hash := testHash("named_text")
	content := fmt.Sprintf("%s\tMalware.Named.Test\n", hash)

	loaded, err := LoadText(db, strings.NewReader(content), "test.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != 1 {
		t.Errorf("expected 1 loaded, got %d", loaded)
	}

	threats, _ := db.Scan(&antivirus.ScanTarget{SHA256: hash, Size: 1})
	if len(threats) != 1 || threats[0].Name != "Malware.Named.Test" {
		t.Errorf("expected threat name 'Malware.Named.Test', got %v", threats)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// LoadFromDir Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestLoadFromDir_MixedFormats(t *testing.T) {
	// Create a temp directory with test files.
	dir := t.TempDir()

	hash1 := testHash("dir_clamav")
	hash2 := testHash("dir_json")
	hash3 := testHash("dir_text")

	// Write ClamAV file.
	os.WriteFile(filepath.Join(dir, "test.hsb"),
		[]byte(fmt.Sprintf("%s:100:ClamAV.Test\n", hash1)), 0644)

	// Write JSON file.
	os.WriteFile(filepath.Join(dir, "test.json"),
		[]byte(fmt.Sprintf(`[{"sha256":"%s","malwareName":"JSON.Test"}]`, hash2)), 0644)

	// Write text file.
	os.WriteFile(filepath.Join(dir, "test.txt"),
		[]byte(fmt.Sprintf("%s Text.Test\n", hash3)), 0644)

	db := New(100, 0.01)
	loaded, errs := LoadFromDir(db, dir)

	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if loaded != 3 {
		t.Errorf("expected 3 loaded from mixed formats, got %d", loaded)
	}
	if db.Count() != 3 {
		t.Errorf("expected DB count 3, got %d", db.Count())
	}
}

func TestLoadFromDir_NonExistent(t *testing.T) {
	db := New(100, 0.01)
	loaded, errs := LoadFromDir(db, "/nonexistent/path/to/sigs")

	if len(errs) != 0 {
		t.Error("non-existent directory should not return errors (just skip)")
	}
	if loaded != 0 {
		t.Errorf("expected 0 loaded, got %d", loaded)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper Function Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestIsHexString(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"abcdef0123456789", true},
		{"ABCDEF0123456789", true},
		{"abcDEF", true},
		{"xyz", false},
		{"abcg", false},
		{"", false},
		{"abc def", false},
	}
	for _, tt := range tests {
		if got := isHexString(tt.input); got != tt.want {
			t.Errorf("isHexString(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestInferCategoryFromName(t *testing.T) {
	tests := []struct {
		name string
		want antivirus.ThreatCategory
	}{
		{"Trojan.Win32.Emotet.A", antivirus.CategoryTrojan},
		{"Ransom.Linux.WannaCry", antivirus.CategoryRansomware},
		{"Worm.Win32.Conficker", antivirus.CategoryWorm},
		{"Exploit.PDF.CVE2021", antivirus.CategoryExploit},
		{"Backdoor.PHP.WebShell", antivirus.CategoryBackdoor},
		{"CoinMiner.Multi.XMRig", antivirus.CategoryCryptominer},
		{"Adware.Win32.Toolbar", antivirus.CategoryAdware},
		{"Unknown.Generic.Malware", antivirus.CategoryGeneric},
		{"Something.With.Ransom.Inside", antivirus.CategoryRansomware},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferCategoryFromName(tt.name); got != tt.want {
				t.Errorf("inferCategoryFromName(%q) = %s, want %s", tt.name, got, tt.want)
			}
		})
	}
}
