// Package handler maps an informer event onto one or more
// reconcile.Request values enqueued on the workqueue.  The three
// canonical handlers cover 99% of controllers:
//
//   - EnqueueRequestForObject: Request == the event's own object.
//     Used for the primary resource a controller owns.
//   - EnqueueRequestForOwner: Request == each OwnerReference on the
//     event's object.  Used for watched children that should trigger
//     their parent's reconciler.
//   - EnqueueRequestsFromMapFunc: arbitrary mapping — for label-
//     based fan-outs and cross-resource dependencies.
package handler

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/apimachinery/meta"
	"axiomnizam.bitbd.net/axiomnizam/internal/controller/predicate"
	"axiomnizam.bitbd.net/axiomnizam/internal/controller/reconcile"
)

// Queue is the narrow subset of workqueue.Interface a handler needs.
// Taking this interface rather than the full workqueue type lets the
// handler be unit-tested with an in-memory slice.
type Queue interface {
	Add(item interface{})
}

// EventHandler converts an informer event into workqueue additions.
// Implementations receive the same raw object the predicate accepted.
type EventHandler interface {
	// OnCreate fires for Create events.
	OnCreate(obj predicate.Object, q Queue)
	// OnUpdate fires for Update events.
	OnUpdate(oldObj, newObj predicate.Object, q Queue)
	// OnDelete fires for Delete events; obj is the last-known state.
	OnDelete(obj predicate.Object, q Queue)
	// OnGeneric fires for channel-source events.
	OnGeneric(obj predicate.Object, q Queue)
}

// EnqueueRequestForObject enqueues the event's own object.  This is
// the correct handler for the primary resource a controller owns.
type EnqueueRequestForObject struct{}

// OnCreate enqueues the new object.
func (EnqueueRequestForObject) OnCreate(obj predicate.Object, q Queue) { enqueueSelf(obj, q) }

// OnUpdate enqueues the new revision — the old is typically
// irrelevant because reconcilers read the current state themselves.
func (EnqueueRequestForObject) OnUpdate(_, newObj predicate.Object, q Queue) {
	enqueueSelf(newObj, q)
}

// OnDelete enqueues the last-known state so the reconciler can clean
// up any dangling children before acknowledging the disappearance.
func (EnqueueRequestForObject) OnDelete(obj predicate.Object, q Queue) { enqueueSelf(obj, q) }

// OnGeneric enqueues the channel-source object.
func (EnqueueRequestForObject) OnGeneric(obj predicate.Object, q Queue) { enqueueSelf(obj, q) }

// enqueueSelf is the shared helper.
func enqueueSelf(obj predicate.Object, q Queue) {
	m, ok := obj.(meta.Object)
	if !ok || m == nil {
		return
	}
	q.Add(reconcile.Request{Namespace: m.GetNamespace(), Name: m.GetName()})
}

// EnqueueRequestsFromMapFunc runs the user-supplied mapper for each
// event and enqueues whatever it returns.  The mapper gets the event
// kind so it can distinguish create-from-update-from-delete when
// needed.
type EnqueueRequestsFromMapFunc struct {
	// Map is the required translation function.
	Map func(predicate.EventKind, predicate.Object) []reconcile.Request
}

// OnCreate delegates to Map.
func (e EnqueueRequestsFromMapFunc) OnCreate(obj predicate.Object, q Queue) {
	e.enqueueAll(predicate.Create, obj, q)
}

// OnUpdate delegates to Map, passing the new object.
func (e EnqueueRequestsFromMapFunc) OnUpdate(_, newObj predicate.Object, q Queue) {
	e.enqueueAll(predicate.Update, newObj, q)
}

// OnDelete delegates to Map.
func (e EnqueueRequestsFromMapFunc) OnDelete(obj predicate.Object, q Queue) {
	e.enqueueAll(predicate.Delete, obj, q)
}

// OnGeneric delegates to Map.
func (e EnqueueRequestsFromMapFunc) OnGeneric(obj predicate.Object, q Queue) {
	e.enqueueAll(predicate.Generic, obj, q)
}

// enqueueAll is the shared dispatcher.
func (e EnqueueRequestsFromMapFunc) enqueueAll(kind predicate.EventKind, obj predicate.Object, q Queue) {
	if e.Map == nil {
		return
	}
	for _, req := range e.Map(kind, obj) {
		q.Add(req)
	}
}
