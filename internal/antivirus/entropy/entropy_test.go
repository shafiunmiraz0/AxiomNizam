package entropy

import (
	"bytes"
	"crypto/rand"
	"strings"
	"sync"
	"testing"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Shannon Entropy Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestShannon_Empty(t *testing.T) {
	if Shannon(nil) != 0 {
		t.Error("empty data should have 0 entropy")
	}
	if Shannon([]byte{}) != 0 {
		t.Error("empty slice should have 0 entropy")
	}
}

func TestShannon_Uniform(t *testing.T) {
	data := bytes.Repeat([]byte{0xAA}, 10000)
	ent := Shannon(data)
	if ent > 0.001 {
		t.Errorf("uniform data should have ~0 entropy, got %.6f", ent)
	}
}

func TestShannon_FullRange(t *testing.T) {
	// All 256 byte values equally distributed → 8.0 bits/byte.
	data := make([]byte, 256*100)
	for i := range data {
		data[i] = byte(i % 256)
	}
	ent := Shannon(data)
	if ent < 7.99 || ent > 8.01 {
		t.Errorf("perfectly distributed data should have ~8.0 entropy, got %.4f", ent)
	}
}

func TestShannon_Text(t *testing.T) {
	text := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 100)
	ent := Shannon(text)
	// English text typically 3.5-5.5 bits/byte.
	if ent < 3.0 || ent > 5.5 {
		t.Errorf("English text entropy should be 3-5.5, got %.2f", ent)
	}
}

func TestShannon_Random(t *testing.T) {
	data := make([]byte, 10000)
	rand.Read(data)
	ent := Shannon(data)
	// Random data should be close to 8.0.
	if ent < 7.8 {
		t.Errorf("random data should have >7.8 entropy, got %.2f", ent)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Profile Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestAnalyze_SmallFile(t *testing.T) {
	data := []byte("hello world")
	p := Analyze(data)
	if p.TotalWindows != 1 {
		t.Errorf("small file should have 1 window, got %d", p.TotalWindows)
	}
	if p.WholeFileEntropy == 0 {
		t.Error("non-empty file should have non-zero entropy")
	}
}

func TestAnalyze_UniformHighEntropy(t *testing.T) {
	// Random data → uniformly high entropy in all windows.
	// 32KB gives 128 windows — large enough for reliable per-window entropy.
	data := make([]byte, 32768)
	rand.Read(data)
	p := Analyze(data)

	if p.TotalWindows < 100 {
		t.Errorf("expected >=100 windows, got %d", p.TotalWindows)
	}
	if p.HighEntropyRatio < 0.80 {
		t.Errorf("random data should have >80%% high-entropy windows, got %.2f", p.HighEntropyRatio)
	}
	if p.EntropyStdDev > 0.5 {
		t.Errorf("random data should have low stddev, got %.4f", p.EntropyStdDev)
	}
}

func TestAnalyze_MixedEntropy(t *testing.T) {
	// Half zeros, half random → mixed entropy profile.
	zeros := bytes.Repeat([]byte{0x00}, 2048)
	random := make([]byte, 2048)
	rand.Read(random)
	data := append(zeros, random...)

	p := Analyze(data)
	if p.LowEntropyWindows == 0 {
		t.Error("expected some low-entropy windows from the zeros section")
	}
	if p.HighEntropyWindows == 0 {
		t.Error("expected some high-entropy windows from the random section")
	}
	// Should NOT be flagged as uniformly packed.
	if p.HighEntropyRatio > 0.80 {
		t.Errorf("mixed data should not have >80%% high-entropy ratio, got %.2f", p.HighEntropyRatio)
	}
}

func TestAnalyze_StdDevCalculation(t *testing.T) {
	// All same entropy → stddev should be ~0.
	data := bytes.Repeat([]byte("ABCDEFGH"), 512) // repetitive, constant entropy
	p := Analyze(data)
	if p.EntropyStdDev > 0.1 {
		t.Errorf("constant-entropy data should have ~0 stddev, got %.4f", p.EntropyStdDev)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Layer Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestLayer_Name(t *testing.T) {
	l := New()
	if l.Name() != "entropy" {
		t.Errorf("expected 'entropy', got %q", l.Name())
	}
}

func TestLayer_Scan_SkipsSmallFiles(t *testing.T) {
	l := New()
	threats, err := l.Scan(&antivirus.ScanTarget{
		Content: []byte("small"), Filename: "tiny.exe",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(threats) != 0 {
		t.Errorf("small files should be skipped, got %d threats", len(threats))
	}
}

func TestLayer_Scan_CleanTextFile(t *testing.T) {
	l := New()
	text := bytes.Repeat([]byte("This is a normal text file with normal content. "), 50)
	threats, err := l.Scan(&antivirus.ScanTarget{
		Content: text, Filename: "readme.txt", MIMEType: "text/plain",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(threats) != 0 {
		t.Errorf("clean text should have no threats, got %d", len(threats))
	}
}

func TestLayer_Scan_SkipsHighEntropyMIME(t *testing.T) {
	l := New()
	// Random data disguised as JPEG — should not trigger entropy flags.
	data := make([]byte, 2048)
	rand.Read(data)
	threats, err := l.Scan(&antivirus.ScanTarget{
		Content: data, Filename: "photo.jpg", MIMEType: "image/jpeg",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Should skip entropy analysis for JPEG.
	for _, th := range threats {
		if !strings.HasPrefix(th.Name, "Entropy.Packer.") {
			t.Errorf("JPEG should only trigger packer findings, got %q", th.Name)
		}
	}
}

func TestLayer_Scan_PackedExecutable(t *testing.T) {
	l := New()
	// Simulate a packed PE: MZ header + random data. 16KB for reliable
	// per-window entropy measurements.
	data := make([]byte, 16384)
	data[0] = 0x4D // M
	data[1] = 0x5A // Z
	rand.Read(data[2:])

	threats, err := l.Scan(&antivirus.ScanTarget{
		Content: data, Filename: "packed.exe", MIMEType: "application/x-dosexec",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(threats) == 0 {
		t.Error("packed executable should trigger entropy findings")
	}

	foundOverall := false
	foundUniform := false
	for _, th := range threats {
		if th.Name == "Entropy.HighOverall" {
			foundOverall = true
		}
		if th.Name == "Entropy.UniformlyPacked" {
			foundUniform = true
		}
		if th.Layer != antivirus.LayerEntropy {
			t.Errorf("wrong layer: %s", th.Layer)
		}
	}
	if !foundOverall {
		t.Error("should detect high overall entropy")
	}
	if !foundUniform {
		t.Error("should detect uniformly packed pattern")
	}
}

func TestLayer_Scan_ConcurrentSafe(t *testing.T) {
	l := New()
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data := make([]byte, 1024)
			rand.Read(data)
			l.Scan(&antivirus.ScanTarget{
				Content: data, Filename: "test.bin", MIMEType: "text/plain",
			})
		}()
	}
	wg.Wait()
	if l.Stats().TotalScans != 30 {
		t.Errorf("expected 30 scans, got %d", l.Stats().TotalScans)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Packer Detection Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestPacker_UPX(t *testing.T) {
	data := make([]byte, 2048)
	copy(data[100:], []byte("UPX!"))
	threats := detectPackers(data)
	if len(threats) == 0 {
		t.Fatal("UPX! marker should be detected")
	}
	if threats[0].Name != "Entropy.Packer.UPX" {
		t.Errorf("wrong name: %s", threats[0].Name)
	}
}

func TestPacker_Themida(t *testing.T) {
	data := make([]byte, 2048)
	copy(data[200:], []byte(".themida"))
	threats := detectPackers(data)
	found := false
	for _, th := range threats {
		if th.Name == "Entropy.Packer.Themida" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Themida marker should be detected")
	}
}

func TestPacker_VMProtect(t *testing.T) {
	data := make([]byte, 2048)
	copy(data[300:], []byte(".vmp0"))
	threats := detectPackers(data)
	found := false
	for _, th := range threats {
		if th.Name == "Entropy.Packer.VMProtect" {
			found = true
			break
		}
	}
	if !found {
		t.Error("VMProtect marker should be detected")
	}
}

func TestPacker_NoneDetected(t *testing.T) {
	data := bytes.Repeat([]byte("clean data "), 200)
	threats := detectPackers(data)
	if len(threats) != 0 {
		t.Errorf("clean data should not trigger packer detection, got %d", len(threats))
	}
}

func TestPacker_Deduplication(t *testing.T) {
	data := make([]byte, 4096)
	// Place multiple UPX markers.
	copy(data[100:], []byte("UPX!"))
	copy(data[200:], []byte("UPX0"))
	threats := detectPackers(data)
	upxCount := 0
	for _, th := range threats {
		if strings.Contains(th.Name, "UPX") {
			upxCount++
		}
	}
	if upxCount > 1 {
		t.Errorf("UPX should be deduplicated, got %d", upxCount)
	}
}

func TestPacker_BeyondScanRange(t *testing.T) {
	data := make([]byte, 8192)
	// Place marker beyond 4KB scan range.
	copy(data[5000:], []byte("UPX!"))
	threats := detectPackers(data)
	if len(threats) != 0 {
		t.Error("packer marker beyond 4KB should not be detected")
	}
}

func TestPackerCount(t *testing.T) {
	if PackerCount() == 0 {
		t.Error("should have packer signatures")
	}
	t.Logf("packer signatures available: %d", PackerCount())
}

// ─────────────────────────────────────────────────────────────────────────────
// MIME Type Helpers Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestIsExpectedHighEntropy(t *testing.T) {
	tests := map[string]bool{
		"image/jpeg":       true,
		"image/png":        true,
		"application/zip":  true,
		"application/gzip": true,
		"text/plain":       false,
		"text/html":        false,
		"":                 false,
	}
	for mime, want := range tests {
		if got := isExpectedHighEntropy(mime); got != want {
			t.Errorf("isExpectedHighEntropy(%q) = %v, want %v", mime, got, want)
		}
	}
}

func TestIsExecutableMIME(t *testing.T) {
	tests := map[string]bool{
		"application/x-executable": true,
		"application/x-dosexec":    true,
		"text/plain":               false,
		"image/jpeg":               false,
	}
	for mime, want := range tests {
		if got := isExecutableMIME(mime); got != want {
			t.Errorf("isExecutableMIME(%q) = %v, want %v", mime, got, want)
		}
	}
}

func TestIsExecutableByMagic(t *testing.T) {
	pe := []byte{0x4D, 0x5A, 0x90, 0x00}
	elf := []byte{0x7F, 0x45, 0x4C, 0x46}
	txt := []byte("text")

	if !isExecutableByMagic(pe) {
		t.Error("PE should be detected as executable")
	}
	if !isExecutableByMagic(elf) {
		t.Error("ELF should be detected as executable")
	}
	if isExecutableByMagic(txt) {
		t.Error("text should not be executable")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Benchmark
// ─────────────────────────────────────────────────────────────────────────────

func BenchmarkShannon_1KB(b *testing.B) {
	data := make([]byte, 1024)
	rand.Read(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Shannon(data)
	}
}

func BenchmarkShannon_1MB(b *testing.B) {
	data := make([]byte, 1024*1024)
	rand.Read(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Shannon(data)
	}
}

func BenchmarkAnalyze_4KB(b *testing.B) {
	data := make([]byte, 4096)
	rand.Read(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Analyze(data)
	}
}

func BenchmarkAnalyze_1MB(b *testing.B) {
	data := make([]byte, 1024*1024)
	rand.Read(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Analyze(data)
	}
}
