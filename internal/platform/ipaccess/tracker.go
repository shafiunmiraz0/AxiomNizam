// Package ipaccess provides IP-based access logging, activity tracking,
// and IP blocking for the AxiomNizam platform.
//
// It captures every request's IP, method, path, status, and user agent,
// maintains per-IP activity summaries, and supports blocking/unblocking IPs.
package ipaccess

import (
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

// Tracker captures IP access activity in memory.
type Tracker struct {
	mu          sync.RWMutex
	entries     []AccessEntry       // ring buffer of recent entries
	maxEntries  int                 // max entries to keep
	ipSummaries map[string]*IPSummary // per-IP aggregated stats
	blockedIPs  map[string]*BlockInfo // blocked IPs
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

// IsPrivateIP checks if an IP is a private/reserved address.
func IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified()
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
