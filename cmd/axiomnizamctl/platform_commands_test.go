package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"axiomnizam.bitbd.net/axiomnizam/internal/client"
)

func setupPlatformCommandTestContext(t *testing.T, serverURL string) {
	t.Helper()

	cfgPath := t.TempDir() + "/config.yaml"
	cm, err := client.NewConfigManagerWithPath(cfgPath)
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}

	err = cm.AddOrUpdateContext(&client.Context{
		Name: "test",
		Cluster: &client.ClusterInfo{
			Server: serverURL,
		},
		User:      "tester",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("failed to add context: %v", err)
	}
	if err := cm.SetCurrentContext("test"); err != nil {
		t.Fatalf("failed to set context: %v", err)
	}

	configManager = cm
	apiClient = client.NewClient(serverURL)
}

func TestPlatformCLICommandsHitExpectedEndpoints(t *testing.T) {
	var (
		mu    sync.Mutex
		paths []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.RequestURI())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "path": r.URL.RequestURI()})
	}))
	defer server.Close()

	setupPlatformCommandTestContext(t, server.URL)

	tests := []struct {
		name     string
		run      func() error
		expected string
	}{
		{
			name:     "tenant list",
			run:      func() error { return tenantListCmd.RunE(tenantListCmd, []string{}) },
			expected: "/api/v1/tenants",
		},
		{
			name:     "webhook list",
			run:      func() error { return webhookListCmd.RunE(webhookListCmd, []string{}) },
			expected: "/api/v1/webhooks",
		},
		{
			name:     "stream list",
			run:      func() error { return streamListCmd.RunE(streamListCmd, []string{}) },
			expected: "/api/v1/streams",
		},
		{
			name:     "export list",
			run:      func() error { return exportListCmd.RunE(exportListCmd, []string{}) },
			expected: "/api/v1/exports",
		},
		{
			name:     "bulk list",
			run:      func() error { return bulkListCmd.RunE(bulkListCmd, []string{}) },
			expected: "/api/v1/bulk/operations",
		},
		{
			name:     "version history",
			run:      func() error { return versionHistoryCmd.RunE(versionHistoryCmd, []string{"apis", "orders"}) },
			expected: "/api/v1/versioning/history/apis/orders",
		},
		{
			name:     "trace get",
			run:      func() error { return traceGetCmd.RunE(traceGetCmd, []string{"trace-1"}) },
			expected: "/api/v1/tracing/traces/trace-1",
		},
		{
			name:     "lineage graph",
			run:      func() error { return lineageGraphCmd.RunE(lineageGraphCmd, []string{"apis", "orders"}) },
			expected: "/api/v1/lineage/apis/orders",
		},
		{
			name: "certificate status",
			run: func() error {
				_ = certStatusCmd.Flags().Set("cert", "apiserver")
				return certStatusCmd.RunE(certStatusCmd, []string{})
			},
			expected: "/api/admin/certificates/status?cert=apiserver",
		},
		{
			name: "certificate renew dry run",
			run: func() error {
				_ = certRenewCmd.Flags().Set("cert", "all")
				_ = certRenewCmd.Flags().Set("dry-run", "true")
				return certRenewCmd.RunE(certRenewCmd, []string{})
			},
			expected: "/api/admin/certificates/renew",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mu.Lock()
			before := len(paths)
			mu.Unlock()

			if err := tc.run(); err != nil {
				t.Fatalf("command failed: %v", err)
			}

			mu.Lock()
			defer mu.Unlock()
			if len(paths) != before+1 {
				t.Fatalf("expected one request, got before=%d after=%d", before, len(paths))
			}
			actual := paths[len(paths)-1]
			if actual != tc.expected {
				t.Fatalf("expected path %s, got %s", tc.expected, actual)
			}
		})
	}
}

func TestRBACAccessRequestCLICommands(t *testing.T) {
	var (
		mu           sync.Mutex
		lastPath     string
		lastMethod   string
		lastBody     map[string]interface{}
		requestCount int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		requestCount++
		lastPath = r.URL.RequestURI()
		lastMethod = r.Method
		lastBody = nil

		if r.Body != nil {
			defer r.Body.Close()
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				lastBody = body
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "path": r.URL.RequestURI()})
	}))
	defer server.Close()

	setupPlatformCommandTestContext(t, server.URL)

	t.Run("list access requests without filters", func(t *testing.T) {
		rbacAccessRequestListCmd.Flags().Set("tenant-id", "")
		rbacAccessRequestListCmd.Flags().Set("principal-id", "")
		rbacAccessRequestListCmd.Flags().Set("status", "")

		if err := rbacAccessRequestListCmd.RunE(rbacAccessRequestListCmd, []string{}); err != nil {
			t.Fatalf("command failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if lastMethod != http.MethodGet {
			t.Fatalf("expected GET, got %s", lastMethod)
		}
		if lastPath != "/api/v1/rbac/access-requests" {
			t.Fatalf("expected path /api/v1/rbac/access-requests, got %s", lastPath)
		}
	})

	t.Run("list access requests with filters", func(t *testing.T) {
		rbacAccessRequestListCmd.Flags().Set("tenant-id", "tenant-1")
		rbacAccessRequestListCmd.Flags().Set("principal-id", "user-1")
		rbacAccessRequestListCmd.Flags().Set("status", "PENDING")

		if err := rbacAccessRequestListCmd.RunE(rbacAccessRequestListCmd, []string{}); err != nil {
			t.Fatalf("command failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if lastMethod != http.MethodGet {
			t.Fatalf("expected GET, got %s", lastMethod)
		}
		expectedPath := "/api/v1/rbac/access-requests?principalId=user-1&status=PENDING&tenantId=tenant-1"
		if lastPath != expectedPath {
			t.Fatalf("expected path %s, got %s", expectedPath, lastPath)
		}
	})

	t.Run("create access request", func(t *testing.T) {
		rbacAccessRequestCreateCmd.Flags().Set("tenant-id", "tenant-1")
		rbacAccessRequestCreateCmd.Flags().Set("principal-id", "user-2")
		rbacAccessRequestCreateCmd.Flags().Set("resource-type", "ROLE")
		rbacAccessRequestCreateCmd.Flags().Set("resource-id", "role-reader")
		rbacAccessRequestCreateCmd.Flags().Set("action", "READ")
		rbacAccessRequestCreateCmd.Flags().Set("duration", "300")
		rbacAccessRequestCreateCmd.Flags().Set("justification", "temporary read access")

		if err := rbacAccessRequestCreateCmd.RunE(rbacAccessRequestCreateCmd, []string{}); err != nil {
			t.Fatalf("command failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if lastMethod != http.MethodPost {
			t.Fatalf("expected POST, got %s", lastMethod)
		}
		if lastPath != "/api/v1/rbac/access-requests" {
			t.Fatalf("expected create path, got %s", lastPath)
		}
		if lastBody == nil {
			t.Fatal("expected JSON body for create request")
		}
		if got, ok := lastBody["tenantId"].(string); !ok || got != "tenant-1" {
			t.Fatalf("expected tenantId=tenant-1, got %#v", lastBody["tenantId"])
		}
		if got, ok := lastBody["principalId"].(string); !ok || got != "user-2" {
			t.Fatalf("expected principalId=user-2, got %#v", lastBody["principalId"])
		}
		if got, ok := lastBody["resourceType"].(string); !ok || got != "ROLE" {
			t.Fatalf("expected resourceType=ROLE, got %#v", lastBody["resourceType"])
		}
		if got, ok := lastBody["action"].(string); !ok || got != "READ" {
			t.Fatalf("expected action=READ, got %#v", lastBody["action"])
		}
		if got, ok := lastBody["resourceId"].(string); !ok || got != "role-reader" {
			t.Fatalf("expected resourceId payload, got %#v", lastBody["resourceId"])
		}
		if got, ok := lastBody["duration"].(float64); !ok || int(got) != 300 {
			t.Fatalf("expected duration=300, got %#v", lastBody["duration"])
		}
		if got, ok := lastBody["justification"].(string); !ok || got != "temporary read access" {
			t.Fatalf("expected justification payload, got %#v", lastBody["justification"])
		}
	})

	t.Run("create requires mandatory flags", func(t *testing.T) {
		mu.Lock()
		before := requestCount
		mu.Unlock()

		rbacAccessRequestCreateCmd.Flags().Set("tenant-id", "")
		rbacAccessRequestCreateCmd.Flags().Set("principal-id", "")
		rbacAccessRequestCreateCmd.Flags().Set("resource-type", "")
		rbacAccessRequestCreateCmd.Flags().Set("action", "")

		err := rbacAccessRequestCreateCmd.RunE(rbacAccessRequestCreateCmd, []string{})
		if err == nil {
			t.Fatal("expected validation error when mandatory flags are missing")
		}

		mu.Lock()
		defer mu.Unlock()
		if requestCount != before {
			t.Fatalf("expected no HTTP request, got before=%d after=%d", before, requestCount)
		}
	})

	t.Run("approve access request", func(t *testing.T) {
		rbacAccessRequestApproveCmd.Flags().Set("approved-by", "admin-approver")

		if err := rbacAccessRequestApproveCmd.RunE(rbacAccessRequestApproveCmd, []string{"request-approve-1"}); err != nil {
			t.Fatalf("command failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if lastMethod != http.MethodPost {
			t.Fatalf("expected POST, got %s", lastMethod)
		}
		if lastPath != "/api/v1/rbac/access-requests/request-approve-1/approve" {
			t.Fatalf("expected approve path, got %s", lastPath)
		}
		if lastBody == nil {
			t.Fatal("expected JSON body for approve request")
		}
		if got, ok := lastBody["approvedBy"].(string); !ok || got != "admin-approver" {
			t.Fatalf("expected approvedBy payload, got %#v", lastBody["approvedBy"])
		}
	})

	t.Run("approve requires approved-by", func(t *testing.T) {
		mu.Lock()
		before := requestCount
		mu.Unlock()

		rbacAccessRequestApproveCmd.Flags().Set("approved-by", "")

		err := rbacAccessRequestApproveCmd.RunE(rbacAccessRequestApproveCmd, []string{"request-approve-2"})
		if err == nil {
			t.Fatal("expected validation error when approved-by is missing")
		}

		mu.Lock()
		defer mu.Unlock()
		if requestCount != before {
			t.Fatalf("expected no HTTP request, got before=%d after=%d", before, requestCount)
		}
	})

	t.Run("reject access request", func(t *testing.T) {
		rbacAccessRequestRejectCmd.Flags().Set("rejected-by", "approver-1")
		rbacAccessRequestRejectCmd.Flags().Set("reason", "requires more review")

		if err := rbacAccessRequestRejectCmd.RunE(rbacAccessRequestRejectCmd, []string{"request-1"}); err != nil {
			t.Fatalf("command failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if lastMethod != http.MethodPost {
			t.Fatalf("expected POST, got %s", lastMethod)
		}
		if lastPath != "/api/v1/rbac/access-requests/request-1/reject" {
			t.Fatalf("expected reject path, got %s", lastPath)
		}
		if lastBody == nil {
			t.Fatal("expected JSON body for reject request")
		}
		if got, ok := lastBody["rejectedBy"].(string); !ok || got != "approver-1" {
			t.Fatalf("expected rejectedBy=approver-1, got %#v", lastBody["rejectedBy"])
		}
		if got, ok := lastBody["reason"].(string); !ok || got != "requires more review" {
			t.Fatalf("expected reason payload, got %#v", lastBody["reason"])
		}
	})

	t.Run("reject requires rejected-by", func(t *testing.T) {
		mu.Lock()
		before := requestCount
		mu.Unlock()

		rbacAccessRequestRejectCmd.Flags().Set("rejected-by", "")
		rbacAccessRequestRejectCmd.Flags().Set("reason", "")

		err := rbacAccessRequestRejectCmd.RunE(rbacAccessRequestRejectCmd, []string{"request-2"})
		if err == nil {
			t.Fatal("expected validation error when rejected-by is missing")
		}

		mu.Lock()
		defer mu.Unlock()
		if requestCount != before {
			t.Fatalf("expected no HTTP request, got before=%d after=%d", before, requestCount)
		}
	})
}

func TestJobScheduleCLICommands(t *testing.T) {
	var (
		mu           sync.Mutex
		lastPath     string
		lastMethod   string
		lastBody     map[string]interface{}
		requestCount int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		requestCount++
		lastPath = r.URL.RequestURI()
		lastMethod = r.Method
		lastBody = nil

		if r.Body != nil {
			defer r.Body.Close()
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				lastBody = body
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "path": r.URL.RequestURI()})
	}))
	defer server.Close()

	setupPlatformCommandTestContext(t, server.URL)

	t.Run("list schedules", func(t *testing.T) {
		if err := JobScheduleListCmd.RunE(JobScheduleListCmd, []string{}); err != nil {
			t.Fatalf("command failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if lastMethod != http.MethodGet {
			t.Fatalf("expected GET, got %s", lastMethod)
		}
		if lastPath != "/api/v1/jobs/schedules" {
			t.Fatalf("expected list schedules path, got %s", lastPath)
		}
	})

	t.Run("set schedule", func(t *testing.T) {
		JobScheduleSetCmd.Flags().Set("schedule", "30m")

		if err := JobScheduleSetCmd.RunE(JobScheduleSetCmd, []string{"job-123"}); err != nil {
			t.Fatalf("command failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if lastMethod != http.MethodPost {
			t.Fatalf("expected POST, got %s", lastMethod)
		}
		if lastPath != "/api/v1/jobs/job-123/schedule" {
			t.Fatalf("expected set schedule path, got %s", lastPath)
		}
		if lastBody == nil {
			t.Fatal("expected JSON payload")
		}
		if got, ok := lastBody["schedule"].(string); !ok || got != "30m" {
			t.Fatalf("expected schedule=30m, got %#v", lastBody["schedule"])
		}
	})

	t.Run("set schedule requires schedule flag", func(t *testing.T) {
		mu.Lock()
		before := requestCount
		mu.Unlock()

		JobScheduleSetCmd.Flags().Set("schedule", "")
		err := JobScheduleSetCmd.RunE(JobScheduleSetCmd, []string{"job-456"})
		if err == nil {
			t.Fatal("expected validation error when --schedule is missing")
		}

		mu.Lock()
		defer mu.Unlock()
		if requestCount != before {
			t.Fatalf("expected no HTTP request, got before=%d after=%d", before, requestCount)
		}
	})

	t.Run("remove schedule", func(t *testing.T) {
		if err := JobScheduleRemoveCmd.RunE(JobScheduleRemoveCmd, []string{"job-789"}); err != nil {
			t.Fatalf("command failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if lastMethod != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", lastMethod)
		}
		if lastPath != "/api/v1/jobs/job-789/schedule" {
			t.Fatalf("expected remove schedule path, got %s", lastPath)
		}
	})
}

func TestTraceIngestionCLICommands(t *testing.T) {
	var (
		mu           sync.Mutex
		lastPath     string
		lastMethod   string
		lastBody     map[string]interface{}
		requestCount int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		requestCount++
		lastPath = r.URL.RequestURI()
		lastMethod = r.Method
		lastBody = nil

		if r.Body != nil {
			defer r.Body.Close()
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				lastBody = body
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "path": r.URL.RequestURI()})
	}))
	defer server.Close()

	setupPlatformCommandTestContext(t, server.URL)

	t.Run("trace ingest", func(t *testing.T) {
		traceIngestCmd.Flags().Set("tenant-id", "tenant-1")
		traceIngestCmd.Flags().Set("service", "api-gateway")
		traceIngestCmd.Flags().Set("operation", "GET /orders")
		traceIngestCmd.Flags().Set("trace-id", "trace-explicit-1")

		if err := traceIngestCmd.RunE(traceIngestCmd, []string{}); err != nil {
			t.Fatalf("command failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if lastMethod != http.MethodPost {
			t.Fatalf("expected POST, got %s", lastMethod)
		}
		if lastPath != "/api/v1/tracing/traces" {
			t.Fatalf("expected trace ingest path, got %s", lastPath)
		}
		if lastBody == nil {
			t.Fatal("expected JSON payload")
		}
		if got, ok := lastBody["tenantId"].(string); !ok || got != "tenant-1" {
			t.Fatalf("expected tenantId=tenant-1, got %#v", lastBody["tenantId"])
		}
		if got, ok := lastBody["id"].(string); !ok || got != "trace-explicit-1" {
			t.Fatalf("expected id=trace-explicit-1, got %#v", lastBody["id"])
		}
		if spans, ok := lastBody["spans"].([]interface{}); !ok || len(spans) == 0 {
			t.Fatalf("expected non-empty spans payload, got %#v", lastBody["spans"])
		}
	})

	t.Run("trace ingest requires mandatory flags", func(t *testing.T) {
		mu.Lock()
		before := requestCount
		mu.Unlock()

		traceIngestCmd.Flags().Set("tenant-id", "")
		traceIngestCmd.Flags().Set("service", "")
		traceIngestCmd.Flags().Set("operation", "")
		err := traceIngestCmd.RunE(traceIngestCmd, []string{})
		if err == nil {
			t.Fatal("expected validation error when required trace ingest flags are missing")
		}

		mu.Lock()
		defer mu.Unlock()
		if requestCount != before {
			t.Fatalf("expected no HTTP request, got before=%d after=%d", before, requestCount)
		}
	})

	t.Run("trace ingestion audit list with filters", func(t *testing.T) {
		traceIngestionAuditListCmd.Flags().Set("tenant-id", "tenant-1")
		traceIngestionAuditListCmd.Flags().Set("username", "alice")
		traceIngestionAuditListCmd.Flags().Set("resource-type", "trace")
		traceIngestionAuditListCmd.Flags().Set("result", "SUCCESS")
		traceIngestionAuditListCmd.Flags().Set("limit", "25")

		if err := traceIngestionAuditListCmd.RunE(traceIngestionAuditListCmd, []string{}); err != nil {
			t.Fatalf("command failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if lastMethod != http.MethodGet {
			t.Fatalf("expected GET, got %s", lastMethod)
		}
		expectedPath := "/api/v1/tracing/ingestion/audit?limit=25&resourceType=trace&result=SUCCESS&tenantId=tenant-1&username=alice"
		if lastPath != expectedPath {
			t.Fatalf("expected path %s, got %s", expectedPath, lastPath)
		}
	})
}

func TestEventBusAckReplayCLICommands(t *testing.T) {
	var (
		mu           sync.Mutex
		lastPath     string
		lastMethod   string
		lastBody     map[string]interface{}
		requestCount int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		requestCount++
		lastPath = r.URL.RequestURI()
		lastMethod = r.Method
		lastBody = nil

		if r.Body != nil {
			defer r.Body.Close()
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				lastBody = body
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "path": r.URL.RequestURI()})
	}))
	defer server.Close()

	setupPlatformCommandTestContext(t, server.URL)

	t.Run("event ack", func(t *testing.T) {
		eventBusAckCmd.Flags().Set("subscription-id", "sub-1")
		eventBusAckCmd.Flags().Set("acknowledged-by", "worker-1")
		eventBusAckCmd.Flags().Set("message", "processed")

		if err := eventBusAckCmd.RunE(eventBusAckCmd, []string{"event-1"}); err != nil {
			t.Fatalf("command failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if lastMethod != http.MethodPost {
			t.Fatalf("expected POST, got %s", lastMethod)
		}
		if lastPath != "/api/v1/eventbus/events/event-1/ack" {
			t.Fatalf("expected ack path, got %s", lastPath)
		}
		if lastBody == nil {
			t.Fatal("expected JSON payload")
		}
		if got, ok := lastBody["acknowledgedBy"].(string); !ok || got != "worker-1" {
			t.Fatalf("expected acknowledgedBy=worker-1, got %#v", lastBody["acknowledgedBy"])
		}
		if got, ok := lastBody["subscriptionId"].(string); !ok || got != "sub-1" {
			t.Fatalf("expected subscriptionId=sub-1, got %#v", lastBody["subscriptionId"])
		}
	})

	t.Run("event ack requires acknowledged-by", func(t *testing.T) {
		mu.Lock()
		before := requestCount
		mu.Unlock()

		eventBusAckCmd.Flags().Set("acknowledged-by", "")
		err := eventBusAckCmd.RunE(eventBusAckCmd, []string{"event-2"})
		if err == nil {
			t.Fatal("expected validation error when --acknowledged-by is missing")
		}

		mu.Lock()
		defer mu.Unlock()
		if requestCount != before {
			t.Fatalf("expected no HTTP request, got before=%d after=%d", before, requestCount)
		}
	})

	t.Run("dlq replay", func(t *testing.T) {
		eventBusDLQReplayCmd.Flags().Set("replay-to-topic", "orders.replayed")
		eventBusDLQReplayCmd.Flags().Set("replayed-by", "operator-1")

		if err := eventBusDLQReplayCmd.RunE(eventBusDLQReplayCmd, []string{"dlq-1"}); err != nil {
			t.Fatalf("command failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if lastMethod != http.MethodPost {
			t.Fatalf("expected POST, got %s", lastMethod)
		}
		if lastPath != "/api/v1/eventbus/dlq/dlq-1/replay" {
			t.Fatalf("expected replay path, got %s", lastPath)
		}
		if lastBody == nil {
			t.Fatal("expected JSON payload")
		}
		if got, ok := lastBody["replayToTopic"].(string); !ok || got != "orders.replayed" {
			t.Fatalf("expected replayToTopic payload, got %#v", lastBody["replayToTopic"])
		}
		if got, ok := lastBody["replayedBy"].(string); !ok || got != "operator-1" {
			t.Fatalf("expected replayedBy payload, got %#v", lastBody["replayedBy"])
		}
	})
}
