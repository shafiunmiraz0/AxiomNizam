package models

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// =====================================================
// P2 resource-ification — APIScan.
//
// APIScanResource is a declarative *scan request* resource.  The
// reconciler turns Spec (endpoint + options) into a ScanRequest,
// invokes the scanner, and records findings / summary on Status.
// This makes ad-hoc security scans a first-class platform resource
// with visible history, generation tracking, and retry semantics.
// =====================================================

const (
	APIScanKind       = "APIScan"
	APIScanAPIVersion = "apiscanner.axiomnizam.io/v1"
)

// --- Output format ---

type OutputFormat string

const (
	FormatTable OutputFormat = "table"
	FormatJSON  OutputFormat = "json"
)

// --- Severity constants ---

const (
	SeverityCritical = "CRITICAL"
	SeverityHigh     = "HIGH"
	SeverityMedium   = "MEDIUM"
	SeverityLow      = "LOW"
	SeverityInfo     = "INFO"
)

// --- Vulnerability types ---

type VulnerabilityType string

const (
	VulnAuthBypass      VulnerabilityType = "auth_bypass"
	VulnSQLInjection    VulnerabilityType = "sql_injection"
	VulnNoSQLInjection  VulnerabilityType = "nosql_injection"
	VulnHTTPMethod      VulnerabilityType = "http_method_validation"
	VulnSecurityHeaders VulnerabilityType = "security_headers"
	VulnParameterTamper VulnerabilityType = "parameter_tampering"
	VulnXSS             VulnerabilityType = "xss"
)

// --- Endpoint ---

type Endpoint struct {
	URL     string
	Method  string
	Body    string
	Headers map[string]string
}

// --- Scan request ---

type ScanRequest struct {
	Endpoint           Endpoint
	Timeout            time.Duration
	RetryCount         int
	RetryBackoff       time.Duration
	InsecureSkipVerify bool
	AuthHeader         string
	AuthValue          string
	Format             OutputFormat
}

// --- Scan result ---

type ScanResult struct {
	Scanner   string            `json:"scanner"`
	Target    string            `json:"target"`
	Method    string            `json:"method"`
	ScannedAt time.Time         `json:"scannedAt"`
	Findings  []Finding         `json:"findings"`
	Checks    []ScanCheckStatus `json:"checks,omitempty"`
	Summary   Summary           `json:"summary"`
}

// --- Check status ---

const (
	CheckAuthBypassDetection = "auth_bypass_detection"
	CheckAuthBypassTesting   = "auth_bypass_testing"
	CheckSQLInjection        = "sql_injection"
	CheckNoSQLInjection      = "nosql_injection"
	CheckHTTPMethod          = "http_method_validation"
	CheckSecurityHeaders     = "security_header_analysis"
	CheckParameterTampering  = "parameter_tampering"
	CheckXSS                 = "xss"
)

type ScanCheckStatus struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Executed bool   `json:"executed"`
	Findings int    `json:"findings"`
}

// --- Finding ---

type Finding struct {
	Type           VulnerabilityType `json:"type"`
	Severity       string            `json:"severity"`
	Title          string            `json:"title"`
	Description    string            `json:"description"`
	Endpoint       string            `json:"endpoint"`
	Method         string            `json:"method"`
	Evidence       string            `json:"evidence,omitempty"`
	Payload        string            `json:"payload,omitempty"`
	Recommendation string            `json:"recommendation,omitempty"`
}

// --- Summary ---

type Summary struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
}

// --- Spec ---

// APIScanSpec is the desired scan to execute.
type APIScanSpec struct {
	Endpoint           Endpoint      `json:"endpoint"`
	Timeout            time.Duration `json:"timeout,omitempty"`
	RetryCount         int           `json:"retryCount,omitempty"`
	RetryBackoff       time.Duration `json:"retryBackoff,omitempty"`
	InsecureSkipVerify bool          `json:"insecureSkipVerify,omitempty"`
	AuthHeader         string        `json:"authHeader,omitempty"`
	AuthValue          string        `json:"authValue,omitempty"`
	Format             OutputFormat  `json:"format,omitempty"`

	// RunOnce makes the scan execute a single time and then stay in
	// Completed state.  When false, the reconciler re-runs on a cadence
	// derived from timing.DefaultRequeueAfter.
	RunOnce bool `json:"runOnce,omitempty"`
}

// --- Resource status ---

// APIScanResourceStatus carries scan telemetry.
type APIScanResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	LastScanAt *time.Time  `json:"lastScanAt,omitempty"`
	LastResult *ScanResult `json:"lastResult,omitempty"`
	ScanCount  int         `json:"scanCount"`
	LastError  string      `json:"lastError,omitempty"`
}

// --- Resource ---

// APIScanResource is the declarative resource for an APIScan job.
type APIScanResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   APIScanSpec           `json:"spec"`
	Status APIScanResourceStatus `json:"status"`
}

// --- resources.Resource implementation ---

func (r *APIScanResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *APIScanResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *APIScanResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *APIScanResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *APIScanResource) DeepCopy() resources.Resource { cp := *r; return &cp }

// --- reconciler.Resource implementation ---

func (r *APIScanResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *APIScanResource) GetGeneration() int64         { return r.Generation }
func (r *APIScanResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
