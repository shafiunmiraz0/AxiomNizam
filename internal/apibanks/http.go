package apibanks

import (
	"net/http"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/apibanks/audit"
	"axiomnizam.bitbd.net/axiomnizam/internal/apibanks/metrics"
	"github.com/gin-gonic/gin"
)

// Handler provides HTTP endpoints for the APIBanks module.
type Handler struct {
	manager     *APIBankManager
	catalog     *APIBankCatalog
	auditLogger *audit.Logger
}

// NewHandler creates a new APIBanks HTTP handler.
func NewHandler(mgr *APIBankManager, auditLog *audit.Logger) *Handler {
	return &Handler{
		manager:     mgr,
		catalog:     NewAPIBankCatalog(mgr),
		auditLogger: auditLog,
	}
}

// RegisterRoutes registers APIBanks routes on the given router group.
// adminMW is optional middleware applied to write endpoints.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, adminMW ...gin.HandlerFunc) {
	rg.GET("", h.ListBanks)
	rg.GET("/:name", h.GetBank)
	rg.POST("", append(adminMW, h.CreateBank)...)
	rg.PUT("/:name", append(adminMW, h.UpdateBank)...)
	rg.DELETE("/:name", append(adminMW, h.DeleteBank)...)
	rg.GET("/:name/apis", h.ListAPIs)
	rg.POST("/:name/apis", append(adminMW, h.AddAPI)...)
	rg.DELETE("/:name/apis/:apiName", append(adminMW, h.RemoveAPI)...)
	rg.GET("/catalog/search", h.CatalogSearch)
	rg.GET("/catalog/by-data-class/:dataClass", h.CatalogByDataClass)
	rg.GET("/catalog/by-owner/:owner", h.CatalogByOwner)
	rg.GET("/catalog/by-tag/:tag", h.CatalogByTag)
	rg.GET("/catalog/all-apis", h.CatalogAllAPIs)
	rg.GET("/audit", h.GetAuditLog)
	rg.GET("/metrics", h.GetMetrics)
}

// --- Bank Endpoints ---

// ListBanks GET /api/v1/apibanks
func (h *Handler) ListBanks(c *gin.Context) {
	banks := h.manager.ListBanks()
	items := make([]APIBankListItem, 0, len(banks))
	for _, b := range banks {
		items = append(items, APIBankListItem{
			Name:        b.Name,
			Namespace:   b.Namespace,
			Description: b.Description,
			Owner:       b.Owner,
			Version:     b.Version,
			APICount:    len(b.APIs),
			Tags:        b.Tags,
			Labels:      b.Labels,
			CreatedAt:   b.CreatedAt,
			UpdatedAt:   b.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, APIBankListResponse{Status: "success", Banks: items, Total: len(items)})
}

// GetBank GET /api/v1/apibanks/:name
func (h *Handler) GetBank(c *gin.Context) {
	name := c.Param("name")
	bank := h.manager.GetBank(name)
	if bank == nil {
		c.JSON(http.StatusNotFound, APIBankMessageResponse{Error: ErrBankNotFound.Error()})
		return
	}
	c.JSON(http.StatusOK, APIBankResponse{Status: "success", Bank: bank})
}

// CreateBank POST /api/v1/apibanks
func (h *Handler) CreateBank(c *gin.Context) {
	var bank APIBank
	if err := c.ShouldBindJSON(&bank); err != nil {
		c.JSON(http.StatusBadRequest, APIBankMessageResponse{Error: err.Error()})
		return
	}
	if bank.Name == "" {
		c.JSON(http.StatusBadRequest, APIBankMessageResponse{Error: ErrNameRequired.Error()})
		return
	}
	if err := h.manager.CreateBank(c.Request.Context(), &bank); err != nil {
		c.JSON(http.StatusConflict, APIBankMessageResponse{Error: err.Error()})
		metrics.Collector.RecordError("create_bank")
		return
	}
	metrics.Collector.RecordCreated()
	metrics.BanksTotal.Inc()
	if h.auditLogger != nil {
		h.auditLogger.LogBank(audit.ActionBankCreated, bank.Name, "bank created: "+bank.Name)
	}
	c.JSON(http.StatusCreated, APIBankResponse{Status: "success", Bank: &bank})
}

// UpdateBank PUT /api/v1/apibanks/:name
func (h *Handler) UpdateBank(c *gin.Context) {
	name := c.Param("name")
	existing := h.manager.GetBank(name)
	if existing == nil {
		c.JSON(http.StatusNotFound, APIBankMessageResponse{Error: ErrBankNotFound.Error()})
		return
	}
	var updates APIBank
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, APIBankMessageResponse{Error: err.Error()})
		return
	}
	if updates.Description != "" {
		existing.Description = updates.Description
	}
	if updates.Owner != "" {
		existing.Owner = updates.Owner
	}
	if updates.Version != "" {
		existing.Version = updates.Version
	}
	if len(updates.Tags) > 0 {
		existing.Tags = updates.Tags
	}
	if len(updates.Labels) > 0 {
		existing.Labels = updates.Labels
	}
	metrics.Collector.RecordUpdated()
	if h.auditLogger != nil {
		h.auditLogger.LogBank(audit.ActionBankUpdated, name, "bank updated")
	}
	c.JSON(http.StatusOK, APIBankResponse{Status: "success", Bank: existing})
}

// DeleteBank DELETE /api/v1/apibanks/:name
func (h *Handler) DeleteBank(c *gin.Context) {
	name := c.Param("name")
	existing := h.manager.GetBank(name)
	if existing == nil {
		c.JSON(http.StatusNotFound, APIBankMessageResponse{Error: ErrBankNotFound.Error()})
		return
	}
	apiCount := len(existing.APIs)
	if err := h.manager.DeleteBank(name); err != nil {
		c.JSON(http.StatusInternalServerError, APIBankMessageResponse{Error: err.Error()})
		return
	}
	metrics.Collector.RecordDeleted()
	metrics.BanksTotal.Dec()
	metrics.APIsTotal.Sub(float64(apiCount))
	if h.auditLogger != nil {
		h.auditLogger.LogBank(audit.ActionBankDeleted, name, "bank deleted")
	}
	c.JSON(http.StatusOK, APIBankMessageResponse{Message: "bank deleted"})
}

// --- API Endpoints ---

// ListAPIs GET /api/v1/apibanks/:name/apis
func (h *Handler) ListAPIs(c *gin.Context) {
	name := c.Param("name")
	bank := h.manager.GetBank(name)
	if bank == nil {
		c.JSON(http.StatusNotFound, APIBankMessageResponse{Error: ErrBankNotFound.Error()})
		return
	}
	c.JSON(http.StatusOK, APIListResponse{Status: "success", APIs: bank.APIs, Total: len(bank.APIs)})
}

// AddAPI POST /api/v1/apibanks/:name/apis
func (h *Handler) AddAPI(c *gin.Context) {
	name := c.Param("name")
	var api APIReference
	if err := c.ShouldBindJSON(&api); err != nil {
		c.JSON(http.StatusBadRequest, APIBankMessageResponse{Error: err.Error()})
		return
	}
	if err := h.manager.AddAPIToBank(c.Request.Context(), name, api); err != nil {
		c.JSON(http.StatusConflict, APIBankMessageResponse{Error: err.Error()})
		metrics.Collector.RecordError("add_api")
		return
	}
	metrics.Collector.RecordAPIAdded()
	metrics.APIsTotal.Inc()
	if h.auditLogger != nil {
		h.auditLogger.LogAPI(audit.ActionAPIAdded, name, api.Name, "api added to bank: "+api.Name)
	}
	c.JSON(http.StatusCreated, APIBankMessageResponse{Message: "api added"})
}

// RemoveAPI DELETE /api/v1/apibanks/:name/apis/:apiName
func (h *Handler) RemoveAPI(c *gin.Context) {
	name := c.Param("name")
	apiName := c.Param("apiName")
	if err := h.manager.RemoveAPIFromBank(c.Request.Context(), name, apiName); err != nil {
		c.JSON(http.StatusNotFound, APIBankMessageResponse{Error: err.Error()})
		return
	}
	metrics.Collector.RecordAPIRemoved()
	metrics.APIsTotal.Dec()
	if h.auditLogger != nil {
		h.auditLogger.LogAPI(audit.ActionAPIRemoved, name, apiName, "api removed from bank: "+apiName)
	}
	c.JSON(http.StatusOK, APIBankMessageResponse{Message: "api removed"})
}

// --- Catalog Endpoints ---

// CatalogSearch GET /api/v1/apibanks/catalog/search?q=
func (h *Handler) CatalogSearch(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	metrics.Collector.RecordCatalogSearch()
	if h.auditLogger != nil {
		h.auditLogger.LogSearch(q, "catalog search: "+q)
	}
	// Search by owner, tag, or data class
	if byOwner := h.manager.GetBanksByOwner(q); len(byOwner) > 0 {
		apis := make([]APIReference, 0)
		for _, b := range byOwner {
			apis = append(apis, b.APIs...)
		}
		c.JSON(http.StatusOK, APIBankCatalogResponse{Status: "success", APIs: apis, Total: len(apis)})
		return
	}
	if byTag := h.manager.GetBanksByTag(q); len(byTag) > 0 {
		apis := make([]APIReference, 0)
		for _, b := range byTag {
			apis = append(apis, b.APIs...)
		}
		c.JSON(http.StatusOK, APIBankCatalogResponse{Status: "success", APIs: apis, Total: len(apis)})
		return
	}
	apis := h.manager.GetAPIsByDataClass(q)
	c.JSON(http.StatusOK, APIBankCatalogResponse{Status: "success", APIs: apis, Total: len(apis)})
}

// CatalogByDataClass GET /api/v1/apibanks/catalog/by-data-class/:dataClass
func (h *Handler) CatalogByDataClass(c *gin.Context) {
	dataClass := c.Param("dataClass")
	metrics.Collector.RecordCatalogSearch()
	apis := h.manager.GetAPIsByDataClass(dataClass)
	c.JSON(http.StatusOK, APIBankCatalogResponse{Status: "success", APIs: apis, Total: len(apis)})
}

// CatalogByOwner GET /api/v1/apibanks/catalog/by-owner/:owner
func (h *Handler) CatalogByOwner(c *gin.Context) {
	owner := c.Param("owner")
	metrics.Collector.RecordCatalogSearch()
	banks := h.manager.GetBanksByOwner(owner)
	apis := make([]APIReference, 0)
	for _, b := range banks {
		apis = append(apis, b.APIs...)
	}
	c.JSON(http.StatusOK, APIBankCatalogResponse{Status: "success", APIs: apis, Total: len(apis)})
}

// CatalogByTag GET /api/v1/apibanks/catalog/by-tag/:tag
func (h *Handler) CatalogByTag(c *gin.Context) {
	tag := c.Param("tag")
	metrics.Collector.RecordCatalogSearch()
	banks := h.manager.GetBanksByTag(tag)
	apis := make([]APIReference, 0)
	for _, b := range banks {
		apis = append(apis, b.APIs...)
	}
	c.JSON(http.StatusOK, APIBankCatalogResponse{Status: "success", APIs: apis, Total: len(apis)})
}

// CatalogAllAPIs GET /api/v1/apibanks/catalog/all-apis
func (h *Handler) CatalogAllAPIs(c *gin.Context) {
	metrics.Collector.RecordCatalogSearch()
	apis := h.catalog.GetAllAPIs()
	c.JSON(http.StatusOK, APIBankCatalogResponse{Status: "success", APIs: apis, Total: len(apis)})
}

// --- Audit ---

// GetAuditLog GET /api/v1/apibanks/audit
func (h *Handler) GetAuditLog(c *gin.Context) {
	if h.auditLogger == nil {
		c.JSON(http.StatusOK, APIBankAuditListResponse{Status: "success", Events: nil, Total: 0})
		return
	}
	events := h.auditLogger.List()
	c.JSON(http.StatusOK, APIBankAuditListResponse{Status: "success", Events: events, Total: len(events)})
}

// --- Metrics ---

// GetMetrics GET /api/v1/apibanks/metrics
func (h *Handler) GetMetrics(c *gin.Context) {
	snap := metrics.Collector.Snapshot()
	snap.TotalBanks = len(h.manager.ListBanks())
	totalAPIs := 0
	for _, b := range h.manager.ListBanks() {
		totalAPIs += len(b.APIs)
	}
	snap.TotalAPIs = totalAPIs
	c.JSON(http.StatusOK, APIBankMetricsResponse{Status: "success", Metrics: snap})
}
