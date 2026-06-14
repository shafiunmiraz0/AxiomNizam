package models

// =====================================================
// WS-4.1 — Alerting Engine as declarative resources
//
// AlertRuleResource defines a condition that, when true for a specified
// duration, creates an AlertIncidentResource and dispatches notifications.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// --- Constants ---

const (
	AlertRuleKind       = "AlertRule"
	AlertRuleAPIVersion = "alerting.axiomnizam.io/v1"

	AlertIncidentKind       = "AlertIncident"
	AlertIncidentAPIVersion = "alerting.axiomnizam.io/v1"

	NotificationChannelKind       = "NotificationChannel"
	NotificationChannelAPIVersion = "alerting.axiomnizam.io/v1"
)

// --- Severity ---

type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityWarning  AlertSeverity = "warning"
	SeverityInfo     AlertSeverity = "info"
)

// --- Condition Types ---

type ConditionType string

const (
	ConditionTypeMetric     ConditionType = "metric"
	ConditionTypeQuality    ConditionType = "quality"
	ConditionTypeEvent      ConditionType = "event"
	ConditionTypeReconciler ConditionType = "reconciler"
	ConditionTypeCustom     ConditionType = "custom"
)

// --- Channel Types ---

type ChannelType string

const (
	ChannelTypeSlack     ChannelType = "slack"
	ChannelTypeEmail     ChannelType = "email"
	ChannelTypeWebhook   ChannelType = "webhook"
	ChannelTypePagerDuty ChannelType = "pagerduty"
	ChannelTypeTeams     ChannelType = "teams"
)

// --- Incident Status ---

type IncidentStatus string

const (
	IncidentFiring       IncidentStatus = "firing"
	IncidentAcknowledged IncidentStatus = "acknowledged"
	IncidentResolved     IncidentStatus = "resolved"
	IncidentSilenced     IncidentStatus = "silenced"
)

// --- Condition Configs ---

type MetricCondition struct {
	Query         string  `json:"query"`         // Metric name or expression
	Operator      string  `json:"operator"`      // gt, lt, eq, ne, gte, lte
	Threshold     float64 `json:"threshold"`
	AggregateOver string  `json:"aggregateOver"` // "5m", "15m", "1h"
	Aggregation   string  `json:"aggregation"`   // avg, max, min, sum, count
}

type QualityCondition struct {
	RuleRef          string `json:"ruleRef"`          // QualityRule resource name
	OnResult         string `json:"onResult"`         // fail, error
	ConsecutiveFails int    `json:"consecutiveFails"` // Trigger after N consecutive
}

type EventCondition struct {
	Topic   string `json:"topic"`   // Event bus topic
	Pattern string `json:"pattern"` // Message pattern to match
	Count   int    `json:"count"`   // Trigger after N events in window
	Window  string `json:"window"`  // Time window
}

type ReconcilerCondition struct {
	Module           string `json:"module"`           // Reconciler module name
	ConsecutiveErrors int   `json:"consecutiveErrors"` // Trigger after N errors
	MaxDuration      string `json:"maxDuration"`      // Alert if reconcile takes longer
}

// --- Alert Condition ---

type AlertCondition struct {
	Type       ConditionType        `json:"type"`
	Metric     *MetricCondition     `json:"metric,omitempty"`
	Quality    *QualityCondition    `json:"quality,omitempty"`
	Event      *EventCondition      `json:"event,omitempty"`
	Reconciler *ReconcilerCondition `json:"reconciler,omitempty"`
}

// --- Channel Reference ---

type ChannelRef struct {
	Name string `json:"name"` // NotificationChannel resource name
}

// --- Escalation ---

type EscalationPolicy struct {
	Levels []EscalationLevel `json:"levels"`
}

type EscalationLevel struct {
	After          string       `json:"after"`          // Duration before escalating
	Channels       []ChannelRef `json:"channels"`
	RepeatInterval string       `json:"repeatInterval"` // How often to repeat
}

// --- AlertRuleSpec ---

type AlertRuleSpec struct {
	DisplayName string        `json:"displayName"`
	Description string        `json:"description,omitempty"`
	Severity    AlertSeverity `json:"severity"`

	// Evaluation
	EvalInterval string         `json:"evalInterval"` // "30s", "1m", "5m"
	Condition    AlertCondition `json:"condition"`
	ForDuration  string         `json:"forDuration,omitempty"` // Must be true for X before firing

	// Notification
	Channels   []ChannelRef      `json:"channels"`
	Escalation *EscalationPolicy `json:"escalation,omitempty"`

	// Suppression
	Silenced     bool       `json:"silenced"`
	SilenceUntil *time.Time `json:"silenceUntil,omitempty"`

	// Grouping
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`

	// Enabled
	Enabled bool `json:"enabled"`
}

// --- AlertRuleResourceStatus ---

type AlertRuleResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	// RuleState: inactive, pending, firing, resolved
	RuleState string `json:"ruleState"`

	// LastEvalAt is when the rule was last evaluated
	LastEvalAt *time.Time `json:"lastEvalAt,omitempty"`

	// LastFiredAt is when the rule last transitioned to firing
	LastFiredAt *time.Time `json:"lastFiredAt,omitempty"`

	// LastResolvedAt is when the rule last resolved
	LastResolvedAt *time.Time `json:"lastResolvedAt,omitempty"`

	// PendingSince is when the condition first became true (before forDuration)
	PendingSince *time.Time `json:"pendingSince,omitempty"`

	// ActiveIncident is the name of the current active incident (if any)
	ActiveIncident string `json:"activeIncident,omitempty"`

	// TotalFirings is the total number of times this rule has fired
	TotalFirings int64 `json:"totalFirings"`

	// EvalCount is the total number of evaluations
	EvalCount int64 `json:"evalCount"`
}

// --- AlertRuleResource ---

type AlertRuleResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   AlertRuleSpec           `json:"spec"`
	Status AlertRuleResourceStatus `json:"status"`
}

func (r *AlertRuleResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *AlertRuleResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *AlertRuleResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *AlertRuleResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *AlertRuleResource) DeepCopy() resources.Resource {
	cp := *r
	return &cp
}
func (r *AlertRuleResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *AlertRuleResource) GetGeneration() int64         { return r.Generation }
func (r *AlertRuleResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// =====================================================
// AlertIncidentResource — active alert instance
// =====================================================

type AlertIncidentSpec struct {
	RuleRef     string        `json:"ruleRef"`     // AlertRule that created this
	Severity    AlertSeverity `json:"severity"`
	Summary     string        `json:"summary"`
	Description string        `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type AlertIncidentResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	IncidentStatus IncidentStatus `json:"incidentStatus"`
	FiredAt        *time.Time     `json:"firedAt,omitempty"`
	AcknowledgedAt *time.Time     `json:"acknowledgedAt,omitempty"`
	AcknowledgedBy string         `json:"acknowledgedBy,omitempty"`
	ResolvedAt     *time.Time     `json:"resolvedAt,omitempty"`
	NotifiedAt     *time.Time     `json:"notifiedAt,omitempty"`
	EscalationLevel int           `json:"escalationLevel"`
	NotifyCount    int            `json:"notifyCount"`
}

type AlertIncidentResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   AlertIncidentSpec           `json:"spec"`
	Status AlertIncidentResourceStatus `json:"status"`
}

func (r *AlertIncidentResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *AlertIncidentResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *AlertIncidentResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *AlertIncidentResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *AlertIncidentResource) DeepCopy() resources.Resource {
	cp := *r
	return &cp
}
func (r *AlertIncidentResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *AlertIncidentResource) GetGeneration() int64         { return r.Generation }
func (r *AlertIncidentResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// =====================================================
// NotificationChannelResource — notification target
// =====================================================

type NotificationChannelSpec struct {
	Type             ChannelType       `json:"type"`
	DisplayName      string            `json:"displayName"`
	Config           map[string]string `json:"config"`
	RateLimitPerHour int               `json:"rateLimitPerHour,omitempty"`
	Templates        map[string]string `json:"templates,omitempty"`
	Enabled          bool              `json:"enabled"`
}

type NotificationChannelResourceStatus struct {
	resources.ObjectStatus `json:",inline"`
	LastSentAt     *time.Time `json:"lastSentAt,omitempty"`
	TotalSent      int64      `json:"totalSent"`
	TotalFailed    int64      `json:"totalFailed"`
	LastError      string     `json:"lastError,omitempty"`
	SentThisHour   int        `json:"sentThisHour"`
}

type NotificationChannelResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   NotificationChannelSpec           `json:"spec"`
	Status NotificationChannelResourceStatus `json:"status"`
}

func (r *NotificationChannelResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *NotificationChannelResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *NotificationChannelResource) GetStatus() *resources.ObjectStatus {
	return &r.Status.ObjectStatus
}
func (r *NotificationChannelResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *NotificationChannelResource) DeepCopy() resources.Resource {
	cp := *r
	return &cp
}
func (r *NotificationChannelResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *NotificationChannelResource) GetGeneration() int64         { return r.Generation }
func (r *NotificationChannelResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
