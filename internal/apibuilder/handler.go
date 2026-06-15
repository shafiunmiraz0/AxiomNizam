package apibuilder

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/archivescan"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/macro"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/metadata"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/mimetype"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/native"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner/svg"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// =====================================================================
// API Builder Handler — GUI-driven API & resource creation,
// CSV-to-Dashboard ingestion, and Dashboard <-> GIS conversion
// =====================================================================

// ---------- API Builder Models ----------

// CustomAPI represents an API endpoint created through the GUI builder
type CustomAPI struct {
	ID             string            `json:"id"`
	APIType        string            `json:"api_type,omitempty"` // rest, graphql
	Name           string            `json:"name"`
	Method         string            `json:"method"` // GET, POST, PUT, DELETE
	Path           string            `json:"path"`
	SQLTemplate    string            `json:"sql_template,omitempty"`
	SQLPolicyMode  string            `json:"sql_policy_mode,omitempty"` // compat, strict
	GraphQLQuery   string            `json:"graphql_query,omitempty"`
	GraphQLOpName  string            `json:"graphql_operation_name,omitempty"`
	Description    string            `json:"description"`
	Category       string            `json:"category"`
	SourceDatabase string            `json:"source_database,omitempty"`
	SourceServer   string            `json:"source_server,omitempty"`
	AuthRequired   bool              `json:"auth_required"`
	RateLimit      int               `json:"rate_limit"` // requests per minute, 0=unlimited
	CacheEnabled   bool              `json:"cache_enabled"`
	CacheTTL       int               `json:"cache_ttl"` // seconds, 0=default(300)
	RequestSchema  *SchemaDefinition `json:"request_schema,omitempty"`
	ResponseSchema *SchemaDefinition `json:"response_schema,omitempty"`
	MockResponse   interface{}       `json:"mock_response,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	QueryParams    []ParamDef        `json:"query_params,omitempty"`
	Status         string            `json:"status"` // active, inactive, draft
	CreatedBy      string            `json:"created_by"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	HitCount       int64             `json:"hit_count"`
	cachedResult   interface{}       // in-memory cache of last test result
	cachedAt       time.Time         // when the cache was last set
	rateBuckets    map[string]*apiRuntimeRateBucket
}

type apiRuntimeRateBucket struct {
	WindowStart time.Time
	Count       int
}

// SchemaDefinition, SchemaField, ParamDef — defined in types.go (aliases from models/)

type customAPISuccessEnvelope struct {
	Message      string      `json:"message"`
	ResponseCode int         `json:"responseCode"`
	Success      bool        `json:"success"`
	Data         interface{} `json:"data"`
}

// ---------- CSV Dashboard Models ----------

// CSVUpload tracks a file upload that was converted to a dashboard
type CSVUpload struct {
	ID             string                   `json:"id"`
	Filename       string                   `json:"filename"`
	FileType       string                   `json:"file_type"` // csv, json, xlsx
	Rows           int                      `json:"rows"`
	Columns        int                      `json:"columns"`
	ColumnNames    []string                 `json:"column_names"`
	ColumnTypes    []string                 `json:"column_types"` // string, number, date, geo_lat, geo_lng, geo_name
	SampleData     []map[string]interface{} `json:"sample_data"`
	DashboardID    string                   `json:"dashboard_id,omitempty"`
	GISDashboardID string                   `json:"gis_dashboard_id,omitempty"`
	HasGeoData     bool                     `json:"has_geo_data"`
	Status         string                   `json:"status"` // uploaded, analyzed, dashboard_created, gis_created
	CreatedAt      time.Time                `json:"created_at"`
}

// ScanResult tracks file scan results
type FileScanRecord struct {
	ID        string            `json:"id"`
	Filename  string            `json:"filename"`
	FileSize  int64             `json:"file_size"`
	SHA256    string            `json:"sha256"`
	Safe      bool              `json:"safe"`
	Findings  []scanner.Finding `json:"findings"`
	ScannedAt time.Time         `json:"scanned_at"`
}

type APIScanReport struct {
	ID        string      `json:"id"`
	ScanType  string      `json:"scan_type"`
	Target    string      `json:"target"`
	Method    string      `json:"method,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	Summary   interface{} `json:"summary,omitempty"`
	Result    interface{} `json:"result"`
}

// ---------- Dashboard <-> GIS Conversion ----------

// ConversionResult is an in-memory conversion record (legacy).
// Use ConversionResource from models/ for new code.
type ConversionResult struct {
	ID             string         `json:"id"`
	SourceType     string         `json:"source_type"`
	SourceID       string         `json:"source_id"`
	TargetType     string         `json:"target_type"`
	TargetID       string         `json:"target_id"`
	FieldMappings  []FieldMapping `json:"field_mappings"`
	GeoFieldsFound []string       `json:"geo_fields_found,omitempty"`
	Confidence     float64        `json:"confidence"`
	Status         string         `json:"status"`
	CreatedAt      time.Time      `json:"created_at"`
}

// ---------- Handler ----------

type APIBuilderHandler struct {
	mu                  sync.RWMutex
	customAPIs          map[string]*CustomAPI
	csvUploads          map[string]*CSVUpload
	conversions         map[string]*ConversionResult
	generatedDashboards map[string]*AnalyticsDashboard
	scanRecords         map[string]*FileScanRecord
	apiScanReports      map[string]*APIScanReport
	apiData             map[string][]map[string]interface{} // stores mock data per API
	csvData             map[string][][]string               // raw CSV data per upload
	db                  map[string]*gorm.DB
	etcd                *clientv3.Client
	kvStore             platformstore.KVStore // Raft KV fallback when etcd is nil
	stateKey            string
	scanOrch            *scanner.Orchestrator
	// reference to analytics & GIS handlers for conversion
	analyticsHandler *AnalyticsHandler
	gisHandler       *GISHandler
}

type apiBuilderState struct {
	CustomAPIs          map[string]*CustomAPI               `json:"custom_apis"`
	APIData             map[string][]map[string]interface{} `json:"api_data"`
	CSVUploads          map[string]*CSVUpload               `json:"csv_uploads,omitempty"`
	Conversions         map[string]*ConversionResult        `json:"conversions,omitempty"`
	GeneratedDashboards map[string]*AnalyticsDashboard      `json:"generated_dashboards,omitempty"`
	ScanRecords         map[string]*FileScanRecord          `json:"scan_records,omitempty"`
	APIScanReports      map[string]*APIScanReport           `json:"api_scan_reports,omitempty"`
}

func NewAPIBuilderHandler(ah *AnalyticsHandler, gh *GISHandler, db map[string]*gorm.DB, etcd *clientv3.Client, avEngine *antivirus.Engine) *APIBuilderHandler {
	// Build scanner pipeline — uses subpackage scanners + native antivirus engine.
	cfg := scanner.LoadConfigFromEnv()
	orchestrator := scanner.NewOrchestratorWithConfig(cfg,
		metadata.NewScannerWithConfig(cfg.MaxFileSize, cfg.NullByteSampleSize, cfg.MaxFilenameLength),
		mimetype.NewScanner(cfg.AllowedMIMETypes),
		svg.NewScanner(),
		macro.NewScanner(),
		archivescan.NewScannerWithLimits(cfg.ArchiveMaxDepth, cfg.ArchiveMaxDecompressedSize, cfg.ArchiveCompressionRatioLimit, cfg.ArchiveMaxFiles),
		native.NewScanner(avEngine),
	)
	h := &APIBuilderHandler{
		customAPIs:          make(map[string]*CustomAPI),
		csvUploads:          make(map[string]*CSVUpload),
		conversions:         make(map[string]*ConversionResult),
		generatedDashboards: make(map[string]*AnalyticsDashboard),
		scanRecords:         make(map[string]*FileScanRecord),
		apiScanReports:      make(map[string]*APIScanReport),
		apiData:             make(map[string][]map[string]interface{}),
		csvData:             make(map[string][][]string),
		db:                  db,
		etcd:                etcd,
		stateKey:            "builder:custom_apis:state",
		scanOrch:            orchestrator,
		analyticsHandler:    ah,
		gisHandler:          gh,
	}
	if !h.loadState() {
		h.seedData()
		h.persistStateLocked()
	}
	return h
}

// SetKVStore wires the Raft KV store for state persistence when etcd is unavailable.
func (h *APIBuilderHandler) SetKVStore(kv platformstore.KVStore) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.kvStore = kv
	// Try to load persisted state from KV store
	h.loadStateFromKV()
}

// SetAVEngine wires the antivirus engine into the scanner orchestrator.
// Called after storage system initialization provides the shared engine.
func (h *APIBuilderHandler) SetAVEngine(engine *antivirus.Engine) {
	h.mu.Lock()
	defer h.mu.Unlock()

	cfg := scanner.LoadConfigFromEnv()
	h.scanOrch = scanner.NewOrchestratorWithConfig(cfg,
		metadata.NewScannerWithConfig(cfg.MaxFileSize, cfg.NullByteSampleSize, cfg.MaxFilenameLength),
		mimetype.NewScanner(cfg.AllowedMIMETypes),
		svg.NewScanner(),
		macro.NewScanner(),
		archivescan.NewScannerWithLimits(cfg.ArchiveMaxDepth, cfg.ArchiveMaxDecompressedSize, cfg.ArchiveCompressionRatioLimit, cfg.ArchiveMaxFiles),
		native.NewScanner(engine),
	)
}

func (h *APIBuilderHandler) loadState() bool {
	if h.etcd == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.etcd.Get(ctx, h.stateKey)
	if err != nil {
		logging.Z().Warn("api-builder: failed to load persisted state", zap.Error(err))
		return false
	}
	if len(resp.Kvs) == 0 {
		return false
	}

	return h.applyState(resp.Kvs[0].Value)
}

// loadStateFromKV loads state from the Raft KV store (called when KV is wired late).
func (h *APIBuilderHandler) loadStateFromKV() {
	if h.kvStore == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	val, err := h.kvStore.Get(ctx, h.stateKey)
	if err != nil {
		logging.Z().Warn("api-builder: failed to load state from KV", zap.Error(err))
		return
	}
	if val == "" {
		return
	}

	h.applyState([]byte(val))
}

// applyState deserializes and applies a persisted state blob.
func (h *APIBuilderHandler) applyState(data []byte) bool {
	var state apiBuilderState
	if err := json.Unmarshal(data, &state); err != nil {
		logging.Z().Warn("api-builder: failed to decode persisted state", zap.Error(err))
		return false
	}

	if state.CustomAPIs == nil {
		state.CustomAPIs = make(map[string]*CustomAPI)
	}
	if state.APIData == nil {
		state.APIData = make(map[string][]map[string]interface{})
	}
	if state.CSVUploads == nil {
		state.CSVUploads = make(map[string]*CSVUpload)
	}
	if state.Conversions == nil {
		state.Conversions = make(map[string]*ConversionResult)
	}
	if state.GeneratedDashboards == nil {
		state.GeneratedDashboards = make(map[string]*AnalyticsDashboard)
	}
	if state.ScanRecords == nil {
		state.ScanRecords = make(map[string]*FileScanRecord)
	}
	if state.APIScanReports == nil {
		state.APIScanReports = make(map[string]*APIScanReport)
	}

	h.customAPIs = state.CustomAPIs
	h.apiData = state.APIData
	h.csvUploads = state.CSVUploads
	h.conversions = state.Conversions
	h.generatedDashboards = state.GeneratedDashboards
	h.scanRecords = state.ScanRecords
	h.apiScanReports = state.APIScanReports

	if h.analyticsHandler != nil && len(h.generatedDashboards) > 0 {
		h.analyticsHandler.mu.Lock()
		for id, dash := range h.generatedDashboards {
			h.analyticsHandler.dashboards[id] = dash
		}
		h.analyticsHandler.mu.Unlock()
	}

	logging.Z().Info("api-builder: state loaded", zap.Int("apis", len(h.customAPIs)))
	return true
}

func (h *APIBuilderHandler) persistStateLocked() {
	state := apiBuilderState{
		CustomAPIs:          h.customAPIs,
		APIData:             h.apiData,
		CSVUploads:          h.csvUploads,
		Conversions:         h.conversions,
		GeneratedDashboards: h.generatedDashboards,
		ScanRecords:         h.scanRecords,
		APIScanReports:      h.apiScanReports,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		logging.Z().Warn("api-builder: failed to encode state", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try etcd first, then KV store
	if h.etcd != nil {
		if _, err := h.etcd.Put(ctx, h.stateKey, string(payload)); err != nil {
			logging.Z().Warn("api-builder: failed to persist state (etcd)", zap.Error(err))
		}
		return
	}

	if h.kvStore != nil {
		if err := h.kvStore.Put(ctx, h.stateKey, string(payload)); err != nil {
			logging.Z().Warn("api-builder: failed to persist state (kv)", zap.Error(err))
		}
	}
}



