package vectorplus

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"context"
	"encoding/json"
	"math"
	"sort"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type Vector []float64

type Record struct {
	ID     string
	Vec    Vector
	Labels map[string]string
}

type SearchResult struct {
	ID       string
	Score    float64
	Distance float64
}

type Index struct {
	mu       sync.RWMutex
	dim      int
	records  map[string]Record
	etcd     *clientv3.Client
	kvStore  platformstore.KVStore
	stateKey string
}

type indexState struct {
	Dim     int               `json:"dim"`
	Records map[string]Record `json:"records"`
}

var (
	globalEtcdMu     sync.RWMutex
	globalEtcdClient *clientv3.Client
	globalKVMu       sync.RWMutex
	globalKVStore    platformstore.KVStore
	defaultStateKey  = "vectorplus:index:state"
)

func NewIndex(dim int, etcd ...*clientv3.Client) *Index {
	if dim < 1 {
		dim = 1
	}

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

	idx := &Index{
		dim:      dim,
		records:  make(map[string]Record),
		etcd:     etcdClient,
		kvStore:  kv,
		stateKey: defaultStateKey,
	}
	idx.loadState()
	return idx
}

// ConfigureKVPersistence sets a KVStore for Raft-mode persistence.
func (idx *Index) ConfigureKVPersistence(kv platformstore.KVStore) {
	idx.mu.Lock()
	idx.kvStore = kv
	idx.mu.Unlock()
	idx.loadState()
}

func ConfigureGlobalPersistence(etcd *clientv3.Client) {
	globalEtcdMu.Lock()
	globalEtcdClient = etcd
	globalEtcdMu.Unlock()
}

// ConfigureGlobalKVPersistence configures KVStore persistence for the global vectorplus index.
func ConfigureGlobalKVPersistence(kv platformstore.KVStore) {
	globalKVMu.Lock()
	globalKVStore = kv
	globalKVMu.Unlock()
}

func (idx *Index) loadState() {
	var data []byte

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if idx.kvStore != nil {
		val, err := idx.kvStore.Get(ctx, idx.stateKey)
		if err != nil {
			return
		}
		data = []byte(val)
	} else {
		etcdClient := idx.etcd
		if etcdClient == nil {
			globalEtcdMu.RLock()
			etcdClient = globalEtcdClient
			globalEtcdMu.RUnlock()
		}
		if etcdClient == nil {
			return
		}
		resp, err := etcdClient.Get(ctx, idx.stateKey)
		if err != nil {
			logging.Z().Info(fmt.Sprintf("vectorplus: failed to load persisted state from etcd: %v", err))
			return
		}
		if len(resp.Kvs) == 0 {
			return
		}
		data = resp.Kvs[0].Value
	}

	var state indexState
	if err := json.Unmarshal(data, &state); err != nil {
		logging.Z().Info(fmt.Sprintf("vectorplus: failed to decode persisted state: %v", err))
		return
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()
	if state.Dim > 0 {
		idx.dim = state.Dim
	}
	if state.Records != nil {
		idx.records = state.Records
	}
	logging.Z().Info(fmt.Sprintf("✅ vectorplus: loaded persistent state (records=%d)", len(idx.records)))
}

func (idx *Index) persistStateLocked() {
	state := indexState{Dim: idx.dim, Records: idx.records}
	payload, err := json.Marshal(state)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("vectorplus: failed to encode state: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if idx.kvStore != nil {
		if err := idx.kvStore.Put(ctx, idx.stateKey, string(payload)); err != nil {
			logging.Z().Info(fmt.Sprintf("vectorplus: failed to persist state to KV: %v", err))
		}
	} else {
		etcdClient := idx.etcd
		if etcdClient == nil {
			globalEtcdMu.RLock()
			etcdClient = globalEtcdClient
			globalEtcdMu.RUnlock()
		}
		if etcdClient == nil {
			return
		}
		if _, err := etcdClient.Put(ctx, idx.stateKey, string(payload)); err != nil {
			logging.Z().Info(fmt.Sprintf("vectorplus: failed to persist state to etcd: %v", err))
		}
	}
}

func (idx *Index) Upsert(r Record) bool {
	if len(r.Vec) != idx.dim || r.ID == "" {
		return false
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if r.Labels == nil {
		r.Labels = map[string]string{}
	}
	idx.records[r.ID] = r
	idx.persistStateLocked()
	return true
}

func (idx *Index) Delete(id string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.records, id)
	idx.persistStateLocked()
}

func dot(a, b Vector) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var out float64
	for i := 0; i < n; i++ {
		out += a[i] * b[i]
	}
	return out
}

func l2(a, b Vector) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var out float64
	for i := 0; i < n; i++ {
		d := a[i] - b[i]
		out += d * d
	}
	return math.Sqrt(out)
}

func (idx *Index) Search(query Vector, k int) []SearchResult {
	if len(query) != idx.dim || k < 1 {
		return nil
	}
	idx.mu.RLock()
	list := make([]Record, 0, len(idx.records))
	for _, r := range idx.records {
		list = append(list, r)
	}
	idx.mu.RUnlock()

	out := make([]SearchResult, 0, len(list))
	for _, r := range list {
		d := l2(query, r.Vec)
		s := dot(query, r.Vec)
		out = append(out, SearchResult{ID: r.ID, Score: s, Distance: d})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Distance == out[j].Distance {
			return out[i].Score > out[j].Score
		}
		return out[i].Distance < out[j].Distance
	})
	if len(out) > k {
		out = out[:k]
	}
	return out
}
