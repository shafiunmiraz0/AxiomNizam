// Package meta provides accessor helpers that read and write the
// metav1.ObjectMeta fields of an arbitrary resource object expressed
// as a map[string]interface{}.  This mirrors the role of
// k8s.io/apimachinery/pkg/api/meta.Accessor for unstructured objects.
//
// Callers that work with typed Go structs can embed *metav1.ObjectMeta
// and access its fields directly; callers that work with dynamic
// objects (e.g. the controllers package that persists to etcd as
// JSON) use these helpers to stay type-agnostic.
package meta

import (
	"fmt"

	metav1 "axiomnizam.bitbd.net/axiomnizam/internal/apimachinery/meta/v1"
)

// Object is the minimal read-write view of a resource's metadata.
// The implementation wraps a map[string]interface{} so mutations are
// reflected in the underlying document.
type Object interface {
	GetName() string
	SetName(string)
	GetNamespace() string
	SetNamespace(string)
	GetUID() string
	SetUID(string)
	GetResourceVersion() string
	SetResourceVersion(string)
	GetGeneration() int64
	SetGeneration(int64)
	GetLabels() map[string]string
	SetLabels(map[string]string)
	GetAnnotations() map[string]string
	SetAnnotations(map[string]string)
	GetFinalizers() []string
	SetFinalizers([]string)
	GetOwnerReferences() []metav1.OwnerReference
	SetOwnerReferences([]metav1.OwnerReference)
}

// Accessor wraps an arbitrary object and returns a read/write view of
// its metadata.  It returns an error when the object is not a map or
// has no metadata field; callers use the error to distinguish "not a
// resource" from "resource with empty meta".
func Accessor(obj interface{}) (Object, error) {
	m, ok := obj.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("meta.Accessor: expected map, got %T", obj)
	}
	meta, ok := m["metadata"].(map[string]interface{})
	if !ok {
		// Create on demand — callers often pass an empty shell and
		// expect SetName to succeed immediately.
		meta = map[string]interface{}{}
		m["metadata"] = meta
	}
	return &mapAccessor{meta: meta}, nil
}

// mapAccessor is the concrete implementation.
type mapAccessor struct {
	meta map[string]interface{}
}

func (a *mapAccessor) get(key string) string {
	if s, ok := a.meta[key].(string); ok {
		return s
	}
	return ""
}

func (a *mapAccessor) GetName() string             { return a.get("name") }
func (a *mapAccessor) SetName(v string)            { a.meta["name"] = v }
func (a *mapAccessor) GetNamespace() string        { return a.get("namespace") }
func (a *mapAccessor) SetNamespace(v string)       { a.meta["namespace"] = v }
func (a *mapAccessor) GetUID() string              { return a.get("uid") }
func (a *mapAccessor) SetUID(v string)             { a.meta["uid"] = v }
func (a *mapAccessor) GetResourceVersion() string  { return a.get("resourceVersion") }
func (a *mapAccessor) SetResourceVersion(v string) { a.meta["resourceVersion"] = v }

// GetGeneration reads metadata.generation, coercing float64 (the type
// encoding/json gives us by default for numbers) to int64.
func (a *mapAccessor) GetGeneration() int64 {
	switch v := a.meta["generation"].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}

// SetGeneration stamps metadata.generation.
func (a *mapAccessor) SetGeneration(v int64) { a.meta["generation"] = v }

// GetLabels returns a shallow copy so callers can mutate freely.
func (a *mapAccessor) GetLabels() map[string]string {
	raw, _ := a.meta["labels"].(map[string]interface{})
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// SetLabels writes the map back in map[string]interface{} form.
func (a *mapAccessor) SetLabels(v map[string]string) {
	out := make(map[string]interface{}, len(v))
	for k, s := range v {
		out[k] = s
	}
	a.meta["labels"] = out
}

// GetAnnotations mirrors GetLabels.
func (a *mapAccessor) GetAnnotations() map[string]string {
	raw, _ := a.meta["annotations"].(map[string]interface{})
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// SetAnnotations mirrors SetLabels.
func (a *mapAccessor) SetAnnotations(v map[string]string) {
	out := make(map[string]interface{}, len(v))
	for k, s := range v {
		out[k] = s
	}
	a.meta["annotations"] = out
}

// GetFinalizers returns a copy of the finalizer list.
func (a *mapAccessor) GetFinalizers() []string {
	raw, _ := a.meta["finalizers"].([]interface{})
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// SetFinalizers writes the list back.
func (a *mapAccessor) SetFinalizers(v []string) {
	out := make([]interface{}, 0, len(v))
	for _, s := range v {
		out = append(out, s)
	}
	a.meta["finalizers"] = out
}

// GetOwnerReferences walks metadata.ownerReferences — a list of
// maps in the JSON encoding — and materialises each as an
// OwnerReference struct.  Unknown fields are silently ignored so the
// helper survives apiVersion drift.
func (a *mapAccessor) GetOwnerReferences() []metav1.OwnerReference {
	raw, _ := a.meta["ownerReferences"].([]interface{})
	out := make([]metav1.OwnerReference, 0, len(raw))
	for _, v := range raw {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		ref := metav1.OwnerReference{}
		if s, ok := m["apiVersion"].(string); ok {
			ref.APIVersion = s
		}
		if s, ok := m["kind"].(string); ok {
			ref.Kind = s
		}
		if s, ok := m["name"].(string); ok {
			ref.Name = s
		}
		if s, ok := m["uid"].(string); ok {
			ref.UID = s
		}
		if b, ok := m["controller"].(bool); ok {
			ref.Controller = &b
		}
		if b, ok := m["blockOwnerDeletion"].(bool); ok {
			ref.BlockOwnerDeletion = &b
		}
		out = append(out, ref)
	}
	return out
}

// SetOwnerReferences rewrites the list in JSON-object form.
func (a *mapAccessor) SetOwnerReferences(v []metav1.OwnerReference) {
	out := make([]interface{}, 0, len(v))
	for _, ref := range v {
		entry := map[string]interface{}{
			"apiVersion": ref.APIVersion,
			"kind":       ref.Kind,
			"name":       ref.Name,
			"uid":        ref.UID,
		}
		if ref.Controller != nil {
			entry["controller"] = *ref.Controller
		}
		if ref.BlockOwnerDeletion != nil {
			entry["blockOwnerDeletion"] = *ref.BlockOwnerDeletion
		}
		out = append(out, entry)
	}
	a.meta["ownerReferences"] = out
}

// TypeAccessor extracts the envelope's TypeMeta.  Returns the zero
// value when the object lacks apiVersion / kind — matches k8s upstream.
func TypeAccessor(obj interface{}) metav1.TypeMeta {
	m, ok := obj.(map[string]interface{})
	if !ok {
		return metav1.TypeMeta{}
	}
	tm := metav1.TypeMeta{}
	if s, ok := m["apiVersion"].(string); ok {
		tm.APIVersion = s
	}
	if s, ok := m["kind"].(string); ok {
		tm.Kind = s
	}
	return tm
}

// NamespaceName renders "<ns>/<name>" for logging / key construction.
func NamespaceName(obj Object) string {
	ns := obj.GetNamespace()
	if ns == "" {
		return obj.GetName()
	}
	return ns + "/" + obj.GetName()
}
