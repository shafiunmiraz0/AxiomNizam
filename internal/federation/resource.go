package federation

// =====================================================
// WS-5.1 -- Federated Query and Virtualization
//
// Canonical type definitions live in models/.
// Type aliases here maintain backward compatibility.
// =====================================================

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/federation/models"
)

// --- Constants ---

const (
	VirtualTableKind       = "VirtualTable"
	VirtualTableAPIVersion = "federation.axiomnizam.io/v1"

	FederatedQueryKind       = "FederatedQuery"
	FederatedQueryAPIVersion = "federation.axiomnizam.io/v1"
)

// --- Type aliases (canonical definitions in models/) ---

type VirtualSource = models.VirtualSource
type JoinCondition = models.JoinCondition
type VirtualColumn = models.VirtualColumn
type DefaultFilter = models.DefaultFilter
type CachePolicy = models.CachePolicy
type VirtualTableSpec = models.VirtualTableSpec
type VirtualTableResourceStatus = models.VirtualTableResourceStatus
type VirtualTableResource = models.VirtualTableResource
type QueryFormat = models.QueryFormat
type FederatedQuerySpec = models.FederatedQuerySpec
type QueryPlanNode = models.QueryPlanNode
type FederatedQueryResourceStatus = models.FederatedQueryResourceStatus
type FederatedQueryResource = models.FederatedQueryResource

// --- Const aliases ---

const (
	QueryFormatJSON  = models.QueryFormatJSON
	QueryFormatCSV   = models.QueryFormatCSV
	QueryFormatArrow = models.QueryFormatArrow
)
