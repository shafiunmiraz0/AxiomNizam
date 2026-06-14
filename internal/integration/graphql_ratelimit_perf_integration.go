package integration

import (
	graphqlpkg "axiomnizam.bitbd.net/axiomnizam/internal/graphql"
	"axiomnizam.bitbd.net/axiomnizam/internal/performance"
	"axiomnizam.bitbd.net/axiomnizam/internal/ratelimit"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Phase1Features integrates all Phase 1 features
type Phase1Features struct {
	GraphQLHandler      *graphqlpkg.Handler
	QuotaHandler        *ratelimit.QuotaHandler
	RateLimitMiddleware *ratelimit.RateLimitMiddleware
	PerformanceHandler  *performance.Handler
	QuotaManager        *ratelimit.QuotaManager
	Analyzer            *performance.QueryPerformanceAnalyzer
}

// NewPhase1Features initializes all Phase 1 features
func NewPhase1Features(db *gorm.DB) *Phase1Features {
	// Initialize GraphQL
	graphqlHandler := graphqlpkg.NewHandler(db)

	// Initialize Rate Limiting & Quotas
	quotaManager := ratelimit.NewQuotaManager()
	rateLimitMiddleware := ratelimit.NewRateLimitMiddleware(quotaManager)
	quotaHandler := ratelimit.NewQuotaHandler(quotaManager)

	// API Documentation support (docs package optional)
	// Will be initialized if available

	// Initialize Query Performance Analyzer
	analyzer := performance.NewQueryPerformanceAnalyzer(100, 10000) // 100ms threshold
	performanceHandler := performance.NewHandler()

	return &Phase1Features{
		GraphQLHandler:      graphqlHandler,
		QuotaHandler:        quotaHandler,
		RateLimitMiddleware: rateLimitMiddleware,
		PerformanceHandler:  performanceHandler,
		QuotaManager:        quotaManager,
		Analyzer:            analyzer,
	}
}

// RegisterRoutes registers all Phase 1 feature routes
func (pf *Phase1Features) RegisterRoutes(router *gin.Engine) {
	// GraphQL endpoints
	graphql := router.Group("/api/graphql")
	{
		graphql.POST("", pf.GraphQLHandler.Query)
		graphql.GET("/schema", pf.GraphQLHandler.GetSchema)
		graphql.GET("/playground", pf.GraphQLHandler.Playground)
	}

	// Rate Limiting & Quotas endpoints
	quota := router.Group("/api/v1/quota")
	quota.Use(pf.RateLimitMiddleware.Handler())
	{
		quota.GET("/:user_id", pf.QuotaHandler.GetQuota)
		quota.PUT("/:user_id", pf.QuotaHandler.SetUserQuota)
		quota.POST("/:user_id/reset", pf.QuotaHandler.ResetQuota)
		quota.GET("", pf.QuotaHandler.ListQuotas)
	}

	endpoints := router.Group("/api/v1/endpoints")
	{
		endpoints.POST("/:endpoint/limit", pf.QuotaHandler.SetEndpointLimit)
	}

	// API Documentation endpoints (optional - can be added later)
	// docs := router.Group("/api/docs")
	// {
	// 	docs.GET("/openapi.json", pf.DocsHandler.GetOpenAPISpec)
	// 	docs.GET("/swagger", pf.DocsHandler.GetSwaggerUI)
	// 	docs.GET("/redoc", pf.DocsHandler.GetReDocUI)
	// 	docs.GET("/markdown", pf.DocsHandler.GetMarkdownDocs)
	// 	docs.GET("/endpoints", pf.DocsHandler.ListEndpoints)
	// 	docs.GET("/endpoints/:id", pf.DocsHandler.GetEndpointDetails)
	// }

	// Performance Monitoring endpoints
	perf := router.Group("/api/v1/performance")
	{
		perf.GET("/stats", pf.PerformanceHandler.GetStats)
		perf.GET("/slow-queries", pf.PerformanceHandler.GetSlowQueries)
		perf.GET("/query-types", pf.PerformanceHandler.GetQueryTypeStats)
		perf.GET("/user-stats", pf.PerformanceHandler.GetUserStats)
		perf.GET("/recommendations", pf.PerformanceHandler.GetRecommendations)
		perf.GET("/percentile/:value", pf.PerformanceHandler.GetPercentile)
		perf.POST("/record", pf.PerformanceHandler.RecordQuery)
		perf.GET("/dashboard", pf.PerformanceHandler.GetDashboard)
	}
}

// ApplyRateLimitMiddleware applies rate limiting to all routes
func (pf *Phase1Features) ApplyRateLimitMiddleware(router *gin.Engine) {
	router.Use(pf.RateLimitMiddleware.Handler())
}

// RegisterQuotaMiddleware registers quota tracking on specific routes
func (pf *Phase1Features) RegisterQuotaMiddleware(group *gin.RouterGroup) {
	group.Use(pf.RateLimitMiddleware.Handler())
}
