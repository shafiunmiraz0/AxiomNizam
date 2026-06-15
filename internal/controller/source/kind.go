// Package source — Kind source wraps a SharedInformer.
//
// The Kind source is the workhorse — it watches the object cache for
// a specific resource type and emits Create/Update/Delete events.
// Because AxiomNizam's informer package uses ResourceEventHandler
// callbacks with pre-typed arguments, the Kind source's job is to
// convert those into the source.Event envelope and funnel them
// through the same predicate+handler dispatch as every other source.
package source

import (
	"context"
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/controller/handler"
	"axiomnizam.bitbd.net/axiomnizam/internal/controller/predicate"
)

// SharedInformer is the minimal subset of informer.SharedInformer
// this source depends on.  Keeping the dependency as an interface
// rather than importing the concrete package avoids a circular
// import between controller/* and informer.
type SharedInformer interface {
	// AddEventHandler registers a callback triple.
	AddEventHandler(h ResourceEventHandler)
}

// ResourceEventHandler matches the informer's callback signature.
// onAdd fires once for every object already in the cache at
// subscription time plus once per subsequent add.
type ResourceEventHandler interface {
	OnAdd(obj interface{})
	OnUpdate(oldObj, newObj interface{})
	OnDelete(obj interface{})
}

// handlerFuncs is a struct form that satisfies ResourceEventHandler
// from plain function literals.
type handlerFuncs struct {
	Add    func(interface{})
	Update func(interface{}, interface{})
	Delete func(interface{})
}

// OnAdd dispatches to Add if set.
func (f handlerFuncs) OnAdd(obj interface{}) {
	if f.Add != nil {
		f.Add(obj)
	}
}

// OnUpdate dispatches to Update if set.
func (f handlerFuncs) OnUpdate(oldObj, newObj interface{}) {
	if f.Update != nil {
		f.Update(oldObj, newObj)
	}
}

// OnDelete dispatches to Delete if set.
func (f handlerFuncs) OnDelete(obj interface{}) {
	if f.Delete != nil {
		f.Delete(obj)
	}
}

// Kind is the informer-backed source.  Callers construct it with the
// desired informer and pass it to a controller; Start registers the
// dispatch callbacks.
type Kind struct {
	// Informer is the shared informer to subscribe to.  Must not be nil.
	Informer SharedInformer
}

// Start wires informer events into the controller pipeline.  Because
// SharedInformer notifications fire on a single goroutine owned by
// the informer, the dispatch happens synchronously — callers who
// need asynchrony should compose a buffered channel on top.
func (k *Kind) Start(_ context.Context, h handler.EventHandler, q handler.Queue, preds ...predicate.Predicate) error {
	if k.Informer == nil {
		return fmt.Errorf("source.Kind: Informer is nil")
	}
	if h == nil {
		return fmt.Errorf("source.Kind: handler is nil")
	}
	asObj := func(v interface{}) predicate.Object {
		o, _ := v.(predicate.Object)
		return o
	}
	k.Informer.AddEventHandler(handlerFuncs{
		Add: func(obj interface{}) {
			dispatch(h, q, preds, Event{Kind: predicate.Create, Object: asObj(obj)})
		},
		Update: func(oldObj, newObj interface{}) {
			dispatch(h, q, preds, Event{Kind: predicate.Update, Old: asObj(oldObj), Object: asObj(newObj)})
		},
		Delete: func(obj interface{}) {
			dispatch(h, q, preds, Event{Kind: predicate.Delete, Object: asObj(obj)})
		},
	})
	return nil
}
