// Package dualwrite provides a helper for Phase 2 of the migration plan.
// Handlers call DualWrite to asynchronously write a resource to etcd
// alongside the existing imperative manager call. The imperative path
// remains authoritative; the etcd write is best-effort.
package dualwrite

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/featureflags"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

// Write asynchronously creates or updates a resource in the etcd store
// if dual-write is enabled for the given module. Non-blocking — fires
// a goroutine and returns immediately. Errors are logged, not returned.
func Write[T store.Resource](module string, s store.ResourceStore[T], obj T) {
	if !featureflags.DualWriteEnabled(module) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Try create first; if conflict (already exists), update.
		err := s.Create(ctx, obj)
		if err != nil {
			err = s.Update(ctx, obj)
		}
		if err != nil {
			logging.Z().Info(fmt.Sprintf("dualwrite module=%s key=%s err=%q", module, obj.GetKey(), err.Error()))
		}
	}()
}
