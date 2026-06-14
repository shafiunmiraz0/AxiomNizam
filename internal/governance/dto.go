package governance

import "axiomnizam.bitbd.net/axiomnizam/internal/governance/models"

// MessageResponse is a generic error/ack response.
type MessageResponse struct {
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Name    string `json:"name,omitempty"`
}

// PolicyListResponse is the response for ListPolicies.
type PolicyListResponse struct {
	Policies []*models.CompliancePolicyResource `json:"policies"`
	Count    int                         `json:"count"`
}

// RetentionPolicyListResponse is the response for ListRetentionPolicies.
type RetentionPolicyListResponse struct {
	RetentionPolicies []*models.RetentionPolicyResource `json:"retentionPolicies"`
	Count             int                        `json:"count"`
}

// AccessRequestListResponse is the response for ListAccessRequests.
type AccessRequestListResponse struct {
	AccessRequests []*models.AccessRequestResource `json:"accessRequests"`
	Count          int                      `json:"count"`
}

// GovernanceSummaryResponse is the response for GetSummary.
type GovernanceSummaryResponse struct {
	TotalPolicies         int     `json:"totalPolicies"`
	CompliantPolicies     int     `json:"compliantPolicies"`
	NonCompliantPolicies  int     `json:"nonCompliantPolicies"`
	TotalViolations       int     `json:"totalViolations"`
	AvgComplianceScore    float64 `json:"avgComplianceScore"`
	PendingAccessRequests int     `json:"pendingAccessRequests"`
	ActiveAccessGrants    int     `json:"activeAccessGrants"`
}
