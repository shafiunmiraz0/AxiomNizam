// Package store — memdb_store.go
//
// Phase 1 of the etcd replacement plan: MemDBStore[T] — an in-memory
// implementation of ResourceStore[T] backed by hashicorp/go-memdb.
//
// This is a drop-in replacement for EtcdStore[T].  It satisfies the
// exact same interface so all 27+ reconcilers can use it without any
// code changes.
//
// Behaviour parity with EtcdStore[T]:
//   - Get returns ErrNotFound for missing keys.
//   - Create returns ErrConflict if the key already exists.
//   - Update sets UpdatedAt on the object metadata.
//   - Delete is a no-op for missing keys (emits event only if found).
//   - Watch returns a buffered channel; slow consumers drop events.
//   - Close tears down all watch subscriptions.
//
// In Phase 4 this store will be wrapped by RaftStore[T] which routes
// writes through Raft consensus and reads from the local go-memdb
// snapshot.  For now it operates standalone (single-node, in-process).
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/errs"

	"github.com/hashicorp/go-memdb"
)

// MemDBStore is an in-memory implementation of ResourceStore[T] using
// hashicorp/go-memdb.  One instance per resource Kind.
type MemDBStore[T Resource] struct {
	db      *memdb.MemDB
	table   string
	factory func() T

	mu       sync.Mutex
	watchers []chan WatchEvent[T]
	closed   bool
}

// NewMemDBStore constructs a MemDBStore.  The `tableName` identifies
// the go-memdb table (typically the resource kind, e.g.
// "catalog_assets").  `factory` allocates a new zero-value T for
// deserialisation.
func NewMemDBStore[T Resource](tableName string, factory func() T) (*MemDBStore[T], error) {
	if tableName == "" {
		tableName = "resources"
	}
	schema := newMemDBSchema(tableName)
	db, err := memdb.NewMemDB(schema)
	if err != nil {
		return nil, fmt.Errorf("MemDBStore: create memdb: %w", err)
	}
	return &MemDBStore[T]{
		db:      db,
		table:   tableName,
		factory: factory,
	}, nil
}

// Get fetches the resource stored under `key`.
// Returns ErrNotFound if the key does not exist.
func (s *MemDBStore[T]) Get(ctx context.Context, key string) (T, error) {
	var zero T

	txn := s.db.Txn(false) // read-only transaction
	defer txn.Abort()

	raw, err := txn.First(s.table, "id", key)
	if err != nil {
		return zero, fmt.Errorf("MemDBStore.Get: %w", err)
	}
	if raw == nil {
		return zero, ErrNotFound
	}

	entry := raw.(*memdbEntry)
	obj := s.factory()
	if err := json.Unmarshal(entry.Data, obj); err != nil {
		return zero, fmt.Errorf("MemDBStore.Get: decode: %w", err)
	}
	return obj, nil
}

// List returns every resource in the given namespace.  An empty
// namespace returns all resources in the table.
func (s *MemDBStore[T]) List(ctx context.Context, namespace string) ([]T, error) {
	txn := s.db.Txn(false)
	defer txn.Abort()

	var it memdb.ResultIterator
	var err error

	if namespace != "" {
		// Use the namespace secondary index.
		it, err = txn.Get(s.table, "namespace", namespace)
	} else {
		// List everything — iterate the primary index.
		it, err = txn.Get(s.table, "id")
	}
	if err != nil {
		return nil, fmt.Errorf("MemDBStore.List: %w", err)
	}

	var out []T
	for raw := it.Next(); raw != nil; raw = it.Next() {
		entry := raw.(*memdbEntry)
		obj := s.factory()
		if err := json.Unmarshal(entry.Data, obj); err != nil {
			return nil, fmt.Errorf("MemDBStore.List: decode: %w", err)
		}
		out = append(out, obj)
	}
	return out, nil
}

// Create persists a new resource.  Returns ErrConflict if the key
// already exists (same semantics as EtcdStore's CAS transaction).
func (s *MemDBStore[T]) Create(ctx context.Context, obj T) error {
	key := obj.GetKey()

	// Stamp metadata — mirrors EtcdStore behaviour.
	meta := obj.GetObjectMeta()
	if meta != nil {
		if meta.CreatedAt.IsZero() {
			meta.CreatedAt = time.Now().UTC()
		}
		meta.UpdatedAt = time.Now().UTC()
		if meta.Generation == 0 {
			meta.Generation = 1
		}
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("MemDBStore.Create: encode: %w", err)
	}

	txn := s.db.Txn(true) // write transaction

	// Check for existing key (CAS: create-if-absent).
	existing, err := txn.First(s.table, "id", key)
	if err != nil {
		txn.Abort()
		return fmt.Errorf("MemDBStore.Create: lookup: %w", err)
	}
	if existing != nil {
		txn.Abort()
		return ErrConflict
	}

	entry := &memdbEntry{
		Key:       key,
		Namespace: extractNamespace(key),
		Data:      data,
	}
	if err := txn.Insert(s.table, entry); err != nil {
		txn.Abort()
		return fmt.Errorf("MemDBStore.Create: insert: %w", err)
	}
	txn.Commit()

	s.emit(WatchEvent[T]{Type: WatchAdded, Object: obj})
	return nil
}

// Update persists an existing resource.  Stamps UpdatedAt on the
// object metadata.  Uses last-writer-wins semantics (same as
// EtcdStore).
func (s *MemDBStore[T]) Update(ctx context.Context, obj T) error {
	key := obj.GetKey()

	meta := obj.GetObjectMeta()
	if meta != nil {
		meta.UpdatedAt = time.Now().UTC()
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("MemDBStore.Update: encode: %w", err)
	}

	entry := &memdbEntry{
		Key:       key,
		Namespace: extractNamespace(key),
		Data:      data,
	}

	txn := s.db.Txn(true)
	if err := txn.Insert(s.table, entry); err != nil {
		txn.Abort()
		return fmt.Errorf("MemDBStore.Update: insert: %w", err)
	}
	txn.Commit()

	s.emit(WatchEvent[T]{Type: WatchModified, Object: obj})
	return nil
}

// Delete removes a resource by key.  Missing keys are a no-op (same
// as EtcdStore).  Emits a WatchDeleted event only if the key existed.
func (s *MemDBStore[T]) Delete(ctx context.Context, key string) error {
	// Read the object first so we can emit it in the watch event.
	prior, getErr := s.Get(ctx, key)
	if getErr != nil && !errs.Is(getErr, ErrNotFound) {
		return getErr
	}

	txn := s.db.Txn(true)

	existing, err := txn.First(s.table, "id", key)
	if err != nil {
		txn.Abort()
		return fmt.Errorf("MemDBStore.Delete: lookup: %w", err)
	}
	if existing == nil {
		txn.Abort()
		return nil // no-op, same as EtcdStore
	}

	if err := txn.Delete(s.table, existing); err != nil {
		txn.Abort()
		return fmt.Errorf("MemDBStore.Delete: %w", err)
	}
	txn.Commit()

	if getErr == nil {
		s.emit(WatchEvent[T]{Type: WatchDeleted, Object: prior})
	}
	return nil
}

// Watch returns a channel that receives lifecycle events for this
// Kind.  Cancel the context to unsubscribe and close the channel.
func (s *MemDBStore[T]) Watch(ctx context.Context) (<-chan WatchEvent[T], error) {
	ch := make(chan WatchEvent[T], 64)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		close(ch)
		return ch, nil
	}
	s.watchers = append(s.watchers, ch)
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		for i, w := range s.watchers {
			if w == ch {
				s.watchers = append(s.watchers[:i], s.watchers[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		close(ch)
	}()

	return ch, nil
}

// Close tears down all watch subscriptions.
func (s *MemDBStore[T]) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for _, w := range s.watchers {
		close(w)
	}
	s.watchers = nil
	return nil
}

// emit fans out a watch event to all active subscribers.  Drops the
// event for any subscriber whose buffer is full (non-blocking).
func (s *MemDBStore[T]) emit(e WatchEvent[T]) {
	s.mu.Lock()
	watchers := append([]chan WatchEvent[T](nil), s.watchers...)
	s.mu.Unlock()
	for _, w := range watchers {
		select {
		case w <- e:
		default:
			// Drop on full buffer rather than block writers.
		}
	}
}

// extractNamespace splits a "namespace/name" key and returns the
// namespace portion.  If there is no slash, returns empty string.
func extractNamespace(key string) string {
	if idx := strings.Index(key, "/"); idx >= 0 {
		return key[:idx]
	}
	return ""
}
