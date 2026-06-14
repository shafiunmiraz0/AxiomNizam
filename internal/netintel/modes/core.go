package modes

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type Mode string

const (
	ModeRealtime     Mode = "realtime"
	ModeForensics    Mode = "forensics"
	ModeThreatHunt   Mode = "threat-hunt"
	ModeCapacityPlan Mode = "capacity-planning"
	ModeAnomalyLab   Mode = "anomaly-lab"
)

type ModeConfig struct {
	Name           Mode
	Enabled        bool
	SamplingRate   float64
	RetentionHours int
	AlertThreshold float64
	Labels         map[string]string
}

type ModeEvent struct {
	Mode      Mode
	EventType string
	Source    string
	Payload   map[string]any
	Timestamp time.Time
}

type Manager struct {
	mu       sync.RWMutex
	configs  map[Mode]ModeConfig
	events   []ModeEvent
	etcd     *clientv3.Client
	kvStore  platformstore.KVStore
	stateKey string
}

type managerState struct {
	Configs map[Mode]ModeConfig `json:"configs"`
	Events  []ModeEvent         `json:"events"`
}

var (
	globalEtcdMu     sync.RWMutex
	globalEtcdClient *clientv3.Client
	globalKVMu       sync.RWMutex
	globalKVStore    platformstore.KVStore
	defaultStateKey  = "netintel:modes:state"
)

func NewManager(etcd ...*clientv3.Client) *Manager {
	var etcdClient *clientv3.Client
	if len(etcd) > 0 {
		etcdClient = etcd[0]
	} else {
		globalEtcdMu.RLock()
		etcdClient = globalEtcdClient
		globalEtcdMu.RUnlock()
	}

	globalKVMu.RLock()
	kv := globalKVStore
	globalKVMu.RUnlock()

	mgr := &Manager{
		configs:  map[Mode]ModeConfig{},
		events:   make([]ModeEvent, 0, 1024),
		etcd:     etcdClient,
		kvStore:  kv,
		stateKey: defaultStateKey,
	}
	mgr.loadState()
	return mgr
}

func ConfigureGlobalPersistence(etcd *clientv3.Client) {
	globalEtcdMu.Lock()
	globalEtcdClient = etcd
	globalEtcdMu.Unlock()
}

// ConfigureGlobalKVPersistence configures KVStore persistence for the global modes manager.
func ConfigureGlobalKVPersistence(kv platformstore.KVStore) {
	globalKVMu.Lock()
	globalKVStore = kv
	globalKVMu.Unlock()
}

// ConfigureKVPersistence sets a KVStore for Raft-mode persistence.
func (m *Manager) ConfigureKVPersistence(kv platformstore.KVStore) {
	m.mu.Lock()
	m.kvStore = kv
	m.mu.Unlock()
	m.loadState()
}

func (m *Manager) loadState() {
	var data []byte

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if m.kvStore != nil {
		val, err := m.kvStore.Get(ctx, m.stateKey)
		if err != nil || val == "" {
			return
		}
		data = []byte(val)
	} else {
		etcdClient := m.etcd
		if etcdClient == nil {
			globalEtcdMu.RLock()
			etcdClient = globalEtcdClient
			globalEtcdMu.RUnlock()
		}
		if etcdClient == nil {
			return
		}
		resp, err := etcdClient.Get(ctx, m.stateKey)
		if err != nil {
			logging.Z().Info(fmt.Sprintf("netintel-modes: failed to load persisted state from etcd: %v", err))
			return
		}
		if len(resp.Kvs) == 0 {
			return
		}
		data = resp.Kvs[0].Value
	}

	var state managerState
	if err := json.Unmarshal(data, &state); err != nil {
		logging.Z().Info(fmt.Sprintf("netintel-modes: failed to decode persisted state: %v", err))
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if state.Configs != nil {
		m.configs = state.Configs
	}
	if state.Events != nil {
		m.events = state.Events
	}
	logging.Z().Info(fmt.Sprintf("✅ netintel-modes: loaded persistent state (configs=%d, events=%d)", len(m.configs), len(m.events)))
}

func (m *Manager) persistStateLocked() {
	state := managerState{
		Configs: m.configs,
		Events:  m.events,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("netintel-modes: failed to encode state: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if m.kvStore != nil {
		if err := m.kvStore.Put(ctx, m.stateKey, string(payload)); err != nil {
			logging.Z().Info(fmt.Sprintf("netintel-modes: failed to persist state to KV: %v", err))
		}
	} else {
		etcdClient := m.etcd
		if etcdClient == nil {
			globalEtcdMu.RLock()
			etcdClient = globalEtcdClient
			globalEtcdMu.RUnlock()
		}
		if etcdClient != nil {
			if _, err := etcdClient.Put(ctx, m.stateKey, string(payload)); err != nil {
				logging.Z().Info(fmt.Sprintf("netintel-modes: failed to persist state to etcd: %v", err))
			}
		}
	}
}

func (m *Manager) Upsert(cfg ModeConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cfg.Labels == nil {
		cfg.Labels = map[string]string{}
	}
	m.configs[cfg.Name] = cfg
	m.persistStateLocked()
}

func (m *Manager) Get(name Mode) (ModeConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg, ok := m.configs[name]
	return cfg, ok
}

func (m *Manager) List() []ModeConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ModeConfig, 0, len(m.configs))
	for _, v := range m.configs {
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return string(out[i].Name) < string(out[j].Name) })
	return out
}

func (m *Manager) Record(ev ModeEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, ev)
	if len(m.events) > 50000 {
		m.events = m.events[len(m.events)-50000:]
	}
	m.persistStateLocked()
}

func (m *Manager) FindByMode(mode Mode) []ModeEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ModeEvent, 0, 128)
	for _, ev := range m.events {
		if strings.EqualFold(string(ev.Mode), string(mode)) {
			out = append(out, ev)
		}
	}
	return out
}
