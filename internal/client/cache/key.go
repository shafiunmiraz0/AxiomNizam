// Package cache — standard key functions.
//
// MetaNamespaceKeyFunc and SplitMetaNamespaceKey are the canonical
// pair every informer configuration uses.  They operate on objects
// that expose metadata via the apimachinery/meta package.
package cache

import (
	"fmt"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/apimachinery/meta"
)

// MetaNamespaceKeyFunc returns "namespace/name" for namespaced
// resources and just "name" for cluster-scoped.  It is the default
// KeyFunc for ThreadSafeStore and DeltaFIFO.
func MetaNamespaceKeyFunc(obj interface{}) (string, error) {
	if ts, ok := obj.(tombstone); ok {
		return ts.Key, nil
	}
	m, err := meta.Accessor(obj)
	if err != nil {
		return "", fmt.Errorf("MetaNamespaceKeyFunc: %w", err)
	}
	ns := m.GetNamespace()
	name := m.GetName()
	if name == "" {
		return "", fmt.Errorf("MetaNamespaceKeyFunc: object has empty name")
	}
	if ns == "" {
		return name, nil
	}
	return ns + "/" + name, nil
}

// SplitMetaNamespaceKey is the inverse.  Returns an error when key
// contains more than one "/" — which would indicate a namespace or
// name with an illegal character.
func SplitMetaNamespaceKey(key string) (namespace, name string, err error) {
	parts := strings.Split(key, "/")
	switch len(parts) {
	case 1:
		return "", parts[0], nil
	case 2:
		return parts[0], parts[1], nil
	default:
		return "", "", fmt.Errorf("SplitMetaNamespaceKey: unexpected key format %q", key)
	}
}

// NamespaceIndex is the standard "by namespace" IndexFunc for
// informers that want cheap namespace-scoped listings.
func NamespaceIndex(obj interface{}) ([]string, error) {
	m, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	return []string{m.GetNamespace()}, nil
}

// LabelIndex returns an IndexFunc that emits one index entry per
// label value for the named label.  Useful for "all objects with
// app=foo" fan-outs.
func LabelIndex(labelKey string) IndexFunc {
	return func(obj interface{}) ([]string, error) {
		m, err := meta.Accessor(obj)
		if err != nil {
			return nil, err
		}
		v, ok := m.GetLabels()[labelKey]
		if !ok {
			return nil, nil
		}
		return []string{v}, nil
	}
}
