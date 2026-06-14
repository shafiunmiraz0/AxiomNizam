// Package source defines the event feed a controller subscribes to.
// Two concrete sources cover nearly all real-world needs:
//
//   - Kind:    events from a SharedInformer watching a specific GVK.
//   - Channel: an arbitrary <-chan Event for external signals
//     (cron ticks, HTTP webhook notifications, message-bus pushes).
//
// A Source is effectively a plumbing adapter between the thing
// producing events and the handler.EventHandler that converts them
// to workqueue.Requests.  The split into Source + Predicate + Handler
// is the same three-stage pipeline controller-runtime uses upstream.
package source

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/controller/handler"
	"axiomnizam.bitbd.net/axiomnizam/internal/controller/predicate"
)

// Event is the canonical envelope passed down the pipeline.  Old is
// nil except for Update events.
type Event struct {
	Kind   predicate.EventKind
	Old    predicate.Object
	Object predicate.Object
}

// Source is the interface every feed implements.  Start begins
// pushing events through the configured handler/predicate chain into
// the queue; it returns when ctx is cancelled.
type Source interface {
	// Start wires the source into the controller's pipeline.  It is
	// expected to return quickly — long-lived work should run in
	// goroutines the source manages itself.
	Start(ctx context.Context, h handler.EventHandler, q handler.Queue, preds ...predicate.Predicate) error
}

// dispatch is the shared funnel every Source implementation calls.
// It enforces the Predicate gate before dispatching to the handler.
func dispatch(h handler.EventHandler, q handler.Queue, preds []predicate.Predicate, evt Event) {
	for _, p := range preds {
		if p == nil {
			continue
		}
		if !p.Accept(evt.Kind, evt.Old, evt.Object) {
			return
		}
	}
	switch evt.Kind {
	case predicate.Create:
		h.OnCreate(evt.Object, q)
	case predicate.Update:
		h.OnUpdate(evt.Old, evt.Object, q)
	case predicate.Delete:
		h.OnDelete(evt.Object, q)
	case predicate.Generic:
		h.OnGeneric(evt.Object, q)
	}
}
