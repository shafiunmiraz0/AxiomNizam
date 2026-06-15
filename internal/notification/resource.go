package notification

// Domain types moved to models/resource.go.
// Type aliases in types.go re-export them for backward compatibility.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
)

type ChannelReconciler struct {
	store store.ResourceStore[*ChannelResource]
}

func NewChannelReconciler(rs store.ResourceStore[*ChannelResource]) *ChannelReconciler {
	return &ChannelReconciler{store: rs}
}

func (rec *ChannelReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*ChannelResource)
	if !ok {
		return reconciler.ReconcileResult{Error: channelErr("notification: wrong type")}
	}
	now := time.Now()
	phase := "Disabled"
	if res.Spec.Enabled {
		phase = "Active"
	}
	res.Status.Phase = phase
	res.Status.ChannelActive = res.Spec.Enabled
	res.Status.ObservedGeneration = res.Generation
	res.Status.LastTransitionTime = now
	if rec.store != nil {
		_ = rec.store.Update(ctx, res)
	}
	return reconciler.ReconcileResult{}
}

type channelErr string

func (e channelErr) Error() string { return string(e) }
