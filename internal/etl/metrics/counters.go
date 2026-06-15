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

var (
	// Pipeline lifecycle counters
	PipelinesCreated = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "axiom_etl",
		Name:      "pipelines_created_total",
		Help:      "Total number of ETL pipelines created",
	})

	PipelinesDeleted = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "axiom_etl",
		Name:      "pipelines_deleted_total",
		Help:      "Total number of ETL pipelines deleted",
	})

	// Run counters
	RunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "axiom_etl",
		Name:      "runs_total",
		Help:      "Total number of ETL pipeline runs",
	}, []string{"outcome"})

	RunDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "axiom_etl",
		Name:      "run_duration_seconds",
		Help:      "Duration of ETL pipeline runs in seconds",
		Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300, 600, 1800, 3600},
	}, []string{"pipeline_id"})

	// Row counters
	RowsRead = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "axiom_etl",
		Name:      "rows_read_total",
		Help:      "Total rows read by ETL pipelines",
	})

	RowsWritten = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "axiom_etl",
		Name:      "rows_written_total",
		Help:      "Total rows written by ETL pipelines",
	})

	RowsFailed = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "axiom_etl",
		Name:      "rows_failed_total",
		Help:      "Total rows that failed during ETL processing",
	})

	// Gauges
	ActivePipelines = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "axiom_etl",
		Name:      "active_pipelines",
		Help:      "Number of currently active ETL pipelines",
	})

	RunningJobs = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "axiom_etl",
		Name:      "running_jobs",
		Help:      "Number of currently running ETL jobs",
	})

	// Collector holds in-memory metrics for API responses.
	Collector = &MetricsCollector{startTime: time.Now()}
)

const (
	etlMetricsKVKey = "etl:metrics:collector"
	etlMetricsTTL   = 5 * time.Second
)

// MetricsCollector holds in-memory ETL metrics.
type MetricsCollector struct {
	mu               sync.RWMutex
	startTime        time.Time
	totalRuns        int64
	successRuns      int64
	failedRuns       int64
	totalRowsRead    int64
	totalRowsWritten int64
	totalRowsFailed  int64
	kvStore          platformstore.KVStore
}

// metricsState is the JSON-serializable snapshot for KV persistence.
type metricsState struct {
	TotalRuns        int64 `json:"total_runs"`
	SuccessRuns      int64 `json:"success_runs"`
	FailedRuns       int64 `json:"failed_runs"`
	TotalRowsRead    int64 `json:"total_rows_read"`
	TotalRowsWritten int64 `json:"total_rows_written"`
	TotalRowsFailed  int64 `json:"total_rows_failed"`
}

// RecordRun records a completed ETL run.
func (m *MetricsCollector) RecordRun(success bool, rowsRead, rowsWritten, rowsFailed int64) {
	atomic.AddInt64(&m.totalRuns, 1)
	atomic.AddInt64(&m.totalRowsRead, rowsRead)
	atomic.AddInt64(&m.totalRowsWritten, rowsWritten)
	atomic.AddInt64(&m.totalRowsFailed, rowsFailed)

	RowsRead.Add(float64(rowsRead))
	RowsWritten.Add(float64(rowsWritten))
	RowsFailed.Add(float64(rowsFailed))

	if success {
		atomic.AddInt64(&m.successRuns, 1)
		RunsTotal.WithLabelValues("success").Inc()
	} else {
		atomic.AddInt64(&m.failedRuns, 1)
		RunsTotal.WithLabelValues("failure").Inc()
	}

	go m.save()
}

// Snapshot returns a point-in-time snapshot.
func (m *MetricsCollector) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		TotalRuns:        atomic.LoadInt64(&m.totalRuns),
		SuccessRuns:      atomic.LoadInt64(&m.successRuns),
		FailedRuns:       atomic.LoadInt64(&m.failedRuns),
		TotalRowsRead:    atomic.LoadInt64(&m.totalRowsRead),
		TotalRowsWritten: atomic.LoadInt64(&m.totalRowsWritten),
		TotalRowsFailed:  atomic.LoadInt64(&m.totalRowsFailed),
		UptimeSeconds:    int64(time.Since(m.startTime).Seconds()),
	}
}

// MetricsSnapshot is a point-in-time snapshot.
type MetricsSnapshot struct {
	TotalRuns        int64 `json:"total_runs"`
	SuccessRuns      int64 `json:"success_runs"`
	FailedRuns       int64 `json:"failed_runs"`
	TotalRowsRead    int64 `json:"total_rows_read"`
	TotalRowsWritten int64 `json:"total_rows_written"`
	TotalRowsFailed  int64 `json:"total_rows_failed"`
	UptimeSeconds    int64 `json:"uptime_seconds"`
}

// ConfigureKVPersistence wires the KVStore for persistence.
func (m *MetricsCollector) ConfigureKVPersistence(kv platformstore.KVStore) {
	m.kvStore = kv
	m.load()
	logging.Z().Info("etl metrics: KVStore persistence configured")
}

func (m *MetricsCollector) load() {
	if m.kvStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), etlMetricsTTL)
	defer cancel()
	data, err := m.kvStore.Get(ctx, etlMetricsKVKey)
	if err != nil || data == "" {
		return
	}
	var st metricsState
	if err := json.Unmarshal([]byte(data), &st); err != nil {
		logging.Z().Error("etl metrics: unmarshal failed")
		return
	}
	atomic.StoreInt64(&m.totalRuns, st.TotalRuns)
	atomic.StoreInt64(&m.successRuns, st.SuccessRuns)
	atomic.StoreInt64(&m.failedRuns, st.FailedRuns)
	atomic.StoreInt64(&m.totalRowsRead, st.TotalRowsRead)
	atomic.StoreInt64(&m.totalRowsWritten, st.TotalRowsWritten)
	atomic.StoreInt64(&m.totalRowsFailed, st.TotalRowsFailed)
}

func (m *MetricsCollector) save() {
	if m.kvStore == nil {
		return
	}
	st := metricsState{
		TotalRuns:        atomic.LoadInt64(&m.totalRuns),
		SuccessRuns:      atomic.LoadInt64(&m.successRuns),
		FailedRuns:       atomic.LoadInt64(&m.failedRuns),
		TotalRowsRead:    atomic.LoadInt64(&m.totalRowsRead),
		TotalRowsWritten: atomic.LoadInt64(&m.totalRowsWritten),
		TotalRowsFailed:  atomic.LoadInt64(&m.totalRowsFailed),
	}
	ctx, cancel := context.WithTimeout(context.Background(), etlMetricsTTL)
	defer cancel()
	encoded, err := json.Marshal(st)
	if err != nil {
		logging.Z().Error("etl metrics: marshal failed")
		return
	}
	if err := m.kvStore.Put(ctx, etlMetricsKVKey, string(encoded)); err != nil {
		logging.Z().Error("etl metrics: kv persist failed")
	}
}
