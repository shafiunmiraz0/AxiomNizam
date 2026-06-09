package securitymon

import (
	"log"
	"sync"
	"time"
)

// AnomalyDetector tracks per-IP and per-user request patterns.
// When a pattern exceeds the baseline by a configurable threshold,
// it flags the source as anomalous and triggers an alert callback.
type AnomalyDetector struct {
	mu             sync.RWMutex
	ipCounts       map[string]*slidingWindow
	userCounts     map[string]*slidingWindow
	baselineWindow time.Duration
	threshold      float64 // multiplier above baseline to trigger anomaly
	onAnomaly      func(AnomalyEvent)
}

// AnomalyEvent describes a detected anomaly.
type AnomalyEvent struct {
	Source    string    `json:"source"`    // IP address or user ID
	Type      string    `json:"type"`      // "ip_spike" or "user_spike"
	Count     int       `json:"count"`     // current count in window
	Baseline  float64   `json:"baseline"`  // rolling average
	Threshold float64   `json:"threshold"` // threshold multiplier
	Timestamp time.Time `json:"timestamp"`
}

// slidingWindow tracks request counts in a time-bucketed window.
type slidingWindow struct {
	buckets    map[int64]int // unix timestamp bucket → count
	bucketSize time.Duration
	maxBuckets int
}

func newSlidingWindow(bucketSize time.Duration, maxBuckets int) *slidingWindow {
	return &slidingWindow{
		buckets:    make(map[int64]int),
		bucketSize: bucketSize,
		maxBuckets: maxBuckets,
	}
}

func (w *slidingWindow) increment() {
	bucket := time.Now().UTC().UnixNano() / int64(w.bucketSize)
	w.buckets[bucket]++
	// Evict old buckets
	cutoff := time.Now().UTC().Add(-time.Duration(w.maxBuckets) * w.bucketSize).UnixNano() / int64(w.bucketSize)
	for k := range w.buckets {
		if k < cutoff {
			delete(w.buckets, k)
		}
	}
}

func (w *slidingWindow) totalCount() int {
	total := 0
	for _, v := range w.buckets {
		total += v
	}
	return total
}

func (w *slidingWindow) average() float64 {
	if len(w.buckets) == 0 {
		return 0
	}
	total := 0
	for _, v := range w.buckets {
		total += v
	}
	return float64(total) / float64(len(w.buckets))
}

// NewAnomalyDetector creates a new anomaly detector.
// baselineWindow: the rolling window for baseline calculation (e.g. 5 minutes).
// threshold: multiplier above baseline to trigger (e.g. 3.0 = 3x the average).
// onAnomaly: callback invoked when an anomaly is detected.
func NewAnomalyDetector(baselineWindow time.Duration, threshold float64, onAnomaly func(AnomalyEvent)) *AnomalyDetector {
	d := &AnomalyDetector{
		ipCounts:       make(map[string]*slidingWindow),
		userCounts:     make(map[string]*slidingWindow),
		baselineWindow: baselineWindow,
		threshold:      threshold,
		onAnomaly:      onAnomaly,
	}
	go d.cleanup()
	return d
}

// RecordRequest records a request from an IP by a user and checks for anomalies.
func (d *AnomalyDetector) RecordRequest(ip, userID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Track IP
	ipWin, ok := d.ipCounts[ip]
	if !ok {
		ipWin = newSlidingWindow(1*time.Minute, int(d.baselineWindow/time.Minute))
		d.ipCounts[ip] = ipWin
	}
	ipWin.increment()

	// Check IP anomaly
	ipCount := ipWin.totalCount()
	ipBaseline := ipWin.average()
	if ipBaseline > 0 && float64(ipCount) > ipBaseline*d.threshold {
		evt := AnomalyEvent{
			Source:    ip,
			Type:      "ip_spike",
			Count:     ipCount,
			Baseline:  ipBaseline,
			Threshold: d.threshold,
			Timestamp: time.Now().UTC(),
		}
		log.Printf("🚨 Anomaly detected: IP %s — %d requests (baseline: %.1f, threshold: %.1fx)",
			ip, ipCount, ipBaseline, d.threshold)
		if d.onAnomaly != nil {
			go d.onAnomaly(evt)
		}
	}

	// Track user
	if userID != "" {
		userWin, ok := d.userCounts[userID]
		if !ok {
			userWin = newSlidingWindow(1*time.Minute, int(d.baselineWindow/time.Minute))
			d.userCounts[userID] = userWin
		}
		userWin.increment()

		userCount := userWin.totalCount()
		userBaseline := userWin.average()
		if userBaseline > 0 && float64(userCount) > userBaseline*d.threshold {
			evt := AnomalyEvent{
				Source:    userID,
				Type:      "user_spike",
				Count:     userCount,
				Baseline:  userBaseline,
				Threshold: d.threshold,
				Timestamp: time.Now().UTC(),
			}
			log.Printf("🚨 Anomaly detected: user %s — %d requests (baseline: %.1f, threshold: %.1fx)",
				userID, userCount, userBaseline, d.threshold)
			if d.onAnomaly != nil {
				go d.onAnomaly(evt)
			}
		}
	}
}

// BaselineWindow returns the configured baseline window duration.
func (d *AnomalyDetector) BaselineWindow() time.Duration {
	return d.baselineWindow
}

// Threshold returns the configured anomaly threshold multiplier.
func (d *AnomalyDetector) Threshold() float64 {
	return d.threshold
}

// GetStats returns current tracking stats.
func (d *AnomalyDetector) GetStats() (uniqueIPs, uniqueUsers int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.ipCounts), len(d.userCounts)
}

// cleanup periodically removes stale tracking data.
func (d *AnomalyDetector) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		d.mu.Lock()
		// Remove IPs with no recent activity
		for ip, win := range d.ipCounts {
			if win.totalCount() == 0 {
				delete(d.ipCounts, ip)
			}
		}
		for uid, win := range d.userCounts {
			if win.totalCount() == 0 {
				delete(d.userCounts, uid)
			}
		}
		d.mu.Unlock()
	}
}
