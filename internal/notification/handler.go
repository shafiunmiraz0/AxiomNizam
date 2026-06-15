package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"

	"go.uber.org/zap"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler handles Discord notifications
type Handler struct {
	discordWebhookURL string
	connections       map[string]*gorm.DB
}

// NotificationRequest represents a notification request
type NotificationRequest struct {
	Title       string `json:"title" binding:"required"`
	Message     string `json:"message" binding:"required"`
	Type        string `json:"type"` // info, success, warning, error
	IncludeData bool   `json:"include_data"`
}

// DiscordEmbed represents a Discord embed object
type DiscordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Timestamp   string         `json:"timestamp"`
	Fields      []DiscordField `json:"fields,omitempty"`
}

// DiscordField represents a field in Discord embed
type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// DiscordMessage represents a Discord webhook message
type DiscordMessage struct {
	Content string         `json:"content"`
	Embeds  []DiscordEmbed `json:"embeds"`
}

// HealthStatusData represents health and status data
type HealthStatusData struct {
	Timestamp string                 `json:"timestamp"`
	Status    string                 `json:"status"`
	Databases map[string]interface{} `json:"databases"`
}

// NewHandler creates a new notification handler
func NewHandler(webhookURL string, connections map[string]*gorm.DB) *Handler {
	return &Handler{
		discordWebhookURL: webhookURL,
		connections:       connections,
	}
}

// SendNotification sends a notification to Discord
func (h *Handler) SendNotification(c *gin.Context) {
	var req NotificationRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Validate notification type
	validTypes := map[string]int{
		"info":    3447003,  // Blue
		"success": 65280,    // Green
		"warning": 16776960, // Yellow
		"error":   16711680, // Red
	}

	notifType := req.Type
	if notifType == "" {
		notifType = "info"
	}

	color, exists := validTypes[notifType]
	if !exists {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Invalid notification type: %s", notifType),
		})
		return
	}

	// Build embed
	embed := DiscordEmbed{
		Title:       req.Title,
		Description: req.Message,
		Color:       color,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	// Add health/status data if requested
	if req.IncludeData {
		healthData := h.getHealthStatusData()
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "System Status",
			Value:  healthData.Status,
			Inline: true,
		})
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "Timestamp",
			Value:  healthData.Timestamp,
			Inline: true,
		})
	}

	// Create Discord message
	msg := DiscordMessage{
		Content: fmt.Sprintf("🔔 **%s** - %s", req.Title, notifType),
		Embeds:  []DiscordEmbed{embed},
	}

	if h.discordWebhookURL == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Discord webhook URL is not configured",
		})
		return
	}

	// Send to Discord
	if err := h.sendToDiscord(msg); err != nil {
		logging.Z().Warn("failed to send Discord notification", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Failed to send notification: %v", err),
		})
		return
	}

	logging.Z().Info("notification sent to Discord", zap.String("title", req.Title))
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Notification sent to Discord",
		"title":   req.Title,
	})
}

// SendHealthNotification sends a health check notification
func (h *Handler) SendHealthNotification(c *gin.Context) {
	healthData := h.getHealthStatusData()

	embed := DiscordEmbed{
		Title:       "🏥 System Health Check",
		Description: "Automated health status notification",
		Color:       3447003, // Blue
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	// Add database status
	for dbName, status := range healthData.Databases {
		statusStr := fmt.Sprintf("%v", status)
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   fmt.Sprintf("%s Status", dbName),
			Value:  statusStr,
			Inline: true,
		})
	}

	embed.Fields = append(embed.Fields, DiscordField{
		Name:   "Timestamp",
		Value:  healthData.Timestamp,
		Inline: false,
	})

	msg := DiscordMessage{
		Content: "🔔 **Health Check** - All systems monitoring",
		Embeds:  []DiscordEmbed{embed},
	}

	if h.discordWebhookURL == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Discord webhook URL is not configured",
		})
		return
	}

	if err := h.sendToDiscord(msg); err != nil {
		logging.Z().Warn("failed to send health notification", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Failed to send health notification: %v", err),
		})
		return
	}

	logging.Z().Info("health notification sent to Discord")
	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"message":     "Health notification sent",
		"health_data": healthData,
	})
}

// SendStatusNotification sends a status notification
func (h *Handler) SendStatusNotification(c *gin.Context) {
	healthData := h.getHealthStatusData()

	embed := DiscordEmbed{
		Title:       "📊 System Status Report",
		Description: "Current system status snapshot",
		Color:       65280, // Green
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	// Add database status
	for dbName, status := range healthData.Databases {
		statusStr := fmt.Sprintf("%v", status)
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   fmt.Sprintf("%s Status", dbName),
			Value:  statusStr,
			Inline: true,
		})
	}

	embed.Fields = append(embed.Fields, DiscordField{
		Name:   "Report Time",
		Value:  healthData.Timestamp,
		Inline: false,
	})

	msg := DiscordMessage{
		Content: "🔔 **Status Report** - System health snapshot",
		Embeds:  []DiscordEmbed{embed},
	}

	if h.discordWebhookURL == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Discord webhook URL is not configured",
		})
		return
	}

	if err := h.sendToDiscord(msg); err != nil {
		logging.Z().Warn("failed to send status notification", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Failed to send status notification: %v", err),
		})
		return
	}

	logging.Z().Info("status notification sent to Discord")
	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"message":     "Status notification sent",
		"status_data": healthData,
	})
}

// GetNotificationStatus returns notification service status
func (h *Handler) GetNotificationStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":      "active",
		"webhook_url": h.discordWebhookURL,
		"notification_types": []string{
			"custom",
			"health",
			"status",
		},
		"supported_types": []string{
			"info",
			"success",
			"warning",
			"error",
		},
	})
}

// sendToDiscord sends message to Discord webhook
func (h *Handler) sendToDiscord(msg DiscordMessage) error {
	if h.discordWebhookURL == "" {
		return fmt.Errorf("Discord webhook URL is not configured")
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	resp, err := http.Post(h.discordWebhookURL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to send to Discord: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Discord webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// getHealthStatusData retrieves current health and status data
func (h *Handler) getHealthStatusData() HealthStatusData {
	data := HealthStatusData{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Status:    "healthy",
		Databases: make(map[string]interface{}),
	}

	// Check each database connection
	dbNames := []string{"mysql", "mariadb", "postgres", "percona", "oracle"}
	for _, dbName := range dbNames {
		if db, exists := h.connections[dbName]; exists && db != nil {
			sqlDB, err := db.DB()
			if err == nil {
				err = sqlDB.Ping()
				if err == nil {
					data.Databases[dbName] = "✅ connected"
				} else {
					data.Databases[dbName] = "❌ error"
					data.Status = "degraded"
				}
			} else {
				data.Databases[dbName] = "❌ error"
				data.Status = "degraded"
			}
		} else {
			data.Databases[dbName] = "⚠️ not configured"
		}
	}

	return data
}
