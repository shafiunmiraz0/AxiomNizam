package streaming

// Re-export domain resource types from the models sub-package
// so existing code referencing streaming.StreamResource etc. continues
// to compile without changes.

import "axiomnizam.bitbd.net/axiomnizam/internal/streaming/models"

// Constants
const (
	StreamKind       = models.StreamKind
	StreamAPIVersion = models.StreamAPIVersion
)

// Resource types
type StreamResource = models.StreamResource
type StreamSpec = models.StreamSpec
type StreamResourceStatus = models.StreamResourceStatus

// Dependent types (moved from models.go because resource structs reference them)
type OutputFormat = models.OutputFormat
type DeliveryMode = models.DeliveryMode

// OutputFormat constants
const (
	FormatJSON     = models.FormatJSON
	FormatCSV      = models.FormatCSV
	FormatParquet  = models.FormatParquet
	FormatXML      = models.FormatXML
	FormatProtobuf = models.FormatProtobuf
	FormatNDJSON   = models.FormatNDJSON
)

// DeliveryMode constants
const (
	DeliveryAtMostOnce  = models.DeliveryAtMostOnce
	DeliveryAtLeastOnce = models.DeliveryAtLeastOnce
	DeliveryExactlyOnce = models.DeliveryExactlyOnce
)
