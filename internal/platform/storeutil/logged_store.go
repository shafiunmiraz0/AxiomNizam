// Package storeutil provides error-handling wrappers for store operations.
//
// The platform's reconcilers persist status updates via store.ResourceStore.
// Historically these calls used `_ = store.Update(...)`, silently discarding
// errors. This package provides logged wrappers that surface failures via
// structured logging and return the error so callers can decide whether to
// retry or degrade gracefully.
//
// Usage in reconcilers:
//
//	import "axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
//
//	if err := storeutil.Update(ctx, r.store, resource); err != nil {
//	    return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 5 * time.Second}
//	}
package storeutil

import (
	"context"
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"

	"go.uber.org/zap"
)

// Update persists a resource status update, logging any error.
// Returns nil if the store is nil (graceful no-op for tests).
func Update[T store.Resource](ctx context.Context, s store.ResourceStore[T], obj T) error {
	if s == nil {
		return nil
	}
	if err := s.Update(ctx, obj); err != nil {
		logging.Z().Error("store update failed",
			zap.String("resource", obj.GetKey()),
			zap.String("kind", obj.GetTypeMeta().Kind),
			zap.Error(err),
		)
		return fmt.Errorf("store update %s/%s: %w", obj.GetTypeMeta().Kind, obj.GetKey(), err)
	}
	return nil
}

// Create persists a new resource, logging any error.
// Returns nil if the store is nil.
func Create[T store.Resource](ctx context.Context, s store.ResourceStore[T], obj T) error {
	if s == nil {
		return nil
	}
	if err := s.Create(ctx, obj); err != nil {
		logging.Z().Error("store create failed",
			zap.String("resource", obj.GetKey()),
			zap.String("kind", obj.GetTypeMeta().Kind),
			zap.Error(err),
		)
		return fmt.Errorf("store create %s/%s: %w", obj.GetTypeMeta().Kind, obj.GetKey(), err)
	}
	return nil
}

// Delete removes a resource, logging any error.
// Returns nil if the store is nil.
func Delete[T store.Resource](ctx context.Context, s store.ResourceStore[T], key string) error {
	if s == nil {
		return nil
	}
	if err := s.Delete(ctx, key); err != nil {
		logging.Z().Error("store delete failed",
			zap.String("key", key),
			zap.Error(err),
		)
		return fmt.Errorf("store delete %s: %w", key, err)
	}
	return nil
}
