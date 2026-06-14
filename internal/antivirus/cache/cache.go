// Package cache implements an LRU scan-result cache for the AxiomNizam
// antivirus engine.
//
// # Purpose
//
// Over 99% of files scanned in a production object storage system are clean.
// When the same file is uploaded multiple times (or accessed via different
// keys), re-scanning wastes CPU. This cache stores recent scan results
// keyed by SHA-256 hash, eliminating redundant work.
//
// # Design
//
// The cache uses a doubly-linked list + map combination for O(1) LRU
// operations:
//
//   - Get: O(1) map lookup + move to front
//   - Put: O(1) map insert + evict if over capacity
//   - Invalidate: O(1) map delete
//   - InvalidateAll: O(1) full reset
//
// # Thread Safety
//
// All operations are protected by a sync.RWMutex. Get uses a write lock
// (since it mutates the LRU order), but the critical section is minimal.
//
// # Cache Invalidation
//
// The cache should be invalidated when:
//
//   - Signature database is updated (new signatures may detect previously
//     "clean" files)
//   - Cache TTL expires (per-entry, checked on Get)
//   - Manual flush via admin API
package cache

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Cache Entry
// ─────────────────────────────────────────────────────────────────────────────

// entry stores a cached scan result along with its LRU metadata.
type entry struct {
	key       string               // SHA-256 hex hash (cache key)
	result    antivirus.ScanResult // cached scan result
	createdAt time.Time            // when this entry was inserted
}

// ─────────────────────────────────────────────────────────────────────────────
// LRU Cache
// ─────────────────────────────────────────────────────────────────────────────

// Cache is a thread-safe LRU scan-result cache keyed by SHA-256 hash.
type Cache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration

	// items maps SHA-256 → list element (containing *entry).
	items map[string]*list.Element

	// order is a doubly-linked list where the front is the most recently
	// used and the back is the least recently used.
	order *list.List

	// Statistics.
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
	inserts   atomic.Int64
}

// New creates a new LRU cache with the given capacity and TTL.
// Capacity of 0 disables the cache (all Gets return miss).
// TTL of 0 means entries never expire (only evicted by LRU).
func New(capacity int, ttl time.Duration) *Cache {
	if capacity < 0 {
		capacity = 0
	}
	return &Cache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element, capacity),
		order:    list.New(),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Core Operations
// ─────────────────────────────────────────────────────────────────────────────

// Get retrieves a cached scan result by SHA-256 hash.
// Returns the result and true on hit, or a zero value and false on miss.
// Expired entries are treated as misses and removed.
func (c *Cache) Get(sha256 string) (antivirus.ScanResult, bool) {
	if c.capacity == 0 || sha256 == "" {
		c.misses.Add(1)
		return antivirus.ScanResult{}, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.items[sha256]
	if !found {
		c.misses.Add(1)
		return antivirus.ScanResult{}, false
	}

	ent := elem.Value.(*entry)

	// Check TTL expiration.
	if c.ttl > 0 && time.Since(ent.createdAt) > c.ttl {
		// Entry expired — remove and return miss.
		c.removeLocked(elem)
		c.misses.Add(1)
		return antivirus.ScanResult{}, false
	}

	// Move to front (most recently used).
	c.order.MoveToFront(elem)
	c.hits.Add(1)

	// Mark as cache hit.
	result := ent.result
	result.CacheHit = true
	return result, true
}

// Put stores a scan result in the cache, keyed by SHA-256 hash.
// If the key already exists, it is updated and moved to front.
// If the cache is at capacity, the least recently used entry is evicted.
func (c *Cache) Put(sha256 string, result antivirus.ScanResult) {
	if c.capacity == 0 || sha256 == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry.
	if elem, found := c.items[sha256]; found {
		ent := elem.Value.(*entry)
		ent.result = result
		ent.createdAt = time.Now()
		c.order.MoveToFront(elem)
		return
	}

	// Evict LRU entry if at capacity.
	for c.order.Len() >= c.capacity {
		c.evictOldestLocked()
	}

	// Insert new entry at front.
	ent := &entry{
		key:       sha256,
		result:    result,
		createdAt: time.Now(),
	}
	elem := c.order.PushFront(ent)
	c.items[sha256] = elem
	c.inserts.Add(1)
}

// Invalidate removes a specific SHA-256 entry from the cache.
// Returns true if the entry was found and removed.
func (c *Cache) Invalidate(sha256 string) bool {
	if c.capacity == 0 {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.items[sha256]
	if !found {
		return false
	}

	c.removeLocked(elem)
	return true
}

// InvalidateAll clears the entire cache. This should be called when the
// signature database is updated, since previously "clean" files may now
// be detected with new signatures.
func (c *Cache) InvalidateAll() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := len(c.items)
	c.items = make(map[string]*list.Element, c.capacity)
	c.order.Init()
	return count
}

// Len returns the current number of entries in the cache.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

// removeLocked removes a specific element from the cache. Caller must hold mu.
func (c *Cache) removeLocked(elem *list.Element) {
	ent := elem.Value.(*entry)
	delete(c.items, ent.key)
	c.order.Remove(elem)
}

// evictOldestLocked removes the least recently used entry. Caller must hold mu.
func (c *Cache) evictOldestLocked() {
	back := c.order.Back()
	if back == nil {
		return
	}
	c.removeLocked(back)
	c.evictions.Add(1)
}

// ─────────────────────────────────────────────────────────────────────────────
// Expired Entry Cleanup
// ─────────────────────────────────────────────────────────────────────────────

// PurgeExpired walks the cache from the oldest entry (back) and removes
// all entries that have exceeded their TTL. Returns the number of entries
// removed. This is designed to be called periodically (e.g. every 5 minutes)
// from a background goroutine to prevent memory buildup from stale entries.
func (c *Cache) PurgeExpired() int {
	if c.capacity == 0 || c.ttl == 0 {
		return 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0

	// Walk from back (oldest) — once we hit a non-expired entry, stop.
	for {
		back := c.order.Back()
		if back == nil {
			break
		}
		ent := back.Value.(*entry)
		if now.Sub(ent.createdAt) <= c.ttl {
			break // this and everything ahead of it is still valid
		}
		c.removeLocked(back)
		removed++
	}

	return removed
}

// ─────────────────────────────────────────────────────────────────────────────
// Statistics
// ─────────────────────────────────────────────────────────────────────────────

// Stats holds cache performance metrics.
type Stats struct {
	Capacity  int     `json:"capacity"`
	Size      int     `json:"size"`
	TTL       string  `json:"ttl"`
	Hits      int64   `json:"hits"`
	Misses    int64   `json:"misses"`
	HitRate   float64 `json:"hitRate"`
	Evictions int64   `json:"evictions"`
	Inserts   int64   `json:"inserts"`
}

// Stats returns a snapshot of cache performance metrics.
func (c *Cache) Stats() Stats {
	c.mu.Lock()
	size := c.order.Len()
	c.mu.Unlock()

	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses

	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	ttlStr := "disabled"
	if c.ttl > 0 {
		ttlStr = c.ttl.String()
	}

	return Stats{
		Capacity:  c.capacity,
		Size:      size,
		TTL:       ttlStr,
		Hits:      hits,
		Misses:    misses,
		HitRate:   hitRate,
		Evictions: c.evictions.Load(),
		Inserts:   c.inserts.Load(),
	}
}
