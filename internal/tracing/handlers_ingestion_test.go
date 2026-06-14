package tracing

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/auth"
	"github.com/gin-gonic/gin"
)

type tracingTestManager struct {
	base *InMemoryTracingManager
}

func (m *tracingTestManager) IngestTrace(trace *Trace) (*Trace, error) {
	return m.base.IngestTrace(trace)
}

func (m *tracingTestManager) IngestSpan(span *Span) (*Span, error) {
	return m.base.IngestSpan(span)
}

func (m *tracingTestManager) RecordIngestionAudit(entry *TraceIngestionAuditLog) error {
	return m.base.RecordIngestionAudit(entry)
}

func (m *tracingTestManager) ListIngestionAudits(filter *TraceIngestionAuditFilter) ([]*TraceIngestionAuditLog, error) {
	return m.base.ListIngestionAudits(filter)
}

func (m *tracingTestManager) GetTrace(traceID string) (*Trace, error) {
	return m.base.GetTrace(traceID)
}

func (m *tracingTestManager) SearchTraces(req *TraceSearchRequest) ([]*TraceSearchResult, error) {
	items, err := m.base.SearchTraces(req)
	if err != nil {
		return nil, err
	}

	results := make([]*TraceSearchResult, 0, len(items))
	for _, item := range items {
		service := ""
		if len(item.Services) > 0 {
			service = item.Services[0]
		}
		operation := item.Root.OperationName
		if operation == "" && len(item.Spans) > 0 {
			operation = item.Spans[0].OperationName
		}
		results = append(results, &TraceSearchResult{
			TraceID:    item.ID,
			Service:    service,
			Operation:  operation,
			StartTime:  item.StartTime,
			Duration:   item.Duration,
			SpanCount:  item.TotalSpans,
			ErrorCount: item.ErrorSpans,
			Status:     item.Status,
		})
	}

	return results, nil
}

func (m *tracingTestManager) GetSpan(spanID string) (*Span, error) {
	return m.base.GetSpan(spanID)
}

func (m *tracingTestManager) GetServiceMap(tenantID string) (*ServiceMap, error) {
	depsMap, err := m.base.GetServiceMap()
	if err != nil {
		return nil, err
	}

	serviceSet := make(map[string]bool)
	dependencies := make([]DependencyMetrics, 0)
	for source, deps := range depsMap {
		serviceSet[source] = true
		for _, dep := range deps {
			serviceSet[dep.Destination] = true
			dependencies = append(dependencies, *dep)
		}
	}

	services := make([]ServiceInfo, 0, len(serviceSet))
	for name := range serviceSet {
		services = append(services, ServiceInfo{Name: name, Status: "healthy", LastSeen: time.Now().UTC()})
	}

	return &ServiceMap{
		TenantID:     tenantID,
		Timestamp:    time.Now().UTC(),
		Services:     services,
		Dependencies: dependencies,
	}, nil
}

func (m *tracingTestManager) GetServiceMetrics(service string) (*TraceMetrics, error) {
	return m.base.GetServiceMetrics(service)
}

func (m *tracingTestManager) GetOperationMetrics(service, operation string) (*SpanMetrics, error) {
	return m.base.GetOperationMetrics(service, operation)
}

func (m *tracingTestManager) ListServices(tenantID string) ([]*ServiceInfo, error) {
	svcMap, err := m.GetServiceMap(tenantID)
	if err != nil {
		return nil, err
	}

	items := make([]*ServiceInfo, 0, len(svcMap.Services))
	for i := range svcMap.Services {
		svc := svcMap.Services[i]
		items = append(items, &svc)
	}
	return items, nil
}

func (m *tracingTestManager) GetErrorAnalysis(tenantID, service string) (*ErrorAnalysis, error) {
	items, err := m.base.AnalyzeErrors(service)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return &ErrorAnalysis{TenantID: tenantID, ErrorType: "none", Count: 0}, nil
	}
	analysis := items[0]
	analysis.TenantID = tenantID
	return analysis, nil
}

func setupTracingIngestionTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)

	manager := &tracingTestManager{base: NewInMemoryTracingManager()}
	handler := NewTracingHandler(manager)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("username", "trace-ingestor")
		c.Set("user", &auth.Claims{Sub: "user-trace-1", PreferredUsername: "trace-ingestor"})
		c.Next()
	})

	group := router.Group("/api/v1/tracing")
	{
		group.POST("/traces", handler.IngestTrace)
		group.POST("/spans", handler.IngestSpan)
		group.GET("/traces/:traceId", handler.GetTrace)
		group.GET("/ingestion/audit", handler.ListIngestionAudits)
	}

	return router
}

func performTracingRequest(t *testing.T, router *gin.Engine, method, path string, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()

	var body *bytes.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}
		body = bytes.NewReader(encoded)
	} else {
		body = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req-tracing-test")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func decodeTracingBody(t *testing.T, rr *httptest.ResponseRecorder, target interface{}) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), target); err != nil {
		t.Fatalf("failed to decode body: %v; body=%s", err, rr.Body.String())
	}
}

func TestTracingIngestionAndAuditFlow(t *testing.T) {
	router := setupTracingIngestionTestRouter()

	traceReq := map[string]interface{}{
		"tenantId": "tenant-1",
		"services": []string{"api-gateway"},
		"spans": []map[string]interface{}{
			{
				"id":            "span-root-1",
				"operationName": "GET /orders",
				"service":       "api-gateway",
				"startTime":     time.Now().UTC().Add(-2 * time.Second),
				"endTime":       time.Now().UTC().Add(-1 * time.Second),
				"status":        "OK",
			},
		},
	}

	traceResp := performTracingRequest(t, router, http.MethodPost, "/api/v1/tracing/traces", traceReq)
	if traceResp.Code != http.StatusCreated {
		t.Fatalf("expected trace ingest 201, got %d: %s", traceResp.Code, traceResp.Body.String())
	}

	var traceBody Trace
	decodeTracingBody(t, traceResp, &traceBody)
	if traceBody.ID == "" {
		t.Fatal("expected generated trace id")
	}

	spanReq := map[string]interface{}{
		"traceId":       traceBody.ID,
		"tenantId":      "tenant-1",
		"operationName": "SELECT orders",
		"service":       "orders-db",
		"startTime":     time.Now().UTC().Add(-1500 * time.Millisecond),
		"endTime":       time.Now().UTC().Add(-1200 * time.Millisecond),
		"status":        "OK",
	}

	spanResp := performTracingRequest(t, router, http.MethodPost, "/api/v1/tracing/spans", spanReq)
	if spanResp.Code != http.StatusCreated {
		t.Fatalf("expected span ingest 201, got %d: %s", spanResp.Code, spanResp.Body.String())
	}

	getResp := performTracingRequest(t, router, http.MethodGet, "/api/v1/tracing/traces/"+traceBody.ID, nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected get trace 200, got %d: %s", getResp.Code, getResp.Body.String())
	}

	var fetched Trace
	decodeTracingBody(t, getResp, &fetched)
	if fetched.TotalSpans < 2 {
		t.Fatalf("expected trace to include at least 2 spans after ingestion, got %d", fetched.TotalSpans)
	}

	auditResp := performTracingRequest(t, router, http.MethodGet, "/api/v1/tracing/ingestion/audit?tenantId=tenant-1", nil)
	if auditResp.Code != http.StatusOK {
		t.Fatalf("expected audit 200, got %d: %s", auditResp.Code, auditResp.Body.String())
	}

	var auditBody struct {
		Logs  []TraceIngestionAuditLog `json:"logs"`
		Count int                      `json:"count"`
	}
	decodeTracingBody(t, auditResp, &auditBody)
	if auditBody.Count < 2 || len(auditBody.Logs) < 2 {
		t.Fatalf("expected at least 2 audit records, got count=%d body=%s", auditBody.Count, auditResp.Body.String())
	}
	if auditBody.Logs[0].Username == "" {
		t.Fatal("expected audit username to be populated from request context")
	}
	if auditBody.Logs[0].RequestID != "req-tracing-test" {
		t.Fatalf("expected request id to propagate, got %q", auditBody.Logs[0].RequestID)
	}
}

func TestTracingIngestionFailureIsAudited(t *testing.T) {
	router := setupTracingIngestionTestRouter()

	badSpanResp := performTracingRequest(t, router, http.MethodPost, "/api/v1/tracing/spans", map[string]interface{}{
		"tenantId":      "tenant-1",
		"service":       "orders-db",
		"operationName": "missing trace id",
	})
	if badSpanResp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for missing traceId, got %d: %s", badSpanResp.Code, badSpanResp.Body.String())
	}

	auditResp := performTracingRequest(t, router, http.MethodGet, "/api/v1/tracing/ingestion/audit?result=FAILURE&resourceType=span", nil)
	if auditResp.Code != http.StatusOK {
		t.Fatalf("expected audit query 200, got %d: %s", auditResp.Code, auditResp.Body.String())
	}

	var auditBody struct {
		Logs  []TraceIngestionAuditLog `json:"logs"`
		Count int                      `json:"count"`
	}
	decodeTracingBody(t, auditResp, &auditBody)
	if auditBody.Count == 0 || len(auditBody.Logs) == 0 {
		t.Fatalf("expected at least one failure audit entry, got body=%s", auditResp.Body.String())
	}
	if auditBody.Logs[0].Result != "FAILURE" {
		t.Fatalf("expected failure result, got %s", auditBody.Logs[0].Result)
	}
	if auditBody.Logs[0].ResourceType != "span" {
		t.Fatalf("expected resourceType=span, got %s", auditBody.Logs[0].ResourceType)
	}
}
