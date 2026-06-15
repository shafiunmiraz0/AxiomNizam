package schemaregistry

import "axiomnizam.bitbd.net/axiomnizam/internal/schemaregistry/models"

// Re-export constants
const (
	SchemaKind       = models.SchemaKind
	SchemaAPIVersion = models.SchemaAPIVersion

	SubjectKind       = models.SubjectKind
	SubjectAPIVersion = models.SubjectAPIVersion
)

// Re-export schema type constants
const (
	SchemaTypeAvro     = models.SchemaTypeAvro
	SchemaTypeJSON     = models.SchemaTypeJSON
	SchemaTypeProtobuf = models.SchemaTypeProtobuf
)

// Re-export compatibility mode constants
const (
	CompatBackward           = models.CompatBackward
	CompatBackwardTransitive = models.CompatBackwardTransitive
	CompatForward            = models.CompatForward
	CompatForwardTransitive  = models.CompatForwardTransitive
	CompatFull               = models.CompatFull
	CompatFullTransitive     = models.CompatFullTransitive
	CompatNone               = models.CompatNone
)

// Type aliases for backward compatibility
type SchemaType = models.SchemaType
type CompatibilityMode = models.CompatibilityMode
type SchemaReference = models.SchemaReference
type SchemaRuleSet = models.SchemaRuleSet
type SchemaRuleEntry = models.SchemaRuleEntry
type SchemaSpec = models.SchemaSpec
type SchemaResourceStatus = models.SchemaResourceStatus
type SchemaResource = models.SchemaResource
type SubjectSpec = models.SubjectSpec
type SubjectResourceStatus = models.SubjectResourceStatus
type SchemaSubjectResource = models.SchemaSubjectResource
