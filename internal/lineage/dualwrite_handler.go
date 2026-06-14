package lineage

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

// LineageDualWriteStore is set when dual-write is enabled.
// Lineage handlers are currently read-only; this store is used by
// the reconciler and will be used by future write endpoints.
type LineageDualWriteStore = store.ResourceStore[*LineageNodeResource]

// SetDualWriteStore attaches an etcd store for dual-write.
func (h *LineageHandler) SetDualWriteStore(s LineageDualWriteStore) { h.dualWriteStore = s }
