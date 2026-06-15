package jobs

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/events"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// MetricsCollector collects job and event metrics
type MetricsCollector struct {
	// Job metrics
	jobsTotal             prometheus.Counter
	jobsCreated           prometheus.Counter
	jobsCompleted         prometheus.Counter
	jobsFailed            prometheus.Counter
	jobsRetried           prometheus.Counter
	jobsCancelled         prometheus.Counter
	jobDuration           prometheus.Histogram
	jobProcessingDuration prometheus.Histogram
	queueSize             prometheus.Gauge
	queueSizeByStatus     *prometheus.GaugeVec
	workersActive         prometheus.Gauge
	workersTotal          prometheus.Gauge
	jobQueueDepth         prometheus.Gauge

	// Event metrics
	eventsTotal          prometheus.Counter
	eventsPublished      prometheus.Counter
	eventsByType         *prometheus.CounterVec
	eventHandlerDuration prometheus.Histogram
	eventHandlerErrors   *prometheus.CounterVec
	eventBusCapacity     prometheus.Gauge

	// Processor metrics
	processorJobsProcessed prometheus.Counter
	processorJobsSucceeded prometheus.Counter
	processorJobsFailed    prometheus.Counter
	processorWorkerCount   prometheus.Gauge
	processorSuccessRate   prometheus.Gauge

}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(namespace string) *MetricsCollector {
	if namespace == "" {
		namespace = "axiom_jobs"
	}

	mc := &MetricsCollector{
		// Job counters
		jobsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "total",
			Help:      "Total number of jobs created",
		}),

		jobsCreated: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "created_total",
			Help:      "Number of jobs created",
		}),

		jobsCompleted: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "completed_total",
			Help:      "Number of jobs completed successfully",
		}),

		jobsFailed: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "failed_total",
			Help:      "Number of jobs failed",
		}),

		jobsRetried: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "retried_total",
			Help:      "Number of jobs retried",
		}),

		jobsCancelled: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "cancelled_total",
			Help:      "Number of jobs cancelled",
		}),

		jobDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "duration_seconds",
			Help:      "Job execution duration in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10),
		}),

		jobProcessingDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "processing_duration_seconds",
			Help:      "Job processing time from submit to completion",
			Buckets:   prometheus.ExponentialBuckets(1, 2, 12),
		}),

		queueSize: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "queue_size",
			Help:      "Current job queue size",
		}),

		queueSizeByStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "queue_size_by_status",
			Help:      "Job queue size by status",
		}, []string{"status"}),

		workersActive: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "processor",
			Name:      "workers_active",
			Help:      "Number of active worker goroutines",
		}),

		workersTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "processor",
			Name:      "workers_total",
			Help:      "Total number of worker goroutines",
		}),

		jobQueueDepth: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "queue_depth",
			Help:      "Maximum queue depth",
		}),

		// Event metrics
		eventsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "events",
			Name:      "total",
			Help:      "Total events published",
		}),

		eventsPublished: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "events",
			Name:      "published_total",
			Help:      "Number of events published",
		}),

		eventsByType: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "events",
			Name:      "by_type_total",
			Help:      "Events published by type",
		}, []string{"type"}),

		eventHandlerDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "events",
			Name:      "handler_duration_seconds",
			Help:      "Event handler execution duration",
			Buckets:   prometheus.ExponentialBuckets(0.01, 2, 10),
		}),

		eventHandlerErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "events",
			Name:      "handler_errors_total",
			Help:      "Event handler errors",
		}, []string{"type"}),

		eventBusCapacity: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "events",
			Name:      "bus_capacity",
			Help:      "Event bus subscriber capacity",
		}),

		// Processor metrics
		processorJobsProcessed: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "processor",
			Name:      "jobs_processed_total",
			Help:      "Total jobs processed",
		}),

		processorJobsSucceeded: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "processor",
			Name:      "jobs_succeeded_total",
			Help:      "Total jobs succeeded",
		}),

		processorJobsFailed: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "processor",
			Name:      "jobs_failed_total",
			Help:      "Total jobs failed",
		}),

		processorWorkerCount: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "processor",
			Name:      "worker_count",
			Help:      "Current worker count",
		}),

		processorSuccessRate: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "processor",
			Name:      "success_rate",
			Help:      "Job success rate (0-1)",
		}),

	}

	return mc
}

// RecordJobCreated records a job creation
func (mc *MetricsCollector) RecordJobCreated(jobType JobType, priority JobPriority) {
	mc.jobsTotal.Inc()
	mc.jobsCreated.Inc()
}

// RecordJobCompleted records a successful job completion
func (mc *MetricsCollector) RecordJobCompleted(jobType JobType, duration time.Duration) {
	mc.jobsCompleted.Inc()
	mc.jobDuration.Observe(duration.Seconds())
	mc.processorJobsSucceeded.Inc()
	mc.processorJobsProcessed.Inc()
}

// RecordJobFailed records a failed job
func (mc *MetricsCollector) RecordJobFailed(jobType JobType, duration time.Duration) {
	mc.jobsFailed.Inc()
	mc.jobDuration.Observe(duration.Seconds())
	mc.processorJobsFailed.Inc()
	mc.processorJobsProcessed.Inc()
}

// RecordJobRetried records a job retry
func (mc *MetricsCollector) RecordJobRetried(jobType JobType) {
	mc.jobsRetried.Inc()
}

// RecordJobCancelled records a cancelled job
func (mc *MetricsCollector) RecordJobCancelled(jobType JobType) {
	mc.jobsCancelled.Inc()
}

// RecordJobProcessingTime records end-to-end job processing time
func (mc *MetricsCollector) RecordJobProcessingTime(duration time.Duration) {
	mc.jobProcessingDuration.Observe(duration.Seconds())
}

// RecordQueueStats records queue statistics
func (mc *MetricsCollector) RecordQueueStats(stats *QueueStats) {
	if stats == nil {
		return
	}

	mc.queueSize.Set(float64(stats.Total))
	mc.queueSizeByStatus.WithLabelValues("pending").Set(float64(stats.Pending))
	mc.queueSizeByStatus.WithLabelValues("running").Set(float64(stats.Running))
	mc.queueSizeByStatus.WithLabelValues("completed").Set(float64(stats.Completed))
	mc.queueSizeByStatus.WithLabelValues("failed").Set(float64(stats.Failed))
	mc.queueSizeByStatus.WithLabelValues("cancelled").Set(float64(stats.Cancelled))
}

// RecordProcessorStats records processor statistics
func (mc *MetricsCollector) RecordProcessorStats(stats *ProcessorStats) {
	if stats == nil {
		return
	}

	mc.workersActive.Set(float64(stats.WorkersActive))
	mc.workersTotal.Set(float64(stats.WorkersTotal))
	mc.processorWorkerCount.Set(float64(stats.WorkersActive))

	if stats.JobsProcessed > 0 {
		successRate := float64(stats.JobsSucceeded) / float64(stats.JobsProcessed)
		mc.processorSuccessRate.Set(successRate)
	}
}

// RecordEventPublished records event publication
func (mc *MetricsCollector) RecordEventPublished(eventType EventType) {
	mc.eventsTotal.Inc()
	mc.eventsPublished.Inc()
	mc.eventsByType.WithLabelValues(string(eventType)).Inc()
}

// RecordEventHandlerDuration records event handler execution time
func (mc *MetricsCollector) RecordEventHandlerDuration(duration time.Duration) {
	mc.eventHandlerDuration.Observe(duration.Seconds())
}

// RecordEventHandlerError records event handler error
func (mc *MetricsCollector) RecordEventHandlerError(eventType EventType) {
	mc.eventHandlerErrors.WithLabelValues(string(eventType)).Inc()
}

// UpdateQueueDepth updates the queue depth
func (mc *MetricsCollector) UpdateQueueDepth(depth int) {
	mc.jobQueueDepth.Set(float64(depth))
}

// StartMetricsCollection starts periodic metrics collection
func StartMetricsCollection(ctx context.Context, manager JobManager, collector *MetricsCollector, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Collect queue stats
				stats, err := manager.GetJobStats(ctx)
				if err == nil {
					collector.RecordQueueStats(stats)
				}

				// Collect processor stats
				procStats := manager.GetProcessorStats()
				if procStats != nil {
					collector.RecordProcessorStats(procStats)
				}
			}
		}
	}()
}

// MetricsMiddleware provides middleware for recording job metrics
type MetricsMiddleware struct {
	collector *MetricsCollector
}

// NewMetricsMiddleware creates a new metrics middleware
func NewMetricsMiddleware(collector *MetricsCollector) *MetricsMiddleware {
	return &MetricsMiddleware{
		collector: collector,
	}
}

// WrapHandler wraps a job handler with metrics collection
func (mm *MetricsMiddleware) WrapHandler(jobType JobType, handler JobHandler) JobHandler {
	return func(ctx context.Context, job *Job) error {
		startTime := time.Now()

		err := handler(ctx, job)

		duration := time.Since(startTime)

		if err != nil {
			mm.collector.RecordJobFailed(jobType, duration)
		} else {
			mm.collector.RecordJobCompleted(jobType, duration)
		}

		return err
	}
}

// WrapEventHandler wraps an event handler with metrics collection
func (mm *MetricsMiddleware) WrapEventHandler(eventType events.EventType, handler events.EventHandler) events.EventHandler {
	return func(ctx context.Context, event *events.Event) error {
		startTime := time.Now()

		// Record event with string conversion
		_ = eventType // metrics collection would go here
		err := handler(ctx, event)

		duration := time.Since(startTime)
		mm.collector.RecordEventHandlerDuration(duration)

		if err != nil {
			mm.collector.RecordEventHandlerError(EventType(eventType))
		}

		return err
	}
}

// MetricsExporter exports metrics in Prometheus format
type MetricsExporter struct {
	collector *MetricsCollector
}

// NewMetricsExporter creates a new metrics exporter
func NewMetricsExporter(collector *MetricsCollector) *MetricsExporter {
	return &MetricsExporter{
		collector: collector,
	}
}

// Export exports current metrics as formatted string
func (me *MetricsExporter) Export() string {
	return fmt.Sprintf(`
# AXIOM NIZAM JOB METRICS EXPORT

## Job Metrics
- Jobs Total: %v
- Jobs Created: %v
- Jobs Completed: %v
- Jobs Failed: %v
- Jobs Retried: %v
- Jobs Cancelled: %v

## Queue Metrics
- Queue Size: %v

## Processor Metrics
- Workers Active: %v
- Workers Total: %v
- Jobs Processed: %v
- Jobs Succeeded: %v
- Jobs Failed: %v

## Event Metrics
- Events Published: %v

`,
		me.collector.jobsTotal,
		me.collector.jobsCreated,
		me.collector.jobsCompleted,
		me.collector.jobsFailed,
		me.collector.jobsRetried,
		me.collector.jobsCancelled,
		me.collector.queueSize,
		me.collector.workersActive,
		me.collector.workersTotal,
		me.collector.processorJobsProcessed,
		me.collector.processorJobsSucceeded,
		me.collector.processorJobsFailed,
		me.collector.eventsPublished,
	)
}

// HealthMetrics provides health-related metrics
type HealthMetrics struct {
	collector *MetricsCollector
	manager   JobManager
}

// NewHealthMetrics creates health metrics monitor
func NewHealthMetrics(collector *MetricsCollector, manager JobManager) *HealthMetrics {
	return &HealthMetrics{
		collector: collector,
		manager:   manager,
	}
}

// CheckHealth evaluates system health based on metrics
func (hm *HealthMetrics) CheckHealth(ctx context.Context) map[string]interface{} {
	stats, _ := hm.manager.GetJobStats(ctx)
	procStats := hm.manager.GetProcessorStats()

	health := map[string]interface{}{
		"timestamp": time.Now(),
		"queue": map[string]interface{}{
			"total":     stats.Total,
			"pending":   stats.Pending,
			"running":   stats.Running,
			"completed": stats.Completed,
			"failed":    stats.Failed,
		},
		"processor": map[string]interface{}{
			"workers_active": procStats.WorkersActive,
			"workers_total":  procStats.WorkersTotal,
		},
		"status": hm.determineStatus(stats, procStats),
	}

	return health
}

// determineStatus determines overall system status
func (hm *HealthMetrics) determineStatus(stats *QueueStats, procStats *ProcessorStats) string {
	if stats == nil || procStats == nil {
		return "unknown"
	}

	// Check if queue is backing up
	if stats.Pending > 1000 {
		return "degraded"
	}

	// Check if failure rate is high
	if stats.Failed > stats.Completed {
		return "warning"
	}

	// Check if workers are all busy
	if procStats.WorkersActive == procStats.WorkersTotal && stats.Pending > 0 {
		return "busy"
	}

	return "healthy"
}
