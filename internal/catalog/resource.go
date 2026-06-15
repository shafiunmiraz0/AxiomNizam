package catalog

// Re-export domain types from models sub-package for backward compatibility.
import "axiomnizam.bitbd.net/axiomnizam/internal/catalog/models"

// --- Constants ---
const (
	CatalogAssetKind       = models.CatalogAssetKind
	CatalogAssetAPIVersion = models.CatalogAssetAPIVersion

	CatalogCollectionKind       = models.CatalogCollectionKind
	CatalogCollectionAPIVersion = models.CatalogCollectionAPIVersion
)

// --- Type aliases for backward compatibility ---
type AssetType = models.AssetType
type RefreshPolicy = models.RefreshPolicy
type DataClassification = models.DataClassification
type CatalogColumn = models.CatalogColumn
type ColumnStats = models.ColumnStats
type CatalogAssetSpec = models.CatalogAssetSpec
type CatalogAssetResourceStatus = models.CatalogAssetResourceStatus
type CatalogAssetResource = models.CatalogAssetResource
type CatalogCollectionSpec = models.CatalogCollectionSpec
type CatalogCollectionResourceStatus = models.CatalogCollectionResourceStatus
type CatalogCollectionResource = models.CatalogCollectionResource

// --- Asset type constants ---
const (
	AssetTypeTable    = models.AssetTypeTable
	AssetTypeView     = models.AssetTypeView
	AssetTypeTopic    = models.AssetTypeTopic
	AssetTypeBucket   = models.AssetTypeBucket
	AssetTypeAPI      = models.AssetTypeAPI
	AssetTypePipeline = models.AssetTypePipeline
	AssetTypeModel    = models.AssetTypeModel
	AssetTypeDataset  = models.AssetTypeDataset
)
