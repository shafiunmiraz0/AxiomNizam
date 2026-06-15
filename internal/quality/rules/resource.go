package rules

// =====================================================
// WS-2.1 — Data Quality Rules as declarative resources
//
// QualityRuleResource defines a data quality check that runs on a
// schedule against a catalog asset. The reconciler evaluates the rule
// and records pass/fail results in the status.
//
// QualityCheckResource records individual check execution results
// for historical tracking and trend analysis.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// --- Constants ---

const (
	QualityRuleKind       = "QualityRule"
	QualityRuleAPIVersion = "quality.axiomnizam.io/v1"

	QualityCheckKind       = "QualityCheck"
	QualityCheckAPIVersion = "quality.axiomnizam.io/v1"
)

// --- Rule Types ---

type QualityRuleType string

const (
	RuleTypeFreshness      QualityRuleType = "freshness"
	RuleTypeVolume         QualityRuleType = "volume"
	RuleTypeSchema         QualityRuleType = "schema"
	RuleTypeNotNull        QualityRuleType = "not_null"
	RuleTypeUnique         QualityRuleType = "unique"
	RuleTypeAcceptedValues QualityRuleType = "accepted_values"
	RuleTypeRange          QualityRuleType = "range"
	RuleTypeRegex          QualityRuleType = "regex"
	RuleTypeReferential    QualityRuleType = "referential"
	RuleTypeCustomSQL      QualityRuleType = "custom_sql"
	RuleTypeStatistical    QualityRuleType = "statistical"
	RuleTypeRowCountChange QualityRuleType = "row_count_change"
	RuleTypeCompleteness   QualityRuleType = "completeness"
	RuleTypeDistribution   QualityRuleType = "distribution"
	RuleTypeTimeliness     QualityRuleType = "timeliness"
)

// --- Check Result ---

type CheckResult string

const (
	CheckResultPass    CheckResult = "pass"
	CheckResultFail    CheckResult = "fail"
	CheckResultError   CheckResult = "error"
	CheckResultSkip    CheckResult = "skip"
	CheckResultWarning CheckResult = "warning"
)

// --- Rule-specific configs ---

type FreshnessRule struct {
	MaxAge          string `json:"maxAge"`          // Duration: "2h", "24h", "7d"
	TimestampColumn string `json:"timestampColumn"` // Column to check for freshness
}

type VolumeRule struct {
	MinRows    int64  `json:"minRows,omitempty"`
	MaxRows    int64  `json:"maxRows,omitempty"`
	GrowthRate string `json:"growthRate,omitempty"` // Max deviation: "5%", "10%"
}

type SchemaRule struct {
	ExpectedColumns  []ExpectedColumn `json:"expectedColumns"`
	AllowExtraColumns bool            `json:"allowExtraColumns"`
}

type ExpectedColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Nullable *bool  `json:"nullable,omitempty"`
	Required bool   `json:"required"`
}

type NotNullRule struct {
	Column    string  `json:"column"`
	Threshold float64 `json:"threshold,omitempty"` // Max null percentage (0-1)
}

type UniqueRule struct {
	Column    string  `json:"column"`
	Threshold float64 `json:"threshold,omitempty"` // Min uniqueness ratio (0-1)
}

type AcceptedValuesRule struct {
	Column string   `json:"column"`
	Values []string `json:"values"`
}

type RangeRule struct {
	Column   string   `json:"column"`
	MinValue *float64 `json:"minValue,omitempty"`
	MaxValue *float64 `json:"maxValue,omitempty"`
}

type RegexRule struct {
	Column  string `json:"column"`
	Pattern string `json:"pattern"`
	Negate  bool   `json:"negate,omitempty"` // True = must NOT match
}

type ReferentialRule struct {
	Column         string `json:"column"`
	ReferenceAsset string `json:"referenceAsset"` // Target catalog asset
	ReferenceColumn string `json:"referenceColumn"`
}

type CustomSQLRule struct {
	Query     string `json:"query"`     // Must return 0 rows to pass
	Threshold int64  `json:"threshold"` // Max allowed failing rows
}

type StatisticalRule struct {
	Column       string  `json:"column"`
	Metric       string  `json:"metric"`       // mean, stddev, median, p95, p99
	MinValue     float64 `json:"minValue,omitempty"`
	MaxValue     float64 `json:"maxValue,omitempty"`
	DeviationPct float64 `json:"deviationPct,omitempty"` // Max % deviation from historical
}

type CompletenessRule struct {
	Column    string  `json:"column"`
	Threshold float64 `json:"threshold"` // Min completeness ratio (0-1)
}

type RowCountChangeRule struct {
	MaxChangePct  float64 `json:"maxChangePct"`  // Max allowed % change
	PreviousCount int64   `json:"previousCount"` // Previous row count (updated by reconciler)
}

type DistributionRule struct {
	Column                    string  `json:"column"`
	MaxCoefficientOfVariation float64 `json:"maxCoefficientOfVariation,omitempty"` // Max CV (stddev/mean), default 2.0
}

type TimelinessRule struct {
	MaxDelay        string `json:"maxDelay"`        // Max acceptable delay: "5m", "1h"
	TimestampColumn string `json:"timestampColumn"`
}

// --- SLA ---

type QualitySLA struct {
	MinPassRate    float64 `json:"minPassRate"`    // 0-1, e.g. 0.99 = 99%
	MaxConsecutiveFails int `json:"maxConsecutiveFails"`
	AlertOnBreach  bool    `json:"alertOnBreach"`
}

// --- QualityRuleSpec ---

type QualityRuleSpec struct {
	// AssetRef references the CatalogAsset this rule applies to
	AssetRef string `json:"assetRef"`

	// DataSourceRef references the DataSource to query
	DataSourceRef string `json:"dataSourceRef"`

	// RuleType is the type of quality check
	RuleType QualityRuleType `json:"ruleType"`

	// Schedule is a cron expression for when to run
	Schedule string `json:"schedule,omitempty"`

	// Interval is a simpler alternative to cron (e.g. "1h", "6h")
	Interval string `json:"interval,omitempty"`

	// Severity: critical, warning, info
	Severity string `json:"severity"`

	// Description of what this rule checks
	Description string `json:"description,omitempty"`

	// Rule-type-specific configuration (only one should be set)
	Freshness      *FreshnessRule      `json:"freshness,omitempty"`
	Volume         *VolumeRule         `json:"volume,omitempty"`
	Schema         *SchemaRule         `json:"schema,omitempty"`
	NotNull        *NotNullRule        `json:"notNull,omitempty"`
	Unique         *UniqueRule         `json:"unique,omitempty"`
	AcceptedValues *AcceptedValuesRule `json:"acceptedValues,omitempty"`
	Range          *RangeRule          `json:"range,omitempty"`
	Regex          *RegexRule          `json:"regex,omitempty"`
	Referential    *ReferentialRule    `json:"referential,omitempty"`
	CustomSQL      *CustomSQLRule      `json:"customSQL,omitempty"`
	Statistical    *StatisticalRule    `json:"statistical,omitempty"`
	Completeness   *CompletenessRule   `json:"completeness,omitempty"`
	RowCountChange *RowCountChangeRule `json:"rowCountChange,omitempty"`
	Distribution   *DistributionRule   `json:"distribution,omitempty"`
	Timeliness     *TimelinessRule     `json:"timeliness,omitempty"`

	// Alerting
	AlertOnFailure bool     `json:"alertOnFailure"`
	AlertChannels  []string `json:"alertChannels,omitempty"`

	// SLA
	SLA *QualitySLA `json:"sla,omitempty"`

	// Enabled controls whether this rule is active
	Enabled bool `json:"enabled"`

	// Tags for organization
	Tags []string `json:"tags,omitempty"`
}

// --- QualityRuleResourceStatus ---

type QualityRuleResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	// LastCheckAt is when the rule was last evaluated
	LastCheckAt *time.Time `json:"lastCheckAt,omitempty"`

	// LastResult is the most recent check result
	LastResult CheckResult `json:"lastResult"`

	// LastFailureMessage describes why the last check failed
	LastFailureMessage string `json:"lastFailureMessage,omitempty"`

	// ConsecutiveFails counts sequential failures
	ConsecutiveFails int `json:"consecutiveFails"`

	// TotalChecks is the total number of evaluations
	TotalChecks int64 `json:"totalChecks"`

	// TotalPasses is the total number of passes
	TotalPasses int64 `json:"totalPasses"`

	// TotalFailures is the total number of failures
	TotalFailures int64 `json:"totalFailures"`

	// PassRate is the historical pass rate (0-1)
	PassRate float64 `json:"passRate"`

	// NextCheckAt is when the next evaluation is scheduled
	NextCheckAt *time.Time `json:"nextCheckAt,omitempty"`

	// SLAStatus: met, at_risk, breached
	SLAStatus string `json:"slaStatus,omitempty"`

	// LastCheckDuration is how long the last check took
	LastCheckDuration string `json:"lastCheckDuration,omitempty"`

	// FailingRows is the count of rows that failed (for SQL-based checks)
	FailingRows int64 `json:"failingRows,omitempty"`
}

// --- QualityRuleResource ---

type QualityRuleResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   QualityRuleSpec           `json:"spec"`
	Status QualityRuleResourceStatus `json:"status"`
}

// --- resources.Resource implementation ---

func (r *QualityRuleResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *QualityRuleResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *QualityRuleResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *QualityRuleResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *QualityRuleResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Tags) > 0 {
		cp.Spec.Tags = make([]string, len(r.Spec.Tags))
		copy(cp.Spec.Tags, r.Spec.Tags)
	}
	return &cp
}

// --- reconciler.Resource implementation ---

func (r *QualityRuleResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *QualityRuleResource) GetGeneration() int64         { return r.Generation }
func (r *QualityRuleResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// =====================================================
// QualityCheckResource — individual check execution record
// =====================================================

type QualityCheckSpec struct {
	RuleRef   string      `json:"ruleRef"`   // Reference to QualityRuleResource
	AssetRef  string      `json:"assetRef"`  // Reference to CatalogAsset
	Result    CheckResult `json:"result"`
	Message   string      `json:"message,omitempty"`
	Duration  string      `json:"duration"`
	FailCount int64       `json:"failCount,omitempty"`
	TotalRows int64       `json:"totalRows,omitempty"`
	CheckedAt time.Time   `json:"checkedAt"`
}

type QualityCheckResourceStatus struct {
	resources.ObjectStatus `json:",inline"`
}

type QualityCheckResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   QualityCheckSpec           `json:"spec"`
	Status QualityCheckResourceStatus `json:"status"`
}

func (r *QualityCheckResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *QualityCheckResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *QualityCheckResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *QualityCheckResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *QualityCheckResource) DeepCopy() resources.Resource {
	cp := *r
	return &cp
}

func (r *QualityCheckResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *QualityCheckResource) GetGeneration() int64         { return r.Generation }
func (r *QualityCheckResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
