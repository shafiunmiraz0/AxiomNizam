package repositories

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
)

// BucketRepository defines CRUD operations for bucket resources.
// Implemented by store.BucketStore.
type BucketRepository interface {
	// Create inserts a new bucket resource. Returns an error if it already exists.
	Create(b *models.BucketResource) error

	// Get retrieves a bucket resource by tenant and name.
	Get(tenantID, name string) (*models.BucketResource, error)

	// Update replaces a bucket resource. Increments generation.
	Update(b *models.BucketResource) error

	// UpdateStatus updates only the status of a bucket resource.
	UpdateStatus(tenantID, name string, status models.BucketStatus) error

	// Delete removes a bucket resource.
	Delete(tenantID, name string) error

	// List returns all bucket resources, optionally filtered by tenant.
	List(tenantID string) []*models.BucketResource

	// ListAll returns all bucket resources across all tenants.
	ListAll() []*models.BucketResource
}
