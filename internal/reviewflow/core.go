package reviewflow

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

type Stage string

const (
	StageDraft    Stage = "draft"
	StageReview   Stage = "review"
	StageApproved Stage = "approved"
	StageRejected Stage = "rejected"
	StageMerged   Stage = "merged"
)

type ReviewItem struct {
	ID          string
	Title       string
	Description string
	Author      string
	Tags        []string
	Score       float64
	Stage       Stage
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Pipeline struct {
	mu       sync.RWMutex
	items    map[string]ReviewItem
	etcd     *clientv3.Client
	kvStore  platformstore.KVStore
	stateKey string
}

type pipelineState struct {
	Items map[string]ReviewItem `json:"items"`
}

var (
	globalEtcdMu     sync.RWMutex
	globalEtcdClient *clientv3.Client
	globalKVMu       sync.RWMutex
	globalKVStore    platformstore.KVStore
	defaultStateKey  = "reviewflow:pipeline:state"
)

func NewPipeline(etcd ...*clientv3.Client) *Pipeline {
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

	p := &Pipeline{items: make(map[string]ReviewItem), etcd: etcdClient, kvStore: kv, stateKey: defaultStateKey}
	p.loadState()
	return p
}

func ConfigureGlobalPersistence(etcd *clientv3.Client) {
	globalEtcdMu.Lock()
	globalEtcdClient = etcd
	globalEtcdMu.Unlock()
}

// ConfigureGlobalKVPersistence configures KVStore persistence for the global reviewflow pipeline.
func ConfigureGlobalKVPersistence(kv platformstore.KVStore) {
	globalKVMu.Lock()
	globalKVStore = kv
	globalKVMu.Unlock()
}

// ConfigureKVPersistence sets a KVStore for Raft-mode persistence.
func (p *Pipeline) ConfigureKVPersistence(kv platformstore.KVStore) {
	p.mu.Lock()
	p.kvStore = kv
	p.mu.Unlock()
	p.loadState()
}

func (p *Pipeline) loadState() {
	var data []byte

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if p.kvStore != nil {
		val, err := p.kvStore.Get(ctx, p.stateKey)
		if err != nil {
			return
		}
		data = []byte(val)
	} else {
		etcdClient := p.etcd
		if etcdClient == nil {
			globalEtcdMu.RLock()
			etcdClient = globalEtcdClient
			globalEtcdMu.RUnlock()
		}
		if etcdClient == nil {
			return
		}
		resp, err := etcdClient.Get(ctx, p.stateKey)
		if err != nil {
			logging.Z().Info(fmt.Sprintf("reviewflow: failed to load persisted state from etcd: %v", err))
			return
		}
		if len(resp.Kvs) == 0 {
			return
		}
		data = resp.Kvs[0].Value
	}

	var state pipelineState
	if err := json.Unmarshal(data, &state); err != nil {
		logging.Z().Info(fmt.Sprintf("reviewflow: failed to decode persisted state: %v", err))
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if state.Items != nil {
		p.items = state.Items
	}
	logging.Z().Info(fmt.Sprintf("✅ reviewflow: loaded persistent state (items=%d)", len(p.items)))
}

func (p *Pipeline) persistStateLocked() {
	state := pipelineState{Items: p.items}
	payload, err := json.Marshal(state)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("reviewflow: failed to encode state: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if p.kvStore != nil {
		if err := p.kvStore.Put(ctx, p.stateKey, string(payload)); err != nil {
			logging.Z().Info(fmt.Sprintf("reviewflow: failed to persist state to KV: %v", err))
		}
	} else {
		etcdClient := p.etcd
		if etcdClient == nil {
			globalEtcdMu.RLock()
			etcdClient = globalEtcdClient
			globalEtcdMu.RUnlock()
		}
		if etcdClient != nil {
			if _, err := etcdClient.Put(ctx, p.stateKey, string(payload)); err != nil {
				logging.Z().Info(fmt.Sprintf("reviewflow: failed to persist state to etcd: %v", err))
			}
		}
	}
}

func (p *Pipeline) Upsert(item ReviewItem) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	if item.Stage == "" {
		item.Stage = StageDraft
	}
	p.items[item.ID] = item
	p.persistStateLocked()
}

func (p *Pipeline) Get(id string) (ReviewItem, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	item, ok := p.items[id]
	return item, ok
}

func (p *Pipeline) ListByStage(stage Stage) []ReviewItem {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]ReviewItem, 0, len(p.items))
	for _, item := range p.items {
		if stage == "" || item.Stage == stage {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func (p *Pipeline) Advance(id string, target Stage) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	item, ok := p.items[id]
	if !ok {
		return false
	}
	item.Stage = target
	item.UpdatedAt = time.Now().UTC()
	p.items[id] = item
	p.persistStateLocked()
	return true
}

func ScoreBySignals(title, desc string, tags []string) float64 {
	score := 0.0
	score += float64(len(strings.Fields(title))) * 0.2
	score += float64(len(strings.Fields(desc))) * 0.05
	score += float64(len(tags)) * 0.7
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		switch t {
		case "security", "reliability", "breaking-change":
			score += 2.0
		case "bugfix", "performance":
			score += 1.3
		case "docs", "chore":
			score += 0.2
		}
	}
	return score
}
