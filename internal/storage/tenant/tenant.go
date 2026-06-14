package tenant

import (
	"fmt"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/store"
)

// Manager enforces multi-tenancy for object storage.
// Each tenant gets isolated buckets using a naming convention.
type Manager struct {
	prefix string // e.g., "axiom-"
	store  *store.BucketStore
}

// NewManager creates a tenant manager with the given prefix.
func NewManager(prefix string, s *store.BucketStore) *Manager {
	return &Manager{
		prefix: prefix,
		store:  s,
	}
}

// ResolveBucketName produces the storage-level bucket name for a tenant.
// Format: {prefix}{tenantID}-{bucketName}
func (tm *Manager) ResolveBucketName(tenantID, bucketName string) string {
	return store.TenantBucketName(tm.prefix, tenantID, bucketName)
}

// ValidateBucketAccess checks whether a tenant is allowed to access a given
// storage-level bucket name. This prevents cross-tenant access.
func (tm *Manager) ValidateBucketAccess(tenantID, storageBucket string) error {
	expected := strings.ToLower(fmt.Sprintf("%s%s-", tm.prefix, tenantID))
	if !strings.HasPrefix(strings.ToLower(storageBucket), expected) {
		return fmt.Errorf("storage: tenant %q is not authorized to access bucket %q", tenantID, storageBucket)
	}
	return nil
}

// ListTenantBuckets returns all buckets belonging to a specific tenant.
func (tm *Manager) ListTenantBuckets(tenantID string) []*models.BucketResource {
	return tm.store.List(tenantID)
}

// CreateTenantBucket registers a new bucket for a tenant.
func (tm *Manager) CreateTenantBucket(tenantID, name string, spec models.BucketSpec) (*models.BucketResource, error) {
	storageName := tm.ResolveBucketName(tenantID, name)
	spec.Name = storageName

	bucket := &models.BucketResource{
		Metadata: models.BucketMetadata{
			Name:     name,
			TenantID: tenantID,
		},
		Spec: spec,
	}

	if err := tm.store.Create(bucket); err != nil {
		return nil, err
	}
	return bucket, nil
}

// DeleteTenantBucket removes a bucket registration for a tenant.
func (tm *Manager) DeleteTenantBucket(tenantID, name string) error {
	return tm.store.Delete(tenantID, name)
}
