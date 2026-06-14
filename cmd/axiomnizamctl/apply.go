package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/client"
	"gopkg.in/yaml.v3"
)

// ApplyOptions controls the apply operation
type ApplyOptions struct {
	Filename  string
	DryRun    bool
	Force     bool
	Record    bool
	Namespace string
	Timeout   time.Duration
}

// ApplyResult contains the result of an apply operation
type ApplyResult struct {
	Kind       string
	Name       string
	Action     string // "created", "updated", "unchanged", "reconciled"
	Message    string
	Timestamp  time.Time
	Generation int64
	Status     string // "success", "pending", "failed"
}

// handleApplyCommand handles the apply command flow
// Maps to controller reconciliation: apply → apiserver → controller workqueue → reconcile
//
// CRITICAL ARCHITECTURE:
// - CLI never talks to database directly
// - All operations flow through API Server
// - API Server enqueues work in controller
// - Controller reconciles desired state
func handleApply(opts ApplyOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	// Step 1: Validate and parse resource
	fmt.Printf("📖 Reading resource from %s...\n", opts.Filename)

	data, err := os.ReadFile(opts.Filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var resourceMap map[string]interface{}
	if err := yaml.Unmarshal(data, &resourceMap); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Step 2: Convert to Resource object
	resource, err := mapToResource(resourceMap, opts.Namespace)
	if err != nil {
		return err
	}

	fmt.Printf("📦 Resource: %s/%s.%s (generation: %d)\n",
		resource.Metadata.Namespace, resource.Kind, resource.Metadata.Name, resource.Metadata.Generation)

	// Step 3: Dry-run if requested
	if opts.DryRun || dry {
		fmt.Println("\n🔍 Dry-run mode: showing what would be applied")
		return showApplyPlan(resource)
	}

	// Step 4: Get resource client from config
	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	// Initialize API client with current context
	server := configManager.GetServer()
	token := configManager.GetToken()

	apiHTTPClient := client.NewClient(server)
	apiHTTPClient.SetToken(token)
	resourceClient := client.NewResourceClient(apiHTTPClient)

	// Step 5: Apply resource through API Server
	fmt.Println("\n📡 Sending to API server...")
	fmt.Printf("   Server: %s\n", server)

	result, err := resourceClient.Apply(ctx, resource)
	if err != nil {
		return fmt.Errorf("failed to apply resource: %w", err)
	}

	// Step 6: Display result
	fmt.Printf("\n✅ Applied successfully!\n\n")
	fmt.Printf("   Kind: %s\n", result.Kind)
	fmt.Printf("   Name: %s\n", result.Metadata.Name)
	fmt.Printf("   Namespace: %s\n", result.Metadata.Namespace)
	fmt.Printf("   Generation: %d\n", result.Metadata.Generation)

	// Step 7: Check reconciliation status from controller
	status, err := resourceClient.GetStatus(ctx, result.Kind, result.Metadata.Name)
	if err == nil && status != nil {
		if phase, ok := status["phase"].(string); ok {
			fmt.Printf("\n🔄 Controller Status: %s\n", phase)
		}
		if conditions, ok := status["conditions"].([]interface{}); ok && len(conditions) > 0 {
			if cond, ok := conditions[0].(map[string]interface{}); ok {
				if reason, ok := cond["reason"].(string); ok {
					fmt.Printf("   Condition: %s\n", reason)
				}
			}
		}
	}

	// Step 8: Watch for reconciliation completion (optional)
	if !opts.Force {
		fmt.Println("\n⏳ Waiting for controller reconciliation...")
		if err := watchReconciliationViaClient(ctx, resourceClient, result.Kind, result.Metadata.Name); err != nil {
			fmt.Printf("⚠️  Reconciliation incomplete: %v\n", err)
		}
	}

	if verbose {
		fmt.Printf("\n💾 Resource stored in API Server\n")
		fmt.Printf("   Generation: %d\n", result.Metadata.Generation)
	}

	return nil
}

// mapToResource converts a YAML map to a Resource struct
func mapToResource(resourceMap map[string]interface{}, defaultNS string) (*client.Resource, error) {
	// Extract basic fields
	apiVersion, _ := resourceMap["apiVersion"].(string)
	if apiVersion == "" {
		apiVersion = "axiom-nizam/v1"
	}

	kind, ok := resourceMap["kind"].(string)
	if !ok || kind == "" {
		return nil, fmt.Errorf("missing or invalid 'kind' field")
	}

	metadata, ok := resourceMap["metadata"].(map[string]interface{})
	if !ok {
		metadata = make(map[string]interface{})
		resourceMap["metadata"] = metadata
	}

	name, ok := metadata["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("missing or invalid 'metadata.name'")
	}

	ns := defaultNS
	if metaNS, ok := metadata["namespace"].(string); ok && metaNS != "" {
		ns = metaNS
	}
	if ns == "" {
		ns = "default"
	}

	generation := int64(1)
	if gen, ok := metadata["generation"].(float64); ok {
		generation = int64(gen)
	}

	// Extract spec and status
	spec := make(map[string]interface{})
	if s, ok := resourceMap["spec"].(map[string]interface{}); ok {
		spec = s
	}

	status := make(map[string]interface{})
	if st, ok := resourceMap["status"].(map[string]interface{}); ok {
		status = st
	}

	// Build Resource object
	resource := &client.Resource{
		APIVersion: apiVersion,
		Kind:       kind,
		Metadata: client.ResourceMetadata{
			Name:       name,
			Namespace:  ns,
			Generation: generation,
		},
		Spec:   spec,
		Status: status,
	}

	return resource, nil
}

// showApplyPlan displays what would be applied
func showApplyPlan(resource *client.Resource) error {
	fmt.Printf("\n📋 Apply Plan\n")
	fmt.Println("─────────────────────────────────")
	fmt.Printf("Kind: %s\n", resource.Kind)
	fmt.Printf("Name: %s\n", resource.Metadata.Name)
	fmt.Printf("Namespace: %s\n", resource.Metadata.Namespace)

	if len(resource.Spec) > 0 {
		fmt.Println("\nSpec:")
		for key, val := range resource.Spec {
			fmt.Printf("  %s: %v\n", key, val)
		}
	}

	fmt.Println("\n💡 Run without --dry-run to apply")
	return nil
}

// getAction determines the action taken based on HTTP status
func getAction(statusCode int) string {
	switch {
	case statusCode == 201:
		return "created"
	case statusCode == 200:
		return "updated"
	case statusCode == 204:
		return "unchanged"
	case statusCode >= 200 && statusCode < 300:
		return "reconciled"
	default:
		return "unknown"
	}
}

// watchReconciliationViaClient watches for controller reconciliation using ResourceClient
func watchReconciliationViaClient(ctx context.Context, resourceClient client.ResourceClient, kind, name string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for reconciliation")
		default:
		}

		status, err := resourceClient.GetStatus(ctx, kind, name)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		if status == nil {
			time.Sleep(1 * time.Second)
			continue
		}

		// Check phase: Ready, Pending, Failed
		if phase, ok := status["phase"].(string); ok {
			switch phase {
			case "Ready", "Active":
				fmt.Printf("✅ Reconciliation complete: %s\n", phase)
				return nil
			case "Failed":
				if reason, ok := status["reason"].(string); ok {
					return fmt.Errorf("reconciliation failed: %s", reason)
				}
				return fmt.Errorf("reconciliation failed")
			}
		}

		time.Sleep(2 * time.Second)
	}
}

// ╔════════════════════════════════════════════════════════════════════════════╗
// ║  DECLARATIVE APPLY ENGINE - Phase 2                                       ║
// ║  Kubernetes-style declarative resource management                         ║
// ╚════════════════════════════════════════════════════════════════════════════╝
//
// ARCHITECTURE PRINCIPLE:
// All CLI operations flow through the API Server and Controller pattern.
// Direct database access from CLI is strictly forbidden.
//
// Command Pattern:
// $ axiomnizamctl apply -f resource.yaml
//
// Flow:
// 1. Parse YAML        → Extract kind, name, namespace, spec
// 2. Validate Schema   → Check required fields, type compliance
// 3. API Server Call   → ResourceClient.Apply() with token auth
// 4. Store Desired     → API Server persists to database
// 5. Controller Loop   → Detects change, enqueues reconciliation
// 6. Reconcile         → Actual vs desired state, execute changes
// 7. Update Status     → Record phase, conditions, generation
// 8. CLI Watches       → Poll until Ready or Failed
//
// Example:
//
//   $ axiomnizamctl apply -f examples/api.yaml
//   📖 Reading resource from examples/api.yaml...
//   📦 Resource: API/production.default (generation: 1)
//   📡 Sending to API server...
//      Server: https://api.axiomnizam.io
//   ✅ Applied successfully!
//   🔄 Controller Status: Pending
//   ⏳ Waiting for controller reconciliation...
//   ✅ Reconciliation complete: Ready
//
// This makes AxiomNizam a platform-grade system where:
// - Resources are declarative (define desired state)
// - Reconciliation is automatic (system drives toward desired state)
// - Status is observable (track progress and errors)
// - Multi-environment capable (config with contexts)
