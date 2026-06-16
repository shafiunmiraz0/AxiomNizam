package apistability

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestCaptureSnapshot(t *testing.T) {
	engine := gin.New()
	engine.GET("/api/v1/users", func(c *gin.Context) {})
	engine.POST("/api/v1/users", func(c *gin.Context) {})
	engine.GET("/api/v1/users/:id", func(c *gin.Context) {})
	engine.PUT("/api/v1/users/:id", func(c *gin.Context) {})
	engine.DELETE("/api/v1/users/:id", func(c *gin.Context) {})

	snap := CaptureSnapshot(engine)

	if len(snap.Routes) != 5 {
		t.Fatalf("expected 5 routes, got %d", len(snap.Routes))
	}

	// Verify sorted order (path first, then method).
	for i := 1; i < len(snap.Routes); i++ {
		prev := snap.Routes[i-1]
		curr := snap.Routes[i]
		if prev.Path > curr.Path {
			t.Errorf("routes not sorted by path: %s > %s", prev.Path, curr.Path)
		}
		if prev.Path == curr.Path && prev.Method > curr.Method {
			t.Errorf("routes not sorted by method: %s > %s", prev.Method, curr.Method)
		}
	}
}

func TestCompareSnapshots_NoChanges(t *testing.T) {
	engine := gin.New()
	engine.GET("/api/v1/users", func(c *gin.Context) {})
	engine.POST("/api/v1/users", func(c *gin.Context) {})

	old := CaptureSnapshot(engine)
	new := CaptureSnapshot(engine)

	changes := CompareSnapshots(old, new)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(changes))
	}
}

func TestCompareSnapshots_RouteRemoved(t *testing.T) {
	oldEngine := gin.New()
	oldEngine.GET("/api/v1/users", func(c *gin.Context) {})
	oldEngine.POST("/api/v1/users", func(c *gin.Context) {})
	oldEngine.DELETE("/api/v1/users/:id", func(c *gin.Context) {})

	newEngine := gin.New()
	newEngine.GET("/api/v1/users", func(c *gin.Context) {})
	newEngine.POST("/api/v1/users", func(c *gin.Context) {})

	old := CaptureSnapshot(oldEngine)
	new := CaptureSnapshot(newEngine)

	changes := CompareSnapshots(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != RouteRemoved {
		t.Errorf("expected ROUTE_REMOVED, got %s", changes[0].Type)
	}
	if changes[0].Route != "/api/v1/users/:id" {
		t.Errorf("expected /api/v1/users/:id, got %s", changes[0].Route)
	}
}

func TestCompareSnapshots_MethodRemoved(t *testing.T) {
	oldEngine := gin.New()
	oldEngine.GET("/api/v1/users", func(c *gin.Context) {})
	oldEngine.POST("/api/v1/users", func(c *gin.Context) {})
	oldEngine.DELETE("/api/v1/users", func(c *gin.Context) {})

	newEngine := gin.New()
	newEngine.GET("/api/v1/users", func(c *gin.Context) {})
	newEngine.POST("/api/v1/users", func(c *gin.Context) {})

	old := CaptureSnapshot(oldEngine)
	new := CaptureSnapshot(newEngine)

	changes := CompareSnapshots(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != MethodRemoved {
		t.Errorf("expected METHOD_REMOVED, got %s", changes[0].Type)
	}
	if changes[0].Method != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", changes[0].Method)
	}
}

func TestCompareSnapshots_AdditiveChange(t *testing.T) {
	oldEngine := gin.New()
	oldEngine.GET("/api/v1/users", func(c *gin.Context) {})

	newEngine := gin.New()
	newEngine.GET("/api/v1/users", func(c *gin.Context) {})
	newEngine.POST("/api/v1/users", func(c *gin.Context) {})
	newEngine.GET("/api/v1/accounts", func(c *gin.Context) {})

	old := CaptureSnapshot(oldEngine)
	new := CaptureSnapshot(newEngine)

	changes := CompareSnapshots(old, new)
	if len(changes) != 0 {
		t.Errorf("additive changes should not be breaking, got %d changes", len(changes))
	}
}

func TestCompareSnapshots_NilSnapshots(t *testing.T) {
	changes := CompareSnapshots(nil, nil)
	if changes != nil {
		t.Errorf("expected nil for nil snapshots, got %d changes", len(changes))
	}
}

func TestSaveLoadSnapshot(t *testing.T) {
	engine := gin.New()
	engine.GET("/api/v1/users", func(c *gin.Context) {})
	engine.POST("/api/v1/users", func(c *gin.Context) {})

	original := CaptureSnapshot(engine)

	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")

	if err := SaveSnapshot(original, path); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}

	if len(loaded.Routes) != len(original.Routes) {
		t.Fatalf("route count mismatch: %d vs %d", len(loaded.Routes), len(original.Routes))
	}

	for i := range original.Routes {
		if loaded.Routes[i].Method != original.Routes[i].Method {
			t.Errorf("method mismatch at %d: %s vs %s", i, loaded.Routes[i].Method, original.Routes[i].Method)
		}
		if loaded.Routes[i].Path != original.Routes[i].Path {
			t.Errorf("path mismatch at %d: %s vs %s", i, loaded.Routes[i].Path, original.Routes[i].Path)
		}
	}
}

func TestSaveLoadSnapshot_RoundTrip(t *testing.T) {
	engine := gin.New()
	engine.GET("/api/v1/users", func(c *gin.Context) {})
	engine.PUT("/api/v1/users/:id", func(c *gin.Context) {})

	original := CaptureSnapshot(engine)

	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")

	if err := SaveSnapshot(original, path); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	// Verify the JSON is valid.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestFormatBreakingChanges(t *testing.T) {
	changes := []BreakingChange{
		{Type: RouteRemoved, Route: "/api/v1/old", Description: "Route GET /api/v1/old removed entirely"},
		{Type: MethodRemoved, Route: "/api/v1/users", Method: "DELETE", Description: "HTTP method DELETE removed from route /api/v1/users"},
	}

	result := FormatBreakingChanges(changes)
	if result == "No breaking changes detected." {
		t.Error("expected non-empty format for breaking changes")
	}
	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
}

func TestFormatBreakingChanges_Empty(t *testing.T) {
	result := FormatBreakingChanges(nil)
	if result != "No breaking changes detected." {
		t.Errorf("expected 'No breaking changes detected.', got %q", result)
	}
}
