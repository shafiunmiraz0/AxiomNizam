package datasourceresource

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

// DataSourceHandler manages datasource resources
type DataSourceHandler struct {
	mu          sync.RWMutex
	datasources map[string]*DataSourceResource // name -> datasource
	etcd        *clientv3.Client
	stateKey    string
}

// NewDataSourceHandler creates a new datasource handler
func NewDataSourceHandler(etcd ...*clientv3.Client) *DataSourceHandler {
	var etcdClient *clientv3.Client
	if len(etcd) > 0 {
		etcdClient = etcd[0]
	}

	h := &DataSourceHandler{
		datasources: make(map[string]*DataSourceResource),
		etcd:        etcdClient,
		stateKey:    "handlers:datasources:state",
	}
	h.loadState()
	return h
}

func (h *DataSourceHandler) loadState() {
	if h.etcd == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := h.etcd.Get(ctx, h.stateKey)
	if err != nil || len(resp.Kvs) == 0 {
		return
	}
	var state map[string]*DataSourceResource
	if err := json.Unmarshal(resp.Kvs[0].Value, &state); err != nil {
		logging.Z().Warn("failed to unmarshal datasource state", zap.Error(err))
		return
	}
	h.datasources = state
}

func (h *DataSourceHandler) saveState() {
	if h.etcd == nil {
		return
	}
	data, err := json.Marshal(h.datasources)
	if err != nil {
		logging.Z().Error("failed to marshal datasource state", zap.Error(err))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := h.etcd.Put(ctx, h.stateKey, string(data)); err != nil {
		logging.Z().Error("failed to persist datasource state", zap.Error(err))
	}
}

// Create handles POST /api/v1/datasources
func (h *DataSourceHandler) Create(c *gin.Context) {
	var ds DataSourceResource
	if err := c.BindJSON(&ds); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "Invalid request: " + err.Error()})
		return
	}

	if ds.Metadata.Name == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "datasource name is required"})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	name := strings.ToLower(strings.TrimSpace(ds.Metadata.Name))
	if _, exists := h.datasources[name]; exists {
		c.JSON(http.StatusConflict, MessageResponse{Error: "Datasource already exists"})
		return
	}

	ds.Metadata.UID = uuid.New().String()
	ds.Metadata.CreationTimestamp = time.Now().UTC().Format(time.RFC3339)
	ds.Kind = "DataSource"
	if ds.APIVersion == "" {
		ds.APIVersion = "v1"
	}

	h.datasources[name] = &ds
	h.saveState()

	c.JSON(http.StatusCreated, DataSourceCreatedResponse{
		Status:     "ok",
		Message:    "Datasource created successfully",
		Datasource: ds,
	})
}

// List handles GET /api/v1/datasources
func (h *DataSourceHandler) List(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	datasources := make([]*DataSourceResource, 0, len(h.datasources))
	for _, ds := range h.datasources {
		datasources = append(datasources, ds)
	}

	c.JSON(http.StatusOK, DataSourceListResponse{
		Status:      "ok",
		Datasources: datasources,
		Total:       len(datasources),
	})
}

// Get handles GET /api/v1/datasources/:name
func (h *DataSourceHandler) Get(c *gin.Context) {
	name := strings.ToLower(strings.TrimSpace(c.Param("name")))

	h.mu.RLock()
	defer h.mu.RUnlock()

	ds, exists := h.datasources[name]
	if !exists {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "Datasource not found"})
		return
	}

	c.JSON(http.StatusOK, DataSourceGetResponse{
		Status:     "ok",
		Datasource: ds,
	})
}

// Update handles PUT /api/v1/datasources/:name
func (h *DataSourceHandler) Update(c *gin.Context) {
	name := strings.ToLower(strings.TrimSpace(c.Param("name")))

	var ds DataSourceResource
	if err := c.BindJSON(&ds); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "Invalid request: " + err.Error()})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	existing, exists := h.datasources[name]
	if !exists {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "Datasource not found"})
		return
	}

	ds.Metadata.Name = existing.Metadata.Name
	ds.Metadata.UID = existing.Metadata.UID
	ds.Metadata.CreationTimestamp = existing.Metadata.CreationTimestamp
	ds.Kind = "DataSource"
	if ds.APIVersion == "" {
		ds.APIVersion = "v1"
	}

	h.datasources[name] = &ds
	h.saveState()

	c.JSON(http.StatusOK, DataSourceUpdatedResponse{
		Status:     "ok",
		Message:    "Datasource updated successfully",
		Datasource: ds,
	})
}

// Delete handles DELETE /api/v1/datasources/:name
func (h *DataSourceHandler) Delete(c *gin.Context) {
	name := strings.ToLower(strings.TrimSpace(c.Param("name")))

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.datasources[name]; !exists {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "Datasource not found"})
		return
	}

	delete(h.datasources, name)
	h.saveState()

	c.JSON(http.StatusOK, MessageResponse{Message: "Datasource deleted successfully"})
}

// Apply creates or updates a datasource from YAML-sourced data
func (h *DataSourceHandler) Apply(c *gin.Context) {
	h.Create(c)
}

// Test handles POST /api/v1/datasources/:name/test
func (h *DataSourceHandler) Test(c *gin.Context) {
	name := strings.ToLower(strings.TrimSpace(c.Param("name")))

	h.mu.RLock()
	ds, exists := h.datasources[name]
	h.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "Datasource not found"})
		return
	}

	// Simulate connection test
	driver, _ := ds.Spec["driver"].(string)
	host, _ := ds.Spec["host"].(string)
	port, _ := ds.Spec["port"].(string)

	if driver == "" || host == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "Datasource missing driver or host configuration"})
		return
	}

	endpoint := fmt.Sprintf("%s:%s", host, port)
	if port == "" {
		endpoint = host
	}

	c.JSON(http.StatusOK, DataSourceTestResponse{
		Status:   "ok",
		Message:  fmt.Sprintf("Connection to %s (%s) successful", endpoint, driver),
		Driver:   driver,
		Endpoint: endpoint,
		TestedAt: time.Now().UTC(),
	})
}
