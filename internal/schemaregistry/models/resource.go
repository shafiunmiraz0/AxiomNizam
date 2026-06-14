package models

// =====================================================
// WS-3.1 — Schema Registry domain Resource types
//
// SchemaResource represents a versioned schema registered for a subject
// (typically a Kafka topic or CDC stream). The reconciler validates
// compatibility against previous versions on registration.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// --- Constants ---

const (
	SchemaKind       = "Schema"
	SchemaAPIVersion = "schema.axiomnizam.io/v1"

	SubjectKind       = "SchemaSubject"
	SubjectAPIVersion = "schema.axiomnizam.io/v1"
)

// --- Schema Types ---

type SchemaType string

const (
	SchemaTypeAvro     SchemaType = "AVRO"
	SchemaTypeJSON     SchemaType = "JSON"
	SchemaTypeProtobuf SchemaType = "PROTOBUF"
)

// --- Compatibility Modes ---

type CompatibilityMode string

const (
	CompatBackward           CompatibilityMode = "BACKWARD"
	CompatBackwardTransitive CompatibilityMode = "BACKWARD_TRANSITIVE"
	CompatForward            CompatibilityMode = "FORWARD"
	CompatForwardTransitive  CompatibilityMode = "FORWARD_TRANSITIVE"
	CompatFull               CompatibilityMode = "FULL"
	CompatFullTransitive     CompatibilityMode = "FULL_TRANSITIVE"
	CompatNone               CompatibilityMode = "NONE"
)

// --- Schema References ---

type SchemaReference struct {
	Name    string `json:"name"`    // Reference name
	Subject string `json:"subject"` // Subject of referenced schema
	Version int    `json:"version"` // Version of referenced schema
}

// --- Schema Rule Set ---

type SchemaRuleSet struct {
	MigrationRules []SchemaRuleEntry `json:"migrationRules,omitempty"`
	DomainRules    []SchemaRuleEntry `json:"domainRules,omitempty"`
}

type SchemaRuleEntry struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"`   // TRANSFORM, CONDITION
	Mode   string            `json:"mode"`   // UPGRADE, DOWNGRADE, BOTH
	Tags   []string          `json:"tags,omitempty"`
	Params map[string]string `json:"params,omitempty"`
}

// --- SchemaSpec ---

type SchemaSpec struct {
	// Subject is the topic or asset name this schema belongs to
	Subject string `json:"subject"`

	// SchemaType: AVRO, JSON, PROTOBUF
	SchemaType SchemaType `json:"schemaType"`

	// Schema is the schema definition (JSON string for Avro/JSON, proto for Protobuf)
	Schema string `json:"schema"`

	// References are cross-schema references
	References []SchemaReference `json:"references,omitempty"`

	// Compatibility mode for this specific schema (overrides subject default)
	Compatibility CompatibilityMode `json:"compatibility,omitempty"`

	// Metadata is arbitrary key-value metadata
	Metadata map[string]string `json:"metadata,omitempty"`

	// RuleSet defines validation and migration rules
	RuleSet *SchemaRuleSet `json:"ruleSet,omitempty"`

	// Description of the schema
	Description string `json:"description,omitempty"`
}

// --- SchemaResourceStatus ---

type SchemaResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	// SchemaID is the globally unique schema identifier
	SchemaID int64 `json:"schemaId"`

	// Version is the version number within the subject
	Version int `json:"version"`

	// Fingerprint is the SHA-256 hash of the normalized schema
	Fingerprint string `json:"fingerprint"`

	// IsLatest indicates if this is the latest version for the subject
	IsLatest bool `json:"isLatest"`

	// RegisteredAt is when this schema was successfully registered
	RegisteredAt *time.Time `json:"registeredAt,omitempty"`

	// CompatibleWith lists version numbers this schema is compatible with
	CompatibleWith []int `json:"compatibleWith,omitempty"`

	// CompatibilityErrors lists any compatibility violations found
	CompatibilityErrors []string `json:"compatibilityErrors,omitempty"`

	// IsCompatible indicates if the schema passed compatibility checks
	IsCompatible bool `json:"isCompatible"`

	// FieldCount is the number of fields/columns in the schema
	FieldCount int `json:"fieldCount"`
}

// --- SchemaResource ---

type SchemaResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   SchemaSpec           `json:"spec"`
	Status SchemaResourceStatus `json:"status"`
}

// --- resources.Resource implementation ---

func (r *SchemaResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *SchemaResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *SchemaResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *SchemaResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *SchemaResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.References) > 0 {
		cp.Spec.References = make([]SchemaReference, len(r.Spec.References))
		copy(cp.Spec.References, r.Spec.References)
	}
	return &cp
}

// --- reconciler.Resource implementation ---

func (r *SchemaResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *SchemaResource) GetGeneration() int64         { return r.Generation }
func (r *SchemaResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// =====================================================
// SchemaSubjectResource -- subject-level configuration
// =====================================================

type SubjectSpec struct {
	// DisplayName is the human-readable subject name
	DisplayName string `json:"displayName,omitempty"`

	// Description of the subject
	Description string `json:"description,omitempty"`

	// Compatibility is the default compatibility mode for this subject
	Compatibility CompatibilityMode `json:"compatibility"`

	// SchemaType is the expected schema type for this subject
	SchemaType SchemaType `json:"schemaType,omitempty"`

	// Owner is the team responsible for this subject
	Owner string `json:"owner,omitempty"`

	// Tags for organization
	Tags []string `json:"tags,omitempty"`

	// MaxVersions is the maximum number of versions to retain (0 = unlimited)
	MaxVersions int `json:"maxVersions,omitempty"`
}

type SubjectResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	// VersionCount is the number of schema versions registered
	VersionCount int `json:"versionCount"`

	// LatestVersion is the latest version number
	LatestVersion int `json:"latestVersion"`

	// LatestSchemaID is the schema ID of the latest version
	LatestSchemaID int64 `json:"latestSchemaId"`

	// LastRegisteredAt is when the last schema was registered
	LastRegisteredAt *time.Time `json:"lastRegisteredAt,omitempty"`
}

type SchemaSubjectResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   SubjectSpec           `json:"spec"`
	Status SubjectResourceStatus `json:"status"`
}

func (r *SchemaSubjectResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *SchemaSubjectResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *SchemaSubjectResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *SchemaSubjectResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *SchemaSubjectResource) DeepCopy() resources.Resource {
	cp := *r
	return &cp
}

func (r *SchemaSubjectResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *SchemaSubjectResource) GetGeneration() int64         { return r.Generation }
func (r *SchemaSubjectResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
