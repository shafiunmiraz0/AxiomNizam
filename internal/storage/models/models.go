package models

import (
	"time"
)

// BucketPhase represents the lifecycle phase of a bucket.
type BucketPhase string

const (
	BucketPhasePending  BucketPhase = "Pending"
	BucketPhaseReady    BucketPhase = "Ready"
	BucketPhaseError    BucketPhase = "Error"
	BucketPhaseLocked   BucketPhase = "Locked"
	BucketPhaseDeleting BucketPhase = "Deleting"
)

// VersioningStatus represents the versioning state.
type VersioningStatus string

const (
	VersioningEnabled   VersioningStatus = "Enabled"
	VersioningDisabled  VersioningStatus = "Disabled"
	VersioningSuspended VersioningStatus = "Suspended"
)

// Condition represents a status condition on a resource.
type Condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"` // "True", "False", "Unknown"
	Reason             string    `json:"reason"`
	Message            string    `json:"message"`
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}

// LifecycleRule defines an object lifecycle policy.
type LifecycleRule struct {
	ID                        string `json:"id"`
	Prefix                    string `json:"prefix"`
	ExpirationDays            int    `json:"expirationDays,omitempty"`
	NoncurrentExpirationDays  int    `json:"noncurrentExpirationDays,omitempty"`
	TransitionDays            int    `json:"transitionDays,omitempty"`
	TransitionStorageClass    string `json:"transitionStorageClass,omitempty"`
	AbortIncompleteUploadDays int    `json:"abortIncompleteUploadDays,omitempty"`
	DeleteMarkerExpiration    bool   `json:"deleteMarkerExpiration,omitempty"`
	Enabled                   bool   `json:"enabled"`
}

// BucketMetadata holds identifying information for a bucket.
type BucketMetadata struct {
	Name        string            `json:"name"`
	TenantID    string            `json:"tenantId"`
	UID         string            `json:"uid,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	CreatedBy   string            `json:"createdBy,omitempty"` // IAM user ID of creator
}

// ObjectLockConfig defines the object lock settings for a bucket (WORM).
type ObjectLockConfig struct {
	Enabled        bool   `json:"enabled"`
	Mode           string `json:"mode,omitempty"` // "GOVERNANCE" or "COMPLIANCE"
	RetentionDays  int    `json:"retentionDays,omitempty"`
	RetentionYears int    `json:"retentionYears,omitempty"`
}

// BucketEncryption defines server-side encryption settings.
type BucketEncryption struct {
	Enabled   bool   `json:"enabled"`
	Algorithm string `json:"algorithm"` // "AES256" or "aws:kms"
	KMSKeyID  string `json:"kmsKeyId,omitempty"`
}

// BucketNotificationConfig defines event notification rules for a bucket.
type BucketNotificationConfig struct {
	Rules []NotificationRule `json:"rules,omitempty"`
}

// NotificationRule defines a single notification rule.
type NotificationRule struct {
	ID     string   `json:"id"`
	Events []string `json:"events"` // e.g., "s3:ObjectCreated:*", "s3:ObjectRemoved:*"
	Prefix string   `json:"prefix,omitempty"`
	Suffix string   `json:"suffix,omitempty"`
	Target string   `json:"target"` // "webhook", "eventbus"
	URL    string   `json:"url,omitempty"`
}

// BucketSpec defines the desired state of a bucket.
type BucketSpec struct {
	Name              string                   `json:"name"`
	Versioning        VersioningStatus         `json:"versioning"`
	LifecyclePolicy   []LifecycleRule          `json:"lifecyclePolicy,omitempty"`
	Region            string                   `json:"region,omitempty"`
	Quota             int64                    `json:"quota,omitempty"`             // bytes, 0 = unlimited
	ReadOpsPerMinute  int                      `json:"readOpsPerMinute,omitempty"`  // object read ops/min, 0 = use system default
	WriteOpsPerMinute int                      `json:"writeOpsPerMinute,omitempty"` // object write ops/min, 0 = use system default
	Encryption        BucketEncryption         `json:"encryption,omitempty"`
	ObjectLock        ObjectLockConfig         `json:"objectLock,omitempty"`
	Notifications     BucketNotificationConfig `json:"notifications,omitempty"`
	Public            bool                     `json:"public,omitempty"` // publicly readable
}

// BucketStatus represents the observed state of a bucket.
type BucketStatus struct {
	Phase              BucketPhase `json:"phase"`
	Endpoint           string      `json:"endpoint,omitempty"`
	Conditions         []Condition `json:"conditions,omitempty"`
	ObjectCount        int64       `json:"objectCount"`
	TotalSize          int64       `json:"totalSize"` // bytes
	ObservedGeneration int64       `json:"observedGeneration"`
	EncryptionActive   bool        `json:"encryptionActive,omitempty"`
	LockActive         bool        `json:"lockActive,omitempty"`
	VersioningActive   bool        `json:"versioningActive,omitempty"`
}

// BucketResource is the CRD-style resource for object storage buckets.
type BucketResource struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   BucketMetadata `json:"metadata"`
	Spec       BucketSpec     `json:"spec"`
	Status     BucketStatus   `json:"status"`
	Generation int64          `json:"generation"`
}

// ObjectInfo contains metadata about a stored object.
type ObjectInfo struct {
	Key            string            `json:"key"`
	Size           int64             `json:"size"`
	ContentType    string            `json:"contentType"`
	ETag           string            `json:"etag"`
	LastModified   time.Time         `json:"lastModified"`
	VersionID      string            `json:"versionId,omitempty"`
	IsDeleteMarker bool              `json:"isDeleteMarker,omitempty"`
	StorageClass   string            `json:"storageClass,omitempty"`
	UserMetadata   map[string]string `json:"userMetadata,omitempty"`
	RetainUntil    *time.Time        `json:"retainUntil,omitempty"`
	LegalHold      bool              `json:"legalHold,omitempty"`
}

// StorageStats holds aggregate storage statistics.
type StorageStats struct {
	TotalBuckets   int   `json:"totalBuckets"`
	TotalObjects   int64 `json:"totalObjects"`
	TotalSizeBytes int64 `json:"totalSizeBytes"`
	TenantCount    int   `json:"tenantCount"`
}

// PreSignedURLResponse contains the generated pre-signed URL.
type PreSignedURLResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
	Method    string    `json:"method"`
	Bucket    string    `json:"bucket"`
	Key       string    `json:"key"`
}

// S3Action represents a permitted S3 operation.
type S3Action string

const (
	S3ActionGetObject           S3Action = "s3:GetObject"
	S3ActionPutObject           S3Action = "s3:PutObject"
	S3ActionDeleteObject        S3Action = "s3:DeleteObject"
	S3ActionListBucket          S3Action = "s3:ListBucket"
	S3ActionGetBucketLoc        S3Action = "s3:GetBucketLocation"
	S3ActionCreateBucket        S3Action = "s3:CreateBucket"
	S3ActionDeleteBucket        S3Action = "s3:DeleteBucket"
	S3ActionGetBucketPolicy     S3Action = "s3:GetBucketPolicy"
	S3ActionPutBucketPolicy     S3Action = "s3:PutBucketPolicy"
	S3ActionGetBucketTagging    S3Action = "s3:GetBucketTagging"
	S3ActionPutBucketTagging    S3Action = "s3:PutBucketTagging"
	S3ActionPutBucketVersioning S3Action = "s3:PutBucketVersioning"
	S3ActionGetBucketVersioning S3Action = "s3:GetBucketVersioning"
	S3ActionPutObjectRetention  S3Action = "s3:PutObjectRetention"
	S3ActionGetObjectRetention  S3Action = "s3:GetObjectRetention"
	S3ActionBypassGovernance    S3Action = "s3:BypassGovernanceRetention"
	S3ActionAll                 S3Action = "s3:*"
)

// S3PolicyStatement is a single statement in an S3 bucket policy.
type S3PolicyStatement struct {
	Sid       string                       `json:"Sid"`
	Effect    string                       `json:"Effect"` // "Allow" or "Deny"
	Principal string                       `json:"Principal"`
	Action    []S3Action                   `json:"Action"`
	Resource  []string                     `json:"Resource"`
	Condition map[string]map[string]string `json:"Condition,omitempty"`
}

// S3BucketPolicy is a complete S3 bucket policy document.
type S3BucketPolicy struct {
	Version   string              `json:"Version"`
	Statement []S3PolicyStatement `json:"Statement"`
}

// StorageRole defines a role that maps to S3 permissions.
type StorageRole string

const (
	StorageRoleAdmin    StorageRole = "storage-admin"    // full access
	StorageRoleWriter   StorageRole = "storage-writer"   // read + write
	StorageRoleReader   StorageRole = "storage-reader"   // read only
	StorageRoleUploader StorageRole = "storage-uploader" // write only
	StorageRoleBrowser  StorageRole = "storage-browser"  // list + read
)

// TenantPolicy tracks the IAM-to-storage policy mapping for a tenant.
type TenantPolicy struct {
	TenantID   string      `json:"tenantId"`
	UserID     string      `json:"userId"`
	Role       StorageRole `json:"role"`
	BucketName string      `json:"bucketName"`
	Prefix     string      `json:"prefix,omitempty"` // optional path restriction
	PolicyJSON string      `json:"policyJson"`
	ExpiresAt  *time.Time  `json:"expiresAt,omitempty"` // time-bound access
	GrantedBy  string      `json:"grantedBy,omitempty"` // IAM user who granted
	GrantedAt  time.Time   `json:"grantedAt"`
}

// ===== Access Keys (Service Account / Application Binding) =====

// AccessKey is a storage-specific credential bound to a user and scope.
// Follows MinIO's service account pattern for application integration.
type AccessKey struct {
	AccessKeyID     string      `json:"accessKeyId" classification:"Sensitive"`
	SecretAccessKey string      `json:"secretAccessKey,omitempty" classification:"Confidential"` // only returned on creation
	Name            string      `json:"name"`
	Description     string      `json:"description,omitempty"`
	UserID          string      `json:"userId"` // owning IAM user
	TenantID        string      `json:"tenantId"`
	Role            StorageRole `json:"role"`
	BucketScope     []string    `json:"bucketScope,omitempty"` // empty = all buckets
	PrefixScope     string      `json:"prefixScope,omitempty"` // restrict to key prefix
	Active          bool        `json:"active"`
	ExpiresAt       *time.Time  `json:"expiresAt,omitempty"`
	CreatedAt       time.Time   `json:"createdAt"`
	LastUsedAt      *time.Time  `json:"lastUsedAt,omitempty"`
	LastUsedIP      string      `json:"lastUsedIp,omitempty"`
}

// ===== Bucket Sharing =====

// BucketShare grants access to a bucket for an application or external user.
type BucketShare struct {
	ID           string      `json:"id"`
	BucketName   string      `json:"bucketName"`
	TenantID     string      `json:"tenantId"`
	GranteeType  string      `json:"granteeType"` // "user", "application", "service-account", "public-link"
	GranteeID    string      `json:"granteeId"`   // IAM user ID, app name, or access key ID
	GranteeName  string      `json:"granteeName,omitempty"`
	Role         StorageRole `json:"role"`
	Prefix       string      `json:"prefix,omitempty"` // restrict to key prefix
	ExpiresAt    *time.Time  `json:"expiresAt,omitempty"`
	SharedBy     string      `json:"sharedBy"` // IAM user who shared
	SharedAt     time.Time   `json:"sharedAt"`
	Active       bool        `json:"active"`
	AccessCount  int64       `json:"accessCount"`
	LastAccessed *time.Time  `json:"lastAccessed,omitempty"`
}

// ===== Enhanced Features =====

// BucketTag is a key-value tag on a bucket.
type BucketTag struct {
	Key   string `json:"key" xml:"Key"`
	Value string `json:"value" xml:"Value"`
}

// CORSRule defines a CORS rule for a bucket.
type CORSRule struct {
	AllowedOrigins []string `json:"allowedOrigins" xml:"AllowedOrigin"`
	AllowedMethods []string `json:"allowedMethods" xml:"AllowedMethod"`
	AllowedHeaders []string `json:"allowedHeaders,omitempty" xml:"AllowedHeader"`
	ExposeHeaders  []string `json:"exposeHeaders,omitempty" xml:"ExposeHeader"`
	MaxAgeSeconds  int      `json:"maxAgeSeconds,omitempty" xml:"MaxAgeSeconds"`
}

// ReplicationRule defines bucket-to-bucket replication.
type ReplicationRule struct {
	ID                string `json:"id"`
	DestinationBucket string `json:"destinationBucket"`
	Prefix            string `json:"prefix,omitempty"`
	Enabled           bool   `json:"enabled"`
}

// StorageEvent represents a storage operation event for audit.
type StorageEvent struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "bucket.created", "object.uploaded", "object.deleted", etc.
	TenantID  string    `json:"tenantId"`
	UserID    string    `json:"userId,omitempty"`
	Bucket    string    `json:"bucket"`
	Key       string    `json:"key,omitempty"`
	Size      int64     `json:"size,omitempty"`
	Details   string    `json:"details,omitempty"`
	SourceIP  string    `json:"sourceIp,omitempty"`
}

// BucketMetrics holds per-bucket performance metrics.
type BucketMetrics struct {
	BucketName     string    `json:"bucketName"`
	TenantID       string    `json:"tenantId"`
	RequestCount   int64     `json:"requestCount"`
	GetRequests    int64     `json:"getRequests"`
	PutRequests    int64     `json:"putRequests"`
	DeleteRequests int64     `json:"deleteRequests"`
	BytesIn        int64     `json:"bytesIn"`
	BytesOut       int64     `json:"bytesOut"`
	ErrorCount     int64     `json:"errorCount"`
	AvgLatencyMs   float64   `json:"avgLatencyMs"`
	LastAccessed   time.Time `json:"lastAccessed"`
	CollectedAt    time.Time `json:"collectedAt"`
}

// SystemMetrics holds aggregate storage system metrics.
type SystemMetrics struct {
	Uptime           string  `json:"uptime"`
	TotalBuckets     int     `json:"totalBuckets"`
	TotalObjects     int64   `json:"totalObjects"`
	TotalSizeBytes   int64   `json:"totalSizeBytes"`
	TenantCount      int     `json:"tenantCount"`
	TotalRequests    int64   `json:"totalRequests"`
	TotalBytesIn     int64   `json:"totalBytesIn"`
	TotalBytesOut    int64   `json:"totalBytesOut"`
	TotalErrors      int64   `json:"totalErrors"`
	ActivePolicies   int     `json:"activePolicies"`
	ActiveAccessKeys int     `json:"activeAccessKeys"`
	ActiveShares     int     `json:"activeShares"`
	BackendHealthy   bool    `json:"backendHealthy"`
	AvgLatencyMs     float64 `json:"avgLatencyMs"`
}

// MultiDeleteRequest represents a batch delete request.
type MultiDeleteRequest struct {
	Objects []DeleteObjectEntry `json:"objects"`
}

// DeleteObjectEntry is a single object key in a batch delete.
type DeleteObjectEntry struct {
	Key       string `json:"key" xml:"Key"`
	VersionID string `json:"versionId,omitempty" xml:"VersionId"`
}

// CopyObjectRequest represents a server-side copy operation.
type CopyObjectRequest struct {
	SourceBucket string `json:"sourceBucket"`
	SourceKey    string `json:"sourceKey"`
	DestBucket   string `json:"destBucket"`
	DestKey      string `json:"destKey"`
}

// ObjectVersion represents a specific version of an object.
type ObjectVersion struct {
	Key          string    `json:"key"`
	VersionID    string    `json:"versionId"`
	IsLatest     bool      `json:"isLatest"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"lastModified"`
	ETag         string    `json:"etag"`
}

// BucketNotification configures event notifications for a bucket.
type BucketNotification struct {
	ID     string   `json:"id"`
	Events []string `json:"events"` // e.g., "s3:ObjectCreated:*", "s3:ObjectRemoved:*"
	Prefix string   `json:"prefix,omitempty"`
	Suffix string   `json:"suffix,omitempty"`
}

// ===== Quota =====

// QuotaInfo provides quota usage details for a bucket.
type QuotaInfo struct {
	Bucket      string  `json:"bucket"`
	TenantID    string  `json:"tenantId"`
	QuotaBytes  int64   `json:"quotaBytes"` // 0 = unlimited
	UsedBytes   int64   `json:"usedBytes"`
	UsedPct     float64 `json:"usedPct"`
	ObjectCount int64   `json:"objectCount"`
}

// BucketRateLimitInfo provides object operation rate-limit settings for a bucket.
type BucketRateLimitInfo struct {
	Bucket                     string `json:"bucket"`
	TenantID                   string `json:"tenantId"`
	ReadOpsPerMinute           int    `json:"readOpsPerMinute"`
	WriteOpsPerMinute          int    `json:"writeOpsPerMinute"`
	EffectiveReadOpsPerMinute  int    `json:"effectiveReadOpsPerMinute"`
	EffectiveWriteOpsPerMinute int    `json:"effectiveWriteOpsPerMinute"`
}
