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

// DataSource Commands
var DataSourceCmd = &cobra.Command{
	Use:   "datasource",
	Short: "Manage data sources",
	Long:  "Create, list, test, apply, and delete data source resources",
}

var DataSourceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new data source",
	Run: func(cmd *cobra.Command, args []string) {
		handleDataSourceCreate()
	},
}

var DataSourceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all data sources",
	Run: func(cmd *cobra.Command, args []string) {
		handleDataSourceList()
	},
}

var DataSourceTestCmd = &cobra.Command{
	Use:   "test [name]",
	Short: "Test data source connection",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleDataSourceTest(args[0])
	},
}

var DataSourceApplyCmd = &cobra.Command{
	Use:   "apply -f [file]",
	Short: "Apply data source from YAML",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("filename")
		if file == "" {
			fmt.Println("❌ filename flag is required")
			return
		}
		handleDataSourceApply(file)
	},
}

var DataSourceDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a data source",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleDataSourceDelete(args[0])
	},
}

var DataSourceDescribeCmd = &cobra.Command{
	Use:   "describe [name]",
	Short: "Show detailed datasource information",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleDataSourceDescribe(args[0])
	},
}

var DataSourceDiffCmd = &cobra.Command{
	Use:   "diff -f [file]",
	Short: "Show differences between file and server",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("filename")
		if file == "" {
			fmt.Println("❌ filename flag is required")
			return
		}
		handleDataSourceDiff(file)
	},
}

// DataSource Get Command
var DataSourceGetCmd = &cobra.Command{
	Use:   "get [name]",
	Short: "Get datasource details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleDataSourceGet(args[0])
	},
}

// DataSource Update Command
var DataSourceUpdateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Update a datasource field",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleDataSourceUpdate(args[0])
	},
}

// Job Commands
var JobCmd = &cobra.Command{
	Use:   "job",
	Short: "Manage jobs",
	Long:  "Create, list, run, get, view logs, cancel, and manage schedules for jobs",
}

var JobCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new job",
	Run: func(cmd *cobra.Command, args []string) {
		handleJobCreate()
	},
}

var JobListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all jobs",
	Run: func(cmd *cobra.Command, args []string) {
		handleJobList()
	},
}

var JobGetCmd = &cobra.Command{
	Use:   "get [job-id]",
	Short: "Get job details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleJobGet(args[0])
	},
}

var JobRunCmd = &cobra.Command{
	Use:   "run [name]",
	Short: "Run a job",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleJobRun(args[0])
	},
}

var JobDeleteCmd2 = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a job",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleJobDelete(args[0])
	},
}

var JobLogsCmd = &cobra.Command{
	Use:   "logs [job-id]",
	Short: "View job logs",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleJobLogs(args[0])
	},
}

var JobCancelCmd = &cobra.Command{
	Use:   "cancel [job-id]",
	Short: "Cancel a running job",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleJobCancel(args[0])
	},
}

var JobDescribeCmd = &cobra.Command{
	Use:   "describe [job-id]",
	Short: "Show detailed job information",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleJobDescribe(args[0])
	},
}

var JobDiffCmd = &cobra.Command{
	Use:   "diff -f [file]",
	Short: "Show differences between file and server",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("filename")
		if file == "" {
			fmt.Println("❌ filename flag is required")
			return
		}
		handleJobDiff(file)
	},
}

var JobStatusCmd = &cobra.Command{
	Use:   "status [job-id]",
	Short: "Get job execution status",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleJobStatus(args[0])
	},
}

var JobScheduleListCmd = &cobra.Command{
	Use:   "schedule-list",
	Short: "List all job schedules",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleJobScheduleList()
	},
}

var JobScheduleSetCmd = &cobra.Command{
	Use:   "schedule-set [job-id]",
	Short: "Set or update a job schedule",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		schedule, _ := cmd.Flags().GetString("schedule")
		return handleJobScheduleSet(args[0], schedule)
	},
}

var JobScheduleRemoveCmd = &cobra.Command{
	Use:   "schedule-remove [job-id]",
	Short: "Remove a job schedule",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleJobScheduleRemove(args[0])
	},
}

// DataSource handlers
func handleDataSourceCreate() {
	fmt.Println("📝 Create DataSource")

	name := promptInput("Name")
	dsType := promptInput("Type (postgres/mysql/oracle)")
	host := promptInput("Host")
	port := promptInput("Port")
	database := promptInput("Database")

	resource := map[string]interface{}{
		"apiVersion": "axiom-nizam.io/v1",
		"kind":       "DataSource",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"type":     dsType,
			"host":     host,
			"port":     port,
			"database": database,
		},
	}

	response, err := apiClient.Post(context.Background(), "/api/v1/datasources", resource)
	if err != nil {
		fmt.Printf("❌ Failed to create datasource: %v\n", err)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		fmt.Printf("✅ DataSource '%s' created\n", name)
	} else {
		fmt.Printf("❌ Failed: %s\n", response.Status)
	}
}

func handleDataSourceList() {
	response, err := apiClient.Get(context.Background(), "/api/v1/datasources", nil)
	if err != nil {
		fmt.Printf("❌ Failed to list datasources: %v\n", err)
		return
	}

	var datasources []map[string]interface{}
	if err := response.JSON(&datasources); err != nil {
		fmt.Printf("❌ Failed to parse response: %v\n", err)
		return
	}

	formatter := output.NewFormatter(outputFormat, os.Stdout)
	formatter.Print(datasources)
}

func handleDataSourceGet(name string) {
	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/datasources/%s", name), nil)
	if err != nil {
		fmt.Printf("❌ Failed to get datasource: %v\n", err)
		return
	}

	if response.StatusCode == 404 {
		fmt.Printf("❌ DataSource '%s' not found\n", name)
		return
	}

	var datasource map[string]interface{}
	if err := response.JSON(&datasource); err != nil {
		fmt.Printf("❌ Failed to parse response: %v\n", err)
		return
	}

	formatter := output.NewFormatter(outputFormat, os.Stdout)
	formatter.Print(datasource)
}

func handleDataSourceUpdate(name string) {
	field := promptInput("Field to update")
	if field == "" {
		fmt.Println("❌ Field cannot be empty")
		return
	}

	value := promptInput("New value")
	if value == "" {
		fmt.Println("❌ Value cannot be empty")
		return
	}

	updatePayload := map[string]interface{}{
		field: value,
	}

	response, err := apiClient.Put(context.Background(), fmt.Sprintf("/api/v1/datasources/%s", name), updatePayload)
	if err != nil {
		fmt.Printf("❌ Failed to update datasource: %v\n", err)
		return
	}

	if response.StatusCode == 404 {
		fmt.Printf("❌ DataSource '%s' not found\n", name)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		fmt.Printf("✅ DataSource '%s' updated successfully\n", name)
	} else {
		fmt.Printf("❌ Failed: %s\n", response.Status)
	}
}

func handleDataSourceTest(name string) {
	fmt.Printf("🧪 Testing connection to '%s'...\n", name)

	response, err := apiClient.Post(context.Background(), fmt.Sprintf("/api/v1/datasources/%s/test", name), nil)
	if err != nil {
		fmt.Printf("❌ Connection test failed: %v\n", err)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		fmt.Printf("✅ Connection successful\n")
	} else {
		fmt.Printf("❌ Connection failed: %s\n", response.Status)
	}
}

func handleDataSourceApply(filename string) {
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

	response, err := apiClient.Post(context.Background(), "/api/v1/datasources", resource)
	if err != nil {
		fmt.Printf("❌ Failed to apply datasource: %v\n", err)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		fmt.Printf("✅ DataSource '%s' applied\n", name)
	} else {
		fmt.Printf("❌ Failed: %s\n", response.Status)
	}
}

func handleDataSourceDelete(name string) {
	if !confirmAction("Are you sure?") {
		fmt.Println("❌ Cancelled")
		return
	}

	response, err := apiClient.Delete(context.Background(), fmt.Sprintf("/api/v1/datasources/%s", name))
	if err != nil {
		fmt.Printf("❌ Failed to delete datasource: %v\n", err)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		fmt.Printf("✅ DataSource '%s' deleted\n", name)
	} else {
		fmt.Printf("❌ Failed: %s\n", response.Status)
	}
}

func handleDataSourceDescribe(name string) {
	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/datasources/%s", name), nil)
	if err != nil {
		output.PrintError(output.ErrNotFound, fmt.Sprintf("DataSource '%s' not found", name))
		return
	}

	if response.StatusCode != 200 {
		output.PrintError(output.ErrServerError, response.Status)
		return
	}

	var datasource map[string]interface{}
	if err := response.JSON(&datasource); err != nil {
		output.PrintError(output.ErrInvalidYAML, "Failed to parse response")
		return
	}

	fmt.Println("\n📊 DataSource Details:")
	fmt.Println("=" + string([]byte{61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61}))

	if metadata, ok := datasource["metadata"].(map[string]interface{}); ok {
		fmt.Printf("Name: %v\n", metadata["name"])
		fmt.Printf("Namespace: %v\n", metadata["namespace"])
		fmt.Printf("Created: %v\n", metadata["creationTimestamp"])
	}

	if spec, ok := datasource["spec"].(map[string]interface{}); ok {
		fmt.Println("\nSpec:")
		fmt.Printf("  Type: %v\n", spec["type"])
		fmt.Printf("  Host: %v\n", spec["host"])
		fmt.Printf("  Port: %v\n", spec["port"])
		fmt.Printf("  Database: %v\n", spec["database"])
	}

	if status, ok := datasource["status"].(map[string]interface{}); ok {
		fmt.Println("\nStatus:")
		fmt.Printf("  Connected: %v\n", status["connected"])
		fmt.Printf("  LastCheck: %v\n", status["lastCheck"])
	}
}

func handleDataSourceDiff(filename string) {
	fileData, err := os.ReadFile(filename)
	if err != nil {
		output.PrintError(output.ErrInvalidInput, fmt.Sprintf("Cannot read file: %v", err))
		return
	}

	var fileResource map[string]interface{}
	if err := yaml.Unmarshal(fileData, &fileResource); err != nil {
		output.PrintError(output.ErrInvalidYAML, "Invalid YAML in file")
		return
	}

	metadata, ok := fileResource["metadata"].(map[string]interface{})
	if !ok {
		output.PrintError(output.ErrInvalidInput, "Missing metadata in file")
		return
	}

	name, ok := metadata["name"].(string)
	if !ok {
		output.PrintError(output.ErrInvalidInput, "Missing metadata.name in file")
		return
	}

	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/datasources/%s", name), nil)
	if err != nil {
		output.PrintError(output.ErrNotFound, fmt.Sprintf("DataSource '%s' not found on server", name))
		return
	}

	var serverResource map[string]interface{}
	if err := response.JSON(&serverResource); err != nil {
		output.PrintError(output.ErrInvalidYAML, "Failed to parse server response")
		return
	}

	fmt.Println("\n📋 Changes (- server, + file):")
	fmt.Println("=" + string([]byte{61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61}))

	fileSpec, _ := fileResource["spec"].(map[string]interface{})
	serverSpec, _ := serverResource["spec"].(map[string]interface{})

	for key := range fileSpec {
		fileVal := fmt.Sprintf("%v", fileSpec[key])
		serverVal := fmt.Sprintf("%v", serverSpec[key])
		if fileVal != serverVal {
			fmt.Printf("- %s: %v\n", key, serverVal)
			fmt.Printf("+ %s: %v\n", key, fileVal)
		}
	}
}

// Job handlers
func handleJobCreate() {
	fmt.Println("📝 Create Job")

	name := promptInput("Job Name")
	if name == "" {
		fmt.Println("❌ Job name cannot be empty")
		return
	}

	jobType := promptInput("Type (sync/transform/backup)")
	schedule := promptInput("Schedule (cron expression, leave empty for manual)")

	resource := map[string]interface{}{
		"apiVersion": "axiom-nizam.io/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"type":     jobType,
			"schedule": schedule,
		},
	}

	response, err := apiClient.Post(context.Background(), "/api/v1/jobs", resource)
	if err != nil {
		fmt.Printf("❌ Failed to create job: %v\n", err)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		fmt.Printf("✅ Job '%s' created\n", name)
	} else {
		fmt.Printf("❌ Failed: %s\n", response.Status)
	}
}

func handleJobList() {
	response, err := apiClient.Get(context.Background(), "/api/v1/jobs", nil)
	if err != nil {
		fmt.Printf("❌ Failed to list jobs: %v\n", err)
		return
	}

	var jobs []map[string]interface{}
	if err := response.JSON(&jobs); err != nil {
		fmt.Printf("❌ Failed to parse response: %v\n", err)
		return
	}

	formatter := output.NewFormatter(outputFormat, os.Stdout)
	formatter.Print(jobs)
}

func handleJobGet(jobID string) {
	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/jobs/%s", jobID), nil)
	if err != nil {
		fmt.Printf("❌ Failed to get job: %v\n", err)
		return
	}

	var job map[string]interface{}
	if err := response.JSON(&job); err != nil {
		fmt.Printf("❌ Failed to parse response: %v\n", err)
		return
	}

	formatter := output.NewFormatter(outputFormat, os.Stdout)
	formatter.Print(job)
}

func handleJobLogs(jobID string) {
	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/jobs/%s/logs", jobID), nil)
	if err != nil {
		fmt.Printf("❌ Failed to get logs: %v\n", err)
		return
	}

	var logs map[string]interface{}
	if err := response.JSON(&logs); err != nil {
		fmt.Printf("❌ Failed to parse response: %v\n", err)
		return
	}

	formatter := output.NewFormatter(outputFormat, os.Stdout)
	formatter.Print(logs)
}

func handleJobCancel(jobID string) {
	if !confirmAction("Cancel this job?") {
		fmt.Println("❌ Cancelled")
		return
	}

	response, err := apiClient.Post(context.Background(), fmt.Sprintf("/api/v1/jobs/%s/cancel", jobID), nil)
	if err != nil {
		fmt.Printf("❌ Failed to cancel job: %v\n", err)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		fmt.Printf("✅ Job '%s' cancelled\n", jobID)
	} else {
		fmt.Printf("❌ Failed: %s\n", response.Status)
	}
}

func handleJobRun(name string) {
	response, err := apiClient.Post(context.Background(), fmt.Sprintf("/api/v1/jobs/%s/run", name), nil)
	if err != nil {
		fmt.Printf("❌ Failed to run job: %v\n", err)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		fmt.Printf("✅ Job '%s' started\n", name)
	} else {
		fmt.Printf("❌ Failed: %s\n", response.Status)
	}
}

func handleJobDelete(name string) {
	if !confirmAction("Are you sure you want to delete this job?") {
		fmt.Println("❌ Cancelled")
		return
	}

	response, err := apiClient.Delete(context.Background(), fmt.Sprintf("/api/v1/jobs/%s", name))
	if err != nil {
		fmt.Printf("❌ Failed to delete job: %v\n", err)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		fmt.Printf("✅ Job '%s' deleted\n", name)
	} else {
		fmt.Printf("❌ Failed: %s\n", response.Status)
	}
}

func handleJobDescribe(jobID string) {
	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/jobs/%s", jobID), nil)
	if err != nil {
		output.PrintError(output.ErrNotFound, fmt.Sprintf("Job '%s' not found", jobID))
		return
	}

	if response.StatusCode != 200 {
		output.PrintError(output.ErrServerError, response.Status)
		return
	}

	var job map[string]interface{}
	if err := response.JSON(&job); err != nil {
		output.PrintError(output.ErrInvalidYAML, "Failed to parse response")
		return
	}

	fmt.Println("\n⚙️ Job Details:")
	fmt.Println("=" + string([]byte{61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61}))

	if metadata, ok := job["metadata"].(map[string]interface{}); ok {
		fmt.Printf("ID: %v\n", metadata["id"])
		fmt.Printf("Name: %v\n", metadata["name"])
		fmt.Printf("Created: %v\n", metadata["creationTimestamp"])
	}

	if spec, ok := job["spec"].(map[string]interface{}); ok {
		fmt.Println("\nSpec:")
		fmt.Printf("  Type: %v\n", spec["type"])
		fmt.Printf("  Interval: %v\n", spec["interval"])
		fmt.Printf("  Retry: %v\n", spec["retryPolicy"])
	}

	if status, ok := job["status"].(map[string]interface{}); ok {
		fmt.Println("\nStatus:")
		fmt.Printf("  Phase: %v\n", status["phase"])
		fmt.Printf("  Progress: %v\n", status["progress"])
		fmt.Printf("  LastRun: %v\n", status["lastRun"])
		fmt.Printf("  NextRun: %v\n", status["nextRun"])
	}
}

func handleJobDiff(filename string) {
	fileData, err := os.ReadFile(filename)
	if err != nil {
		output.PrintError(output.ErrInvalidInput, fmt.Sprintf("Cannot read file: %v", err))
		return
	}

	var fileResource map[string]interface{}
	if err := yaml.Unmarshal(fileData, &fileResource); err != nil {
		output.PrintError(output.ErrInvalidYAML, "Invalid YAML in file")
		return
	}

	metadata, ok := fileResource["metadata"].(map[string]interface{})
	if !ok {
		output.PrintError(output.ErrInvalidInput, "Missing metadata in file")
		return
	}

	jobID, ok := metadata["id"].(string)
	if !ok {
		output.PrintError(output.ErrInvalidInput, "Missing metadata.id in file")
		return
	}

	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/jobs/%s", jobID), nil)
	if err != nil {
		output.PrintError(output.ErrNotFound, fmt.Sprintf("Job '%s' not found on server", jobID))
		return
	}

	var serverResource map[string]interface{}
	if err := response.JSON(&serverResource); err != nil {
		output.PrintError(output.ErrInvalidYAML, "Failed to parse server response")
		return
	}

	fmt.Println("\n📋 Changes (- server, + file):")
	fmt.Println("=" + string([]byte{61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61}))

	fileSpec, _ := fileResource["spec"].(map[string]interface{})
	serverSpec, _ := serverResource["spec"].(map[string]interface{})

	for key := range fileSpec {
		fileVal := fmt.Sprintf("%v", fileSpec[key])
		serverVal := fmt.Sprintf("%v", serverSpec[key])
		if fileVal != serverVal {
			fmt.Printf("- %s: %v\n", key, serverVal)
			fmt.Printf("+ %s: %v\n", key, fileVal)
		}
	}
}

func handleJobStatus(jobID string) {
	response, err := apiClient.Get(context.Background(), fmt.Sprintf("/api/v1/jobs/%s", jobID), nil)
	if err != nil {
		output.PrintError(output.ErrNotFound, fmt.Sprintf("Job '%s' not found", jobID))
		return
	}

	if response.StatusCode != 200 {
		output.PrintError(output.ErrServerError, response.Status)
		return
	}

	var job map[string]interface{}
	if err := response.JSON(&job); err != nil {
		output.PrintError(output.ErrInvalidYAML, "Failed to parse response")
		return
	}

	fmt.Println("\n📊 Job Status:")
	fmt.Println("=" + string([]byte{61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61, 61}))

	if status, ok := job["status"].(map[string]interface{}); ok {
		phase := fmt.Sprintf("%v", status["phase"])
		switch phase {
		case "Running":
			fmt.Printf("Phase: 🔄 %s\n", phase)
		case "Succeeded":
			fmt.Printf("Phase: ✅ %s\n", phase)
		case "Failed":
			fmt.Printf("Phase: ❌ %s\n", phase)
		case "Pending":
			fmt.Printf("Phase: ⏳ %s\n", phase)
		default:
			fmt.Printf("Phase: %s\n", phase)
		}

		fmt.Printf("Progress: %v%%\n", status["progress"])
		fmt.Printf("LastRun: %v\n", status["lastRun"])
		fmt.Printf("NextRun: %v\n", status["nextRun"])

		if lastErr, ok := status["lastError"].(string); ok && lastErr != "" {
			fmt.Printf("Last Error: %s\n", lastErr)
		}
	}
}

func handleJobScheduleList() error {
	return getAndPrint("/api/v1/jobs/schedules")
}

func handleJobScheduleSet(jobID, schedule string) error {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return NewCommandError(ErrInvalidInput, "--schedule is required")
	}

	payload := map[string]interface{}{
		"schedule": schedule,
	}
	return postAndPrint(fmt.Sprintf("/api/v1/jobs/%s/schedule", jobID), payload)
}

func handleJobScheduleRemove(jobID string) error {
	return deleteAndPrint(fmt.Sprintf("/api/v1/jobs/%s/schedule", jobID))
}

func init() {
	DataSourceCmd.AddCommand(DataSourceCreateCmd)
	DataSourceCmd.AddCommand(DataSourceListCmd)
	DataSourceCmd.AddCommand(DataSourceGetCmd)
	DataSourceCmd.AddCommand(DataSourceUpdateCmd)
	DataSourceCmd.AddCommand(DataSourceTestCmd)
	DataSourceCmd.AddCommand(DataSourceApplyCmd)
	DataSourceCmd.AddCommand(DataSourceDeleteCmd)
	DataSourceCmd.AddCommand(DataSourceDescribeCmd)
	DataSourceCmd.AddCommand(DataSourceDiffCmd)

	DataSourceApplyCmd.Flags().StringP("filename", "f", "", "YAML file path")
	DataSourceDiffCmd.Flags().StringP("filename", "f", "", "YAML file path")

	JobCmd.AddCommand(JobCreateCmd)
	JobCmd.AddCommand(JobListCmd)
	JobCmd.AddCommand(JobGetCmd)
	JobCmd.AddCommand(JobRunCmd)
	JobCmd.AddCommand(JobLogsCmd)
	JobCmd.AddCommand(JobCancelCmd)
	JobCmd.AddCommand(JobDeleteCmd2)
	JobCmd.AddCommand(JobDescribeCmd)
	JobCmd.AddCommand(JobDiffCmd)
	JobCmd.AddCommand(JobStatusCmd)
	JobCmd.AddCommand(JobScheduleListCmd)
	JobCmd.AddCommand(JobScheduleSetCmd)
	JobCmd.AddCommand(JobScheduleRemoveCmd)

	JobDiffCmd.Flags().StringP("filename", "f", "", "YAML file path")
	JobScheduleSetCmd.Flags().String("schedule", "", "Schedule expression (for example: 15m or cron expression)")
}
