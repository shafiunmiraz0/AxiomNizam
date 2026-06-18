package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// ====================================
// TRACING COMMANDS
// ====================================

var TracingCmd = &cobra.Command{
	Use:   "tracing",
	Short: "Manage distributed tracing",
}

var tracingSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search traces",
	RunE: func(cmd *cobra.Command, args []string) error {
		service, _ := cmd.Flags().GetString("service")
		params := ""
		if service != "" {
			params = "?service=" + service
		}
		return getAndPrint("/api/v1/tracing/traces/search" + params)
	},
}

var tracingGetCmd = &cobra.Command{
	Use:   "get [traceId]",
	Short: "Get trace by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/tracing/traces/" + args[0])
	},
}

var tracingServiceMapCmd = &cobra.Command{
	Use:   "service-map",
	Short: "Show service dependency map",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/tracing/service-map")
	},
}

var tracingServicesCmd = &cobra.Command{
	Use:   "services",
	Short: "List traced services",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/tracing/services")
	},
}

// ====================================
// ENCRYPTION COMMANDS
// ====================================

var EncryptionCmd = &cobra.Command{
	Use:   "encryption",
	Short: "Manage encryption keys and policies",
}

var encryptionKeyListCmd = &cobra.Command{
	Use:   "keys",
	Short: "List encryption keys",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/encryption/keys")
	},
}

var encryptionKeyCreateCmd = &cobra.Command{
	Use:   "create-key",
	Short: "Create encryption key",
	RunE: func(cmd *cobra.Command, args []string) error {
		payload := map[string]interface{}{
			"tenantId":  promptInput("Tenant ID"),
			"name":      promptInput("Key name"),
			"algorithm": promptInput("Algorithm (AES-256-GCM)"),
			"keyLength": 256,
		}
		return postAndPrint("/api/v1/encryption/keys", payload)
	},
}

var encryptionKeyRotateCmd = &cobra.Command{
	Use:   "rotate-key [id]",
	Short: "Rotate encryption key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return postAndPrint("/api/v1/encryption/keys/"+args[0]+"/rotate", nil)
	},
}

var encryptionPoliciesCmd = &cobra.Command{
	Use:   "policies",
	Short: "List encryption policies",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/encryption/policies")
	},
}

// ====================================
// CONDUCTOR COMMANDS
// ====================================

var ConductorCmd = &cobra.Command{
	Use:   "conductor",
	Short: "Manage message queue producers and consumers",
}

var conductorProducerListCmd = &cobra.Command{
	Use:   "producers",
	Short: "List producers",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/conductor/producers")
	},
}

var conductorConsumerListCmd = &cobra.Command{
	Use:   "consumers",
	Short: "List consumers",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/conductor/consumers")
	},
}

var conductorStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show conductor statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/conductor/stats")
	},
}

var conductorDLQCmd = &cobra.Command{
	Use:   "dlq",
	Short: "List dead-letter queue entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/conductor/dlq")
	},
}

// ====================================
// ETL COMMANDS
// ====================================

var ETLCmd = &cobra.Command{
	Use:   "etl",
	Short: "Manage ETL pipelines",
}

var etlListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ETL pipelines",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/etl/pipelines")
	},
}

var etlGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get ETL pipeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/etl/pipelines/" + args[0])
	},
}

var etlRunCmd = &cobra.Command{
	Use:   "run [id]",
	Short: "Run ETL pipeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return postAndPrint("/api/v1/etl/pipelines/"+args[0]+"/run", nil)
	},
}

var etlRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "List ETL runs",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/etl/runs")
	},
}

var etlConnectorsCmd = &cobra.Command{
	Use:   "connectors",
	Short: "List ETL connectors",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/etl/connectors")
	},
}

// ====================================
// CDC COMMANDS
// ====================================

var CDCCmd = &cobra.Command{
	Use:   "cdc",
	Short: "Manage CDC pipelines",
}

var cdcListCmd = &cobra.Command{
	Use:   "list",
	Short: "List CDC pipelines",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/cdc/pipelines")
	},
}

var cdcGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get CDC pipeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/cdc/pipelines/" + args[0])
	},
}

var cdcStartCmd = &cobra.Command{
	Use:   "start [id]",
	Short: "Start CDC pipeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return postAndPrint("/api/v1/cdc/pipelines/"+args[0]+"/start", nil)
	},
}

var cdcStopCmd = &cobra.Command{
	Use:   "stop [id]",
	Short: "Stop CDC pipeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return postAndPrint("/api/v1/cdc/pipelines/"+args[0]+"/stop", nil)
	},
}

// ====================================
// STORAGE COMMANDS
// ====================================

var StorageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Manage object storage (buckets, objects)",
}

var storageBucketListCmd = &cobra.Command{
	Use:   "buckets",
	Short: "List buckets",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/storage/buckets")
	},
}

var storageBucketCreateCmd = &cobra.Command{
	Use:   "create-bucket [name]",
	Short: "Create bucket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return postAndPrint("/api/v1/storage/buckets", map[string]interface{}{"name": args[0]})
	},
}

var storageObjectListCmd = &cobra.Command{
	Use:   "objects [bucket]",
	Short: "List objects in bucket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/storage/buckets/" + args[0] + "/objects")
	},
}

// ====================================
// NETINTEL COMMANDS
// ====================================

var NetIntelCmd = &cobra.Command{
	Use:   "netintel",
	Short: "Network intelligence and observability",
}

var netintelSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show NetIntel summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/netintel/summary")
	},
}

var netintelTopologyCmd = &cobra.Command{
	Use:   "topology",
	Short: "Show network topology",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/netintel/topology")
	},
}

var netintelAnomaliesCmd = &cobra.Command{
	Use:   "anomalies",
	Short: "List anomalies",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/netintel/anomalies")
	},
}

var netintelAlertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "List alerts",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/netintel/alerts")
	},
}

// ====================================
// NETINTEL WIFI COMMANDS
// ====================================

var netintelWiFiCmd = &cobra.Command{
	Use:   "wifi",
	Short: "WiFi network scanning and monitoring",
}

var netintelWiFiScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan nearby WiFi networks",
	RunE: func(cmd *cobra.Command, args []string) error {
		return postAndPrint("/api/v1/netintel/wifi/scan", nil)
	},
}

var netintelWiFiNetworksCmd = &cobra.Command{
	Use:   "networks",
	Short: "List cached WiFi scan results",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/netintel/wifi/networks")
	},
}

var netintelWiFiHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Show WiFi scan history",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/netintel/wifi/history")
	},
}

var netintelWiFiStreamCmd = &cobra.Command{
	Use:   "stream",
	Short: "Live stream WiFi scan results via WebSocket",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("WebSocket streaming requires a WebSocket client.")
		fmt.Println("Connect to: ws://<host>:<port>/api/v1/netintel/wifi/stream")
		fmt.Println("Use a tool like websocat: websocat ws://localhost:8000/api/v1/netintel/wifi/stream")
		return nil
	},
}

// ====================================
// GIS COMMANDS
// ====================================

var GISCmd = &cobra.Command{
	Use:   "gis",
	Short: "Manage GIS layers, regions, markers, datasets",
}

var gisLayersCmd = &cobra.Command{
	Use:   "layers",
	Short: "List GIS layers",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/gis/layers")
	},
}

var gisRegionsCmd = &cobra.Command{
	Use:   "regions",
	Short: "List GIS regions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/gis/regions")
	},
}

var gisDatasetsCmd = &cobra.Command{
	Use:   "datasets",
	Short: "List GIS datasets",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/gis/datasets")
	},
}

var gisSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show GIS summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/gis/summary")
	},
}

// ====================================
// ANALYTICS COMMANDS
// ====================================

var AnalyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Manage analytics dashboards",
}

var analyticsDashboardsCmd = &cobra.Command{
	Use:   "dashboards",
	Short: "List analytics dashboards",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/analytics/dashboards")
	},
}

var analyticsDashboardGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get dashboard",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/analytics/dashboards/" + args[0])
	},
}

// ====================================
// DEPLOYMENT COMMANDS
// ====================================

var DeploymentCmd = &cobra.Command{
	Use:   "deployment",
	Short: "Manage canary/blue-green deployments",
}

var deploymentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create deployment",
	RunE: func(cmd *cobra.Command, args []string) error {
		payload := map[string]interface{}{
			"jobId":    promptInput("Job ID"),
			"version":  promptInput("Version"),
			"strategy": promptInput("Strategy (canary|blue-green)"),
			"canary":   1,
		}
		return postAndPrint("/api/v1/deployments", payload)
	},
}

var deploymentGetCmd = &cobra.Command{
	Use:   "get [jobId]",
	Short: "Get deployment status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/deployments/" + args[0])
	},
}

var deploymentPromoteCmd = &cobra.Command{
	Use:   "promote [jobId]",
	Short: "Promote canary deployment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return postAndPrint("/api/v1/deployments/"+args[0]+"/promote", nil)
	},
}

// ====================================
// HEARTBEAT COMMANDS
// ====================================

var HeartbeatCmd = &cobra.Command{
	Use:   "heartbeat",
	Short: "Manage heartbeat tracking",
}

var heartbeatBeatCmd = &cobra.Command{
	Use:   "beat [id]",
	Short: "Send heartbeat",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ttl, _ := cmd.Flags().GetInt("ttl")
		if ttl == 0 {
			ttl = 30
		}
		return postAndPrint("/api/v1/heartbeat/beat", map[string]interface{}{"id": args[0], "ttl": ttl})
	},
}

var heartbeatAliveCmd = &cobra.Command{
	Use:   "alive [id]",
	Short: "Check if entity is alive",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/heartbeat/alive/" + args[0])
	},
}

var heartbeatExpiredCmd = &cobra.Command{
	Use:   "expired",
	Short: "List expired entities",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/heartbeat/expired")
	},
}

// ====================================
// SERVICE REGISTRY COMMANDS
// ====================================

var ServiceRegistryCmd = &cobra.Command{
	Use:   "service-registry",
	Short: "Manage service registry",
}

var svcRegListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered services",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		params := ""
		if name != "" {
			params = "?name=" + name
		}
		return getAndPrint("/api/v1/service-registry/services" + params)
	},
}

var svcRegGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get service details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/service-registry/services/" + args[0])
	},
}

// ====================================
// AUTOPILOT COMMANDS
// ====================================

var AutopilotCmd = &cobra.Command{
	Use:   "autopilot",
	Short: "Cluster autopilot evaluation",
}

var autopilotEvaluateCmd = &cobra.Command{
	Use:   "evaluate",
	Short: "Evaluate cluster health and get decisions",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("ℹ️  Autopilot evaluate requires peer data. Use the API directly:")
		fmt.Println("   POST /api/v1/autopilot/evaluate")
		return nil
	},
}

// ====================================
// NOTIFICATION COMMANDS
// ====================================

var NotificationCmd = &cobra.Command{
	Use:   "notify",
	Short: "Send notifications",
}

var notifySendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send notification",
	RunE: func(cmd *cobra.Command, args []string) error {
		message := strings.Join(args, " ")
		if message == "" {
			message = promptInput("Message")
		}
		return postAndPrint("/api/v1/notifications/send", map[string]interface{}{
			"message": message,
			"type":    "info",
		})
	},
}

var notifyStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get notification service status",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/api/v1/notifications/status")
	},
}

// ====================================
// RECONCILER COMMANDS
// ====================================

var ReconcilerCmd = &cobra.Command{
	Use:   "reconciler",
	Short: "View reconciler health and status",
}

var reconcilerHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "Show reconciler health summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrint("/health/reconcilers")
	},
}

// ====================================
// REGISTER ALL EXTENDED COMMANDS
// ====================================

func registerExtendedCommands(root *cobra.Command) {
	// Tracing
	TracingCmd.AddCommand(tracingSearchCmd, tracingGetCmd, tracingServiceMapCmd, tracingServicesCmd)
	tracingSearchCmd.Flags().String("service", "", "Filter by service name")
	root.AddCommand(TracingCmd)

	// Encryption
	EncryptionCmd.AddCommand(encryptionKeyListCmd, encryptionKeyCreateCmd, encryptionKeyRotateCmd, encryptionPoliciesCmd)
	root.AddCommand(EncryptionCmd)

	// Conductor
	ConductorCmd.AddCommand(conductorProducerListCmd, conductorConsumerListCmd, conductorStatsCmd, conductorDLQCmd)
	root.AddCommand(ConductorCmd)

	// ETL
	ETLCmd.AddCommand(etlListCmd, etlGetCmd, etlRunCmd, etlRunsCmd, etlConnectorsCmd)
	root.AddCommand(ETLCmd)

	// CDC
	CDCCmd.AddCommand(cdcListCmd, cdcGetCmd, cdcStartCmd, cdcStopCmd)
	root.AddCommand(CDCCmd)

	// Storage
	StorageCmd.AddCommand(storageBucketListCmd, storageBucketCreateCmd, storageObjectListCmd)
	root.AddCommand(StorageCmd)

	// NetIntel
	NetIntelCmd.AddCommand(netintelSummaryCmd, netintelTopologyCmd, netintelAnomaliesCmd, netintelAlertsCmd, netintelWiFiCmd)
	netintelWiFiCmd.AddCommand(netintelWiFiScanCmd, netintelWiFiNetworksCmd, netintelWiFiHistoryCmd, netintelWiFiStreamCmd)
	root.AddCommand(NetIntelCmd)

	// GIS
	GISCmd.AddCommand(gisLayersCmd, gisRegionsCmd, gisDatasetsCmd, gisSummaryCmd)
	root.AddCommand(GISCmd)

	// Analytics
	AnalyticsCmd.AddCommand(analyticsDashboardsCmd, analyticsDashboardGetCmd)
	root.AddCommand(AnalyticsCmd)

	// Deployment
	DeploymentCmd.AddCommand(deploymentCreateCmd, deploymentGetCmd, deploymentPromoteCmd)
	root.AddCommand(DeploymentCmd)

	// Heartbeat
	HeartbeatCmd.AddCommand(heartbeatBeatCmd, heartbeatAliveCmd, heartbeatExpiredCmd)
	heartbeatBeatCmd.Flags().Int("ttl", 30, "TTL in seconds")
	root.AddCommand(HeartbeatCmd)

	// Service Registry
	ServiceRegistryCmd.AddCommand(svcRegListCmd, svcRegGetCmd)
	svcRegListCmd.Flags().String("name", "", "Filter by service name")
	root.AddCommand(ServiceRegistryCmd)

	// Autopilot
	AutopilotCmd.AddCommand(autopilotEvaluateCmd)
	root.AddCommand(AutopilotCmd)

	// Notification
	NotificationCmd.AddCommand(notifySendCmd, notifyStatusCmd)
	root.AddCommand(NotificationCmd)

	// Reconciler
	ReconcilerCmd.AddCommand(reconcilerHealthCmd)
	root.AddCommand(ReconcilerCmd)
}
