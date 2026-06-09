package securitymon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// SIEMExporter exports audit events to an external SIEM system.
// Supports webhook (HTTP POST), file, and stdout output.
type SIEMExporter struct {
	webhookURL string
	filePath   string
	httpClient *http.Client
	metrics    *SecurityMetrics
}

// SIEMEvent is the event format exported to the SIEM.
type SIEMEvent struct {
	Timestamp   time.Time              `json:"timestamp"`
	EventType   string                 `json:"event_type"`
	Severity    string                 `json:"severity"`
	UserID      string                 `json:"user_id,omitempty"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	Resource    string                 `json:"resource,omitempty"`
	Action      string                 `json:"action,omitempty"`
	Outcome     string                 `json:"outcome"`
	Message     string                 `json:"message"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Source      string                 `json:"source"` // "axiomnizam"
}

// NewSIEMExporter creates a new SIEM exporter.
// webhookURL: if set, events are POSTed as JSON to this URL.
// filePath: if set, events are appended to this file as JSON lines.
// If both are empty, events are logged to stdout.
func NewSIEMExporter(webhookURL, filePath string, metrics *SecurityMetrics) *SIEMExporter {
	return &SIEMExporter{
		webhookURL: webhookURL,
		filePath:   filePath,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		metrics:    metrics,
	}
}

// LoadSIEMExporterFromEnv creates a SIEM exporter from environment variables.
// SIEM_WEBHOOK_URL: HTTP endpoint for SIEM events
// SIEM_EXPORT_FILE: file path for JSON lines export
func LoadSIEMExporterFromEnv(metrics *SecurityMetrics) *SIEMExporter {
	return NewSIEMExporter(
		os.Getenv("SIEM_WEBHOOK_URL"),
		os.Getenv("SIEM_EXPORT_FILE"),
		metrics,
	)
}

// Export sends a single event to the configured SIEM destination.
func (e *SIEMExporter) Export(ctx context.Context, event SIEMEvent) error {
	event.Source = "axiomnizam"

	if e.webhookURL != "" {
		return e.exportWebhook(ctx, event)
	}
	if e.filePath != "" {
		return e.exportFile(event)
	}
	// Default: log to stdout
	return e.exportStdout(event)
}

// ExportBatch sends multiple events.
func (e *SIEMExporter) ExportBatch(ctx context.Context, events []SIEMEvent) error {
	for _, evt := range events {
		if err := e.Export(ctx, evt); err != nil {
			if e.metrics != nil {
				e.metrics.RecordSIEMExport(false)
			}
			return err
		}
		if e.metrics != nil {
			e.metrics.RecordSIEMExport(true)
		}
	}
	return nil
}

func (e *SIEMExporter) exportWebhook(ctx context.Context, event SIEMEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal SIEM event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create SIEM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AxiomNizam-SIEM/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send SIEM event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("SIEM webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func (e *SIEMExporter) exportFile(event SIEMEvent) error {
	f, err := os.OpenFile(e.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open SIEM export file: %w", err)
	}
	defer f.Close()

	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal SIEM event: %w", err)
	}
	line = append(line, '\n')
	_, err = f.Write(line)
	return err
}

func (e *SIEMExporter) exportStdout(event SIEMEvent) error {
	data, _ := json.Marshal(event)
	log.Printf("[SIEM] %s", string(data))
	return nil
}
