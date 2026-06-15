package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/waitx"
	"github.com/spf13/cobra"
)

const (
	waitFlagDialTimeout    = "dial-timeout"
	waitFlagRequestTimeout = "request-timeout"
	waitFlagConnectTimeout = "connect-timeout"
)

var (
	waitTimeout          time.Duration
	waitInterval         time.Duration
	waitMaxInterval      time.Duration
	waitInvertCheck      bool
	waitRetryPolicy      string
	waitRetryCoefficient float64
	waitRetrySequence    string
	waitQuiet            bool

	waitTCPDialTimeout time.Duration

	waitHTTPMethod             string
	waitHTTPExpectStatusCode   int
	waitHTTPExpectBodyRegex    string
	waitHTTPHeaders            []string
	waitHTTPBody               string
	waitHTTPInsecureSkipVerify bool
	waitHTTPRequestTimeout     time.Duration

	waitGRPCService            string
	waitGRPCUseTLS             bool
	waitGRPCInsecureSkipVerify bool
	waitGRPCTLSServerName      string
	waitGRPCDialTimeout        time.Duration
	waitGRPCRequestTimeout     time.Duration

	waitK8SPodName    string
	waitK8SSelector   string
	waitK8SNamespace  string
	waitK8SKubeconfig string
	waitK8SContext    string
	waitK8SMinReady   int
	waitK8SKubectl    string

	waitCustomExpectedExitCode int
	waitCustomWorkingDir       string

	waitExternalBinary string

	waitDNSNameServer     string
	waitDNSExpectedValues []string
	waitDNSDialTimeout    time.Duration

	waitRedisExpectedKey        string
	waitRedisExpectedValueRegex string
	waitRedisDialTimeout        time.Duration
	waitRedisReadTimeout        time.Duration

	waitMySQLExpectedTable     string
	waitMySQLConnectTimeout    time.Duration
	waitPostgresExpectedTable  string
	waitPostgresConnectTimeout time.Duration
	waitMongoConnectTimeout    time.Duration

	waitRabbitDialTimeout   time.Duration
	waitKafkaDialTimeout    time.Duration
	waitInfluxReqTimeout    time.Duration
	waitInfluxExpectStatus  int
	waitTemporalDialTimeout time.Duration
)

var waitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Wait for external and platform dependencies",
	Long: `Wait for endpoints, ports, gRPC health checks, Kubernetes pod readiness,
and custom command conditions with configurable retry/backoff policies.`,
}

var waitTCPCmd = &cobra.Command{
	Use:   "tcp ADDRESS [ADDRESS...]",
	Short: "Wait for one or more TCP addresses to become reachable",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checkers := make([]waitx.Checker, 0, len(args))
		for _, address := range args {
			checkers = append(checkers, waitx.TCPChecker{
				Address:     strings.TrimSpace(address),
				DialTimeout: waitTCPDialTimeout,
			})
		}

		checker := waitx.Checker(waitx.MultiChecker{
			NamePrefix: "tcp",
			Checkers:   checkers,
			Parallel:   len(checkers) > 1,
		})
		return executeWaitChecker(cmd, checker)
	},
}

var waitDNSCmd = &cobra.Command{
	Use:   "dns RECORD_TYPE ADDRESS",
	Short: "Wait for DNS records (A, AAAA, CNAME, MX, TXT, NS)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		checker := waitx.DNSChecker{
			RecordType:     strings.TrimSpace(args[0]),
			Address:        strings.TrimSpace(args[1]),
			NameServer:     strings.TrimSpace(waitDNSNameServer),
			ExpectedValues: waitDNSExpectedValues,
			DialTimeout:    waitDNSDialTimeout,
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitHTTPCmd = &cobra.Command{
	Use:   "http URL",
	Short: "Wait for an HTTP endpoint and optional response expectations",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		headers, err := parseHeaderAssignments(waitHTTPHeaders)
		if err != nil {
			return err
		}

		var bodyRegex *regexp.Regexp
		if strings.TrimSpace(waitHTTPExpectBodyRegex) != "" {
			bodyRegex, err = regexp.Compile(waitHTTPExpectBodyRegex)
			if err != nil {
				return NewCommandError(ErrInvalidInput, "Invalid --expect-body-regex", err.Error())
			}
		}

		checker := waitx.HTTPChecker{
			URL:              strings.TrimSpace(args[0]),
			Method:           waitHTTPMethod,
			Headers:          headers,
			Body:             waitHTTPBody,
			ExpectStatusCode: waitHTTPExpectStatusCode,
			ExpectBodyRegex:  bodyRegex,
			InsecureSkipTLS:  waitHTTPInsecureSkipVerify,
			RequestTimeout:   waitHTTPRequestTimeout,
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitGRPCHealthCmd = &cobra.Command{
	Use:   "grpc-health TARGET",
	Short: "Wait for gRPC health status SERVING",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checker := waitx.GRPCHealthChecker{
			Target:             strings.TrimSpace(args[0]),
			Service:            strings.TrimSpace(waitGRPCService),
			UseTLS:             waitGRPCUseTLS,
			TLSServerName:      strings.TrimSpace(waitGRPCTLSServerName),
			InsecureSkipVerify: waitGRPCInsecureSkipVerify,
			DialTimeout:        waitGRPCDialTimeout,
			RequestTimeout:     waitGRPCRequestTimeout,
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitK8SPodCmd = &cobra.Command{
	Use:   "k8s-pod",
	Short: "Wait for Kubernetes pod readiness (by name or selector)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(waitK8SPodName) == "" && strings.TrimSpace(waitK8SSelector) == "" {
			return NewCommandError(ErrInvalidInput, "Either --name or --selector is required")
		}

		ns := strings.TrimSpace(waitK8SNamespace)
		if ns == "" {
			ns = strings.TrimSpace(namespace)
		}
		if ns == "" {
			ns = "default"
		}

		checker := waitx.KubernetesPodReadinessChecker{
			KubectlBinary: strings.TrimSpace(waitK8SKubectl),
			PodName:       strings.TrimSpace(waitK8SPodName),
			LabelSelector: strings.TrimSpace(waitK8SSelector),
			Namespace:     ns,
			Kubeconfig:    strings.TrimSpace(waitK8SKubeconfig),
			Context:       strings.TrimSpace(waitK8SContext),
			MinReady:      waitK8SMinReady,
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitCustomCmd = &cobra.Command{
	Use:   "custom COMMAND [ARGS...]",
	Short: "Wait for a custom command to return an expected exit code",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checker := waitx.CommandChecker{
			Command:          args,
			ExpectedExitCode: waitCustomExpectedExitCode,
			WorkingDirectory: strings.TrimSpace(waitCustomWorkingDir),
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitMySQLCmd = &cobra.Command{
	Use:   "mysql DSN",
	Short: "Wait for MySQL connectivity and optional table existence",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checker := waitx.MySQLChecker{
			DSN:            strings.TrimSpace(args[0]),
			ExpectedTable:  strings.TrimSpace(waitMySQLExpectedTable),
			ConnectTimeout: waitMySQLConnectTimeout,
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitPostgreSQLCmd = &cobra.Command{
	Use:   "postgresql DSN",
	Short: "Wait for PostgreSQL connectivity and optional table existence",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checker := waitx.PostgreSQLChecker{
			DSN:            strings.TrimSpace(args[0]),
			ExpectedTable:  strings.TrimSpace(waitPostgresExpectedTable),
			ConnectTimeout: waitPostgresConnectTimeout,
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitMongoDBCmd = &cobra.Command{
	Use:   "mongodb URI",
	Short: "Wait for MongoDB connectivity",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checker := waitx.MongoDBChecker{
			URI:            strings.TrimSpace(args[0]),
			ConnectTimeout: waitMongoConnectTimeout,
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitRedisCmd = &cobra.Command{
	Use:   "redis ADDRESS",
	Short: "Wait for Redis and optional key/value expectations",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var valueRegex *regexp.Regexp
		if strings.TrimSpace(waitRedisExpectedValueRegex) != "" {
			re, err := regexp.Compile(waitRedisExpectedValueRegex)
			if err != nil {
				return NewCommandError(ErrInvalidInput, "Invalid --expect-value-regex", err.Error())
			}
			valueRegex = re
		}

		checker := waitx.RedisChecker{
			Address:            strings.TrimSpace(args[0]),
			ExpectedKey:        strings.TrimSpace(waitRedisExpectedKey),
			ExpectedValueRegex: valueRegex,
			DialTimeout:        waitRedisDialTimeout,
			ReadTimeout:        waitRedisReadTimeout,
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitRabbitMQCmd = &cobra.Command{
	Use:   "rabbitmq URL",
	Short: "Wait for RabbitMQ endpoint reachability",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checker := waitx.RabbitMQChecker{
			URL:         strings.TrimSpace(args[0]),
			DialTimeout: waitRabbitDialTimeout,
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitKafkaCmd = &cobra.Command{
	Use:   "kafka BROKER [BROKER...]",
	Short: "Wait for one or more Kafka broker endpoints",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checker := waitx.KafkaChecker{
			Brokers:     args,
			DialTimeout: waitKafkaDialTimeout,
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitInfluxDBCmd = &cobra.Command{
	Use:   "influxdb URL",
	Short: "Wait for InfluxDB health endpoint",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checker := waitx.InfluxDBChecker{
			URL:              strings.TrimSpace(args[0]),
			RequestTimeout:   waitInfluxReqTimeout,
			ExpectStatusCode: waitInfluxExpectStatus,
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitTemporalCmd = &cobra.Command{
	Use:   "temporal TARGET",
	Short: "Wait for Temporal server endpoint reachability",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checker := waitx.TemporalServerChecker{
			Target:      strings.TrimSpace(args[0]),
			DialTimeout: waitTemporalDialTimeout,
		}
		return executeWaitChecker(cmd, checker)
	},
}

var waitExternalCmd = &cobra.Command{
	Use:   "external WAIT4X_ARGS...",
	Short: "Run wait4x through a controlled wrapper with command allowlist",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wrapper := waitx.NewControlledWait4xWrapper(strings.TrimSpace(waitExternalBinary))
		if err := wrapper.Run(cmd.Context(), args); err != nil {
			return NewCommandError(ErrServerError, "Controlled wait4x execution failed", err.Error())
		}
		return nil
	},
}

func executeWaitChecker(cmd *cobra.Command, checker waitx.Checker) error {
	opts, err := buildWaitOptions()
	if err != nil {
		return err
	}

	started := time.Now()
	if verbose {
		opts.OnRetry = func(event waitx.AttemptEvent) {
			fmt.Printf("retry=%d checker=%s elapsed=%s delay=%s error=%v\n",
				event.Attempt,
				event.Checker,
				event.Elapsed.Round(time.Millisecond),
				event.Delay.Round(time.Millisecond),
				event.Err,
			)
		}
	}

	if err := waitx.WaitContext(cmd.Context(), checker, opts); err != nil {
		return NewCommandError(ErrTimeout, "Wait condition failed", err.Error())
	}

	if !waitQuiet {
		printSuccessMessage(
			fmt.Sprintf("Condition satisfied: %s", checker.Name()),
			fmt.Sprintf("Elapsed: %s", time.Since(started).Round(time.Millisecond)),
		)
	}
	return nil
}

func buildWaitOptions() (waitx.WaitOptions, error) {
	sequence, err := waitx.ParseDurationSequence(waitRetrySequence)
	if err != nil {
		return waitx.WaitOptions{}, NewCommandError(ErrInvalidInput, "Invalid --retry-sequence", err.Error())
	}

	strategy, err := waitx.NewRetryStrategy(waitRetryPolicy, waitRetryCoefficient, sequence)
	if err != nil {
		return waitx.WaitOptions{}, NewCommandError(ErrInvalidInput, "Invalid retry strategy", err.Error())
	}

	return waitx.WaitOptions{
		Timeout:       waitTimeout,
		Interval:      waitInterval,
		MaxInterval:   waitMaxInterval,
		InvertCheck:   waitInvertCheck,
		RetryStrategy: strategy,
	}, nil
}

func parseHeaderAssignments(values []string) (map[string]string, error) {
	headers := make(map[string]string)
	for _, raw := range values {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			return nil, NewCommandError(ErrInvalidInput, "Invalid --header format", "use 'Key: Value'")
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, NewCommandError(ErrInvalidInput, "Invalid --header format", "header key cannot be empty")
		}
		headers[key] = value
	}
	return headers, nil
}

func init() {
	waitCmd.PersistentFlags().DurationVarP(&waitTimeout, "timeout", "t", 30*time.Second, "Maximum total wait duration (0 means no timeout)")
	waitCmd.PersistentFlags().DurationVarP(&waitInterval, "interval", "i", 1*time.Second, "Base interval between checks")
	waitCmd.PersistentFlags().DurationVar(&waitMaxInterval, "max-interval", 30*time.Second, "Maximum retry interval")
	waitCmd.PersistentFlags().BoolVar(&waitInvertCheck, "invert-check", false, "Invert check result (wait for not-ready)")
	waitCmd.PersistentFlags().StringVar(&waitRetryPolicy, "retry-policy", waitx.RetryLinear, "Retry policy: linear|exponential|fibonacci|custom")
	waitCmd.PersistentFlags().Float64Var(&waitRetryCoefficient, "retry-coefficient", 2.0, "Retry multiplier for exponential fallback")
	waitCmd.PersistentFlags().StringVar(&waitRetrySequence, "retry-sequence", "", "Comma-separated durations for custom retry policy (e.g. 500ms,1s,2s)")
	waitCmd.PersistentFlags().BoolVar(&waitQuiet, "quiet", false, "Suppress success output")

	waitTCPCmd.Flags().DurationVar(&waitTCPDialTimeout, waitFlagDialTimeout, 3*time.Second, "TCP dial timeout per attempt")

	waitDNSCmd.Flags().StringVarP(&waitDNSNameServer, "nameserver", "n", "", "Nameserver to query against (e.g. 8.8.8.8:53)")
	waitDNSCmd.Flags().StringArrayVar(&waitDNSExpectedValues, "expect", nil, "Expected DNS record value (repeat flag for multiple values)")
	waitDNSCmd.Flags().DurationVar(&waitDNSDialTimeout, waitFlagDialTimeout, 3*time.Second, "DNS nameserver dial timeout")

	waitHTTPCmd.Flags().StringVar(&waitHTTPMethod, "method", "GET", "HTTP method")
	waitHTTPCmd.Flags().IntVar(&waitHTTPExpectStatusCode, "expect-status-code", 200, "Expected HTTP status code (0 to disable)")
	waitHTTPCmd.Flags().StringVar(&waitHTTPExpectBodyRegex, "expect-body-regex", "", "Regex to match response body")
	waitHTTPCmd.Flags().StringArrayVar(&waitHTTPHeaders, "header", nil, "HTTP request header in 'Key: Value' format")
	waitHTTPCmd.Flags().StringVar(&waitHTTPBody, "body", "", "HTTP request body")
	waitHTTPCmd.Flags().BoolVar(&waitHTTPInsecureSkipVerify, "insecure-skip-verify", false, "Skip TLS certificate verification")
	waitHTTPCmd.Flags().DurationVar(&waitHTTPRequestTimeout, waitFlagRequestTimeout, 5*time.Second, "Per-request timeout")

	waitGRPCHealthCmd.Flags().StringVar(&waitGRPCService, "service", "", "gRPC health service name (empty checks server default)")
	waitGRPCHealthCmd.Flags().BoolVar(&waitGRPCUseTLS, "tls", false, "Use TLS transport")
	waitGRPCHealthCmd.Flags().BoolVar(&waitGRPCInsecureSkipVerify, "insecure-skip-verify", false, "Skip TLS certificate verification (for self-signed certs)")
	waitGRPCHealthCmd.Flags().StringVar(&waitGRPCTLSServerName, "tls-server-name", "", "TLS server name override")
	waitGRPCHealthCmd.Flags().DurationVar(&waitGRPCDialTimeout, waitFlagDialTimeout, 5*time.Second, "gRPC dial timeout per attempt")
	waitGRPCHealthCmd.Flags().DurationVar(&waitGRPCRequestTimeout, waitFlagRequestTimeout, 5*time.Second, "gRPC health request timeout")

	waitK8SPodCmd.Flags().StringVar(&waitK8SPodName, "name", "", "Pod name to check")
	waitK8SPodCmd.Flags().StringVar(&waitK8SSelector, "selector", "", "Label selector for pod set (e.g. app=my-api)")
	waitK8SPodCmd.Flags().StringVar(&waitK8SNamespace, "k8s-namespace", "", "Kubernetes namespace (defaults to --namespace)")
	waitK8SPodCmd.Flags().StringVar(&waitK8SKubeconfig, "k8s-kubeconfig", "", "Path to kubeconfig for kubectl")
	waitK8SPodCmd.Flags().StringVar(&waitK8SContext, "k8s-context", "", "Kubernetes context for kubectl")
	waitK8SPodCmd.Flags().IntVar(&waitK8SMinReady, "min-ready", 1, "Minimum ready pods required")
	waitK8SPodCmd.Flags().StringVar(&waitK8SKubectl, "kubectl-binary", "kubectl", "kubectl binary path")

	waitCustomCmd.Flags().IntVar(&waitCustomExpectedExitCode, "expect-exit-code", 0, "Expected command exit code")
	waitCustomCmd.Flags().StringVar(&waitCustomWorkingDir, "working-dir", "", "Working directory for command execution")

	waitMySQLCmd.Flags().StringVar(&waitMySQLExpectedTable, "expect-table", "", "Expected MySQL table name")
	waitMySQLCmd.Flags().DurationVar(&waitMySQLConnectTimeout, waitFlagConnectTimeout, 5*time.Second, "MySQL connect timeout")

	waitPostgreSQLCmd.Flags().StringVar(&waitPostgresExpectedTable, "expect-table", "", "Expected PostgreSQL table name in current schemas")
	waitPostgreSQLCmd.Flags().DurationVar(&waitPostgresConnectTimeout, waitFlagConnectTimeout, 5*time.Second, "PostgreSQL connect timeout")

	waitMongoDBCmd.Flags().DurationVar(&waitMongoConnectTimeout, waitFlagConnectTimeout, 5*time.Second, "MongoDB connect timeout")

	waitRedisCmd.Flags().StringVar(&waitRedisExpectedKey, "expect-key", "", "Expected Redis key to exist")
	waitRedisCmd.Flags().StringVar(&waitRedisExpectedValueRegex, "expect-value-regex", "", "Regex expected to match Redis key value")
	waitRedisCmd.Flags().DurationVar(&waitRedisDialTimeout, waitFlagDialTimeout, 5*time.Second, "Redis dial timeout")
	waitRedisCmd.Flags().DurationVar(&waitRedisReadTimeout, "read-timeout", 5*time.Second, "Redis read timeout")

	waitRabbitMQCmd.Flags().DurationVar(&waitRabbitDialTimeout, waitFlagDialTimeout, 5*time.Second, "RabbitMQ dial timeout")
	waitKafkaCmd.Flags().DurationVar(&waitKafkaDialTimeout, waitFlagDialTimeout, 5*time.Second, "Kafka broker dial timeout")
	waitInfluxDBCmd.Flags().DurationVar(&waitInfluxReqTimeout, waitFlagRequestTimeout, 5*time.Second, "InfluxDB request timeout")
	waitInfluxDBCmd.Flags().IntVar(&waitInfluxExpectStatus, "expect-status-code", 0, "Expected InfluxDB status code (0 accepts any 2xx)")
	waitTemporalCmd.Flags().DurationVar(&waitTemporalDialTimeout, waitFlagDialTimeout, 5*time.Second, "Temporal server dial timeout")

	waitExternalCmd.Flags().StringVar(&waitExternalBinary, "binary", "wait4x", "wait4x binary path")

	waitCmd.AddCommand(waitTCPCmd)
	waitCmd.AddCommand(waitDNSCmd)
	waitCmd.AddCommand(waitHTTPCmd)
	waitCmd.AddCommand(waitGRPCHealthCmd)
	waitCmd.AddCommand(waitK8SPodCmd)
	waitCmd.AddCommand(waitMySQLCmd)
	waitCmd.AddCommand(waitPostgreSQLCmd)
	waitCmd.AddCommand(waitMongoDBCmd)
	waitCmd.AddCommand(waitRedisCmd)
	waitCmd.AddCommand(waitRabbitMQCmd)
	waitCmd.AddCommand(waitKafkaCmd)
	waitCmd.AddCommand(waitInfluxDBCmd)
	waitCmd.AddCommand(waitTemporalCmd)
	waitCmd.AddCommand(waitCustomCmd)
	waitCmd.AddCommand(waitExternalCmd)
}
