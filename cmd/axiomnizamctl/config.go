package main

import (
	"fmt"
	"os"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/client"
	"axiomnizam.bitbd.net/axiomnizam/internal/output"
	"github.com/spf13/cobra"
)

var (
	configShowToken bool
	mergeConfigs    bool
	flattenOutput   bool
)

var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage kubeconfig-style configuration",
	Long: `Manage AxiomNizam CLI configuration files and contexts.

The configuration follows Kubernetes kubeconfig format:
- Multiple contexts for different servers
- Each context has cluster, user, and namespace info
- Authentication tokens stored securely
- Supports context switching

Similar to kubectl, AxiomNizam uses ~/.axiomnizam/config by default.`,
}

var ConfigViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Display merged config",
	Long:  "Display merged kubeconfig. Use --flatten to show flattened output.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleConfigView()
	},
}

var ConfigCurrentContextCmd = &cobra.Command{
	Use:   "current-context",
	Short: "Display the current context",
	Long:  "Display the current context name",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleConfigCurrentContext()
	},
}

var ConfigUseContextCmd = &cobra.Command{
	Use:   "use-context [context-name]",
	Short: "Set the current context",
	Long:  "Set the current context to use for operations",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleConfigUseContext(args[0])
	},
}

var ConfigGetClustersCmd = &cobra.Command{
	Use:   "get-clusters",
	Short: "List all available clusters",
	Long:  "List all configured clusters with their server URLs",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleConfigGetClusters()
	},
}

var ConfigSetContextCmd = &cobra.Command{
	Use:   "set-context [context-name] --cluster=[cluster] --user=[user]",
	Short: "Set a context entry in kubeconfig",
	Long: `Set a context entry in the kubeconfig.

Examples:
  # Create a new context
  axiomnizamctl config set-context mycontext --cluster=mycluster --user=myuser

  # Update an existing context
  axiomnizamctl config set-context dev --cluster=dev-cluster --namespace=dev`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleConfigSetContext(args[0], cmd)
	},
}

var ConfigSetClusterCmd = &cobra.Command{
	Use:   "set-cluster [cluster-name] --server=[server-url]",
	Short: "Set a cluster entry in kubeconfig",
	Long:  "Set a cluster entry in the kubeconfig",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleConfigSetCluster(args[0], cmd)
	},
}

var ConfigDeleteContextCmd = &cobra.Command{
	Use:   "delete-context [context-name]",
	Short: "Delete a context from kubeconfig",
	Long:  "Remove a context and all its references",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleConfigDeleteContext(args[0])
	},
}

var ConfigRenameContextCmd = &cobra.Command{
	Use:   "rename-context [old-name] [new-name]",
	Short: "Rename a context",
	Long:  "Rename a context in the kubeconfig",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleConfigRenameContext(args[0], args[1])
	},
}

func handleConfigView() error {
	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	config := configManager.GetCurrentContext()
	if config == nil {
		return NewCommandError(ErrConfigError, "No context configured")
	}

	formatter := output.NewFormatter(outputFormat, os.Stdout)
	if configShowToken {
		fmt.Printf("Current token: %s\n", configManager.GetToken())
	}
	formatter.Print(config)
	return nil
}

func handleConfigCurrentContext() error {
	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	context := configManager.GetCurrentContext()
	if context == nil {
		return NewCommandError(ErrConfigError, "No context configured")
	}

	fmt.Println(context.Name)
	return nil
}

func handleConfigUseContext(contextName string) error {
	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	if err := configManager.SetCurrentContext(contextName); err != nil {
		return NewCommandError(ErrConfigError, fmt.Sprintf("Failed to set context: %s", contextName), err.Error())
	}

	printSuccessMessage(fmt.Sprintf("Switched to context '%s'", contextName))
	return nil
}

func handleConfigGetClusters() error {
	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	contexts := configManager.ListContexts()

	if len(contexts) == 0 {
		printInfoMessage("No contexts configured")
		return nil
	}

	fmt.Println("\nConfigured Clusters:")
	fmt.Println(strings.Repeat("─", 50))

	seenClusters := make(map[string]bool)

	for _, ctx := range contexts {
		if ctx.Cluster != nil && !seenClusters[ctx.Cluster.Server] {
			seenClusters[ctx.Cluster.Server] = true
			fmt.Printf("  • %s\n", ctx.Cluster.Server)
		}
	}

	return nil
}

func handleConfigSetContext(contextName string, cmd *cobra.Command) error {
	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	clusterName, _ := cmd.Flags().GetString("cluster")
	userName, _ := cmd.Flags().GetString("user")
	ns, _ := cmd.Flags().GetString("namespace")

	if ns == "" {
		ns = "default"
	}

	// Get existing cluster or use specified
	var clusterInfo *client.ClusterInfo
	if clusterName != "" {
		// Find cluster by name
		contexts := configManager.ListContexts()
		for _, ctx := range contexts {
			if ctx.Cluster != nil && ctx.Cluster.Server == clusterName {
				clusterInfo = ctx.Cluster
				break
			}
		}
		if clusterInfo == nil {
			clusterInfo = &client.ClusterInfo{Server: clusterName}
		}
	} else {
		ctx := configManager.GetCurrentContext()
		if ctx != nil && ctx.Cluster != nil {
			clusterInfo = ctx.Cluster
		}
	}

	if clusterInfo == nil {
		clusterInfo = &client.ClusterInfo{Server: "http://localhost:8000"}
	}

	context := &client.Context{
		Name:      contextName,
		Cluster:   clusterInfo,
		User:      userName,
		Namespace: ns,
	}

	if err := configManager.AddOrUpdateContext(context); err != nil {
		return fmt.Errorf("failed to set context: %w", err)
	}

	fmt.Printf("✅ Context '%s' set\n", contextName)
	return nil
}

func handleConfigSetCluster(clusterName string, cmd *cobra.Command) error {
	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	server, _ := cmd.Flags().GetString("server")
	if server == "" {
		return fmt.Errorf("--server flag is required")
	}

	// Update all contexts using this cluster
	contexts := configManager.ListContexts()
	for _, ctx := range contexts {
		if ctx.Cluster != nil && ctx.Cluster.Server == clusterName {
			ctx.Cluster.Server = server
		}
	}

	fmt.Printf("✅ Cluster '%s' set to %s\n", clusterName, server)
	return nil
}

func handleConfigDeleteContext(contextName string) error {
	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	if err := configManager.DeleteContext(contextName); err != nil {
		return fmt.Errorf("failed to delete context: %w", err)
	}

	fmt.Printf("✅ Context '%s' deleted\n", contextName)
	return nil
}

func handleConfigRenameContext(oldName, newName string) error {
	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	contexts := configManager.ListContexts()
	var found *client.Context
	for _, ctx := range contexts {
		if ctx.Name == oldName {
			found = &ctx
			break
		}
	}

	if found == nil {
		return fmt.Errorf("context '%s' not found", oldName)
	}

	found.Name = newName
	if err := configManager.AddOrUpdateContext(found); err != nil {
		return fmt.Errorf("failed to rename context: %w", err)
	}

	if configManager.GetCurrentContext().Name == oldName {
		configManager.SetCurrentContext(newName)
	}

	fmt.Printf("✅ Context '%s' renamed to '%s'\n", oldName, newName)
	return nil
}

func init() {
	// View flags
	ConfigViewCmd.Flags().BoolVar(&configShowToken, "show-token", false, "Display token in output")
	ConfigViewCmd.Flags().BoolVar(&flattenOutput, "flatten", false, "Show flattened output")
	ConfigViewCmd.Flags().BoolVar(&mergeConfigs, "merge", false, "Merge multiple kubeconfigs")

	// Set-context flags
	ConfigSetContextCmd.Flags().String("cluster", "", "Cluster name")
	ConfigSetContextCmd.Flags().String("user", "", "User name")
	ConfigSetContextCmd.Flags().String("namespace", "default", "Namespace")

	// Set-cluster flags
	ConfigSetClusterCmd.Flags().String("server", "", "Server URL (required)")
	ConfigSetClusterCmd.Flags().Bool("insecure-skip-tls-verify", false, "Skip TLS verification")
	ConfigSetClusterCmd.Flags().String("certificate-authority", "", "Path to CA certificate")

	// Register subcommands
	ConfigCmd.AddCommand(ConfigViewCmd)
	ConfigCmd.AddCommand(ConfigCurrentContextCmd)
	ConfigCmd.AddCommand(ConfigUseContextCmd)
	ConfigCmd.AddCommand(ConfigGetClustersCmd)
	ConfigCmd.AddCommand(ConfigSetContextCmd)
	ConfigCmd.AddCommand(ConfigSetClusterCmd)
	ConfigCmd.AddCommand(ConfigDeleteContextCmd)
	ConfigCmd.AddCommand(ConfigRenameContextCmd)
}
