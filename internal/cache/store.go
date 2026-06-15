package cache

import (
	"fmt"
	"sync"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// Store interface for storing resources
type Store interface {
	// Add adds an object to the store
	Add(obj interface{}) error

	// Update updates an object in the store
	Update(obj interface{}) error

	// Delete deletes an object from the store
	Delete(obj interface{}) error

	// Get retrieves object by key (namespace/name)
	GetByKey(key string) (interface{}, bool, error)

	// List lists all objects
	List() []interface{}

	// ListBySelector lists objects matching selector
	ListBySelector(selector map[string]string) []interface{}

	// Clear clears all objects
	Clear() error

	// Resync resynchronizes the store
	Resync() error
}

// ThreadSafeStore implements a thread-safe object store
type ThreadSafeStore struct {
	objects map[string]interface{}
	mu      sync.RWMutex
}

// NewThreadSafeStore creates a new thread-safe store
func NewThreadSafeStore() *ThreadSafeStore {
	return &ThreadSafeStore{
		objects: make(map[string]interface{}),
	}
}

// Add adds an object to the store
func (tss *ThreadSafeStore) Add(obj interface{}) error {
	key := extractKeyFromObject(obj)
	tss.mu.Lock()
	defer tss.mu.Unlock()
	tss.objects[key] = obj
	return nil
}

// Update updates an object in the store
func (tss *ThreadSafeStore) Update(obj interface{}) error {
	return tss.Add(obj) // In simple store, update is same as add
}

// Delete deletes an object from the store
func (tss *ThreadSafeStore) Delete(obj interface{}) error {
	key := extractKeyFromObject(obj)
	tss.mu.Lock()
	defer tss.mu.Unlock()
	delete(tss.objects, key)
	return nil
}

// GetByKey retrieves object by key
func (tss *ThreadSafeStore) GetByKey(key string) (interface{}, bool, error) {
	tss.mu.RLock()
	defer tss.mu.RUnlock()
	obj, ok := tss.objects[key]
	return obj, ok, nil
}

// List lists all objects
func (tss *ThreadSafeStore) List() []interface{} {
	tss.mu.RLock()
	defer tss.mu.RUnlock()

	list := make([]interface{}, 0, len(tss.objects))
	for _, obj := range tss.objects {
		list = append(list, obj)
	}
	return list
}

// ListBySelector lists objects matching selector
func (tss *ThreadSafeStore) ListBySelector(selector map[string]string) []interface{} {
	if selector == nil || len(selector) == 0 {
		return tss.List()
	}

	tss.mu.RLock()
	defer tss.mu.RUnlock()

	var results []interface{}
	for _, obj := range tss.objects {
		if matchesSelector(obj, selector) {
			results = append(results, obj)
		}
	}
	return results
}

// Clear clears all objects
func (tss *ThreadSafeStore) Clear() error {
	tss.mu.Lock()
	defer tss.mu.Unlock()
	tss.objects = make(map[string]interface{})
	return nil
}

// Resync resyncs the store (no-op for simple store)
func (tss *ThreadSafeStore) Resync() error {
	return nil
}

// Indexer provides indexed access to objects
type Indexer interface {
	// Add adds an object to indexer
	Add(obj interface{}) error

	// Update updates an object in indexer
	Update(oldObj, newObj interface{}) error

	// Delete deletes an object from indexer
	Delete(obj interface{}) error

	// Index returns objects matching index criteria
	Index(indexName string, indexKey string) ([]interface{}, error)

	// IndexKeys returns keys for an index
	IndexKeys(indexName string, indexedValue string) ([]string, error)

	// ByIndex returns objects by index
	ByIndex(indexName string, indexedValue string) ([]interface{}, error)

	// AddIndexers adds index functions
	AddIndexers(indexers map[string]IndexFunc) error
}

// SimpleIndexer implements basic indexing
type SimpleIndexer struct {
	indexes    map[string]map[string][]interface{}
	indexFuncs map[string]IndexFunc
	mu         sync.RWMutex
}

// NewIndexer creates a new indexer
func NewIndexer() *SimpleIndexer {
	return &SimpleIndexer{
		indexes:    make(map[string]map[string][]interface{}),
		indexFuncs: make(map[string]IndexFunc),
	}
}

// Add adds an object to indexer
func (si *SimpleIndexer) Add(obj interface{}) error {
	si.mu.Lock()
	defer si.mu.Unlock()

	for indexName, indexFunc := range si.indexFuncs {
		values, err := indexFunc(obj)
		if err != nil {
			continue
		}

		if si.indexes[indexName] == nil {
			si.indexes[indexName] = make(map[string][]interface{})
		}

		for _, value := range values {
			si.indexes[indexName][value] = append(si.indexes[indexName][value], obj)
		}
	}

	return nil
}

// Update updates an object in indexer
func (si *SimpleIndexer) Update(oldObj, newObj interface{}) error {
	_ = si.Delete(oldObj)
	return si.Add(newObj)
}

// Delete deletes an object from indexer
func (si *SimpleIndexer) Delete(obj interface{}) error {
	si.mu.Lock()
	defer si.mu.Unlock()

	for indexName := range si.indexes {
		for key := range si.indexes[indexName] {
			// Remove object from index
			objs := si.indexes[indexName][key]
			filtered := make([]interface{}, 0)
			for _, o := range objs {
				if !objectsEqual(o, obj) {
					filtered = append(filtered, o)
				}
			}
			si.indexes[indexName][key] = filtered
		}
	}

	return nil
}

// Index returns objects matching index criteria
func (si *SimpleIndexer) Index(indexName string, indexKey string) ([]interface{}, error) {
	si.mu.RLock()
	defer si.mu.RUnlock()

	if _, ok := si.indexes[indexName]; !ok {
		return nil, fmt.Errorf("unknown index %s", indexName)
	}

	return si.indexes[indexName][indexKey], nil
}

// IndexKeys returns keys for an index
func (si *SimpleIndexer) IndexKeys(indexName string, indexedValue string) ([]string, error) {
	si.mu.RLock()
	defer si.mu.RUnlock()

	if _, ok := si.indexes[indexName]; !ok {
		return nil, fmt.Errorf("unknown index %s", indexName)
	}

	keys := make([]string, 0)
	for key := range si.indexes[indexName] {
		if key == indexedValue {
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// ByIndex returns objects by index
func (si *SimpleIndexer) ByIndex(indexName string, indexedValue string) ([]interface{}, error) {
	return si.Index(indexName, indexedValue)
}

// AddIndexers adds index functions
func (si *SimpleIndexer) AddIndexers(indexers map[string]IndexFunc) error {
	si.mu.Lock()
	defer si.mu.Unlock()

	for name, fn := range indexers {
		si.indexFuncs[name] = fn
		si.indexes[name] = make(map[string][]interface{})
	}

	return nil
}

// Helper functions

func extractKeyFromObject(obj interface{}) string {
	if res, ok := obj.(resources.Resource); ok {
		meta := res.GetObjectMeta()
		if meta.Namespace != "" {
			return meta.Namespace + "/" + meta.Name
		}
		return meta.Name
	}
	return ""
}

func matchesSelector(obj interface{}, selector map[string]string) bool {
	if res, ok := obj.(resources.Resource); ok {
		meta := res.GetObjectMeta()
		for key, value := range selector {
			if labelVal, ok := meta.Labels[key]; !ok || labelVal != value {
				return false
			}
		}
		return true
	}
	return false
}

func objectsEqual(obj1, obj2 interface{}) bool {
	if res1, ok1 := obj1.(resources.Resource); ok1 {
		if res2, ok2 := obj2.(resources.Resource); ok2 {
			meta1 := res1.GetObjectMeta()
			meta2 := res2.GetObjectMeta()
			return meta1.Name == meta2.Name && meta1.Namespace == meta2.Namespace
		}
	}
	return false
}

// IndexFunc defines a function that extracts values for indexing
type IndexFunc func(obj interface{}) ([]string, error)

// NamespaceIndexFunc indexes objects by namespace
func NamespaceIndexFunc(obj interface{}) ([]string, error) {
	if res, ok := obj.(resources.Resource); ok {
		meta := res.GetObjectMeta()
		return []string{meta.Namespace}, nil
	}
	return nil, fmt.Errorf("invalid object type")
}

// OwnerReferenceIndexFunc indexes objects by owner reference
func OwnerReferenceIndexFunc(obj interface{}) ([]string, error) {
	if res, ok := obj.(resources.Resource); ok {
		meta := res.GetObjectMeta()
		keys := make([]string, len(meta.OwnerReferences))
		for i, owner := range meta.OwnerReferences {
			keys[i] = fmt.Sprintf("%s/%s/%s", owner.APIVersion, owner.Kind, owner.Name)
		}
		return keys, nil
	}
	return nil, fmt.Errorf("invalid object type")
}

// LabelIndexFunc creates an index function for a specific label
func LabelIndexFunc(labelKey string) IndexFunc {
	return func(obj interface{}) ([]string, error) {
		if res, ok := obj.(resources.Resource); ok {
			meta := res.GetObjectMeta()
			if value, ok := meta.Labels[labelKey]; ok {
				return []string{value}, nil
			}
			return []string{}, nil
		}
		return nil, fmt.Errorf("invalid object type")
	}
}
