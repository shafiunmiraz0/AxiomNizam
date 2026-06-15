package models

// =====================================================
// WS-4.3 — SLO/SLA Tracking as declarative resources
//
// SLOResource defines a Service Level Objective with error budget
// tracking. The reconciler periodically evaluates the SLI (Service
// Level Indicator) and calculates burn rate, remaining budget, and
// time-to-exhaust.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// --- Constants ---

const (
	SLOKind       = "SLO"
	SLOAPIVersion = "slo.axiomnizam.io/v1"
)

// --- SLI Types ---

type SLIType string

const (
	SLITypeAvailability SLIType = "availability"
	SLITypeLatency      SLIType = "latency"
	SLITypeQuality      SLIType = "quality"
	SLITypeFreshness    SLIType = "freshness"
	SLITypeThroughput   SLIType = "throughput"
)

// --- SLI Spec ---

type SLISpec struct {
	// Type of service level indicator
	Type SLIType `json:"type"`

	// GoodQuery is the metric query for good events (numerator)
	GoodQuery string `json:"goodQuery"`

	// TotalQuery is the metric query for total events (denominator)
	TotalQuery string `json:"totalQuery"`

	// ThresholdMs is the latency threshold for latency SLIs
	ThresholdMs int64 `json:"thresholdMs,omitempty"`
}

// --- Burn Rate Alert ---

type BurnRateAlert struct {
	// BurnRate threshold that triggers the alert (e.g. 14.4 = 14.4x normal)
	BurnRate float64 `json:"burnRate"`

	// LookbackWindow is how far back to measure burn rate
	LookbackWindow string `json:"lookbackWindow"` // "1h", "6h"

	// Severity of the alert
	Severity string `json:"severity"` // critical, warning

	// Channels to notify
	Channels []string `json:"channels,omitempty"`
}

// --- SLOSpec ---

type SLOSpec struct {
	// DisplayName is the human-readable SLO name
	DisplayName string `json:"displayName" binding:"required"`

	// Description of what this SLO measures
	Description string `json:"description,omitempty"`

	// Service is what service/component this SLO covers
	Service string `json:"service" binding:"required"`

	// Target is the SLO target (e.g. 0.999 = 99.9%)
	Target float64 `json:"target" binding:"required,gt=0,lt=1"`

	// Window is the evaluation window (e.g. "30d", "7d")
	Window string `json:"window" binding:"required"`

	// Indicator defines how to measure the SLI
	Indicator SLISpec `json:"indicator"`

	// BurnRateAlerts defines alerts based on error budget burn rate
	BurnRateAlerts []BurnRateAlert `json:"burnRateAlerts,omitempty"`

	// EvalInterval is how often to re-evaluate (default: "1m")
	EvalInterval string `json:"evalInterval,omitempty"`

	// Owner is the team responsible for this SLO
	Owner string `json:"owner,omitempty"`

	// Labels for grouping and filtering
	Labels map[string]string `json:"labels,omitempty"`
}

// --- SLOResourceStatus ---

type SLOResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	// CurrentSLI is the current measured SLI value (0-1)
	CurrentSLI float64 `json:"currentSli"`

	// ErrorBudget is the remaining error budget (0-1, where 1 = full budget)
	ErrorBudget float64 `json:"errorBudget"`

	// BudgetConsumed is how much budget has been used (0-1)
	BudgetConsumed float64 `json:"budgetConsumed"`

	// BurnRate is the current burn rate (1.0 = normal, >1 = burning faster)
	BurnRate float64 `json:"burnRate"`

	// IsBreaching indicates if the SLO is currently being breached
	IsBreaching bool `json:"isBreaching"`

	// TimeToExhaust is the estimated time until budget is exhausted at current burn rate
	TimeToExhaust string `json:"timeToExhaust,omitempty"`

	// WindowStart is the start of the current evaluation window
	WindowStart *time.Time `json:"windowStart,omitempty"`

	// WindowEnd is the end of the current evaluation window
	WindowEnd *time.Time `json:"windowEnd,omitempty"`

	// GoodEvents is the count of good events in the window
	GoodEvents int64 `json:"goodEvents"`

	// TotalEvents is the count of total events in the window
	TotalEvents int64 `json:"totalEvents"`

	// LastEvaluatedAt is when the SLO was last evaluated
	LastEvaluatedAt *time.Time `json:"lastEvaluatedAt,omitempty"`
}

// --- SLOResource ---

type SLOResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   SLOSpec           `json:"spec"`
	Status SLOResourceStatus `json:"status"`
}

// --- resources.Resource implementation ---

func (r *SLOResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *SLOResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *SLOResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *SLOResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *SLOResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.BurnRateAlerts) > 0 {
		cp.Spec.BurnRateAlerts = make([]BurnRateAlert, len(r.Spec.BurnRateAlerts))
		copy(cp.Spec.BurnRateAlerts, r.Spec.BurnRateAlerts)
	}
	return &cp
}
func (r *SLOResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *SLOResource) GetGeneration() int64         { return r.Generation }
func (r *SLOResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
