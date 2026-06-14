package alerting

// =====================================================
// WS-4.2 — Notification Channels
//
// Multi-channel notification dispatch for alert incidents.
// Supports Slack, email (SMTP), webhook, PagerDuty, and
// Microsoft Teams.
// =====================================================

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/alerting/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/resilience"
)

// NotificationMessage is the payload sent to a channel.
type NotificationMessage struct {
	Title       string            `json:"title"`
	Body        string            `json:"body"`
	Severity    string            `json:"severity"`
	RuleName    string            `json:"ruleName"`
	IncidentID  string            `json:"incidentId,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	FiredAt     time.Time         `json:"firedAt"`
	ResolvedAt  *time.Time        `json:"resolvedAt,omitempty"`
	Status      string            `json:"status"` // firing, resolved
}

// ChannelDispatcher sends notifications through configured channels.
type ChannelDispatcher struct {
	httpClient *http.Client
}

// NewChannelDispatcher creates a new dispatcher.
func NewChannelDispatcher() *ChannelDispatcher {
	return &ChannelDispatcher{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Dispatch sends a notification through the specified channel with retry.
func (d *ChannelDispatcher) Dispatch(ctx context.Context, channel *models.NotificationChannelResource, msg NotificationMessage) error {
	return resilience.DoVoid(ctx, resilience.Config{
		MaxAttempts:  3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		Name:         "notify-" + channel.Name,
	}, func(ctx context.Context) error {
		return d.dispatchOnce(ctx, channel, msg)
	})
}

// dispatchOnce sends a single notification attempt.
func (d *ChannelDispatcher) dispatchOnce(ctx context.Context, channel *models.NotificationChannelResource, msg NotificationMessage) error {
	switch models.ChannelType(channel.Spec.Type) {
	case models.ChannelTypeSlack:
		return d.sendSlack(ctx, channel, msg)
	case models.ChannelTypeEmail:
		return d.sendEmail(ctx, channel, msg)
	case models.ChannelTypeWebhook:
		return d.sendWebhook(ctx, channel, msg)
	case models.ChannelTypePagerDuty:
		return d.sendPagerDuty(ctx, channel, msg)
	case models.ChannelTypeTeams:
		return d.sendTeams(ctx, channel, msg)
	default:
		return fmt.Errorf("unsupported channel type: %s", channel.Spec.Type)
	}
}

// sendSlack sends a notification via Slack webhook.
func (d *ChannelDispatcher) sendSlack(ctx context.Context, channel *models.NotificationChannelResource, msg NotificationMessage) error {
	webhookURL := channel.Spec.Config["webhookUrl"]
	if webhookURL == "" {
		return fmt.Errorf("slack channel missing webhookUrl config")
	}

	// Build Slack message payload.
	color := "#36a64f" // green
	if msg.Status == "firing" {
		switch msg.Severity {
		case "critical":
			color = "#ff0000"
		case "warning":
			color = "#ffaa00"
		default:
			color = "#0088ff"
		}
	}

	slackPayload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":  color,
				"title":  msg.Title,
				"text":   msg.Body,
				"footer": fmt.Sprintf("AxiomNizam Alerting | %s", msg.RuleName),
				"ts":     msg.FiredAt.Unix(),
				"fields": []map[string]interface{}{
					{"title": "Severity", "value": msg.Severity, "short": true},
					{"title": "Status", "value": msg.Status, "short": true},
				},
			},
		},
	}

	// Add channel mention if configured.
	if ch := channel.Spec.Config["channel"]; ch != "" {
		slackPayload["channel"] = ch
	}

	return d.postJSON(ctx, webhookURL, slackPayload)
}

// sendEmail sends a notification via SMTP.
func (d *ChannelDispatcher) sendEmail(_ context.Context, channel *models.NotificationChannelResource, msg NotificationMessage) error {
	host := channel.Spec.Config["smtpHost"]
	port := channel.Spec.Config["smtpPort"]
	from := channel.Spec.Config["from"]
	to := channel.Spec.Config["to"]
	user := channel.Spec.Config["smtpUser"]
	password := channel.Spec.Config["smtpPassword"]

	if host == "" || from == "" || to == "" {
		return fmt.Errorf("email channel missing required config (smtpHost, from, to)")
	}

	if port == "" {
		port = "587"
	}

	recipients := strings.Split(to, ",")
	subject := fmt.Sprintf("[%s] %s - %s", strings.ToUpper(msg.Severity), msg.Status, msg.Title)

	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\n\nRule: %s\nSeverity: %s\nStatus: %s\nFired At: %s\n",
		from, to, subject, msg.Body, msg.RuleName, msg.Severity, msg.Status, msg.FiredAt.Format(time.RFC3339))

	addr := fmt.Sprintf("%s:%s", host, port)

	var auth smtp.Auth
	if user != "" && password != "" {
		auth = smtp.PlainAuth("", user, password, host)
	}

	return smtp.SendMail(addr, auth, from, recipients, []byte(body))
}

// sendWebhook sends a notification via HTTP POST.
func (d *ChannelDispatcher) sendWebhook(ctx context.Context, channel *models.NotificationChannelResource, msg NotificationMessage) error {
	url := channel.Spec.Config["url"]
	if url == "" {
		return fmt.Errorf("webhook channel missing url config")
	}

	payload := map[string]interface{}{
		"title":      msg.Title,
		"body":       msg.Body,
		"severity":   msg.Severity,
		"status":     msg.Status,
		"ruleName":   msg.RuleName,
		"incidentId": msg.IncidentID,
		"firedAt":    msg.FiredAt.Format(time.RFC3339),
		"labels":     msg.Labels,
	}

	if msg.ResolvedAt != nil {
		payload["resolvedAt"] = msg.ResolvedAt.Format(time.RFC3339)
	}

	return d.postJSON(ctx, url, payload)
}

// sendPagerDuty sends a notification via PagerDuty Events API v2.
func (d *ChannelDispatcher) sendPagerDuty(ctx context.Context, channel *models.NotificationChannelResource, msg NotificationMessage) error {
	routingKey := channel.Spec.Config["routingKey"]
	if routingKey == "" {
		return fmt.Errorf("pagerduty channel missing routingKey config")
	}

	eventAction := "trigger"
	if msg.Status == "resolved" {
		eventAction = "resolve"
	}

	pdSeverity := "warning"
	switch msg.Severity {
	case "critical":
		pdSeverity = "critical"
	case "warning":
		pdSeverity = "warning"
	case "info":
		pdSeverity = "info"
	}

	payload := map[string]interface{}{
		"routing_key":  routingKey,
		"event_action": eventAction,
		"dedup_key":    msg.RuleName,
		"payload": map[string]interface{}{
			"summary":   msg.Title,
			"severity":  pdSeverity,
			"source":    "axiomnizam",
			"component": msg.RuleName,
			"group":     "alerting",
			"custom_details": map[string]interface{}{
				"body":   msg.Body,
				"labels": msg.Labels,
			},
		},
	}

	return d.postJSON(ctx, "https://events.pagerduty.com/v2/enqueue", payload)
}

// sendTeams sends a notification via Microsoft Teams webhook.
func (d *ChannelDispatcher) sendTeams(ctx context.Context, channel *models.NotificationChannelResource, msg NotificationMessage) error {
	webhookURL := channel.Spec.Config["webhookUrl"]
	if webhookURL == "" {
		return fmt.Errorf("teams channel missing webhookUrl config")
	}

	themeColor := "00ff00"
	if msg.Status == "firing" {
		switch msg.Severity {
		case "critical":
			themeColor = "ff0000"
		case "warning":
			themeColor = "ffaa00"
		default:
			themeColor = "0088ff"
		}
	}

	teamsPayload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": themeColor,
		"summary":    msg.Title,
		"sections": []map[string]interface{}{
			{
				"activityTitle": msg.Title,
				"facts": []map[string]string{
					{"name": "Severity", "value": msg.Severity},
					{"name": "Status", "value": msg.Status},
					{"name": "Rule", "value": msg.RuleName},
					{"name": "Fired At", "value": msg.FiredAt.Format(time.RFC3339)},
				},
				"text": msg.Body,
			},
		},
	}

	return d.postJSON(ctx, webhookURL, teamsPayload)
}

// postJSON sends a JSON payload via HTTP POST.
func (d *ChannelDispatcher) postJSON(ctx context.Context, url string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("notification failed with status %d", resp.StatusCode)
	}

	return nil
}
