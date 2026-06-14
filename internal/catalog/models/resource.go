package models

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// =====================================================
// WS-1.1 — Data Catalog as declarative resources
//
// CatalogAssetResource represents a discoverable data asset (table,
// view, topic, bucket, API, pipeline) in the unified metadata registry.
//
// CatalogCollectionResource groups assets into logical collections.
// =====================================================

// --- Constants ---

const (
	CatalogAssetKind       = "CatalogAsset"
	CatalogAssetAPIVersion = "catalog.axiomnizam.io/v1"

	CatalogCollectionKind       = "CatalogCollection"
	CatalogCollectionAPIVersion = "catalog.axiomnizam.io/v1"
)

// --- Asset Types ---

type AssetType string

const (
	AssetTypeTable    AssetType = "table"
	AssetTypeView     AssetType = "view"
	AssetTypeTopic    AssetType = "topic"
	AssetTypeBucket   AssetType = "bucket"
	AssetTypeAPI      AssetType = "api"
	AssetTypePipeline AssetType = "pipeline"
	AssetTypeModel    AssetType = "model"
	AssetTypeDataset  AssetType = "dataset"
)

// --- Refresh Policy ---

type RefreshPolicy struct {
	Interval string `json:"interval,omitempty"`
	Schedule string `json:"schedule,omitempty"`
	Enabled  bool   `json:"enabled"`
}

// --- Data Classification ---

type DataClassification struct {
	Level         string   `json:"level"`
	Categories    []string `json:"categories,omitempty"`
	PII           bool     `json:"pii"`
	Sensitive     bool     `json:"sensitive"`
	Encrypted     bool     `json:"encrypted"`
	RetentionDays int      `json:"retentionDays,omitempty"`
}

// --- Column Metadata ---

type CatalogColumn struct {
	Name           string      `json:"name"`
	Type           string      `json:"type"`
	Nullable       bool        `json:"nullable"`
	Description    string      `json:"description,omitempty"`
	IsPrimaryKey   bool        `json:"isPrimaryKey,omitempty"`
	IsForeignKey   bool        `json:"isForeignKey,omitempty"`
	ForeignKeyRef  string      `json:"foreignKeyRef,omitempty"`
	DefaultValue   string      `json:"defaultValue,omitempty"`
	Classification string      `json:"classification,omitempty"`
	IsPII          bool        `json:"isPII,omitempty"`
	Tags           []string    `json:"tags,omitempty"`
	Stats          *ColumnStats `json:"stats,omitempty"`
}

type ColumnStats struct {
	NullCount     int64   `json:"nullCount"`
	NullPercent   float64 `json:"nullPercent"`
	DistinctCount int64   `json:"distinctCount"`
	MinValue      string  `json:"minValue,omitempty"`
	MaxValue      string  `json:"maxValue,omitempty"`
	AvgLength     float64 `json:"avgLength,omitempty"`
	SampleValues  []string `json:"sampleValues,omitempty"`
}

// --- CatalogAssetSpec ---

type CatalogAssetSpec struct {
	AssetType      AssetType         `json:"assetType"`
	DataSourceRef  string            `json:"dataSourceRef"`
	Database       string            `json:"database,omitempty"`
	Schema         string            `json:"schema,omitempty"`
	TableName      string            `json:"tableName,omitempty"`
	Owner          string            `json:"owner,omitempty"`
	Domain         string            `json:"domain,omitempty"`
	Description    string            `json:"description,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	Classification DataClassification `json:"classification,omitempty"`
	Columns        []CatalogColumn   `json:"columns,omitempty"`
	RefreshPolicy  RefreshPolicy     `json:"refreshPolicy,omitempty"`
	Upstream       []string          `json:"upstream,omitempty"`
	Downstream     []string          `json:"downstream,omitempty"`
	Documentation  string            `json:"documentation,omitempty"`
	QualityRules   []string          `json:"qualityRules,omitempty"`
	ContractRef    string            `json:"contractRef,omitempty"`
}

// --- CatalogAssetResourceStatus ---

type CatalogAssetResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	RowCount               int64              `json:"rowCount"`
	SizeBytes              int64              `json:"sizeBytes"`
	ColumnCount            int                `json:"columnCount"`
	IndexCount             int                `json:"indexCount"`
	LastScannedAt          *time.Time         `json:"lastScannedAt,omitempty"`
	LastModifiedAt         *time.Time         `json:"lastModifiedAt,omitempty"`
	QualityScore           float64            `json:"qualityScore"`
	PopularityScore        float64            `json:"popularityScore"`
	FreshnessStatus        string             `json:"freshnessStatus"`
	ScanError              string             `json:"scanError,omitempty"`
	ConsecutiveScanFailures int              `json:"consecutiveScanFailures,omitempty"`
	ProfiledAt             *time.Time         `json:"profiledAt,omitempty"`
	ClassificationDetected *DataClassification `json:"classificationDetected,omitempty"`
}

// --- CatalogAssetResource ---

type CatalogAssetResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   CatalogAssetSpec           `json:"spec"`
	Status CatalogAssetResourceStatus `json:"status"`
}

func (r *CatalogAssetResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *CatalogAssetResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *CatalogAssetResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *CatalogAssetResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *CatalogAssetResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Columns) > 0 {
		cp.Spec.Columns = make([]CatalogColumn, len(r.Spec.Columns))
		copy(cp.Spec.Columns, r.Spec.Columns)
	}
	if len(r.Spec.Tags) > 0 {
		cp.Spec.Tags = make([]string, len(r.Spec.Tags))
		copy(cp.Spec.Tags, r.Spec.Tags)
	}
	return &cp
}

func (r *CatalogAssetResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *CatalogAssetResource) GetGeneration() int64         { return r.Generation }
func (r *CatalogAssetResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// =====================================================
// CatalogCollectionResource — logical grouping of assets
// =====================================================

type CatalogCollectionSpec struct {
	DisplayName  string            `json:"displayName"`
	Description  string            `json:"description,omitempty"`
	Domain       string            `json:"domain,omitempty"`
	Owner        string            `json:"owner,omitempty"`
	AssetSelector map[string]string `json:"assetSelector,omitempty"`
	AssetRefs    []string          `json:"assetRefs,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
}

type CatalogCollectionResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	AssetCount       int        `json:"assetCount"`
	TotalSizeBytes   int64      `json:"totalSizeBytes"`
	AvgQualityScore  float64    `json:"avgQualityScore"`
	LastUpdatedAt    *time.Time `json:"lastUpdatedAt,omitempty"`
}

type CatalogCollectionResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   CatalogCollectionSpec           `json:"spec"`
	Status CatalogCollectionResourceStatus `json:"status"`
}

func (r *CatalogCollectionResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *CatalogCollectionResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *CatalogCollectionResource) GetStatus() *resources.ObjectStatus {
	return &r.Status.ObjectStatus
}
func (r *CatalogCollectionResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *CatalogCollectionResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.AssetRefs) > 0 {
		cp.Spec.AssetRefs = make([]string, len(r.Spec.AssetRefs))
		copy(cp.Spec.AssetRefs, r.Spec.AssetRefs)
	}
	return &cp
}

func (r *CatalogCollectionResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *CatalogCollectionResource) GetGeneration() int64         { return r.Generation }
func (r *CatalogCollectionResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
