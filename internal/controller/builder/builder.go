// Package builder provides a fluent API for wiring together a
// controller's Reconciler, Sources, Predicates, and Handlers.  The
// upstream controller-runtime Builder is the canonical template —
// this one is pared down to what AxiomNizam actually uses.
//
// Example:
//
//	err := builder.New(mgr).
//	    For(jobKindInformer, handler.EnqueueRequestForObject{}).
//	    Owns(podKindInformer, handler.EnqueueRequestForOwner{
//	        OwnerKind: "Job", IsController: true,
//	    }).
//	    WithPredicates(predicate.GenerationChangedPredicate{}).
//	    WithConcurrency(4).
//	    Complete(&JobReconciler{})
//
// The Build*() / Complete() split matches upstream: Build returns
// the Controller value for callers that want to start it manually;
// Complete registers it with the Manager and returns only the error.
package builder

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/controller/handler"
	"axiomnizam.bitbd.net/axiomnizam/internal/controller/predicate"
	"axiomnizam.bitbd.net/axiomnizam/internal/controller/reconcile"
	"axiomnizam.bitbd.net/axiomnizam/internal/controller/source"
)

// Manager is the narrow subset of manager.Manager the Builder uses.
// Defined locally to avoid an import cycle.
type Manager interface {
	// Add registers a Runnable with the manager.  The Runnable will
	// be started when the manager starts, and the manager blocks on
	// Start until every Runnable returns.
	Add(r Runnable) error
}

// Runnable mirrors manager.Runnable — any long-lived goroutine that
// wants lifecycle coordination.
type Runnable = interface {
	Start(stopCh <-chan struct{}) error
}

// Builder assembles a single Controller.
type Builder struct {
	mgr      Manager
	watches  []watchSpec
	preds    []predicate.Predicate
	parallel int
	name     string
	reconcil reconcile.Reconciler
}

// watchSpec records one Source+Handler pair to register at Complete time.
type watchSpec struct {
	src     source.Source
	handler handler.EventHandler
	preds   []predicate.Predicate
}

// New returns a Builder bound to mgr.  Further calls add watches,
// predicates, and configuration; Complete produces a controller and
// registers it with the manager.
func New(mgr Manager) *Builder { return &Builder{mgr: mgr, parallel: 1} }

// Named sets a human-readable name used in logs and metrics.  If
// unset, Complete derives a name from the Reconciler's %T.
func (b *Builder) Named(name string) *Builder { b.name = name; b.parallel = b.parallel; return b }

// For adds the primary watch — the resource whose reconciler this
// controller drives.  Typically paired with EnqueueRequestForObject.
func (b *Builder) For(src source.Source, h handler.EventHandler, preds ...predicate.Predicate) *Builder {
	b.watches = append(b.watches, watchSpec{src: src, handler: h, preds: preds})
	return b
}

// Owns adds a secondary watch for a child resource.  Typically
// paired with EnqueueRequestForOwner so changes to the child trigger
// reconciliation of its parent.
func (b *Builder) Owns(src source.Source, h handler.EventHandler, preds ...predicate.Predicate) *Builder {
	b.watches = append(b.watches, watchSpec{src: src, handler: h, preds: preds})
	return b
}

// Watches is the general-purpose form — used for cross-resource
// dependencies that are neither "for" (primary) nor "owns" (child).
func (b *Builder) Watches(src source.Source, h handler.EventHandler, preds ...predicate.Predicate) *Builder {
	b.watches = append(b.watches, watchSpec{src: src, handler: h, preds: preds})
	return b
}

// WithPredicates adds predicates applied to every watch that does
// not specify its own.
func (b *Builder) WithPredicates(preds ...predicate.Predicate) *Builder {
	b.preds = append(b.preds, preds...)
	return b
}

// WithConcurrency sets the number of reconciler goroutines.  Default 1.
func (b *Builder) WithConcurrency(n int) *Builder {
	if n > 0 {
		b.parallel = n
	}
	return b
}

// Complete finalises the controller, registers it with the manager,
// and returns any validation error.  Builders that do not call
// Complete never start anything — useful for unit tests that want to
// introspect the assembled spec.
func (b *Builder) Complete(r reconcile.Reconciler) error {
	if r == nil {
		return fmt.Errorf("builder: nil Reconciler")
	}
	if b.mgr == nil {
		return fmt.Errorf("builder: nil Manager")
	}
	if len(b.watches) == 0 {
		return fmt.Errorf("builder: no watches configured — add at least one For()")
	}
	b.reconcil = r
	ctrl := &Controller{
		Name:        b.chooseName(r),
		Reconciler:  r,
		Watches:     b.watches,
		GlobalPreds: b.preds,
		WorkerCount: b.parallel,
	}
	return b.mgr.Add(ctrl)
}

// chooseName picks a log-friendly controller name.
func (b *Builder) chooseName(r reconcile.Reconciler) string {
	if b.name != "" {
		return b.name
	}
	return fmt.Sprintf("%T", r)
}
