package apibanks

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apibanks/audit"
	"axiomnizam.bitbd.net/axiomnizam/internal/apibanks/metrics"
)

// --- Bank DTOs ---

// APIBankListItem is a typed entry in the bank list.
type APIBankListItem struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Description string            `json:"description"`
	Owner       string            `json:"owner"`
	Version     string            `json:"version"`
	APICount    int               `json:"api_count"`
	Tags        []string          `json:"tags,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// APIBankListResponse is the typed response for GET /api/v1/apibanks.
type APIBankListResponse struct {
	Status string            `json:"status"`
	Banks  []APIBankListItem `json:"banks"`
	Total  int               `json:"total"`
}

// APIBankResponse is the typed response for a single bank.
type APIBankResponse struct {
	Status string   `json:"status"`
	Bank   *APIBank `json:"bank"`
}

// --- API DTOs ---

// APIListResponse is the typed response for listing APIs in a bank.
type APIListResponse struct {
	Status string         `json:"status"`
	APIs   []APIReference `json:"apis"`
	Total  int            `json:"total"`
}

// --- Catalog DTOs ---

// APIBankCatalogResponse is the typed response for catalog queries.
type APIBankCatalogResponse struct {
	Status string         `json:"status"`
	APIs   []APIReference `json:"apis"`
	Total  int            `json:"total"`
}

// APIBankGroupedResponse groups banks by a field.
type APIBankGroupedResponse struct {
	Status  string              `json:"status"`
	Banks   map[string][]*APIBank `json:"banks"`
	Total   int                 `json:"total"`
}

// --- Metrics DTOs ---

// APIBankMetricsResponse is the typed response for GET /api/v1/apibanks/metrics.
type APIBankMetricsResponse struct {
	Status  string         `json:"status"`
	Metrics metrics.Snapshot `json:"metrics"`
}

// --- Audit DTOs ---

// APIBankAuditListResponse is the typed response for GET /api/v1/apibanks/audit.
type APIBankAuditListResponse struct {
	Status string        `json:"status"`
	Events []audit.Event `json:"events"`
	Total  int           `json:"total"`
}

// --- Generic DTO ---

// APIBankMessageResponse is a generic ack/error response.
type APIBankMessageResponse struct {
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}
