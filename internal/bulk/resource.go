package bulk

import "axiomnizam.bitbd.net/axiomnizam/internal/bulk/models"

const (
	BulkOperationKind       = models.BulkOperationKind
	BulkOperationAPIVersion = models.BulkOperationAPIVersion
)

type BulkOpType = models.BulkOpType
type BulkOpStatus = models.BulkOpStatus
type BulkItemStatus = models.BulkItemStatus
type BulkImportFormat = models.BulkImportFormat
type BulkOperation = models.BulkOperation
type BulkItem = models.BulkItem
type BulkItemResult = models.BulkItemResult
type BulkItemError = models.BulkItemError
type BulkErrorSummary = models.BulkErrorSummary
type BulkOperationOptions = models.BulkOperationOptions
type RetryPolicy = models.RetryPolicy
type BulkOperationRequest = models.BulkOperationRequest
type BulkOperationResponse = models.BulkOperationResponse
type BulkOperationProgress = models.BulkOperationProgress
type BulkImportRequest = models.BulkImportRequest
type BulkExportRequest = models.BulkExportRequest
type BulkOperationSpec = models.BulkOperationSpec
type BulkOperationResourceStatus = models.BulkOperationResourceStatus
type BulkOperationResource = models.BulkOperationResource

const (
	BulkOpCreate  = models.BulkOpCreate
	BulkOpUpdate  = models.BulkOpUpdate
	BulkOpDelete  = models.BulkOpDelete
	BulkOpPatch   = models.BulkOpPatch
	BulkOpReplace = models.BulkOpReplace
	BulkOpUpsert  = models.BulkOpUpsert

	BulkOpPending   = models.BulkOpPending
	BulkOpRunning   = models.BulkOpRunning
	BulkOpCompleted = models.BulkOpCompleted
	BulkOpFailed    = models.BulkOpFailed
	BulkOpCancelled = models.BulkOpCancelled
	BulkOpPartial   = models.BulkOpPartial

	BulkItemPending = models.BulkItemPending
	BulkItemSuccess = models.BulkItemSuccess
	BulkItemFailed  = models.BulkItemFailed
	BulkItemSkipped = models.BulkItemSkipped
	BulkItemRetry   = models.BulkItemRetry

	FormatJSON    = models.FormatJSON
	FormatCSV     = models.FormatCSV
	FormatParquet = models.FormatParquet
	FormatXML     = models.FormatXML
)
