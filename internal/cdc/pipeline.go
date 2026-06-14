package cdc

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// =====================================================
// CDC Pipeline Engine — Dynamic Change Data Capture
// Configurable pipelines with source/sink connectors
// and built-in observability
// =====================================================

// --- Pipeline Models ---
// PipelineStatus, CDCSource, CDCSink, CDCFilters are defined in
// cdc/models and re-exported via cdc/types.go.

type CDCPipeline struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Source      CDCSource              `json:"source"`
	Sink        CDCSink                `json:"sink"`
	Filters     CDCFilters             `json:"filters"`
	Status      PipelineStatus         `json:"status"`
	Config      map[string]interface{} `json:"config,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	EventCount  int64                  `json:"event_count"`
	ErrorCount  int64                  `json:"error_count"`
	LastEventAt *time.Time             `json:"last_event_at,omitempty"`
	Lag         string                 `json:"lag,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
}

// CDCSource, CDCSink, CDCFilters are defined in cdc/models and
// re-exported via cdc/types.go.

// --- Source/Sink Types ---

type CDCSourceType struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Method     string   `json:"method"` // binlog, wal, oplog, polling, webhook
	Database   string   `json:"database,omitempty"`
	ConfigKeys []string `json:"config_keys"`
	Icon       string   `json:"icon"`
}

type CDCSinkType struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Category   string   `json:"category"` // stream, database, storage, api
	ConfigKeys []string `json:"config_keys"`
	Icon       string   `json:"icon"`
}

// --- Observability ---

type CDCObservability struct {
	mu              sync.RWMutex
	PipelinesTotal  int                  `json:"pipelines_total"`
	PipelinesActive int                  `json:"pipelines_active"`
	PipelinesPaused int                  `json:"pipelines_paused"`
	PipelinesFailed int                  `json:"pipelines_failed"`
	TotalEvents     int64                `json:"total_events"`
	EventsPerSecond float64              `json:"events_per_second"`
	TotalErrors     int64                `json:"total_errors"`
	ErrorRate       float64              `json:"error_rate_percent"`
	AvgLagMs        float64              `json:"avg_lag_ms"`
	EventsByOp      map[string]int64     `json:"events_by_operation"`
	EventsByTable   map[string]int64     `json:"events_by_table"`
	ErrorsByType    map[string]int       `json:"errors_by_type"`
	ThroughputLog   []CDCThroughputPoint `json:"throughput_log"`
	LastUpdated     time.Time            `json:"last_updated"`
}

type CDCThroughputPoint struct {
	Timestamp    time.Time `json:"timestamp"`
	EventsPerSec float64   `json:"events_per_sec"`
	Pipeline     string    `json:"pipeline"`
	Lag          float64   `json:"lag_ms"`
}

// --- Pipeline Engine ---

type PipelineEngine struct {
	mu            sync.RWMutex
	pipelines     map[string]*CDCPipeline
	sourceTypes   []CDCSourceType
	sinkTypes     []CDCSinkType
	observability *CDCObservability
	cdc           *ChangeDataCapture // reference to core CDC
	sequence      int64
	etcd          *clientv3.Client
	stateKey      string
}

type pipelineEngineState struct {
	Pipelines     map[string]*CDCPipeline `json:"pipelines"`
	Observability *CDCObservability       `json:"observability"`
	Sequence      int64                   `json:"sequence"`
}

func NewPipelineEngine(cdc *ChangeDataCapture, etcd ...*clientv3.Client) *PipelineEngine {
	var etcdClient *clientv3.Client
	if len(etcd) > 0 {
		etcdClient = etcd[0]
	}

	pe := &PipelineEngine{
		pipelines: make(map[string]*CDCPipeline),
		cdc:       cdc,
		observability: &CDCObservability{
			EventsByOp:    make(map[string]int64),
			EventsByTable: make(map[string]int64),
			ErrorsByType:  make(map[string]int),
			ThroughputLog: make([]CDCThroughputPoint, 0),
			LastUpdated:   time.Now(),
		},
		etcd:     etcdClient,
		stateKey: "cdc:pipelines:state",
	}
	pe.registerTypes()
	if !pe.loadState() {
		pe.persistStateLocked()
	}
	return pe
}

func (pe *PipelineEngine) loadState() bool {
	if pe.etcd == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := pe.etcd.Get(ctx, pe.stateKey)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("cdc-pipeline: failed to load persisted state from etcd: %v", err))
		return false
	}
	if len(resp.Kvs) == 0 {
		return false
	}

	var state pipelineEngineState
	if err := json.Unmarshal(resp.Kvs[0].Value, &state); err != nil {
		logging.Z().Info(fmt.Sprintf("cdc-pipeline: failed to decode persisted state: %v", err))
		return false
	}

	if state.Pipelines != nil {
		pe.pipelines = state.Pipelines
	}
	if state.Observability != nil {
		pe.observability = state.Observability
	}
	pe.sequence = state.Sequence
	logging.Z().Info(fmt.Sprintf("cdc-pipeline: restored state from etcd (%d pipelines)", len(pe.pipelines)))
	return true
}

func (pe *PipelineEngine) persistStateLocked() {
	if pe.etcd == nil {
		return
	}

	state := pipelineEngineState{
		Pipelines:     pe.pipelines,
		Observability: pe.observability,
		Sequence:      pe.sequence,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("cdc-pipeline: failed to encode state: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := pe.etcd.Put(ctx, pe.stateKey, string(payload)); err != nil {
		logging.Z().Info(fmt.Sprintf("cdc-pipeline: failed to persist state to etcd: %v", err))
	}
}

func (pe *PipelineEngine) registerTypes() {
	pe.sourceTypes = []CDCSourceType{
		{ID: "mysql_binlog", Name: "MySQL Binary Log", Method: "binlog", Database: "mysql", ConfigKeys: []string{"host", "port", "database", "server_id", "binlog_position"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><path d="M4 7l8-4 8 4M4 7v10l8 4 8-4V7M4 7l8 4 8-4M12 11v10"/></svg>`},
		{ID: "pg_wal", Name: "PostgreSQL WAL", Method: "wal", Database: "postgres", ConfigKeys: []string{"host", "port", "database", "slot_name", "publication"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v12c0 1.66 3.58 3 8 3s8-1.34 8-3V6"/><path d="M4 12c0 1.66 3.58 3 8 3s8-1.34 8-3"/></svg>`},
		{ID: "mongo_oplog", Name: "MongoDB Oplog", Method: "oplog", Database: "mongodb", ConfigKeys: []string{"uri", "database", "replica_set"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><path d="M12 2C9 5 8 8 8 12c0 4 2 7 4 10"/><path d="M12 2c3 3 4 6 4 10 0 4-2 7-4 10"/><path d="M12 6v16"/></svg>`},
		{ID: "polling", Name: "Table Polling", Method: "polling", ConfigKeys: []string{"host", "database", "table", "poll_interval", "cursor_column"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18"/><path d="M9 3v18"/><circle cx="15" cy="15" r="2"/><path d="M15 13v-1"/></svg>`},
		{ID: "api_webhook", Name: "API Webhook", Method: "webhook", ConfigKeys: []string{"endpoint", "secret", "events"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><circle cx="12" cy="12" r="9"/><path d="M3.6 9h16.8"/><path d="M3.6 15h16.8"/><path d="M12 3c-2.8 2.4-4 5.6-4 9s1.2 6.6 4 9c2.8-2.4 4-5.6 4-9s-1.2-6.6-4-9"/></svg>`},
		{ID: "mariadb_binlog", Name: "MariaDB Binary Log", Method: "binlog", Database: "mariadb", ConfigKeys: []string{"host", "port", "database", "server_id"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v6c0 1.66 3.58 3 8 3s8-1.34 8-3V6"/><path d="M4 12v6c0 1.66 3.58 3 8 3s8-1.34 8-3v-6"/><path d="M9 6v12" opacity="0.4"/></svg>`},
	}

	pe.sinkTypes = []CDCSinkType{
		{ID: "kafka", Name: "Apache Kafka", Category: "stream", ConfigKeys: []string{"brokers", "topic", "key_field"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><circle cx="8" cy="6" r="2"/><circle cx="8" cy="18" r="2"/><circle cx="16" cy="12" r="2"/><path d="M10 6h4l2 6-2 6h-4"/><path d="M8 8v8"/></svg>`},
		{ID: "webhook", Name: "Webhook (HTTP)", Category: "api", ConfigKeys: []string{"url", "method", "headers", "retry_count"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/></svg>`},
		{ID: "postgres", Name: "PostgreSQL", Category: "database", ConfigKeys: []string{"host", "port", "database", "table"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v12c0 1.66 3.58 3 8 3s8-1.34 8-3V6"/><path d="M4 12c0 1.66 3.58 3 8 3s8-1.34 8-3"/></svg>`},
		{ID: "elasticsearch", Name: "Elasticsearch", Category: "search", ConfigKeys: []string{"url", "index", "pipeline"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><circle cx="11" cy="11" r="7"/><path d="M21 21l-4.35-4.35"/><path d="M8 11h6"/></svg>`},
		{ID: "s3", Name: "S3/MinIO", Category: "storage", ConfigKeys: []string{"endpoint", "bucket", "prefix", "format"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z"/></svg>`},
		{ID: "redis", Name: "Redis Stream", Category: "stream", ConfigKeys: []string{"host", "port", "stream_key"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><path d="M4 12l8-4 8 4-8 4-8-4z"/><path d="M4 12v4l8 4 8-4v-4"/><path d="M4 8v4l8 4 8-4V8"/><path d="M4 8l8-4 8 4-8 4-8-4z"/></svg>`},
		{ID: "api", Name: "REST API", Category: "api", ConfigKeys: []string{"url", "method", "auth"}, Icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="ax-icon"><circle cx="12" cy="12" r="9"/><path d="M3.6 9h16.8"/><path d="M3.6 15h16.8"/><path d="M12 3c-2.8 2.4-4 5.6-4 9s1.2 6.6 4 9c2.8-2.4 4-5.6 4-9s-1.2-6.6-4-9"/></svg>`},
	}
}

// --- Source/Sink Type Queries ---

func (pe *PipelineEngine) GetSourceTypes() []CDCSourceType { return pe.sourceTypes }
func (pe *PipelineEngine) GetSinkTypes() []CDCSinkType     { return pe.sinkTypes }

// --- Pipeline CRUD ---

func (pe *PipelineEngine) CreatePipeline(p *CDCPipeline) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	if p.ID == "" {
		pe.sequence++
		p.ID = fmt.Sprintf("cdc-pipe-%d", pe.sequence)
	}
	p.Status = CDCCreated
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	pe.pipelines[p.ID] = p

	pe.observability.mu.Lock()
	pe.observability.PipelinesTotal++
	pe.observability.mu.Unlock()
	pe.persistStateLocked()
	return nil
}

func (pe *PipelineEngine) GetPipeline(id string) (*CDCPipeline, bool) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	p, ok := pe.pipelines[id]
	return p, ok
}

func (pe *PipelineEngine) ListPipelines() []*CDCPipeline {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	result := make([]*CDCPipeline, 0, len(pe.pipelines))
	for _, p := range pe.pipelines {
		result = append(result, p)
	}
	return result
}

func (pe *PipelineEngine) UpdatePipeline(id string, updates map[string]interface{}) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	p, ok := pe.pipelines[id]
	if !ok {
		return fmt.Errorf("pipeline not found: %s", id)
	}
	if name, ok := updates["name"].(string); ok && name != "" {
		p.Name = name
	}
	if desc, ok := updates["description"].(string); ok {
		p.Description = desc
	}
	p.UpdatedAt = time.Now()
	pe.persistStateLocked()
	return nil
}

func (pe *PipelineEngine) DeletePipeline(id string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	p, ok := pe.pipelines[id]
	if !ok {
		return fmt.Errorf("pipeline not found: %s", id)
	}

	pe.observability.mu.Lock()
	pe.observability.PipelinesTotal--
	if p.Status == CDCActive {
		pe.observability.PipelinesActive--
	}
	pe.observability.mu.Unlock()

	delete(pe.pipelines, id)
	pe.persistStateLocked()
	return nil
}

// --- Pipeline Actions ---

func (pe *PipelineEngine) StartPipeline(id string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	p, ok := pe.pipelines[id]
	if !ok {
		return fmt.Errorf("pipeline not found: %s", id)
	}
	p.Status = CDCActive
	p.UpdatedAt = time.Now()

	pe.observability.mu.Lock()
	pe.observability.PipelinesActive++
	pe.observability.mu.Unlock()
	pe.persistStateLocked()
	return nil
}

func (pe *PipelineEngine) PausePipeline(id string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	p, ok := pe.pipelines[id]
	if !ok {
		return fmt.Errorf("pipeline not found: %s", id)
	}
	if p.Status == CDCActive {
		pe.observability.mu.Lock()
		pe.observability.PipelinesActive--
		pe.observability.PipelinesPaused++
		pe.observability.mu.Unlock()
	}
	p.Status = CDCPaused
	p.UpdatedAt = time.Now()
	pe.persistStateLocked()
	return nil
}

func (pe *PipelineEngine) StopPipeline(id string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	p, ok := pe.pipelines[id]
	if !ok {
		return fmt.Errorf("pipeline not found: %s", id)
	}
	if p.Status == CDCActive {
		pe.observability.mu.Lock()
		pe.observability.PipelinesActive--
		pe.observability.mu.Unlock()
	} else if p.Status == CDCPaused {
		pe.observability.mu.Lock()
		pe.observability.PipelinesPaused--
		pe.observability.mu.Unlock()
	}
	p.Status = CDCStopped
	p.UpdatedAt = time.Now()
	pe.persistStateLocked()
	return nil
}

func (pe *PipelineEngine) GetObservability() *CDCObservability {
	pe.observability.mu.RLock()
	defer pe.observability.mu.RUnlock()
	snap := *pe.observability
	return &snap
}

// --- Seed ---

func (pe *PipelineEngine) seedPipelines() {
	now := time.Now()
	lastEvent := now.Add(-30 * time.Second)

	// Pipeline 1: MySQL binlog → Kafka
	p1 := &CDCPipeline{
		ID:          "cdc-mysql-kafka",
		Name:        "MySQL Binlog → Kafka",
		Description: "Captures MySQL binary log changes and publishes to Kafka topics in real-time",
		Source: CDCSource{
			Type: "mysql_binlog", Connector: "mysql",
			Config: map[string]interface{}{"host": "localhost", "port": 3306, "database": "axiomnizam", "server_id": 1001, "binlog_position": "mysql-bin.000042:1248"},
			Tables: []string{"users", "orders", "products"},
		},
		Sink: CDCSink{
			Type: "kafka", Connector: "kafka",
			Config:    map[string]interface{}{"brokers": "localhost:9092", "topic": "axiomnizam.cdc.{table}", "key_field": "id"},
			BatchSize: 100,
		},
		Filters:     CDCFilters{Operations: []string{"INSERT", "UPDATE", "DELETE"}, Exclude: []string{"_audit_log"}},
		Status:      CDCActive,
		CreatedAt:   now.Add(-168 * time.Hour),
		UpdatedAt:   now,
		EventCount:  2847293,
		ErrorCount:  42,
		LastEventAt: &lastEvent,
		Lag:         "120ms",
		Tags:        []string{"production", "critical", "real-time"},
	}

	// Pipeline 2: PostgreSQL WAL → Elasticsearch
	p2 := &CDCPipeline{
		ID:          "cdc-pg-elastic",
		Name:        "PostgreSQL WAL → Elasticsearch",
		Description: "Streams PostgreSQL write-ahead log changes to Elasticsearch for search indexing",
		Source: CDCSource{
			Type: "pg_wal", Connector: "postgres",
			Config: map[string]interface{}{"host": "localhost", "port": 5432, "database": "axiomnizam", "slot_name": "cdc_slot_1", "publication": "cdc_pub"},
			Tables: []string{"products", "categories", "reviews"},
		},
		Sink: CDCSink{
			Type: "elasticsearch", Connector: "elasticsearch",
			Config:    map[string]interface{}{"url": "http://localhost:9200", "index": "products-{date}", "pipeline": "cdc-enrichment"},
			BatchSize: 500,
		},
		Filters:     CDCFilters{Operations: []string{"INSERT", "UPDATE"}},
		Status:      CDCActive,
		CreatedAt:   now.Add(-96 * time.Hour),
		UpdatedAt:   now,
		EventCount:  589421,
		ErrorCount:  7,
		LastEventAt: &lastEvent,
		Lag:         "45ms",
		Tags:        []string{"search", "indexing"},
	}

	// Pipeline 3: MongoDB Oplog → Webhook
	p3 := &CDCPipeline{
		ID:          "cdc-mongo-webhook",
		Name:        "MongoDB Oplog → Webhooks",
		Description: "Watches MongoDB oplog and triggers webhooks for downstream consumers",
		Source: CDCSource{
			Type: "mongo_oplog", Connector: "mongodb",
			Config: map[string]interface{}{"uri": "mongodb://localhost:27017", "database": "analytics", "replica_set": "rs0"},
			Tables: []string{"events", "sessions", "user_actions"},
		},
		Sink: CDCSink{
			Type: "webhook", Connector: "webhook",
			Config:    map[string]interface{}{"url": "https://hooks.example.com/cdc", "method": "POST", "retry_count": 3},
			BatchSize: 1,
		},
		Filters:     CDCFilters{Operations: []string{"INSERT"}},
		Status:      CDCPaused,
		CreatedAt:   now.Add(-48 * time.Hour),
		UpdatedAt:   now,
		EventCount:  125840,
		ErrorCount:  312,
		LastEventAt: &lastEvent,
		Lag:         "2s",
		Tags:        []string{"webhooks", "notifications"},
	}

	// Pipeline 4: Polling → S3 Archive
	p4 := &CDCPipeline{
		ID:          "cdc-poll-s3",
		Name:        "Table Polling → S3 Archive",
		Description: "Polls audit tables periodically and archives changes to S3 for compliance",
		Source: CDCSource{
			Type: "polling", Connector: "mysql",
			Config: map[string]interface{}{"host": "localhost", "database": "axiomnizam", "table": "audit_log", "poll_interval": "60s", "cursor_column": "id"},
			Tables: []string{"audit_log"},
		},
		Sink: CDCSink{
			Type: "s3", Connector: "s3",
			Config:    map[string]interface{}{"endpoint": "http://minio:9000", "bucket": "cdc-archive", "prefix": "audit/{year}/{month}/{day}/", "format": "parquet"},
			BatchSize: 1000,
		},
		Filters:     CDCFilters{},
		Status:      CDCActive,
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
		EventCount:  45200,
		ErrorCount:  0,
		LastEventAt: &lastEvent,
		Lag:         "60s",
		Tags:        []string{"compliance", "archive", "s3"},
	}

	// Pipeline 5: MariaDB → Redis Stream
	p5 := &CDCPipeline{
		ID:          "cdc-maria-redis",
		Name:        "MariaDB Binlog → Redis Stream",
		Description: "Captures MariaDB changes for real-time cache invalidation via Redis Streams",
		Source: CDCSource{
			Type: "mariadb_binlog", Connector: "mariadb",
			Config: map[string]interface{}{"host": "localhost", "port": 3307, "database": "axiomnizam", "server_id": 2001},
			Tables: []string{"sessions", "user_preferences", "cache_keys"},
		},
		Sink: CDCSink{
			Type: "redis", Connector: "redis",
			Config:    map[string]interface{}{"host": "localhost", "port": 6379, "stream_key": "cdc:cache-invalidation"},
			BatchSize: 50,
		},
		Filters:    CDCFilters{Operations: []string{"UPDATE", "DELETE"}},
		Status:     CDCStopped,
		CreatedAt:  now.Add(-12 * time.Hour),
		UpdatedAt:  now,
		EventCount: 0,
		Tags:       []string{"cache", "invalidation"},
	}

	pe.pipelines[p1.ID] = p1
	pe.pipelines[p2.ID] = p2
	pe.pipelines[p3.ID] = p3
	pe.pipelines[p4.ID] = p4
	pe.pipelines[p5.ID] = p5

	// Seed observability
	pe.observability.PipelinesTotal = 5
	pe.observability.PipelinesActive = 3
	pe.observability.PipelinesPaused = 1
	pe.observability.TotalEvents = p1.EventCount + p2.EventCount + p3.EventCount + p4.EventCount
	pe.observability.EventsPerSecond = 1247.5
	pe.observability.TotalErrors = p1.ErrorCount + p2.ErrorCount + p3.ErrorCount
	pe.observability.ErrorRate = 0.01
	pe.observability.AvgLagMs = 85.0
	pe.observability.EventsByOp = map[string]int64{"INSERT": 2100000, "UPDATE": 1350000, "DELETE": 157754}
	pe.observability.EventsByTable = map[string]int64{
		"users": 892000, "orders": 1250000, "products": 520000,
		"events": 125840, "audit_log": 45200, "sessions": 380000,
	}
	pe.observability.ErrorsByType = map[string]int{"timeout": 180, "connection": 95, "serialization": 42, "sink_unavailable": 44}

	// Seed throughput log
	for i := 24; i >= 0; i-- {
		pe.observability.ThroughputLog = append(pe.observability.ThroughputLog, CDCThroughputPoint{
			Timestamp:    now.Add(-time.Duration(i) * time.Hour),
			EventsPerSec: 1200 + float64(i%5)*100,
			Pipeline:     "cdc-mysql-kafka",
			Lag:          float64(50 + i*3),
		})
	}
}
