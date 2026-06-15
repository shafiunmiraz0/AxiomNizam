// Package handler — EnqueueRequestForOwner walks the event object's
// OwnerReferences and enqueues each owner whose Kind matches the
// configured OwnerType.  Used for "reconcile the parent when a child
// changes" semantics — the single most common cross-resource pattern
// in k8s controllers.
package handler

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/apimachinery/meta"
	"axiomnizam.bitbd.net/axiomnizam/internal/controller/predicate"
	"axiomnizam.bitbd.net/axiomnizam/internal/controller/reconcile"
)

// EnqueueRequestForOwner enqueues each OwnerReference of the event
// object whose APIVersion/Kind matches the configured pair.  When
// IsController is true, only the single reference with
// controller=true is enqueued; false enqueues all matching refs.
type EnqueueRequestForOwner struct {
	// OwnerAPIVersion matches OwnerReference.APIVersion.  Empty
	// string matches any APIVersion.
	OwnerAPIVersion string
	// OwnerKind matches OwnerReference.Kind.  Empty string matches
	// any kind — rarely useful but supported.
	OwnerKind string
	// IsController restricts enqueue to the controlling owner only.
	IsController bool
}

// OnCreate delegates.
func (e EnqueueRequestForOwner) OnCreate(obj predicate.Object, q Queue) { e.enqueue(obj, q) }

// OnUpdate enqueues the new object's owners — covers the common
// case where a child is moved under a different parent.
func (e EnqueueRequestForOwner) OnUpdate(_, newObj predicate.Object, q Queue) {
	e.enqueue(newObj, q)
}

// OnDelete enqueues the disappearing child's owners so the parent
// gets a chance to recreate or tidy up.
func (e EnqueueRequestForOwner) OnDelete(obj predicate.Object, q Queue) { e.enqueue(obj, q) }

// OnGeneric delegates.
func (e EnqueueRequestForOwner) OnGeneric(obj predicate.Object, q Queue) { e.enqueue(obj, q) }

// enqueue is the shared walker.
func (e EnqueueRequestForOwner) enqueue(obj predicate.Object, q Queue) {
	m, ok := obj.(meta.Object)
	if !ok || m == nil {
		return
	}
	refs := m.GetOwnerReferences()
	for _, ref := range refs {
		if e.IsController && (ref.Controller == nil || !*ref.Controller) {
			continue
		}
		if e.OwnerAPIVersion != "" && ref.APIVersion != e.OwnerAPIVersion {
			continue
		}
		if e.OwnerKind != "" && ref.Kind != e.OwnerKind {
			continue
		}
		// OwnerReferences are namespace-local in k8s — a child in
		// namespace N may only be owned by objects in namespace N.
		// The meta.Object returns the child's namespace, which is
		// also the owner's namespace.
		q.Add(reconcile.Request{
			Namespace: m.GetNamespace(),
			Name:      ref.Name,
		})
	}
}
