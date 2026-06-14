package costing

// =====================================================
// WS-4.4 — Real-Time Usage Tracking Middleware
//
// Gin middleware that intercepts API requests and records
// usage metrics (request count, bandwidth, latency) as
// models.UsageRecordResources for cost attribution.
// =====================================================

import (
	"context"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/costing/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"github.com/gin-gonic/gin"

	"go.uber.org/zap"
)

// UsageTracker records API usage for cost attribution.
type UsageTracker struct {
	store     store.ResourceStore[*models.UsageRecordResource]
	mu        sync.Mutex
	buffer    []models.UsageRecordSpec
	batchSize int
	flushInterval time.Duration
}

// NewUsageTracker creates a new tracker.
func NewUsageTracker(s store.ResourceStore[*models.UsageRecordResource], batchSize int) *UsageTracker {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &UsageTracker{
		store:         s,
		batchSize:     batchSize,
		flushInterval: 30 * time.Second,
	}
}

// Middleware returns a Gin middleware that tracks API usage per tenant.
func (t *UsageTracker) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Process request.
		c.Next()

		// Record usage after response.
		duration := time.Since(start)
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = "default"
		}

		record := models.UsageRecordSpec{
			TenantID:  tenantID,
			Dimension: models.DimensionAPI,
			Quantity:  1,
			Credits:   0.001, // Default: 0.001 credits per API call
			Source:    fmt.Sprintf("%s %s", c.Request.Method, c.FullPath()),
			Timestamp: start,
			Metadata: map[string]string{
				"method":     c.Request.Method,
				"path":       c.FullPath(),
				"status":     fmt.Sprintf("%d", c.Writer.Status()),
				"latency_ms": fmt.Sprintf("%d", duration.Milliseconds()),
				"bytes":      fmt.Sprintf("%d", c.Writer.Size()),
			},
		}

		t.record(record)
	}
}

// TrackQuery records a query execution for cost attribution.
func (t *UsageTracker) TrackQuery(tenantID, queryID string, rowsScanned int64, duration time.Duration) {
	t.record(models.UsageRecordSpec{
		TenantID:  tenantID,
		Dimension: models.DimensionQuery,
		Quantity:  float64(rowsScanned),
		Credits:   float64(rowsScanned) * 0.000001, // 1 credit per million rows
		Source:    queryID,
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"rows_scanned": fmt.Sprintf("%d", rowsScanned),
			"duration_ms":  fmt.Sprintf("%d", duration.Milliseconds()),
		},
	})
}

// TrackPipeline records a pipeline execution for cost attribution.
func (t *UsageTracker) TrackPipeline(tenantID, pipelineName string, recordsProcessed int64, duration time.Duration) {
	t.record(models.UsageRecordSpec{
		TenantID:  tenantID,
		Dimension: models.DimensionPipeline,
		Quantity:  float64(recordsProcessed),
		Credits:   float64(recordsProcessed) * 0.00001, // 1 credit per 100K records
		Source:    pipelineName,
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"records":     fmt.Sprintf("%d", recordsProcessed),
			"duration_ms": fmt.Sprintf("%d", duration.Milliseconds()),
		},
	})
}

// TrackStorage records storage usage for cost attribution.
func (t *UsageTracker) TrackStorage(tenantID, bucketName string, bytesStored int64) {
	gbStored := float64(bytesStored) / (1024 * 1024 * 1024)
	t.record(models.UsageRecordSpec{
		TenantID:  tenantID,
		Dimension: models.DimensionStorage,
		Quantity:  gbStored,
		Credits:   gbStored * 0.023, // ~$0.023 per GB/month (S3-like pricing)
		Source:    bucketName,
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"bytes": fmt.Sprintf("%d", bytesStored),
		},
	})
}

// record buffers a usage record and flushes when batch is full.
func (t *UsageTracker) record(spec models.UsageRecordSpec) {
	t.mu.Lock()
	t.buffer = append(t.buffer, spec)
	shouldFlush := len(t.buffer) >= t.batchSize
	t.mu.Unlock()

	if shouldFlush {
		t.Flush(context.Background())
	}
}

// Flush writes all buffered records to the store.
func (t *UsageTracker) Flush(ctx context.Context) {
	t.mu.Lock()
	if len(t.buffer) == 0 {
		t.mu.Unlock()
		return
	}
	batch := t.buffer
	t.buffer = nil
	t.mu.Unlock()

	if t.store == nil {
		return
	}

	for i, spec := range batch {
		// Respect context cancellation.
		select {
		case <-ctx.Done():
			return
		default:
		}

		record := &models.UsageRecordResource{
			Spec: spec,
		}
		record.Kind = models.UsageRecordKind
		record.APIVersion = models.UsageRecordAPIVersion
		record.Name = fmt.Sprintf("usage-%s-%d-%d", spec.TenantID, spec.Timestamp.UnixNano(), i)
		record.CreatedAt = spec.Timestamp
		record.Generation = 1
		record.Status.Phase = "Recorded"

		if err := t.store.Create(ctx, record); err != nil {
			logging.Z().Warn("usage record creation failed",
				zap.String("tenant", spec.TenantID),
				zap.String("dimension", string(spec.Dimension)),
				zap.Error(err),
			)
		}
	}
}

// BufferSize returns the current number of buffered records.
func (t *UsageTracker) BufferSize() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.buffer)
}
