package metrics

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	metricsKVKey = "netintel:metrics:collector"
	metricsTTL   = 5 * time.Second
)

var (
	// Log ingestion
	LogEntriesIngested = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "axiom_netintel",
		Name:      "log_entries_ingested_total",
		Help:      "Total log entries ingested",
	}, []string{LabelLogType, LabelSeverity})

	// Parsers
	ParsersActive = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "axiom_netintel",
		Name:      "parsers_active",
		Help:      "Number of active parsers",
	})

	ParserOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "axiom_netintel",
		Name:      "parser_operations_total",
		Help:      "Total parser CRUD operations",
	}, []string{LabelOperation})

	// Topology
	TopologyNodesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "axiom_netintel",
		Name:      "topology_nodes_total",
		Help:      "Total topology nodes",
	})

	TopologyEdgesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "axiom_netintel",
		Name:      "topology_edges_total",
		Help:      "Total topology edges",
	})

	// Anomalies and alerts
	AnomaliesActive = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "axiom_netintel",
		Name:      "anomalies_active",
		Help:      "Number of active anomalies",
	})

	AlertsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "axiom_netintel",
		Name:      "alerts_active",
		Help:      "Number of active alerts",
	})

	AnomalyOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "axiom_netintel",
		Name:      "anomaly_operations_total",
		Help:      "Total anomaly acknowledge/resolve operations",
	}, []string{LabelOperation})

	AlertOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "axiom_netintel",
		Name:      "alert_operations_total",
		Help:      "Total alert acknowledge/resolve operations",
	}, []string{LabelOperation})

	// Predictions
	PredictionsGenerated = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "axiom_netintel",
		Name:      "predictions_generated_total",
		Help:      "Total movement predictions generated",
	})

	// Mode events
	ModeEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "axiom_netintel",
		Name:      "mode_events_total",
		Help:      "Total mode events recorded",
	}, []string{LabelMode})

	// Parse duration
	ParseDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "axiom_netintel",
		Name:      "parse_duration_seconds",
		Help:      "Duration of log parsing operations",
		Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
	}, []string{LabelLogType})

	// Collector holds in-memory metrics for the API.
	Collector = &MetricsCollector{startTime: time.Now()}
)

// MetricsCollector holds in-memory netintel metrics for API responses.
type MetricsCollector struct {
	mu                sync.RWMutex
	kvStore           platformstore.KVStore
	startTime         time.Time
	totalIngested     int64
	totalParsed       int64
	totalAnomalies    int64
	totalAlerts       int64
	totalPredictions  int64
	totalModes        int64
	byLogType         map[string]int64
	bySeverity        map[string]int64
}

type collectorState struct {
	TotalIngested    int64            `json:"total_ingested"`
	TotalParsed      int64            `json:"total_parsed"`
	TotalAnomalies   int64            `json:"total_anomalies"`
	TotalAlerts      int64            `json:"total_alerts"`
	TotalPredictions int64            `json:"total_predictions"`
	TotalModes       int64            `json:"total_modes"`
	ByLogType        map[string]int64 `json:"by_log_type"`
	BySeverity       map[string]int64 `json:"by_severity"`
}

// ConfigureKVPersistence wires KVStore for metrics persistence.
func (m *MetricsCollector) ConfigureKVPersistence(kv platformstore.KVStore) {
	m.kvStore = kv
	m.load()
	logging.Z().Info("netintel metrics: KVStore persistence configured")
}

// RecordLogEntry records a log ingestion event.
func (m *MetricsCollector) RecordLogEntry(logType, severity string) {
	atomic.AddInt64(&m.totalIngested, 1)

	LogEntriesIngested.WithLabelValues(logType, severity).Inc()

	m.mu.Lock()
	if m.byLogType == nil {
		m.byLogType = make(map[string]int64)
	}
	if m.bySeverity == nil {
		m.bySeverity = make(map[string]int64)
	}
	m.byLogType[logType]++
	m.bySeverity[severity]++
	m.mu.Unlock()

	go m.save()
}

// RecordParserOperation records a parser CRUD operation.
func (m *MetricsCollector) RecordParserOperation(operation string) {
	ParserOperations.WithLabelValues(operation).Inc()
	go m.save()
}

// RecordAnomalyOperation records an anomaly acknowledge/resolve.
func (m *MetricsCollector) RecordAnomalyOperation(operation string) {
	AnomalyOperations.WithLabelValues(operation).Inc()
	go m.save()
}

// RecordAlertOperation records an alert acknowledge/resolve.
func (m *MetricsCollector) RecordAlertOperation(operation string) {
	AlertOperations.WithLabelValues(operation).Inc()
	go m.save()
}

// RecordPrediction records a prediction generation.
func (m *MetricsCollector) RecordPrediction() {
	atomic.AddInt64(&m.totalPredictions, 1)
	PredictionsGenerated.Inc()
	go m.save()
}

// RecordModeEvent records a mode event.
func (m *MetricsCollector) RecordModeEvent(mode string) {
	atomic.AddInt64(&m.totalModes, 1)
	ModeEventsTotal.WithLabelValues(mode).Inc()
	go m.save()
}

// Snapshot returns a point-in-time snapshot for API responses.
func (m *MetricsCollector) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	byLogType := make(map[string]int64)
	for k, v := range m.byLogType {
		byLogType[k] = v
	}
	bySeverity := make(map[string]int64)
	for k, v := range m.bySeverity {
		bySeverity[k] = v
	}

	return MetricsSnapshot{
		TotalIngested:    atomic.LoadInt64(&m.totalIngested),
		TotalParsed:      atomic.LoadInt64(&m.totalParsed),
		TotalAnomalies:   atomic.LoadInt64(&m.totalAnomalies),
		TotalAlerts:      atomic.LoadInt64(&m.totalAlerts),
		TotalPredictions: atomic.LoadInt64(&m.totalPredictions),
		TotalModes:       atomic.LoadInt64(&m.totalModes),
		UptimeSeconds:    int64(time.Since(m.startTime).Seconds()),
		ByLogType:        byLogType,
		BySeverity:       bySeverity,
	}
}

// MetricsSnapshot is a point-in-time snapshot of all netintel metrics.
type MetricsSnapshot struct {
	TotalIngested    int64            `json:"total_ingested"`
	TotalParsed      int64            `json:"total_parsed"`
	TotalAnomalies   int64            `json:"total_anomalies"`
	TotalAlerts      int64            `json:"total_alerts"`
	TotalPredictions int64            `json:"total_predictions"`
	TotalModes       int64            `json:"total_modes"`
	UptimeSeconds    int64            `json:"uptime_seconds"`
	ByLogType        map[string]int64 `json:"by_log_type"`
	BySeverity       map[string]int64 `json:"by_severity"`
}

func (m *MetricsCollector) save() {
	if m.kvStore == nil {
		return
	}
	m.mu.RLock()
	state := collectorState{
		TotalIngested:    atomic.LoadInt64(&m.totalIngested),
		TotalParsed:      atomic.LoadInt64(&m.totalParsed),
		TotalAnomalies:   atomic.LoadInt64(&m.totalAnomalies),
		TotalAlerts:      atomic.LoadInt64(&m.totalAlerts),
		TotalPredictions: atomic.LoadInt64(&m.totalPredictions),
		TotalModes:       atomic.LoadInt64(&m.totalModes),
		ByLogType:        m.byLogType,
		BySeverity:       m.bySeverity,
	}
	m.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), metricsTTL)
	defer cancel()

	data, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := m.kvStore.Put(ctx, metricsKVKey, string(data)); err != nil {
		logging.Z().Info("netintel metrics: kv persist failed")
	}
}

func (m *MetricsCollector) load() {
	if m.kvStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), metricsTTL)
	defer cancel()

	data, err := m.kvStore.Get(ctx, metricsKVKey)
	if err != nil || data == "" {
		return
	}
	var state collectorState
	if err := json.Unmarshal([]byte(data), &state); err != nil {
		logging.Z().Info("netintel metrics: unmarshal failed")
		return
	}
	atomic.StoreInt64(&m.totalIngested, state.TotalIngested)
	atomic.StoreInt64(&m.totalParsed, state.TotalParsed)
	atomic.StoreInt64(&m.totalAnomalies, state.TotalAnomalies)
	atomic.StoreInt64(&m.totalAlerts, state.TotalAlerts)
	atomic.StoreInt64(&m.totalPredictions, state.TotalPredictions)
	atomic.StoreInt64(&m.totalModes, state.TotalModes)
	if state.ByLogType != nil {
		m.byLogType = state.ByLogType
	}
	if state.BySeverity != nil {
		m.bySeverity = state.BySeverity
	}
	logging.Z().Info("netintel metrics: loaded persistent state")
}
