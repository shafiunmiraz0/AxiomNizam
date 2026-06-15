package apiserver

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/kubeplus/admission"
	"axiomnizam.bitbd.net/axiomnizam/internal/kubeplus/crd"
	"axiomnizam.bitbd.net/axiomnizam/internal/kubeplus/scheduler"
	"axiomnizam.bitbd.net/axiomnizam/internal/netintel/modes"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
	"axiomnizam.bitbd.net/axiomnizam/internal/reviewflow"
	"axiomnizam.bitbd.net/axiomnizam/internal/vectorplus"
	"github.com/gin-gonic/gin"
)

// ResourceStore stores resources in memory with persistence
type ResourceStore struct {
	mu        sync.RWMutex
	resources map[string]map[string]resources.Resource // namespace -> name -> resource
	watchers  map[string][]ResourceWatcher             // namespace -> list of watchers
}

// ResourceWatcher watches for resource changes
type ResourceWatcher interface {
	OnAdd(resource resources.Resource)
	OnUpdate(oldResource, newResource resources.Resource)
	OnDelete(resource resources.Resource)
}

// NewResourceStore creates a new resource store
func NewResourceStore() *ResourceStore {
	return &ResourceStore{
		resources: make(map[string]map[string]resources.Resource),
		watchers:  make(map[string][]ResourceWatcher),
	}
}

// Create stores a new resource
func (rs *ResourceStore) Create(resource resources.Resource) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	meta := resource.GetObjectMeta()
	namespace := meta.Namespace
	if namespace == "" {
		namespace = "default"
	}

	if rs.resources[namespace] == nil {
		rs.resources[namespace] = make(map[string]resources.Resource)
	}

	if rs.resources[namespace][meta.Name] != nil {
		return fmt.Errorf("resource already exists: %s/%s", namespace, meta.Name)
	}

	rs.resources[namespace][meta.Name] = resource.DeepCopy()
	rs.notifyWatchers(namespace, WatchEventAdded, resource, nil)

	return nil
}

// Get retrieves a resource
func (rs *ResourceStore) Get(namespace, name string) (resources.Resource, error) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if namespace == "" {
		namespace = "default"
	}

	if rs.resources[namespace] == nil {
		return nil, fmt.Errorf(errResourceNotFound, namespace, name)
	}

	resource := rs.resources[namespace][name]
	if resource == nil {
		return nil, fmt.Errorf(errResourceNotFound, namespace, name)
	}

	return resource.DeepCopy(), nil
}

// Update updates an existing resource
func (rs *ResourceStore) Update(resource resources.Resource) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	meta := resource.GetObjectMeta()
	namespace := meta.Namespace
	if namespace == "" {
		namespace = "default"
	}

	if rs.resources[namespace] == nil {
		return fmt.Errorf(errResourceNotFound, namespace, meta.Name)
	}

	oldResource := rs.resources[namespace][meta.Name]
	if oldResource == nil {
		return fmt.Errorf(errResourceNotFound, namespace, meta.Name)
	}

	rs.resources[namespace][meta.Name] = resource.DeepCopy()
	rs.notifyWatchers(namespace, WatchEventModified, resource, oldResource)

	return nil
}

// Delete removes a resource
func (rs *ResourceStore) Delete(namespace, name string) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if namespace == "" {
		namespace = "default"
	}

	if rs.resources[namespace] == nil {
		return fmt.Errorf(errResourceNotFound, namespace, name)
	}

	resource := rs.resources[namespace][name]
	if resource == nil {
		return fmt.Errorf(errResourceNotFound, namespace, name)
	}

	delete(rs.resources[namespace], name)
	rs.notifyWatchers(namespace, WatchEventDeleted, resource, nil)

	return nil
}

// List returns all resources in a namespace
func (rs *ResourceStore) List(namespace string, selector map[string]string) ([]resources.Resource, error) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if namespace == "" {
		namespace = "default"
	}

	var result []resources.Resource

	if rs.resources[namespace] == nil {
		return result, nil
	}

	for _, resource := range rs.resources[namespace] {
		// Filter by labels if selector provided
		if len(selector) > 0 {
			if !resource.GetObjectMeta().MatchesLabels(selector) {
				continue
			}
		}
		result = append(result, resource.DeepCopy())
	}

	return result, nil
}

// Watch registers a watcher for resource changes
func (rs *ResourceStore) Watch(namespace string, watcher ResourceWatcher) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if namespace == "" {
		namespace = "default"
	}

	rs.watchers[namespace] = append(rs.watchers[namespace], watcher)
}

// notifyWatchers notifies all watchers of a change
func (rs *ResourceStore) notifyWatchers(namespace string, eventType WatchEventType, newResource, oldResource resources.Resource) {
	watchers := rs.watchers[namespace]

	for _, watcher := range watchers {
		switch eventType {
		case WatchEventAdded:
			go watcher.OnAdd(newResource)
		case WatchEventModified:
			go watcher.OnUpdate(oldResource, newResource)
		case WatchEventDeleted:
			go watcher.OnDelete(newResource)
		}
	}
}

// WatchEventType represents type of watch event
type WatchEventType string

const (
	WatchEventAdded    WatchEventType = "ADDED"
	WatchEventModified WatchEventType = "MODIFIED"
	WatchEventDeleted  WatchEventType = "DELETED"

	errResourceNotFound = "resource not found: %s/%s"
	errUnknownKind      = "unknown resource kind: %s"
	routeNsKindName     = "/:namespace/:kind/:name"
)

// APIServer provides REST API for resources
type APIServer struct {
	store          *ResourceStore
	router         *gin.Engine
	admission      *admission.Engine
	scheduler      *scheduler.Scheduler
	crdRegistry    *crd.Registry
	modeManager    *modes.Manager
	vectorIndex    *vectorplus.Index
	reviewPipeline *reviewflow.Pipeline
}

// NewAPIServer creates a new API server
func NewAPIServer(store *ResourceStore) *APIServer {
	admissionEngine := admission.NewEngine()
	admissionEngine.RegisterPolicy("template-001", 100, admission.PolicyTemplate001)
	admissionEngine.RegisterPolicy("template-002", 90, admission.PolicyTemplate002)
	admissionEngine.RegisterPolicy("template-003", 80, admission.PolicyTemplate003)

	return &APIServer{
		store:          store,
		router:         gin.New(),
		admission:      admissionEngine,
		scheduler:      scheduler.NewScheduler(),
		crdRegistry:    crd.NewRegistry(),
		modeManager:    modes.NewManager(),
		vectorIndex:    vectorplus.NewIndex(4),
		reviewPipeline: reviewflow.NewPipeline(),
	}
}

// RegisterRoutes registers all API routes
func (as *APIServer) RegisterRoutes() {
	api := as.router.Group("/api/v1")

	// Generic resource endpoints
	api.POST("/:namespace/:kind", as.CreateResource)
	api.GET(routeNsKindName, as.GetResource)
	api.PUT(routeNsKindName, as.UpdateResource)
	api.DELETE(routeNsKindName, as.DeleteResource)
	api.GET("/:namespace/:kind", as.ListResources)

	// Apply endpoint (CLI applies resources here)
	api.POST("/:namespace/:kind/apply", as.ApplyResource)

	// Status subresource
	api.GET(routeNsKindName+"/status", as.GetResourceStatus)
	api.PUT(routeNsKindName+"/status", as.UpdateResourceStatus)

	// Extended module routes for kubeplus, netintel modes, vectorplus, and reviewflow.
	as.registerFeatureRoutes(api)
}

func (as *APIServer) registerFeatureRoutes(api *gin.RouterGroup) {
	kubeplusAPI := api.Group("/kubeplus")
	{
		kubeplusAPI.GET("/admission/policies", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"policies": as.admission.ListPolicies()})
		})
		kubeplusAPI.POST("/admission/evaluate", func(c *gin.Context) {
			var req admission.AdmissionRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if req.Timestamp.IsZero() {
				req.Timestamp = time.Now().UTC()
			}
			c.JSON(http.StatusOK, as.admission.Evaluate(req))
		})

		kubeplusAPI.PUT("/scheduler/nodes/:name", func(c *gin.Context) {
			var node scheduler.Node
			if err := c.ShouldBindJSON(&node); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			node.Name = c.Param("name")
			if strings.TrimSpace(node.Name) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "node name is required"})
				return
			}
			as.scheduler.UpsertNode(node)
			c.JSON(http.StatusOK, gin.H{"message": "node upserted", "node": node.Name})
		})
		kubeplusAPI.GET("/scheduler/nodes", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"nodes": as.scheduler.ListNodes()})
		})
		kubeplusAPI.POST("/scheduler/score", func(c *gin.Context) {
			var w scheduler.Workload
			if err := c.ShouldBindJSON(&w); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"decisions": as.scheduler.Score(w)})
		})
		kubeplusAPI.POST("/scheduler/pick", func(c *gin.Context) {
			var w scheduler.Workload
			if err := c.ShouldBindJSON(&w); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			best, ok := as.scheduler.PickBest(w)
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "no suitable node found"})
				return
			}
			c.JSON(http.StatusOK, best)
		})

		kubeplusAPI.POST("/crd/definitions", func(c *gin.Context) {
			var def crd.Definition
			if err := c.ShouldBindJSON(&def); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if err := as.crdRegistry.Register(def); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "definition registered"})
		})
		kubeplusAPI.GET("/crd/definitions", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"definitions": as.crdRegistry.List()})
		})
		kubeplusAPI.POST("/crd/validate", func(c *gin.Context) {
			var body struct {
				Group   string         `json:"group"`
				Kind    string         `json:"kind"`
				Version string         `json:"version"`
				Spec    map[string]any `json:"spec"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			def, ok := as.crdRegistry.Get(body.Group, body.Kind)
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "definition not found"})
				return
			}
			if len(def.Versions) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "definition has no versions"})
				return
			}
			fields := def.Versions[0].Fields
			if strings.TrimSpace(body.Version) != "" {
				for _, v := range def.Versions {
					if v.Version == body.Version {
						fields = v.Fields
						break
					}
				}
			}
			c.JSON(http.StatusOK, crd.ValidateSpec(fields, body.Spec))
		})
	}

	netintelAPI := api.Group("/netintel")
	{
		netintelAPI.GET("/modes", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"modes": as.modeManager.List()})
		})
		netintelAPI.PUT("/modes/:name", func(c *gin.Context) {
			var cfg modes.ModeConfig
			if err := c.ShouldBindJSON(&cfg); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Name = modes.Mode(strings.ToLower(strings.TrimSpace(c.Param("name"))))
			if cfg.Name == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "mode name is required"})
				return
			}
			as.modeManager.Upsert(cfg)
			c.JSON(http.StatusOK, gin.H{"message": "mode upserted", "mode": cfg})
		})
		netintelAPI.POST("/modes/events", func(c *gin.Context) {
			var ev modes.ModeEvent
			if err := c.ShouldBindJSON(&ev); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if ev.Timestamp.IsZero() {
				ev.Timestamp = time.Now().UTC()
			}
			as.modeManager.Record(ev)
			c.JSON(http.StatusOK, gin.H{"message": "event recorded"})
		})
		netintelAPI.GET("/modes/:name/events", func(c *gin.Context) {
			name := modes.Mode(strings.ToLower(strings.TrimSpace(c.Param("name"))))
			c.JSON(http.StatusOK, gin.H{"events": as.modeManager.FindByMode(name)})
		})
		netintelAPI.POST("/modes/detect", func(c *gin.Context) {
			var body struct {
				Detector int       `json:"detector"`
				Samples  []float64 `json:"samples"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			var score float64
			switch body.Detector {
			case 2:
				score = modes.Detector002(body.Samples)
			case 3:
				score = modes.Detector003(body.Samples)
			case 4:
				score = modes.Detector004(body.Samples)
			case 5:
				score = modes.Detector005(body.Samples)
			default:
				score = modes.Detector001(body.Samples)
			}
			c.JSON(http.StatusOK, gin.H{"detector": body.Detector, "score": score})
		})
	}

	vectorAPI := api.Group("/vectorplus")
	{
		vectorAPI.PUT("/records/:id", func(c *gin.Context) {
			var body struct {
				Vec    []float64         `json:"vec"`
				Labels map[string]string `json:"labels"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			rec := vectorplus.Record{ID: c.Param("id"), Vec: vectorplus.Vector(body.Vec), Labels: body.Labels}
			if !as.vectorIndex.Upsert(rec) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vector size or id"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "record upserted", "id": rec.ID})
		})
		vectorAPI.DELETE("/records/:id", func(c *gin.Context) {
			as.vectorIndex.Delete(c.Param("id"))
			c.JSON(http.StatusOK, gin.H{"message": "record deleted", "id": c.Param("id")})
		})
		vectorAPI.POST("/search", func(c *gin.Context) {
			var body struct {
				Query []float64 `json:"query"`
				K     int       `json:"k"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if body.K < 1 {
				body.K = 5
			}
			c.JSON(http.StatusOK, gin.H{"results": as.vectorIndex.Search(vectorplus.Vector(body.Query), body.K)})
		})
		vectorAPI.POST("/similarity", func(c *gin.Context) {
			var body struct {
				A      []float64 `json:"a"`
				B      []float64 `json:"b"`
				Metric int       `json:"metric"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			a := vectorplus.Vector(body.A)
			b := vectorplus.Vector(body.B)
			var score float64
			switch body.Metric {
			case 2:
				score = vectorplus.SimilarityMetric002(a, b)
			case 3:
				score = vectorplus.SimilarityMetric003(a, b)
			case 4:
				score = vectorplus.SimilarityMetric004(a, b)
			case 5:
				score = vectorplus.SimilarityMetric005(a, b)
			default:
				score = vectorplus.SimilarityMetric001(a, b)
			}
			c.JSON(http.StatusOK, gin.H{"metric": body.Metric, "score": score})
		})
	}

	reviewAPI := api.Group("/reviewflow")
	{
		reviewAPI.PUT("/items/:id", func(c *gin.Context) {
			var item reviewflow.ReviewItem
			if err := c.ShouldBindJSON(&item); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			item.ID = c.Param("id")
			if strings.TrimSpace(item.ID) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "item id is required"})
				return
			}
			if item.Score == 0 {
				item.Score = reviewflow.ScoreBySignals(item.Title, item.Description, item.Tags)
			}
			as.reviewPipeline.Upsert(item)
			c.JSON(http.StatusOK, gin.H{"message": "item upserted", "id": item.ID})
		})
		reviewAPI.GET("/items/:id", func(c *gin.Context) {
			item, ok := as.reviewPipeline.Get(c.Param("id"))
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
				return
			}
			c.JSON(http.StatusOK, item)
		})
		reviewAPI.GET("/items", func(c *gin.Context) {
			stage := reviewflow.Stage(strings.TrimSpace(c.Query("stage")))
			c.JSON(http.StatusOK, gin.H{"items": as.reviewPipeline.ListByStage(stage)})
		})
		reviewAPI.POST("/items/:id/stage", func(c *gin.Context) {
			var body struct {
				Stage reviewflow.Stage `json:"stage"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if !as.reviewPipeline.Advance(c.Param("id"), body.Stage) {
				c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "stage updated", "id": c.Param("id"), "stage": body.Stage})
		})
		reviewAPI.POST("/score", func(c *gin.Context) {
			var body struct {
				Title       string   `json:"title"`
				Description string   `json:"description"`
				Tags        []string `json:"tags"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"score": reviewflow.ScoreBySignals(body.Title, body.Description, body.Tags)})
		})
		reviewAPI.POST("/quality", func(c *gin.Context) {
			var body struct {
				Item  reviewflow.ReviewItem `json:"item"`
				Check int                   `json:"check"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			var score float64
			switch body.Check {
			case 2:
				score = reviewflow.QualityCheck002(body.Item)
			case 3:
				score = reviewflow.QualityCheck003(body.Item)
			case 4:
				score = reviewflow.QualityCheck004(body.Item)
			case 5:
				score = reviewflow.QualityCheck005(body.Item)
			default:
				score = reviewflow.QualityCheck001(body.Item)
			}
			c.JSON(http.StatusOK, gin.H{"check": body.Check, "score": score})
		})
	}
}

// CreateResource creates a new resource
func (as *APIServer) CreateResource(c *gin.Context) {
	kind := c.Param("kind")
	namespace := c.Param("namespace")

	var resource resources.Resource

	// Unmarshal based on kind
	switch kind {
	case "workloads":
		var wr resources.WorkloadResource
		if err := c.ShouldBindJSON(&wr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		wr.ObjectMeta.Namespace = namespace
		resource = &wr

	case "pipelines":
		var pr resources.PipelineResource
		if err := c.ShouldBindJSON(&pr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		pr.ObjectMeta.Namespace = namespace
		resource = &pr

	case "schedules":
		var sr resources.ScheduleResource
		if err := c.ShouldBindJSON(&sr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		sr.ObjectMeta.Namespace = namespace
		resource = &sr

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf(errUnknownKind, kind)})
		return
	}

	if err := as.store.Create(resource); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, resource)
}

// GetResource gets a resource
func (as *APIServer) GetResource(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")

	resource, err := as.store.Get(namespace, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resource)
}

// UpdateResource updates a resource
func (as *APIServer) UpdateResource(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")
	kind := c.Param("kind")

	// Get existing resource (just to verify it exists)
	_, err := as.store.Get(namespace, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Parse new resource
	var resource resources.Resource
	switch kind {
	case "workloads":
		var wr resources.WorkloadResource
		if err := c.ShouldBindJSON(&wr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		wr.ObjectMeta.Namespace = namespace
		wr.ObjectMeta.Name = name
		wr.ObjectMeta.UpdatedAt = time.Now()
		wr.ObjectMeta.Generation++
		resource = &wr

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf(errUnknownKind, kind)})
		return
	}

	if err := as.store.Update(resource); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resource)
}

// DeleteResource deletes a resource
func (as *APIServer) DeleteResource(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")

	if err := as.store.Delete(namespace, name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ListResources lists resources
func (as *APIServer) ListResources(c *gin.Context) {
	namespace := c.Param("namespace")

	// Parse label selector
	selector := make(map[string]string)
	if labelSelector := c.Query("labelSelector"); labelSelector != "" {
		// Parse selector format: key1=value1,key2=value2
		for _, pair := range strings.Split(labelSelector, ",") {
			parts := strings.Split(pair, "=")
			if len(parts) == 2 {
				selector[parts[0]] = parts[1]
			}
		}
	}

	resources, err := as.store.List(namespace, selector)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": resources})
}

// GetResourceStatus gets resource status
func (as *APIServer) GetResourceStatus(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")

	resource, err := as.store.Get(namespace, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resource.GetStatus())
}

// parseResourceByKind unmarshals a resource from the request based on its kind.
func (as *APIServer) parseResourceByKind(c *gin.Context, kind, namespace string) (resources.Resource, bool) {
	switch kind {
	case "workloads":
		var wr resources.WorkloadResource
		if err := c.ShouldBindJSON(&wr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return nil, false
		}
		wr.ObjectMeta.Namespace = namespace
		return &wr, true

	case "pipelines":
		var pr resources.PipelineResource
		if err := c.ShouldBindJSON(&pr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return nil, false
		}
		pr.ObjectMeta.Namespace = namespace
		return &pr, true

	case "schedules":
		var sr resources.ScheduleResource
		if err := c.ShouldBindJSON(&sr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return nil, false
		}
		sr.ObjectMeta.Namespace = namespace
		return &sr, true

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf(errUnknownKind, kind)})
		return nil, false
	}
}

// ApplyResource applies a resource (create or update + enqueue for reconciliation)
func (as *APIServer) ApplyResource(c *gin.Context) {
	kind := c.Param("kind")
	namespace := c.Param("namespace")

	resource, ok := as.parseResourceByKind(c, kind, namespace)
	if !ok {
		return
	}

	// Check if it already exists
	meta := resource.GetObjectMeta()
	existing, err := as.store.Get(namespace, meta.Name)

	if err != nil {
		// Create new resource
		resource.GetObjectMeta().Generation = 1
		resource.GetObjectMeta().CreatedAt = time.Now()
		resource.GetObjectMeta().UpdatedAt = time.Now()

		if err := as.store.Create(resource); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
	} else {
		// Update existing resource
		if existing.GetObjectMeta().Generation != meta.Generation {
			c.JSON(http.StatusConflict, gin.H{"error": "Generation mismatch"})
			return
		}

		resource.GetObjectMeta().Generation = existing.GetObjectMeta().Generation + 1
		resource.GetObjectMeta().CreatedAt = existing.GetObjectMeta().CreatedAt
		resource.GetObjectMeta().UpdatedAt = time.Now()

		if err := as.store.Update(resource); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Return with status=Pending to indicate reconciliation will run
	status := resource.GetStatus()
	if status == nil {
		status = &resources.ObjectStatus{}
	}
	status.Phase = "Pending"

	c.JSON(http.StatusAccepted, gin.H{
		"kind":       kind,
		"name":       meta.Name,
		"namespace":  namespace,
		"generation": resource.GetObjectMeta().Generation,
		"status":     status,
	})
}

// UpdateResourceStatus updates resource status
func (as *APIServer) UpdateResourceStatus(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")

	resource, err := as.store.Get(namespace, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var status resources.ObjectStatus
	if err := c.ShouldBindJSON(&status); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status.LastTransitionTime = time.Now()
	resource.SetStatus(&status)

	if err := as.store.Update(resource); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resource.GetStatus())
}

// Run starts the API server
func (as *APIServer) Run(addr string) error {
	as.RegisterRoutes()
	return as.router.Run(addr)
}

// Router returns the underlying http.Handler for use with custom http.Server
func (as *APIServer) Router() http.Handler {
	return as.router
}
