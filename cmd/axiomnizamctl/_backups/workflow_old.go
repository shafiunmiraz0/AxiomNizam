package main

import (
	"context"
	"fmt"
	"os"

	"axiomnizam.bitbd.net/axiomnizam/internal/workflows"
	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
)

var WorkflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage workflows",
	Long:  "Create, list, execute, and manage workflows",
}

var WorkflowApplyCmd = &cobra.Command{
	Use:   "apply -f workflow.yaml",
	Short: "Apply a workflow",
	Long:  "Apply a workflow definition",
	RunE: func(cmd *cobra.Command, args []string) error {
		filename, _ := cmd.Flags().GetString("filename")
		if filename == "" {
			return fmt.Errorf("--filename is required")
		}
		return handleWorkflowApply(filename)
	},
}

var WorkflowRunCmd = &cobra.Command{
	Use:   "run [workflow-name]",
	Short: "Run a workflow",
	Long:  "Execute a workflow manually",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleWorkflowRun(args[0])
	},
}

var WorkflowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all workflows",
	Long:  "Display all configured workflows",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleWorkflowList()
	},
}

var WorkflowGetCmd = &cobra.Command{
	Use:   "get [workflow-name]",
	Short: "Get workflow details",
	Long:  "Display a specific workflow and its steps",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleWorkflowGet(args[0])
	},
}

var WorkflowStatusCmd = &cobra.Command{
	Use:   "status [execution-id]",
	Short: "Get workflow execution status",
	Long:  "Display the status of a workflow execution",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleWorkflowStatus(args[0])
	},
}

// handleWorkflowApply applies a workflow
func handleWorkflowApply(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read workflow file: %w", err)
	}

	var workflow workflows.Workflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return fmt.Errorf("failed to parse workflow: %w", err)
	}

	ctx := context.Background()
	if err := workflows.AddWorkflow(ctx, &workflow); err != nil {
		return fmt.Errorf("failed to apply workflow: %w", err)
	}

	fmt.Printf("✅ Workflow '%s' applied successfully\n", workflow.Name)
	fmt.Printf("   Steps: %d\n", len(workflow.Steps))
	fmt.Printf("   Enabled: %v\n", workflow.Enabled)

	return nil
}

// handleWorkflowRun runs a workflow
func handleWorkflowRun(workflowName string) error {
	ctx := context.Background()
	triggerContext := map[string]interface{}{
		"source": "cli",
		"user":   "admin",
	}

	fmt.Printf("▶️  Running workflow: %s\n\n", workflowName)

	execution, err := workflows.Execute(ctx, workflowName, triggerContext)
	if err != nil {
		return fmt.Errorf("failed to execute workflow: %w", err)
	}

	fmt.Printf("Execution ID: %s\n", execution.ID)
	fmt.Printf("Status: %s\n", execution.Status)
	fmt.Printf("Completed Steps: %d/%d\n\n", execution.CompletedSteps, execution.TotalSteps)

	fmt.Println("Step Executions:")
	for i, step := range execution.StepExecutions {
		duration := "pending"
		if step.EndTime != nil {
			duration = step.EndTime.Sub(step.StartTime).String()
		}
		fmt.Printf("  %d. [%s] %s (%s)\n", i+1, step.Status, step.StepName, duration)
		if step.Error != "" {
			fmt.Printf("     Error: %s\n", step.Error)
		}
	}

	if execution.Error != "" {
		fmt.Printf("\n❌ Workflow failed: %s\n", execution.Error)
	} else {
		fmt.Printf("\n✅ Workflow completed: %s\n", execution.Status)
	}

	return nil
}

// handleWorkflowList lists workflows
func handleWorkflowList() error {
	workflows := workflows.GlobalWorkflowEngine.ListWorkflows()

	fmt.Println("📋 Configured Workflows\n")
	fmt.Printf("%-30s %-5s %-10s %s\n", "NAME", "STEPS", "ENABLED", "DESCRIPTION")
	fmt.Println(repeatString("─", 80))

	for _, w := range workflows {
		enabled := "yes"
		if !w.Enabled {
			enabled = "no"
		}
		fmt.Printf("%-30s %-5d %-10s %s\n", w.Name, len(w.Steps), enabled, w.Description)
	}

	return nil
}

// handleWorkflowGet gets workflow details
func handleWorkflowGet(name string) error {
	workflow := workflows.GlobalWorkflowEngine.GetWorkflow(name)
	if workflow == nil {
		return fmt.Errorf("workflow not found: %s", name)
	}

	fmt.Printf("📋 Workflow: %s\n\n", name)
	fmt.Printf("Description: %s\n", workflow.Description)
	fmt.Printf("Version: %s\n", workflow.Version)
	fmt.Printf("Enabled: %v\n", workflow.Enabled)
	fmt.Printf("Triggers: %d\n", len(workflow.Triggers))
	fmt.Printf("Steps: %d\n\n", len(workflow.Steps))

	fmt.Println("Steps:")
	for i, step := range workflow.Steps {
		fmt.Printf("  %d. [%s] %s\n", i+1, step.Type, step.Name)
		fmt.Printf("     Action: %s\n", step.Action)
		fmt.Printf("     Timeout: %v\n", step.Timeout)
		fmt.Printf("     Retry: %d\n", step.Retry)
	}

	return nil
}

// handleWorkflowStatus shows execution status
func handleWorkflowStatus(executionID string) error {
	execution := workflows.GlobalWorkflowEngine.GetExecution(executionID)
	if execution == nil {
		return fmt.Errorf("execution not found: %s", executionID)
	}

	fmt.Printf("📊 Execution: %s\n\n", executionID)
	fmt.Printf("Workflow: %s\n", execution.WorkflowName)
	fmt.Printf("Status: %s\n", execution.Status)
	fmt.Printf("Completed Steps: %d/%d\n", execution.CompletedSteps, execution.TotalSteps)
	fmt.Printf("Duration: %v\n\n", execution.EndTime.Sub(execution.StartTime))

	if len(execution.StepExecutions) > 0 {
		fmt.Println("Step Executions:")
		for i, step := range execution.StepExecutions {
			duration := "running"
			if step.EndTime != nil {
				duration = step.EndTime.Sub(step.StartTime).String()
			}
			fmt.Printf("  %d. [%s] %s (%s)\n", i+1, step.Status, step.StepName, duration)
		}
	}

	if execution.Error != "" {
		fmt.Printf("\nError: %s\n", execution.Error)
	}

	return nil
}

func init() {
	WorkflowApplyCmd.Flags().StringP("filename", "f", "", "Path to workflow file")
	WorkflowCmd.AddCommand(WorkflowApplyCmd)
	WorkflowCmd.AddCommand(WorkflowRunCmd)
	WorkflowCmd.AddCommand(WorkflowListCmd)
	WorkflowCmd.AddCommand(WorkflowGetCmd)
	WorkflowCmd.AddCommand(WorkflowStatusCmd)
}
