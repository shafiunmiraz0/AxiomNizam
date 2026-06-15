package testutil

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
)

// TestTenantID is a fixed tenant ID for testing.
const TestTenantID = "test-tenant"

// TestBucketName is a fixed bucket name for testing.
const TestBucketName = "test-bucket"

// TestObjectKey is a fixed object key for testing.
const TestObjectKey = "test-object.txt"

// NewTestBucket creates a test BucketResource with sensible defaults.
func NewTestBucket() *models.BucketResource {
	now := time.Now().UTC()
	return &models.BucketResource{
		APIVersion: "v1",
		Kind:       "Bucket",
		Metadata: models.BucketMetadata{
			Name:      TestBucketName,
			TenantID:  TestTenantID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Spec: models.BucketSpec{
			Name:       TestBucketName,
			Region:     "us-east-1",
			Versioning: models.VersioningDisabled,
		},
		Status: models.BucketStatus{
			Phase:    models.BucketPhaseReady,
			Endpoint: "http://localhost:8000",
		},
	}
}

// NewTestBucketWithObjects creates a test bucket with object count set.
func NewTestBucketWithObjects(count int, size int64) *models.BucketResource {
	b := NewTestBucket()
	b.Status.ObjectCount = int64(count)
	b.Status.TotalSize = size
	return b
}
