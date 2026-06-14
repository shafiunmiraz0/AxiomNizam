package main

import (
	"fmt"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apiscanner"
	"github.com/spf13/cobra"
)

var (
	discoverDomainHeaders            []string
	discoverDomainTimeout            time.Duration
	discoverDomainFormat             string
	discoverDomainInsecureSkipVerify bool
	discoverDomainMaxSubdomains      int
	discoverDomainMaxHints           int
	discoverDomainSchemes            []string
	discoverDomainIncludeScanIDs     []string
	discoverDomainExcludeScanIDs     []string
)

var discoverDomainCmd = &cobra.Command{
	Use:   "domain DOMAIN",
	Short: "Discover common API-related subdomains and endpoint hints",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDiscoverDomain(cmd, args[0])
	},
}

func runDiscoverDomain(cmd *cobra.Command, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return NewCommandError(ErrInvalidInput, "Domain target is required")
	}

	headers, err := parseHeaderAssignments(discoverDomainHeaders)
	if err != nil {
		return err
	}

	format, err := parseAPIScanFormat(discoverDomainFormat)
	if err != nil {
		return err
	}

	if discoverDomainMaxSubdomains < 1 {
		return NewCommandError(ErrInvalidInput, "Invalid --max-subdomains value", "must be >= 1")
	}
	if discoverDomainMaxHints < 1 {
		return NewCommandError(ErrInvalidInput, "Invalid --max-hints value", "must be >= 1")
	}

	if verbose {
		printInfoMessage(
			fmt.Sprintf("Starting domain discovery: %s", target),
			fmt.Sprintf("timeout=%s insecure-skip-verify=%t max-subdomains=%d max-hints=%d format=%s", discoverDomainTimeout, discoverDomainInsecureSkipVerify, discoverDomainMaxSubdomains, discoverDomainMaxHints, format),
		)
	}

	result, discoverErr := apiscanner.DiscoverDomain(cmd.Context(), apiscanner.DiscoverDomainRequest{
		Target:             target,
		Headers:            headers,
		Timeout:            discoverDomainTimeout,
		InsecureSkipVerify: discoverDomainInsecureSkipVerify,
		MaxSubdomains:      discoverDomainMaxSubdomains,
		MaxHints:           discoverDomainMaxHints,
		Schemes:            discoverDomainSchemes,
		IncludeIDs:         discoverDomainIncludeScanIDs,
		ExcludeIDs:         discoverDomainExcludeScanIDs,
	})
	if discoverErr != nil {
		return NewCommandError(ErrServerError, "Domain discovery failed", discoverErr.Error())
	}

	output, renderErr := apiscanner.RenderDomainDiscoveryOutput(result, format)
	if renderErr != nil {
		return NewCommandError(ErrServerError, "Failed to render domain discovery output", renderErr.Error())
	}

	fmt.Println(output)
	return nil
}

func init() {
	discoverDomainCmd.Flags().StringArrayVar(&discoverDomainHeaders, "header", nil, "HTTP request header in 'Key: Value' format")
	discoverDomainCmd.Flags().DurationVar(&discoverDomainTimeout, "timeout", 20*time.Second, "Maximum discovery duration")
	discoverDomainCmd.Flags().StringVar(&discoverDomainFormat, "format", string(apiscanner.FormatTable), "Output format: table|json")
	discoverDomainCmd.Flags().BoolVar(&discoverDomainInsecureSkipVerify, "insecure-skip-verify", false, "Skip TLS certificate verification")
	discoverDomainCmd.Flags().IntVar(&discoverDomainMaxSubdomains, "max-subdomains", 32, "Maximum number of subdomain candidates")
	discoverDomainCmd.Flags().IntVar(&discoverDomainMaxHints, "max-hints", 48, "Maximum number of API hints to collect")
	discoverDomainCmd.Flags().StringArrayVar(&discoverDomainSchemes, "scheme", []string{"https", "http"}, "Schemes to probe (repeat or comma-separate: https,http)")
	discoverDomainCmd.Flags().StringArrayVar(&discoverDomainIncludeScanIDs, "include-scan-id", nil, "Only run these discovery scan IDs (repeat or comma-separate)")
	discoverDomainCmd.Flags().StringArrayVar(&discoverDomainExcludeScanIDs, "exclude-scan-id", nil, "Skip these discovery scan IDs (repeat or comma-separate)")

	discoverCmd.AddCommand(discoverDomainCmd)
}
