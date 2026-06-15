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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	mfaMetricsKVKey = "gatekeeper:metrics:collector"
	mfaMetricsTTL   = 3 * time.Second
)

// Collector gathers metrics for MFA operations.
type Collector struct {
	mu sync.RWMutex

	// Counter metrics
	EnrollmentsTotal      prometheus.Counter
	VerificationsTotal    prometheus.Counter
	VerificationFailures  prometheus.Counter
	BackupCodesUsed       prometheus.Counter
	TrustedDevicesCreated prometheus.Counter

	// Gauge metrics
	ActiveFactorsTotal prometheus.Gauge
	HighRiskEvents     prometheus.Gauge

	// Histogram metrics (latency)
	VerificationDuration prometheus.Histogram
	EnrollmentDuration   prometheus.Histogram

	// Atomic counters for KV persistence (survive restarts)
	totalEnrollments   atomic.Int64
	totalVerifications atomic.Int64
	totalFailures      atomic.Int64
	totalBackupUsed    atomic.Int64
	totalDevicesCreated atomic.Int64
	activeFactors      atomic.Int64
	highRiskCount      atomic.Int64
	startTime          time.Time

	// KV store for Raft persistence
	kvStore platformstore.KVStore
}

// mfaCollectorState is a serializable snapshot of the Collector's state.
type mfaCollectorState struct {
	TotalEnrollments   int64     `json:"totalEnrollments"`
	TotalVerifications int64     `json:"totalVerifications"`
	TotalFailures      int64     `json:"totalFailures"`
	TotalBackupUsed    int64     `json:"totalBackupUsed"`
	TotalDevicesCreated int64   `json:"totalDevicesCreated"`
	ActiveFactors      int64     `json:"activeFactors"`
	HighRiskCount      int64     `json:"highRiskCount"`
	StartTime          time.Time `json:"startTime"`
}

// ConfigureKVPersistence enables KVStore-backed persistence for MFA metrics.
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

	ctx, cancel := context.WithTimeout(context.Background(), mfaMetricsTTL)
	defer cancel()

	val, err := kv.Get(ctx, mfaMetricsKVKey)
	if err != nil || val == "" {
		return // not found or empty
	}

	var state mfaCollectorState
	if err := json.Unmarshal([]byte(val), &state); err != nil {
		logging.Z().Info(fmt.Sprintf("⚠️  gatekeeper metrics: failed to unmarshal state: %v", err))
		return
	}

	c.totalEnrollments.Store(state.TotalEnrollments)
	c.totalVerifications.Store(state.TotalVerifications)
	c.totalFailures.Store(state.TotalFailures)
	c.totalBackupUsed.Store(state.TotalBackupUsed)
	c.totalDevicesCreated.Store(state.TotalDevicesCreated)
	c.activeFactors.Store(state.ActiveFactors)
	c.highRiskCount.Store(state.HighRiskCount)
	c.startTime = state.StartTime

	// Re-apply counters to Prometheus gauges
	c.ActiveFactorsTotal.Set(float64(state.ActiveFactors))
	c.HighRiskEvents.Set(float64(state.HighRiskCount))

	logging.Z().Info(fmt.Sprintf("✅ gatekeeper metrics: loaded persistent state (enrollments=%d, verifications=%d)",
		state.TotalEnrollments, state.TotalVerifications))
}

func (c *Collector) save() {
	c.mu.RLock()
	kv := c.kvStore
	c.mu.RUnlock()
	if kv == nil {
		return
	}

	state := mfaCollectorState{
		TotalEnrollments:    c.totalEnrollments.Load(),
		TotalVerifications:  c.totalVerifications.Load(),
		TotalFailures:       c.totalFailures.Load(),
		TotalBackupUsed:     c.totalBackupUsed.Load(),
		TotalDevicesCreated: c.totalDevicesCreated.Load(),
		ActiveFactors:       c.activeFactors.Load(),
		HighRiskCount:       c.highRiskCount.Load(),
		StartTime:           c.startTime,
	}

	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), mfaMetricsTTL)
	defer cancel()
	if err := kv.Put(ctx, mfaMetricsKVKey, string(data)); err != nil {
		logging.Z().Error(fmt.Sprintf("gatekeeper metrics: kv persist failed: %v", err))
	}
}

// NewCollector creates a new metrics collector with Prometheus.
func NewCollector() *Collector {
	return &Collector{
		startTime: time.Now(),
		EnrollmentsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "axiom_mfa",
			Name:      "enrollments_total",
			Help:      "Total number of factor enrollments",
		}),
		VerificationsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "axiom_mfa",
			Name:      "verifications_total",
			Help:      "Total number of successful MFA verifications",
		}),
		VerificationFailures: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "axiom_mfa",
			Name:      "verification_failures_total",
			Help:      "Total number of failed MFA verifications",
		}),
		BackupCodesUsed: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "axiom_mfa",
			Name:      "backup_codes_used_total",
			Help:      "Total number of backup codes consumed",
		}),
		TrustedDevicesCreated: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "axiom_mfa",
			Name:      "trusted_devices_total",
			Help:      "Total number of trusted devices created",
		}),
		ActiveFactorsTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "axiom_mfa",
			Name:      "active_factors",
			Help:      "Number of active MFA factors",
		}),
		HighRiskEvents: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "axiom_mfa",
			Name:      "high_risk_events",
			Help:      "Number of high-risk authentication events",
		}),
		VerificationDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: "axiom_mfa",
			Name:      "verification_duration_seconds",
			Help:      "Time taken to verify MFA code",
			Buckets:   prometheus.DefBuckets,
		}),
		EnrollmentDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: "axiom_mfa",
			Name:      "enrollment_duration_seconds",
			Help:      "Time taken to complete MFA enrollment",
			Buckets:   prometheus.DefBuckets,
		}),
	}
}

// RecordEnrollment increments enrollment counter.
func (c *Collector) RecordEnrollment() {
	c.EnrollmentsTotal.Inc()
	c.totalEnrollments.Add(1)
	go c.save()
}

// RecordVerification increments verification counter.
func (c *Collector) RecordVerification() {
	c.VerificationsTotal.Inc()
	c.totalVerifications.Add(1)
	go c.save()
}

// RecordVerificationFailure increments failure counter.
func (c *Collector) RecordVerificationFailure() {
	c.VerificationFailures.Inc()
	c.totalFailures.Add(1)
	go c.save()
}

// RecordBackupCodeUsed increments backup code usage counter.
func (c *Collector) RecordBackupCodeUsed() {
	c.BackupCodesUsed.Inc()
	c.totalBackupUsed.Add(1)
	go c.save()
}

// RecordTrustedDeviceCreated increments trusted device counter.
func (c *Collector) RecordTrustedDeviceCreated() {
	c.TrustedDevicesCreated.Inc()
	c.totalDevicesCreated.Add(1)
	go c.save()
}

// SetActiveFactors sets the current number of active factors.
func (c *Collector) SetActiveFactors(count float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ActiveFactorsTotal.Set(count)
	c.activeFactors.Store(int64(count))
	go c.save()
}

// SetHighRiskEvents sets the current number of high-risk events.
func (c *Collector) SetHighRiskEvents(count float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.HighRiskEvents.Set(count)
	c.highRiskCount.Store(int64(count))
	go c.save()
}

// RecordVerificationDuration records verification latency.
func (c *Collector) RecordVerificationDuration(duration time.Duration) {
	c.VerificationDuration.Observe(duration.Seconds())
}

// RecordEnrollmentDuration records enrollment latency.
func (c *Collector) RecordEnrollmentDuration(duration time.Duration) {
	c.EnrollmentDuration.Observe(duration.Seconds())
}

// SimpleMetrics provides a non-Prometheus implementation for testing.
type SimpleMetrics struct {
	mu                      sync.RWMutex
	Enrollments             int64
	Verifications           int64
	VerificationFailures    int64
	BackupCodesUsed         int64
	TrustedDevicesCreated   int64
	ActiveFactorsCount      int64
	HighRiskEventsCount     int64
	VerificationDurationSum time.Duration
	EnrollmentDurationSum   time.Duration
}

// RecordEnrollment increments enrollment counter.
func (s *SimpleMetrics) RecordEnrollment() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Enrollments++
}

// RecordVerification increments verification counter.
func (s *SimpleMetrics) RecordVerification() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Verifications++
}

// RecordVerificationFailure increments failure counter.
func (s *SimpleMetrics) RecordVerificationFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.VerificationFailures++
}

// RecordBackupCodeUsed increments backup code counter.
func (s *SimpleMetrics) RecordBackupCodeUsed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BackupCodesUsed++
}

// RecordTrustedDeviceCreated increments trusted device counter.
func (s *SimpleMetrics) RecordTrustedDeviceCreated() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TrustedDevicesCreated++
}

// SetActiveFactors sets active factors gauge.
func (s *SimpleMetrics) SetActiveFactors(count float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ActiveFactorsCount = int64(count)
}

// SetHighRiskEvents sets high-risk events gauge.
func (s *SimpleMetrics) SetHighRiskEvents(count float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.HighRiskEventsCount = int64(count)
}

// RecordVerificationDuration records verification latency.
func (s *SimpleMetrics) RecordVerificationDuration(duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.VerificationDurationSum += duration
}

// RecordEnrollmentDuration records enrollment latency.
func (s *SimpleMetrics) RecordEnrollmentDuration(duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.EnrollmentDurationSum += duration
}
