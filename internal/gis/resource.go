package gis

// =====================================================
// Phase 6 P2 — GIS resource-ification.
//
// GISResource wraps GIS entities (Layer, Region, Marker, Dataset)
// as a single declarative resource with a Kind discriminator.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	GISResourceKind       = "GISResource"
	GISResourceAPIVersion = "gis.axiomnizam.io/v1"
)

// --- GIS entity types (shared with handler) ---

// GISLayer represents a map layer.
type GISLayer struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Type      string      `json:"type"` // geojson, tile, marker, heatmap
	Visible   bool        `json:"visible"`
	Style     LayerStyle  `json:"style,omitempty"`
	URL       string      `json:"url,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	CreatedAt time.Time   `json:"createdAt"`
}

// LayerStyle defines visual properties for a layer.
type LayerStyle struct {
	Color       string  `json:"color,omitempty"`
	Weight      float64 `json:"weight,omitempty"`
	Opacity     float64 `json:"opacity,omitempty"`
	FillColor   string  `json:"fillColor,omitempty"`
	FillOpacity float64 `json:"fillOpacity,omitempty"`
}

// GISRegion represents a geographic region (division/district/upazila).
type GISRegion struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"` // division, district, upazila
	ParentID   string                 `json:"parentId,omitempty"`
	Center     [2]float64             `json:"center"`           // [lat, lng]
	Bounds     [4]float64             `json:"bounds,omitempty"` // [minLat, minLng, maxLat, maxLng]
	Properties map[string]interface{} `json:"properties,omitempty"`
	GeoJSON    interface{}            `json:"geojson,omitempty"`
}

// GISMarker represents a point marker on the map.
type GISMarker struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Lat        float64                `json:"lat"`
	Lng        float64                `json:"lng"`
	Category   string                 `json:"category"`
	Icon       string                 `json:"icon,omitempty"`
	Color      string                 `json:"color,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// GISDataset represents a named data collection for choropleth/analysis.
type GISDataset struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	Unit        string                   `json:"unit,omitempty"`
	Columns     []DatasetColumn          `json:"columns"`
	Rows        []map[string]interface{} `json:"rows"`
	CreatedAt   time.Time                `json:"createdAt"`
	UpdatedAt   time.Time                `json:"updatedAt"`
}

// DatasetColumn defines a column in a dataset.
type DatasetColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"` // string, number, date
}

// GISSummary is returned for the overview endpoint.
type GISSummary struct {
	TotalLayers   int            `json:"totalLayers"`
	TotalRegions  int            `json:"totalRegions"`
	TotalMarkers  int            `json:"totalMarkers"`
	TotalDatasets int            `json:"totalDatasets"`
	RegionsByType map[string]int `json:"regionsByType"`
	MapCenter     [2]float64     `json:"mapCenter"`
	DefaultZoom   int            `json:"defaultZoom"`
}

// GISResourceSpec is the desired state of a GIS entity.
type GISResourceSpec struct {
	// GISKind discriminates: "Layer", "Region", "Marker", "Dataset"
	GISKind     string                 `json:"gisKind"`
	DisplayName string                 `json:"displayName,omitempty"`
	Description string                 `json:"description,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Visible     *bool                  `json:"visible,omitempty"`
	Style       *LayerStyle            `json:"style,omitempty"`
	URL         string                 `json:"url,omitempty"`
	ParentID    string                 `json:"parentId,omitempty"`
	Center      [2]float64             `json:"center,omitempty"`
	Bounds      [4]float64             `json:"bounds,omitempty"`
	Lat         float64                `json:"lat,omitempty"`
	Lng         float64                `json:"lng,omitempty"`
	Category    string                 `json:"category,omitempty"`
	Icon        string                 `json:"icon,omitempty"`
	Color       string                 `json:"color,omitempty"`
	Unit        string                 `json:"unit,omitempty"`
	Columns     []DatasetColumn        `json:"columns,omitempty"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
}

// GISResourceStatus extends the canonical object status.
type GISResourceStatus struct {
	resources.ObjectStatus `json:",inline"`
	EntityCount            int `json:"entityCount,omitempty"`
}

// GISResource is the declarative resource for GIS entities.
type GISResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   GISResourceSpec   `json:"spec"`
	Status GISResourceStatus `json:"status"`
}

func (r *GISResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *GISResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *GISResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *GISResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *GISResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *GISResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *GISResource) GetGeneration() int64         { return r.Generation }
func (r *GISResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// NewGISResourceFromLayer converts a GISLayer to a GISResource.
func NewGISResourceFromLayer(layer *GISLayer) *GISResource {
	visible := layer.Visible
	return &GISResource{
		TypeMeta:   resources.TypeMeta{APIVersion: GISResourceAPIVersion, Kind: GISResourceKind},
		ObjectMeta: resources.ObjectMeta{Name: "layer-" + layer.ID, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(), Labels: map[string]string{"gisKind": "Layer"}},
		Spec:       GISResourceSpec{GISKind: "Layer", DisplayName: layer.Name, Type: layer.Type, Visible: &visible, Style: &layer.Style, URL: layer.URL},
		Status:     GISResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: "Active"}},
	}
}

// NewGISResourceFromRegion converts a GISRegion to a GISResource.
func NewGISResourceFromRegion(region *GISRegion) *GISResource {
	return &GISResource{
		TypeMeta:   resources.TypeMeta{APIVersion: GISResourceAPIVersion, Kind: GISResourceKind},
		ObjectMeta: resources.ObjectMeta{Name: "region-" + region.ID, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(), Labels: map[string]string{"gisKind": "Region"}},
		Spec:       GISResourceSpec{GISKind: "Region", DisplayName: region.Name, Type: region.Type, ParentID: region.ParentID, Center: region.Center, Bounds: region.Bounds, Properties: region.Properties},
		Status:     GISResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: "Active"}},
	}
}

// NewGISResourceFromMarker converts a GISMarker to a GISResource.
func NewGISResourceFromMarker(marker *GISMarker) *GISResource {
	return &GISResource{
		TypeMeta:   resources.TypeMeta{APIVersion: GISResourceAPIVersion, Kind: GISResourceKind},
		ObjectMeta: resources.ObjectMeta{Name: "marker-" + marker.ID, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(), Labels: map[string]string{"gisKind": "Marker"}},
		Spec:       GISResourceSpec{GISKind: "Marker", DisplayName: marker.Name, Lat: marker.Lat, Lng: marker.Lng, Category: marker.Category, Icon: marker.Icon, Color: marker.Color, Properties: marker.Properties},
		Status:     GISResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: "Active"}},
	}
}

// NewGISResourceFromDataset converts a GISDataset to a GISResource.
func NewGISResourceFromDataset(ds *GISDataset) *GISResource {
	return &GISResource{
		TypeMeta:   resources.TypeMeta{APIVersion: GISResourceAPIVersion, Kind: GISResourceKind},
		ObjectMeta: resources.ObjectMeta{Name: "dataset-" + ds.ID, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(), Labels: map[string]string{"gisKind": "Dataset"}},
		Spec:       GISResourceSpec{GISKind: "Dataset", DisplayName: ds.Name, Description: ds.Description, Unit: ds.Unit, Columns: ds.Columns},
		Status:     GISResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: "Active"}},
	}
}
