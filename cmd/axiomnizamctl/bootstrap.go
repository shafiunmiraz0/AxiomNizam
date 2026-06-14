package main

import (
	"fmt"
	"os"

	"axiomnizam.bitbd.net/axiomnizam/internal/client"
	"github.com/spf13/cobra"
)

func initializeCLIContext(cmd *cobra.Command) error {
	if skipConfigInit(cmd) {
		return nil
	}

	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	if contextName != "" {
		if err := configManager.SetCurrentContext(contextName); err != nil {
			return NewCommandError(ErrConfigError, "Failed to switch context", err.Error())
		}
	}

	if err := ensureAPIClientFromConfig(); err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
		}
	}

	return nil
}

func ensureConfigManagerLoaded() error {
	if configManager != nil {
		return nil
	}

	configPath := kubeconfig
	if configPath == "" {
		configPath = client.DefaultConfigPath()
	}

	cm, err := client.NewConfigManagerWithPath(configPath)
	if err != nil || cm == nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "⚠️  Config initialization fallback: %v\n", err)
		}
		cm = client.NewConfigManager()
	}

	configManager = cm
	if err := configManager.Load(); err != nil && verbose {
		fmt.Fprintf(os.Stderr, "⚠️  Failed to load config: %v\n", err)
	}

	return nil
}

func ensureAPIClientFromConfig() error {
	if configManager == nil {
		if err := ensureConfigManagerLoaded(); err != nil {
			return err
		}
	}

	cfg := configManager.GetCurrentContext()
	if cfg == nil || cfg.Cluster == nil || cfg.Cluster.Server == "" {
		return NewCommandError(ErrConfigError, "No context configured", "Run 'axiomnizamctl config use-context <name>'")
	}

	if apiClient == nil {
		apiClient = client.NewClient(cfg.Cluster.Server)
	} else {
		apiClient.SetBaseURL(cfg.Cluster.Server)
	}

	token := configManager.GetToken()
	if token != "" {
		apiClient.SetToken(token)
	}

	return nil
}
