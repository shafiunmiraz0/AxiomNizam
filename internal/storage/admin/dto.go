package admin

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
)

// --- Bucket DTOs ---

// CreateBucketRequest is the API request for creating a bucket.
type CreateBucketRequest struct {
	Name        string                    `json:"name" binding:"required"`
	Versioning  models.VersioningStatus   `json:"versioning"`
	Tags        []models.BucketTag        `json:"tags"`
	Encryption  *models.BucketEncryption  `json:"encryption"`
	Quota       *models.QuotaInfo         `json:"quota"`
	Labels      map[string]string         `json:"labels"`
}

// BucketResponse is the API response for a bucket.
type BucketResponse struct {
	Name           string                  `json:"name"`
	TenantID       string                  `json:"tenant_id"`
	Region         string                  `json:"region"`
	Phase          models.BucketPhase      `json:"phase"`
	Versioning     models.VersioningStatus `json:"versioning"`
	ObjectCount    int64                   `json:"object_count"`
	TotalSize      int64                   `json:"total_size"`
	Labels         map[string]string       `json:"labels,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

// ListBucketsResponse is the API response for listing buckets.
type ListBucketsResponse struct {
	Buckets []BucketResponse `json:"buckets"`
}

// --- Object DTOs ---

// ObjectResponse is the API response for an object.
type ObjectResponse struct {
	Key          string            `json:"key"`
	Size         int64             `json:"size"`
	ETag         string            `json:"etag"`
	ContentType  string            `json:"content_type"`
	StorageClass string            `json:"storage_class"`
	LastModified time.Time         `json:"last_modified"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// ListObjectsResponse is the API response for listing objects.
type ListObjectsResponse struct {
	Objects  []ObjectResponse `json:"objects"`
	Prefix   string           `json:"prefix,omitempty"`
	Count    int              `json:"count"`
}

// PutObjectMetadataRequest is the API request for updating object metadata.
type PutObjectMetadataRequest struct {
	Metadata map[string]string `json:"metadata" binding:"required"`
}

// --- Presign DTOs ---

// PresignURLRequest is the API request for generating a presigned URL.
type PresignURLRequest struct {
	Key         string `json:"key" binding:"required"`
	Method      string `json:"method" binding:"required"`
	Expires     int    `json:"expires"`
	AccessKeyID string `json:"access_key_id"`
}

// PresignURLResponse is the API response for a presigned URL.
type PresignURLResponse struct {
	URL        string    `json:"url"`
	Method     string    `json:"method"`
	Key        string    `json:"key"`
	Expires    int       `json:"expires"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// --- Access Key DTOs ---

// CreateAccessKeyRequest is the API request for creating an access key.
type CreateAccessKeyRequest struct {
	Name        string             `json:"name" binding:"required"`
	Description string             `json:"description"`
	Role        models.StorageRole `json:"role" binding:"required"`
}

// AccessKeyResponse is the API response for an access key.
type AccessKeyResponse struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Role        models.StorageRole `json:"role"`
	KeyPreview  string             `json:"key_preview"`
	CreatedAt   time.Time          `json:"created_at"`
}

// --- Share DTOs ---

// CreateBucketShareRequest is the API request for creating a bucket share.
type CreateBucketShareRequest struct {
	GranteeType string             `json:"grantee_type" binding:"required"`
	GranteeID   string             `json:"grantee_id" binding:"required"`
	Role        models.StorageRole `json:"role" binding:"required"`
	ExpiresAt   *time.Time         `json:"expires_at"`
}

// BucketShareResponse is the API response for a bucket share.
type BucketShareResponse struct {
	ID          string             `json:"id"`
	Bucket      string             `json:"bucket"`
	GranteeType string             `json:"grantee_type"`
	GranteeID   string             `json:"grantee_id"`
	Role        models.StorageRole `json:"role"`
	CreatedAt   time.Time          `json:"created_at"`
	ExpiresAt   *time.Time         `json:"expires_at,omitempty"`
}

// --- Rate Limit DTOs ---

// SetBucketRateLimitRequest is the API request for setting bucket rate limits.
type SetBucketRateLimitRequest struct {
	ReadOpsPerMinute  int `json:"read_ops_per_minute"`
	WriteOpsPerMinute int `json:"write_ops_per_minute"`
}

// BucketRateLimitResponse is the API response for bucket rate limits.
type BucketRateLimitResponse struct {
	Bucket           string `json:"bucket"`
	ReadOpsPerMinute int    `json:"read_ops_per_minute"`
	WriteOpsPerMinute int   `json:"write_ops_per_minute"`
}

// --- Policy DTOs ---

// CreatePolicyRequest is the API request for creating an access policy.
type CreatePolicyRequest struct {
	TenantID string             `json:"tenant_id" binding:"required"`
	UserID   string             `json:"user_id" binding:"required"`
	Bucket   string             `json:"bucket" binding:"required"`
	Role     models.StorageRole `json:"role" binding:"required"`
}

// --- Event DTOs ---

// EventResponse is the API response for an audit event.
type EventResponse struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Bucket    string    `json:"bucket"`
	Key       string    `json:"key,omitempty"`
	UserID    string    `json:"user_id,omitempty"`
	SourceIP  string    `json:"source_ip,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
