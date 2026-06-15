// Package source — Channel source for external events.
//
// The channel source is typically wired to:
//   - Webhook receivers that need to trigger a reconcile for a
//     specific namespace/name when an external system notifies us.
//   - Cron-style periodic sweeps where the controller wants to
//     revisit every object on a schedule regardless of informer
//     activity.
//
// Channel events carry a predicate.Generic kind so handlers can tell
// them apart from cache-backed Create/Update/Delete events.
package source

import (
	"context"
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/controller/handler"
	"axiomnizam.bitbd.net/axiomnizam/internal/controller/predicate"
)

// Channel is a Source that forwards values from a caller-owned chan
// into the controller pipeline.  The channel must stay open for the
// life of the controller; closing it halts the source.
type Channel struct {
	// Source is the upstream event stream.  Closing it stops Start.
	Source <-chan Event
}

// Start spawns a goroutine that drains Source until ctx is cancelled
// or the channel closes.  Returns an error if Source is nil.
func (c *Channel) Start(ctx context.Context, h handler.EventHandler, q handler.Queue, preds ...predicate.Predicate) error {
	if c.Source == nil {
		return fmt.Errorf("source.Channel: Source chan is nil")
	}
	if h == nil {
		return fmt.Errorf("source.Channel: handler is nil")
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-c.Source:
				if !ok {
					return
				}
				// Normalise: Channel events are always Generic
				// regardless of what the caller set, so handlers can
				// rely on the kind.
				evt.Kind = predicate.Generic
				dispatch(h, q, preds, evt)
			}
		}
	}()
	return nil
}
