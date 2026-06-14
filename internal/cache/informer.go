package cache

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// Informer watches resources and maintains a local cache
type Informer interface {
	// Start starts the informer
	Start(ctx context.Context) error

	// Stop stops the informer
	Stop()

	// HasSynced returns true if initial list from server completed
	HasSynced() bool

	// GetStore returns the underlying store
	GetStore() Store

	// AddEventHandler adds a handler for events
	AddEventHandler(handler ResourceEventHandler) error

	// GetByKey gets resource by key (namespace/name)
	GetByKey(key string) (interface{}, bool, error)

	// GetIndexer returns the underlying indexer
	GetIndexer() Indexer
}

// SharedInformer is a thread-safe informer shared across components
type SharedInformer struct {
	store           Store
	indexer         Indexer
	watcher         ResourceWatcher
	eventHandlers   []ResourceEventHandler
	hasSynced       bool
	resyncPeriod    time.Duration
	lastResyncTime  time.Time
	mu              sync.RWMutex
	stopCh          chan struct{}
	syncedCh        chan struct{}
	resourceVersion string
}

// ResourceWatcher watches for resource changes
type ResourceWatcher interface {
	Watch(ctx context.Context) (<-chan WatchEvent, error)
}

// WatchEvent represents a change to a resource
type WatchEvent struct {
	Type   EventWatchType // Added, Modified, Deleted
	Object interface{}
	Error  error
}

// EventWatchType represents the type of watch event
type EventWatchType string

const (
	WatchEventAdded    EventWatchType = "ADDED"
	WatchEventModified EventWatchType = "MODIFIED"
	WatchEventDeleted  EventWatchType = "DELETED"
)

// ResourceEventHandler handles resource events
type ResourceEventHandler interface {
	OnAdd(obj interface{}, isInitialList bool) error
	OnUpdate(oldObj, newObj interface{}) error
	OnDelete(obj interface{}) error
}

// HandlerFuncs provides simple handler implementation
type HandlerFuncs struct {
	AddFunc    func(obj interface{}, isInitialList bool) error
	UpdateFunc func(oldObj, newObj interface{}) error
	DeleteFunc func(obj interface{}) error
}

// OnAdd implements ResourceEventHandler
func (h *HandlerFuncs) OnAdd(obj interface{}, isInitialList bool) error {
	if h.AddFunc != nil {
		return h.AddFunc(obj, isInitialList)
	}
	return nil
}

// OnUpdate implements ResourceEventHandler
func (h *HandlerFuncs) OnUpdate(oldObj, newObj interface{}) error {
	if h.UpdateFunc != nil {
		return h.UpdateFunc(oldObj, newObj)
	}
	return nil
}

// OnDelete implements ResourceEventHandler
func (h *HandlerFuncs) OnDelete(obj interface{}) error {
	if h.DeleteFunc != nil {
		return h.DeleteFunc(obj)
	}
	return nil
}

// NewSharedInformer creates a new shared informer
func NewSharedInformer(
	watcher ResourceWatcher,
	resyncPeriod time.Duration,
) *SharedInformer {
	return &SharedInformer{
		store:         NewThreadSafeStore(),
		indexer:       NewIndexer(),
		watcher:       watcher,
		eventHandlers: make([]ResourceEventHandler, 0),
		resyncPeriod:  resyncPeriod,
		stopCh:        make(chan struct{}),
		syncedCh:      make(chan struct{}),
	}
}

// Start starts the informer
func (si *SharedInformer) Start(ctx context.Context) error {
	si.mu.Lock()
	defer si.mu.Unlock()

	// Start watch loop
	go si.run(ctx)

	// Wait for initial sync
	select {
	case <-si.syncedCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Second):
		return nil // Timeout, but continue running
	}
}

// Stop stops the informer
func (si *SharedInformer) Stop() {
	si.mu.Lock()
	defer si.mu.Unlock()
	close(si.stopCh)
}

// HasSynced returns true if initial list from server completed
func (si *SharedInformer) HasSynced() bool {
	si.mu.RLock()
	defer si.mu.RUnlock()
	return si.hasSynced
}

// GetStore returns the underlying store
func (si *SharedInformer) GetStore() Store {
	return si.store
}

// GetIndexer returns the underlying indexer
func (si *SharedInformer) GetIndexer() Indexer {
	return si.indexer
}

// AddEventHandler adds a handler for events
func (si *SharedInformer) AddEventHandler(handler ResourceEventHandler) error {
	si.mu.Lock()
	defer si.mu.Unlock()
	si.eventHandlers = append(si.eventHandlers, handler)
	return nil
}

// GetByKey gets resource by key (namespace/name)
func (si *SharedInformer) GetByKey(key string) (interface{}, bool, error) {
	return si.store.GetByKey(key)
}

// run is the main watch loop
func (si *SharedInformer) run(ctx context.Context) {
	eventsCh, err := si.watcher.Watch(ctx)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("cache informer: initial watch failed: %v", err))
		return
	}

	initialSyncDone := false

	for {
		select {
		case <-si.stopCh:
			return

		case event := <-eventsCh:
			if event.Error != nil {
				// Restart watch on error with backoff
				logging.Z().Info(fmt.Sprintf("cache informer: watch error, restarting: %v", event.Error))
				time.Sleep(time.Second)
				newCh, watchErr := si.watcher.Watch(ctx)
				if watchErr != nil {
					logging.Z().Info(fmt.Sprintf("cache informer: watch restart failed: %v", watchErr))
					time.Sleep(5 * time.Second)
					continue
				}
				eventsCh = newCh
				continue
			}

			si.handleWatchEvent(event, !initialSyncDone)

			if !initialSyncDone {
				initialSyncDone = true
				si.mu.Lock()
				si.hasSynced = true
				close(si.syncedCh)
				si.mu.Unlock()
			}

		case <-time.After(si.resyncPeriod):
			// Periodic resync
			si.resync()
		}
	}
}

// handleWatchEvent processes a watch event
func (si *SharedInformer) handleWatchEvent(event WatchEvent, isInitialList bool) {
	si.mu.RLock()
	handlers := make([]ResourceEventHandler, len(si.eventHandlers))
	copy(handlers, si.eventHandlers)
	si.mu.RUnlock()

	key := extractKey(event.Object)

	switch event.Type {
	case WatchEventAdded:
		si.store.Add(event.Object)
		si.indexer.Add(event.Object)
		for _, handler := range handlers {
			handler.OnAdd(event.Object, isInitialList)
		}

	case WatchEventModified:
		oldObj, _, _ := si.store.GetByKey(key)
		si.store.Update(event.Object)
		si.indexer.Update(oldObj, event.Object)
		for _, handler := range handlers {
			handler.OnUpdate(oldObj, event.Object)
		}

	case WatchEventDeleted:
		oldObj, _, _ := si.store.GetByKey(key)
		si.store.Delete(event.Object)
		si.indexer.Delete(event.Object)
		for _, handler := range handlers {
			handler.OnDelete(oldObj)
		}
	}
}

// resync resyncs the cache
func (si *SharedInformer) resync() {
	si.mu.Lock()
	defer si.mu.Unlock()
	si.lastResyncTime = time.Now()
}

// SharedIndexInformer combines informer with indexing
type SharedIndexInformer struct {
	informer *SharedInformer
	indexes  map[string]IndexFunc
}

// NewSharedIndexInformer creates a new shared index informer
func NewSharedIndexInformer(
	watcher ResourceWatcher,
	resyncPeriod time.Duration,
) *SharedIndexInformer {
	return &SharedIndexInformer{
		informer: NewSharedInformer(watcher, resyncPeriod),
		indexes:  make(map[string]IndexFunc),
	}
}

// AddIndexers adds index functions
func (sii *SharedIndexInformer) AddIndexers(indexers map[string]IndexFunc) error {
	for name, fn := range indexers {
		sii.indexes[name] = fn
	}
	return nil
}

// Index returns all objects matching index criteria
func (sii *SharedIndexInformer) Index(indexName string, obj interface{}) ([]interface{}, error) {
	indexFunc, ok := sii.indexes[indexName]
	if !ok {
		return nil, nil
	}

	values, err := indexFunc(obj)
	if err != nil {
		return nil, err
	}

	var results []interface{}
	for range values {
		objs := sii.informer.GetStore().ListBySelector(nil)
		results = append(results, objs...)
	}

	return results, nil
}

// Start starts the informer
func (sii *SharedIndexInformer) Start(ctx context.Context) error {
	return sii.informer.Start(ctx)
}

// Stop stops the informer
func (sii *SharedIndexInformer) Stop() {
	sii.informer.Stop()
}

// GetStore returns the underlying store
func (sii *SharedIndexInformer) GetStore() Store {
	return sii.informer.GetStore()
}

// HasSynced returns true if initial sync completed
func (sii *SharedIndexInformer) HasSynced() bool {
	return sii.informer.HasSynced()
}

// AddEventHandler adds a handler
func (sii *SharedIndexInformer) AddEventHandler(handler ResourceEventHandler) error {
	return sii.informer.AddEventHandler(handler)
}

// extractKey extracts namespace/name key from object
func extractKey(obj interface{}) string {
	if res, ok := obj.(resources.Resource); ok {
		meta := res.GetObjectMeta()
		if meta.Namespace != "" {
			return meta.Namespace + "/" + meta.Name
		}
		return meta.Name
	}
	return ""
}

// ExtractKey is an exported version of extractKey for use by other packages
func ExtractKey(obj interface{}) string {
	return extractKey(obj)
}

// InformerFactory creates informers for different resources
type InformerFactory struct {
	informers map[string]Informer
	mu        sync.RWMutex
}

// NewInformerFactory creates a new informer factory
func NewInformerFactory() *InformerFactory {
	return &InformerFactory{
		informers: make(map[string]Informer),
	}
}

// GetInformer gets or creates an informer for a resource type
func (f *InformerFactory) GetInformer(resourceType string, watcher ResourceWatcher) Informer {
	f.mu.Lock()
	defer f.mu.Unlock()

	if informer, ok := f.informers[resourceType]; ok {
		return informer
	}

	informer := NewSharedInformer(watcher, 5*time.Minute)
	f.informers[resourceType] = informer
	return informer
}

// Start starts all informers
func (f *InformerFactory) Start(ctx context.Context) error {
	f.mu.RLock()
	informers := make([]Informer, 0, len(f.informers))
	for _, informer := range f.informers {
		informers = append(informers, informer)
	}
	f.mu.RUnlock()

	for _, informer := range informers {
		if err := informer.Start(ctx); err != nil {
			return err
		}
	}

	return nil
}

// Stop stops all informers
func (f *InformerFactory) Stop() {
	f.mu.RLock()
	informers := make([]Informer, 0, len(f.informers))
	for _, informer := range f.informers {
		informers = append(informers, informer)
	}
	f.mu.RUnlock()

	for _, informer := range informers {
		informer.Stop()
	}
}
