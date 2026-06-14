package encryption

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/dualwrite"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/featureflags"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const encryptionDWModule = "encryption"

type EncryptionKeyDualWriteStore = store.ResourceStore[*EncryptionKeyResource]

func (h *EncryptionHandler) SetKeyDualWriteStore(s EncryptionKeyDualWriteStore) { h.keyDualWriteStore = s }

func (h *EncryptionHandler) isAuthoritative() bool {
	return h.keyDualWriteStore != nil && featureflags.ReconcilerAuthoritative(encryptionDWModule)
}

func (h *EncryptionHandler) dualWriteKey(key *EncryptionKey) {
	if h.keyDualWriteStore == nil || key == nil {
		return
	}
	resource := &EncryptionKeyResource{
		TypeMeta:   resources.TypeMeta{APIVersion: EncryptionKeyAPIVersion, Kind: EncryptionKeyKind},
		ObjectMeta: resources.ObjectMeta{Name: key.ID, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		Spec: EncryptionKeySpec{TenantID: key.TenantID, Description: key.Description, KeyType: key.KeyType, Algorithm: key.Algorithm, KeyLength: key.KeyLength, IsDefault: key.IsDefault, Owner: key.Owner, Tags: key.Tags},
		Status: EncryptionKeyResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: string(key.Status)}, KeyStatus: key.Status, Version: key.Version},
	}
	dualwrite.Write(encryptionDWModule, h.keyDualWriteStore, resource)
}
