package catalog

// HTTP handlers for the Data Catalog API.
//
// Routes:
//   GET    /api/v1/catalog/assets              — List all catalog assets
//   GET    /api/v1/catalog/assets/:name        — Get a specific asset
//   POST   /api/v1/catalog/assets              — Register a new asset
//   PUT    /api/v1/catalog/assets/:name        — Update an asset
//   DELETE /api/v1/catalog/assets/:name        — Delete an asset
//   GET    /api/v1/catalog/search              — Full-text search
//   POST   /api/v1/catalog/scan/:datasource    — Trigger datasource scan
//   GET    /api/v1/catalog/domains             — List business domains
//   GET    /api/v1/catalog/statistics          — Platform-wide catalog stats
//   GET    /api/v1/catalog/collections         — List collections
//   POST   /api/v1/catalog/collections         — Create collection
//   GET    /api/v1/catalog/collections/:name   — Get collection

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/validate"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CatalogHandlers provides HTTP handlers for catalog operations.
type CatalogHandlers struct {
	assetStore      store.ResourceStore[*CatalogAssetResource]
	collectionStore store.ResourceStore[*CatalogCollectionResource]
	scanner         *Scanner
}

// NewCatalogHandlers creates handlers with the given stores.
func NewCatalogHandlers(
	assetStore store.ResourceStore[*CatalogAssetResource],
	collectionStore store.ResourceStore[*CatalogCollectionResource],
	scanner *Scanner,
) *CatalogHandlers {
	return &CatalogHandlers{
		assetStore:      assetStore,
		collectionStore: collectionStore,
		scanner:         scanner,
	}
}

// RegisterRoutes mounts catalog routes on the given router group.
func (h *CatalogHandlers) RegisterRoutes(rg *gin.RouterGroup) {
	catalog := rg.Group("/catalog")
	{
		// Assets
		catalog.GET("/assets", h.ListAssets)
		catalog.GET("/assets/:name", h.GetAsset)
		catalog.POST("/assets", h.CreateAsset)
		catalog.PUT("/assets/:name", h.UpdateAsset)
		catalog.DELETE("/assets/:name", h.DeleteAsset)

		// Search
		catalog.GET("/search", h.SearchAssets)

		// Scan
		catalog.POST("/scan/:datasource", h.ScanDataSource)

		// Domains
		catalog.GET("/domains", h.ListDomains)

		// Statistics
		catalog.GET("/statistics", h.GetStatistics)

		// Collections
		catalog.GET("/collections", h.ListCollections)
		catalog.POST("/collections", h.CreateCollection)
		catalog.GET("/collections/:name", h.GetCollection)
	}
}

// ListAssets returns all catalog assets with optional filtering.
func (h *CatalogHandlers) ListAssets(c *gin.Context) {
	ctx := c.Request.Context()

	assets, err := h.assetStore.List(ctx, "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListAssets"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: "failed to list assets: " + err.Error()})
		return
	}

	// Apply filters.
	domain := c.Query("domain")
	assetType := c.Query("type")
	owner := c.Query("owner")
	tag := c.Query("tag")

	var filtered []*CatalogAssetResource
	for _, asset := range assets {
		if domain != "" && asset.Spec.Domain != domain {
			continue
		}
		if assetType != "" && string(asset.Spec.AssetType) != assetType {
			continue
		}
		if owner != "" && asset.Spec.Owner != owner {
			continue
		}
		if tag != "" && !containsString(asset.Spec.Tags, tag) {
			continue
		}
		filtered = append(filtered, asset)
	}

	c.JSON(http.StatusOK, AssetListResponse{Items: filtered, Total: len(filtered)})
}

// GetAsset returns a specific catalog asset.
func (h *CatalogHandlers) GetAsset(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	ctx := c.Request.Context()

	asset, err := h.assetStore.Get(ctx, name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "asset not found", Name: name})
		return
	}

	c.JSON(http.StatusOK, asset)
}

// CreateAsset registers a new catalog asset.
func (h *CatalogHandlers) CreateAsset(c *gin.Context) {
	var asset CatalogAssetResource
	if err := c.ShouldBindJSON(&asset); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	now := time.Now()
	asset.TypeMeta = resources.TypeMeta{
		APIVersion: CatalogAssetAPIVersion,
		Kind:       CatalogAssetKind,
	}
	if asset.UID == "" {
		asset.UID = uuid.New().String()
	}
	asset.Generation = 1
	asset.CreatedAt = now
	asset.UpdatedAt = now
	asset.Status.Phase = "Pending"
	asset.Status.LastTransitionTime = now

	ctx := c.Request.Context()
	if err := h.assetStore.Create(ctx, &asset); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: "asset already exists or creation failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, asset)
}

// UpdateAsset updates an existing catalog asset.
func (h *CatalogHandlers) UpdateAsset(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	ctx := c.Request.Context()

	existing, err := h.assetStore.Get(ctx, name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "asset not found", Name: name})
		return
	}

	var update CatalogAssetSpec
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	existing.Spec = update
	existing.Generation++
	existing.UpdatedAt = time.Now()

	if err := h.assetStore.Update(ctx, existing); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "UpdateAsset"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: "failed to update asset: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, existing)
}

// DeleteAsset removes a catalog asset.
func (h *CatalogHandlers) DeleteAsset(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	ctx := c.Request.Context()

	if err := h.assetStore.Delete(ctx, name); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "asset not found or delete failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "asset deleted", Name: name})
}

// SearchAssets performs full-text search across catalog assets.
func (h *CatalogHandlers) SearchAssets(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "query parameter 'q' is required"})
		return
	}

	ctx := c.Request.Context()
	assets, err := h.assetStore.List(ctx, "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "SearchAssets"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: "failed to search assets"})
		return
	}

	queryLower := strings.ToLower(query)
	var results []*CatalogAssetResource
	for _, asset := range assets {
		if matchesSearch(asset, queryLower) {
			results = append(results, asset)
		}
	}

	c.JSON(http.StatusOK, CatalogSearchResponse{Query: query, Results: results, Total: len(results)})
}

// ScanDataSource triggers a discovery scan of a datasource.
func (h *CatalogHandlers) ScanDataSource(c *gin.Context) {
	datasource := validate.PathParam(c, "datasource")
	if datasource == "" {
		return
	}

	if h.scanner == nil {
		c.JSON(http.StatusServiceUnavailable, MessageResponse{Error: "catalog scanner not configured"})
		return
	}

	var req ScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Use defaults if no body provided.
		req = ScanRequest{
			DataSourceRef: datasource,
			IncludeViews:  true,
		}
	} else {
		req.DataSourceRef = datasource
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	result, err := h.scanner.Scan(ctx, req)
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ScanDataSource"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: "scan failed: " + err.Error()})
		return
	}

	// Create CatalogAssetResources for discovered assets.
	now := time.Now()
	var scanErrors []string
	for _, discovered := range result.DiscoveredAssets {
		assetName := fmt.Sprintf("%s.%s.%s", discovered.Database, discovered.Schema, discovered.Name)
		asset := &CatalogAssetResource{
			TypeMeta: resources.TypeMeta{
				APIVersion: CatalogAssetAPIVersion,
				Kind:       CatalogAssetKind,
			},
			ObjectMeta: resources.ObjectMeta{
				Name:      assetName,
				UID:       uuid.New().String(),
				Generation: 1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Spec: CatalogAssetSpec{
				AssetType:     discovered.Type,
				DataSourceRef: datasource,
				Database:      discovered.Database,
				Schema:        discovered.Schema,
				TableName:     discovered.Name,
				Columns:       discovered.Columns,
				RefreshPolicy: RefreshPolicy{Enabled: true, Interval: "6h"},
			},
			Status: CatalogAssetResourceStatus{
				ObjectStatus: resources.ObjectStatus{
					Phase:              "Active",
					LastTransitionTime: now,
					ObservedGeneration: 1,
				},
				RowCount:        discovered.RowCount,
				SizeBytes:       discovered.SizeBytes,
				ColumnCount:     len(discovered.Columns),
				LastScannedAt:   &now,
				FreshnessStatus: "fresh",
			},
		}

		// Try to create; if exists, update.
		if err := h.assetStore.Create(c.Request.Context(), asset); err != nil {
			// Asset may already exist — try update.
			existing, getErr := h.assetStore.Get(c.Request.Context(), assetName)
			if getErr == nil {
				existing.Spec.Columns = discovered.Columns
				existing.Status.RowCount = discovered.RowCount
				existing.Status.SizeBytes = discovered.SizeBytes
				existing.Status.ColumnCount = len(discovered.Columns)
				existing.Status.LastScannedAt = &now
				existing.UpdatedAt = now
				if updateErr := h.assetStore.Update(c.Request.Context(), existing); updateErr != nil {
					scanErrors = append(scanErrors, fmt.Sprintf("failed to update %s: %v", assetName, updateErr))
				} else {
					result.AssetsUpdated++
				}
			} else {
				scanErrors = append(scanErrors, fmt.Sprintf("failed to create %s: %v", assetName, err))
			}
		} else {
			result.AssetsCreated++
		}
	}

	response := ScanResultResponse{
		DataSourceRef: result.DataSourceRef,
		AssetsFound:   result.AssetsFound,
		AssetsCreated: result.AssetsCreated,
		AssetsUpdated: result.AssetsUpdated,
		Duration:      result.Duration.String(),
	}
	if len(scanErrors) > 0 {
		response.Errors = scanErrors
		response.PartialFailure = true
	}
	if len(result.Errors) > 0 {
		response.ScanErrors = result.Errors
	}

	c.JSON(http.StatusOK, response)
}

// ListDomains returns all unique business domains.
func (h *CatalogHandlers) ListDomains(c *gin.Context) {
	ctx := c.Request.Context()
	assets, err := h.assetStore.List(ctx, "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListDomains"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: "failed to list assets"})
		return
	}

	domainMap := make(map[string]int)
	for _, asset := range assets {
		if asset.Spec.Domain != "" {
			domainMap[asset.Spec.Domain]++
		}
	}

	type DomainInfo struct {
		Name       string `json:"name"`
		AssetCount int    `json:"assetCount"`
	}

	var domains []DomainInfo
	for name, count := range domainMap {
		domains = append(domains, DomainInfo{Name: name, AssetCount: count})
	}

	c.JSON(http.StatusOK, DomainListResponse{Domains: domains, Total: len(domains)})
}

// GetStatistics returns platform-wide catalog statistics.
func (h *CatalogHandlers) GetStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	assets, err := h.assetStore.List(ctx, "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "GetStatistics"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: "failed to compute statistics"})
		return
	}

	stats := struct {
		TotalAssets    int              `json:"totalAssets"`
		ByType         map[string]int   `json:"byType"`
		ByDomain       map[string]int   `json:"byDomain"`
		TotalRows      int64            `json:"totalRows"`
		TotalSizeBytes int64            `json:"totalSizeBytes"`
		AvgQuality     float64          `json:"avgQuality"`
		StaleAssets    int              `json:"staleAssets"`
		PIIAssets      int              `json:"piiAssets"`
	}{
		ByType:   make(map[string]int),
		ByDomain: make(map[string]int),
	}

	var qualitySum float64
	var qualityCount int

	for _, asset := range assets {
		stats.TotalAssets++
		stats.ByType[string(asset.Spec.AssetType)]++
		if asset.Spec.Domain != "" {
			stats.ByDomain[asset.Spec.Domain]++
		}
		stats.TotalRows += asset.Status.RowCount
		stats.TotalSizeBytes += asset.Status.SizeBytes
		if asset.Status.QualityScore > 0 {
			qualitySum += asset.Status.QualityScore
			qualityCount++
		}
		if asset.Status.FreshnessStatus == "stale" {
			stats.StaleAssets++
		}
		if asset.Spec.Classification.PII {
			stats.PIIAssets++
		}
	}

	if qualityCount > 0 {
		stats.AvgQuality = qualitySum / float64(qualityCount)
	}

	c.JSON(http.StatusOK, stats)
}

// ListCollections returns all catalog collections.
func (h *CatalogHandlers) ListCollections(c *gin.Context) {
	ctx := c.Request.Context()
	collections, err := h.collectionStore.List(ctx, "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListCollections"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: "failed to list collections"})
		return
	}
	c.JSON(http.StatusOK, CollectionListResponse{Items: collections, Total: len(collections)})
}

// CreateCollection creates a new catalog collection.
func (h *CatalogHandlers) CreateCollection(c *gin.Context) {
	var collection CatalogCollectionResource
	if err := c.ShouldBindJSON(&collection); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	now := time.Now()
	collection.TypeMeta = resources.TypeMeta{
		APIVersion: CatalogCollectionAPIVersion,
		Kind:       CatalogCollectionKind,
	}
	if collection.UID == "" {
		collection.UID = uuid.New().String()
	}
	collection.Generation = 1
	collection.CreatedAt = now
	collection.UpdatedAt = now
	collection.Status.Phase = "Active"
	collection.Status.LastTransitionTime = now

	ctx := c.Request.Context()
	if err := h.collectionStore.Create(ctx, &collection); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: "collection already exists: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, collection)
}

// GetCollection returns a specific collection.
func (h *CatalogHandlers) GetCollection(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	ctx := c.Request.Context()

	collection, err := h.collectionStore.Get(ctx, name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "collection not found", Name: name})
		return
	}

	c.JSON(http.StatusOK, collection)
}

// --- Helpers ---

func matchesSearch(asset *CatalogAssetResource, query string) bool {
	// Search across name, description, domain, tags, columns.
	if strings.Contains(strings.ToLower(asset.Name), query) {
		return true
	}
	if strings.Contains(strings.ToLower(asset.Spec.Description), query) {
		return true
	}
	if strings.Contains(strings.ToLower(asset.Spec.Domain), query) {
		return true
	}
	if strings.Contains(strings.ToLower(asset.Spec.TableName), query) {
		return true
	}
	for _, tag := range asset.Spec.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	for _, col := range asset.Spec.Columns {
		if strings.Contains(strings.ToLower(col.Name), query) {
			return true
		}
	}
	return false
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}


