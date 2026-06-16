package apistability

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// RouteInfo describes a single registered API endpoint.
type RouteInfo struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// APISnapshot is a point-in-time capture of the entire API surface.
type APISnapshot struct {
	Routes []RouteInfo `json:"routes"`
}

// BreakingChangeType categorizes the kind of breaking change detected.
type BreakingChangeType string

const (
	// RouteRemoved means an endpoint was deleted.
	RouteRemoved BreakingChangeType = "ROUTE_REMOVED"
	// MethodRemoved means an HTTP method was removed from a route.
	MethodRemoved BreakingChangeType = "METHOD_REMOVED"
)

// BreakingChange describes a single breaking change between two API snapshots.
type BreakingChange struct {
	Type        BreakingChangeType `json:"type"`
	Route       string             `json:"route"`
	Method      string             `json:"method,omitempty"`
	Description string             `json:"description"`
}

// CaptureSnapshot walks the Gin engine's registered routes and captures them.
func CaptureSnapshot(engine *gin.Engine) *APISnapshot {
	routes := engine.Routes()
	info := make([]RouteInfo, 0, len(routes))
	for _, r := range routes {
		info = append(info, RouteInfo{
			Method: r.Method,
			Path:   r.Path,
		})
	}
	sort.Slice(info, func(i, j int) bool {
		if info[i].Path != info[j].Path {
			return info[i].Path < info[j].Path
		}
		return info[i].Method < info[j].Method
	})
	return &APISnapshot{Routes: info}
}

// CompareSnapshots compares two API snapshots and returns any breaking changes.
// A breaking change is a route or method that existed in 'old' but not in 'new'.
func CompareSnapshots(old, new *APISnapshot) []BreakingChange {
	if old == nil || new == nil {
		return nil
	}

	newSet := make(map[string]bool, len(new.Routes))
	for _, r := range new.Routes {
		newSet[r.Method+" "+r.Path] = true
	}

	var changes []BreakingChange
	for _, r := range old.Routes {
		key := r.Method + " " + r.Path
		if !newSet[key] {
			// Check if the route exists with a different method (method removed).
			routeExists := false
			for _, nr := range new.Routes {
				if nr.Path == r.Path {
					routeExists = true
					break
				}
			}
			if routeExists {
				changes = append(changes, BreakingChange{
					Type:        MethodRemoved,
					Route:       r.Path,
					Method:      r.Method,
					Description: fmt.Sprintf("HTTP method %s removed from route %s", r.Method, r.Path),
				})
			} else {
				changes = append(changes, BreakingChange{
					Type:        RouteRemoved,
					Route:       r.Path,
					Method:      r.Method,
					Description: fmt.Sprintf("Route %s %s removed entirely", r.Method, r.Path),
				})
			}
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

// SaveSnapshot writes an API snapshot to a JSON file.
func SaveSnapshot(snapshot *APISnapshot, path string) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadSnapshot reads an API snapshot from a JSON file.
func LoadSnapshot(path string) (*APISnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snapshot APISnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// FormatBreakingChanges returns a human-readable summary of breaking changes.
func FormatBreakingChanges(changes []BreakingChange) string {
	if len(changes) == 0 {
		return "No breaking changes detected."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("BREAKING CHANGES DETECTED (%d):\n", len(changes)))
	for _, c := range changes {
		sb.WriteString(fmt.Sprintf("  [%s] %s\n", c.Type, c.Description))
	}
	return sb.String()
}
