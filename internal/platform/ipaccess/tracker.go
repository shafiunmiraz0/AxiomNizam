// Package ipaccess provides IP-based access logging, activity tracking,
// and IP blocking for the AxiomNizam platform.
//
// It captures every request's IP, method, path, status, and user agent,
// maintains per-IP activity summaries, and supports blocking/unblocking IPs.
package ipaccess

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// AccessEntry represents a single HTTP request from an IP.
type AccessEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	IP         string    `json:"ip"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	StatusCode int       `json:"statusCode"`
	UserAgent  string    `json:"userAgent"`
	Duration   string    `json:"duration"`
}

// IPSummary aggregates activity for a single IP.
type IPSummary struct {
	IP            string    `json:"ip"`
	TotalRequests int64     `json:"totalRequests"`
	LastSeen      time.Time `json:"lastSeen"`
	FirstSeen     time.Time `json:"firstSeen"`
	TopEndpoints  []EndpointStat `json:"topEndpoints,omitempty"`
	UserAgents    []string  `json:"userAgents,omitempty"`
	StatusCounts  map[int]int `json:"statusCounts,omitempty"`
	IsBlocked     bool      `json:"isBlocked"`
	IsPrivate     bool      `json:"isPrivate"`
	BlockedAt     time.Time `json:"blockedAt,omitempty"`
	BlockedReason string    `json:"blockedReason,omitempty"`
}

// EndpointStat tracks hit count for a specific method+path.
type EndpointStat struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Count  int    `json:"count"`
}

// RateLimitConfig defines per-IP rate limiting.
type RateLimitConfig struct {
	MaxRequests       int           `json:"maxRequests"`       // max requests per window
	RequestsPerMinute int           `json:"requestsPerMinute"` // per-minute limit
	BurstLimit        int           `json:"burstLimit"`        // burst limit (10s window)
	Window            string        `json:"window"`            // e.g. "1m", "5m", "1h"
	windowDur         time.Duration
	SetAt             time.Time     `json:"setAt"`
	SetBy             string        `json:"setBy"`
}

// ipRequestWindow tracks request timestamps for rate limiting.
type ipRequestWindow struct {
	minuteTimestamps []time.Time
	burstTimestamps  []time.Time
}

// Tracker captures IP access activity in memory.
type Tracker struct {
	mu          sync.RWMutex
	entries     []AccessEntry       // ring buffer of recent entries
	maxEntries  int                 // max entries to keep
	ipSummaries map[string]*IPSummary // per-IP aggregated stats
	blockedIPs  map[string]*BlockInfo // blocked IPs
	rateLimits  map[string]*RateLimitConfig // per-IP rate limits
	ipWindows   map[string]*ipRequestWindow // per-IP request timestamps for rate limiting
	kvStore     kvStore             // optional KV persistence for blocklist
}

// kvStore is a minimal interface matching platformstore.KVStore.
type kvStore interface {
	Get(ctx context.Context, key string) (string, error)
	Put(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
}

// BlockInfo stores why and when an IP was blocked.
type BlockInfo struct {
	BlockedAt time.Time `json:"blockedAt"`
	Reason    string    `json:"reason"`
	BlockedBy string    `json:"blockedBy"`
}

// NewTracker creates a new IP access tracker.
// maxEntries controls how many recent log entries to keep in memory.
func NewTracker(maxEntries int) *Tracker {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	return &Tracker{
		entries:     make([]AccessEntry, 0, maxEntries),
		maxEntries:  maxEntries,
		ipSummaries: make(map[string]*IPSummary),
		blockedIPs:  make(map[string]*BlockInfo),
		rateLimits:  make(map[string]*RateLimitConfig),
		ipWindows:   make(map[string]*ipRequestWindow),
	}
}

// SetKVStore enables KV persistence for the blocklist and rate limits.
func (t *Tracker) SetKVStore(store kvStore) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.kvStore = store
	go t.loadFromKV()
}

const kvBlocklistKey = "ipaccess:blocklist"
const kvRateLimitsKey = "ipaccess:rate-limits"

func (t *Tracker) loadFromKV() {
	if t.kvStore == nil {
		return
	}
	ctx := context.Background()

	// Load blocklist
	if val, err := t.kvStore.Get(ctx, kvBlocklistKey); err == nil && val != "" {
		var blocked map[string]*BlockInfo
		if json.Unmarshal([]byte(val), &blocked) == nil {
			t.mu.Lock()
			for ip, info := range blocked {
				t.blockedIPs[ip] = info
				if summary, exists := t.ipSummaries[ip]; exists {
					summary.IsBlocked = true
					summary.BlockedAt = info.BlockedAt
					summary.BlockedReason = info.Reason
				}
			}
			t.mu.Unlock()
			log.Printf("✅ IP access: loaded %d blocked IPs from KV", len(blocked))
		}
	}

	// Load rate limits
	if val, err := t.kvStore.Get(ctx, kvRateLimitsKey); err == nil && val != "" {
		var limits map[string]*RateLimitConfig
		if json.Unmarshal([]byte(val), &limits) == nil {
			t.mu.Lock()
			for ip, cfg := range limits {
				if d, err := time.ParseDuration(cfg.Window); err == nil {
					cfg.windowDur = d
				}
				t.rateLimits[ip] = cfg
			}
			t.mu.Unlock()
			log.Printf("✅ IP access: loaded %d rate limits from KV", len(limits))
		}
	}
}

func (t *Tracker) persistBlocklist() {
	if t.kvStore == nil {
		return
	}
	data, err := json.Marshal(t.blockedIPs)
	if err != nil {
		return
	}
	ctx := context.Background()
	if err := t.kvStore.Put(ctx, kvBlocklistKey, string(data)); err != nil {
		log.Printf("⚠️  IP access: failed to persist blocklist: %v", err)
	}
}

func (t *Tracker) persistRateLimits() {
	if t.kvStore == nil {
		return
	}
	data, err := json.Marshal(t.rateLimits)
	if err != nil {
		return
	}
	ctx := context.Background()
	if err := t.kvStore.Put(ctx, kvRateLimitsKey, string(data)); err != nil {
		log.Printf("⚠️  IP access: failed to persist rate limits: %v", err)
	}
}

// Record adds a new access entry. This is called by the middleware for every request.
func (t *Tracker) Record(entry AccessEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Add to ring buffer.
	if len(t.entries) >= t.maxEntries {
		// Shift oldest out.
		copy(t.entries, t.entries[1:])
		t.entries[len(t.entries)-1] = entry
	} else {
		t.entries = append(t.entries, entry)
	}

	// Update per-IP summary.
	summary, exists := t.ipSummaries[entry.IP]
	if !exists {
		summary = &IPSummary{
			IP:           entry.IP,
			StatusCounts: make(map[int]int),
			FirstSeen:    entry.Timestamp,
			IsPrivate:    IsPrivateIP(entry.IP),
		}
		t.ipSummaries[entry.IP] = summary
	}

	summary.TotalRequests++
	summary.LastSeen = entry.Timestamp
	summary.StatusCounts[entry.StatusCode]++

	// Track user agents (keep unique, max 5).
	uaFound := false
	for _, ua := range summary.UserAgents {
		if ua == entry.UserAgent {
			uaFound = true
			break
		}
	}
	if !uaFound && len(summary.UserAgents) < 5 && entry.UserAgent != "" {
		summary.UserAgents = append(summary.UserAgents, entry.UserAgent)
	}

	// Track endpoints (simplified — just increment counts).
	// We keep a map for O(1) lookup then sort on read.
	// For memory efficiency, we store in the summary directly.
	epFound := false
	for i := range summary.TopEndpoints {
		if summary.TopEndpoints[i].Method == entry.Method && summary.TopEndpoints[i].Path == entry.Path {
			summary.TopEndpoints[i].Count++
			epFound = true
			break
		}
	}
	if !epFound {
		summary.TopEndpoints = append(summary.TopEndpoints, EndpointStat{
			Method: entry.Method,
			Path:   entry.Path,
			Count:  1,
		})
	}

	// Keep only top 20 endpoints per IP.
	if len(summary.TopEndpoints) > 20 {
		sort.Slice(summary.TopEndpoints, func(i, j int) bool {
			return summary.TopEndpoints[i].Count > summary.TopEndpoints[j].Count
		})
		summary.TopEndpoints = summary.TopEndpoints[:20]
	}
}

// IsBlocked checks if an IP is blocked.
func (t *Tracker) IsBlocked(ip string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, blocked := t.blockedIPs[ip]
	return blocked
}

// BlockIP adds an IP to the blocklist.
func (t *Tracker) BlockIP(ip, reason, blockedBy string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.blockedIPs[ip] = &BlockInfo{
		BlockedAt: time.Now(),
		Reason:    reason,
		BlockedBy: blockedBy,
	}
	if summary, exists := t.ipSummaries[ip]; exists {
		summary.IsBlocked = true
		summary.BlockedAt = time.Now()
		summary.BlockedReason = reason
	}
	go t.persistBlocklist()
}

// UnblockIP removes an IP from the blocklist.
func (t *Tracker) UnblockIP(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.blockedIPs, ip)
	if summary, exists := t.ipSummaries[ip]; exists {
		summary.IsBlocked = false
		summary.BlockedAt = time.Time{}
		summary.BlockedReason = ""
	}
	go t.persistBlocklist()
}

// GetBlockedIPs returns all blocked IPs with their block info.
func (t *Tracker) GetBlockedIPs() map[string]*BlockInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make(map[string]*BlockInfo, len(t.blockedIPs))
	for k, v := range t.blockedIPs {
		result[k] = v
	}
	return result
}

// GetRateLimits returns all configured rate limits.
func (t *Tracker) GetRateLimits() map[string]*RateLimitConfig {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make(map[string]*RateLimitConfig, len(t.rateLimits))
	for k, v := range t.rateLimits {
		result[k] = v
	}
	return result
}

// CheckRateLimit checks if an IP has exceeded its rate limit.
// Returns (allowed, limit, remaining).
func (t *Tracker) CheckRateLimit(ip string) (bool, int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	cfg, hasLimit := t.rateLimits[ip]
	if !hasLimit {
		return true, 0, 0 // no rate limit configured
	}

	now := time.Now()
	window, exists := t.ipWindows[ip]
	if !exists {
		window = &ipRequestWindow{}
		t.ipWindows[ip] = window
	}

	// Prune timestamps outside the window
	cutoff := now.Add(-cfg.windowDur)
	pruned := window.minuteTimestamps[:0]
	for _, ts := range window.minuteTimestamps {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}
	window.minuteTimestamps = pruned

	if len(window.minuteTimestamps) >= cfg.MaxRequests {
		return false, cfg.MaxRequests, 0
	}

	window.minuteTimestamps = append(window.minuteTimestamps, now)
	return true, cfg.MaxRequests, cfg.MaxRequests - len(window.minuteTimestamps)
}

// GetRecentEntries returns the most recent access entries (up to limit).
func (t *Tracker) GetRecentEntries(limit int) []AccessEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if limit <= 0 || limit > len(t.entries) {
		limit = len(t.entries)
	}

	// Return last N entries (most recent).
	result := make([]AccessEntry, limit)
	copy(result, t.entries[len(t.entries)-limit:])
	return result
}

// GetIPSnapshots returns all IP summaries sorted by last seen (most recent first).
func (t *Tracker) GetIPSnapshots() []*IPSummary {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*IPSummary, 0, len(t.ipSummaries))
	for _, s := range t.ipSummaries {
		// Sort endpoints by count.
		sort.Slice(s.TopEndpoints, func(i, j int) bool {
			return s.TopEndpoints[i].Count > s.TopEndpoints[j].Count
		})
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastSeen.After(result[j].LastSeen)
	})
	return result
}

// GetIPSummary returns the summary for a specific IP.
func (t *Tracker) GetIPSummary(ip string) *IPSummary {
	t.mu.RLock()
	defer t.mu.RUnlock()
	summary, exists := t.ipSummaries[ip]
	if !exists {
		return nil
	}
	sort.Slice(summary.TopEndpoints, func(i, j int) bool {
		return summary.TopEndpoints[i].Count > summary.TopEndpoints[j].Count
	})
	return summary
}

// GetEntriesForIP returns recent entries for a specific IP.
func (t *Tracker) GetEntriesForIP(ip string, limit int) []AccessEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []AccessEntry
	for i := len(t.entries) - 1; i >= 0; i-- {
		if t.entries[i].IP == ip {
			result = append(result, t.entries[i])
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result
}

// UniqueIPCount returns the number of unique IPs seen.
func (t *Tracker) UniqueIPCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.ipSummaries)
}

// TotalEntries returns the total number of entries in the buffer.
func (t *Tracker) TotalEntries() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.entries)
}

// IsRateLimited checks if an IP has exceeded its rate limit.
func (t *Tracker) IsRateLimited(ip string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	cfg, exists := t.rateLimits[ip]
	if !exists {
		return false // no rate limit set for this IP
	}

	win, exists := t.ipWindows[ip]
	if !exists {
		return false
	}

	now := time.Now()

	// Check burst limit (10-second window)
	if cfg.BurstLimit > 0 {
		burstWindow := 10 * time.Second
		// Clean old burst entries
		cutoff := now.Add(-burstWindow)
		var fresh []time.Time
		for _, ts := range win.burstTimestamps {
			if ts.After(cutoff) {
				fresh = append(fresh, ts)
			}
		}
		win.burstTimestamps = fresh
		if len(win.burstTimestamps) >= cfg.BurstLimit {
			return true
		}
	}

	// Check per-minute limit
	if cfg.RequestsPerMinute > 0 {
		cutoff := now.Add(-time.Minute)
		var fresh []time.Time
		for _, ts := range win.minuteTimestamps {
			if ts.After(cutoff) {
				fresh = append(fresh, ts)
			}
		}
		win.minuteTimestamps = fresh
		if len(win.minuteTimestamps) >= cfg.RequestsPerMinute {
			return true
		}
	}

	return false
}

// RecordRate tracks a request for rate limiting purposes.
func (t *Tracker) RecordRate(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.rateLimits[ip]; !exists {
		return // no rate limit set
	}

	win, exists := t.ipWindows[ip]
	if !exists {
		win = &ipRequestWindow{}
		t.ipWindows[ip] = win
	}

	now := time.Now()
	win.minuteTimestamps = append(win.minuteTimestamps, now)
	win.burstTimestamps = append(win.burstTimestamps, now)
}

// SetRateLimit sets a per-IP rate limit.
func (t *Tracker) SetRateLimit(ip string, cfg RateLimitConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cfg.Window != "" {
		if d, err := time.ParseDuration(cfg.Window); err == nil {
			cfg.windowDur = d
		}
	}
	if cfg.windowDur == 0 {
		cfg.windowDur = 1 * time.Minute
		cfg.Window = "1m"
	}
	cfg.SetAt = time.Now()
	t.rateLimits[ip] = &cfg
	go t.persistRateLimits()
}

// GetRateLimit returns the rate limit config for an IP.
func (t *Tracker) GetRateLimit(ip string) *RateLimitConfig {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cfg, exists := t.rateLimits[ip]
	if !exists {
		return nil
	}
	return cfg
}

// GetAllRateLimits returns all rate limit configs.
func (t *Tracker) GetAllRateLimits() map[string]*RateLimitConfig {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make(map[string]*RateLimitConfig, len(t.rateLimits))
	for k, v := range t.rateLimits {
		result[k] = v
	}
	return result
}

// RemoveRateLimit removes a per-IP rate limit.
func (t *Tracker) RemoveRateLimit(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.rateLimits, ip)
	delete(t.ipWindows, ip)
	go t.persistRateLimits()
}

// GetRateLimitStatus returns rate limit status for an IP.
func (t *Tracker) GetRateLimitStatus(ip string) map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	cfg, exists := t.rateLimits[ip]
	if !exists {
		return map[string]interface{}{"limited": false, "configured": false}
	}

	now := time.Now()
	win, winExists := t.ipWindows[ip]

	minuteUsed := 0
	burstUsed := 0
	if winExists {
		for _, ts := range win.minuteTimestamps {
			if ts.After(now.Add(-time.Minute)) {
				minuteUsed++
			}
		}
		for _, ts := range win.burstTimestamps {
			if ts.After(now.Add(-10 * time.Second)) {
				burstUsed++
			}
		}
	}

	return map[string]interface{}{
		"limited":           t.IsRateLimited(ip),
		"configured":        true,
		"requestsPerMinute": cfg.RequestsPerMinute,
		"burstLimit":        cfg.BurstLimit,
		"minuteUsed":        minuteUsed,
		"burstUsed":         burstUsed,
		"setAt":             cfg.SetAt,
	}
}

// IsPrivateIP checks if an IP is a private/reserved/non-public address.
func IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return true // malformed IP treated as non-public
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast()
}

// NormalizeIP extracts the IP from "ip:port" format.
func NormalizeIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Try as-is.
		return strings.TrimSpace(addr)
	}
	return strings.TrimSpace(host)
}
