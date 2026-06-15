// Package client canonical reconciler bridge (P0.1).
package client

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

func (r *DefaultReconciler) AsReconciler() reconciler.Reconciler {
	return reconciler.ReconcilerFunc(func(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
		var kind, namespace, name string
		if rr, ok := obj.(resources.Resource); ok {
			if tm := rr.GetTypeMeta(); tm != nil { kind = tm.Kind }
			if om := rr.GetObjectMeta(); om != nil { namespace = om.Namespace; name = om.Name }
		}
		if name == "" {
			key := obj.GetKey()
			for i := 0; i < len(key); i++ {
				if key[i] == '/' { namespace = key[:i]; name = key[i+1:]; break }
			}
			if name == "" { name = key }
		}
		res := r.Reconcile(ctx, kind, namespace, name)
		return reconciler.ReconcileResult{ Requeue: res.Error != nil || res.Phase == "Failed", Error: res.Error }
	})
}
