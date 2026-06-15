package metrics

// EtcdKeySpaceMonitor periodically counts keys per etcd prefix and
// exposes the counts for the /health/reconcilers endpoint and alerting.
// This is Phase 0.4 of the migration plan.

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// KeySpaceStats holds the count for one etcd prefix.
type KeySpaceStats struct {
	Prefix    string    `json:"prefix"`
	KeyCount  int64     `json:"keyCount"`
	LastCheck time.Time `json:"lastCheck"`
}

// EtcdKeySpaceMonitor tracks key counts per prefix.
type EtcdKeySpaceMonitor struct {
	client   *clientv3.Client
	prefixes []string
	interval time.Duration

	mu    sync.RWMutex
	stats map[string]*KeySpaceStats
}

// NewEtcdKeySpaceMonitor creates a monitor for the given prefixes.
func NewEtcdKeySpaceMonitor(client *clientv3.Client, prefixes []string, interval time.Duration) *EtcdKeySpaceMonitor {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	stats := make(map[string]*KeySpaceStats, len(prefixes))
	for _, p := range prefixes {
		stats[p] = &KeySpaceStats{Prefix: p}
	}
	return &EtcdKeySpaceMonitor{
		client:   client,
		prefixes: prefixes,
		interval: interval,
		stats:    stats,
	}
}

// Start begins the background polling loop. Cancel ctx to stop.
func (m *EtcdKeySpaceMonitor) Start(ctx context.Context) {
	go m.loop(ctx)
}

// GetStats returns a snapshot of all prefix counts.
func (m *EtcdKeySpaceMonitor) GetStats() []KeySpaceStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]KeySpaceStats, 0, len(m.stats))
	for _, s := range m.stats {
		out = append(out, *s)
	}
	return out
}

// GetPrefixCount returns the key count for a single prefix.
func (m *EtcdKeySpaceMonitor) GetPrefixCount(prefix string) (int64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.stats[prefix]
	if !ok {
		return 0, false
	}
	return s.KeyCount, true
}

func (m *EtcdKeySpaceMonitor) loop(ctx context.Context) {
	// Run once immediately at startup.
	m.poll(ctx)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.poll(ctx)
		}
	}
}

func (m *EtcdKeySpaceMonitor) poll(ctx context.Context) {
	if m.client == nil {
		return
	}
	for _, prefix := range m.prefixes {
		countCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		resp, err := m.client.Get(countCtx, prefix, clientv3.WithPrefix(), clientv3.WithCountOnly())
		cancel()

		m.mu.Lock()
		s, ok := m.stats[prefix]
		if !ok {
			s = &KeySpaceStats{Prefix: prefix}
			m.stats[prefix] = s
		}
		if err != nil {
			logging.Z().Info(fmt.Sprintf("etcd-keyspace module=%s err=%q", prefix, err.Error()))
		} else {
			s.KeyCount = resp.Count
			s.LastCheck = time.Now()

			// Alert threshold: log warning if any prefix exceeds 10,000 keys.
			if resp.Count > 10000 {
				logging.Z().Info(fmt.Sprintf("⚠️  etcd-keyspace prefix=%s count=%d exceeds 10000 threshold", prefix, resp.Count))
			}
		}
		m.mu.Unlock()
	}
}
