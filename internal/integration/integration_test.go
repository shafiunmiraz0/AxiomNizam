package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apibanks"
	"axiomnizam.bitbd.net/axiomnizam/internal/mesh"
)

const (
	testOwnerUser  = "test-user"
	testOwnerInteg = "integration-test"
)

// TestDataMeshIntegration tests data mesh integration
func TestDataMeshIntegration(t *testing.T) {
	ctx := context.Background()

	// Create domain
	domain := &mesh.DataDomain{
		Name:  "TestDomain",
		Owner: testOwnerUser,
	}
	mesh.GlobalDataMesh.CreateDomain(ctx, domain)

	// Create product
	product := &mesh.DataProduct{
		Name:  "TestProduct",
		Owner: testOwnerUser,
		Schema: map[string]interface{}{
			"field1": "string",
			"field2": "int",
		},
	}
	mesh.GlobalDataMesh.CreateDataProduct(ctx, "TestDomain", product)

	// Subscribe
	subscription, err := mesh.GlobalDataMesh.Subscribe(ctx, "TestDomain", "TestProduct", "subscriber", "default")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	if subscription == nil {
		t.Fatal("Expected subscription to be created")
	}

	t.Logf("✅ Data mesh integration test passed")
}

// TestAPIBankIntegration tests API bank integration
func TestAPIBankIntegration(t *testing.T) {
	ctx := context.Background()

	// Create bank
	bank := &apibanks.APIBank{
		Name:  "TestBank",
		Owner: testOwnerUser,
	}
	if err := apibanks.GlobalAPIBankManager.CreateBank(ctx, bank); err != nil {
		t.Fatalf("Failed to create bank: %v", err)
	}

	// Add API
	api := &apibanks.APIReference{
		Name:        "TestAPI",
		Endpoint:    "https://api.test.com/v1/test",
		Kind:        "API",
		DataClasses: []string{"test"},
	}
	if err := apibanks.GlobalAPIBankManager.AddAPIToBank(ctx, "TestBank", *api); err != nil {
		t.Fatalf("Failed to add API: %v", err)
	}

	// Get bank
	retrieved := apibanks.GlobalAPIBankManager.GetBank("TestBank")
	if retrieved == nil {
		t.Fatal("Expected bank to be retrieved")
	}

	if len(retrieved.APIs) != 1 {
		t.Fatalf("Expected 1 API, got %d", len(retrieved.APIs))
	}

	t.Logf("✅ API bank integration test passed")
}

// TestComplianceIntegration tests compliance integration
func TestComplianceIntegration(t *testing.T) {
	ctx := context.Background()
	auditor := NewComplianceAuditor(10000)

	// Record operations
	op := Operation{
		Operation:    "TestOperation",
		User:         testOwnerUser,
		Resource:     "test-resource",
		ResourceType: "TestType",
		Action:       "test",
		Status:       "success",
	}

	if err := auditor.RecordOperation(ctx, op); err != nil {
		t.Fatalf("Failed to record operation: %v", err)
	}

	// Generate report
	report := auditor.GenerateReport(AuditFilter{})

	if report.TotalOperations == 0 {
		t.Fatal("Expected operations in report")
	}

	t.Logf("✅ Compliance integration test passed: %d operations recorded", report.TotalOperations)
}

// TestHealthMonitoring tests health monitoring
func TestHealthMonitoring(t *testing.T) {
	ctx := context.Background()

	health := GlobalHealthMonitor.CheckHealth(ctx)

	if health == nil {
		t.Fatal("Expected health check result")
	}

	if health.Status == "" {
		t.Fatal("Expected health status")
	}

	if len(health.Components) == 0 {
		t.Fatal("Expected health components")
	}

	t.Logf("✅ Health monitoring test passed: system status %s", health.Status)
}

// TestCatalogIntegration tests catalog integration
func TestCatalogIntegration(t *testing.T) {
	ctx := context.Background()

	// Create test data
	domain := &mesh.DataDomain{Name: "CatalogTest", Owner: "test"}
	mesh.GlobalDataMesh.CreateDomain(ctx, domain)

	product := &mesh.DataProduct{
		Name:  "CatalogTestProduct",
		Owner: "test",
		Tags:  []string{"catalog-test"},
	}
	mesh.GlobalDataMesh.CreateDataProduct(ctx, "CatalogTest", product)

	// Get complete catalog
	catalog := NewCatalogIntegration().GetCompleteDataCatalog()

	if catalog == nil {
		t.Fatal("Expected catalog")
	}

	t.Logf("✅ Catalog integration test passed")
}

// TestDataQualityMonitoring tests quality monitoring
func TestDataQualityMonitoring(t *testing.T) {
	ctx := context.Background()

	// Create test product
	domain := &mesh.DataDomain{Name: "QualityTest", Owner: "test"}
	mesh.GlobalDataMesh.CreateDomain(ctx, domain)

	product := &mesh.DataProduct{
		Name:   "QualityTestProduct",
		Owner:  "test",
		Schema: map[string]interface{}{"field": "string"},
		SLA:    mesh.SLA{Availability: "99%"},
	}
	mesh.GlobalDataMesh.CreateDataProduct(ctx, "QualityTest", product)

	// Check quality
	quality := NewDataQualityMonitor(mesh.GlobalDataMesh).CheckProductQuality("QualityTest", "QualityTestProduct")

	if quality == nil {
		t.Fatal("Expected quality report")
	}

	score := quality["qualityScore"].(int)
	if score <= 0 || score > 100 {
		t.Fatalf("Expected quality score between 0-100, got %d", score)
	}

	t.Logf("✅ Quality monitoring test passed: score %d%%", score)
}

// TestDataLineageAnalysis tests lineage analysis
func TestDataLineageAnalysis(t *testing.T) {
	ctx := context.Background()

	// Create test setup
	domain := &mesh.DataDomain{Name: "LineageTest", Owner: "test"}
	mesh.GlobalDataMesh.CreateDomain(ctx, domain)

	product := &mesh.DataProduct{
		Name:  "LineageTestProduct",
		Owner: "test",
	}
	mesh.GlobalDataMesh.CreateDataProduct(ctx, "LineageTest", product)

	// Analyze lineage
	analysis := NewDataLineageAnalyzer().AnalyzeDataFlow("LineageTest", "LineageTestProduct")

	if analysis == nil {
		t.Fatal("Expected lineage analysis")
	}

	if analysis["dataProduct"] == "" {
		t.Fatal("Expected data product in analysis")
	}

	t.Logf("✅ Lineage analysis test passed")
}

// TestAlertGeneration tests alert generation
func TestAlertGeneration(t *testing.T) {
	ctx := context.Background()

	alerts := GlobalAlertManager.GenerateAlerts(ctx)

	// Alerts may be empty or contain health warnings
	activeAlerts := GlobalAlertManager.GetActiveAlerts()

	t.Logf("✅ Alert generation test passed: %d generated, %d active alerts", len(alerts), len(activeAlerts))
}

// TestPlatformMetrics tests metrics collection
func TestPlatformMetrics(t *testing.T) {
	ctx := context.Background()

	metrics := GlobalPlatformMetricsCollector.CollectMetrics(ctx)

	if metrics == nil {
		t.Fatal("Expected metrics")
	}

	if metrics.Timestamp.IsZero() {
		t.Fatal("Expected metrics timestamp")
	}

	if metrics.DataMeshMetrics == nil {
		t.Fatal("Expected data mesh metrics")
	}

	if metrics.APIBankMetrics == nil {
		t.Fatal("Expected API bank metrics")
	}

	t.Logf("✅ Platform metrics test passed")
}

// TestFullIntegration performs comprehensive integration test
func TestFullIntegration(t *testing.T) {
	ctx := context.Background()
	auditor := NewComplianceAuditor(10000)

	// 1. Create domain
	domain := &mesh.DataDomain{Name: "FullIntegrationTest", Owner: testOwnerInteg}
	mesh.GlobalDataMesh.CreateDomain(ctx, domain)
	t.Logf("✅ Created domain")

	// 2. Create product
	product := &mesh.DataProduct{
		Name:  "IntegrationTestProduct",
		Owner: testOwnerInteg,
		Schema: map[string]interface{}{
			"id":   "string",
			"data": "string",
		},
		Tags: []string{"integration"},
	}
	mesh.GlobalDataMesh.CreateDataProduct(ctx, "FullIntegrationTest", product)
	t.Logf("✅ Created product")

	// 3. Create subscription
	sub, err := mesh.GlobalDataMesh.Subscribe(ctx, "FullIntegrationTest", "IntegrationTestProduct", "integration-consumer", "default")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	t.Logf("✅ Created subscription: %s", sub.ID)

	// 4. Create API bank
	bank := &apibanks.APIBank{
		Name:  "IntegrationTestBank",
		Owner: testOwnerInteg,
	}
	if err := apibanks.GlobalAPIBankManager.CreateBank(ctx, bank); err != nil {
		t.Fatalf("Failed to create bank: %v", err)
	}
	t.Logf("✅ Created API bank")

	// 5. Record operations in audit
	for i := 0; i < 3; i++ {
		op := Operation{
			Operation:    fmt.Sprintf("TestOp%d", i),
			User:         testOwnerInteg,
			Resource:     "FullIntegrationTest/IntegrationTestProduct",
			ResourceType: "DataProduct",
			Action:       "read",
			Status:       "success",
		}
		if err := auditor.RecordOperation(ctx, op); err != nil {
			t.Fatalf("Failed to record operation: %v", err)
		}
	}
	t.Logf("✅ Recorded audit operations")

	// 6. Generate reports
	health := GlobalHealthMonitor.CheckHealth(ctx)
	t.Logf("✅ Health check: %s", health.Status)

	complianceReport := auditor.GenerateReport(AuditFilter{})
	t.Logf("✅ Compliance report: %d operations, %.1f%% success",
		complianceReport.TotalOperations,
		float64(complianceReport.SuccessfulOps)*100/float64(complianceReport.TotalOperations))

	qualityReport := NewDataQualityMonitor(mesh.GlobalDataMesh).GetQualityReport("FullIntegrationTest")
	t.Logf("✅ Quality report: avg score %v%%", qualityReport["averageQualityScore"])

	lineageAnalysis := NewDataLineageAnalyzer().AnalyzeDataFlow("FullIntegrationTest", "IntegrationTestProduct")
	t.Logf("✅ Lineage analysis: %v", lineageAnalysis["dataProduct"])

	// 7. Collect metrics
	metrics := GlobalPlatformMetricsCollector.CollectMetrics(ctx)
	t.Logf("✅ Metrics collected: %d domains, %d banks",
		metrics.DataMeshMetrics["domainCount"],
		metrics.APIBankMetrics["bankCount"])

	// 8. Generate alerts
	alerts := GlobalAlertManager.GenerateAlerts(ctx)
	t.Logf("✅ Alerts: %d new alerts", len(alerts))

	t.Logf("\n✨ Full integration test completed successfully!")
}

// BenchmarkComplianceRecording benchmarks compliance recording
func BenchmarkComplianceRecording(b *testing.B) {
	ctx := context.Background()
	op := Operation{
		Operation:    "BenchmarkOp",
		User:         "benchmark",
		Resource:     "bench-resource",
		ResourceType: "BenchType",
		Action:       "benchmark",
		Status:       "success",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewComplianceAuditor(10000).RecordOperation(ctx, op)
	}
	b.ReportAllocs()
}

// BenchmarkHealthCheck benchmarks health checking
func BenchmarkHealthCheck(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GlobalHealthMonitor.CheckHealth(ctx)
	}
	b.ReportAllocs()
}

// BenchmarkMetricsCollection benchmarks metrics collection
func BenchmarkMetricsCollection(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GlobalPlatformMetricsCollector.CollectMetrics(ctx)
	}
	b.ReportAllocs()
}

// TestConcurrentOperations tests concurrent system operations
func TestConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	errChan := make(chan error, 10)

	// Concurrent compliance recording
	for i := 0; i < 5; i++ {
		go func(id int) {
			op := Operation{
				Operation:    fmt.Sprintf("ConcurrentOp%d", id),
				User:         fmt.Sprintf("user%d", id),
				Resource:     fmt.Sprintf("resource%d", id),
				ResourceType: "TestType",
				Action:       "concurrent",
				Status:       "success",
			}
			errChan <- NewComplianceAuditor(10000).RecordOperation(ctx, op)
		}(i)
	}

	// Concurrent health checks
	for i := 0; i < 5; i++ {
		go func() {
			_ = GlobalHealthMonitor.CheckHealth(ctx)
			errChan <- nil
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("Concurrent operation failed: %v", err)
		}
	}

	t.Logf("✅ Concurrent operations test passed")
}

// TestSystemStability tests system stability over time
func TestSystemStability(t *testing.T) {
	ctx := context.Background()
	duration := 1 * time.Second
	end := time.Now().Add(duration)

	operations := 0
	errors := 0

	for time.Now().Before(end) {
		op := Operation{
			Operation:    "StabilityTest",
			User:         "stability-test",
			Resource:     "stability-resource",
			ResourceType: "StabilityType",
			Action:       "test",
			Status:       "success",
		}

		if err := NewComplianceAuditor(10000).RecordOperation(ctx, op); err != nil {
			errors++
		}
		operations++

		_ = GlobalHealthMonitor.CheckHealth(ctx)
		_ = GlobalPlatformMetricsCollector.CollectMetrics(ctx)
	}

	successRate := float64(operations-errors) / float64(operations)
	t.Logf("✅ Stability test: %d operations, %.2f%% success rate", operations, successRate*100)

	if successRate < 0.99 {
		t.Fatalf("Expected >99%% success rate, got %.2f%%", successRate*100)
	}
}
