package netintel

// MessageResponse is a generic error/ack response.
type MessageResponse struct {
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Name    string `json:"name,omitempty"`
}

// --- NetIntel Response DTOs ---

type SummaryResponse struct {
	Status  string      `json:"status"`
	Summary interface{} `json:"summary"`
}

type ObservabilityResponse struct {
	Status        string      `json:"status"`
	Observability interface{} `json:"observability"`
}

type LogTypesResponse struct {
	Status   string      `json:"status"`
	LogTypes interface{} `json:"log_types"`
	Total    int         `json:"total"`
}

type ParserListResponse struct {
	Status  string      `json:"status"`
	Parsers interface{} `json:"parsers"`
	Total   int         `json:"total"`
}

type ParserResponse struct {
	Status string      `json:"status"`
	Parser interface{} `json:"parser"`
}

type EntryListResponse struct {
	Status  string      `json:"status"`
	Entries interface{} `json:"entries"`
	Total   int         `json:"total"`
}

type StatsResponse struct {
	Status string      `json:"status"`
	Stats  interface{} `json:"stats"`
}

type TopologyResponse struct {
	Status   string      `json:"status"`
	Topology interface{} `json:"topology"`
}

type TopologyNodeResponse struct {
	Status string      `json:"status"`
	Node   interface{} `json:"node"`
}

type HeatmapResponse struct {
	Status  string      `json:"status"`
	Heatmap interface{} `json:"heatmap"`
}

type TrendsResponse struct {
	Status string      `json:"status"`
	Metric string      `json:"metric"`
	Hours  int         `json:"hours"`
	Trend  interface{} `json:"trend"`
}

type PredictionsResponse struct {
	Status      string      `json:"status"`
	Predictions interface{} `json:"predictions"`
	Total       int         `json:"total"`
}

type TrackListResponse struct {
	Status string      `json:"status"`
	Tracks interface{} `json:"tracks"`
	Total  int         `json:"total"`
}

type TrackResponse struct {
	Status string      `json:"status"`
	Track  interface{} `json:"track"`
}

type AnomalyListResponse struct {
	Status    string      `json:"status"`
	Anomalies interface{} `json:"anomalies"`
	Total     int         `json:"total"`
}

type AlertListResponse struct {
	Status string      `json:"status"`
	Alerts interface{} `json:"alerts"`
	Total  int         `json:"total"`
}

type ForecastListResponse struct {
	Status    string      `json:"status"`
	Forecasts interface{} `json:"forecasts"`
}

type ForecastResponse struct {
	Status   string      `json:"status"`
	Forecast interface{} `json:"forecast"`
}

// HealthResponse is the module health check response.
type HealthResponse struct {
	Status        string `json:"status"`
	UptimeSec     int64  `json:"uptime_seconds"`
	TotalIngested int64  `json:"total_ingested"`
	Module        string `json:"module"`
}

// MetricsEndpointResponse is the module metrics snapshot response.
type MetricsEndpointResponse struct {
	TotalIngested  int64            `json:"total_ingested"`
	TotalAnomalies int64            `json:"total_anomalies"`
	TotalAlerts    int64            `json:"total_alerts"`
	UptimeSeconds  int64            `json:"uptime_seconds"`
	ByLogType      map[string]int64 `json:"by_log_type,omitempty"`
	BySeverity     map[string]int64 `json:"by_severity,omitempty"`
}

// AuditLogResponse is the audit log response.
type AuditLogResponse struct {
	Events interface{} `json:"events"`
	Count  int         `json:"count"`
}

// --- WiFi Scan Response DTOs ---

// WiFiScanResponse is the response for a WiFi scan trigger.
type WiFiScanResponse struct {
	Status string      `json:"status"`
	Result interface{} `json:"result"`
}

// WiFiNetworksResponse is the response for listing cached WiFi networks.
type WiFiNetworksResponse struct {
	Status   string      `json:"status"`
	Networks interface{} `json:"networks"`
	Total    int         `json:"total"`
}

// WiFiHistoryResponse is the response for WiFi scan history.
type WiFiHistoryResponse struct {
	Status  string      `json:"status"`
	History interface{} `json:"history"`
	Total   int         `json:"total"`
}

// --- Modes Response DTOs ---

// ModesListResponse is the modes list response.
type ModesListResponse struct {
	Status string      `json:"status"`
	Modes  interface{} `json:"modes"`
}

// ModesUpsertResponse is the mode upsert response.
type ModesUpsertResponse struct {
	Message string      `json:"message"`
	Mode    interface{} `json:"mode"`
}

// ModesEventsResponse is the mode events list response.
type ModesEventsResponse struct {
	Status string      `json:"status"`
	Events interface{} `json:"events"`
}

// ModesDetectResponse is the anomaly detector response.
type ModesDetectResponse struct {
	Detector int     `json:"detector"`
	Score    float64 `json:"score"`
}
