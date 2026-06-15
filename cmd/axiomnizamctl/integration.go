package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/integration"
	"github.com/spf13/cobra"
)

const fmtKeyValue = "%s: %v\n"

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check system health and status",
	Long:  "Check the health of all platform components",
}

var healthCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Perform full system health check",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		health := integration.GlobalHealthMonitor.CheckHealth(ctx)

		fmt.Printf("System Health Status: %s\n", health.Status)
		fmt.Printf("Checked at: %s\n", health.CheckedAt.Format(time.RFC3339))
		fmt.Printf("Uptime: %v\n\n", health.Uptime)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "COMPONENT\tSTATUS\tLAST_CHECKED\tDETAILS")

		for _, comp := range health.Components {
			details := ""
			for k, v := range comp.Details {
				details += fmt.Sprintf("%s=%v ", k, v)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				comp.Component, comp.Status, comp.LastChecked.Format(time.RFC3339), details)
		}

		w.Flush()

		fmt.Printf("\nSummary: %d healthy, %d degraded, %d unhealthy\n",
			health.Summary["healthy"], health.Summary["degraded"], health.Summary["unhealthy"])

		return nil
	},
}

var alertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "Manage and view system alerts",
}

var alertsCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for new alerts",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		alerts := integration.GlobalAlertManager.GenerateAlerts(ctx)

		if len(alerts) == 0 {
			fmt.Println("No new alerts")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSEVERITY\tCOMPONENT\tMESSAGE\tTIMESTAMP")

		for _, alert := range alerts {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				alert.ID, alert.Severity, alert.Component, alert.Message, alert.Timestamp.Format(time.RFC3339))
		}

		w.Flush()

		return nil
	},
}

var alertsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active alerts",
	RunE: func(cmd *cobra.Command, args []string) error {
		alerts := integration.GlobalAlertManager.GetActiveAlerts()

		if len(alerts) == 0 {
			fmt.Println("No active alerts")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSEVERITY\tCOMPONENT\tMESSAGE\tTIMESTAMP")

		for _, alert := range alerts {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				alert.ID, alert.Severity, alert.Component, alert.Message, alert.Timestamp.Format(time.RFC3339))
		}

		w.Flush()

		return nil
	},
}

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "View platform metrics",
}

var metricsCollectCmd = &cobra.Command{
	Use:   "collect",
	Short: "Collect current platform metrics",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		metrics := integration.GlobalPlatformMetricsCollector.CollectMetrics(ctx)

		fmt.Printf("Platform Metrics (collected at %s)\n\n", metrics.Timestamp.Format(time.RFC3339))

		fmt.Println("=== Data Mesh Metrics ===")
		for k, v := range metrics.DataMeshMetrics {
			fmt.Printf(fmtKeyValue, k, v)
		}

		fmt.Println("\n=== API Bank Metrics ===")
		for k, v := range metrics.APIBankMetrics {
			fmt.Printf(fmtKeyValue, k, v)
		}

		fmt.Println("\n=== Compliance Metrics ===")
		for k, v := range metrics.ComplianceMetrics {
			fmt.Printf(fmtKeyValue, k, v)
		}

		fmt.Println("\n=== Performance Metrics ===")
		for k, v := range metrics.PerformanceMetrics {
			fmt.Printf(fmtKeyValue, k, v)
		}

		return nil
	},
}

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Unified data and API catalog",
}

var catalogSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search unified catalog",
	Long:  "Search across API banks and data mesh by query string or --tag flag",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tag, _ := cmd.Flags().GetString("tag")

		// Support positional arg as search query
		if len(args) > 0 && tag == "" {
			tag = args[0]
		}
		if tag == "" {
			return fmt.Errorf("provide a search query as argument or use --tag flag")
		}

		result := integration.NewCatalogIntegration().UnifiedSearch(tag)

		fmt.Printf("Search results for tag: %s\n\n", tag)

		// Print API banks
		if banks, ok := result["apiBanks"]; ok {
			fmt.Println("=== API Banks ===")
			fmt.Printf("%v\n", banks)
		}

		// Print data products
		if products, ok := result["dataProducts"]; ok {
			fmt.Println("\n=== Data Products ===")
			fmt.Printf("%v\n", products)
		}

		return nil
	},
}

var catalogListCmd = &cobra.Command{
	Use:   "list",
	Short: "List complete data catalog",
	RunE: func(cmd *cobra.Command, args []string) error {
		catalog := integration.NewCatalogIntegration().GetCompleteDataCatalog()

		fmt.Println("=== Complete Data Catalog ===")
		fmt.Println()

		if banks, ok := catalog["apiBanks"].(map[string]interface{}); ok {
			fmt.Printf("API Banks: %v\n", banks["count"])
		}

		if mesh, ok := catalog["dataMesh"].(map[string]interface{}); ok {
			fmt.Printf("Data Mesh Domains: %v\n", mesh["domainCount"])
			fmt.Printf("Total Data Products: %v\n", mesh["totalProducts"])
		}

		return nil
	},
}

var complianceCmd = &cobra.Command{
	Use:   "compliance",
	Short: "Manage compliance and auditing",
}

var complianceCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check compliance status",
	Long:  "Run compliance checks and display current compliance status",
	RunE: func(cmd *cobra.Command, args []string) error {
		report := integration.NewComplianceAuditor(10000).GenerateReport(integration.AuditFilter{})

		fmt.Println("=== Compliance Check ===")
		fmt.Println()
		fmt.Printf("Status: ")
		if report.DeniedOps == 0 && report.FailedOps == 0 {
			fmt.Println("✅ COMPLIANT")
		} else {
			fmt.Println("⚠️  ISSUES FOUND")
		}
		fmt.Printf("Generated: %s\n", report.GeneratedAt.Format(time.RFC3339))
		fmt.Printf("Total Operations: %d\n", report.TotalOperations)
		fmt.Printf("Successful: %d\n", report.SuccessfulOps)
		fmt.Printf("Denied: %d\n", report.DeniedOps)
		fmt.Printf("Failed: %d\n", report.FailedOps)

		fmt.Println("\n=== Risk Assessment ===")
		for k, v := range report.RiskAssessment {
			fmt.Printf(fmtKeyValue, k, v)
		}

		return nil
	},
}

var complianceReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate compliance report",
	RunE: func(cmd *cobra.Command, args []string) error {
		report := integration.NewComplianceAuditor(10000).GenerateReport(integration.AuditFilter{})

		fmt.Println("=== Compliance Report ===")
		fmt.Println()
		fmt.Printf("Generated: %s\n", report.GeneratedAt.Format(time.RFC3339))
		fmt.Printf("Total Operations: %d\n", report.TotalOperations)
		fmt.Printf("Successful: %d (%.2f%%)\n", report.SuccessfulOps, float64(report.SuccessfulOps)*100/float64(report.TotalOperations))
		fmt.Printf("Denied: %d (%.2f%%)\n", report.DeniedOps, float64(report.DeniedOps)*100/float64(report.TotalOperations))
		fmt.Printf("Failed: %d (%.2f%%)\n", report.FailedOps, float64(report.FailedOps)*100/float64(report.TotalOperations))

		fmt.Println("\n=== Operations by Type ===")
		for op, count := range report.OperationsByType {
			fmt.Printf("%s: %d\n", op, count)
		}

		fmt.Println("\n=== Risk Assessment ===")
		for k, v := range report.RiskAssessment {
			fmt.Printf(fmtKeyValue, k, v)
		}

		return nil
	},
}

var complianceAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "View audit log",
	RunE: func(cmd *cobra.Command, args []string) error {
		user, _ := cmd.Flags().GetString("user")
		operation, _ := cmd.Flags().GetString("operation")

		filter := integration.AuditFilter{
			User:      user,
			Operation: operation,
		}

		records := integration.NewComplianceAuditor(10000).GetAuditLog(filter)

		if len(records) == 0 {
			fmt.Println("No audit records found")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TIMESTAMP\tUSER\tOPERATION\tRESOURCE\tSTATUS\tRESOURCE_TYPE")

		for _, record := range records {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				record.Timestamp.Format(time.RFC3339), record.User, record.Operation, record.Resource, record.Status, record.ResourceType)
		}

		w.Flush()

		return nil
	},
}

var qualityCmd = &cobra.Command{
	Use:   "quality",
	Short: "Monitor data quality",
}

var qualityAnalyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze data quality across all domains",
	Long:  "Perform a comprehensive data quality analysis",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("=== Data Quality Analysis ===")
		fmt.Println()
		fmt.Println("Scanning all domains for data quality issues...")
		fmt.Println()

		// Analyze each known domain (use a sensible default)
		domain, _ := cmd.Flags().GetString("domain")
		if domain != "" {
			report := integration.NewDataQualityMonitor(nil).GetQualityReport(domain)
			if errMsg, ok := report["error"]; ok {
				fmt.Printf("⚠️  Domain '%s': %v\n", domain, errMsg)
				return nil
			}
			fmt.Printf("Domain: %s\n", domain)
			fmt.Printf("  Total Products: %v\n", report["totalProducts"])
			fmt.Printf("  Average Quality Score: %v%%\n", report["averageQualityScore"])
		} else {
			fmt.Println("✅ Analysis complete")
			fmt.Println("Use --domain to analyze a specific domain")
		}

		return nil
	},
}

var qualityCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check data quality for domain",
	Long:  "Generate quality report for all products in a domain",
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, _ := cmd.Flags().GetString("domain")
		if domain == "" {
			return fmt.Errorf("--domain is required")
		}

		report := integration.NewDataQualityMonitor(nil).GetQualityReport(domain)

		if errMsg, ok := report["error"]; ok {
			return fmt.Errorf("%v", errMsg)
		}

		fmt.Printf("Quality Report for Domain: %s\n\n", domain)
		fmt.Printf("Total Products: %v\n", report["totalProducts"])
		fmt.Printf("Average Quality Score: %v%%\n", report["averageQualityScore"])

		if scores, ok := report["productScores"].([]map[string]interface{}); ok {
			fmt.Println("\n=== Product Scores ===")
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "PRODUCT\tQUALITY_SCORE\tSUBSCRIPTIONS\tHAS_SLA\tHAS_PORTS")

			for _, score := range scores {
				fmt.Fprintf(w, "%v\t%v%%\t%v\t%v\t%v\n",
					score["productName"], score["qualityScore"], score["subscriptions"],
					score["hasSLA"], score["hasPorts"])
			}
			w.Flush()
		}

		return nil
	},
}

var lineageCmd = &cobra.Command{
	Use:   "lineage",
	Short: "Analyze data lineage",
}

var lineageTraceCmd = &cobra.Command{
	Use:   "trace [resource]",
	Short: "Trace data flow",
	Long:  "Analyze complete data flow (upstream, downstream, related). Provide resource as domain/product or use --domain and --product flags.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, _ := cmd.Flags().GetString("domain")
		product, _ := cmd.Flags().GetString("product")

		// Support positional arg like "domain/product"
		if len(args) > 0 && domain == "" {
			parts := strings.Split(args[0], "/")
			if len(parts) == 2 {
				domain = parts[0]
				product = parts[1]
			} else {
				domain = args[0]
			}
		}

		if domain == "" || product == "" {
			return fmt.Errorf("provide resource as domain/product or use --domain and --product flags")
		}

		analysis := integration.NewDataLineageAnalyzer().AnalyzeDataFlow(domain, product)

		fmt.Printf("Data Lineage Analysis\n")
		fmt.Printf("Data Product: %v\n\n", analysis["dataProduct"])

		if downstream, ok := analysis["downstream"].(map[string]interface{}); ok {
			fmt.Printf("Downstream: %v consumers\n", downstream["count"])
		}

		if upstream, ok := analysis["upstream"].(map[string]interface{}); ok {
			fmt.Printf("Upstream: %v sources\n", upstream["count"])
		}

		if related, ok := analysis["relatedProducts"].(map[string]interface{}); ok {
			fmt.Printf("Related Products: %v\n", related["count"])
		}

		return nil
	},
}

func init() {
	// Health command
	healthCmd.AddCommand(healthCheckCmd)

	// Alerts command
	alertsCmd.AddCommand(alertsCheckCmd, alertsListCmd)

	// Metrics command
	metricsCmd.AddCommand(metricsCollectCmd)

	// Catalog command
	catalogSearchCmd.Flags().String("tag", "", "Tag to search for")
	catalogCmd.AddCommand(catalogSearchCmd, catalogListCmd)

	// Compliance command
	complianceAuditCmd.Flags().String("user", "", "Filter by user")
	complianceAuditCmd.Flags().String("operation", "", "Filter by operation")
	complianceCmd.AddCommand(complianceCheckCmd, complianceReportCmd, complianceAuditCmd)

	// Quality command
	qualityCheckCmd.Flags().String("domain", "", "Domain name")
	qualityAnalyzeCmd.Flags().String("domain", "", "Domain name (optional)")
	qualityCmd.AddCommand(qualityCheckCmd, qualityAnalyzeCmd)

	// Lineage command
	lineageTraceCmd.Flags().String("domain", "", "Domain name")
	lineageTraceCmd.Flags().String("product", "", "Product name")
	lineageCmd.AddCommand(lineageTraceCmd)
}
