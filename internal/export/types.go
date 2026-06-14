package export

// Re-export domain resource types from the models sub-package
// so existing code referencing export.ExportJobResource etc. continues
// to compile without changes.

import "axiomnizam.bitbd.net/axiomnizam/internal/export/models"

// Constants
const (
	ExportJobKind       = models.ExportJobKind
	ExportJobAPIVersion = models.ExportJobAPIVersion
)

// Resource types
type ExportJobResource = models.ExportJobResource
type ExportJobSpec = models.ExportJobSpec
type ExportJobResourceStatus = models.ExportJobResourceStatus

// Dependent types (moved from models.go because resource structs reference them)
type ExportStatus = models.ExportStatus
type ExportFormat = models.ExportFormat
type ExportSource = models.ExportSource
type CompressionType = models.CompressionType
type EncryptionConfig = models.EncryptionConfig
type ExportDestination = models.ExportDestination
type ScheduleConfig = models.ScheduleConfig
type NotificationConfig = models.NotificationConfig

// ExportStatus constants
const (
	ExportPending     = models.ExportPending
	ExportValidating  = models.ExportValidating
	ExportQueued      = models.ExportQueued
	ExportRunning     = models.ExportRunning
	ExportProcessing  = models.ExportProcessing
	ExportCompressing = models.ExportCompressing
	ExportEncrypting  = models.ExportEncrypting
	ExportUploading   = models.ExportUploading
	ExportCompleted   = models.ExportCompleted
	ExportFailed      = models.ExportFailed
	ExportCancelled   = models.ExportCancelled
	ExportPartial     = models.ExportPartial
)

// ExportFormat constants
const (
	FormatJSON     = models.FormatJSON
	FormatCSV      = models.FormatCSV
	FormatXML      = models.FormatXML
	FormatParquet  = models.FormatParquet
	FormatAvro     = models.FormatAvro
	FormatProtobuf = models.FormatProtobuf
	FormatExcel    = models.FormatExcel
	FormatPDF      = models.FormatPDF
	FormatSQL      = models.FormatSQL
	FormatNDJSON   = models.FormatNDJSON
)

// CompressionType constants
const (
	CompressionNone   = models.CompressionNone
	CompressionGzip   = models.CompressionGzip
	CompressionBrotli = models.CompressionBrotli
	CompressionLZ4    = models.CompressionLZ4
	CompressionZstd   = models.CompressionZstd
)
