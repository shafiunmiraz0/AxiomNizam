package apibuilder

import "axiomnizam.bitbd.net/axiomnizam/internal/apibuilder/models"

// Type aliases for backward compatibility — types live in models/.
type (
	APIType       = models.APIType
	APIStatus     = models.APIStatus
	HTTPMethod    = models.HTTPMethod
	UploadStatus  = models.UploadStatus
	ScanVerdict   = models.ScanVerdict

	CustomAPIResource  = models.CustomAPIResource
	CustomAPISpec      = models.CustomAPISpec
	CustomAPIStatus    = models.CustomAPIStatus
	CSVUploadResource  = models.CSVUploadResource
	CSVUploadSpec      = models.CSVUploadSpec
	CSVUploadStatus    = models.CSVUploadStatus
	ConversionResource = models.ConversionResource
	ConversionSpec     = models.ConversionSpec
	ConversionStatusT  = models.ConversionStatus
	FieldMapping       = models.FieldMapping

	// Shared schema types
	SchemaDefinition = models.SchemaDefinition
	SchemaField      = models.SchemaField
	ParamDef         = models.ParamDef
)

// Re-export constants.
const (
	APITypeREST    = models.APITypeREST
	APITypeGraphQL = models.APITypeGraphQL

	APIStatusActive   = models.APIStatusActive
	APIStatusInactive = models.APIStatusInactive
	APIStatusDraft    = models.APIStatusDraft

	MethodGET    = models.MethodGET
	MethodPOST   = models.MethodPOST
	MethodPUT    = models.MethodPUT
	MethodDELETE = models.MethodDELETE

	UploadStatusUploaded         = models.UploadStatusUploaded
	UploadStatusAnalyzed         = models.UploadStatusAnalyzed
	UploadStatusDashboardCreated = models.UploadStatusDashboardCreated
	UploadStatusGISCreated       = models.UploadStatusGISCreated

	VerdictSafe    = models.VerdictSafe
	VerdictUnsafe  = models.VerdictUnsafe
	VerdictUnknown = models.VerdictUnknown
)
