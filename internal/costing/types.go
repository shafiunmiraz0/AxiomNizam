package costing

// Type aliases re-exported from models/ for backward compatibility.

import "axiomnizam.bitbd.net/axiomnizam/internal/costing/models"

type UsageDimension = models.UsageDimension
type Quota = models.Quota
type RateCard = models.RateCard
type CostAlert = models.CostAlert
type CostPolicySpec = models.CostPolicySpec
type CostPolicyResourceStatus = models.CostPolicyResourceStatus
type CostPolicyResource = models.CostPolicyResource
type UsageRecordSpec = models.UsageRecordSpec
type UsageRecordResourceStatus = models.UsageRecordResourceStatus
type UsageRecordResource = models.UsageRecordResource

const (
	CostPolicyKind       = models.CostPolicyKind
	CostPolicyAPIVersion = models.CostPolicyAPIVersion
	UsageRecordKind      = models.UsageRecordKind
	UsageRecordAPIVersion = models.UsageRecordAPIVersion

	DimensionAPI      = models.DimensionAPI
	DimensionQuery    = models.DimensionQuery
	DimensionPipeline = models.DimensionPipeline
	DimensionStorage  = models.DimensionStorage
)
