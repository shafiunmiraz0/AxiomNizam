package conformance

// Kind → Store → WorkQueue → Reconciler integration test.
//
// This test exercises the canonical control-plane path end-to-end using
// an APIBank resource:
//
//   1. Put a resource into the store (simulating a Create via API).
//   2. Drop its key onto the workqueue (as an informer event handler
//      would).
//   3. Run a minimal controller loop that pops keys, fetches from the
//      store, and invokes the reconciler.
//   4. Assert the reconciler observed the generation and wrote status
//      back through the store.
//
// The goal is to catch regressions in the wiring contracts between
// these packages, not to exercise any one of them in depth.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apibanks"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
	"axiomnizam.bitbd.net/axiomnizam/internal/workqueue"
)

// memStore is a minimal in-memory ResourceStore[T] used for the
// integration test.  It deliberately avoids the etcd implementation so
// the test is hermetic.
type memStore[T store.Resource] struct {
	mu      sync.RWMutex
	items   map[string]T
	watches []chan store.WatchEvent[T]
}

func newMemStore[T store.Resource]() *memStore[T] {
	return &memStore[T]{items: make(map[string]T)}
}

func (s *memStore[T]) Get(_ context.Context, key string) (T, error) {
	var zero T
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[key]
	if !ok {
		return zero, store.ErrNotFound
	}
	return v, nil
}

func (s *memStore[T]) List(_ context.Context, namespace string) ([]T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]T, 0, len(s.items))
	for _, v := range s.items {
		if namespace != "" && v.GetObjectMeta().Namespace != namespace {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *memStore[T]) Create(_ context.Context, obj T) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[obj.GetKey()] = obj
	return nil
}

func (s *memStore[T]) Update(_ context.Context, obj T) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[obj.GetKey()] = obj
	return nil
}

func (s *memStore[T]) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	return nil
}

func (s *memStore[T]) Watch(ctx context.Context) (<-chan store.WatchEvent[T], error) {
	ch := make(chan store.WatchEvent[T], 16)
	s.mu.Lock()
	s.watches = append(s.watches, ch)
	s.mu.Unlock()
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func (s *memStore[T]) Close() error { return nil }

func TestIntegration_KindStoreWorkQueueReconciler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 1. Store: create a new APIBank resource.
	st := newMemStore[*apibanks.APIBankResource]()
	bank := &apibanks.APIBankResource{
		TypeMeta: resources.TypeMeta{
			Kind:       apibanks.APIBankKind,
			APIVersion: apibanks.APIBankAPIVersion,
		},
		ObjectMeta: resources.ObjectMeta{
			Name:       "payments",
			Namespace:  "default",
			UID:        "uid-payments-1",
			Generation: 1,
		},
		Spec: apibanks.APIBankSpec{
			Description: "payments APIs",
			Owner:       "platform",
			APIs: []apibanks.APIReference{
				{Name: "charge", Kind: "REST", Endpoint: "/charge"},
				{Name: "refund", Kind: "REST", Endpoint: "/refund"},
			},
		},
	}
	if err := st.Create(ctx, bank); err != nil {
		t.Fatalf("store.Create: %v", err)
	}

	// 2. WorkQueue: enqueue the resource key (as an informer would).
	q := workqueue.NewSimpleQueue(nil)
	defer q.Shutdown()
	if err := q.Add(bank.GetKey()); err != nil {
		t.Fatalf("queue.Add: %v", err)
	}

	// 3. Reconciler: build and run one iteration of the worker loop.
	mgr := apibanks.NewAPIBankManager()
	rec := apibanks.NewAPIBankReconciler(st, mgr)

	item, err := q.Get()
	if err != nil {
		t.Fatalf("queue.Get: %v", err)
	}
	fetched, err := st.Get(ctx, item.Key)
	if err != nil {
		t.Fatalf("store.Get after dequeue: %v", err)
	}
	result := rec.Reconcile(ctx, reconciler.Resource(fetched))
	if result.Error != nil {
		t.Fatalf("reconcile error: %v", result.Error)
	}
	if err := q.Done(item.Key); err != nil {
		t.Fatalf("queue.Done: %v", err)
	}

	// 4. Assertions: status was written back through the store and
	//    reflects the spec generation.
	after, err := st.Get(ctx, bank.GetKey())
	if err != nil {
		t.Fatalf("store.Get after reconcile: %v", err)
	}
	if after.Status.ObservedGeneration != 1 {
		t.Errorf("ObservedGeneration = %d, want 1", after.Status.ObservedGeneration)
	}
	if after.Status.APICount != 2 {
		t.Errorf("APICount = %d, want 2", after.Status.APICount)
	}
	if after.Status.Phase != "Ready" {
		t.Errorf("Phase = %q, want Ready", after.Status.Phase)
	}
	if after.Status.LastSyncedAt == nil {
		t.Error("LastSyncedAt not recorded")
	}

	// 5. Bank was created in the manager as a side effect.
	if got := mgr.GetBank("payments"); got == nil {
		t.Error("APIBankManager does not hold the reconciled bank")
	}

	// Sentinel round-trip: ErrNotFound is shared across the platform.
	if _, err := st.Get(ctx, "missing/key"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
