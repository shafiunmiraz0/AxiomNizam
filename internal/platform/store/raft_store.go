// Package store — raft_store.go
//
// Phase 4 of the etcd replacement plan: RaftStore[T] — the unified
// ResourceStore[T] implementation that reads from go-memdb and writes
// through Raft consensus.
//
// Behaviour:
//   - Get / List  → read directly from go-memdb (fast, local, no Raft
//     round-trip).  Consistent on the leader because the FSM is always
//     up-to-date.
//   - Create / Update / Delete → encode as a Raft Command, submit via
//     raft.Apply, wait for commit.  The FSM applies the mutation to
//     go-memdb, then we emit the watch event locally.
//   - Watch → same fan-out channel pattern as EtcdStore / MemDBStore.
//
// This store requires a running Raft server (internal/platform/raft).
// For single-node development the server auto-bootstraps.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/errs"
	axraft "axiomnizam.bitbd.net/axiomnizam/internal/platform/raft"

	"github.com/hashicorp/go-memdb"
)

// RaftStore is a ResourceStore[T] that reads from go-memdb and writes
// through Raft consensus.  One instance per resource Kind.
type RaftStore[T Resource] struct {
	raftServer *axraft.Server
	db         *memdb.MemDB
	table      string
	factory    func() T

	// applyTimeout is how long to wait for a Raft log entry to commit.
	applyTimeout time.Duration

	mu       sync.Mutex
	watchers []chan WatchEvent[T]
	closed   bool
}

// RaftStoreConfig holds options for constructing a RaftStore.
type RaftStoreConfig struct {
	// ApplyTimeout is the maximum time to wait for a Raft write to
	// commit.  Defaults to 5 seconds.
	ApplyTimeout time.Duration
}

// NewRaftStore constructs a RaftStore.  The `raftServer` must be
// started and the go-memdb schema must include `tableName`.
func NewRaftStore[T Resource](
	raftServer *axraft.Server,
	tableName string,
	factory func() T,
	opts *RaftStoreConfig,
) *RaftStore[T] {
	timeout := 5 * time.Second
	if opts != nil && opts.ApplyTimeout > 0 {
		timeout = opts.ApplyTimeout
	}
	return &RaftStore[T]{
		raftServer:   raftServer,
		db:           raftServer.DB(),
		table:        tableName,
		factory:      factory,
		applyTimeout: timeout,
	}
}

// Get reads directly from go-memdb (no Raft round-trip).
func (s *RaftStore[T]) Get(ctx context.Context, key string) (T, error) {
	var zero T

	txn := s.db.Txn(false)
	defer txn.Abort()

	raw, err := txn.First(s.table, "id", key)
	if err != nil {
		return zero, fmt.Errorf("RaftStore.Get: %w", err)
	}
	if raw == nil {
		return zero, ErrNotFound
	}

	entry := raw.(*memdbEntry)
	obj := s.factory()
	if err := json.Unmarshal(entry.Data, obj); err != nil {
		return zero, fmt.Errorf("RaftStore.Get: decode: %w", err)
	}
	return obj, nil
}

// List reads directly from go-memdb.
func (s *RaftStore[T]) List(ctx context.Context, namespace string) ([]T, error) {
	txn := s.db.Txn(false)
	defer txn.Abort()

	var it memdb.ResultIterator
	var err error

	if namespace != "" {
		it, err = txn.Get(s.table, "namespace", namespace)
	} else {
		it, err = txn.Get(s.table, "id")
	}
	if err != nil {
		return nil, fmt.Errorf("RaftStore.List: %w", err)
	}

	var out []T
	for raw := it.Next(); raw != nil; raw = it.Next() {
		entry := raw.(*memdbEntry)
		obj := s.factory()
		if err := json.Unmarshal(entry.Data, obj); err != nil {
			return nil, fmt.Errorf("RaftStore.List: decode: %w", err)
		}
		out = append(out, obj)
	}
	return out, nil
}

// Create submits a Create command through Raft.  Returns ErrConflict
// if the key already exists (enforced by the FSM).
func (s *RaftStore[T]) Create(ctx context.Context, obj T) error {
	key := obj.GetKey()

	// Stamp metadata — same as EtcdStore.
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
		return fmt.Errorf("RaftStore.Create: encode: %w", err)
	}

	cmd := &axraft.Command{
		Type:      axraft.CommandCreate,
		Table:     s.table,
		Key:       key,
		Namespace: raftExtractNamespace(key),
		Data:      data,
	}
	encoded, err := axraft.EncodeCommand(cmd)
	if err != nil {
		return fmt.Errorf("RaftStore.Create: %w", err)
	}

	if err := s.raftServer.Apply(encoded, s.applyTimeout); err != nil {
		// Map FSM conflict errors to our sentinel.
		if strings.Contains(err.Error(), "conflict") {
			return ErrConflict
		}
		return fmt.Errorf("RaftStore.Create: %w", err)
	}

	s.emit(WatchEvent[T]{Type: WatchAdded, Object: obj})
	return nil
}

// Update submits an Update command through Raft.
func (s *RaftStore[T]) Update(ctx context.Context, obj T) error {
	key := obj.GetKey()

	meta := obj.GetObjectMeta()
	if meta != nil {
		meta.UpdatedAt = time.Now().UTC()
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("RaftStore.Update: encode: %w", err)
	}

	cmd := &axraft.Command{
		Type:      axraft.CommandUpdate,
		Table:     s.table,
		Key:       key,
		Namespace: raftExtractNamespace(key),
		Data:      data,
	}
	encoded, err := axraft.EncodeCommand(cmd)
	if err != nil {
		return fmt.Errorf("RaftStore.Update: %w", err)
	}

	if err := s.raftServer.Apply(encoded, s.applyTimeout); err != nil {
		return fmt.Errorf("RaftStore.Update: %w", err)
	}

	s.emit(WatchEvent[T]{Type: WatchModified, Object: obj})
	return nil
}

// Delete submits a Delete command through Raft.  Missing keys are a
// no-op.
func (s *RaftStore[T]) Delete(ctx context.Context, key string) error {
	// Read the object first for the watch event.
	prior, getErr := s.Get(ctx, key)
	if getErr != nil && !errs.Is(getErr, ErrNotFound) {
		return getErr
	}

	cmd := &axraft.Command{
		Type:      axraft.CommandDelete,
		Table:     s.table,
		Key:       key,
		Namespace: raftExtractNamespace(key),
	}
	encoded, err := axraft.EncodeCommand(cmd)
	if err != nil {
		return fmt.Errorf("RaftStore.Delete: %w", err)
	}

	if err := s.raftServer.Apply(encoded, s.applyTimeout); err != nil {
		return fmt.Errorf("RaftStore.Delete: %w", err)
	}

	if getErr == nil {
		s.emit(WatchEvent[T]{Type: WatchDeleted, Object: prior})
	}
	return nil
}

// Watch returns a channel that receives lifecycle events.
func (s *RaftStore[T]) Watch(ctx context.Context) (<-chan WatchEvent[T], error) {
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
func (s *RaftStore[T]) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for _, w := range s.watchers {
		close(w)
	}
	s.watchers = nil
	return nil
}

func (s *RaftStore[T]) emit(e WatchEvent[T]) {
	s.mu.Lock()
	watchers := append([]chan WatchEvent[T](nil), s.watchers...)
	s.mu.Unlock()
	for _, w := range watchers {
		select {
		case w <- e:
		default:
		}
	}
}

// raftExtractNamespace splits a "namespace/name" key.
func raftExtractNamespace(key string) string {
	if idx := strings.Index(key, "/"); idx >= 0 {
		return key[:idx]
	}
	return ""
}
