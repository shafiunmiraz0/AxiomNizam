package slo

// Type aliases re-exporting from models/ so existing code compiles unchanged.

import "axiomnizam.bitbd.net/axiomnizam/internal/slo/models"

const (
	SLOKind       = models.SLOKind
	SLOAPIVersion = models.SLOAPIVersion
)

type SLIType = models.SLIType

const (
	SLITypeAvailability = models.SLITypeAvailability
	SLITypeLatency      = models.SLITypeLatency
	SLITypeQuality      = models.SLITypeQuality
	SLITypeFreshness    = models.SLITypeFreshness
	SLITypeThroughput   = models.SLITypeThroughput
)

type SLISpec = models.SLISpec
type BurnRateAlert = models.BurnRateAlert
type SLOSpec = models.SLOSpec
type SLOResourceStatus = models.SLOResourceStatus
type SLOResource = models.SLOResource
