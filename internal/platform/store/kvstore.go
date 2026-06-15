// Package store — kvstore.go
//
// Phase 6 of the etcd replacement plan: KVStore interface.
//
// Several modules (workflows, vectorplus, reviewflow, storage, IAM)
// use etcd directly as a simple key-value store rather than through
// the typed ResourceStore[T] interface.  This file provides a thin
// KVStore abstraction with implementations for both etcd and go-memdb
// so those modules can be migrated without a full resource-type
// refactor.
//
// Operations:
//   - Get / Put / Delete — basic KV CRUD
//   - List — prefix scan
//   - PutWithTTL — time-limited entries (for tokens, sessions)
//   - CAS — compare-and-swap for atomic initialisation
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	axraft "axiomnizam.bitbd.net/axiomnizam/internal/platform/raft"

	"github.com/hashicorp/go-memdb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// KVStore is a backend-agnostic key-value store for modules that need
// raw KV access (not typed ResourceStore).
type KVStore interface {
	// Get retrieves the value for a key.  Returns ("", nil) if not found.
	Get(ctx context.Context, key string) (string, error)

	// Put stores a value under a key.
	Put(ctx context.Context, key, value string) error

	// PutWithTTL stores a value with a time-to-live.  After the TTL
	// expires the key is automatically removed.  Implementations that
	// don't support TTL natively should store the entry and rely on
	// periodic cleanup.
	PutWithTTL(ctx context.Context, key, value string, ttl time.Duration) error

	// Delete removes a key.
	Delete(ctx context.Context, key string) error

	// List returns all key-value pairs whose key starts with prefix.
	List(ctx context.Context, prefix string) (map[string]string, error)

	// CAS performs a compare-and-swap: if the key does not exist, set
	// it to value and return (value, true, nil).  If the key already
	// exists, return (existingValue, false, nil).
	CAS(ctx context.Context, key, value string) (string, bool, error)
}

// ─────────────────────────────────────────────
// etcd implementation
// ─────────────────────────────────────────────

// EtcdKVStore implements KVStore using an etcd client.
type EtcdKVStore struct {
	client *clientv3.Client
}

// NewEtcdKVStore creates a KVStore backed by etcd.
func NewEtcdKVStore(client *clientv3.Client) *EtcdKVStore {
	return &EtcdKVStore{client: client}
}

func (s *EtcdKVStore) Get(ctx context.Context, key string) (string, error) {
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return "", fmt.Errorf("etcd kv get: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return "", nil
	}
	return string(resp.Kvs[0].Value), nil
}

func (s *EtcdKVStore) Put(ctx context.Context, key, value string) error {
	_, err := s.client.Put(ctx, key, value)
	if err != nil {
		return fmt.Errorf("etcd kv put: %w", err)
	}
	return nil
}

func (s *EtcdKVStore) PutWithTTL(ctx context.Context, key, value string, ttl time.Duration) error {
	lease, err := s.client.Grant(ctx, int64(ttl.Seconds()))
	if err != nil {
		return fmt.Errorf("etcd kv grant lease: %w", err)
	}
	_, err = s.client.Put(ctx, key, value, clientv3.WithLease(lease.ID))
	if err != nil {
		return fmt.Errorf("etcd kv put with ttl: %w", err)
	}
	return nil
}

func (s *EtcdKVStore) Delete(ctx context.Context, key string) error {
	_, err := s.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("etcd kv delete: %w", err)
	}
	return nil
}

func (s *EtcdKVStore) List(ctx context.Context, prefix string) (map[string]string, error) {
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd kv list: %w", err)
	}
	result := make(map[string]string, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		result[string(kv.Key)] = string(kv.Value)
	}
	return result, nil
}

func (s *EtcdKVStore) CAS(ctx context.Context, key, value string) (string, bool, error) {
	txnResp, err := s.client.Txn(ctx).
		If(clientv3.Compare(clientv3.Version(key), "=", 0)).
		Then(clientv3.OpPut(key, value)).
		Else(clientv3.OpGet(key)).
		Commit()
	if err != nil {
		return "", false, fmt.Errorf("etcd kv cas: %w", err)
	}
	if txnResp.Succeeded {
		return value, true, nil
	}
	// Key already existed — return existing value.
	existing := ""
	if len(txnResp.Responses) > 0 {
		rangeResp := txnResp.Responses[0].GetResponseRange()
		if rangeResp != nil && len(rangeResp.Kvs) > 0 {
			existing = string(rangeResp.Kvs[0].Value)
		}
	}
	return existing, false, nil
}

// ─────────────────────────────────────────────
// go-memdb implementation (for Raft backend)
// ─────────────────────────────────────────────

const kvTableName = "_kv"

// kvEntry is stored in the go-memdb _kv table.
type kvEntry struct {
	Key       string
	Namespace string // unused for KV, but required by schema
	Data      []byte
	ExpiresAt time.Time // zero means no expiry
}

// MemDBKVStore implements KVStore using go-memdb.  For the Raft
// backend, writes go through Raft consensus.
type MemDBKVStore struct {
	db         *memdb.MemDB
	raftServer *axraft.Server // nil for standalone memdb mode

	mu      sync.Mutex
	ttlStop chan struct{}
}

// NewMemDBKVStore creates a KVStore backed by go-memdb.  If
// raftServer is non-nil, writes are routed through Raft.
func NewMemDBKVStore(db *memdb.MemDB, raftServer *axraft.Server) *MemDBKVStore {
	s := &MemDBKVStore{
		db:         db,
		raftServer: raftServer,
		ttlStop:    make(chan struct{}),
	}
	// Start background TTL reaper.
	go s.ttlReaper()
	return s
}

func (s *MemDBKVStore) Get(ctx context.Context, key string) (string, error) {
	txn := s.db.Txn(false)
	defer txn.Abort()

	raw, err := txn.First(kvTableName, "id", key)
	if err != nil {
		return "", fmt.Errorf("memdb kv get: %w", err)
	}
	if raw == nil {
		return "", nil
	}
	data, expiresAt := kvExtract(raw)
	// Check TTL expiry.
	if !expiresAt.IsZero() && time.Now().After(expiresAt) {
		return "", nil
	}
	return string(data), nil
}

func (s *MemDBKVStore) Put(ctx context.Context, key, value string) error {
	return s.putInternal(ctx, key, value, time.Time{})
}

func (s *MemDBKVStore) PutWithTTL(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.putInternal(ctx, key, value, time.Now().Add(ttl))
}

func (s *MemDBKVStore) putInternal(_ context.Context, key, value string, expiresAt time.Time) error {
	if s.raftServer != nil {
		// Route through Raft for consensus.
		cmd := &axraft.Command{
			Type:  axraft.CommandUpdate,
			Table: kvTableName,
			Key:   key,
			Data:  []byte(value),
		}
		encoded, err := axraft.EncodeCommand(cmd)
		if err != nil {
			return fmt.Errorf("memdb kv put encode: %w", err)
		}
		if err := s.raftServer.Apply(encoded, 5*time.Second); err != nil {
			return fmt.Errorf("memdb kv put raft: %w", err)
		}
		// Also store TTL metadata locally (TTL is node-local concern).
		if !expiresAt.IsZero() {
			s.setTTL(key, expiresAt)
		}
		return nil
	}

	// Direct memdb write (standalone mode).
	entry := &kvEntry{
		Key:       key,
		Data:      []byte(value),
		ExpiresAt: expiresAt,
	}
	txn := s.db.Txn(true)
	if err := txn.Insert(kvTableName, entry); err != nil {
		txn.Abort()
		return fmt.Errorf("memdb kv put: %w", err)
	}
	txn.Commit()
	return nil
}

func (s *MemDBKVStore) Delete(ctx context.Context, key string) error {
	if s.raftServer != nil {
		cmd := &axraft.Command{
			Type:  axraft.CommandDelete,
			Table: kvTableName,
			Key:   key,
		}
		encoded, err := axraft.EncodeCommand(cmd)
		if err != nil {
			return fmt.Errorf("memdb kv delete encode: %w", err)
		}
		return s.raftServer.Apply(encoded, 5*time.Second)
	}

	txn := s.db.Txn(true)
	raw, err := txn.First(kvTableName, "id", key)
	if err != nil {
		txn.Abort()
		return fmt.Errorf("memdb kv delete lookup: %w", err)
	}
	if raw != nil {
		if err := txn.Delete(kvTableName, raw); err != nil {
			txn.Abort()
			return fmt.Errorf("memdb kv delete: %w", err)
		}
	}
	txn.Commit()
	return nil
}

func (s *MemDBKVStore) List(ctx context.Context, prefix string) (map[string]string, error) {
	txn := s.db.Txn(false)
	defer txn.Abort()

	it, err := txn.Get(kvTableName, "id")
	if err != nil {
		return nil, fmt.Errorf("memdb kv list: %w", err)
	}

	now := time.Now()
	result := make(map[string]string)
	for raw := it.Next(); raw != nil; raw = it.Next() {
		key, data, expiresAt := kvExtractFull(raw)
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if !expiresAt.IsZero() && now.After(expiresAt) {
			continue // expired
		}
		result[key] = string(data)
	}
	return result, nil
}

func (s *MemDBKVStore) CAS(ctx context.Context, key, value string) (string, bool, error) {
	// For memdb, we do an atomic check-and-set in a write transaction.
	txn := s.db.Txn(true)

	raw, err := txn.First(kvTableName, "id", key)
	if err != nil {
		txn.Abort()
		return "", false, fmt.Errorf("memdb kv cas lookup: %w", err)
	}
	if raw != nil {
		data, expiresAt := kvExtract(raw)
		if !expiresAt.IsZero() && time.Now().After(expiresAt) {
			// Expired — treat as absent, fall through to create.
		} else {
			txn.Abort()
			return string(data), false, nil
		}
	}

	// Key doesn't exist — create it.
	entry := &kvEntry{Key: key, Data: []byte(value)}
	if err := txn.Insert(kvTableName, entry); err != nil {
		txn.Abort()
		return "", false, fmt.Errorf("memdb kv cas insert: %w", err)
	}
	txn.Commit()
	return value, true, nil
}

// kvExtract reads Data and ExpiresAt from a raw go-memdb entry,
// handling both *kvEntry (from MemDBKVStore) and *kvFSMEntry
// (from the Raft FSM) which have the same field layout.
func kvExtract(raw interface{}) (data []byte, expiresAt time.Time) {
	if e, ok := raw.(*kvEntry); ok {
		return e.Data, e.ExpiresAt
	}
	// For FSM-inserted entries (different Go type, same field names),
	// use a structural interface to extract fields without importing
	// the raft package (which would create a circular dependency).
	type dataGetter interface {
		GetData() []byte
	}
	if d, ok := raw.(dataGetter); ok {
		return d.GetData(), time.Time{}
	}
	// Fallback: use struct field access via the kvCompat adapter.
	return kvExtractViaReflect(raw)
}

// kvExtractFull reads Key, Data, and ExpiresAt from a raw entry.
func kvExtractFull(raw interface{}) (key string, data []byte, expiresAt time.Time) {
	if e, ok := raw.(*kvEntry); ok {
		return e.Key, e.Data, e.ExpiresAt
	}
	return kvExtractFullViaReflect(raw)
}

// kvExtractViaReflect handles entries from the Raft FSM which have
// the same field names but a different Go type.
func kvExtractViaReflect(raw interface{}) ([]byte, time.Time) {
	// Use JSON round-trip as a safe fallback.
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, time.Time{}
	}
	var m struct {
		Data      []byte    `json:"Data"`
		ExpiresAt time.Time `json:"ExpiresAt"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, time.Time{}
	}
	return m.Data, m.ExpiresAt
}

func kvExtractFullViaReflect(raw interface{}) (string, []byte, time.Time) {
	b, err := json.Marshal(raw)
	if err != nil {
		return "", nil, time.Time{}
	}
	var m struct {
		Key       string    `json:"Key"`
		Data      []byte    `json:"Data"`
		ExpiresAt time.Time `json:"ExpiresAt"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return "", nil, time.Time{}
	}
	return m.Key, m.Data, m.ExpiresAt
}

// setTTL records TTL metadata for a key (used in Raft mode where the
// FSM doesn't track expiry).
func (s *MemDBKVStore) setTTL(key string, expiresAt time.Time) {
	// Update the entry's ExpiresAt in a local write transaction.
	txn := s.db.Txn(true)
	raw, err := txn.First(kvTableName, "id", key)
	if err != nil || raw == nil {
		txn.Abort()
		return
	}
	data, _ := kvExtract(raw)
	updated := &kvEntry{
		Key:       key,
		Data:      data,
		ExpiresAt: expiresAt,
	}
	if err := txn.Insert(kvTableName, updated); err != nil {
		txn.Abort()
		return
	}
	txn.Commit()
}

// ttlReaper periodically removes expired entries.
func (s *MemDBKVStore) ttlReaper() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.reapExpired()
		case <-s.ttlStop:
			return
		}
	}
}

func (s *MemDBKVStore) reapExpired() {
	txn := s.db.Txn(true)
	it, err := txn.Get(kvTableName, "id")
	if err != nil {
		txn.Abort()
		return
	}

	now := time.Now()
	var toDelete []interface{}
	for raw := it.Next(); raw != nil; raw = it.Next() {
		_, expiresAt := kvExtract(raw)
		if !expiresAt.IsZero() && now.After(expiresAt) {
			toDelete = append(toDelete, raw)
		}
	}

	for _, d := range toDelete {
		_ = txn.Delete(kvTableName, d)
	}
	txn.Commit()
}

// Close stops the TTL reaper.
func (s *MemDBKVStore) Close() {
	close(s.ttlStop)
}

// ─────────────────────────────────────────────
// KVStore from BackendManager
// ─────────────────────────────────────────────

// kvEntryJSON is used for JSON serialization in snapshots.
type kvEntryJSON struct {
	Key       string    `json:"key"`
	Data      string    `json:"data"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// NewKVStore creates a KVStore from the active BackendManager.
func NewKVStore(bm *BackendManager) KVStore {
	switch bm.Backend {
	case BackendRaft:
		return NewMemDBKVStore(bm.RaftServer.DB(), bm.RaftServer)
	default:
		return NewEtcdKVStore(bm.EtcdClient)
	}
}

// KVTableSchema returns the go-memdb table schema for the _kv table.
// This must be included in the multi-table schema when using Raft.
func KVTableSchema() *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: kvTableName,
		Indexes: map[string]*memdb.IndexSchema{
			"id": {
				Name:    "id",
				Unique:  true,
				Indexer: &memdb.StringFieldIndex{Field: "Key"},
			},
		},
	}
}
