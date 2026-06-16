// Package stability implements the "never break userspace" principle.
// It provides API surface registry, breaking change detection, and
// error format enforcement middleware.
package stability

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// RouteDescriptor describes a single registered API endpoint.
type RouteDescriptor struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// APISurface is a point-in-time capture of the entire API surface.
type APISurface struct {
	Routes []RouteDescriptor `json:"routes"`
}

// BreakingChangeType categorizes the kind of breaking change detected.
type BreakingChangeType string

const (
	RouteRemoved  BreakingChangeType = "ROUTE_REMOVED"
	MethodRemoved BreakingChangeType = "METHOD_REMOVED"
)

// BreakingChange describes a single breaking change between two API surfaces.
type BreakingChange struct {
	Type        BreakingChangeType `json:"type"`
	Route       string             `json:"route"`
	Method      string             `json:"method,omitempty"`
	Description string             `json:"description"`
}

// Registry captures and compares API surfaces.
type Registry struct {
	mu       sync.RWMutex
	surface  *APISurface
	engine   *gin.Engine
}

// NewRegistry creates a new API surface registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// CaptureSnapshot walks the Gin engine's registered routes and captures them.
func (r *Registry) CaptureSnapshot(engine *gin.Engine) *APISurface {
	routes := engine.Routes()
	info := make([]RouteDescriptor, 0, len(routes))
	for _, rt := range routes {
		info = append(info, RouteDescriptor{
			Method: rt.Method,
			Path:   rt.Path,
		})
	}
	sort.Slice(info, func(i, j int) bool {
		if info[i].Path != info[j].Path {
			return info[i].Path < info[j].Path
		}
		return info[i].Method < info[j].Method
	})

	surface := &APISurface{Routes: info}

	r.mu.Lock()
	r.surface = surface
	r.engine = engine
	r.mu.Unlock()

	return surface
}

// CurrentSurface returns the last captured surface.
func (r *Registry) CurrentSurface() *APISurface {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.surface
}

// CompareSurfaces compares two API surfaces and returns any breaking changes.
// A breaking change is a route or method that existed in 'old' but not in 'new'.
func CompareSurfaces(old, new *APISurface) []BreakingChange {
	if old == nil || new == nil {
		return nil
	}

	newSet := make(map[string]bool, len(new.Routes))
	for _, rt := range new.Routes {
		newSet[rt.Method+" "+rt.Path] = true
	}

	var changes []BreakingChange
	for _, rt := range old.Routes {
		key := rt.Method + " " + rt.Path
		if newSet[key] {
			continue
		}
		// Check if the route exists with a different method.
		routeExists := false
		for _, nr := range new.Routes {
			if nr.Path == rt.Path {
				routeExists = true
				break
			}
		}
		if routeExists {
			changes = append(changes, BreakingChange{
				Type:        MethodRemoved,
				Route:       rt.Path,
				Method:      rt.Method,
				Description: fmt.Sprintf("HTTP method %s removed from route %s", rt.Method, rt.Path),
			})
		} else {
			changes = append(changes, BreakingChange{
				Type:        RouteRemoved,
				Route:       rt.Path,
				Method:      rt.Method,
				Description: fmt.Sprintf("Route %s %s removed entirely", rt.Method, rt.Path),
			})
		}
	}

	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Route != changes[j].Route {
			return changes[i].Route < changes[j].Route
		}
		return changes[i].Method < changes[j].Method
	})
	return changes
}

// SaveSurface writes an API surface to a JSON file.
func SaveSurface(surface *APISurface, path string) error {
	data, err := json.MarshalIndent(surface, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadSurface reads an API surface from a JSON file.
func LoadSurface(path string) (*APISurface, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var surface APISurface
	if err := json.Unmarshal(data, &surface); err != nil {
		return nil, err
	}
	return &surface, nil
}

// FormatBreakingChanges returns a human-readable summary of breaking changes.
func FormatBreakingChanges(changes []BreakingChange) string {
	if len(changes) == 0 {
		return "No breaking changes detected."
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "BREAKING CHANGES DETECTED (%d):\n", len(changes))
	for _, c := range changes {
		fmt.Fprintf(&sb, "  [%s] %s\n", c.Type, c.Description)
	}
	return sb.String()
}
