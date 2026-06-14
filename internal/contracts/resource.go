package contracts

// Type aliases re-exporting from models/ so existing code compiles unchanged.

import "axiomnizam.bitbd.net/axiomnizam/internal/contracts/models"

const (
	Kind       = models.Kind
	APIVersion = models.APIVersion
)

// Backward-compatible aliases for the prefixed constant names.
const (
	DataContractKind       = models.Kind
	DataContractAPIVersion = models.APIVersion
)

// Re-export domain Resource types from models subpackage.
type CompatibilityMode = models.CompatibilityMode

const (
	CompatBackward = models.CompatBackward
	CompatForward  = models.CompatForward
	CompatFull     = models.CompatFull
	CompatNone     = models.CompatNone
)

type ContractColumn = models.ContractColumn
type ContractSchema = models.ContractSchema
type ContractSLA = models.ContractSLA
type ContractQuality = models.ContractQuality
type DataContractSpec = models.DataContractSpec
type ContractViolation = models.ContractViolation
type DataContractResourceStatus = models.DataContractResourceStatus
type DataContractResource = models.DataContractResource
