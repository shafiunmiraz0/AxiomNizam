package models

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// APIType
// ─────────────────────────────────────────────────────────────────────────────

type APIType string

const (
	APITypeREST     APIType = "rest"
	APITypeGraphQL  APIType = "graphql"
)

// ─────────────────────────────────────────────────────────────────────────────
// APIStatus
// ─────────────────────────────────────────────────────────────────────────────

type APIStatus string

const (
	APIStatusActive   APIStatus = "active"
	APIStatusInactive APIStatus = "inactive"
	APIStatusDraft    APIStatus = "draft"
)

// ─────────────────────────────────────────────────────────────────────────────
// HTTPMethod
// ─────────────────────────────────────────────────────────────────────────────

type HTTPMethod string

const (
	MethodGET    HTTPMethod = "GET"
	MethodPOST   HTTPMethod = "POST"
	MethodPUT    HTTPMethod = "PUT"
	MethodDELETE HTTPMethod = "DELETE"
)

// ─────────────────────────────────────────────────────────────────────────────
// UploadStatus
// ─────────────────────────────────────────────────────────────────────────────

type UploadStatus string

const (
	UploadStatusUploaded        UploadStatus = "uploaded"
	UploadStatusAnalyzed        UploadStatus = "analyzed"
	UploadStatusDashboardCreated UploadStatus = "dashboard_created"
	UploadStatusGISCreated      UploadStatus = "gis_created"
)

// ─────────────────────────────────────────────────────────────────────────────
// ScanVerdict
// ─────────────────────────────────────────────────────────────────────────────

type ScanVerdict string

const (
	VerdictSafe     ScanVerdict = "safe"
	VerdictUnsafe   ScanVerdict = "unsafe"
	VerdictUnknown  ScanVerdict = "unknown"
)

// ─────────────────────────────────────────────────────────────────────────────
// CustomAPIResource — API endpoint created through the GUI builder
// ─────────────────────────────────────────────────────────────────────────────

type CustomAPIResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   CustomAPISpec   `json:"spec"`
	Status CustomAPIStatus `json:"status"`
}

type CustomAPISpec struct {
	APIType        APIType             `json:"apiType"`
	Name           string              `json:"name"`
	Method         HTTPMethod          `json:"method"`
	Path           string              `json:"path"`
	SQLTemplate    string              `json:"sqlTemplate,omitempty"`
	SQLPolicyMode  string              `json:"sqlPolicyMode,omitempty"`
	GraphQLQuery   string              `json:"graphqlQuery,omitempty"`
	GraphQLOpName  string              `json:"graphqlOperationName,omitempty"`
	Description    string              `json:"description,omitempty"`
	Category       string              `json:"category,omitempty"`
	SourceDatabase string              `json:"sourceDatabase,omitempty"`
	SourceServer   string              `json:"sourceServer,omitempty"`
	AuthRequired   bool                `json:"authRequired"`
	RateLimit      int                 `json:"rateLimit,omitempty"`
	CacheEnabled   bool                `json:"cacheEnabled"`
	CacheTTL       int                 `json:"cacheTtl,omitempty"`
	RequestSchema  *SchemaDefinition   `json:"requestSchema,omitempty"`
	ResponseSchema *SchemaDefinition   `json:"responseSchema,omitempty"`
	Headers        map[string]string   `json:"headers,omitempty"`
	QueryParams    []ParamDef          `json:"queryParams,omitempty"`
}

type SchemaDefinition struct {
	Type       string                 `json:"type"`
	Properties map[string]SchemaField `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
	Example    any                    `json:"example,omitempty"`
}

type SchemaField struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Default     any      `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type ParamDef struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
	Default     string `json:"default,omitempty"`
}

type CustomAPIStatus struct {
	resources.ObjectStatus `json:",inline"`
	HitCount   int64     `json:"hitCount,omitempty"`
	LastHitAt  time.Time `json:"lastHitAt,omitempty"`
}

func (r *CustomAPIResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *CustomAPIResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *CustomAPIResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *CustomAPIResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *CustomAPIResource) DeepCopy() resources.Resource         { out := *r; return &out }
func (r *CustomAPIResource) GetKey() string                       { return r.Namespace + "/" + r.Name }
func (r *CustomAPIResource) GetGeneration() int64                 { return r.ObjectMeta.Generation }
func (r *CustomAPIResource) GetObservedGeneration() int64         { return r.Status.ObservedGeneration }

// ─────────────────────────────────────────────────────────────────────────────
// CSVUploadResource — file upload converted to dashboard
// ─────────────────────────────────────────────────────────────────────────────

type CSVUploadResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   CSVUploadSpec   `json:"spec"`
	Status CSVUploadStatus `json:"status"`
}

type CSVUploadSpec struct {
	Filename    string   `json:"filename"`
	FileType    string   `json:"fileType"`
	Rows        int      `json:"rows"`
	Columns     int      `json:"columns"`
	ColumnNames []string `json:"columnNames,omitempty"`
	ColumnTypes []string `json:"columnTypes,omitempty"`
	HasGeoData  bool     `json:"hasGeoData"`
}

type CSVUploadStatus struct {
	resources.ObjectStatus `json:",inline"`
	DashboardID    string `json:"dashboardId,omitempty"`
	GISDashboardID string `json:"gisDashboardId,omitempty"`
}

func (r *CSVUploadResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *CSVUploadResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *CSVUploadResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *CSVUploadResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *CSVUploadResource) DeepCopy() resources.Resource         { out := *r; return &out }
func (r *CSVUploadResource) GetKey() string                       { return r.Namespace + "/" + r.Name }
func (r *CSVUploadResource) GetGeneration() int64                 { return r.ObjectMeta.Generation }
func (r *CSVUploadResource) GetObservedGeneration() int64         { return r.Status.ObservedGeneration }

// ─────────────────────────────────────────────────────────────────────────────
// ConversionResource — dashboard↔GIS conversion
// ─────────────────────────────────────────────────────────────────────────────

type ConversionResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   ConversionSpec   `json:"spec"`
	Status ConversionStatus `json:"status"`
}

type ConversionSpec struct {
	SourceType string `json:"sourceType"`
	SourceID   string `json:"sourceId"`
	TargetType string `json:"targetType"`
}

type ConversionStatus struct {
	resources.ObjectStatus `json:",inline"`
	TargetID       string         `json:"targetId,omitempty"`
	FieldMappings  []FieldMapping `json:"fieldMappings,omitempty"`
	GeoFieldsFound []string       `json:"geoFieldsFound,omitempty"`
	Confidence     float64        `json:"confidence,omitempty"`
}

type FieldMapping struct {
	SourceField string `json:"sourceField"`
	TargetField string `json:"targetField"`
	MappingType string `json:"mappingType"`
}

func (r *ConversionResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *ConversionResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *ConversionResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *ConversionResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *ConversionResource) DeepCopy() resources.Resource         { out := *r; return &out }
func (r *ConversionResource) GetKey() string                       { return r.Namespace + "/" + r.Name }
func (r *ConversionResource) GetGeneration() int64                 { return r.ObjectMeta.Generation }
func (r *ConversionResource) GetObservedGeneration() int64         { return r.Status.ObservedGeneration }
