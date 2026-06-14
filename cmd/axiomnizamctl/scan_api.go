package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apiscanner"
	"github.com/spf13/cobra"
)

var (
	apiScanMethod             string
	apiScanHeaders            []string
	apiScanBody               string
	apiScanTimeout            time.Duration
	apiScanFormat             string
	apiScanRetryCount         int
	apiScanRetryBackoff       time.Duration
	apiScanInsecureSkipVerify bool
	apiScanAuthHeader         string
	apiScanAuthValue          string
)

var scanAPICmd = &cobra.Command{
	Use:   "api URL",
	Short: "Scan an API endpoint for auth bypass, injection, XSS, and misconfigurations",
	Long: `Runtime API security scanner inspired by modern DAST workflows.

Checks:
- Authentication bypass
- SQL injection
- NoSQL injection
- HTTP method validation
- Security headers
- Parameter tampering
- XSS reflection`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAPIScan(cmd, args[0])
	},
}

func runAPIScan(cmd *cobra.Command, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return NewCommandError(ErrInvalidInput, "Scan target is required")
	}

	if !isSupportedHTTPMethod(apiScanMethod) {
		return NewCommandError(ErrInvalidInput, "Invalid --method value", "supported methods: GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD")
	}

	headers, err := parseHeaderAssignments(apiScanHeaders)
	if err != nil {
		return err
	}

	format, err := parseAPIScanFormat(apiScanFormat)
	if err != nil {
		return err
	}

	request := apiscanner.ScanRequest{
		Endpoint: apiscanner.Endpoint{
			URL:     target,
			Method:  strings.ToUpper(strings.TrimSpace(apiScanMethod)),
			Body:    apiScanBody,
			Headers: headers,
		},
		Timeout:            apiScanTimeout,
		RetryCount:         apiScanRetryCount,
		RetryBackoff:       apiScanRetryBackoff,
		InsecureSkipVerify: apiScanInsecureSkipVerify,
		AuthHeader:         strings.TrimSpace(apiScanAuthHeader),
		AuthValue:          strings.TrimSpace(apiScanAuthValue),
		Format:             format,
	}

	if verbose {
		printInfoMessage(
			fmt.Sprintf("Starting API scan: %s %s", request.Endpoint.Method, request.Endpoint.URL),
			fmt.Sprintf("retry=%d backoff=%s timeout=%s insecure-skip-verify=%t format=%s", apiScanRetryCount, apiScanRetryBackoff, apiScanTimeout, apiScanInsecureSkipVerify, format),
		)
	}

	engine := apiscanner.NewEngine()
	result, scanErr := engine.Scan(cmd.Context(), request)
	if scanErr != nil {
		return NewCommandError(ErrServerError, "API scan failed", scanErr.Error())
	}

	output, renderErr := apiscanner.RenderOutput(result, format)
	if renderErr != nil {
		return NewCommandError(ErrServerError, "Failed to render API scan output", renderErr.Error())
	}

	fmt.Println(output)
	return nil
}

func parseAPIScanFormat(raw string) (apiscanner.OutputFormat, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(apiscanner.FormatTable):
		return apiscanner.FormatTable, nil
	case string(apiscanner.FormatJSON):
		return apiscanner.FormatJSON, nil
	default:
		return "", NewCommandError(ErrInvalidInput, "Invalid --format value", "supported formats: table, json")
	}
}

func isSupportedHTTPMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodHead:
		return true
	default:
		return false
	}
}

func init() {
	scanAPICmd.Flags().StringVar(&apiScanMethod, "method", http.MethodGet, "HTTP method to use for baseline requests")
	scanAPICmd.Flags().StringArrayVar(&apiScanHeaders, "header", nil, "HTTP request header in 'Key: Value' format")
	scanAPICmd.Flags().StringVar(&apiScanBody, "body", "", "Request body for non-GET API scans")
	scanAPICmd.Flags().DurationVar(&apiScanTimeout, "timeout", 30*time.Second, "Maximum scan duration")
	scanAPICmd.Flags().StringVar(&apiScanFormat, "format", string(apiscanner.FormatTable), "Output format: table|json")
	scanAPICmd.Flags().IntVar(&apiScanRetryCount, "retry-count", 1, "Retry count for failed probe requests")
	scanAPICmd.Flags().DurationVar(&apiScanRetryBackoff, "retry-backoff", time.Second, "Base backoff duration for exponential retry")
	scanAPICmd.Flags().BoolVar(&apiScanInsecureSkipVerify, "insecure-skip-verify", false, "Skip TLS certificate verification")
	scanAPICmd.Flags().StringVar(&apiScanAuthHeader, "auth-header", "Authorization", "Header used for authenticated baseline checks")
	scanAPICmd.Flags().StringVar(&apiScanAuthValue, "auth-value", "", "Authentication value used for auth bypass checks (e.g. 'Bearer token')")

	scanCmd.AddCommand(scanAPICmd)
}
