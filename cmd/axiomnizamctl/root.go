package main

import (
	"fmt"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/client"
	"github.com/spf13/cobra"
)

var (
	configManager *client.ConfigManager
	apiClient     *client.Client
	outputFormat  string
	namespace     string
	verbose       bool
	kubeconfig    string
	contextName   string
	dry           bool
)

var RootCmd = &cobra.Command{
	Use:   "axiomnizamctl",
	Short: "AxiomNizam CLI - Control plane for data APIs",
	Long: `AxiomNizam CLI - Kubernetes-style control plane for APIs, policies, workflows, and data sources.

AxiomNizam provides a kubectl-like interface for managing cloud-native data infrastructure.
It supports declarative configuration management, policy enforcement, and automated workflows.

Website: https://axiom-nizam.io
Docs: https://docs.axiom-nizam.io`,
	Version:       "1.0.0",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return initializeCLIContext(cmd)
	},
}

// skipConfigInit returns true for commands that don't need config initialization.
func skipConfigInit(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	leaf := strings.TrimSpace(strings.ToLower(cmd.Name()))
	if leaf == "help" || leaf == "--help" || leaf == "-h" {
		return true
	}

	topLevelNoConfig := map[string]bool{
		"login":      true,
		"logout":     true,
		"version":    true,
		"completion": true,
		"config":     true,
	}
	if topLevelNoConfig[leaf] {
		return true
	}

	return commandPathContains(cmd, "wait") ||
		commandPathContains(cmd, "scan") ||
		commandPathContains(cmd, "discover")
}

func commandPathContains(cmd *cobra.Command, name string) bool {
	needle := strings.TrimSpace(strings.ToLower(name))
	if needle == "" {
		return false
	}

	for current := cmd; current != nil; current = current.Parent() {
		if strings.EqualFold(current.Name(), needle) {
			return true
		}
	}

	return false
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("AxiomNizam CLI v1.0.0")
	},
}

func init() {
	bindPersistentFlags(RootCmd)
	registerRootCommands(RootCmd)
}
