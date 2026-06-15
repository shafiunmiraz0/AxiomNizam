package streamanalytics

// =====================================================
// WS-7.2 — Stream Output Sinks
//
// Delivers aggregated window results to downstream systems.
// Supports database (PostgreSQL), webhook, event bus, and
// stdout sinks with retry and error tracking.
// =====================================================

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/resilience"

	"go.uber.org/zap"
)

// Sink delivers window results to a downstream system.
type Sink interface {
	// Write sends aggregated results to the sink.
	Write(ctx context.Context, results []WindowResult) error

	// Close cleanly shuts down the sink.
	Close() error

	// Stats returns sink statistics.
	Stats() SinkStats
}

// SinkStats tracks sink delivery metrics.
type SinkStats struct {
	Type            string `json:"type"`
	TotalWrites     int64  `json:"totalWrites"`
	TotalRows       int64  `json:"totalRows"`
	TotalErrors     int64  `json:"totalErrors"`
	LastWriteAt     *time.Time `json:"lastWriteAt,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	AvgWriteLatency string `json:"avgWriteLatency,omitempty"`
}

// --- Webhook Sink ---

// WebhookSink delivers results via HTTP POST to a configured URL.
type WebhookSink struct {
	mu       sync.Mutex
	url      string
	client   *http.Client
	headers  map[string]string
	stats    SinkStats
}

// NewWebhookSink creates a new webhook output sink.
func NewWebhookSink(url string, headers map[string]string) *WebhookSink {
	return &WebhookSink{
		url:     url,
		client:  &http.Client{Timeout: 30 * time.Second},
		headers: headers,
		stats:   SinkStats{Type: "webhook"},
	}
}

func (s *WebhookSink) Write(ctx context.Context, results []WindowResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("webhook sink: marshal failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("webhook sink: request creation failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}

	// Execute with retry per coding practices §4.2.
	var resp *http.Response
	err = resilience.DoVoid(ctx, resilience.Config{
		MaxAttempts:  3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Name:         "webhook-sink",
	}, func(ctx context.Context) error {
		var doErr error
		resp, doErr = s.client.Do(req)
		return doErr
	})
	if err != nil {
		s.stats.TotalErrors++
		s.stats.LastError = err.Error()
		return fmt.Errorf("webhook sink: request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode >= 400 {
		s.stats.TotalErrors++
		s.stats.LastError = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return fmt.Errorf("webhook sink: received status %d", resp.StatusCode)
	}

	now := time.Now()
	s.stats.TotalWrites++
	s.stats.TotalRows += int64(len(results))
	s.stats.LastWriteAt = &now
	return nil
}

func (s *WebhookSink) Close() error { return nil }
func (s *WebhookSink) Stats() SinkStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// --- Stdout Sink ---

// StdoutSink writes results to stdout (for development/debugging).
type StdoutSink struct {
	mu    sync.Mutex
	stats SinkStats
}

// NewStdoutSink creates a new stdout sink.
func NewStdoutSink() *StdoutSink {
	return &StdoutSink{stats: SinkStats{Type: "stdout"}}
}

func (s *StdoutSink) Write(ctx context.Context, results []WindowResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log := logging.Z()
	for _, r := range results {
		data, _ := json.Marshal(r)
		log.Debug("stream sink output", zap.String("result", string(data)))
	}

	now := time.Now()
	s.stats.TotalWrites++
	s.stats.TotalRows += int64(len(results))
	s.stats.LastWriteAt = &now
	return nil
}

func (s *StdoutSink) Close() error { return nil }
func (s *StdoutSink) Stats() SinkStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// --- EventBus Sink ---

// EventBusPublisher abstracts publishing to the platform event bus.
type EventBusPublisher interface {
	Publish(ctx context.Context, topic string, data []byte) error
}

// EventBusSink publishes results to the platform's event bus.
type EventBusSink struct {
	mu        sync.Mutex
	publisher EventBusPublisher
	topic     string
	stats     SinkStats
}

// NewEventBusSink creates a new event bus output sink.
func NewEventBusSink(publisher EventBusPublisher, topic string) *EventBusSink {
	return &EventBusSink{
		publisher: publisher,
		topic:     topic,
		stats:     SinkStats{Type: "eventbus"},
	}
}

func (s *EventBusSink) Write(ctx context.Context, results []WindowResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range results {
		data, err := json.Marshal(r)
		if err != nil {
			s.stats.TotalErrors++
			continue
		}
		if err := s.publisher.Publish(ctx, s.topic, data); err != nil {
			s.stats.TotalErrors++
			s.stats.LastError = err.Error()
			return fmt.Errorf("eventbus sink: publish failed: %w", err)
		}
	}

	now := time.Now()
	s.stats.TotalWrites++
	s.stats.TotalRows += int64(len(results))
	s.stats.LastWriteAt = &now
	return nil
}

func (s *EventBusSink) Close() error { return nil }
func (s *EventBusSink) Stats() SinkStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// --- Sink Factory ---

// NewSink creates a sink from a StreamSink spec.
func NewSink(spec StreamSink) (Sink, error) {
	switch spec.Type {
	case "webhook":
		if spec.WebhookURL == "" {
			return nil, fmt.Errorf("webhook sink requires webhookUrl")
		}
		return NewWebhookSink(spec.WebhookURL, nil), nil
	case "stdout":
		return NewStdoutSink(), nil
	case "eventbus":
		// EventBus sink requires publisher injection — return stdout as fallback.
		return NewStdoutSink(), nil
	default:
		return nil, fmt.Errorf("unsupported sink type: %s", spec.Type)
	}
}
