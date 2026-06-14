package gc

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// StoreAdapter wraps a generic ResourceStore[T] into the ResourceLister
// interface that the GarbageCollector needs.
type StoreAdapter[T store.Resource] struct {
	store   store.ResourceStore[T]
	factory func() T
}

// NewStoreAdapter creates an adapter for the given store.
func NewStoreAdapter[T store.Resource](s store.ResourceStore[T], factory func() T) *StoreAdapter[T] {
	return &StoreAdapter[T]{store: s, factory: factory}
}

// ListPendingDeletion returns all resources where DeletedAt is set.
func (a *StoreAdapter[T]) ListPendingDeletion(ctx context.Context) ([]resources.Resource, error) {
	all, err := a.store.List(ctx, "")
	if err != nil {
		return nil, err
	}
	var out []resources.Resource
	for _, obj := range all {
		meta := obj.GetObjectMeta()
		if meta != nil && meta.DeletedAt != nil {
			out = append(out, obj)
		}
	}
	return out, nil
}

// Delete hard-removes a resource by key.
func (a *StoreAdapter[T]) Delete(ctx context.Context, key string) error {
	return a.store.Delete(ctx, key)
}

// ListByOwnerUID returns all resources whose OwnerReferences contain the
// given UID.
func (a *StoreAdapter[T]) ListByOwnerUID(ctx context.Context, ownerUID string) ([]resources.Resource, error) {
	all, err := a.store.List(ctx, "")
	if err != nil {
		return nil, err
	}
	var out []resources.Resource
	for _, obj := range all {
		meta := obj.GetObjectMeta()
		if meta == nil {
			continue
		}
		for _, ref := range meta.OwnerReferences {
			if ref.UID == ownerUID {
				out = append(out, obj)
				break
			}
		}
	}
	return out, nil
}

// SetDeletedAt marks a resource for soft-deletion by setting DeletedAt.
func (a *StoreAdapter[T]) SetDeletedAt(ctx context.Context, key string) error {
	obj, err := a.store.Get(ctx, key)
	if err != nil {
		return err
	}
	meta := obj.GetObjectMeta()
	if meta == nil {
		return nil
	}
	now := time.Now().UTC()
	meta.DeletedAt = &now
	return a.store.Update(ctx, obj)
}
