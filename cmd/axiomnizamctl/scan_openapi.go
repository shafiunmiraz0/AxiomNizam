package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apiscanner"
	"github.com/spf13/cobra"
)

var (
	openapiScanBaseURL            string
	openapiScanHeaders            []string
	openapiScanTimeout            time.Duration
	openapiScanFormat             string
	openapiScanRetryCount         int
	openapiScanRetryBackoff       time.Duration
	openapiScanInsecureSkipVerify bool
	openapiScanAuthHeader         string
	openapiScanAuthValue          string
	openapiScanMaxEndpoints       int
)

var scanOpenAPICmd = &cobra.Command{
	Use:   "openapi PATH_OR_URL",
	Short: "Scan all endpoints from an OpenAPI specification",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runOpenAPIScan(cmd, args[0])
	},
}

func runOpenAPIScan(cmd *cobra.Command, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return NewCommandError(ErrInvalidInput, "OpenAPI target is required")
	}

	headers, err := parseHeaderAssignments(openapiScanHeaders)
	if err != nil {
		return err
	}

	format, err := parseAPIScanFormat(openapiScanFormat)
	if err != nil {
		return err
	}

	if openapiScanMaxEndpoints < 1 {
		return NewCommandError(ErrInvalidInput, "Invalid --max-endpoints value", "must be >= 1")
	}

	endpoints, loadErr := apiscanner.LoadOpenAPIEndpoints(
		cmd.Context(),
		target,
		strings.TrimSpace(openapiScanBaseURL),
		headers,
		openapiScanTimeout,
		openapiScanInsecureSkipVerify,
	)
	if loadErr != nil {
		return NewCommandError(ErrInvalidInput, "Failed to load OpenAPI endpoints", loadErr.Error())
	}

	if len(endpoints) > openapiScanMaxEndpoints {
		endpoints = endpoints[:openapiScanMaxEndpoints]
	}

	if verbose {
		printInfoMessage(
			fmt.Sprintf("Starting OpenAPI scan: endpoints=%d source=%s", len(endpoints), target),
			fmt.Sprintf("retry=%d backoff=%s timeout=%s format=%s", openapiScanRetryCount, openapiScanRetryBackoff, openapiScanTimeout, format),
		)
	}

	engine := apiscanner.NewEngine()
	results := make([]apiscanner.ScanResult, 0, len(endpoints))
	allFindings := make([]apiscanner.Finding, 0)

	for _, endpoint := range endpoints {
		request := apiscanner.ScanRequest{
			Endpoint: apiscanner.Endpoint{
				URL:     endpoint.URL,
				Method:  endpoint.Method,
				Body:    endpoint.Body,
				Headers: mergeHeaderMaps(headers, endpoint.Headers),
			},
			Timeout:            openapiScanTimeout,
			RetryCount:         openapiScanRetryCount,
			RetryBackoff:       openapiScanRetryBackoff,
			InsecureSkipVerify: openapiScanInsecureSkipVerify,
			AuthHeader:         strings.TrimSpace(openapiScanAuthHeader),
			AuthValue:          strings.TrimSpace(openapiScanAuthValue),
			Format:             format,
		}

		result, scanErr := engine.Scan(cmd.Context(), request)
		if scanErr != nil {
			if verbose {
				printWarningMessage("OpenAPI operation scan failed", fmt.Sprintf("endpoint=%s method=%s error=%v", endpoint.URL, endpoint.Method, scanErr))
			}
			continue
		}

		results = append(results, result)
		allFindings = append(allFindings, result.Findings...)
	}

	if len(results) == 0 {
		return NewCommandError(ErrServerError, "OpenAPI scan failed", "no operations were successfully scanned")
	}

	summary := summarizeFindings(allFindings)

	if format == apiscanner.FormatJSON {
		payload := struct {
			Scanner        string                  `json:"scanner"`
			Source         string                  `json:"source"`
			ScannedAt      time.Time               `json:"scannedAt"`
			Operations     int                     `json:"operations"`
			OperationScans []apiscanner.ScanResult `json:"operationScans"`
			Summary        apiscanner.Summary      `json:"summary"`
		}{
			Scanner:        "api-scanner-openapi",
			Source:         target,
			ScannedAt:      time.Now().UTC(),
			Operations:     len(results),
			OperationScans: results,
			Summary:        summary,
		}

		encoded, marshalErr := json.MarshalIndent(payload, "", "  ")
		if marshalErr != nil {
			return NewCommandError(ErrServerError, "Failed to render OpenAPI scan output", marshalErr.Error())
		}
		fmt.Println(string(encoded))
		return nil
	}

	for index, result := range results {
		if index > 0 {
			fmt.Println()
		}
		output, renderErr := apiscanner.RenderOutput(result, format)
		if renderErr != nil {
			return NewCommandError(ErrServerError, "Failed to render OpenAPI operation output", renderErr.Error())
		}
		fmt.Println(output)
	}

	fmt.Println()
	fmt.Println("OPENAPI SUMMARY")
	fmt.Printf("SOURCE: %s\n", target)
	fmt.Printf("SCANNED OPERATIONS: %d\n", len(results))
	fmt.Printf("TOTAL FINDINGS: %d\n", summary.Total)
	fmt.Printf("CRITICAL: %d HIGH: %d MEDIUM: %d LOW: %d INFO: %d\n", summary.Critical, summary.High, summary.Medium, summary.Low, summary.Info)

	return nil
}

func mergeHeaderMaps(global map[string]string, specific map[string]string) map[string]string {
	merged := make(map[string]string, len(global)+len(specific))
	for key, value := range global {
		merged[key] = value
	}
	for key, value := range specific {
		merged[key] = value
	}
	return merged
}

func summarizeFindings(findings []apiscanner.Finding) apiscanner.Summary {
	summary := apiscanner.Summary{Total: len(findings)}
	for _, finding := range findings {
		switch strings.ToUpper(strings.TrimSpace(finding.Severity)) {
		case apiscanner.SeverityCritical:
			summary.Critical++
		case apiscanner.SeverityHigh:
			summary.High++
		case apiscanner.SeverityMedium:
			summary.Medium++
		case apiscanner.SeverityLow:
			summary.Low++
		default:
			summary.Info++
		}
	}
	return summary
}

func init() {
	scanOpenAPICmd.Flags().StringVar(&openapiScanBaseURL, "base-url", "", "Override base URL for relative OpenAPI paths")
	scanOpenAPICmd.Flags().StringArrayVar(&openapiScanHeaders, "header", nil, "HTTP request header in 'Key: Value' format")
	scanOpenAPICmd.Flags().DurationVar(&openapiScanTimeout, "timeout", 45*time.Second, "Maximum scan duration")
	scanOpenAPICmd.Flags().StringVar(&openapiScanFormat, "format", string(apiscanner.FormatTable), "Output format: table|json")
	scanOpenAPICmd.Flags().IntVar(&openapiScanRetryCount, "retry-count", 1, "Retry count for failed operation probes")
	scanOpenAPICmd.Flags().DurationVar(&openapiScanRetryBackoff, "retry-backoff", time.Second, "Base backoff duration for exponential retry")
	scanOpenAPICmd.Flags().BoolVar(&openapiScanInsecureSkipVerify, "insecure-skip-verify", false, "Skip TLS certificate verification")
	scanOpenAPICmd.Flags().StringVar(&openapiScanAuthHeader, "auth-header", "Authorization", "Header used for authenticated baseline checks")
	scanOpenAPICmd.Flags().StringVar(&openapiScanAuthValue, "auth-value", "", "Authentication value used for auth bypass checks (e.g. 'Bearer token')")
	scanOpenAPICmd.Flags().IntVar(&openapiScanMaxEndpoints, "max-endpoints", 150, "Maximum number of OpenAPI operations to scan")

	scanCmd.AddCommand(scanOpenAPICmd)
}
