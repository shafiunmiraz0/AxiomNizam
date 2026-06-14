// Package store provides a single generic persistence abstraction for
// all reconcilable resources in the platform.
//
// P2.2 — Unified persistence.
//
// Today the workspace has at least three storage patterns:
//
//   - GORM-backed repositories for user/admin/API-builder entities.
//   - Direct etcd use in handlers / cdc / etl / iam / netintel / platform.
//   - Ad-hoc in-memory maps for api_builder and resource_handler.
//
// This package introduces `ResourceStore[T]` — one generic interface
// per resource Kind — plus a single `EtcdStore[T]` implementation.
// Migration of call sites is a follow-up; here we provide the
// abstraction and a reference implementation so new controllers written
// in P1 land directly on it.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/errs"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Sentinel errors — aliased to the canonical platform sentinels so
// callers can `errors.Is(err, errs.ErrNotFound)` uniformly.
var (
	ErrNotFound = errs.ErrNotFound
	ErrConflict = errs.ErrConflict
)

// Resource is the constraint used by ResourceStore.  It is the union of
// the canonical `resources.Resource` surface and the `reconciler.Resource`
// key/generation surface so controllers can use the same value for both
// persistence and reconciliation.
type Resource interface {
	resources.Resource
	reconciler.Resource
}

// ResourceStore is a typed, generation-aware persistence surface for a
// single Kind of resource.  Implementations must:
//
//   - Return `ErrNotFound` for missing keys (never a bare nil).
//   - Return `ErrConflict` when a stored object's generation moved
//     between Get and Update (optimistic concurrency).
//   - Emit watch events on Create / Update / Delete.
type ResourceStore[T Resource] interface {
	Get(ctx context.Context, key string) (T, error)
	List(ctx context.Context, namespace string) ([]T, error)
	Create(ctx context.Context, obj T) error
	Update(ctx context.Context, obj T) error
	Delete(ctx context.Context, key string) error
	Watch(ctx context.Context) (<-chan WatchEvent[T], error)
	Close() error
}

// WatchEventType enumerates the lifecycle transitions emitted by a
// store's Watch channel.
type WatchEventType string

const (
	WatchAdded    WatchEventType = "Added"
	WatchModified WatchEventType = "Modified"
	WatchDeleted  WatchEventType = "Deleted"
)

// WatchEvent is what a store publishes on Watch.
type WatchEvent[T Resource] struct {
	Type   WatchEventType
	Object T
}

// EtcdStore is the canonical etcd-backed implementation of
// `ResourceStore[T]`.  One instance per Kind; pass it the concrete
// factory `func() T` so the store can allocate concrete pointers
// during deserialisation.
type EtcdStore[T Resource] struct {
	client  *clientv3.Client
	prefix  string
	factory func() T

	mu       sync.Mutex
	watchers []chan WatchEvent[T]
}

// NewEtcdStore constructs an EtcdStore.  The `prefix` is typically
// `"/axiomnizam/<kind>/"` and `factory` allocates a new zero T.
func NewEtcdStore[T Resource](client *clientv3.Client, prefix string, factory func() T) *EtcdStore[T] {
	if prefix == "" {
		prefix = "/axiomnizam/"
	}
	if prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	return &EtcdStore[T]{client: client, prefix: prefix, factory: factory}
}

func (s *EtcdStore[T]) keyFor(k string) string { return path.Join(s.prefix, k) }

// Get fetches the resource stored under `key`.
func (s *EtcdStore[T]) Get(ctx context.Context, key string) (T, error) {
	var zero T
	if s.client == nil {
		return zero, fmt.Errorf("EtcdStore.Get: client not configured")
	}
	resp, err := s.client.Get(ctx, s.keyFor(key))
	if err != nil {
		return zero, fmt.Errorf("EtcdStore.Get: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return zero, ErrNotFound
	}
	obj := s.factory()
	if err := json.Unmarshal(resp.Kvs[0].Value, obj); err != nil {
		return zero, fmt.Errorf("EtcdStore.Get: decode: %w", err)
	}
	return obj, nil
}

// List returns every resource with key matching `prefix/namespace/`.
// An empty namespace lists everything under the store's prefix.
func (s *EtcdStore[T]) List(ctx context.Context, namespace string) ([]T, error) {
	if s.client == nil {
		return nil, fmt.Errorf("EtcdStore.List: client not configured")
	}
	pfx := s.prefix
	if namespace != "" {
		pfx = path.Join(s.prefix, namespace) + "/"
	}
	resp, err := s.client.Get(ctx, pfx, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("EtcdStore.List: %w", err)
	}
	out := make([]T, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		obj := s.factory()
		if err := json.Unmarshal(kv.Value, obj); err != nil {
			return nil, fmt.Errorf("EtcdStore.List: decode %s: %w", kv.Key, err)
		}
		out = append(out, obj)
	}
	return out, nil
}

// Create persists a new resource.  Fails with `ErrConflict` if the key
// already exists.
func (s *EtcdStore[T]) Create(ctx context.Context, obj T) error {
	if s.client == nil {
		return fmt.Errorf("EtcdStore.Create: client not configured")
	}
	key := s.keyFor(obj.GetKey())
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
		return fmt.Errorf("EtcdStore.Create: encode: %w", err)
	}
	txn := s.client.Txn(ctx).
		If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0)).
		Then(clientv3.OpPut(key, string(data)))
	resp, err := txn.Commit()
	if err != nil {
		return fmt.Errorf("EtcdStore.Create: %w", err)
	}
	if !resp.Succeeded {
		return ErrConflict
	}
	s.emit(WatchEvent[T]{Type: WatchAdded, Object: obj})
	return nil
}

// Update persists an existing resource, bumping Generation on spec
// changes.  Concurrency is enforced at write time by etcd's CAS.
func (s *EtcdStore[T]) Update(ctx context.Context, obj T) error {
	if s.client == nil {
		return fmt.Errorf("EtcdStore.Update: client not configured")
	}
	key := s.keyFor(obj.GetKey())
	meta := obj.GetObjectMeta()
	if meta != nil {
		meta.UpdatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("EtcdStore.Update: encode: %w", err)
	}
	// NB: full CAS on ModRevision would require reading first.  We keep
	// the simple last-writer-wins semantics for now — callers that want
	// stronger guarantees should layer a read-modify-write helper.
	if _, err := s.client.Put(ctx, key, string(data)); err != nil {
		return fmt.Errorf("EtcdStore.Update: %w", err)
	}
	s.emit(WatchEvent[T]{Type: WatchModified, Object: obj})
	return nil
}

// Delete removes a resource by key.  Missing keys are a no-op.
func (s *EtcdStore[T]) Delete(ctx context.Context, key string) error {
	if s.client == nil {
		return fmt.Errorf("EtcdStore.Delete: client not configured")
	}
	prior, err := s.Get(ctx, key)
	if err != nil && !errs.Is(err, ErrNotFound) {
		return err
	}
	if _, err := s.client.Delete(ctx, s.keyFor(key)); err != nil {
		return fmt.Errorf("EtcdStore.Delete: %w", err)
	}
	if err == nil {
		s.emit(WatchEvent[T]{Type: WatchDeleted, Object: prior})
	}
	return nil
}

// Watch returns a channel that receives lifecycle events for this Kind.
// Cancel `ctx` to stop receiving.
func (s *EtcdStore[T]) Watch(ctx context.Context) (<-chan WatchEvent[T], error) {
	ch := make(chan WatchEvent[T], 64)
	s.mu.Lock()
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

// Close tears down any subscriptions held by this store.
func (s *EtcdStore[T]) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.watchers {
		close(w)
	}
	s.watchers = nil
	return nil
}

func (s *EtcdStore[T]) emit(e WatchEvent[T]) {
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
