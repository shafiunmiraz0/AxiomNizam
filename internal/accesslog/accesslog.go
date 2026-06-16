// Package accesslog provides an in-memory access log with IP tracking
// and IP blocking capabilities. It captures HTTP request metadata in a
// ring buffer and maintains a persistent IP blocklist.
package accesslog

import (
	"context"
	"encoding/json"
	"net"
	"sort"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

const (
	ringBufferSize  = 10000
	kvKey           = "accesslog:blocked-ips"
	kvTimeout       = 3 * time.Second
)

// Entry represents a single HTTP request in the access log.
type Entry struct {
	IP         string        `json:"ip"`
	Method     string        `json:"method"`
	Path       string        `json:"path"`
	StatusCode int           `json:"statusCode"`
	Latency    time.Duration `json:"latency"`
	UserAgent  string        `json:"userAgent"`
	Timestamp  time.Time     `json:"timestamp"`
}

// IPStats tracks request statistics for a single IP.
type IPStats struct {
	IP           string    `json:"ip"`
	RequestCount int       `json:"requestCount"`
	FirstSeen    time.Time `json:"firstSeen"`
	LastSeen     time.Time `json:"lastSeen"`
	LastPath     string    `json:"lastPath"`
	LastMethod   string    `json:"lastMethod"`
	LastStatus   int       `json:"lastStatus"`
}

// BlockedIP represents a blocked IP address.
type BlockedIP struct {
	IP        string    `json:"ip"`
	Reason    string    `json:"reason"`
	BlockedAt time.Time `json:"blockedAt"`
}

// Store is the access log store with ring buffer and IP blocklist.
type Store struct {
	mu          sync.RWMutex
	entries     []Entry
	head        int
	count       int
	ipStats     map[string]*IPStats
	blockedIPs  map[string]*BlockedIP
	kvStore     platformstore.KVStore
}

// NewStore creates a new access log store.
func NewStore() *Store {
	return &Store{
		entries:    make([]Entry, ringBufferSize),
		ipStats:    make(map[string]*IPStats),
		blockedIPs: make(map[string]*BlockedIP),
	}
}

// SetKVStore wires the KV store for blocklist persistence.
func (s *Store) SetKVStore(kv platformstore.KVStore) {
	s.kvStore = kv
	s.loadBlockedIPs()
}

// Record adds an entry to the ring buffer and updates IP stats.
func (s *Store) Record(entry Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Add to ring buffer.
	s.entries[s.head] = entry
	s.head = (s.head + 1) % ringBufferSize
	if s.count < ringBufferSize {
		s.count++
	}

	// Update IP stats.
	stats, exists := s.ipStats[entry.IP]
	if !exists {
		stats = &IPStats{
			IP:        entry.IP,
			FirstSeen: entry.Timestamp,
		}
		s.ipStats[entry.IP] = stats
	}
	stats.RequestCount++
	stats.LastSeen = entry.Timestamp
	stats.LastPath = entry.Path
	stats.LastMethod = entry.Method
	stats.LastStatus = entry.StatusCode
}

// GetEntries returns recent access log entries, optionally filtered by IP.
func (s *Store) GetEntries(limit int, ipFilter string) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > ringBufferSize {
		limit = 500
	}

	result := make([]Entry, 0, limit)
	// Iterate from newest to oldest.
	for i := 0; i < s.count && len(result) < limit; i++ {
		idx := (s.head - 1 - i + ringBufferSize) % ringBufferSize
		entry := s.entries[idx]
		if entry.Timestamp.IsZero() {
			continue
		}
		if ipFilter != "" && entry.IP != ipFilter {
			continue
		}
		result = append(result, entry)
	}
	return result
}

// GetUniqueIPs returns all unique IPs with their request statistics.
func (s *Store) GetUniqueIPs() []IPStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]IPStats, 0, len(s.ipStats))
	for _, stats := range s.ipStats {
		result = append(result, *stats)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].RequestCount > result[j].RequestCount
	})
	return result
}

// GetPublicIPs returns only public (non-private) IPs with stats.
func (s *Store) GetPublicIPs() []IPStats {
	all := s.GetUniqueIPs()
	public := make([]IPStats, 0, len(all))
	for _, stats := range all {
		if isPublicIP(stats.IP) {
			public = append(public, stats)
		}
	}
	return public
}

// BlockIP adds an IP to the blocklist.
func (s *Store) BlockIP(ip, reason string) {
	s.mu.Lock()
	s.blockedIPs[ip] = &BlockedIP{
		IP:        ip,
		Reason:    reason,
		BlockedAt: time.Now(),
	}
	s.mu.Unlock()
	s.saveBlockedIPs()
}

// UnblockIP removes an IP from the blocklist.
func (s *Store) UnblockIP(ip string) {
	s.mu.Lock()
	delete(s.blockedIPs, ip)
	s.mu.Unlock()
	s.saveBlockedIPs()
}

// IsBlocked checks if an IP is in the blocklist.
func (s *Store) IsBlocked(ip string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, blocked := s.blockedIPs[ip]
	return blocked
}

// GetBlockedIPs returns all blocked IPs.
func (s *Store) GetBlockedIPs() []BlockedIP {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]BlockedIP, 0, len(s.blockedIPs))
	for _, b := range s.blockedIPs {
		result = append(result, *b)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].BlockedAt.After(result[j].BlockedAt)
	})
	return result
}

// saveBlockedIPs persists the blocklist to KV store.
func (s *Store) saveBlockedIPs() {
	if s.kvStore == nil {
		return
	}
	s.mu.RLock()
	data, err := json.Marshal(s.blockedIPs)
	s.mu.RUnlock()
	if err != nil {
		logging.Z().Error("accesslog: failed to marshal blocked IPs")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), kvTimeout)
	defer cancel()
	if err := s.kvStore.Put(ctx, kvKey, string(data)); err != nil {
		logging.Z().Error("accesslog: failed to persist blocked IPs")
	}
}

// loadBlockedIPs loads the blocklist from KV store.
func (s *Store) loadBlockedIPs() {
	if s.kvStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), kvTimeout)
	defer cancel()
	val, err := s.kvStore.Get(ctx, kvKey)
	if err != nil || val == "" {
		return
	}
	var blocked map[string]*BlockedIP
	if err := json.Unmarshal([]byte(val), &blocked); err != nil {
		logging.Z().Error("accesslog: failed to unmarshal blocked IPs")
		return
	}
	s.mu.Lock()
	s.blockedIPs = blocked
	s.mu.Unlock()
	logging.Z().Info("✅ Access log: loaded blocked IPs from KV store")
}

// isPublicIP checks if an IP is a public (non-private) address.
func isPublicIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	// Loopback
	if ip.IsLoopback() {
		return false
	}
	// Private ranges
	if ip.IsPrivate() {
		return false
	}
	// Link-local
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return true
}
