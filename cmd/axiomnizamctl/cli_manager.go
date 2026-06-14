package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ghodss/yaml"
	"axiomnizam.bitbd.net/axiomnizam/internal/controllers"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources/apiresource"
)

// CLIManager manages API resources via CLI
type CLIManager struct {
	store      *apiresource.Store
	controller *controllers.APIResourceController
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewCLIManager creates CLI manager
func NewCLIManager() *CLIManager {
	ctx, cancel := context.WithCancel(context.Background())

	store := apiresource.NewStore()
	controller := controllers.NewAPIResourceController(store, 3)
	controller.Start(ctx)

	return &CLIManager{
		store:      store,
		controller: controller,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Shutdown gracefully stops the manager
func (cm *CLIManager) Shutdown() {
	cm.controller.Stop()
	cm.cancel()
}

// Apply reads YAML and stores the resource
func (cm *CLIManager) Apply(filePath string) error {
	fmt.Printf("📝 Applying: %s\n\n", filePath)

	// Read YAML file
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse YAML
	var data map[string]interface{}
	err = yaml.Unmarshal(content, &data)
	if err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Extract metadata
	metadata := data["metadata"].(map[string]interface{})
	name := metadata["name"].(string)
	namespace := "default"
	if ns, ok := metadata["namespace"].(string); ok {
		namespace = ns
	}

	// Extract spec
	spec := data["spec"].(map[string]interface{})

	// Create resource with spec map
	api := apiresource.New(namespace, name, spec)

	// Store it
	stored, err := cm.store.Create(cm.ctx, api)
	if err != nil {
		return fmt.Errorf("failed to store resource: %w", err)
	}

	fmt.Printf("✅ Resource stored:\n")
	fmt.Printf("   Name:      %s\n", stored.Metadata.Name)
	fmt.Printf("   Namespace: %s\n", stored.Metadata.Namespace)
	fmt.Printf("   Status:    %s\n", stored.Status.Phase)
	fmt.Printf("   Message:   %s\n\n", stored.Status.Message)

	// Enqueue for reconciliation
	cm.controller.Enqueue(namespace, name)
	fmt.Printf("🔄 Enqueued for reconciliation\n\n")

	// Watch status until ready or error
	return cm.watchStatus(namespace, name)
}

// watchStatus watches the resource until it's ready
func (cm *CLIManager) watchStatus(namespace, name string) error {
	fmt.Printf("⏳ Waiting for reconciliation...\n\n")

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for resource to be ready")

		case <-ticker.C:
			api, err := cm.store.Get(cm.ctx, namespace, name)
			if err != nil {
				continue
			}

			// Print status
			fmt.Printf("\r[%s] %s", time.Now().Format("15:04:05"), api.Status.Phase)

			// Check if done
			if api.Status.Ready {
				fmt.Printf("\n\n✨ Resource is READY!\n\n")
				printResourceDetails(api)
				return nil
			}

			if api.Status.Phase == "Failed" {
				fmt.Printf("\n\n❌ Resource FAILED\n")
				fmt.Printf("   Message: %s\n\n", api.Status.Message)
				return fmt.Errorf("resource failed: %s", api.Status.Message)
			}
		}
	}
}

// Get retrieves and displays resources
func (cm *CLIManager) Get(namespace string) error {
	fmt.Printf("📋 Listing APIResources in %s:\n\n", namespace)

	apis, err := cm.store.List(cm.ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to list resources: %w", err)
	}

	if len(apis) == 0 {
		fmt.Printf("No resources found\n\n")
		return nil
	}

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tREADY\tAGE\tMESSAGE")

	for _, api := range apis {
		age := time.Since(api.Metadata.CreatedAt).Round(time.Second)
		ready := "false"
		if api.Status.Ready {
			ready = "true"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%s\n",
			api.Metadata.Name,
			api.Status.Phase,
			ready,
			age,
			api.Status.Message,
		)
	}

	w.Flush()
	fmt.Printf("\n")

	return nil
}

// Describe shows detailed resource info
func (cm *CLIManager) Describe(namespace, name string) error {
	fmt.Printf("📖 Resource Details: %s/%s\n\n", namespace, name)

	api, err := cm.store.Get(cm.ctx, namespace, name)
	if err != nil {
		return fmt.Errorf("resource not found: %w", err)
	}

	printResourceDetails(api)
	return nil
}

// printResourceDetails prints full resource details
func printResourceDetails(api *apiresource.APIResource) {
	fmt.Printf("Metadata:\n")
	fmt.Printf("  Name:       %s\n", api.Metadata.Name)
	fmt.Printf("  Namespace:  %s\n", api.Metadata.Namespace)
	fmt.Printf("  UID:        %s\n", api.Metadata.UID)
	fmt.Printf("  Generation: %d\n", api.Metadata.Generation)
	fmt.Printf("  Created:    %s\n", api.Metadata.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Updated:    %s\n\n", api.Metadata.UpdatedAt.Format("2006-01-02 15:04:05"))

	fmt.Printf("Spec:\n")
	fmt.Printf("  BasePath:    %s\n", api.Spec.BasePath)
	fmt.Printf("  Title:       %s\n", api.Spec.Title)
	fmt.Printf("  Description: %s\n", api.Spec.Description)
	fmt.Printf("  Version:     %s\n", api.Spec.Version)
	fmt.Printf("  Timeout:     %ds\n\n", api.Spec.Timeout)

	fmt.Printf("Status:\n")
	fmt.Printf("  Phase:   %s\n", api.Status.Phase)
	fmt.Printf("  Ready:   %v\n", api.Status.Ready)
	fmt.Printf("  Message: %s\n", api.Status.Message)
	if !api.Status.LastUpdate.IsZero() {
		fmt.Printf("  Last Update: %s\n", api.Status.LastUpdate.Format("2006-01-02 15:04:05"))
	}

	if len(api.Status.Conditions) > 0 {
		fmt.Printf("\n  Conditions:\n")
		for _, cond := range api.Status.Conditions {
			fmt.Printf("    - %s: %s (%s)\n", cond.Type, cond.Status, cond.Message)
		}
	}

	fmt.Printf("\n")
}

// ShowControllerMetrics displays controller metrics
func (cm *CLIManager) ShowControllerMetrics() {
	metrics := cm.controller.GetMetrics()

	fmt.Printf("\n📊 Controller Metrics:\n\n")
	fmt.Printf("  Total Reconciles:      %v\n", metrics["total_reconciles"])
	fmt.Printf("  Successful Reconciles: %v\n", metrics["successful_reconciles"])
	fmt.Printf("  Failed Reconciles:     %v\n", metrics["failed_reconciles"])
	fmt.Printf("  Queue Length:          %v\n", metrics["queue_length"])
	fmt.Printf("  Processing:            %v\n", metrics["processing_length"])
	fmt.Printf("  Resources Ready:       %v\n", metrics["resources_ready"])
	fmt.Printf("  Resources Creating:    %v\n", metrics["resources_creating"])
	fmt.Printf("  Resources Failed:      %v\n\n", metrics["resources_failed"])
}
