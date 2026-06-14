package models

// =====================================================
// WS-4.4 — Cost Attribution domain Resource types
//
// CostPolicyResource defines per-tenant cost policies with quotas,
// rate cards, and budget alerts. UsageRecordResource tracks actual
// resource consumption for chargeback and billing.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// --- Constants ---

const (
	CostPolicyKind       = "CostPolicy"
	CostPolicyAPIVersion = "costing.axiomnizam.io/v1"

	UsageRecordKind       = "UsageRecord"
	UsageRecordAPIVersion = "costing.axiomnizam.io/v1"
)

// --- Usage Dimensions ---

type UsageDimension string

const (
	DimensionQuery    UsageDimension = "query"
	DimensionPipeline UsageDimension = "pipeline"
	DimensionStorage  UsageDimension = "storage"
	DimensionAPI      UsageDimension = "api"
	DimensionCDC      UsageDimension = "cdc"
	DimensionCompute  UsageDimension = "compute"
)

// --- Quota ---

type Quota struct {
	Limit        float64 `json:"limit"`
	Action       string  `json:"action"`       // warn, throttle, block
	CurrentUsage float64 `json:"currentUsage"`
}

// --- Rate Card ---

type RateCard struct {
	QueryCreditPerRow    float64 `json:"queryCreditPerRow,omitempty"`
	PipelineCreditPerRun float64 `json:"pipelineCreditPerRun,omitempty"`
	StorageCreditPerGB   float64 `json:"storageCreditPerGB,omitempty"`
	APICreditPer1000     float64 `json:"apiCreditPer1000,omitempty"`
	CDCCreditPerMillion  float64 `json:"cdcCreditPerMillion,omitempty"`
}

// --- Cost Alert ---

type CostAlert struct {
	ThresholdPercent float64  `json:"thresholdPercent"` // 0.8 = 80% of quota
	Channels         []string `json:"channels,omitempty"`
	Message          string   `json:"message,omitempty"`
}

// --- CostPolicySpec ---

type CostPolicySpec struct {
	// TenantID is the tenant this policy applies to
	TenantID string `json:"tenantId"`

	// BillingPeriod: monthly, weekly, daily
	BillingPeriod string `json:"billingPeriod"`

	// Quotas per dimension
	Quotas map[string]Quota `json:"quotas,omitempty"`

	// Alerts for budget thresholds
	Alerts []CostAlert `json:"alerts,omitempty"`

	// RateCard defines pricing per unit
	RateCard RateCard `json:"rateCard"`

	// Enabled controls whether cost tracking is active
	Enabled bool `json:"enabled"`
}

// --- CostPolicyResourceStatus ---

type CostPolicyResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	// CurrentPeriodStart is the start of the current billing period
	CurrentPeriodStart *time.Time `json:"currentPeriodStart,omitempty"`

	// CurrentPeriodEnd is the end of the current billing period
	CurrentPeriodEnd *time.Time `json:"currentPeriodEnd,omitempty"`

	// TotalCreditsUsed is the total credits consumed this period
	TotalCreditsUsed float64 `json:"totalCreditsUsed"`

	// TotalCreditsLimit is the total credit limit for this period
	TotalCreditsLimit float64 `json:"totalCreditsLimit"`

	// UsageByDimension breaks down usage per dimension
	UsageByDimension map[string]float64 `json:"usageByDimension,omitempty"`

	// QuotaBreaches lists dimensions that have exceeded their quota
	QuotaBreaches []string `json:"quotaBreaches,omitempty"`

	// LastAggregatedAt is when usage was last aggregated
	LastAggregatedAt *time.Time `json:"lastAggregatedAt,omitempty"`
}

// --- CostPolicyResource ---

type CostPolicyResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   CostPolicySpec           `json:"spec"`
	Status CostPolicyResourceStatus `json:"status"`
}

func (r *CostPolicyResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *CostPolicyResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *CostPolicyResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *CostPolicyResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *CostPolicyResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Quotas) > 0 {
		cp.Spec.Quotas = make(map[string]Quota, len(r.Spec.Quotas))
		for k, v := range r.Spec.Quotas {
			cp.Spec.Quotas[k] = v
		}
	}
	return &cp
}
func (r *CostPolicyResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *CostPolicyResource) GetGeneration() int64         { return r.Generation }
func (r *CostPolicyResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// =====================================================
// UsageRecordResource -- individual usage records
// =====================================================

// UsageRecordSpec captures a single usage event.
type UsageRecordSpec struct {
	TenantID  string            `json:"tenantId"`
	Dimension UsageDimension    `json:"dimension"`
	Quantity  float64           `json:"quantity"`  // Raw quantity (rows, bytes, requests)
	Credits   float64           `json:"credits"`   // Computed credits based on rate card
	Source    string            `json:"source"`    // What generated this usage (query ID, pipeline name)
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type UsageRecordResourceStatus struct {
	resources.ObjectStatus `json:",inline"`
	Aggregated bool `json:"aggregated"` // Whether this record has been rolled up
}

type UsageRecordResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   UsageRecordSpec           `json:"spec"`
	Status UsageRecordResourceStatus `json:"status"`
}

func (r *UsageRecordResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *UsageRecordResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *UsageRecordResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *UsageRecordResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *UsageRecordResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *UsageRecordResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *UsageRecordResource) GetGeneration() int64         { return r.Generation }
func (r *UsageRecordResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
