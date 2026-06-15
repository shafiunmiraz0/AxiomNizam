package models

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	StreamJobKind       = "StreamJob"
	StreamJobAPIVersion = "streamanalytics.axiomnizam.io/v1"
)

// --- Stream Source ---

type StreamSource struct {
	Type          string `json:"type"`          // kafka, eventbus
	Topic         string `json:"topic"`
	ConsumerGroup string `json:"consumerGroup,omitempty"`
	StartOffset   string `json:"startOffset,omitempty"` // earliest, latest, timestamp
}

// --- Window Spec ---

type WindowSpec struct {
	Type  string `json:"type"`            // tumbling, sliding, session
	Size  string `json:"size"`            // "5m", "1h"
	Slide string `json:"slide,omitempty"` // For sliding windows
	Gap   string `json:"gap,omitempty"`   // For session windows
}

// --- Aggregation Spec ---

type AggregationSpec struct {
	OutputField string `json:"outputField"`
	Function    string `json:"function"` // count, sum, avg, min, max, p50, p95, p99, distinct_count
	InputField  string `json:"inputField"`
}

// --- Filter Spec ---

type FilterSpec struct {
	Field    string `json:"field"`
	Operator string `json:"operator"` // eq, ne, gt, lt, gte, lte, in, contains
	Value    string `json:"value"`
}

// --- Stream Sink ---

type StreamSink struct {
	Type          string `json:"type"`          // postgres, webhook, eventbus, stdout
	DataSourceRef string `json:"dataSourceRef,omitempty"`
	Table         string `json:"table,omitempty"`
	Topic         string `json:"topic,omitempty"`
	WebhookURL    string `json:"webhookUrl,omitempty"`
}

// --- StreamJobSpec ---

type StreamJobSpec struct {
	DisplayName  string            `json:"displayName"`
	Description  string            `json:"description,omitempty"`
	Source       StreamSource      `json:"source"`
	Window       WindowSpec        `json:"window"`
	Aggregations []AggregationSpec `json:"aggregations"`
	Filters      []FilterSpec      `json:"filters,omitempty"`
	GroupBy      []string          `json:"groupBy,omitempty"`
	Sink         StreamSink        `json:"sink"`
	Parallelism  int               `json:"parallelism,omitempty"`
	Watermark    string            `json:"watermark,omitempty"` // Late event tolerance
	Enabled      bool              `json:"enabled"`
}

// --- StreamJobResourceStatus ---

type StreamJobResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	JobStatus        string     `json:"jobStatus"`         // pending, running, paused, stopped, failed
	EventsProcessed  int64      `json:"eventsProcessed"`
	EventsDropped    int64      `json:"eventsDropped"`
	WindowsFlushed   int64      `json:"windowsFlushed"`
	LastCheckpointAt *time.Time `json:"lastCheckpointAt,omitempty"`
	CurrentLag       int64      `json:"currentLag"`
	AvgProcessingMs  float64    `json:"avgProcessingMs"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	LastErrorMessage string     `json:"lastErrorMessage,omitempty"`
}

// --- StreamJobResource ---

type StreamJobResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   StreamJobSpec           `json:"spec"`
	Status StreamJobResourceStatus `json:"status"`
}

func (r *StreamJobResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *StreamJobResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *StreamJobResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *StreamJobResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *StreamJobResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Aggregations) > 0 {
		cp.Spec.Aggregations = make([]AggregationSpec, len(r.Spec.Aggregations))
		copy(cp.Spec.Aggregations, r.Spec.Aggregations)
	}
	if len(r.Spec.GroupBy) > 0 {
		cp.Spec.GroupBy = make([]string, len(r.Spec.GroupBy))
		copy(cp.Spec.GroupBy, r.Spec.GroupBy)
	}
	return &cp
}
func (r *StreamJobResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *StreamJobResource) GetGeneration() int64         { return r.Generation }
func (r *StreamJobResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
