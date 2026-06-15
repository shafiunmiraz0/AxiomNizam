package graphql

import (
	"net/http"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler handles GraphQL requests.
type Handler struct {
	resolver *QueryResolver
}

// NewHandler creates a new GraphQL handler.
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{
		resolver: NewQueryResolver(db),
	}
}

// Request represents a GraphQL request.
type Request struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
	OperationName string                 `json:"operationName,omitempty"`
}

// Response represents a GraphQL response.
type Response struct {
	Data   interface{}   `json:"data,omitempty"`
	Errors []interface{} `json:"errors,omitempty"`
}

// Query handles POST /api/graphql
func (h *Handler) Query(c *gin.Context) {
	var req Request

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request: " + err.Error(),
		})
		return
	}

	if req.Query == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Query is required",
		})
		return
	}

	data, err := h.resolver.ResolveQuery(c.Request.Context(), req.Query, req.Variables, req.OperationName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Data: data,
	})
}

// GetSchema handles GET /api/graphql/schema
func (h *Handler) GetSchema(c *gin.Context) {
	_, err := h.resolver.BuildSchema()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status: "ok",
		Data: map[string]interface{}{
			"schema":         "GraphQL schema available",
			"query_endpoint": "/api/graphql",
			"playground":     "/api/graphql/playground",
		},
	})
}

// Playground handles GET /graphql (interactive playground)
func (h *Handler) Playground(c *gin.Context) {
	html := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>GraphQL Playground</title>
		<link rel="stylesheet" href="https://unpkg.com/graphql-playground-react/build/static/css/index.css"/>
	</head>
	<body>
		<div id="root"></div>
		<script src="https://unpkg.com/graphql-playground-react/build/umd/graphql-playground.js"></script>
		<script>
			window.addEventListener('load', function (event) {
				GraphQLPlayground.init(document.getElementById('root'), {
					endpoint: '/api/graphql',
				})
			})
		</script>
	</body>
	</html>
	`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

// RegisterRoutes registers GraphQL routes on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware ...gin.HandlerFunc) {
	rg.POST("/graphql", append(authMiddleware, h.Query)...)
	rg.GET("/graphql/schema", append(authMiddleware, h.GetSchema)...)
	rg.GET("/graphql/playground", append(authMiddleware, h.Playground)...)
}
