package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func makeResult(verdict antivirus.ScanVerdict) antivirus.ScanResult {
	return antivirus.ScanResult{
		Verdict:   verdict,
		ScannedAt: time.Now(),
	}
}

func fakeSHA(n int) string {
	return fmt.Sprintf("%064x", n)
}

// ─────────────────────────────────────────────────────────────────────────────
// Basic Operations
// ─────────────────────────────────────────────────────────────────────────────

func TestCache_PutGet(t *testing.T) {
	c := New(100, time.Hour)
	sha := fakeSHA(1)
	result := makeResult(antivirus.VerdictClean)

	c.Put(sha, result)
	got, ok := c.Get(sha)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Verdict != antivirus.VerdictClean {
		t.Errorf("wrong verdict: %s", got.Verdict)
	}
	if !got.CacheHit {
		t.Error("CacheHit should be true")
	}
}

func TestCache_Miss(t *testing.T) {
	c := New(100, time.Hour)
	_, ok := c.Get(fakeSHA(999))
	if ok {
		t.Error("expected cache miss")
	}
}

func TestCache_UpdateExisting(t *testing.T) {
	c := New(100, time.Hour)
	sha := fakeSHA(1)

	c.Put(sha, makeResult(antivirus.VerdictClean))
	c.Put(sha, makeResult(antivirus.VerdictMalware))

	got, ok := c.Get(sha)
	if !ok {
		t.Fatal("expected hit")
	}
	if got.Verdict != antivirus.VerdictMalware {
		t.Errorf("expected updated verdict, got %s", got.Verdict)
	}
	if c.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", c.Len())
	}
}

func TestCache_Invalidate(t *testing.T) {
	c := New(100, time.Hour)
	sha := fakeSHA(1)
	c.Put(sha, makeResult(antivirus.VerdictClean))

	removed := c.Invalidate(sha)
	if !removed {
		t.Error("expected Invalidate to return true")
	}

	_, ok := c.Get(sha)
	if ok {
		t.Error("expected miss after invalidation")
	}
}

func TestCache_Invalidate_NonExistent(t *testing.T) {
	c := New(100, time.Hour)
	if c.Invalidate(fakeSHA(999)) {
		t.Error("invalidating non-existent key should return false")
	}
}

func TestCache_InvalidateAll(t *testing.T) {
	c := New(100, time.Hour)
	for i := 0; i < 10; i++ {
		c.Put(fakeSHA(i), makeResult(antivirus.VerdictClean))
	}

	removed := c.InvalidateAll()
	if removed != 10 {
		t.Errorf("expected 10 removed, got %d", removed)
	}
	if c.Len() != 0 {
		t.Errorf("expected empty cache, got %d", c.Len())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// LRU Eviction
// ─────────────────────────────────────────────────────────────────────────────

func TestCache_LRU_Eviction(t *testing.T) {
	c := New(3, time.Hour)

	c.Put(fakeSHA(1), makeResult(antivirus.VerdictClean))
	c.Put(fakeSHA(2), makeResult(antivirus.VerdictClean))
	c.Put(fakeSHA(3), makeResult(antivirus.VerdictClean))

	// Adding a 4th should evict sha(1) (oldest).
	c.Put(fakeSHA(4), makeResult(antivirus.VerdictClean))

	if c.Len() != 3 {
		t.Errorf("expected 3 entries, got %d", c.Len())
	}
	if _, ok := c.Get(fakeSHA(1)); ok {
		t.Error("sha(1) should have been evicted")
	}
	if _, ok := c.Get(fakeSHA(4)); !ok {
		t.Error("sha(4) should be present")
	}
}

func TestCache_LRU_AccessRefreshes(t *testing.T) {
	c := New(3, time.Hour)

	c.Put(fakeSHA(1), makeResult(antivirus.VerdictClean))
	c.Put(fakeSHA(2), makeResult(antivirus.VerdictClean))
	c.Put(fakeSHA(3), makeResult(antivirus.VerdictClean))

	// Access sha(1), making it most recently used.
	c.Get(fakeSHA(1))

	// Adding sha(4) should evict sha(2) (now the oldest).
	c.Put(fakeSHA(4), makeResult(antivirus.VerdictClean))

	if _, ok := c.Get(fakeSHA(1)); !ok {
		t.Error("sha(1) should still be present (was accessed)")
	}
	if _, ok := c.Get(fakeSHA(2)); ok {
		t.Error("sha(2) should have been evicted (oldest after sha(1) access)")
	}
}

func TestCache_LRU_EvictionStats(t *testing.T) {
	c := New(2, time.Hour)

	c.Put(fakeSHA(1), makeResult(antivirus.VerdictClean))
	c.Put(fakeSHA(2), makeResult(antivirus.VerdictClean))
	c.Put(fakeSHA(3), makeResult(antivirus.VerdictClean)) // evicts sha(1)

	stats := c.Stats()
	if stats.Evictions != 1 {
		t.Errorf("expected 1 eviction, got %d", stats.Evictions)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TTL Expiration
// ─────────────────────────────────────────────────────────────────────────────

func TestCache_TTL_Expired(t *testing.T) {
	c := New(100, 50*time.Millisecond)
	sha := fakeSHA(1)
	c.Put(sha, makeResult(antivirus.VerdictClean))

	// Should be a hit immediately.
	if _, ok := c.Get(sha); !ok {
		t.Fatal("expected hit before TTL")
	}

	// Wait for TTL to expire.
	time.Sleep(100 * time.Millisecond)

	_, ok := c.Get(sha)
	if ok {
		t.Error("expected miss after TTL expiration")
	}

	// Entry should be removed.
	if c.Len() != 0 {
		t.Errorf("expired entry should be removed, got len=%d", c.Len())
	}
}

func TestCache_TTL_Zero_NeverExpires(t *testing.T) {
	c := New(100, 0) // TTL=0 means no expiration
	sha := fakeSHA(1)
	c.Put(sha, makeResult(antivirus.VerdictClean))

	// Should always hit (no TTL).
	if _, ok := c.Get(sha); !ok {
		t.Error("TTL=0 entries should never expire")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PurgeExpired
// ─────────────────────────────────────────────────────────────────────────────

func TestCache_PurgeExpired(t *testing.T) {
	c := New(100, 50*time.Millisecond)

	c.Put(fakeSHA(1), makeResult(antivirus.VerdictClean))
	c.Put(fakeSHA(2), makeResult(antivirus.VerdictClean))

	time.Sleep(100 * time.Millisecond)

	// Add a fresh entry.
	c.Put(fakeSHA(3), makeResult(antivirus.VerdictClean))

	purged := c.PurgeExpired()
	if purged != 2 {
		t.Errorf("expected 2 purged, got %d", purged)
	}
	if c.Len() != 1 {
		t.Errorf("expected 1 remaining, got %d", c.Len())
	}
}

func TestCache_PurgeExpired_NoneExpired(t *testing.T) {
	c := New(100, time.Hour)
	c.Put(fakeSHA(1), makeResult(antivirus.VerdictClean))

	purged := c.PurgeExpired()
	if purged != 0 {
		t.Errorf("expected 0 purged, got %d", purged)
	}
}

func TestCache_PurgeExpired_Disabled(t *testing.T) {
	c := New(0, time.Hour)
	if c.PurgeExpired() != 0 {
		t.Error("disabled cache should return 0")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Disabled Cache (capacity=0)
// ─────────────────────────────────────────────────────────────────────────────

func TestCache_Disabled(t *testing.T) {
	c := New(0, time.Hour)
	sha := fakeSHA(1)

	c.Put(sha, makeResult(antivirus.VerdictClean))
	_, ok := c.Get(sha)
	if ok {
		t.Error("disabled cache should always miss")
	}
	if c.Len() != 0 {
		t.Error("disabled cache should have 0 entries")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Edge Cases
// ─────────────────────────────────────────────────────────────────────────────

func TestCache_EmptyKey(t *testing.T) {
	c := New(100, time.Hour)
	c.Put("", makeResult(antivirus.VerdictClean))
	_, ok := c.Get("")
	if ok {
		t.Error("empty key should always miss")
	}
}

func TestCache_SingleCapacity(t *testing.T) {
	c := New(1, time.Hour)
	c.Put(fakeSHA(1), makeResult(antivirus.VerdictClean))
	c.Put(fakeSHA(2), makeResult(antivirus.VerdictClean))

	if c.Len() != 1 {
		t.Errorf("expected 1, got %d", c.Len())
	}
	if _, ok := c.Get(fakeSHA(1)); ok {
		t.Error("sha(1) should have been evicted")
	}
	if _, ok := c.Get(fakeSHA(2)); !ok {
		t.Error("sha(2) should be present")
	}
}

func TestCache_MalwareResult(t *testing.T) {
	c := New(100, time.Hour)
	sha := fakeSHA(1)

	result := antivirus.ScanResult{
		Verdict: antivirus.VerdictMalware,
		Threats: []antivirus.ThreatInfo{
			{Name: "TestMalware", Severity: antivirus.SeverityCritical},
		},
	}
	c.Put(sha, result)

	got, ok := c.Get(sha)
	if !ok {
		t.Fatal("expected hit")
	}
	if got.Verdict != antivirus.VerdictMalware {
		t.Error("wrong verdict")
	}
	if len(got.Threats) != 1 || got.Threats[0].Name != "TestMalware" {
		t.Error("threats not preserved")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Concurrency
// ─────────────────────────────────────────────────────────────────────────────

func TestCache_Concurrent(t *testing.T) {
	c := New(1000, time.Hour)
	var wg sync.WaitGroup

	// 50 goroutines each doing 100 puts + gets.
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				sha := fakeSHA(id*100 + i)
				c.Put(sha, makeResult(antivirus.VerdictClean))
				c.Get(sha)
			}
		}(g)
	}
	wg.Wait()

	stats := c.Stats()
	if stats.Size > 1000 {
		t.Errorf("cache exceeded capacity: %d", stats.Size)
	}
	if stats.Hits+stats.Misses == 0 {
		t.Error("expected some cache activity")
	}
	t.Logf("concurrent: size=%d hits=%d misses=%d evictions=%d",
		stats.Size, stats.Hits, stats.Misses, stats.Evictions)
}

func TestCache_ConcurrentInvalidateAll(t *testing.T) {
	c := New(1000, time.Hour)
	var wg sync.WaitGroup

	// Writers.
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				c.Put(fakeSHA(id*50+i), makeResult(antivirus.VerdictClean))
			}
		}(g)
	}

	// Concurrent flush.
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(2 * time.Millisecond)
		c.InvalidateAll()
	}()

	wg.Wait()
	// No panic = success.
}

// ─────────────────────────────────────────────────────────────────────────────
// Statistics
// ─────────────────────────────────────────────────────────────────────────────

func TestCache_Stats(t *testing.T) {
	c := New(100, 30*time.Minute)

	c.Put(fakeSHA(1), makeResult(antivirus.VerdictClean))
	c.Put(fakeSHA(2), makeResult(antivirus.VerdictClean))
	c.Get(fakeSHA(1))  // hit
	c.Get(fakeSHA(99)) // miss

	stats := c.Stats()
	if stats.Capacity != 100 {
		t.Errorf("wrong capacity: %d", stats.Capacity)
	}
	if stats.Size != 2 {
		t.Errorf("wrong size: %d", stats.Size)
	}
	if stats.TTL != "30m0s" {
		t.Errorf("wrong TTL: %s", stats.TTL)
	}
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.HitRate < 0.49 || stats.HitRate > 0.51 {
		t.Errorf("expected 50%% hit rate, got %.2f", stats.HitRate)
	}
	if stats.Inserts != 2 {
		t.Errorf("expected 2 inserts, got %d", stats.Inserts)
	}
}

func TestCache_Stats_DisabledTTL(t *testing.T) {
	c := New(100, 0)
	stats := c.Stats()
	if stats.TTL != "disabled" {
		t.Errorf("expected 'disabled', got %q", stats.TTL)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Benchmark
// ─────────────────────────────────────────────────────────────────────────────

func BenchmarkCache_Put(b *testing.B) {
	c := New(100_000, time.Hour)
	result := makeResult(antivirus.VerdictClean)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(fakeSHA(i%100_000), result)
	}
}

func BenchmarkCache_Get_Hit(b *testing.B) {
	c := New(100_000, time.Hour)
	sha := fakeSHA(1)
	c.Put(sha, makeResult(antivirus.VerdictClean))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(sha)
	}
}

func BenchmarkCache_Get_Miss(b *testing.B) {
	c := New(100_000, time.Hour)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(fakeSHA(i))
	}
}
