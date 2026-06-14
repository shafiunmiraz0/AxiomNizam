package models

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// CheckType
// ─────────────────────────────────────────────────────────────────────────────

type CheckType string

const (
	CheckTypeTCP       CheckType = "tcp"
	CheckTypeHTTP      CheckType = "http"
	CheckTypeDNS       CheckType = "dns"
	CheckTypeGRPC      CheckType = "grpc"
	CheckTypeRedis     CheckType = "redis"
	CheckTypeMySQL     CheckType = "mysql"
	CheckTypePostgreSQL CheckType = "postgresql"
	CheckTypeMongoDB   CheckType = "mongodb"
	CheckTypeRabbitMQ  CheckType = "rabbitmq"
	CheckTypeKafka     CheckType = "kafka"
	CheckTypeInfluxDB  CheckType = "influxdb"
	CheckTypeTemporal  CheckType = "temporal"
	CheckTypeCommand   CheckType = "exec"
	CheckTypeK8sPod    CheckType = "k8s-pod"
)

// ─────────────────────────────────────────────────────────────────────────────
// CheckStatus
// ─────────────────────────────────────────────────────────────────────────────

type CheckStatus string

const (
	CheckStatusPending  CheckStatus = "pending"
	CheckStatusRunning  CheckStatus = "running"
	CheckStatusReady    CheckStatus = "ready"
	CheckStatusFailed   CheckStatus = "failed"
	CheckStatusTimeout  CheckStatus = "timeout"
	CheckStatusInverted CheckStatus = "inverted"
)

// ─────────────────────────────────────────────────────────────────────────────
// RetryPolicy
// ─────────────────────────────────────────────────────────────────────────────

type RetryPolicy string

const (
	RetryPolicyLinear      RetryPolicy = "linear"
	RetryPolicyExponential RetryPolicy = "exponential"
	RetryPolicyFibonacci   RetryPolicy = "fibonacci"
	RetryPolicyCustom      RetryPolicy = "custom"
)

// ─────────────────────────────────────────────────────────────────────────────
// WaitCheckResource — declarative wait condition
// ─────────────────────────────────────────────────────────────────────────────

type WaitCheckResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   WaitCheckSpec   `json:"spec"`
	Status WaitCheckStatus `json:"status"`
}

type WaitCheckSpec struct {
	CheckType    CheckType       `json:"checkType"`
	Target       string          `json:"target"`
	Timeout      string          `json:"timeout,omitempty"`
	Interval     string          `json:"interval,omitempty"`
	MaxInterval  string          `json:"maxInterval,omitempty"`
	RetryPolicy  RetryPolicy     `json:"retryPolicy,omitempty"`
	InvertCheck  bool            `json:"invertCheck,omitempty"`
	// Check-specific options
	Options      CheckOptions    `json:"options"`
}

type CheckOptions struct {
	// TCP
	DialTimeout string `json:"dialTimeout,omitempty"`

	// HTTP
	Method           string            `json:"method,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Body             string            `json:"body,omitempty"`
	ExpectStatusCode int               `json:"expectStatusCode,omitempty"`
	ExpectBodyRegex  string            `json:"expectBodyRegex,omitempty"`
	InsecureSkipTLS  bool              `json:"insecureSkipTLS,omitempty"`
	RequestTimeout   string            `json:"requestTimeout,omitempty"`

	// DNS
	RecordType     string   `json:"recordType,omitempty"`
	NameServer     string   `json:"nameServer,omitempty"`
	ExpectedValues []string `json:"expectedValues,omitempty"`

	// gRPC
	Service       string `json:"service,omitempty"`
	UseTLS        bool   `json:"useTLS,omitempty"`
	TLSServerName string `json:"tlsServerName,omitempty"`

	// Redis
	ExpectedKey        string `json:"expectedKey,omitempty"`
	ExpectedValueRegex string `json:"expectedValueRegex,omitempty"`

	// MySQL / PostgreSQL
	DSN           string `json:"dsn,omitempty"`
	ExpectedTable string `json:"expectedTable,omitempty"`

	// MongoDB
	URI string `json:"uri,omitempty"`

	// RabbitMQ
	URL string `json:"url,omitempty"`

	// Kafka
	Brokers []string `json:"brokers,omitempty"`

	// K8s
	PodName       string `json:"podName,omitempty"`
	LabelSelector string `json:"labelSelector,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	Kubeconfig    string `json:"kubeconfig,omitempty"`
	KubeContext   string `json:"kubeContext,omitempty"`
	MinReady      int    `json:"minReady,omitempty"`

	// Command
	Command          []string `json:"command,omitempty"`
	ExpectedExitCode int      `json:"expectedExitCode,omitempty"`
	WorkingDirectory string   `json:"workingDirectory,omitempty"`
}

type WaitCheckStatus struct {
	resources.ObjectStatus `json:",inline"`
	LastCheckAt   time.Time    `json:"lastCheckAt,omitempty"`  //nolint:omitzero
	LastResult    CheckStatus  `json:"lastResult,omitempty"`
	Attempts      int          `json:"attempts,omitempty"`
	LastError     string       `json:"lastError,omitempty"`
	TotalChecks   int64        `json:"totalChecks,omitempty"`
	TotalSuccesses int64       `json:"totalSuccesses,omitempty"`
	TotalFailures int64        `json:"totalFailures,omitempty"`
	AvgLatencyMs  float64      `json:"avgLatencyMs,omitempty"`
}

func (r *WaitCheckResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *WaitCheckResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *WaitCheckResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *WaitCheckResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *WaitCheckResource) DeepCopy() resources.Resource {
	out := *r
	return &out
}
func (r *WaitCheckResource) GetKey() string {
	return r.Namespace + "/" + r.Name
}
func (r *WaitCheckResource) GetGeneration() int64          { return r.ObjectMeta.Generation }
func (r *WaitCheckResource) GetObservedGeneration() int64  { return r.Status.ObservedGeneration }

// ─────────────────────────────────────────────────────────────────────────────
// CheckGroupResource — group of wait checks (all must pass)
// ─────────────────────────────────────────────────────────────────────────────

type CheckGroupResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec   CheckGroupSpec   `json:"spec"`
	Status CheckGroupStatus `json:"status"`
}

type CheckGroupSpec struct {
	Checks    []string `json:"checks"`    // References to WaitCheckResource names
	Parallel  bool     `json:"parallel"`  // Run checks in parallel
	MinReady  int      `json:"minReady"`  // Minimum checks that must pass (0 = all)
	Timeout   string   `json:"timeout"`   // Overall group timeout
}

type CheckGroupStatus struct {
	resources.ObjectStatus `json:",inline"`
	ReadyCount int    `json:"readyCount,omitempty"`
	TotalCount int    `json:"totalCount,omitempty"`
	GroupReady bool   `json:"groupReady,omitempty"`
}

func (r *CheckGroupResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *CheckGroupResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *CheckGroupResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *CheckGroupResource) SetStatus(s *resources.ObjectStatus)  { r.Status.ObjectStatus = *s }
func (r *CheckGroupResource) DeepCopy() resources.Resource {
	out := *r
	return &out
}
func (r *CheckGroupResource) GetKey() string {
	return r.Namespace + "/" + r.Name
}
func (r *CheckGroupResource) GetGeneration() int64          { return r.ObjectMeta.Generation }
func (r *CheckGroupResource) GetObservedGeneration() int64  { return r.Status.ObservedGeneration }
