package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apiscanner"
	"github.com/spf13/cobra"
)

const defaultGraphQLQuery = "query { __typename }"

var (
	graphqlScanHeaders            []string
	graphqlScanQuery              string
	graphqlScanQueryFile          string
	graphqlScanOperationName      string
	graphqlScanTimeout            time.Duration
	graphqlScanFormat             string
	graphqlScanRetryCount         int
	graphqlScanRetryBackoff       time.Duration
	graphqlScanInsecureSkipVerify bool
	graphqlScanAuthHeader         string
	graphqlScanAuthValue          string
	graphqlScanCheckIntrospection bool
)

var scanGraphQLCmd = &cobra.Command{
	Use:   "graphql ENDPOINT",
	Short: "Scan a GraphQL endpoint for API vulnerabilities",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGraphQLScan(cmd, args[0])
	},
}

func runGraphQLScan(cmd *cobra.Command, endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return NewCommandError(ErrInvalidInput, "GraphQL endpoint is required")
	}

	headers, err := parseHeaderAssignments(graphqlScanHeaders)
	if err != nil {
		return err
	}

	format, err := parseAPIScanFormat(graphqlScanFormat)
	if err != nil {
		return err
	}

	query, err := resolveGraphQLQuery(graphqlScanQuery, graphqlScanQueryFile)
	if err != nil {
		return NewCommandError(ErrInvalidInput, "Invalid GraphQL query input", err.Error())
	}

	body := map[string]interface{}{
		"query": query,
	}
	if trimmed := strings.TrimSpace(graphqlScanOperationName); trimmed != "" {
		body["operationName"] = trimmed
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return NewCommandError(ErrServerError, "Failed to build GraphQL request body", err.Error())
	}

	headers["Content-Type"] = "application/json"
	request := apiscanner.ScanRequest{
		Endpoint: apiscanner.Endpoint{
			URL:     endpoint,
			Method:  http.MethodPost,
			Body:    string(bodyBytes),
			Headers: headers,
		},
		Timeout:            graphqlScanTimeout,
		RetryCount:         graphqlScanRetryCount,
		RetryBackoff:       graphqlScanRetryBackoff,
		InsecureSkipVerify: graphqlScanInsecureSkipVerify,
		AuthHeader:         strings.TrimSpace(graphqlScanAuthHeader),
		AuthValue:          strings.TrimSpace(graphqlScanAuthValue),
		Format:             format,
	}

	if verbose {
		printInfoMessage(
			fmt.Sprintf("Starting GraphQL scan: endpoint=%s", endpoint),
			fmt.Sprintf("retry=%d backoff=%s timeout=%s introspection-check=%t format=%s", graphqlScanRetryCount, graphqlScanRetryBackoff, graphqlScanTimeout, graphqlScanCheckIntrospection, format),
		)
	}

	engine := apiscanner.NewEngine()
	result, scanErr := engine.Scan(cmd.Context(), request)
	if scanErr != nil {
		return NewCommandError(ErrServerError, "GraphQL scan failed", scanErr.Error())
	}

	if graphqlScanCheckIntrospection {
		introspectionFinding, introspectionErr := checkGraphQLIntrospection(cmd.Context(), request)
		if introspectionErr != nil {
			if verbose {
				printWarningMessage("GraphQL introspection probe failed", introspectionErr.Error())
			}
		} else if introspectionFinding != nil {
			result.Findings = append(result.Findings, *introspectionFinding)
			result.Summary = summarizeFindings(result.Findings)
		}
	}

	output, renderErr := apiscanner.RenderOutput(result, format)
	if renderErr != nil {
		return NewCommandError(ErrServerError, "Failed to render GraphQL scan output", renderErr.Error())
	}

	fmt.Println(output)
	return nil
}

func resolveGraphQLQuery(inline string, filePath string) (string, error) {
	if strings.TrimSpace(filePath) != "" {
		payload, err := os.ReadFile(strings.TrimSpace(filePath))
		if err != nil {
			return "", fmt.Errorf("failed to read query file: %w", err)
		}
		query := strings.TrimSpace(string(payload))
		if query == "" {
			return "", fmt.Errorf("query file is empty")
		}
		return query, nil
	}

	if strings.TrimSpace(inline) != "" {
		return strings.TrimSpace(inline), nil
	}

	return defaultGraphQLQuery, nil
}

func checkGraphQLIntrospection(ctx context.Context, req apiscanner.ScanRequest) (*apiscanner.Finding, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: req.InsecureSkipVerify} //nolint:gosec
	client := &http.Client{Timeout: req.Timeout, Transport: transport}

	probeBody := map[string]string{
		"query": "query IntrospectionProbe { __schema { queryType { name } } }",
	}
	bodyBytes, _ := json.Marshal(probeBody)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.Endpoint.URL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	for key, value := range req.Endpoint.Headers {
		httpReq.Header.Set(key, value)
	}
	if req.AuthHeader != "" && req.AuthValue != "" {
		httpReq.Header.Set(req.AuthHeader, req.AuthValue)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if readErr != nil {
		return nil, readErr
	}

	bodyText := string(body)
	if resp.StatusCode < 400 && strings.Contains(bodyText, "__schema") {
		finding := apiscanner.Finding{
			Type:           apiscanner.VulnerabilityType("graphql_introspection"),
			Severity:       apiscanner.SeverityMedium,
			Title:          "GraphQL introspection appears enabled",
			Description:    "The endpoint returned schema metadata for an introspection query.",
			Endpoint:       req.Endpoint.URL,
			Method:         http.MethodPost,
			Evidence:       fmt.Sprintf("status=%d body contains __schema", resp.StatusCode),
			Recommendation: "Disable introspection in production or restrict it to trusted roles.",
		}
		return &finding, nil
	}

	return nil, nil
}

func init() {
	scanGraphQLCmd.Flags().StringArrayVar(&graphqlScanHeaders, "header", nil, "HTTP request header in 'Key: Value' format")
	scanGraphQLCmd.Flags().StringVar(&graphqlScanQuery, "query", "", "Inline GraphQL query for baseline requests")
	scanGraphQLCmd.Flags().StringVar(&graphqlScanQueryFile, "query-file", "", "Path to file containing GraphQL query")
	scanGraphQLCmd.Flags().StringVar(&graphqlScanOperationName, "operation-name", "", "Optional GraphQL operation name")
	scanGraphQLCmd.Flags().DurationVar(&graphqlScanTimeout, "timeout", 45*time.Second, "Maximum scan duration")
	scanGraphQLCmd.Flags().StringVar(&graphqlScanFormat, "format", string(apiscanner.FormatTable), "Output format: table|json")
	scanGraphQLCmd.Flags().IntVar(&graphqlScanRetryCount, "retry-count", 1, "Retry count for failed probe requests")
	scanGraphQLCmd.Flags().DurationVar(&graphqlScanRetryBackoff, "retry-backoff", time.Second, "Base backoff duration for exponential retry")
	scanGraphQLCmd.Flags().BoolVar(&graphqlScanInsecureSkipVerify, "insecure-skip-verify", false, "Skip TLS certificate verification")
	scanGraphQLCmd.Flags().StringVar(&graphqlScanAuthHeader, "auth-header", "Authorization", "Header used for authenticated baseline checks")
	scanGraphQLCmd.Flags().StringVar(&graphqlScanAuthValue, "auth-value", "", "Authentication value used for auth bypass checks (e.g. 'Bearer token')")
	scanGraphQLCmd.Flags().BoolVar(&graphqlScanCheckIntrospection, "check-introspection", true, "Check whether GraphQL introspection is exposed")

	scanCmd.AddCommand(scanGraphQLCmd)
}
