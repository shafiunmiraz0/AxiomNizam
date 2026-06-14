package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/bulk"
	"axiomnizam.bitbd.net/axiomnizam/internal/database"
	"axiomnizam.bitbd.net/axiomnizam/internal/eventbus"
	exportpkg "axiomnizam.bitbd.net/axiomnizam/internal/export"
	"axiomnizam.bitbd.net/axiomnizam/internal/streaming"
	"axiomnizam.bitbd.net/axiomnizam/internal/tenant"
	"axiomnizam.bitbd.net/axiomnizam/internal/versioning"
	"axiomnizam.bitbd.net/axiomnizam/internal/webhooks"
	"github.com/gin-gonic/gin"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func newEtcdTestStore(t *testing.T, prefix string) *platformStateStore {
	t.Helper()

	endpoint := os.Getenv("ETCD_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:2379"
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Skipf("skipping etcd-backed test: unable to connect to etcd at %s: %v", endpoint, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := client.Get(ctx, "__platform_test_health__"); err != nil {
		_ = client.Close()
		t.Skipf("skipping etcd-backed test: etcd health check failed at %s: %v", endpoint, err)
	}

	t.Cleanup(func() {
		_ = client.Close()
	})

	uniquePrefix := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	return newPlatformStateStore(&database.Connections{Etcd: client}, uniquePrefix)
}

func testRequest(t *testing.T, router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()

	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestPlatformEndpointGroupsIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newEtcdTestStore(t, "test-platform")

	bulkMgr := newPersistentBulkManager(store)
	eventMgr := newPersistentEventBusManager(store)
	exportMgr := &exportManagerAdapter{base: newPersistentExportCoreManager(store)}
	streamMgr := newPersistentStreamManager(store)
	webhookMgr := newPersistentWebhookManager(store)
	tenantMgr := newPersistentTenantManager(store)
	versionMgr := newPersistentVersionManager(store)

	router := gin.New()

	bulkHandler := bulk.NewBulkHandler(bulkMgr)
	eventHandler := eventbus.NewEventBusHandler(eventMgr)
	exportHandler := exportpkg.NewExportHandler(exportMgr)
	streamHandler := streaming.NewStreamHandler(streamMgr)
	webhookHandler := webhooks.NewWebhookHandler(webhookMgr)
	tenantHandler := tenant.NewTenantHandler(tenantMgr)
	versionHandler := versioning.NewVersionHandler(versionMgr)

	bulkAPI := router.Group("/api/v1/bulk/operations")
	{
		bulkAPI.POST("", bulkHandler.SubmitBulkOperation)
		bulkAPI.GET("", bulkHandler.ListOperations)
	}

	eventAPI := router.Group("/api/v1/eventbus")
	{
		eventAPI.POST("/topics", eventHandler.CreateTopic)
		eventAPI.GET("/topics", eventHandler.ListTopics)
	}

	exportAPI := router.Group("/api/v1/exports")
	{
		exportAPI.POST("", exportHandler.SubmitExport)
		exportAPI.GET("", exportHandler.ListExports)
	}

	streamAPI := router.Group("/api/v1/streams")
	{
		streamAPI.POST("", streamHandler.CreateStreamRequest)
		streamAPI.GET("", streamHandler.ListStreams)
	}

	webhookAPI := router.Group("/api/v1/webhooks")
	{
		webhookAPI.POST("", webhookHandler.CreateWebhook)
		webhookAPI.GET("", webhookHandler.ListWebhooks)
	}

	tenantAPI := router.Group("/api/v1/tenants")
	{
		tenantAPI.POST("", tenantHandler.CreateTenant)
		tenantAPI.GET("", tenantHandler.ListTenants)
	}

	versionAPI := router.Group("/api/v1/versioning")
	{
		versionAPI.POST("/snapshots/:resourceType/:resourceId", versionHandler.CreateSnapshot)
		versionAPI.GET("/history/:resourceType/:resourceId", versionHandler.GetHistory)
	}

	bulkCreate := testRequest(t, router, http.MethodPost, "/api/v1/bulk/operations", map[string]interface{}{
		"tenantId": "tenant-a",
		"type":     "CREATE",
		"items": []map[string]interface{}{
			{"id": "item-1"},
		},
	})
	if bulkCreate.Code != http.StatusAccepted {
		t.Fatalf("expected bulk create 202, got %d: %s", bulkCreate.Code, bulkCreate.Body.String())
	}

	bulkList := testRequest(t, router, http.MethodGet, "/api/v1/bulk/operations?tenantId=tenant-a", nil)
	if bulkList.Code != http.StatusOK {
		t.Fatalf("expected bulk list 200, got %d: %s", bulkList.Code, bulkList.Body.String())
	}

	topicCreate := testRequest(t, router, http.MethodPost, "/api/v1/eventbus/topics", map[string]interface{}{
		"name":       "orders",
		"partitions": 1,
	})
	if topicCreate.Code != http.StatusCreated {
		t.Fatalf("expected topic create 201, got %d: %s", topicCreate.Code, topicCreate.Body.String())
	}

	topicList := testRequest(t, router, http.MethodGet, "/api/v1/eventbus/topics", nil)
	if topicList.Code != http.StatusOK {
		t.Fatalf("expected topic list 200, got %d: %s", topicList.Code, topicList.Body.String())
	}

	exportCreate := testRequest(t, router, http.MethodPost, "/api/v1/exports", map[string]interface{}{
		"tenantId": "tenant-a",
		"name":     "orders-export",
		"format":   "CSV",
		"source": map[string]interface{}{
			"type":     "table",
			"database": "main",
			"table":    "orders",
		},
		"destination": map[string]interface{}{
			"type": "local",
			"path": "/tmp",
		},
	})
	if exportCreate.Code != http.StatusAccepted {
		t.Fatalf("expected export create 202, got %d: %s", exportCreate.Code, exportCreate.Body.String())
	}

	streamCreate := testRequest(t, router, http.MethodPost, "/api/v1/streams", map[string]interface{}{
		"tenantId": "tenant-a",
		"query":    "select 1",
	})
	if streamCreate.Code != http.StatusCreated {
		t.Fatalf("expected stream create 201, got %d: %s", streamCreate.Code, streamCreate.Body.String())
	}

	webhookCreate := testRequest(t, router, http.MethodPost, "/api/v1/webhooks", map[string]interface{}{
		"name":   "ops-hook",
		"url":    "https://example.org/hook",
		"events": []string{"job.completed"},
	})
	if webhookCreate.Code != http.StatusCreated {
		t.Fatalf("expected webhook create 201, got %d: %s", webhookCreate.Code, webhookCreate.Body.String())
	}

	tenantCreate := testRequest(t, router, http.MethodPost, "/api/v1/tenants", map[string]interface{}{
		"name":  "acme",
		"owner": "owner-1",
	})
	if tenantCreate.Code != http.StatusCreated {
		t.Fatalf("expected tenant create 201, got %d: %s", tenantCreate.Code, tenantCreate.Body.String())
	}

	tenantList := testRequest(t, router, http.MethodGet, "/api/v1/tenants?owner=owner-1", nil)
	if tenantList.Code != http.StatusOK {
		t.Fatalf("expected tenant list 200, got %d: %s", tenantList.Code, tenantList.Body.String())
	}

	snapshotCreate := testRequest(t, router, http.MethodPost, "/api/v1/versioning/snapshots/apis/demo-api", map[string]interface{}{
		"name": "first-snapshot",
	})
	if snapshotCreate.Code != http.StatusCreated {
		t.Fatalf("expected snapshot create 201, got %d: %s", snapshotCreate.Code, snapshotCreate.Body.String())
	}

	historyGet := testRequest(t, router, http.MethodGet, "/api/v1/versioning/history/apis/demo-api", nil)
	if historyGet.Code != http.StatusOK {
		t.Fatalf("expected history get 200, got %d: %s", historyGet.Code, historyGet.Body.String())
	}
}

func TestPersistentBulkManagerRehydratesState(t *testing.T) {
	store := newEtcdTestStore(t, "test-persist")
	first := newPersistentBulkManager(store)

	op, err := first.SubmitOperation(&bulk.BulkOperation{
		TenantID: "tenant-z",
		Type:     bulk.BulkOpCreate,
		Items:    []bulk.BulkItem{{ID: "item-1"}},
	})
	if err != nil {
		t.Fatalf("submit operation failed: %v", err)
	}

	second := newPersistentBulkManager(store)
	restored, err := second.GetOperation(op.ID)
	if err != nil {
		t.Fatalf("expected restored operation, got error: %v", err)
	}
	if restored.ID != op.ID {
		t.Fatalf("expected operation ID %s, got %s", op.ID, restored.ID)
	}
}
