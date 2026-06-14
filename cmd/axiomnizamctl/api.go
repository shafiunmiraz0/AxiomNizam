package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/output"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var APICmd = &cobra.Command{
	Use:   "api",
	Short: "Manage APIs",
	Long:  "Create, list, get, update, delete, and apply API resources",
}

var APICreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new API",
	Long:  "Interactively create a new API resource",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAPICreate()
	},
}

var APIListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all APIs",
	Long:  "List all API resources in namespace",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAPIList()
	},
}

var APIGetCmd = &cobra.Command{
	Use:   "get [name]",
	Short: "Get API details",
	Long:  "Get detailed information about a specific API",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAPIGet(args[0])
	},
}

var APIUpdateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Update an API",
	Long:  "Update a specific field in an API resource",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAPIUpdate(args[0])
	},
}

var APIDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete an API",
	Long:  "Delete an API resource (requires confirmation)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAPIDelete(args[0])
	},
}

var APIApplyCmd = &cobra.Command{
	Use:   "apply -f [file]",
	Short: "Apply API from YAML (triggers controller reconciliation)",
	Long: `Create or update an API resource from YAML file.

This command uses Kubernetes-style reconciliation:
1. Parses and validates the YAML file
2. Sends to API server with metadata
3. Controller detects change and queues reconciliation
4. Reconciler applies desired state
5. Status is updated with reconciliation result

Examples:
  axiomnizamctl api apply -f api.yaml
  axiomnizamctl api apply -f api.yaml --dry-run
  axiomnizamctl api apply -f api.yaml --namespace prod`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filename, _ := cmd.Flags().GetString("filename")
		if filename == "" {
			return fmt.Errorf("filename flag (-f) is required")
		}

		opts := ApplyOptions{
			Filename:  filename,
			DryRun:    dry,
			Force:     false,
			Namespace: namespace,
			Timeout:   30 * time.Second,
		}

		if force, _ := cmd.Flags().GetBool("force"); force {
			opts.Force = force
		}

		if t, _ := cmd.Flags().GetDuration("timeout"); t > 0 {
			opts.Timeout = t
		}

		return handleApply(opts)
	},
}

var APIDescribeCmd = &cobra.Command{
	Use:   "describe [name]",
	Short: "Show detailed API information",
	Long:  "Show detailed information about an API including status and events",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAPIDescribe(args[0])
	},
}

var APIDiffCmd = &cobra.Command{
	Use:   "diff -f [file]",
	Short: "Show differences between file and server",
	Long:  "Show what would change if you applied a YAML file",
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("filename")
		if file == "" {
			return NewCommandError(ErrInvalidInput, "filename flag (-f) is required")
		}
		return handleAPIDiff(file)
	},
}

func handleAPICreate() error {
	if err := validateServerConnection(); err != nil {
		return err
	}
	if err := validateNamespace(); err != nil {
		return err
	}

	fmt.Println("📝 Create API Resource")

	name := promptInput("API Name")
	if err := validateResourceName(name); err != nil {
		return err
	}

	db := promptInput("Database")
	if db == "" {
		return NewCommandError(ErrInvalidInput, "Database cannot be empty")
	}

	table := promptInput("Table")
	if table == "" {
		return NewCommandError(ErrInvalidInput, "Table cannot be empty")
	}

	apiResource := map[string]interface{}{
		"apiVersion": "axiom-nizam.io/v1",
		"kind":       "API",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"database": db,
			"table":    table,
			"rateLimit": map[string]interface{}{
				"enabled":             true,
				"requests_per_second": 100,
			},
		},
	}

	response, err := apiClient.PostSimple("/api/v1/apis", apiResource)
	if err != nil {
		return NewCommandError(ErrNetwork, "Failed to create API", err.Error())
	}

	if response.StatusCode >= 400 {
		return NewCommandError(ErrServerError, fmt.Sprintf("Server error (%d)", response.StatusCode), response.Status)
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		printSuccessMessage(fmt.Sprintf("API '%s' created successfully", name))
	}

	return nil
}

func handleAPIList() error {
	if err := validateServerConnection(); err != nil {
		return err
	}
	if err := validateNamespace(); err != nil {
		return err
	}

	fmt.Println("📋 APIs in namespace: " + namespace)

	response, err := apiClient.GetSimple(fmt.Sprintf("/api/v1/namespaces/%s/apis", namespace))
	if err != nil {
		return NewCommandError(ErrNetwork, "Failed to list APIs", err.Error())
	}

	if response.StatusCode == 404 {
		printInfoMessage("No APIs found in this namespace")
		return nil
	}

	if response.StatusCode >= 400 {
		return NewCommandError(ErrServerError, fmt.Sprintf("Server error (%d)", response.StatusCode), response.Status)
	}

	var apis []map[string]interface{}
	if err := response.JSON(&apis); err != nil {
		return NewCommandError(ErrInvalidInput, "Failed to parse response", err.Error())
	}

	if len(apis) == 0 {
		printInfoMessage("No APIs found in this namespace")
		return nil
	}

	formatter := output.NewFormatter(outputFormat, os.Stdout)
	formatter.Print(apis)

	return nil
}

func handleAPIGet(name string) error {
	if err := validateServerConnection(); err != nil {
		return err
	}
	if err := validateNamespace(); err != nil {
		return err
	}
	if err := validateResourceName(name); err != nil {
		return err
	}

	response, err := apiClient.GetSimple(fmt.Sprintf("/api/v1/namespaces/%s/apis/%s", namespace, name))
	if err != nil {
		return NewCommandError(ErrNetwork, "Failed to get API", err.Error())
	}

	if response.StatusCode == 404 {
		return NewCommandError(ErrNotFound, fmt.Sprintf("API '%s' not found in namespace '%s'", name, namespace))
	}

	if response.StatusCode >= 400 {
		return NewCommandError(ErrServerError, fmt.Sprintf("Server error (%d)", response.StatusCode), response.Status)
	}

	var api map[string]interface{}
	if err := response.JSON(&api); err != nil {
		return NewCommandError(ErrInvalidInput, "Failed to parse response", err.Error())
	}

	formatter := output.NewFormatter(outputFormat, os.Stdout)
	formatter.Print(api)

	return nil
}

func handleAPIUpdate(name string) error {
	if err := validateServerConnection(); err != nil {
		return err
	}
	if err := validateNamespace(); err != nil {
		return err
	}
	if err := validateResourceName(name); err != nil {
		return err
	}

	field := promptInput("Field to update")
	if field == "" {
		return NewCommandError(ErrInvalidInput, "Field cannot be empty")
	}

	value := promptInput("New value")
	if value == "" {
		return NewCommandError(ErrInvalidInput, "Value cannot be empty")
	}

	updatePayload := map[string]interface{}{
		field: value,
	}

	response, err := apiClient.PutSimple(fmt.Sprintf("/api/v1/namespaces/%s/apis/%s", namespace, name), updatePayload)
	if err != nil {
		return NewCommandError(ErrNetwork, "Failed to update API", err.Error())
	}

	if response.StatusCode == 404 {
		return NewCommandError(ErrNotFound, fmt.Sprintf("API '%s' not found", name))
	}

	if response.StatusCode >= 400 {
		return NewCommandError(ErrServerError, fmt.Sprintf("Server error (%d)", response.StatusCode), response.Status)
	}

	printSuccessMessage(fmt.Sprintf("API '%s' updated successfully", name))

	return nil
}

func handleAPIDelete(name string) error {
	if err := validateServerConnection(); err != nil {
		return err
	}
	if err := validateNamespace(); err != nil {
		return err
	}
	if err := validateResourceName(name); err != nil {
		return err
	}

	if !confirmAction("Are you sure you want to delete this API?") {
		printWarningMessage("Deletion cancelled")
		return nil
	}

	response, err := apiClient.DeleteSimple(fmt.Sprintf("/api/v1/namespaces/%s/apis/%s", namespace, name))
	if err != nil {
		return NewCommandError(ErrNetwork, "Failed to delete API", err.Error())
	}

	if response.StatusCode == 404 {
		return NewCommandError(ErrNotFound, fmt.Sprintf("API '%s' not found", name))
	}

	if response.StatusCode >= 400 {
		return NewCommandError(ErrServerError, fmt.Sprintf("Server error (%d)", response.StatusCode), response.Status)
	}

	printSuccessMessage(fmt.Sprintf("API '%s' deleted successfully", name))

	return nil
}

func handleAPIApply(filename string) error {
	if err := validateServerConnection(); err != nil {
		return err
	}
	if err := validateNamespace(); err != nil {
		return err
	}

	// Read YAML file
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return NewCommandError(ErrFileNotFound, fmt.Sprintf("File not found: %s", filename))
		}
		return NewCommandError(ErrFileNotFound, "Failed to read file", err.Error())
	}

	// Parse YAML
	var resource map[string]interface{}
	if err := yaml.Unmarshal(data, &resource); err != nil {
		return NewCommandError(ErrYAMLError, "Failed to parse YAML", err.Error())
	}

	// Extract name from metadata
	metadata, ok := resource["metadata"].(map[string]interface{})
	if !ok {
		return NewCommandError(ErrInvalidInput, "Invalid resource: missing or invalid metadata")
	}

	name, ok := metadata["name"].(string)
	if !ok || name == "" {
		return NewCommandError(ErrInvalidInput, "Invalid resource: missing metadata.name")
	}

	if err := validateResourceName(name); err != nil {
		return err
	}

	// Send to server
	response, err := apiClient.PostSimple("/api/v1/apis", resource)
	if err != nil {
		return NewCommandError(ErrNetwork, "Failed to apply API", err.Error())
	}

	if response.StatusCode >= 400 {
		return NewCommandError(ErrServerError, fmt.Sprintf("Server error (%d)", response.StatusCode), response.Status)
	}

	printSuccessMessage(fmt.Sprintf("API '%s' applied successfully", name), "Resource has been created/updated on the server")

	return nil
}

func handleAPIDescribe(name string) error {
	if err := validateServerConnection(); err != nil {
		return err
	}
	if err := validateNamespace(); err != nil {
		return err
	}
	if err := validateResourceName(name); err != nil {
		return err
	}

	response, err := apiClient.GetSimple(fmt.Sprintf("/api/v1/namespaces/%s/apis/%s", namespace, name))
	if err != nil {
		return NewCommandError(ErrNetwork, "Failed to describe API", err.Error())
	}

	if response.StatusCode == 404 {
		return NewCommandError(ErrNotFound, fmt.Sprintf("API '%s' not found in namespace '%s'", name, namespace))
	}

	if response.StatusCode >= 400 {
		return NewCommandError(ErrServerError, fmt.Sprintf("Server error (%d)", response.StatusCode), response.Status)
	}

	var api map[string]interface{}
	if err := response.JSON(&api); err != nil {
		return NewCommandError(ErrInvalidInput, "Failed to parse response", err.Error())
	}

	// Print detailed info
	metadata, ok := api["metadata"].(map[string]interface{})
	if !ok {
		return NewCommandError(ErrInvalidInput, "Invalid API response format")
	}

	spec, ok := api["spec"].(map[string]interface{})
	if !ok {
		return NewCommandError(ErrInvalidInput, "Invalid API response format")
	}

	status, ok := api["status"].(map[string]interface{})
	if !ok {
		status = make(map[string]interface{})
	}

	fmt.Printf("\n📋 API: %s\n", name)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Namespace:  %v\n", metadata["namespace"])
	fmt.Printf("Database:   %v\n", spec["database"])
	fmt.Printf("Table:      %v\n", spec["table"])
	if phase, ok := status["phase"]; ok {
		fmt.Printf("Status:     %v\n", phase)
	}
	if age, ok := metadata["age"]; ok {
		fmt.Printf("Age:        %v\n", age)
	}
	fmt.Println()

	// Try to get events
	eventsResp, _ := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/namespaces/%s/apis/%s/events", namespace, name), nil)
	if eventsResp != nil && eventsResp.StatusCode == 200 {
		var events []map[string]interface{}
		if err := eventsResp.JSON(&events); err == nil && len(events) > 0 {
			fmt.Println("📝 Recent Events:")
			headers := []string{"TYPE", "REASON", "MESSAGE", "AGE"}
			rows := [][]string{}
			for _, event := range events {
				rows = append(rows, []string{
					fmt.Sprintf("%v", event["type"]),
					fmt.Sprintf("%v", event["reason"]),
					fmt.Sprintf("%v", event["message"]),
					fmt.Sprintf("%v", event["age"]),
				})
			}
			printTable(headers, rows)
		}
	}
	fmt.Println()

	return nil
}

func handleAPIDiff(filename string) error {
	if err := validateServerConnection(); err != nil {
		return err
	}
	if err := validateNamespace(); err != nil {
		return err
	}

	// Read YAML file
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return NewCommandError(ErrFileNotFound, fmt.Sprintf("File not found: %s", filename))
		}
		return NewCommandError(ErrFileNotFound, "Failed to read file", err.Error())
	}

	// Parse YAML
	var resource map[string]interface{}
	if err := yaml.Unmarshal(data, &resource); err != nil {
		return NewCommandError(ErrYAMLError, "Failed to parse YAML", err.Error())
	}

	metadata, ok := resource["metadata"].(map[string]interface{})
	if !ok {
		return NewCommandError(ErrInvalidInput, "Invalid resource: missing or invalid metadata")
	}

	name, ok := metadata["name"].(string)
	if !ok || name == "" {
		return NewCommandError(ErrInvalidInput, "Invalid resource: missing metadata.name")
	}

	if err := validateResourceName(name); err != nil {
		return err
	}

	// Get current from server
	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/namespaces/%s/apis/%s", namespace, name), nil)
	if err != nil {
		return NewCommandError(ErrNetwork, "Failed to get resource from server", err.Error())
	}

	if response.StatusCode == 404 {
		fmt.Printf("📄 Resource '%s' does not exist on server\n", name)
		fmt.Printf("⚠️  Applying this file will CREATE a new resource\n\n")
		return nil
	}

	if response.StatusCode >= 400 {
		return NewCommandError(ErrServerError, fmt.Sprintf("Server error (%d)", response.StatusCode), response.Status)
	}

	var current map[string]interface{}
	if err := response.JSON(&current); err != nil {
		return NewCommandError(ErrInvalidInput, "Failed to parse server response", err.Error())
	}

	fileSpec, ok := resource["spec"].(map[string]interface{})
	if !ok {
		return NewCommandError(ErrInvalidInput, "Invalid resource: missing spec")
	}

	currentSpec, ok := current["spec"].(map[string]interface{})
	if !ok {
		return NewCommandError(ErrInvalidInput, "Invalid server response: missing spec")
	}

	fmt.Printf("\n📊 Diff: %s\n", name)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	hasDiff := false
	for key := range fileSpec {
		fileVal := fmt.Sprintf("%v", fileSpec[key])
		currVal := fmt.Sprintf("%v", currentSpec[key])
		if fileVal != currVal {
			hasDiff = true
			fmt.Printf("  %s\n", key)
			fmt.Printf("    - %v (current)\n", currVal)
			fmt.Printf("    + %v (new)\n", fileVal)
		}
	}

	if !hasDiff {
		fmt.Println("✅ No differences - file matches server state")
	}
	fmt.Println()

	return nil
}

func init() {
	APICmd.AddCommand(APICreateCmd)
	APICmd.AddCommand(APIListCmd)
	APICmd.AddCommand(APIGetCmd)
	APICmd.AddCommand(APIUpdateCmd)
	APICmd.AddCommand(APIDeleteCmd)
	APICmd.AddCommand(APIApplyCmd)
	APICmd.AddCommand(APIDescribeCmd)
	APICmd.AddCommand(APIDiffCmd)

	// Apply command flags
	APIApplyCmd.Flags().StringP("filename", "f", "", "YAML file path (required)")
	APIApplyCmd.Flags().BoolP("force", "", false, "Skip waiting for reconciliation")
	APIApplyCmd.Flags().Duration("timeout", 30*time.Second, "Timeout for reconciliation")
	APIApplyCmd.MarkFlagRequired("filename")

	// Diff command flags
	APIDiffCmd.Flags().StringP("filename", "f", "", "YAML file path (required)")
	APIDiffCmd.MarkFlagRequired("filename")
}
