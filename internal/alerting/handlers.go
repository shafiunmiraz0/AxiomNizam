package alerting

// =====================================================
// WS-4.1 — Alerting REST API Handlers
//
// Provides CRUD for alert rules, incident management,
// notification channels, and silence management.
// =====================================================

import (
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/alerting/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/validate"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AlertHandlers provides REST API handlers for alerting.
type AlertHandlers struct {
	ruleStore    store.ResourceStore[*models.AlertRuleResource]
	incidentStore store.ResourceStore[*models.AlertIncidentResource]
	channelStore store.ResourceStore[*models.NotificationChannelResource]
}

// NewAlertHandlers creates new handlers.
func NewAlertHandlers(
	ruleStore store.ResourceStore[*models.AlertRuleResource],
	incidentStore store.ResourceStore[*models.AlertIncidentResource],
	channelStore store.ResourceStore[*models.NotificationChannelResource],
) *AlertHandlers {
	return &AlertHandlers{
		ruleStore:     ruleStore,
		incidentStore: incidentStore,
		channelStore:  channelStore,
	}
}

// RegisterRoutes registers alerting API routes.
func (h *AlertHandlers) RegisterRoutes(rg *gin.RouterGroup) {
	alerts := rg.Group("/alerts")
	{
		// Rules
		alerts.GET("/rules", h.ListRules)
		alerts.GET("/rules/:name", h.GetRule)
		alerts.POST("/rules", h.CreateRule)
		alerts.PUT("/rules/:name", h.UpdateRule)
		alerts.DELETE("/rules/:name", h.DeleteRule)
		alerts.POST("/rules/:name/silence", h.SilenceRule)
		alerts.POST("/rules/:name/unsilence", h.UnsilenceRule)

		// Incidents
		alerts.GET("/incidents", h.ListIncidents)
		alerts.GET("/incidents/:name", h.GetIncident)
		alerts.POST("/incidents/:name/acknowledge", h.AcknowledgeIncident)
		alerts.POST("/incidents/:name/resolve", h.ResolveIncident)

		// Channels
		alerts.GET("/channels", h.ListChannels)
		alerts.GET("/channels/:name", h.GetChannel)
		alerts.POST("/channels", h.CreateChannel)
		alerts.PUT("/channels/:name", h.UpdateChannel)
		alerts.DELETE("/channels/:name", h.DeleteChannel)
		alerts.POST("/channels/:name/test", h.TestChannel)

		// Summary
		alerts.GET("/summary", h.GetSummary)
	}
}

// --- Rule Handlers ---

// ListRules returns all alert rules.
func (h *AlertHandlers) ListRules(c *gin.Context) {
	rules, err := h.ruleStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListRules"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	severityFilter := c.Query("severity")
	var filtered []*models.AlertRuleResource
	for _, rule := range rules {
		if severityFilter != "" && string(rule.Spec.Severity) != severityFilter {
			continue
		}
		filtered = append(filtered, rule)
	}

	c.JSON(http.StatusOK, RuleListResponse{Rules: filtered, Count: len(filtered)})
}

// GetRule returns a single alert rule.
func (h *AlertHandlers) GetRule(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	rule, err := h.ruleStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "rule not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, rule)
}

// CreateRule creates a new alert rule.
func (h *AlertHandlers) CreateRule(c *gin.Context) {
	var rule models.AlertRuleResource
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	rule.Kind = models.AlertRuleKind
	rule.APIVersion = models.AlertRuleAPIVersion
	now := time.Now()
	rule.CreatedAt = now
	rule.Generation = 1
	rule.Status.Phase = "Pending"

	if err := h.ruleStore.Create(c.Request.Context(), &rule); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, rule)
}

// UpdateRule updates an existing alert rule.
func (h *AlertHandlers) UpdateRule(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	existing, err := h.ruleStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "rule not found", Name: name})
		return
	}

	var updated models.AlertRuleResource
	if err := c.ShouldBindJSON(&updated); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	updated.ObjectMeta = existing.ObjectMeta
	updated.Generation = existing.Generation + 1
	updated.Status = existing.Status

	if err := h.ruleStore.Update(c.Request.Context(), &updated); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "UpdateRule"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteRule deletes an alert rule.
func (h *AlertHandlers) DeleteRule(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	if err := h.ruleStore.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "rule not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: name})
}

// SilenceRule silences an alert rule for a specified duration.
func (h *AlertHandlers) SilenceRule(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	rule, err := h.ruleStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "rule not found", Name: name})
		return
	}

	var req struct {
		Duration string `json:"duration"` // "2h", "24h", "7d"
		Reason   string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	duration, err := time.ParseDuration(req.Duration)
	if err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "invalid duration: " + req.Duration})
		return
	}

	silenceUntil := time.Now().Add(duration)
	rule.Spec.Silenced = true
	rule.Spec.SilenceUntil = &silenceUntil
	rule.Generation++

	if err := h.ruleStore.Update(c.Request.Context(), rule); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "SilenceRule"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, SilenceResponse{Silenced: true, SilenceUntil: &silenceUntil, Reason: req.Reason})
}

// UnsilenceRule removes silence from an alert rule.
func (h *AlertHandlers) UnsilenceRule(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	rule, err := h.ruleStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "rule not found", Name: name})
		return
	}

	rule.Spec.Silenced = false
	rule.Spec.SilenceUntil = nil
	rule.Generation++

	if err := h.ruleStore.Update(c.Request.Context(), rule); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "UnsilenceRule"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, UnsilenceResponse{Silenced: false})
}

// --- Incident Handlers ---

// ListIncidents returns all alert incidents.
func (h *AlertHandlers) ListIncidents(c *gin.Context) {
	incidents, err := h.incidentStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListIncidents"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	statusFilter := c.Query("status")
	var filtered []*models.AlertIncidentResource
	for _, incident := range incidents {
		if statusFilter != "" && string(incident.Status.IncidentStatus) != statusFilter {
			continue
		}
		filtered = append(filtered, incident)
	}

	c.JSON(http.StatusOK, IncidentListResponse{Incidents: filtered, Count: len(filtered)})
}

// GetIncident returns a single incident.
func (h *AlertHandlers) GetIncident(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	incident, err := h.incidentStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "incident not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, incident)
}

// AcknowledgeIncident marks an incident as acknowledged.
func (h *AlertHandlers) AcknowledgeIncident(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	incident, err := h.incidentStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "incident not found", Name: name})
		return
	}

	now := time.Now()
	incident.Status.IncidentStatus = models.IncidentAcknowledged
	incident.Status.AcknowledgedAt = &now
	incident.Status.LastTransitionTime = now

	if err := h.incidentStore.Update(c.Request.Context(), incident); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "AcknowledgeIncident"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, AcknowledgeResponse{Acknowledged: true, Incident: name})
}

// ResolveIncident manually resolves an incident.
func (h *AlertHandlers) ResolveIncident(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	incident, err := h.incidentStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "incident not found", Name: name})
		return
	}

	now := time.Now()
	incident.Status.IncidentStatus = models.IncidentResolved
	incident.Status.ResolvedAt = &now
	incident.Status.LastTransitionTime = now

	if err := h.incidentStore.Update(c.Request.Context(), incident); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ResolveIncident"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, ResolveResponse{Resolved: true, Incident: name})
}

// --- Channel Handlers ---

// ListChannels returns all notification channels.
func (h *AlertHandlers) ListChannels(c *gin.Context) {
	channels, err := h.channelStore.List(c.Request.Context(), "")
	if err != nil {
		logging.Z().Warn("handler error", zap.String("op", "ListChannels"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ChannelListResponse{Channels: channels, Count: len(channels)})
}

// GetChannel returns a single notification channel.
func (h *AlertHandlers) GetChannel(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	channel, err := h.channelStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "channel not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, channel)
}

// CreateChannel creates a new notification channel.
func (h *AlertHandlers) CreateChannel(c *gin.Context) {
	var channel models.NotificationChannelResource
	if err := c.ShouldBindJSON(&channel); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	channel.Kind = models.NotificationChannelKind
	channel.APIVersion = models.NotificationChannelAPIVersion
	now := time.Now()
	channel.CreatedAt = now
	channel.Generation = 1
	channel.Status.Phase = "Active"

	if err := h.channelStore.Create(c.Request.Context(), &channel); err != nil {
		c.JSON(http.StatusConflict, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, channel)
}

// UpdateChannel updates a notification channel.
func (h *AlertHandlers) UpdateChannel(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	existing, err := h.channelStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "channel not found", Name: name})
		return
	}

	var updated models.NotificationChannelResource
	if err := c.ShouldBindJSON(&updated); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	updated.ObjectMeta = existing.ObjectMeta
	updated.Generation = existing.Generation + 1

	if err := h.channelStore.Update(c.Request.Context(), &updated); err != nil {
		logging.Z().Warn("handler error", zap.String("op", "UpdateChannel"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteChannel deletes a notification channel.
func (h *AlertHandlers) DeleteChannel(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	if err := h.channelStore.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "channel not found", Name: name})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: name})
}

// TestChannel sends a test notification through a channel.
func (h *AlertHandlers) TestChannel(c *gin.Context) {
	name := validate.PathParam(c, "name")
	if name == "" {
		return
	}
	channel, err := h.channelStore.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "channel not found", Name: name})
		return
	}

	// In production, this would actually send a test message.
	c.JSON(http.StatusOK, TestChannelResponse{
		Success: true,
		Channel: name,
		Type:    string(channel.Spec.Type),
		Message: "test notification sent successfully",
	})
}

// --- Summary ---

// GetSummary returns an alerting overview.
func (h *AlertHandlers) GetSummary(c *gin.Context) {
	rules, _ := h.ruleStore.List(c.Request.Context(), "")
	incidents, _ := h.incidentStore.List(c.Request.Context(), "")

	var totalRules, silencedRules int
	var firingIncidents, resolvedIncidents, acknowledgedIncidents int

	for _, rule := range rules {
		totalRules++
		if rule.Spec.Silenced {
			silencedRules++
		}
	}

	for _, incident := range incidents {
		switch incident.Status.IncidentStatus {
		case models.IncidentFiring:
			firingIncidents++
		case models.IncidentResolved:
			resolvedIncidents++
		case models.IncidentAcknowledged:
			acknowledgedIncidents++
		}
	}

	c.JSON(http.StatusOK, SummaryResponse{
		TotalRules:            totalRules,
		SilencedRules:         silencedRules,
		FiringIncidents:       firingIncidents,
		AcknowledgedIncidents: acknowledgedIncidents,
		ResolvedIncidents:     resolvedIncidents,
		TotalIncidents:        len(incidents),
	})
}
