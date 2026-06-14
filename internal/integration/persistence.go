package integration

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"context"
	"encoding/json"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

var (
	persistenceMu         sync.RWMutex
	globalPersistenceEtcd *clientv3.Client
	globalPersistenceKV   platformstore.KVStore
)

func integrationEtcdClient() *clientv3.Client {
	persistenceMu.RLock()
	defer persistenceMu.RUnlock()
	return globalPersistenceEtcd
}

func integrationKVStore() platformstore.KVStore {
	persistenceMu.RLock()
	defer persistenceMu.RUnlock()
	return globalPersistenceKV
}

func ConfigureGlobalPersistence(etcd *clientv3.Client) {
	persistenceMu.Lock()
	globalPersistenceEtcd = etcd
	persistenceMu.Unlock()

	// Phase 13: Global singletons removed.
	// Consumers should call ConfigurePersistence on their own instances.
}

// ConfigureGlobalKVPersistence configures KVStore persistence for all integration sub-modules.
func ConfigureGlobalKVPersistence(kv platformstore.KVStore) {
	persistenceMu.Lock()
	globalPersistenceKV = kv
	persistenceMu.Unlock()
}

func loadStateFromEtcd(etcd *clientv3.Client, stateKey string, out interface{}) bool {
	if etcd == nil || stateKey == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := etcd.Get(ctx, stateKey)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("integration: failed to load persisted state from etcd (%s): %v", stateKey, err))
		return false
	}
	if len(resp.Kvs) == 0 {
		return false
	}

	if err := json.Unmarshal(resp.Kvs[0].Value, out); err != nil {
		logging.Z().Info(fmt.Sprintf("integration: failed to decode persisted state (%s): %v", stateKey, err))
		return false
	}
	return true
}

func saveStateToEtcd(etcd *clientv3.Client, stateKey string, payload interface{}) {
	if etcd == nil || stateKey == "" {
		return
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("integration: failed to encode state (%s): %v", stateKey, err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := etcd.Put(ctx, stateKey, string(encoded)); err != nil {
		logging.Z().Info(fmt.Sprintf("integration: failed to persist state to etcd (%s): %v", stateKey, err))
	}
}

// loadStateFromKV loads state from a KVStore into out. Returns true on success.
func loadStateFromKV(kv platformstore.KVStore, stateKey string, out interface{}) bool {
	if kv == nil || stateKey == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	val, err := kv.Get(ctx, stateKey)
	if err != nil || val == "" {
		return false // not found or empty
	}

	if err := json.Unmarshal([]byte(val), out); err != nil {
		logging.Z().Info(fmt.Sprintf("integration: failed to decode persisted state (%s): %v", stateKey, err))
		return false
	}
	return true
}

// saveStateToKV persists state to a KVStore.
func saveStateToKV(kv platformstore.KVStore, stateKey string, payload interface{}) {
	if kv == nil || stateKey == "" {
		return
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("integration: failed to encode state (%s): %v", stateKey, err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := kv.Put(ctx, stateKey, string(encoded)); err != nil {
		logging.Z().Info(fmt.Sprintf("integration: failed to persist state to KV (%s): %v", stateKey, err))
	}
}
