package versioning

// Phase 2/3 dual-write and authoritative-mode extensions for VersionHandler.

import (
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/dualwrite"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/featureflags"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const dualWriteModule = "versioning"

// DualWriteStore is set on the handler when dual-write is enabled.
type DualWriteStore = store.ResourceStore[*VersionPolicyResource]

// SetDualWriteStore attaches an etcd store for dual-write.
func (h *VersionHandler) SetDualWriteStore(s DualWriteStore) {
	h.dualWriteStore = s
}

// isAuthoritative returns true when the reconciler is authoritative for this module.
func (h *VersionHandler) isAuthoritative() bool {
	return h.dualWriteStore != nil && featureflags.ReconcilerAuthoritative(dualWriteModule)
}

// buildSnapshotResource creates a VersionPolicyResource from request params.
func (h *VersionHandler) buildSnapshotResource(resourceType, resourceID, name, description string, tags []string) *VersionPolicyResource {
	resName := fmt.Sprintf("%s-%s-%s", resourceType, resourceID, name)
	return &VersionPolicyResource{
		TypeMeta: resources.TypeMeta{
			APIVersion: VersionPolicyAPIVersion,
			Kind:       VersionPolicyKind,
		},
		ObjectMeta: resources.ObjectMeta{
			Name:       resName,
			Namespace:  "default",
			Generation: 1,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Labels: map[string]string{
				"resourceType": resourceType,
				"resourceId":   resourceID,
				"snapshot":     name,
			},
		},
		Spec: VersionPolicySpec{
			ResourceType:    resourceType,
			AutoSnapshot:    true,
			RollbackEnabled: true,
			DiffEnabled:     true,
			Enabled:         true,
		},
		Status: VersionPolicyResourceStatus{
			ObjectStatus: resources.ObjectStatus{
				Phase: "Pending",
			},
			PolicyActive: false,
		},
	}
}

// dualWriteSnapshot writes a VersionPolicyResource to etcd after
// a snapshot is created via the imperative manager (Phase 2).
func (h *VersionHandler) dualWriteSnapshot(snapshot *Snapshot) {
	if h.dualWriteStore == nil || snapshot == nil {
		return
	}
	resource := h.buildSnapshotResource(snapshot.ResourceType, snapshot.ResourceID, snapshot.Name, snapshot.Description, snapshot.Tags)
	resource.Status.Phase = "Active"
	resource.Status.PolicyActive = true
	dualwrite.Write(dualWriteModule, h.dualWriteStore, resource)
}
