package repositories

// Compile-time interface satisfaction checks.
// These ensure concrete types implement the repository interfaces.

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/store"
)

var _ BucketRepository = (*store.BucketStore)(nil)
