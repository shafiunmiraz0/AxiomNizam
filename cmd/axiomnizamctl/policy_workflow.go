package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/output"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Policy Commands
var PolicyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Manage policies",
	Long:  "Apply, list, get, and delete policy resources",
}

var PolicyApplyCmd = &cobra.Command{
	Use:   "apply -f [file]",
	Short: "Apply policy from YAML",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("filename")
		if file == "" {
			fmt.Println("❌ filename flag is required")
			return
		}
		handlePolicyApply(file)
	},
}

var PolicyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all policies",
	Run: func(cmd *cobra.Command, args []string) {
		handlePolicyList()
	},
}

var PolicyGetCmd = &cobra.Command{
	Use:   "get [name]",
	Short: "Get policy details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handlePolicyGet(args[0])
	},
}

var PolicyDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a policy",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handlePolicyDelete(args[0])
	},
}

var PolicyDescribeCmd = &cobra.Command{
	Use:   "describe [name]",
	Short: "Show detailed policy information",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handlePolicyDescribe(args[0])
	},
}

var PolicyDiffCmd = &cobra.Command{
	Use:   "diff -f [file]",
	Short: "Show differences between file and server",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("filename")
		if file == "" {
			fmt.Println("❌ filename flag is required")
			return
		}
		handlePolicyDiff(file)
	},
}

// Workflow Commands
var WorkflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage workflows",
	Long:  "Run, list, apply, and check status of workflows",
}

var WorkflowApplyCmd = &cobra.Command{
	Use:   "apply -f [file]",
	Short: "Apply workflow from YAML",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("filename")
		if file == "" {
			fmt.Println("❌ filename flag is required")
			return
		}
		handleWorkflowApply(file)
	},
}

var WorkflowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all workflows",
	Run: func(cmd *cobra.Command, args []string) {
		handleWorkflowList()
	},
}

var WorkflowRunCmd = &cobra.Command{
	Use:   "run [name]",
	Short: "Run a workflow",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleWorkflowRun(args[0])
	},
}

var WorkflowStatusCmd = &cobra.Command{
	Use:   "status [name|execution-id]",
	Short: "Get workflow or execution status",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleWorkflowStatus(args[0])
	},
}

var WorkflowDescribeCmd = &cobra.Command{
	Use:   "describe [name]",
	Short: "Show detailed workflow information",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleWorkflowDescribe(args[0])
	},
}

var WorkflowDiffCmd = &cobra.Command{
	Use:   "diff -f [file]",
	Short: "Show differences between file and server",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("filename")
		if file == "" {
			fmt.Println("❌ filename flag is required")
			return
		}
		handleWorkflowDiff(file)
	},
}

// Policy handlers
func handlePolicyApply(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("❌ Failed to read file: %v\n", err)
		return
	}

	var resource map[string]interface{}
	if err := yaml.Unmarshal(data, &resource); err != nil {
		fmt.Printf("❌ Failed to parse YAML: %v\n", err)
		return
	}

	metadata, ok := resource["metadata"].(map[string]interface{})
	if !ok {
		fmt.Println("❌ Invalid resource: missing metadata")
		return
	}

	name, ok := metadata["name"].(string)
	if !ok {
		fmt.Println("❌ Invalid resource: missing metadata.name")
		return
	}

	response, err := apiClient.Post(context.Background(), "/api/v1/policies", resource)
	if err != nil {
		fmt.Printf("❌ Failed to apply policy: %v\n", err)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		fmt.Printf("✅ Policy '%s' applied successfully\n", name)
	} else {
		fmt.Printf("❌ Failed: %s\n", response.Status)
	}
}

func handlePolicyList() {
	response, err := apiClient.Get(context.Background(), "/api/v1/policies", nil)
	if err != nil {
		fmt.Printf("❌ Failed to list policies: %v\n", err)
		return
	}

	var policies []map[string]interface{}
	if err := response.JSON(&policies); err != nil {
		fmt.Printf("❌ Failed to parse response: %v\n", err)
		return
	}

	formatter := output.NewFormatter(outputFormat, os.Stdout)
	formatter.Print(policies)
}

func handlePolicyGet(name string) {
	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/namespaces/%s/policies/%s", namespace, name), nil)
	if err != nil {
		fmt.Printf("❌ Failed to get policy: %v\n", err)
		return
	}

	var policy map[string]interface{}
	if err := response.JSON(&policy); err != nil {
		fmt.Printf("❌ Failed to parse response: %v\n", err)
		return
	}

	formatter := output.NewFormatter(outputFormat, os.Stdout)
	formatter.Print(policy)
}

func handlePolicyDelete(name string) {
	if !confirmAction("Are you sure?") {
		fmt.Println("❌ Cancelled")
		return
	}

	response, err := apiClient.Delete(context.Background(), fmt.Sprintf("/api/v1/namespaces/%s/policies/%s", namespace, name))
	if err != nil {
		output.PrintError(output.ErrServerError, err.Error())
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		output.PrintSuccess(fmt.Sprintf("Policy '%s' deleted", name))
	} else {
		output.PrintError(output.ErrNotFound, fmt.Sprintf("Failed to delete policy: %s", response.Status))
	}
}

func handlePolicyDescribe(name string) {
	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/namespaces/%s/policies/%s", namespace, name), nil)
	if err != nil {
		output.PrintError(output.ErrServerError, err.Error())
		return
	}

	var policy map[string]interface{}
	if err := response.JSON(&policy); err != nil {
		output.PrintError(output.ErrInvalidInput, "Failed to parse response")
		return
	}

	metadata := policy["metadata"].(map[string]interface{})
	spec := policy["spec"].(map[string]interface{})

	fmt.Printf("\n📋 Policy: %s\n", name)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Namespace:  %v\n", metadata["namespace"])
	fmt.Printf("Rules:      %v\n", spec["rules"])
	fmt.Println()
}

func handlePolicyDiff(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		output.PrintError(output.ErrInvalidInput, "Failed to read file: "+err.Error())
		return
	}

	var resource map[string]interface{}
	if err := yaml.Unmarshal(data, &resource); err != nil {
		output.PrintError(output.ErrInvalidYAML, "Failed to parse YAML: "+err.Error())
		return
	}

	metadata := resource["metadata"].(map[string]interface{})
	name := metadata["name"].(string)

	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/namespaces/%s/policies/%s", namespace, name), nil)
	if err != nil || response.StatusCode == 404 {
		fmt.Printf("📄 Policy '%s' does not exist on server\n", name)
		fmt.Printf("⚠️  Applying will CREATE a new policy\n\n")
		return
	}

	var current map[string]interface{}
	response.JSON(&current)

	fmt.Printf("\n📊 Policy Diff: %s\n", name)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Println("✅ Check before applying")
	fmt.Println()
}

// Workflow handlers
func handleWorkflowApply(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("❌ Failed to read file: %v\n", err)
		return
	}

	var resource map[string]interface{}
	if err := yaml.Unmarshal(data, &resource); err != nil {
		fmt.Printf("❌ Failed to parse YAML: %v\n", err)
		return
	}

	metadata, ok := resource["metadata"].(map[string]interface{})
	if !ok {
		fmt.Println("❌ Invalid resource: missing metadata")
		return
	}

	name, ok := metadata["name"].(string)
	if !ok {
		fmt.Println("❌ Invalid resource: missing metadata.name")
		return
	}

	response, err := apiClient.Post(context.Background(), "/api/v1/workflows", resource)
	if err != nil {
		fmt.Printf("❌ Failed to apply workflow: %v\n", err)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		fmt.Printf("✅ Workflow '%s' applied\n", name)
	} else {
		fmt.Printf("❌ Failed: %s\n", response.Status)
	}
}

func handleWorkflowList() {
	response, err := apiClient.Get(context.Background(), "/api/v1/workflows", nil)
	if err != nil {
		fmt.Printf("❌ Failed to list workflows: %v\n", err)
		return
	}

	var workflows []map[string]interface{}
	if err := response.JSON(&workflows); err != nil {
		fmt.Printf("❌ Failed to parse response: %v\n", err)
		return
	}

	formatter := output.NewFormatter(outputFormat, os.Stdout)
	formatter.Print(workflows)
}

func handleWorkflowRun(name string) {
	response, err := apiClient.Post(context.Background(), fmt.Sprintf("/api/v1/workflows/%s/run", name), nil)
	if err != nil {
		fmt.Printf("❌ Failed to run workflow: %v\n", err)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		var runResponse struct {
			Message   string `json:"message"`
			Status    string `json:"status"`
			Execution struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"execution"`
		}

		if err := response.JSON(&runResponse); err != nil {
			fmt.Printf("✅ Workflow '%s' started\n", name)
			return
		}

		execID := runResponse.Execution.ID
		execStatus := runResponse.Execution.Status
		if execStatus == "" {
			execStatus = runResponse.Status
		}

		if runResponse.Message != "" {
			fmt.Printf("✅ %s\n", runResponse.Message)
		} else {
			fmt.Printf("✅ Workflow '%s' executed\n", name)
		}

		if execID != "" {
			fmt.Printf("   Execution ID: %s\n", execID)
		}
		if execStatus != "" {
			fmt.Printf("   Status: %s\n", execStatus)
		}
	} else {
		fmt.Printf("❌ Failed: %s\n", response.Status)
	}
}

func handleWorkflowStatus(target string) {
	target = strings.TrimSpace(target)
	if target == "" {
		output.PrintError(output.ErrInvalidInput, "workflow name or execution id is required")
		return
	}

	if looksLikeWorkflowExecutionID(target) {
		handleWorkflowExecutionStatus(target)
		return
	}

	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/namespaces/%s/workflows/%s", namespace, target), nil)
	if err != nil {
		output.PrintError(output.ErrServerError, err.Error())
		return
	}

	var workflow map[string]interface{}
	if err := response.JSON(&workflow); err != nil {
		output.PrintError(output.ErrInvalidInput, "Failed to parse response")
		return
	}

	statusRaw, ok := workflow["status"]
	if !ok {
		output.PrintError(output.ErrInvalidInput, "Workflow response missing status")
		return
	}
	status, ok := statusRaw.(map[string]interface{})
	if !ok {
		output.PrintError(output.ErrInvalidInput, "Invalid workflow status response")
		return
	}

	fmt.Printf("\n📊 Workflow: %s\n", target)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	if phase, ok := status["phase"]; ok {
		fmt.Printf("Status:      %v\n", phase)
	}
	if progress, ok := status["progress"]; ok {
		fmt.Printf("Progress:    %v\n", progress)
	}
	if lastRun, ok := status["lastRun"]; ok {
		fmt.Printf("Last Run:    %v\n", lastRun)
	}
	if nextRun, ok := status["nextRun"]; ok {
		fmt.Printf("Next Run:    %v\n", nextRun)
	}
	fmt.Println()
}

func handleWorkflowExecutionStatus(executionID string) {
	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/workflows/executions/%s", executionID), nil)
	if err != nil {
		output.PrintError(output.ErrServerError, err.Error())
		return
	}

	var execution map[string]interface{}
	if err := response.JSON(&execution); err != nil {
		output.PrintError(output.ErrInvalidInput, "Failed to parse execution response")
		return
	}

	fmt.Printf("\n📊 Workflow Execution: %s\n", executionID)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	if workflowName, ok := execution["workflowName"]; ok {
		fmt.Printf("Workflow:    %v\n", workflowName)
	}
	if status, ok := execution["status"]; ok {
		fmt.Printf("Status:      %v\n", status)
	}
	if completed, ok := execution["completedSteps"]; ok {
		fmt.Printf("Completed:   %v", completed)
		if total, ok := execution["totalSteps"]; ok {
			fmt.Printf("/%v", total)
		}
		fmt.Printf(" steps\n")
	}
	if startedAt, ok := execution["startTime"]; ok {
		fmt.Printf("Started At:  %v\n", startedAt)
	}
	if endedAt, ok := execution["endTime"]; ok {
		fmt.Printf("Ended At:    %v\n", endedAt)
	}
	if errValue, ok := execution["error"]; ok {
		if errText := strings.TrimSpace(fmt.Sprintf("%v", errValue)); errText != "" {
			fmt.Printf("Error:       %s\n", errText)
		}
	}
	fmt.Println()
}

func looksLikeWorkflowExecutionID(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(normalized, "exec-") || strings.HasPrefix(normalized, "execution-")
}

func handleWorkflowDescribe(name string) {
	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/namespaces/%s/workflows/%s", namespace, name), nil)
	if err != nil {
		output.PrintError(output.ErrServerError, err.Error())
		return
	}

	var workflow map[string]interface{}
	if err := response.JSON(&workflow); err != nil {
		output.PrintError(output.ErrInvalidInput, "Failed to parse response")
		return
	}

	_ = workflow["metadata"].(map[string]interface{})
	metadata := workflow["metadata"].(map[string]interface{})
	status := workflow["status"].(map[string]interface{})
	spec := workflow["spec"].(map[string]interface{})

	fmt.Printf("\n📋 Workflow: %s\n", name)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Namespace:  %v\n", metadata["namespace"])
	if schedule, ok := spec["schedule"]; ok {
		fmt.Printf("Schedule:   %v\n", schedule)
	}
	if phase, ok := status["phase"]; ok {
		fmt.Printf("Phase:      %v\n", phase)
	}
	fmt.Println()
}

func handleWorkflowDiff(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		output.PrintError(output.ErrInvalidInput, "Failed to read file: "+err.Error())
		return
	}

	var resource map[string]interface{}
	if err := yaml.Unmarshal(data, &resource); err != nil {
		output.PrintError(output.ErrInvalidYAML, "Failed to parse YAML: "+err.Error())
		return
	}

	_ = resource["metadata"].(map[string]interface{})
	name := resource["metadata"].(map[string]interface{})["name"].(string)

	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/namespaces/%s/workflows/%s", namespace, name), nil)
	if err != nil || response.StatusCode == 404 {
		fmt.Printf("📄 Workflow '%s' does not exist on server\n", name)
		fmt.Printf("⚠️  Applying will CREATE a new workflow\n\n")
		return
	}

	var current map[string]interface{}
	response.JSON(&current)

	fmt.Printf("\n📊 Workflow Diff: %s\n", name)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Println("✅ Check before applying")
	fmt.Println()
}

func init() {
	PolicyCmd.AddCommand(PolicyApplyCmd)
	PolicyCmd.AddCommand(PolicyListCmd)
	PolicyCmd.AddCommand(PolicyGetCmd)
	PolicyCmd.AddCommand(PolicyDeleteCmd)
	PolicyCmd.AddCommand(PolicyDescribeCmd)
	PolicyCmd.AddCommand(PolicyDiffCmd)

	PolicyApplyCmd.Flags().StringP("filename", "f", "", "YAML file path")
	PolicyDiffCmd.Flags().StringP("filename", "f", "", "YAML file path")

	WorkflowCmd.AddCommand(WorkflowApplyCmd)
	WorkflowCmd.AddCommand(WorkflowListCmd)
	WorkflowCmd.AddCommand(WorkflowRunCmd)
	WorkflowCmd.AddCommand(WorkflowStatusCmd)
	WorkflowCmd.AddCommand(WorkflowDescribeCmd)
	WorkflowCmd.AddCommand(WorkflowDiffCmd)

	WorkflowApplyCmd.Flags().StringP("filename", "f", "", "YAML file path")
	WorkflowDiffCmd.Flags().StringP("filename", "f", "", "YAML file path")
}
