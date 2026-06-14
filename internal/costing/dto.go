package costing

import "axiomnizam.bitbd.net/axiomnizam/internal/costing/models"

// MessageResponse is a generic error/ack response.
type MessageResponse struct {
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Name    string `json:"name,omitempty"`
}

// PolicyListResponse is the API response for listing cost policies.
type PolicyListResponse struct {
	Policies []*models.CostPolicyResource `json:"policies"`
	Count    int                   `json:"count"`
}

// UsageResponse is the API response for aggregated usage.
type UsageResponse struct {
	TotalCredits float64            `json:"totalCredits"`
	TotalRecords int                `json:"totalRecords"`
	ByDimension  map[string]float64 `json:"byDimension"`
	ByTenant     map[string]float64 `json:"byTenant"`
}

// TenantUsageResponse is the API response for tenant usage.
type TenantUsageResponse struct {
	Tenant       string             `json:"tenant"`
	TotalCredits float64            `json:"totalCredits"`
	Records      int                `json:"records"`
	ByDimension  map[string]float64 `json:"byDimension"`
}

// ReportResponse is the API response for cost report.
type ReportResponse struct {
	Report  []TenantReportLine `json:"report"`
	Tenants int                `json:"tenants"`
}

// TenantReportLine is a single tenant cost report entry.
type TenantReportLine struct {
	TenantID     string             `json:"tenantId"`
	TotalUsed    float64            `json:"totalUsed"`
	TotalLimit   float64            `json:"totalLimit"`
	UsagePercent float64            `json:"usagePercent"`
	OverQuota    bool               `json:"overQuota"`
	ByDimension  map[string]float64 `json:"byDimension"`
}
