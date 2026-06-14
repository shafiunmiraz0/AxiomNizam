package models

// =====================================================
// Domain resource types for the Export module.
//
// Moved from the parent package to provide a clean
// models/ sub-package that other modules can import
// without pulling in the full export implementation.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	ExportJobKind       = "ExportJob"
	ExportJobAPIVersion = "export.axiomnizam.io/v1"
)

// --- Dependent types (needed by the resource structs) ---

// ExportStatus represents export state
type ExportStatus string

const (
	ExportPending     ExportStatus = "PENDING"
	ExportValidating  ExportStatus = "VALIDATING"
	ExportQueued      ExportStatus = "QUEUED"
	ExportRunning     ExportStatus = "RUNNING"
	ExportProcessing  ExportStatus = "PROCESSING"
	ExportCompressing ExportStatus = "COMPRESSING"
	ExportEncrypting  ExportStatus = "ENCRYPTING"
	ExportUploading   ExportStatus = "UPLOADING"
	ExportCompleted   ExportStatus = "COMPLETED"
	ExportFailed      ExportStatus = "FAILED"
	ExportCancelled   ExportStatus = "CANCELLED"
	ExportPartial     ExportStatus = "PARTIAL"
)

// ExportFormat defines output format
type ExportFormat string

const (
	FormatJSON     ExportFormat = "JSON"
	FormatCSV      ExportFormat = "CSV"
	FormatXML      ExportFormat = "XML"
	FormatParquet  ExportFormat = "PARQUET"
	FormatAvro     ExportFormat = "AVRO"
	FormatProtobuf ExportFormat = "PROTOBUF"
	FormatExcel    ExportFormat = "EXCEL"
	FormatPDF      ExportFormat = "PDF"
	FormatSQL      ExportFormat = "SQL"
	FormatNDJSON   ExportFormat = "NDJSON"
)

// ExportSource defines what to export
type ExportSource struct {
	Type           string `json:"type"` // "resource", "query", "database", "table"
	ResourceType   string `json:"resourceType,omitempty"`
	ResourceID     string `json:"resourceId,omitempty"`
	Database       string `json:"database,omitempty"`
	Table          string `json:"table,omitempty"`
	Query          string `json:"query,omitempty"`
	IncludeRelated bool   `json:"includeRelated"`
	IncludeAudit   bool   `json:"includeAudit"`
	IncludeHistory bool   `json:"includeHistory"`
}

// CompressionType for export compression
type CompressionType string

const (
	CompressionNone   CompressionType = "NONE"
	CompressionGzip   CompressionType = "GZIP"
	CompressionBrotli CompressionType = "BROTLI"
	CompressionLZ4    CompressionType = "LZ4"
	CompressionZstd   CompressionType = "ZSTD"
)

// EncryptionConfig for export encryption
type EncryptionConfig struct {
	Enabled     bool   `json:"enabled"`
	Algorithm   string `json:"algorithm"`   // "AES-256-GCM", "AES-256-CBC"
	KeyID       string `json:"keyId"`       // Reference to encryption key
	KeyProvider string `json:"keyProvider"` // "kms", "vault", "local"
	Passphrase  string `json:"passphrase"`  // Client-side encryption key
}

// ExportDestination where to store export
type ExportDestination struct {
	Type              string             `json:"type"`             // "local", "s3", "gcs", "azure", "ftp", "sftp"
	Path              string             `json:"path"`             // Local path or remote path
	Bucket            string             `json:"bucket,omitempty"` // S3/GCS bucket
	Region            string             `json:"region,omitempty"` // AWS region
	AccessKey         string             `json:"accessKey,omitempty"`
	SecretKey         string             `json:"secretKey,omitempty"`
	Endpoint          string             `json:"endpoint,omitempty"` // Custom endpoint
	StorageClass      string             `json:"storageClass"`       // S3 storage class
	ACL               string             `json:"acl"`                // S3 ACL
	MakePublic        bool               `json:"makePublic"`
	CreateBackup      bool               `json:"createBackup"` // Keep backup copy
	OverwriteExisting bool               `json:"overwriteExisting"`
	Notification      NotificationConfig `json:"notification"`
}

// ScheduleConfig for recurring exports
type ScheduleConfig struct {
	Enabled    bool   `json:"enabled"`
	Cron       string `json:"cron"` // Cron expression
	Timezone   string `json:"timezone"`
	Frequency  string `json:"frequency"`           // "hourly", "daily", "weekly", "monthly"
	Time       string `json:"time"`                // HH:MM
	DayOfWeek  string `json:"dayOfWeek,omitempty"` // "monday", "tuesday", etc
	DayOfMonth int    `json:"dayOfMonth,omitempty"`
	MaxRuns    int    `json:"maxRuns"`   // 0 = unlimited
	Retention  int    `json:"retention"` // Days to keep scheduled exports
}

// NotificationConfig for export notifications
type NotificationConfig struct {
	Enabled      bool
	OnSuccess    bool
	OnFailure    bool
	Webhooks     []string // Webhook URLs
	Email        []string // Email addresses
	SlackWebhook string   // Slack webhook
}

// --- Resource types ---

// ExportJobSpec is the desired state of an export job.
type ExportJobSpec struct {
	TenantID    string                 `json:"tenantId,omitempty"`
	Description string                 `json:"description,omitempty"`
	Format      ExportFormat           `json:"format"`
	Source      ExportSource           `json:"source"`
	Query       string                 `json:"query,omitempty"`
	Filters     map[string]interface{} `json:"filters,omitempty"`
	Columns     []string               `json:"columns,omitempty"`
	Compression CompressionType        `json:"compression,omitempty"`
	Encryption  EncryptionConfig       `json:"encryption,omitempty"`
	Destination ExportDestination      `json:"destination"`
	Schedule    ScheduleConfig         `json:"schedule,omitempty"`

	// Cancel, when true, asks the controller to cancel the export.
	Cancel bool `json:"cancel,omitempty"`
}

// ExportJobResourceStatus extends the canonical object status.
type ExportJobResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	ExportStatus  ExportStatus `json:"exportStatus"`
	Progress      float64      `json:"progress"`
	RecordCount   int64        `json:"recordCount"`
	FileSize      int64        `json:"fileSize"`
	ProcessedRows int64        `json:"processedRows"`
	SkippedRows   int64        `json:"skippedRows"`
	ErrorRows     int64        `json:"errorRows"`
	DownloadURL   string       `json:"downloadUrl,omitempty"`
	FailureReason string       `json:"failureReason,omitempty"`
	StartedAt     *time.Time   `json:"startedAt,omitempty"`
	CompletedAt   *time.Time   `json:"completedAt,omitempty"`
}

// ExportJobResource is the declarative resource for an ExportJob.
type ExportJobResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   ExportJobSpec           `json:"spec"`
	Status ExportJobResourceStatus `json:"status"`
}

func (r *ExportJobResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *ExportJobResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *ExportJobResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *ExportJobResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *ExportJobResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Columns) > 0 {
		cp.Spec.Columns = append([]string(nil), r.Spec.Columns...)
	}
	return &cp
}
func (r *ExportJobResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *ExportJobResource) GetGeneration() int64         { return r.Generation }
func (r *ExportJobResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
