package stability

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

	reg := NewRegistry()
	surface := reg.CaptureSnapshot(engine)

	if len(surface.Routes) != 5 {
		t.Fatalf("expected 5 routes, got %d", len(surface.Routes))
	}

	// Verify sorted order.
	for i := 1; i < len(surface.Routes); i++ {
		prev := surface.Routes[i-1]
		curr := surface.Routes[i]
		if prev.Path > curr.Path {
			t.Errorf("routes not sorted by path: %s > %s", prev.Path, curr.Path)
		}
		if prev.Path == curr.Path && prev.Method > curr.Method {
			t.Errorf("routes not sorted by method: %s > %s", prev.Method, curr.Method)
		}
	}
}

func TestCompareSurfaces_NoChanges(t *testing.T) {
	engine := gin.New()
	engine.GET("/api/v1/users", func(c *gin.Context) {})
	engine.POST("/api/v1/users", func(c *gin.Context) {})

	reg := NewRegistry()
	old := reg.CaptureSnapshot(engine)
	new_ := reg.CaptureSnapshot(engine)

	changes := CompareSurfaces(old, new_)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(changes))
	}
}

func TestCompareSurfaces_RouteRemoved(t *testing.T) {
	oldEngine := gin.New()
	oldEngine.GET("/api/v1/users", func(c *gin.Context) {})
	oldEngine.POST("/api/v1/users", func(c *gin.Context) {})
	oldEngine.DELETE("/api/v1/users/:id", func(c *gin.Context) {})

	newEngine := gin.New()
	newEngine.GET("/api/v1/users", func(c *gin.Context) {})
	newEngine.POST("/api/v1/users", func(c *gin.Context) {})

	reg := NewRegistry()
	old := reg.CaptureSnapshot(oldEngine)
	new_ := reg.CaptureSnapshot(newEngine)

	changes := CompareSurfaces(old, new_)
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

func TestCompareSurfaces_MethodRemoved(t *testing.T) {
	oldEngine := gin.New()
	oldEngine.GET("/api/v1/users", func(c *gin.Context) {})
	oldEngine.POST("/api/v1/users", func(c *gin.Context) {})
	oldEngine.DELETE("/api/v1/users", func(c *gin.Context) {})

	newEngine := gin.New()
	newEngine.GET("/api/v1/users", func(c *gin.Context) {})
	newEngine.POST("/api/v1/users", func(c *gin.Context) {})

	reg := NewRegistry()
	old := reg.CaptureSnapshot(oldEngine)
	new_ := reg.CaptureSnapshot(newEngine)

	changes := CompareSurfaces(old, new_)
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

func TestCompareSurfaces_AdditiveChange(t *testing.T) {
	oldEngine := gin.New()
	oldEngine.GET("/api/v1/users", func(c *gin.Context) {})

	newEngine := gin.New()
	newEngine.GET("/api/v1/users", func(c *gin.Context) {})
	newEngine.POST("/api/v1/users", func(c *gin.Context) {})
	newEngine.GET("/api/v1/accounts", func(c *gin.Context) {})

	reg := NewRegistry()
	old := reg.CaptureSnapshot(oldEngine)
	new_ := reg.CaptureSnapshot(newEngine)

	changes := CompareSurfaces(old, new_)
	if len(changes) != 0 {
		t.Errorf("additive changes should not be breaking, got %d changes", len(changes))
	}
}

func TestCompareSurfaces_NilSurfaces(t *testing.T) {
	changes := CompareSurfaces(nil, nil)
	if changes != nil {
		t.Errorf("expected nil for nil surfaces, got %d changes", len(changes))
	}
}

func TestSaveLoadSurface(t *testing.T) {
	engine := gin.New()
	engine.GET("/api/v1/users", func(c *gin.Context) {})
	engine.POST("/api/v1/users", func(c *gin.Context) {})

	reg := NewRegistry()
	original := reg.CaptureSnapshot(engine)

	dir := t.TempDir()
	path := filepath.Join(dir, "surface.json")

	if err := SaveSurface(original, path); err != nil {
		t.Fatalf("SaveSurface failed: %v", err)
	}

	loaded, err := LoadSurface(path)
	if err != nil {
		t.Fatalf("LoadSurface failed: %v", err)
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

func TestSaveSurface_ValidJSON(t *testing.T) {
	engine := gin.New()
	engine.GET("/api/v1/users", func(c *gin.Context) {})

	reg := NewRegistry()
	surface := reg.CaptureSnapshot(engine)

	dir := t.TempDir()
	path := filepath.Join(dir, "surface.json")

	if err := SaveSurface(surface, path); err != nil {
		t.Fatalf("SaveSurface failed: %v", err)
	}

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
}

func TestFormatBreakingChanges_Empty(t *testing.T) {
	result := FormatBreakingChanges(nil)
	if result != "No breaking changes detected." {
		t.Errorf("expected 'No breaking changes detected.', got %q", result)
	}
}

func TestValidateErrorFormat(t *testing.T) {
	tests := []struct {
		name   string
		body   []byte
		expect bool
	}{
		{"standard", []byte(`{"error":"not found","code":"NOT_FOUND"}`), true},
		{"with details", []byte(`{"error":"not found","code":"NOT_FOUND","details":"user 123"}`), true},
		{"no code", []byte(`{"error":"not found"}`), false},
		{"no error", []byte(`{"code":"NOT_FOUND"}`), false},
		{"empty", []byte(`{}`), false},
		{"invalid json", []byte(`not json`), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateErrorFormat(tt.body)
			if got != tt.expect {
				t.Errorf("ValidateErrorFormat(%s) = %v, want %v", tt.name, got, tt.expect)
			}
		})
	}
}

func TestIsStandardErrorCode(t *testing.T) {
	standard := []string{"NOT_FOUND", "ALREADY_EXISTS", "CONFLICT", "UNAUTHORIZED", "FORBIDDEN", "INVALID_INPUT", "TIMEOUT", "UNAVAILABLE", "NOT_IMPLEMENTED", "RATE_LIMITED", "INTERNAL_ERROR", "PRECONDITION_FAILED"}
	for _, code := range standard {
		if !IsStandardErrorCode(code) {
			t.Errorf("expected %s to be standard", code)
		}
	}

nonstandard := []string{"CUSTOM_ERROR", "MY_CODE", ""}
	for _, code := range nonstandard {
		if IsStandardErrorCode(code) {
			t.Errorf("expected %s to NOT be standard", code)
		}
	}
}
