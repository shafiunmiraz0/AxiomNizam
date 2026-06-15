// Package runtime is a minimal stand-in for the registration and
// codec machinery in k8s.io/apimachinery/pkg/runtime.  A Scheme maps
// GroupVersionKind to a concrete Go type so that a decoder can take
// a JSON blob with {"apiVersion":"...", "kind":"..."} and dispatch
// to the correct struct.
//
// AxiomNizam does not need the full conversion-to-internal-version
// machinery upstream uses, so this package is deliberately smaller:
// just register, recognise, and decode.
package runtime

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	metav1 "axiomnizam.bitbd.net/axiomnizam/internal/apimachinery/meta/v1"
)

// Object is the interface every registered type must satisfy.  Any
// struct that embeds metav1.TypeMeta automatically does.
type Object interface {
	GetObjectKind() *metav1.TypeMeta
	// DeepCopyObject returns a deep copy.  Implementations typically
	// delegate to a code-generated DeepCopy method, but runtime does
	// not require any specific generator.
	DeepCopyObject() Object
}

// Scheme is the type registry.
type Scheme struct {
	mu     sync.RWMutex
	byGVK  map[metav1.GroupVersionKind]reflect.Type
	byType map[reflect.Type][]metav1.GroupVersionKind
}

// NewScheme constructs an empty Scheme.
func NewScheme() *Scheme {
	return &Scheme{
		byGVK:  map[metav1.GroupVersionKind]reflect.Type{},
		byType: map[reflect.Type][]metav1.GroupVersionKind{},
	}
}

// AddKnownTypes registers one or more exemplar objects under gv.
// The Kind is derived from the struct name via reflection.  Registering
// the same type twice under different GVKs is allowed — useful for
// alias versions during a migration.
func (s *Scheme) AddKnownTypes(gv metav1.GroupVersion, types ...Object) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range types {
		rt := reflect.TypeOf(t)
		if rt.Kind() == reflect.Ptr {
			rt = rt.Elem()
		}
		gvk := gv.WithKind(rt.Name())
		s.byGVK[gvk] = rt
		s.byType[rt] = append(s.byType[rt], gvk)
	}
}

// AddKnownTypeWithName registers t under the explicit kind name, for
// types whose Go name differs from their exposed Kind.
func (s *Scheme) AddKnownTypeWithName(gvk metav1.GroupVersionKind, t Object) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rt := reflect.TypeOf(t)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	s.byGVK[gvk] = rt
	s.byType[rt] = append(s.byType[rt], gvk)
}

// New constructs a new zero-valued instance of the type registered
// under gvk.  Returns an error for unknown kinds.
func (s *Scheme) New(gvk metav1.GroupVersionKind) (Object, error) {
	s.mu.RLock()
	rt, ok := s.byGVK[gvk]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown kind %s", gvk)
	}
	v := reflect.New(rt).Interface()
	obj, ok := v.(Object)
	if !ok {
		return nil, fmt.Errorf("registered type %s does not satisfy runtime.Object", gvk)
	}
	return obj, nil
}

// ObjectKinds returns every GVK under which obj is registered.  Used
// by encoders that need to stamp TypeMeta before marshaling.
func (s *Scheme) ObjectKinds(obj Object) ([]metav1.GroupVersionKind, error) {
	rt := reflect.TypeOf(obj)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	kinds, ok := s.byType[rt]
	if !ok {
		return nil, fmt.Errorf("type %s is not registered", rt.Name())
	}
	return kinds, nil
}

// Recognises reports whether gvk has a registered type.
func (s *Scheme) Recognises(gvk metav1.GroupVersionKind) bool {
	s.mu.RLock()
	_, ok := s.byGVK[gvk]
	s.mu.RUnlock()
	return ok
}

// Decode parses data into a typed Object using TypeMeta to select
// the target type.  Callers that know the type up front should use
// json.Unmarshal directly; Decode is for dynamic dispatch.
func (s *Scheme) Decode(data []byte) (Object, error) {
	var envelope metav1.TypeMeta
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	gvk := envelope.GroupVersionKind()
	if gvk.Empty() {
		return nil, fmt.Errorf("object has no apiVersion / kind")
	}
	obj, err := s.New(gvk)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, obj); err != nil {
		return nil, fmt.Errorf("decode into %s: %w", gvk, err)
	}
	return obj, nil
}

// Encode is the inverse of Decode: it stamps TypeMeta from the first
// registered GVK for obj and serialises to JSON.
func (s *Scheme) Encode(obj Object) ([]byte, error) {
	kinds, err := s.ObjectKinds(obj)
	if err != nil {
		return nil, err
	}
	if len(kinds) == 0 {
		return nil, fmt.Errorf("no GVK registered for %T", obj)
	}
	obj.GetObjectKind().SetGroupVersionKind(kinds[0])
	return json.Marshal(obj)
}
