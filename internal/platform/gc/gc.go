// Package gc implements a garbage collector that enforces owner-reference
// cascading deletion and finalizer gating for platform resources.
//
// Design inspired by K8s garbage collector:
//   - Resources with DeletedAt set and no Finalizers are hard-deleted.
//   - After deletion, children whose OwnerReferences match the deleted
//     resource's UID have their DeletedAt set (soft-delete cascade).
//   - Resources with pending Finalizers are skipped until a reconciler
//     clears them.
package gc

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const gcComponent = "gc"

// keyGetter is implemented by store.Resource and provides GetKey().
type keyGetter interface {
	GetKey() string
}

// resourceKey safely extracts the key from a resource.
// Falls back to UID if GetKey() is not available.
func resourceKey(res resources.Resource) string {
	if kg, ok := res.(keyGetter); ok {
		return kg.GetKey()
	}
	meta := res.GetObjectMeta()
	if meta != nil {
		return meta.UID
	}
	return ""
}

// ResourceLister is the interface the GC uses to interact with a resource
// store.  The generic StoreAdapter[T] satisfies it for any ResourceStore[T].
type ResourceLister interface {
	// ListPendingDeletion returns resources whose DeletedAt is set.
	ListPendingDeletion(ctx context.Context) ([]resources.Resource, error)

	// Delete hard-removes a resource by key.
	Delete(ctx context.Context, key string) error

	// ListByOwnerUID returns all resources that reference the given UID
	// in their OwnerReferences.
	ListByOwnerUID(ctx context.Context, ownerUID string) ([]resources.Resource, error)

	// SetDeletedAt marks a resource for deletion by setting DeletedAt.
	SetDeletedAt(ctx context.Context, key string) error
}

// registeredStore pairs a name with its adapter.
type registeredStore struct {
	name    string
	lister  ResourceLister
}

// GarbageCollector periodically scans all registered stores and:
//   - hard-deletes resources with DeletedAt set and no Finalizers,
//   - cascades soft-delete to children via OwnerReferences.
type GarbageCollector struct {
	interval time.Duration
	stores   []registeredStore
}

// NewGarbageCollector creates a GC that runs every interval.
func NewGarbageCollector(interval time.Duration) *GarbageCollector {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &GarbageCollector{interval: interval}
}

// Register adds a store adapter to the GC.  Call before Start.
func (gc *GarbageCollector) Register(name string, lister ResourceLister) {
	gc.stores = append(gc.stores, registeredStore{name: name, lister: lister})
}

// Start runs the GC loop until ctx is cancelled.
func (gc *GarbageCollector) Start(ctx context.Context) {
	logging.Z().Info(fmt.Sprintf("%s: starting (interval=%s, stores=%d)", gcComponent, gc.interval, len(gc.stores)))
	ticker := time.NewTicker(gc.interval)
	defer ticker.Stop()

	// Run once immediately on startup.
	gc.run(ctx)

	for {
		select {
		case <-ctx.Done():
			logging.Z().Info(fmt.Sprintf("%s: stopped", gcComponent))
			return
		case <-ticker.C:
			gc.run(ctx)
		}
	}
}

// run executes a single GC pass across all registered stores.
func (gc *GarbageCollector) run(ctx context.Context) {
	start := time.Now()
	var totalScanned, totalDeleted, totalSkipped, totalCascaded int

	for _, rs := range gc.stores {
		deleted, skipped, cascaded := gc.processStore(ctx, rs)
		totalDeleted += deleted
		totalSkipped += skipped
		totalCascaded += cascaded
		totalScanned += deleted + skipped
	}

	duration := time.Since(start)
	GCPassDurationSeconds.Observe(duration.Seconds())
	GCPassesTotal.Inc()
	ResourcesDeletedTotal.Add(float64(totalDeleted))
	ResourcesSkippedTotal.Add(float64(totalSkipped))
	ResourcesCascadedTotal.Add(float64(totalCascaded))

	if totalDeleted > 0 || totalCascaded > 0 {
		logging.Z().Info(fmt.Sprintf("%s: pass complete — scanned=%d deleted=%d skipped=%d cascaded=%d duration=%s",
			gcComponent, totalScanned, totalDeleted, totalSkipped, totalCascaded, duration))
	}
}

// processStore handles one registered store.  Returns counts.
func (gc *GarbageCollector) processStore(ctx context.Context, rs registeredStore) (deleted, skipped, cascaded int) {
	pending, err := rs.lister.ListPendingDeletion(ctx)
	if err != nil {
		logging.Z().Warn(fmt.Sprintf("%s: %s list failed: %v", gcComponent, rs.name, err))
		return
	}

	for _, res := range pending {
		meta := res.GetObjectMeta()
		if meta == nil {
			continue
		}

		// Finalizer gate — skip if any finalizers remain.
		if len(meta.Finalizers) > 0 {
			skipped++
			continue
		}

		uid := meta.UID
		key := resourceKey(res)

		// Hard-delete the resource.
		if err := rs.lister.Delete(ctx, key); err != nil {
			logging.Z().Warn(fmt.Sprintf("%s: %s delete %q failed: %v", gcComponent, rs.name, key, err))
			continue
		}
		deleted++

		// Cascade: mark children for deletion.
		cascaded += gc.cascadeDeletion(ctx, uid)
	}
	return
}

// cascadeDeletion marks all resources across all stores that reference
// the given owner UID for soft-deletion.
func (gc *GarbageCollector) cascadeDeletion(ctx context.Context, ownerUID string) int {
	var count int
	for _, rs := range gc.stores {
		children, err := rs.lister.ListByOwnerUID(ctx, ownerUID)
		if err != nil {
			logging.Z().Warn(fmt.Sprintf("%s: %s list-by-owner failed: %v", gcComponent, rs.name, err))
			continue
		}
		for _, child := range children {
			childMeta := child.GetObjectMeta()
			if childMeta == nil || childMeta.DeletedAt != nil {
				continue // already marked
			}
			childKey := resourceKey(child)
			if err := rs.lister.SetDeletedAt(ctx, childKey); err != nil {
				logging.Z().Warn(fmt.Sprintf("%s: cascade %s.%s failed: %v", gcComponent, rs.name, childKey, err))
				continue
			}
			count++
		}
	}
	return count
}
