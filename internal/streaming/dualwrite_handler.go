package streaming

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

const streamingDWModule = "streaming"

type StreamingDualWriteStore = store.ResourceStore[*StreamResource]

func (h *StreamHandler) SetDualWriteStore(s StreamingDualWriteStore) { h.dualWriteStore = s }
