package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/alerting"
	alertingmodels "axiomnizam.bitbd.net/axiomnizam/internal/alerting/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/anonymization"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
	"axiomnizam.bitbd.net/axiomnizam/internal/autopilot"
	"axiomnizam.bitbd.net/axiomnizam/internal/catalog"
	"axiomnizam.bitbd.net/axiomnizam/internal/config"
	"axiomnizam.bitbd.net/axiomnizam/internal/contracts"
	"axiomnizam.bitbd.net/axiomnizam/internal/costing"
	"axiomnizam.bitbd.net/axiomnizam/internal/database"
	"axiomnizam.bitbd.net/axiomnizam/internal/deployment"
	"axiomnizam.bitbd.net/axiomnizam/internal/featurestore"
	"axiomnizam.bitbd.net/axiomnizam/internal/federation"
	"axiomnizam.bitbd.net/axiomnizam/internal/governance"
	governancemodels "axiomnizam.bitbd.net/axiomnizam/internal/governance/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/heartbeat"
	"axiomnizam.bitbd.net/axiomnizam/internal/migrations"
	"axiomnizam.bitbd.net/axiomnizam/internal/mlpipeline"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/ipaccess"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/ratelimit"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
	"axiomnizam.bitbd.net/axiomnizam/internal/schemaregistry"
	"axiomnizam.bitbd.net/axiomnizam/internal/serviceregistry"
	"axiomnizam.bitbd.net/axiomnizam/internal/slo"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage"
	"axiomnizam.bitbd.net/axiomnizam/internal/stream"
	"axiomnizam.bitbd.net/axiomnizam/internal/streamanalytics"
	"axiomnizam.bitbd.net/axiomnizam/internal/trivy"
	"github.com/gin-gonic/gin"
)

// WireUnwiredModules initializes modules that were previously unwired
// and registers their routes.  This function is called after the
// reconciler section.
func WireUnwiredModules(
	conns *database.Connections,
	cfg *config.Config,
	router *gin.Engine,
	authMiddleware gin.HandlerFunc,
	adminOrSysMiddleware gin.HandlerFunc,
	backendMgr *platformstore.BackendManager,
	platformManagers *platform.Managers,
	storageSys *storage.System,
) {
	// ====================================
	// MIGRATIONS (previously unwired)
	// ====================================
	if conns.PostgreSQL != nil {
		if migrationErr := migrations.RunMigrations(conns.PostgreSQL); migrationErr != nil {
			log.Printf("⚠️  Database migrations failed: %v", migrationErr)
		} else {
			log.Println("✅ Database migrations completed successfully")
		}
	}

	// ====================================
	// HEARTBEAT TRACKER (previously unwired)
	// ====================================
	heartbeatTracker := heartbeat.New(func(id string) {
		log.Printf("⚠️  Heartbeat expired for entity: %s", id)
	})
	heartbeatTracker.ReapInterval = 5 * time.Second
	heartbeatTracker.Start()
	log.Println("✅ Heartbeat tracker started")

	// ====================================
	// SERVICE REGISTRY (previously unwired)
	// ====================================
	svcRegistry := serviceregistry.New()
	log.Println("✅ Service registry started")

	// ====================================
	// AUTOPILOT (previously unwired)
	// ====================================
	autopilotInstance := autopilot.New(autopilot.Config{
		MaxTrailingLogs:      250,
		LastContactThreshold: 200 * time.Millisecond,
		DeadServerCleanup:    true,
		MinQuorum:            3,
	})
	_ = autopilotInstance // available for cluster health evaluation
	log.Println("✅ Autopilot initialized")

	// ====================================
	// TRIVY VULNERABILITY SCANNER (previously unwired)
	// ====================================
	trivyBinaryPath := strings.TrimSpace(os.Getenv("TRIVY_BINARY_PATH"))
	if trivyBinaryPath == "" {
		trivyBinaryPath = "trivy"
	}
	trivyEngine := trivy.NewEngine(trivyBinaryPath)
	log.Printf("✅ Trivy vulnerability scanner initialized (binary: %s)", trivyBinaryPath)

	// ====================================
	// TRIVY SCANNER ROUTES
	// ====================================
	trivyAPI := router.Group("/api/v1/trivy", authMiddleware)
	{
		trivyAPI.POST("/scan", adminOrSysMiddleware, func(c *gin.Context) {
			var req trivy.ScanRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			req.UseExternal = true
			result, err := trivyEngine.Scan(c.Request.Context(), req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, result)
		})
	}

	// ====================================
	// DEPLOYMENT CONTROLLER ROUTES
	// ====================================
	deploymentControllers := make(map[string]*deployment.Controller)
	var deploymentMu sync.Mutex

	deploymentAPI := router.Group("/api/v1/deployments", authMiddleware)
	{
		deploymentAPI.POST("", adminOrSysMiddleware, func(c *gin.Context) {
			var spec deployment.Spec
			if err := c.ShouldBindJSON(&spec); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if strings.TrimSpace(spec.JobID) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "jobId is required"})
				return
			}
			deploymentMu.Lock()
			ctrl := deployment.NewController(spec)
			deploymentControllers[spec.JobID] = ctrl
			deploymentMu.Unlock()
			c.JSON(http.StatusCreated, gin.H{"message": "deployment created", "jobId": spec.JobID, "state": ctrl.State()})
		})
		deploymentAPI.GET("/:jobId", func(c *gin.Context) {
			jobID := strings.TrimSpace(c.Param("jobId"))
			deploymentMu.Lock()
			ctrl, ok := deploymentControllers[jobID]
			deploymentMu.Unlock()
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
				return
			}
			c.JSON(http.StatusOK, ctrl.State())
		})
		deploymentAPI.POST("/:jobId/promote", adminOrSysMiddleware, func(c *gin.Context) {
			jobID := strings.TrimSpace(c.Param("jobId"))
			deploymentMu.Lock()
			ctrl, ok := deploymentControllers[jobID]
			deploymentMu.Unlock()
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
				return
			}
			if !ctrl.Promote() {
				c.JSON(http.StatusConflict, gin.H{"error": "promotion not available — canaries may not be healthy or deployment not in running phase"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "deployment promoted", "state": ctrl.State()})
		})
		deploymentAPI.POST("/:jobId/fail", adminOrSysMiddleware, func(c *gin.Context) {
			jobID := strings.TrimSpace(c.Param("jobId"))
			var body struct {
				Reason string `json:"reason"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
				return
			}
			if strings.TrimSpace(body.Reason) == "" {
				body.Reason = "manual rollback"
			}
			deploymentMu.Lock()
			ctrl, ok := deploymentControllers[jobID]
			deploymentMu.Unlock()
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
				return
			}
			decision := ctrl.Fail(body.Reason)
			c.JSON(http.StatusOK, gin.H{"message": "deployment failed", "decision": decision, "state": ctrl.State()})
		})
	}

	// ====================================
	// SERVICE REGISTRY ROUTES
	// ====================================
	svcRegistryAPI := router.Group("/api/v1/service-registry", authMiddleware)
	{
		svcRegistryAPI.POST("/services", adminOrSysMiddleware, func(c *gin.Context) {
			var svc serviceregistry.Service
			if err := c.ShouldBindJSON(&svc); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if strings.TrimSpace(svc.ID) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "service id is required"})
				return
			}
			if svc.Checks == nil {
				svc.Checks = make(map[string]*serviceregistry.Check)
			}
			svcRegistry.Register(&svc)
			c.JSON(http.StatusCreated, gin.H{"message": "service registered", "id": svc.ID})
		})
		svcRegistryAPI.DELETE("/services/:id", adminOrSysMiddleware, func(c *gin.Context) {
			svcRegistry.Deregister(strings.TrimSpace(c.Param("id")))
			c.JSON(http.StatusOK, gin.H{"message": "service deregistered"})
		})
		svcRegistryAPI.GET("/services/:id", func(c *gin.Context) {
			svc, ok := svcRegistry.Get(strings.TrimSpace(c.Param("id")))
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "service not found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"service": svc, "status": svc.Rollup()})
		})
		svcRegistryAPI.GET("/services", func(c *gin.Context) {
			name := strings.TrimSpace(c.Query("name"))
			if name != "" {
				c.JSON(http.StatusOK, gin.H{"services": svcRegistry.ByName(name)})
				return
			}
			c.JSON(http.StatusOK, gin.H{"services": svcRegistry.ByName("")})
		})
		svcRegistryAPI.PUT("/services/:id/checks/:checkId", adminOrSysMiddleware, func(c *gin.Context) {
			var body struct {
				Status string `json:"status"`
				Notes  string `json:"notes"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if err := svcRegistry.UpdateCheck(strings.TrimSpace(c.Param("id")), strings.TrimSpace(c.Param("checkId")), serviceregistry.Status(body.Status), body.Notes); err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "check updated"})
		})
	}

	// ====================================
	// HEARTBEAT ROUTES
	// ====================================
	heartbeatAPI := router.Group("/api/v1/heartbeat", authMiddleware)
	{
		heartbeatAPI.POST("/beat", func(c *gin.Context) {
			var body struct {
				ID  string `json:"id"`
				TTL int    `json:"ttl"` // seconds
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if strings.TrimSpace(body.ID) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
				return
			}
			ttl := time.Duration(body.TTL) * time.Second
			if ttl <= 0 {
				ttl = 30 * time.Second
			}
			heartbeatTracker.Beat(body.ID, ttl)
			c.JSON(http.StatusOK, gin.H{"message": "heartbeat recorded", "id": body.ID, "ttl_seconds": int(ttl.Seconds())})
		})
		heartbeatAPI.GET("/alive/:id", func(c *gin.Context) {
			id := strings.TrimSpace(c.Param("id"))
			c.JSON(http.StatusOK, gin.H{"id": id, "alive": heartbeatTracker.IsAlive(id)})
		})
		heartbeatAPI.GET("/expired", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"expired": heartbeatTracker.Expired()})
		})
		heartbeatAPI.DELETE("/:id", adminOrSysMiddleware, func(c *gin.Context) {
			heartbeatTracker.Delete(strings.TrimSpace(c.Param("id")))
			c.JSON(http.StatusOK, gin.H{"message": "heartbeat entry deleted"})
		})
	}

	// ====================================
	// AUTOPILOT ROUTES
	// ====================================
	router.POST("/api/v1/autopilot/evaluate", adminOrSysMiddleware, func(c *gin.Context) {
		var body struct {
			Peers       []autopilot.Server `json:"peers"`
			LeaderIndex uint64             `json:"leaderIndex"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		decisions := autopilotInstance.Evaluate(c.Request.Context(), body.Peers, body.LeaderIndex)
		c.JSON(http.StatusOK, gin.H{"decisions": decisions})
	})

	// ====================================
	// ANTIVIRUS MANAGEMENT API (uses existing engine from storage module)
	// ====================================
	if storageSys != nil && storageSys.AVEngine != nil {
		avHandler := antivirus.NewAPIHandler(storageSys.AVEngine)
		if backendMgr != nil {
			avHandler.SetKVStore(backendMgr.KV())
		}
		avHandler.RegisterRoutes(router.Group("/api/v1", authMiddleware), adminOrSysMiddleware)
		log.Println("✅ Antivirus management API registered (reusing storage engine)")
	} else {
		log.Println("⚠️  Antivirus engine not available — management API skipped")
	}

	// ====================================
	// SCANNER CONFIG API (uses existing orchestrator from storage module)
	// ====================================
	if storageSys != nil && storageSys.ScanOrch != nil {
		var kvStore platformstore.KVStore
		if backendMgr != nil {
			kvStore = backendMgr.KV()
		}
		scannerHandler := scanner.NewScannerConfigHandler(storageSys.ScanOrch, kvStore)
		scGroup := router.Group("/api/v1/scanner", authMiddleware)
		adminSc := scGroup.Group("/")
		adminSc.Use(adminOrSysMiddleware)
		{
			adminSc.GET("/config", scannerHandler.GetConfig)
			adminSc.PUT("/config", scannerHandler.UpdateConfig)
		}
		log.Println("✅ Scanner config API registered")
	}

	// ====================================
	// IP ACCESS LOG & BLOCKING
	// ====================================
	ipTracker := ipaccess.NewTracker(10000)
	router.Use(ipaccess.AccessTrackerMiddleware(ipTracker))
	ipAccessHandler := ipaccess.NewHandler(ipTracker)
	ipAccessAPI := router.Group("/api/v1/ip-access", authMiddleware)
	adminIPA := ipAccessAPI.Group("/")
	adminIPA.Use(adminOrSysMiddleware)
	{
		adminIPA.GET("/log", ipAccessHandler.GetAccessLog)
		adminIPA.GET("/ips", ipAccessHandler.GetIPList)
		adminIPA.GET("/ips/:ip", ipAccessHandler.GetIPDetail)
		adminIPA.GET("/ips/:ip/log", ipAccessHandler.GetIPLog)
		adminIPA.GET("/blocked", ipAccessHandler.GetBlockedIPs)
		adminIPA.POST("/block", ipAccessHandler.BlockIP)
		adminIPA.POST("/unblock", ipAccessHandler.UnblockIP)
	}
	log.Println("✅ IP Access Log & Blocking registered")

	// ====================================
	// RATE LIMITING (previously unwired)
	// ====================================
	quotaMgr := ratelimit.NewQuotaManager()
	rlMiddleware := ratelimit.NewRateLimitMiddleware(quotaMgr)
	quotaHandler := ratelimit.NewQuotaHandler(quotaMgr)
	router.Use(rlMiddleware.Handler())
	quotaAPI := router.Group("/api/v1/quotas", authMiddleware)
	{
		quotaAPI.GET("", quotaHandler.ListQuotas)
		quotaAPI.GET("/:userID", quotaHandler.GetQuota)
		quotaAPI.POST("/:userID", adminOrSysMiddleware, quotaHandler.SetUserQuota)
		quotaAPI.DELETE("/:userID", adminOrSysMiddleware, quotaHandler.ResetQuota)
		quotaAPI.POST("/endpoint", adminOrSysMiddleware, quotaHandler.SetEndpointLimit)
	}
	log.Println("✅ Rate limiting module started")

	// ====================================
	// STREAM BROKER (previously unwired)
	// ====================================
	streamBroker := stream.NewBroker(10000)
	streamHTTP := stream.HTTPHandler(streamBroker)
	router.Any("/api/v1/stream", func(c *gin.Context) {
		streamHTTP.ServeHTTP(c.Writer, c.Request)
	})
	log.Println("✅ Stream broker started")

	// ====================================
	// FEATURE MODULES (storage-backed)
	// ====================================
	if backendMgr != nil {
		// Alerting
		alertRuleStore := platformstore.NewStore[*alertingmodels.AlertRuleResource](backendMgr, "alert-rules", func() *alertingmodels.AlertRuleResource { return &alertingmodels.AlertRuleResource{} })
		alertIncidentStore := platformstore.NewStore[*alertingmodels.AlertIncidentResource](backendMgr, "alert-incidents", func() *alertingmodels.AlertIncidentResource { return &alertingmodels.AlertIncidentResource{} })
		alertChannelStore := platformstore.NewStore[*alertingmodels.NotificationChannelResource](backendMgr, "alert-channels", func() *alertingmodels.NotificationChannelResource { return &alertingmodels.NotificationChannelResource{} })
		alertHandlers := alerting.NewAlertHandlers(alertRuleStore, alertIncidentStore, alertChannelStore)
		alertHandlers.RegisterRoutes(router.Group("/api/v1", authMiddleware))
		log.Println("✅ Alerting module started")

		// Governance
		complianceStore := platformstore.NewStore[*governancemodels.CompliancePolicyResource](backendMgr, "compliance-policies", func() *governancemodels.CompliancePolicyResource { return &governancemodels.CompliancePolicyResource{} })
		retentionStore := platformstore.NewStore[*governancemodels.RetentionPolicyResource](backendMgr, "retention-policies", func() *governancemodels.RetentionPolicyResource { return &governancemodels.RetentionPolicyResource{} })
		accessReqStore := platformstore.NewStore[*governancemodels.AccessRequestResource](backendMgr, "access-requests", func() *governancemodels.AccessRequestResource { return &governancemodels.AccessRequestResource{} })
		govHandlers := governance.NewGovernanceHandlers(complianceStore, retentionStore, accessReqStore)
		govHandlers.RegisterRoutes(router.Group("/api/v1", authMiddleware))
		log.Println("✅ Governance module started")

		// SLO
		sloStore := platformstore.NewStore[*slo.SLOResource](backendMgr, "slos", func() *slo.SLOResource { return &slo.SLOResource{} })
		sloHandlers := slo.NewSLOHandlers(sloStore)
		sloHandlers.RegisterRoutes(router.Group("/api/v1", authMiddleware))
		log.Println("✅ SLO module started")

		// Catalog
		assetStore := platformstore.NewStore[*catalog.CatalogAssetResource](backendMgr, "catalog-assets", func() *catalog.CatalogAssetResource { return &catalog.CatalogAssetResource{} })
		collectionStore := platformstore.NewStore[*catalog.CatalogCollectionResource](backendMgr, "catalog-collections", func() *catalog.CatalogCollectionResource { return &catalog.CatalogCollectionResource{} })
		catalogHandlers := catalog.NewCatalogHandlers(assetStore, collectionStore, nil)
		catalogHandlers.RegisterRoutes(router.Group("/api/v1", authMiddleware))
		log.Println("✅ Catalog module started")

		// Costing
		costPolicyStore := platformstore.NewStore[*costing.CostPolicyResource](backendMgr, "cost-policies", func() *costing.CostPolicyResource { return &costing.CostPolicyResource{} })
		usageStore := platformstore.NewStore[*costing.UsageRecordResource](backendMgr, "usage-records", func() *costing.UsageRecordResource { return &costing.UsageRecordResource{} })
		costHandlers := costing.NewCostHandlers(costPolicyStore, usageStore)
		costHandlers.RegisterRoutes(router.Group("/api/v1", authMiddleware))
		log.Println("✅ Costing module started")

		// Contracts
		contractStore := platformstore.NewStore[*contracts.DataContractResource](backendMgr, "data-contracts", func() *contracts.DataContractResource { return &contracts.DataContractResource{} })
		contractHandlers := contracts.NewContractHandlers(contractStore)
		contractHandlers.RegisterRoutes(router.Group("/api/v1", authMiddleware))
		log.Println("✅ Contracts module started")

		// Schema Registry
		schemaStore := platformstore.NewStore[*schemaregistry.SchemaResource](backendMgr, "schemas", func() *schemaregistry.SchemaResource { return &schemaregistry.SchemaResource{} })
		subjectStore := platformstore.NewStore[*schemaregistry.SchemaSubjectResource](backendMgr, "schema-subjects", func() *schemaregistry.SchemaSubjectResource { return &schemaregistry.SchemaSubjectResource{} })
		schemaChecker := schemaregistry.NewJSONSchemaCompatibilityChecker()
		schemaHandlers := schemaregistry.NewSchemaRegistryHandlers(schemaStore, subjectStore, schemaChecker)
		schemaHandlers.RegisterRoutes(router.Group("/api/v1", authMiddleware))
		log.Println("✅ Schema Registry module started")

		// Anonymization
		anonymPolicyStore := platformstore.NewStore[*anonymization.AnonymizationPolicyResource](backendMgr, "anonymization-policies", func() *anonymization.AnonymizationPolicyResource { return &anonymization.AnonymizationPolicyResource{} })
		anonymHandlers := anonymization.NewAnonymizationHandlers(anonymPolicyStore)
		anonymHandlers.RegisterRoutes(router.Group("/api/v1", authMiddleware))
		log.Println("✅ Anonymization module started")

		// Stream Analytics
		streamJobStore := platformstore.NewStore[*streamanalytics.StreamJobResource](backendMgr, "stream-jobs", func() *streamanalytics.StreamJobResource { return &streamanalytics.StreamJobResource{} })
		streamHandlers := streamanalytics.NewStreamAnalyticsHandlers(streamJobStore)
		streamHandlers.RegisterRoutes(router.Group("/api/v1", authMiddleware))
		log.Println("✅ Stream Analytics module started")

		// Feature Store
		featureGroupStore := platformstore.NewStore[*featurestore.FeatureGroupResource](backendMgr, "feature-groups", func() *featurestore.FeatureGroupResource { return &featurestore.FeatureGroupResource{} })
		featureHandlers := featurestore.NewFeatureStoreHandlers(featureGroupStore)
		featureHandlers.RegisterRoutes(router.Group("/api/v1", authMiddleware))
		log.Println("✅ Feature Store module started")

		// Federation
		vtStore := platformstore.NewStore[*federation.VirtualTableResource](backendMgr, "virtual-tables", func() *federation.VirtualTableResource { return &federation.VirtualTableResource{} })
		fedQueryStore := platformstore.NewStore[*federation.FederatedQueryResource](backendMgr, "federated-queries", func() *federation.FederatedQueryResource { return &federation.FederatedQueryResource{} })
		fedHandlers := federation.NewFederationHandlers(vtStore, fedQueryStore, nil)
		fedHandlers.RegisterRoutes(router.Group("/api/v1", authMiddleware))
		log.Println("✅ Federation module started")

		// ML Pipeline
		mlPipelineStore := platformstore.NewStore[*mlpipeline.MLPipelineResource](backendMgr, "ml-pipelines", func() *mlpipeline.MLPipelineResource { return &mlpipeline.MLPipelineResource{} })
		modelDeployStore := platformstore.NewStore[*mlpipeline.ModelDeploymentResource](backendMgr, "model-deployments", func() *mlpipeline.ModelDeploymentResource { return &mlpipeline.ModelDeploymentResource{} })
		mlHandlers := mlpipeline.NewMLPipelineHandlers(mlPipelineStore, modelDeployStore)
		mlHandlers.RegisterRoutes(router.Group("/api/v1", authMiddleware))
		log.Println("✅ ML Pipeline module started")
	} else {
		log.Println("⚠️  Storage backend not available — feature modules (alerting, governance, slo, catalog, costing, contracts, schemaregistry, anonymization, streamanalytics, featurestore, federation, mlpipeline) skipped")
	}
}

// PrintStartupBanner prints the server startup information.
func PrintStartupBanner(cfg *config.Config, iamOnlyAuth bool) {
	apiPort := cfg.API.Port
	apiHost := cfg.API.Host

	scheme := "https"
	if os.Getenv("TLS_ENABLED") == "false" {
		scheme = "http"
	}
	fmt.Printf("📡 AxiomNizam running on %s://%s:%s\n", scheme, apiHost, apiPort)
	fmt.Println("\n🔐 RBAC Security Model:")
	fmt.Println("  ✅ READ  operations (GET)     - Allowed for all authenticated users")
	fmt.Println("  ❌ WRITE operations (POST/PUT/DELETE) - Allowed ONLY for users with 'admin' role")
	fmt.Println()
	fmt.Println("Available endpoints:")
	fmt.Println("  GET  /health                  - Health check (no auth)")
	fmt.Println("  GET  /status                  - Check all connections (no auth)")
	fmt.Println("  ANY  /api/custom/*path        - Execute API Builder runtime APIs")
	fmt.Println()
	fmt.Println("Admin endpoints (admin role required):")
	fmt.Println("  POST /api/admin/database/create       - Create a new database")
	fmt.Println("  GET  /api/admin/database/list         - List all databases")
	fmt.Println("  GET  /api/admin/database/servers      - List default and connected DB servers")
	fmt.Println("  POST /api/admin/database/connect      - Connect a new DB server")
	fmt.Println("  PUT  /api/admin/database/servers/:key - Update a custom DB server")
	fmt.Println("  DELETE /api/admin/database/servers/:key - Delete a custom DB server")
	fmt.Println("  POST /api/admin/table/create          - Create a new table")
	fmt.Println("  GET  /api/admin/table/list            - List all tables")
	fmt.Println()
	if !iamOnlyAuth {
		fmt.Println("User Management endpoints (admin only):")
		fmt.Println("  GET    /api/v1/users            - List all platform users")
		fmt.Println("  GET    /api/v1/users/:id        - Get a platform user")
		fmt.Println("  POST   /api/v1/users            - Create a platform user")
		fmt.Println("  PUT    /api/v1/users/:id        - Update a platform user")
		fmt.Println("  DELETE /api/v1/users/:id        - Delete a platform user")
		fmt.Println()
	}
	fmt.Println("Dynamic Query endpoints:")
	fmt.Println("  GET  /api/{db}/query            - Execute SELECT queries with parameters")
	fmt.Println("       Example: /api/mysql/query?q=SELECT * FROM users&params=1")
	fmt.Println("  POST /api/{db}/query            - Execute query body (admin/system-manager only)")
	fmt.Println("       Body: {\"query\": \"SQL_QUERY\", \"params\": [\"value1\", \"value2\"]}")
	fmt.Println("  POST /api/{db}/query/batch      - Execute multiple queries at once (admin/system-manager only)")
	fmt.Println("       Body: [{\"query\": \"SQL_QUERY\", \"params\": []}]")
	fmt.Println("  GET  /api/{db}/schema           - Get table schema")
	fmt.Println("       Example: /api/mysql/schema?table=users")
	fmt.Println("  Available databases: postgres (others configured dynamically from UI)")
	fmt.Println()
	fmt.Println("Notification endpoints (authenticated users):")
	fmt.Println("  POST /api/notifications/send    - Send custom notification to Discord")
	fmt.Println("  POST /api/notifications/health  - Send health check notification")
	fmt.Println("  POST /api/notifications/status  - Send status report notification")
}
