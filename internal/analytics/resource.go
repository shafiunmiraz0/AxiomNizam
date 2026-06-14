package analytics

// Phase 6 P2 — Analytics resource-ification.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	DashboardKind       = "AnalyticsDashboard"
	DashboardAPIVersion = "analytics.axiomnizam.io/v1"
)

// --- Widget types (shared with handler) ---

type Widget struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"` // bar, line, pie, doughnut, area, heatmap, table, gauge, scatter, radar, funnel, kpi, log
	Title   string                 `json:"title"`
	Width   int                    `json:"width"`  // grid columns 1-12
	Height  int                    `json:"height"` // grid rows 1-4
	Order   int                    `json:"order"`
	Config  WidgetConfig           `json:"config"`
	Data    WidgetData             `json:"data"`
	Options map[string]interface{} `json:"options,omitempty"`
}

type WidgetConfig struct {
	XAxis      string   `json:"xAxis,omitempty"`
	YAxis      string   `json:"yAxis,omitempty"`
	GroupBy    string   `json:"groupBy,omitempty"`
	Colors     []string `json:"colors,omitempty"`
	ShowLegend bool     `json:"showLegend"`
	ShowGrid   bool     `json:"showGrid"`
	Stacked    bool     `json:"stacked,omitempty"`
	Animation  bool     `json:"animation"`
	DataSource string   `json:"dataSource,omitempty"`
}

type WidgetData struct {
	Labels   []string                 `json:"labels,omitempty"`
	Datasets []ChartDataset           `json:"datasets,omitempty"`
	Rows     []map[string]interface{} `json:"rows,omitempty"`
	Columns  []TableColumn            `json:"columns,omitempty"`
	Value    interface{}              `json:"value,omitempty"`
	Min      float64                  `json:"min,omitempty"`
	Max      float64                  `json:"max,omitempty"`
	Entries  []LogEntry               `json:"entries,omitempty"`
}

type ChartDataset struct {
	Label           string    `json:"label"`
	Data            []float64 `json:"data"`
	BackgroundColor []string  `json:"backgroundColor,omitempty"`
	BorderColor     string    `json:"borderColor,omitempty"`
	BorderWidth     int       `json:"borderWidth,omitempty"`
	Fill            bool      `json:"fill,omitempty"`
	Tension         float64   `json:"tension,omitempty"`
}

type TableColumn struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"` // string, number, date, status, currency
	Sortable bool   `json:"sortable"`
}

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"` // info, warn, error, debug
	Message   string `json:"message"`
	Source    string `json:"source"`
}

type DashboardFilter struct {
	ID      string         `json:"id"`
	Label   string         `json:"label"`
	Type    string         `json:"type"` // select, date-range, multi-select, search
	Key     string         `json:"key"`
	Options []FilterOption `json:"options,omitempty"`
	Default string         `json:"default,omitempty"`
}

type FilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// --- Resource types ---

type DashboardSpec struct {
	DisplayName string            `json:"displayName"`
	Description string            `json:"description,omitempty"`
	Category    string            `json:"category,omitempty"`
	Widgets     []Widget          `json:"widgets,omitempty"`
	Filters     []DashboardFilter `json:"filters,omitempty"`
}

type DashboardResourceStatus struct {
	resources.ObjectStatus `json:",inline"`
	WidgetCount            int `json:"widgetCount"`
}

type DashboardResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec                 DashboardSpec           `json:"spec"`
	Status               DashboardResourceStatus `json:"status"`
}

func (r *DashboardResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *DashboardResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *DashboardResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *DashboardResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *DashboardResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *DashboardResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *DashboardResource) GetGeneration() int64         { return r.Generation }
func (r *DashboardResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// DashboardReconciler reconciles DashboardResource.
type DashboardReconciler struct {
	store store.ResourceStore[*DashboardResource]
}

func NewDashboardReconciler(rs store.ResourceStore[*DashboardResource]) *DashboardReconciler {
	return &DashboardReconciler{store: rs}
}

func (r *DashboardReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*DashboardResource)
	if !ok {
		return reconciler.ReconcileResult{Error: dashboardErr("analytics: wrong type")}
	}
	now := time.Now()
	res.Status.Phase = "Active"
	res.Status.WidgetCount = len(res.Spec.Widgets)
	res.Status.ObservedGeneration = res.Generation
	res.Status.LastTransitionTime = now
	if r.store != nil {
		_ = r.store.Update(ctx, res)
	}
	return reconciler.ReconcileResult{}
}

type dashboardErr string

func (e dashboardErr) Error() string { return string(e) }
