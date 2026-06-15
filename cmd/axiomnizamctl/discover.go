package main

import (
	"fmt"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apiscanner"
	"github.com/spf13/cobra"
)

var (
	discoverHeaders            []string
	discoverTimeout            time.Duration
	discoverFormat             string
	discoverInsecureSkipVerify bool
	discoverMaxPaths           int
	discoverIncludeScanIDs     []string
	discoverExcludeScanIDs     []string
	discoverListScanIDs        bool
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover API exposed paths and service fingerprints",
	Long: `Discovery mode inspired by DAST reconnaissance flows.

Current support:
- discover api URL
- discover domain DOMAIN

Utility:
- discover --list-scan-ids`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if discoverListScanIDs {
			printSupportedDiscoveryScanIDs()
			return nil
		}
		return cmd.Help()
	},
}

func printSupportedDiscoveryScanIDs() {
	supported := apiscanner.GetSupportedDiscoveryScanIDs()

	fmt.Println("SUPPORTED DISCOVERY SCAN IDS")
	fmt.Println()
	fmt.Println("API")
	for _, id := range supported.API {
		fmt.Printf("- %s\n", id)
	}

	fmt.Println()
	fmt.Println("DOMAIN")
	for _, id := range supported.Domain {
		fmt.Printf("- %s\n", id)
	}
}

var discoverAPICmd = &cobra.Command{
	Use:   "api URL",
	Short: "Discover well-known paths, exposed files, and fingerprints for an API",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDiscoverAPI(cmd, args[0])
	},
}

func runDiscoverAPI(cmd *cobra.Command, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return NewCommandError(ErrInvalidInput, "Discovery target is required")
	}

	headers, err := parseHeaderAssignments(discoverHeaders)
	if err != nil {
		return err
	}

	format, err := parseAPIScanFormat(discoverFormat)
	if err != nil {
		return err
	}

	if discoverMaxPaths < 1 {
		return NewCommandError(ErrInvalidInput, "Invalid --max-paths value", "must be >= 1")
	}

	if verbose {
		printInfoMessage(
			fmt.Sprintf("Starting API discovery: %s", target),
			fmt.Sprintf("timeout=%s insecure-skip-verify=%t max-paths=%d format=%s", discoverTimeout, discoverInsecureSkipVerify, discoverMaxPaths, format),
		)
	}

	result, discoverErr := apiscanner.DiscoverAPI(cmd.Context(), apiscanner.DiscoverRequest{
		Target:             target,
		Headers:            headers,
		Timeout:            discoverTimeout,
		InsecureSkipVerify: discoverInsecureSkipVerify,
		MaxPaths:           discoverMaxPaths,
		IncludeIDs:         discoverIncludeScanIDs,
		ExcludeIDs:         discoverExcludeScanIDs,
	})
	if discoverErr != nil {
		return NewCommandError(ErrServerError, "API discovery failed", discoverErr.Error())
	}

	output, renderErr := apiscanner.RenderDiscoveryOutput(result, format)
	if renderErr != nil {
		return NewCommandError(ErrServerError, "Failed to render discovery output", renderErr.Error())
	}

	fmt.Println(output)
	return nil
}

func init() {
	discoverCmd.Flags().BoolVar(&discoverListScanIDs, "list-scan-ids", false, "Print all supported discovery scan IDs")

	discoverAPICmd.Flags().StringArrayVar(&discoverHeaders, "header", nil, "HTTP request header in 'Key: Value' format")
	discoverAPICmd.Flags().DurationVar(&discoverTimeout, "timeout", 20*time.Second, "Maximum discovery duration")
	discoverAPICmd.Flags().StringVar(&discoverFormat, "format", string(apiscanner.FormatTable), "Output format: table|json")
	discoverAPICmd.Flags().BoolVar(&discoverInsecureSkipVerify, "insecure-skip-verify", false, "Skip TLS certificate verification")
	discoverAPICmd.Flags().IntVar(&discoverMaxPaths, "max-paths", 64, "Maximum number of discovery paths to probe")
	discoverAPICmd.Flags().StringArrayVar(&discoverIncludeScanIDs, "include-scan-id", nil, "Only run these discovery scan IDs (repeat or comma-separate)")
	discoverAPICmd.Flags().StringArrayVar(&discoverExcludeScanIDs, "exclude-scan-id", nil, "Skip these discovery scan IDs (repeat or comma-separate)")

	discoverCmd.AddCommand(discoverAPICmd)
}
