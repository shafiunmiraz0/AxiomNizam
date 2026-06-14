package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

// GenericResource is the wire-format DTO served by the /api/v1/resources
// endpoints. It is intentionally JSON-stable (clients rely on this shape via
// the Postman collections) and acts as a thin projection of the canonical
// BaseResource used elsewhere in the control plane.
//
// Invariants (P0.3):
//   - This type DOES NOT drive reconciliation. Status transitions must be
//     performed by a real controller reading from the work queue, not by a
//     fake `go func(){ sleep; Ready }` simulation inside the HTTP handler.
//   - GenericResource satisfies reconciler.Resource via GetKey / GetGeneration
//     / GetObservedGeneration so it can be enqueued for reconciliation.
type GenericResource struct {
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Metadata   GenericResourceMetadata `json:"metadata"`
	Spec       map[string]interface{} `json:"spec,omitempty"`
	Status     GenericResourceStatus  `json:"status,omitempty"`
}

// GetKey implements reconciler.Resource. Returns "namespace/name".
func (g *GenericResource) GetKey() string {
	if g == nil {
		return ""
	}
	if g.Metadata.Namespace == "" {
		return g.Metadata.Name
	}
	return g.Metadata.Namespace + "/" + g.Metadata.Name
}

// GetGeneration implements reconciler.Resource.
func (g *GenericResource) GetGeneration() int64 {
	if g == nil {
		return 0
	}
	return g.Metadata.Generation
}

// GetObservedGeneration implements reconciler.Resource.
func (g *GenericResource) GetObservedGeneration() int64 {
	if g == nil {
		return 0
	}
	return g.Status.ObservedGeneration
}

// GenericResourceMetadata holds resource metadata. The field set and JSON tags are
// kept backward-compatible with pre-P0 clients.
type GenericResourceMetadata struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	UID               string            `json:"uid,omitempty"`
	Generation        int64             `json:"generation,omitempty"`
	CreationTimestamp string            `json:"creationTimestamp,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
}

// GenericResourceStatus holds resource status. ObservedGeneration was added in P0.1
// so the HTTP-surface DTO participates in generation-based reconciliation.
type GenericResourceStatus struct {
	Phase              string                   `json:"phase,omitempty"`
	Conditions         []map[string]interface{} `json:"conditions,omitempty"`
	Message            string                   `json:"message,omitempty"`
	ObservedGeneration int64                    `json:"observedGeneration,omitempty"`
}

// GenericResourceHandler manages generic Kubernetes-style resources
type GenericResourceHandler struct {
	mu        sync.RWMutex
	resources map[string]map[string]map[string]*GenericResource // kind -> namespace -> name -> resource
	etcd      *clientv3.Client
	stateKey  string
}

// NewGenericResourceHandler creates a new resource handler
func NewGenericResourceHandler(etcd *clientv3.Client) *GenericResourceHandler {
	h := &GenericResourceHandler{
		resources: make(map[string]map[string]map[string]*GenericResource),
		etcd:      etcd,
		stateKey:  "handlers:resources:state",
	}
	h.loadState()
	return h
}

func (h *GenericResourceHandler) loadState() {
	if h.etcd == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.etcd.Get(ctx, h.stateKey)
	if err != nil {
		logging.Z().Warn("resources: failed to load persisted state", zap.Error(err))
		return
	}
	if len(resp.Kvs) == 0 {
		return
	}

	var resourcesState map[string]map[string]map[string]*GenericResource
	if err := json.Unmarshal(resp.Kvs[0].Value, &resourcesState); err != nil {
		logging.Z().Warn("resources: failed to decode persisted state", zap.Error(err))
		return
	}
	if resourcesState == nil {
		resourcesState = make(map[string]map[string]map[string]*GenericResource)
	}
	h.resources = resourcesState
}

func (h *GenericResourceHandler) persistStateLocked() {
	if h.etcd == nil {
		return
	}

	payload, err := json.Marshal(h.resources)
	if err != nil {
		logging.Z().Warn("resources: failed to encode state", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := h.etcd.Put(ctx, h.stateKey, string(payload)); err != nil {
		logging.Z().Warn("resources: failed to persist state", zap.Error(err))
	}
}

func (h *GenericResourceHandler) ensureKind(kind string) {
	if h.resources[kind] == nil {
		h.resources[kind] = make(map[string]map[string]*GenericResource)
	}
}

func (h *GenericResourceHandler) ensureNamespace(kind, ns string) {
	h.ensureKind(kind)
	if h.resources[kind][ns] == nil {
		h.resources[kind][ns] = make(map[string]*GenericResource)
	}
}

// CreateOrUpdate creates or updates a resource (POST)
func (h *GenericResourceHandler) CreateOrUpdate(c *gin.Context) {
	kind := NormalizeKind(c.Param("kind"))
	ns := c.Param("namespace")
	if ns == "" {
		ns = "default"
	}

	var resource GenericResource
	if err := c.ShouldBindJSON(&resource); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.ensureNamespace(kind, ns)

	name := resource.Metadata.Name
	if name == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "metadata.name is required"})
		return
	}

	existing := h.resources[kind][ns][name]
	now := time.Now().UTC().Format(time.RFC3339)

	if existing != nil {
		resource.Metadata.UID = existing.Metadata.UID
		resource.Metadata.CreationTimestamp = existing.Metadata.CreationTimestamp
		resource.Metadata.Generation = existing.Metadata.Generation + 1
		resource.Metadata.Namespace = ns
		resource.Status.Phase = "Reconciling"
		h.resources[kind][ns][name] = &resource
		h.persistStateLocked()
		c.JSON(http.StatusOK, &resource)
	} else {
		resource.Metadata.UID = uuid.New().String()
		resource.Metadata.CreationTimestamp = now
		resource.Metadata.Generation = 1
		resource.Metadata.Namespace = ns
		resource.Status.Phase = "Pending"
		h.resources[kind][ns][name] = &resource
		h.persistStateLocked()
		c.JSON(http.StatusCreated, &resource)
	}
}

// Get retrieves a resource by namespace/kind/name
func (h *GenericResourceHandler) Get(c *gin.Context) {
	kind := NormalizeKind(c.Param("kind"))
	ns := c.Param("namespace")
	name := c.Param("name")

	if ns == "" {
		ns = "default"
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.resources[kind] == nil || h.resources[kind][ns] == nil || h.resources[kind][ns][name] == nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: fmt.Sprintf("resource not found: %s/%s/%s", kind, ns, name)})
		return
	}

	c.JSON(http.StatusOK, h.resources[kind][ns][name])
}

// List returns all resources of a kind in a namespace
func (h *GenericResourceHandler) List(c *gin.Context) {
	kind := NormalizeKind(c.Param("kind"))
	ns := c.Param("namespace")

	if ns == "" {
		ns = "default"
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.resources[kind] == nil || h.resources[kind][ns] == nil {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	items := make([]*GenericResource, 0, len(h.resources[kind][ns]))
	for _, r := range h.resources[kind][ns] {
		items = append(items, r)
	}

	c.JSON(http.StatusOK, items)
}

// Update updates a resource (PUT)
func (h *GenericResourceHandler) Update(c *gin.Context) {
	kind := NormalizeKind(c.Param("kind"))
	ns := c.Param("namespace")
	name := c.Param("name")

	if ns == "" {
		ns = "default"
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.resources[kind] == nil || h.resources[kind][ns] == nil || h.resources[kind][ns][name] == nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: fmt.Sprintf("resource not found: %s/%s/%s", kind, ns, name)})
		return
	}

	existing := h.resources[kind][ns][name]

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	if existing.Spec == nil {
		existing.Spec = make(map[string]interface{})
	}
	for k, v := range updates {
		existing.Spec[k] = v
	}
	existing.Metadata.Generation++
	existing.Status.Phase = "Reconciling"
	h.persistStateLocked()

	c.JSON(http.StatusOK, existing)
}

// Delete removes a resource
func (h *GenericResourceHandler) Delete(c *gin.Context) {
	kind := NormalizeKind(c.Param("kind"))
	ns := c.Param("namespace")
	name := c.Param("name")

	if ns == "" {
		ns = "default"
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.resources[kind] == nil || h.resources[kind][ns] == nil || h.resources[kind][ns][name] == nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: fmt.Sprintf("resource not found: %s/%s/%s", kind, ns, name)})
		return
	}

	delete(h.resources[kind][ns], name)
	h.persistStateLocked()
	c.JSON(http.StatusOK, MessageResponse{Message: fmt.Sprintf("%s '%s' deleted", kind, name)})
}

// GetStatus returns the status subresource
func (h *GenericResourceHandler) GetStatus(c *gin.Context) {
	kind := NormalizeKind(c.Param("kind"))
	ns := c.Param("namespace")
	name := c.Param("name")

	if ns == "" {
		ns = "default"
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.resources[kind] == nil || h.resources[kind][ns] == nil || h.resources[kind][ns][name] == nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "resource not found"})
		return
	}

	c.JSON(http.StatusOK, h.resources[kind][ns][name].Status)
}

// ListAll returns all resources of a kind across all namespaces (for non-namespaced queries)
func (h *GenericResourceHandler) ListAll(c *gin.Context) {
	kind := NormalizeKind(c.Param("kind"))

	h.mu.RLock()
	defer h.mu.RUnlock()

	items := make([]*GenericResource, 0)

	if h.resources[kind] != nil {
		for _, nsResources := range h.resources[kind] {
			for _, r := range nsResources {
				items = append(items, r)
			}
		}
	}

	c.JSON(http.StatusOK, items)
}

// Events returns events for a resource
func (h *GenericResourceHandler) Events(c *gin.Context) {
	kind := NormalizeKind(c.Param("kind"))
	ns := c.Param("namespace")
	name := c.Param("name")

	if ns == "" {
		ns = "default"
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.resources[kind] == nil || h.resources[kind][ns] == nil || h.resources[kind][ns][name] == nil {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	r := h.resources[kind][ns][name]
	events := []map[string]interface{}{
		{
			"type":    "Normal",
			"reason":  "Created",
			"message": fmt.Sprintf("%s '%s' was created", r.Kind, name),
			"age":     r.Metadata.CreationTimestamp,
		},
		{
			"type":    "Normal",
			"reason":  "Reconciled",
			"message": fmt.Sprintf("%s '%s' reconciled to generation %d", r.Kind, name, r.Metadata.Generation),
			"age":     time.Now().UTC().Format(time.RFC3339),
		},
	}

	c.JSON(http.StatusOK, events)
}

// FindResourceByKindAndName finds the first matching resource across namespaces.
// It returns a defensive copy to avoid callers mutating internal handler state.
func (h *GenericResourceHandler) FindResourceByKindAndName(kind, name string) (*GenericResource, bool) {
	normalizedKind := NormalizeKind(kind)

	h.mu.RLock()
	defer h.mu.RUnlock()

	for storedKind, byNamespace := range h.resources {
		for _, byName := range byNamespace {
			for _, res := range byName {
				if !strings.EqualFold(res.Metadata.Name, name) {
					continue
				}

				if normalizedKind != "" {
					if !(strings.EqualFold(storedKind, normalizedKind) ||
						strings.EqualFold(res.Kind, normalizedKind) ||
						strings.EqualFold(res.Kind, kind)) {
						continue
					}
				}

				return CloneGenericResource(res), true
			}
		}
	}

	return nil, false
}

// CloneGenericResource creates a deep copy of a GenericResource.
func CloneGenericResource(in *GenericResource) *GenericResource {
	if in == nil {
		return nil
	}

	out := *in
	out.Metadata.Labels = CloneStringMap(in.Metadata.Labels)
	out.Metadata.Annotations = CloneStringMap(in.Metadata.Annotations)
	out.Spec = CloneAnyMap(in.Spec)

	if len(in.Status.Conditions) > 0 {
		out.Status.Conditions = make([]map[string]interface{}, len(in.Status.Conditions))
		for i := range in.Status.Conditions {
			out.Status.Conditions[i] = CloneAnyMap(in.Status.Conditions[i])
		}
	}

	return &out
}

// CloneStringMap creates a copy of a string map.
func CloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}

	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// CloneAnyMap creates a copy of an interface{} map.
func CloneAnyMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}

	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// NormalizeKind normalizes resource kind names for consistent storage
func NormalizeKind(kind string) string {
	kind = strings.ToLower(kind)
	switch kind {
	case "apis", "api":
		return "apis"
	case "policies", "policy":
		return "policies"
	case "workflows", "workflow":
		return "workflows"
	case "datasources", "datasource":
		return "datasources"
	case "jobs", "job":
		return "jobs"
	default:
		return kind
	}
}
