package encryption

// =====================================================
// P2 resource-ification -- Encryption.
//
// Canonical type definitions live in models/.
// Type aliases here maintain backward compatibility.
// =====================================================

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/encryption/models"
)

const (
	EncryptionKeyKind          = models.EncryptionKeyKind
	EncryptionKeyAPIVersion    = models.EncryptionKeyAPIVersion
	EncryptionPolicyKind       = models.EncryptionPolicyKind
	EncryptionPolicyAPIVersion = models.EncryptionPolicyAPIVersion
)

// --- Type aliases (canonical definitions in models/) ---

type EncryptionKeySpec = models.EncryptionKeySpec
type EncryptionKeyResourceStatus = models.EncryptionKeyResourceStatus
type EncryptionKeyResource = models.EncryptionKeyResource
type EncryptionPolicySpec = models.EncryptionPolicySpec
type EncryptionPolicyResourceStatus = models.EncryptionPolicyResourceStatus
type EncryptionPolicyResource = models.EncryptionPolicyResource
