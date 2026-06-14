package metrics

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
)

const (
	metricsKVKey   = "storage:metrics:collector"
	metricsTimeout = 3 * time.Second
)

// Collector aggregates storage operation metrics.
type Collector struct {
	mu      sync.RWMutex
	buckets map[string]*bucketMetric // key = tenantID/bucketName

	totalRequests  atomic.Int64
	totalBytesIn   atomic.Int64
	totalBytesOut  atomic.Int64
	totalErrors    atomic.Int64
	totalLatencyNs atomic.Int64

	startTime time.Time
	kvStore   platformstore.KVStore
}

// collectorState is a serializable snapshot of the Collector's state.
type collectorState struct {
	Buckets        map[string]*bucketMetric `json:"buckets"`
	TotalRequests  int64                    `json:"totalRequests"`
	TotalBytesIn   int64                    `json:"totalBytesIn"`
	TotalBytesOut  int64                    `json:"totalBytesOut"`
	TotalErrors    int64                    `json:"totalErrors"`
	TotalLatencyNs int64                    `json:"totalLatencyNs"`
	StartTime      time.Time                `json:"startTime"`
}

type bucketMetric struct {
	requests   int64
	gets       int64
	puts       int64
	deletes    int64
	bytesIn    int64
	bytesOut   int64
	errors     int64
	latency    int64 // cumulative nanoseconds
	count      int64 // for average
	lastAccess time.Time
}

// NewCollector creates a new metrics collector.
func NewCollector() *Collector {
	return &Collector{
		buckets:   make(map[string]*bucketMetric),
		startTime: time.Now(),
	}
}

// ConfigureKVPersistence enables KVStore-backed persistence for metrics.
func (c *Collector) ConfigureKVPersistence(kv platformstore.KVStore) {
	c.mu.Lock()
	c.kvStore = kv
	c.mu.Unlock()
	c.load()
}

func (c *Collector) load() {
	c.mu.Lock()
	kv := c.kvStore
	c.mu.Unlock()
	if kv == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), metricsTimeout)
	defer cancel()

	val, err := kv.Get(ctx, metricsKVKey)
	if err != nil || val == "" {
		return // not found or empty
	}

	var state collectorState
	if err := json.Unmarshal([]byte(val), &state); err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  storage metrics: failed to unmarshal state: %v", err))
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.buckets = state.Buckets
	if c.buckets == nil {
		c.buckets = make(map[string]*bucketMetric)
	}
	c.totalRequests.Store(state.TotalRequests)
	c.totalBytesIn.Store(state.TotalBytesIn)
	c.totalBytesOut.Store(state.TotalBytesOut)
	c.totalErrors.Store(state.TotalErrors)
	c.totalLatencyNs.Store(state.TotalLatencyNs)
	c.startTime = state.StartTime
	logging.Z().Info(fmt.Sprintf("✅ storage metrics: loaded persistent state (requests=%d)", state.TotalRequests))
}

func (c *Collector) save() {
	c.mu.RLock()
	kv := c.kvStore
	if kv == nil {
		c.mu.RUnlock()
		return
	}

	state := collectorState{
		Buckets:        c.buckets,
		TotalRequests:  c.totalRequests.Load(),
		TotalBytesIn:   c.totalBytesIn.Load(),
		TotalBytesOut:  c.totalBytesOut.Load(),
		TotalErrors:    c.totalErrors.Load(),
		TotalLatencyNs: c.totalLatencyNs.Load(),
		StartTime:      c.startTime,
	}
	c.mu.RUnlock()

	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), metricsTimeout)
	defer cancel()
	if err := kv.Put(ctx, metricsKVKey, string(data)); err != nil {
		logging.Z().Error(fmt.Sprintf("storage metrics: kv persist failed: %v", err))
	}
}

func bKey(tenantID, bucket string) string {
	return tenantID + "/" + bucket
}

// RecordRequest tracks a storage API request.
func (c *Collector) RecordRequest(tenantID, bucket, op string, bytes int64, latency time.Duration, isError bool) {
	c.totalRequests.Add(1)
	c.totalLatencyNs.Add(int64(latency))

	if isError {
		c.totalErrors.Add(1)
	}

	c.mu.Lock()
	k := bKey(tenantID, bucket)
	m, ok := c.buckets[k]
	if !ok {
		m = &bucketMetric{}
		c.buckets[k] = m
	}

	m.requests++
	m.latency += int64(latency)
	m.count++
	m.lastAccess = time.Now()

	if isError {
		m.errors++
	}

	switch op {
	case "GET":
		m.gets++
		m.bytesOut += bytes
		c.totalBytesOut.Add(bytes)
	case "PUT":
		m.puts++
		m.bytesIn += bytes
		c.totalBytesIn.Add(bytes)
	case "DELETE":
		m.deletes++
	}
	c.mu.Unlock()

	// Async save to avoid blocking the request path.
	go c.save()
}

// GetBucketMetrics returns metrics for a specific bucket.
func (c *Collector) GetBucketMetrics(tenantID, bucket string) models.BucketMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	m, ok := c.buckets[bKey(tenantID, bucket)]
	if !ok {
		return models.BucketMetrics{BucketName: bucket, TenantID: tenantID, CollectedAt: time.Now()}
	}

	avgLat := float64(0)
	if m.count > 0 {
		avgLat = float64(m.latency) / float64(m.count) / float64(time.Millisecond)
	}

	return models.BucketMetrics{
		BucketName:     bucket,
		TenantID:       tenantID,
		RequestCount:   m.requests,
		GetRequests:    m.gets,
		PutRequests:    m.puts,
		DeleteRequests: m.deletes,
		BytesIn:        m.bytesIn,
		BytesOut:       m.bytesOut,
		ErrorCount:     m.errors,
		AvgLatencyMs:   avgLat,
		LastAccessed:   m.lastAccess,
		CollectedAt:    time.Now(),
	}
}

// GetAllBucketMetrics returns metrics for all tracked buckets.
func (c *Collector) GetAllBucketMetrics() []models.BucketMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]models.BucketMetrics, 0, len(c.buckets))
	for _, m := range c.buckets {
		avgLat := float64(0)
		if m.count > 0 {
			avgLat = float64(m.latency) / float64(m.count) / float64(time.Millisecond)
		}
		result = append(result, models.BucketMetrics{
			RequestCount:   m.requests,
			GetRequests:    m.gets,
			PutRequests:    m.puts,
			DeleteRequests: m.deletes,
			BytesIn:        m.bytesIn,
			BytesOut:       m.bytesOut,
			ErrorCount:     m.errors,
			AvgLatencyMs:   avgLat,
			LastAccessed:   m.lastAccess,
			CollectedAt:    time.Now(),
		})
	}
	return result
}

// GetSystemMetrics returns aggregate system-level metrics.
func (c *Collector) GetSystemMetrics(bucketCount, objectCount int, totalSize int64, tenantCount, policyCount int, healthy bool) models.SystemMetrics {
	total := c.totalRequests.Load()
	avgLat := float64(0)
	if total > 0 {
		avgLat = float64(c.totalLatencyNs.Load()) / float64(total) / float64(time.Millisecond)
	}

	uptime := time.Since(c.startTime)
	uptimeStr := uptime.Round(time.Second).String()

	return models.SystemMetrics{
		Uptime:         uptimeStr,
		TotalBuckets:   bucketCount,
		TotalObjects:   int64(objectCount),
		TotalSizeBytes: totalSize,
		TenantCount:    tenantCount,
		TotalRequests:  total,
		TotalBytesIn:   c.totalBytesIn.Load(),
		TotalBytesOut:  c.totalBytesOut.Load(),
		TotalErrors:    c.totalErrors.Load(),
		ActivePolicies: policyCount,
		BackendHealthy: healthy,
		AvgLatencyMs:   avgLat,
	}
}
