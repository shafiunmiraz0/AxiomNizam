package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources/apiresource"
)

const testAPIName = "test-api"

// verifyResourceExists verifies a resource exists in store and returns it.
func verifyResourceExists(t *testing.T, cm *CLIManager, ctx context.Context, namespace, apiName string) *apiresource.APIResource {
	t.Helper()
	time.Sleep(100 * time.Millisecond)
	api, err := cm.store.Get(ctx, namespace, apiName)
	if err != nil {
		t.Fatalf("Store Get failed: %v", err)
	}
	if api == nil {
		t.Fatal("Resource not found in store")
	}
	return api
}

// verifyPendingStatus checks that the resource starts in Pending state.
func verifyPendingStatus(t *testing.T, api *apiresource.APIResource) {
	t.Helper()
	if api.Status.Phase != "Pending" {
		t.Fatalf("Expected Phase=Pending, got %s", api.Status.Phase)
	}
	if api.Status.Ready {
		t.Fatal("Expected Ready=false initially")
	}
}

// waitForReady polls until the resource reaches Ready or times out.
func waitForReady(t *testing.T, cm *CLIManager, ctx context.Context, namespace, apiName string) {
	t.Helper()
	maxWait := time.Now().Add(5 * time.Second)
	for time.Now().Before(maxWait) {
		api, err := cm.store.Get(ctx, namespace, apiName)
		if err != nil {
			t.Fatalf("Store Get failed during wait: %v", err)
		}
		if api.Status.Phase == "Ready" && api.Status.Ready {
			return
		}
		if api.Status.Phase == "Failed" {
			t.Fatalf("Resource entered Failed state: %s", api.Status.Message)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// verifyFinalStatus checks the final resource state after reconciliation.
func verifyFinalStatus(t *testing.T, cm *CLIManager, ctx context.Context, namespace, apiName string) {
	t.Helper()
	api, err := cm.store.Get(ctx, namespace, apiName)
	if err != nil {
		t.Fatalf("Final Get failed: %v", err)
	}
	if api.Status.Phase != "Ready" {
		t.Fatalf("Expected final Phase=Ready, got %s", api.Status.Phase)
	}
	if !api.Status.Ready {
		t.Fatal("Expected final Ready=true")
	}
	if api.Metadata.Generation < 2 {
		t.Fatalf("Expected Generation >= 2 after reconciliation, got %d", api.Metadata.Generation)
	}
}

// verifyListQueryable ensures the resource appears in store List results.
func verifyListQueryable(t *testing.T, cm *CLIManager, ctx context.Context, namespace, apiName string) {
	t.Helper()
	apis, err := cm.store.List(ctx, namespace)
	if err != nil {
		t.Fatalf("Store List failed: %v", err)
	}
	for _, a := range apis {
		if a.Metadata.Name == apiName && a.Metadata.Namespace == namespace {
			return
		}
	}
	t.Fatal("Resource not found in List results")
}

// TestReconciliationLoopComplete validates the end-to-end lifecycle
func TestReconciliationLoopComplete(t *testing.T) {
	tests := []struct {
		name      string
		yamlPath  string
		namespace string
		apiName   string
	}{
		{
			name:      "Complete APIResource Lifecycle",
			yamlPath:  "examples/api.yaml",
			namespace: "default",
			apiName:   "users-api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := NewCLIManager()
			defer cm.Shutdown()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := cm.Apply(tt.yamlPath)
			if err != nil {
				t.Fatalf("Apply failed: %v", err)
			}

			api := verifyResourceExists(t, cm, ctx, tt.namespace, tt.apiName)
			verifyPendingStatus(t, api)
			waitForReady(t, cm, ctx, tt.namespace, tt.apiName)
			verifyFinalStatus(t, cm, ctx, tt.namespace, tt.apiName)
			verifyListQueryable(t, cm, ctx, tt.namespace, tt.apiName)

			t.Log("RECONCILIATION LOOP TEST PASSED")
		})
	}
}

// TestStatusTransitions verifies all state transitions
func TestStatusTransitions(t *testing.T) {
	cm := NewCLIManager()
	defer cm.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a test resource directly
	spec := map[string]interface{}{
		"basePath":    "/api/v1/test",
		"title":       "Test API",
		"description": "Test resource for transitions",
		"version":     "1.0",
		"timeout":     30,
	}

	api := apiresource.New("default", testAPIName, spec)

	// Verify initial state is Pending
	if api.Status.Phase != "Pending" {
		t.Fatalf("Expected initial Phase=Pending, got %s", api.Status.Phase)
	}
	t.Logf("✓ Initial state: Phase=%s, Ready=%v", api.Status.Phase, api.Status.Ready)

	// Store resource
	stored, err := cm.store.Create(ctx, api)
	if err != nil {
		t.Fatalf("Store Create failed: %v", err)
	}

	// Simulate state transitions
	stored.MarkCreating("Initializing API")
	_, err = cm.store.Update(ctx, stored)
	if err != nil {
		t.Fatalf("Update to Creating failed: %v", err)
	}

	retrieved, err := cm.store.Get(ctx, "default", testAPIName)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Status.Phase != "Creating" {
		t.Fatalf("Expected Creating state, got %s", retrieved.Status.Phase)
	}
	t.Logf("✓ Transitioned to: Phase=%s, Ready=%v", retrieved.Status.Phase, retrieved.Status.Ready)

	// Transition to Ready
	retrieved.SetReady(true)
	retrieved.SetPhase("Ready")
	_, err = cm.store.Update(ctx, retrieved)
	if err != nil {
		t.Fatalf("Update to Ready failed: %v", err)
	}

	final, err := cm.store.Get(ctx, "default", testAPIName)
	if err != nil {
		t.Fatalf("Final Get failed: %v", err)
	}

	if final.Status.Phase != "Ready" || !final.Status.Ready {
		t.Fatalf("Expected Ready state, got Phase=%s, Ready=%v", final.Status.Phase, final.Status.Ready)
	}
	t.Logf("✓ Final state: Phase=%s, Ready=%v", final.Status.Phase, final.Status.Ready)

	t.Log("\n✓ All status transitions validated successfully")
}

// BenchmarkReconciliation measures reconciliation performance
func BenchmarkReconciliation(b *testing.B) {
	cm := NewCLIManager()
	defer cm.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := map[string]interface{}{
		"basePath":    "/api/v1/bench",
		"title":       "Benchmark API",
		"description": "Resource for performance testing",
		"version":     "1.0",
		"timeout":     30,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		apiName := fmt.Sprintf("bench-api-%d", i)
		api := apiresource.New("default", apiName, spec)

		_, err := cm.store.Create(ctx, api)
		if err != nil {
			b.Fatalf("Create failed: %v", err)
		}

		cm.controller.Enqueue("default", apiName)

		// Wait for reconciliation
		maxWait := time.Now().Add(10 * time.Second)
		for time.Now().Before(maxWait) {
			retrieved, err := cm.store.Get(ctx, "default", apiName)
			if err != nil {
				b.Fatalf("Get failed: %v", err)
			}

			if retrieved.Status.Phase == "Ready" {
				break
			}

			time.Sleep(100 * time.Millisecond)
		}
	}
}
