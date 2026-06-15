package main

import (
	"fmt"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/trivy"
	"github.com/spf13/cobra"
)

const (
	scanDefaultSeverity = "HIGH,CRITICAL"
)

var (
	scanSeverity      string
	scanIgnoreUnfixed bool
	scanTimeout       time.Duration
	scanFormat        string
	scanExternal      bool
	scanRetryCount    int
	scanRetryBackoff  time.Duration
	scanBinary        string
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan images, filesystems, repositories, and Kubernetes/Helm configs",
	Long: `Security scanning powered by Trivy with normalized results and retry support.

Architecture:
CLI -> internal/trivy -> optional external wrapper -> trivy binary`,
}

var scanImageCmd = &cobra.Command{
	Use:   "image IMAGE",
	Short: "Scan a container image",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTrivyScan(cmd, trivy.TargetImage, args[0])
	},
}

var scanFSCmd = &cobra.Command{
	Use:   "fs PATH",
	Short: "Scan a filesystem path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTrivyScan(cmd, trivy.TargetFS, args[0])
	},
}

var scanK8sCmd = &cobra.Command{
	Use:   "k8s PATH",
	Short: "Scan Kubernetes YAML files or Helm charts",
	Long:  "Scans Kubernetes manifests and Helm chart directories using Trivy config scanning.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTrivyScan(cmd, trivy.TargetK8s, args[0])
	},
}

var scanRepoCmd = &cobra.Command{
	Use:   "repo PATH",
	Short: "Scan a repository path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTrivyScan(cmd, trivy.TargetRepo, args[0])
	},
}

func runTrivyScan(cmd *cobra.Command, kind trivy.TargetKind, target string) error {
	if strings.TrimSpace(target) == "" {
		return NewCommandError(ErrInvalidInput, "Scan target is required")
	}

	severity := trivy.ParseSeverityCSV(scanSeverity)
	format, err := parseScanFormat(scanFormat)
	if err != nil {
		return err
	}

	request := trivy.ScanRequest{
		Kind:          kind,
		Target:        strings.TrimSpace(target),
		Severity:      severity,
		IgnoreUnfixed: scanIgnoreUnfixed,
		Timeout:       scanTimeout,
		Format:        format,
		UseExternal:   scanExternal,
		RetryCount:    scanRetryCount,
		RetryBackoff:  scanRetryBackoff,
	}

	if verbose {
		printInfoMessage(
			fmt.Sprintf("Starting Trivy scan: kind=%s target=%s", kind, target),
			fmt.Sprintf("severity=%s ignore-unfixed=%t retry-count=%d retry-backoff=%s timeout=%s external=%t format=%s",
				strings.Join(severity, ","),
				scanIgnoreUnfixed,
				scanRetryCount,
				scanRetryBackoff,
				scanTimeout,
				scanExternal,
				format,
			),
		)
	}

	engine := trivy.NewEngine(scanBinary)
	result, scanErr := engine.Scan(cmd.Context(), request)
	if scanErr != nil {
		return NewCommandError(ErrServerError, "Trivy scan failed", scanErr.Error())
	}

	output, renderErr := trivy.RenderOutput(result, format)
	if renderErr != nil {
		return NewCommandError(ErrServerError, "Failed to render scan output", renderErr.Error())
	}

	fmt.Println(output)

	return nil
}

func parseScanFormat(raw string) (trivy.OutputFormat, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(trivy.FormatTable):
		return trivy.FormatTable, nil
	case string(trivy.FormatJSON):
		return trivy.FormatJSON, nil
	default:
		return "", NewCommandError(ErrInvalidInput, "Invalid --format value", "supported formats: table, json")
	}
}

func init() {
	scanCmd.PersistentFlags().StringVar(&scanSeverity, "severity", scanDefaultSeverity, "Comma-separated severity filter (CRITICAL,HIGH,MEDIUM,LOW,UNKNOWN)")
	scanCmd.PersistentFlags().BoolVar(&scanIgnoreUnfixed, "ignore-unfixed", false, "Ignore unfixed vulnerabilities")
	scanCmd.PersistentFlags().DurationVar(&scanTimeout, "timeout", 5*time.Minute, "Maximum scan duration")
	scanCmd.PersistentFlags().StringVar(&scanFormat, "format", string(trivy.FormatTable), "Output format: table|json")
	scanCmd.PersistentFlags().BoolVar(&scanExternal, "external", true, "Use external Trivy binary execution")
	scanCmd.PersistentFlags().IntVar(&scanRetryCount, "retry-count", 1, "Retry count when scan fails")
	scanCmd.PersistentFlags().DurationVar(&scanRetryBackoff, "retry-backoff", time.Second, "Base backoff duration for exponential retry")
	scanCmd.PersistentFlags().StringVar(&scanBinary, "trivy-binary", "trivy", "Path to trivy binary")

	scanCmd.AddCommand(scanImageCmd)
	scanCmd.AddCommand(scanFSCmd)
	scanCmd.AddCommand(scanK8sCmd)
	scanCmd.AddCommand(scanRepoCmd)
}
