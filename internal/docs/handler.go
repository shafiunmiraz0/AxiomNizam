package docs

import (
	"net/http"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"github.com/gin-gonic/gin"
)

// Handler handles documentation endpoints.
type Handler struct {
	generator *OpenAPIGenerator
}

// NewHandler creates a new docs handler.
func NewHandler() *Handler {
	generator := NewOpenAPIGenerator(OpenAPIInfo{
		Title:       "AxiomNizam API",
		Version:     "1.0.0",
		Description: "AxiomNizam Platform API",
	})
	return &Handler{
		generator: generator,
	}
}

// GetOpenAPISpec handles GET /api/docs/openapi.json
func (h *Handler) GetOpenAPISpec(c *gin.Context) {
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]string{
			"title":   "AxiomNizam API",
			"version": "1.0.0",
		},
	}
	c.JSON(http.StatusOK, spec)
}

// GetSwaggerUI handles GET /api/docs/swagger
func (h *Handler) GetSwaggerUI(c *gin.Context) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>API Docs</title>
</head>
<body>
    <h1>API Documentation</h1>
    <p>Visit /api/docs/openapi.json for the OpenAPI spec.</p>
</body>
</html>`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

// GetReDocUI handles GET /api/docs/redoc
func (h *Handler) GetReDocUI(c *gin.Context) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>ReDoc</title>
</head>
<body>
    <h1>API Documentation</h1>
    <p>Visit /api/docs/openapi.json for the OpenAPI spec.</p>
</body>
</html>`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

// GetMarkdownDocs handles GET /api/docs/markdown
func (h *Handler) GetMarkdownDocs(c *gin.Context) {
	markdown := h.generator.GetEndpointMarkdown()
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, markdown)
}

// ListEndpoints handles GET /api/docs/endpoints
func (h *Handler) ListEndpoints(c *gin.Context) {
	endpoints := make([]map[string]interface{}, 0)

	c.JSON(http.StatusOK, models.Response{
		Status: "ok",
		Data: map[string]interface{}{
			"endpoints": endpoints,
			"total":     len(endpoints),
		},
	})
}

// GetEndpointDetails handles GET /api/docs/endpoints/:id
func (h *Handler) GetEndpointDetails(c *gin.Context) {
	id := c.Param("id")

	if id == "" || id == "0" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "endpoint id is required",
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status: "ok",
		Data:   h.generator.BuildOpenAPI(),
	})
}

// RegisterRoutes registers docs routes on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/docs/openapi.json", h.GetOpenAPISpec)
	rg.GET("/docs/swagger", h.GetSwaggerUI)
	rg.GET("/docs/redoc", h.GetReDocUI)
	rg.GET("/docs/markdown", h.GetMarkdownDocs)
	rg.GET("/docs/endpoints", h.ListEndpoints)
	rg.GET("/docs/endpoints/:id", h.GetEndpointDetails)
}
