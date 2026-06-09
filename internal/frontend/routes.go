package frontend

import (
	"html/template"
	"log"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers all frontend page routes on the given Gin engine.
// It loads HTML templates from internal/frontend/templates/ and serves static files.
func RegisterRoutes(router *gin.Engine, handler *Handler) {
	// Resolve template directory — works from both local dev and Docker container.
	templateDir := findTemplateDir()
	if templateDir == "" {
		log.Println("⚠️  Frontend templates not found — frontend pages disabled")
		return
	}

	// Set template functions
	router.SetFuncMap(template.FuncMap{
		"safeHTML": func(html string) template.HTML {
			return template.HTML(html)
		},
	})

	// Load HTML templates
	router.LoadHTMLGlob(filepath.Join(templateDir, "*.html"))

	// Serve static files (CSS, JS, fonts, images)
	router.Static("/static", templateDir)

	// Page routes
	router.GET("/", handler.Dashboard)
	router.GET("/signup", handler.Signup)
	router.GET("/login", handler.Login)
	router.GET("/admin", handler.Admin)
	router.GET("/system-manager", handler.SystemManager)
	router.GET("/manager", handler.Manager)
	router.GET("/gis", handler.GIS)
	router.GET("/analytics", handler.Analytics)
	router.GET("/cdc-etl", handler.CDCETL)
	router.GET("/netintel", handler.NetIntel)
	router.GET("/conductor", handler.Conductor)
	router.GET("/governance", handler.requireFrontendRoles("admin", "system-manager"), handler.Governance)
	router.GET("/operations-center", handler.requireFrontendRoles("admin", "system-manager", "manager"), handler.OperationsCenter)
	router.GET("/lineage-version", handler.requireFrontendRoles("admin", "system-manager"), handler.VersionLineage)
	router.GET("/iam-admin", handler.requireFrontendRoles("system-manager"), handler.IAMAdmin)
	router.GET("/object-storage", handler.requireFrontendRoles("system-manager"), handler.ObjectStorage)
	router.GET("/two-factor", handler.TwoFactor)
	router.GET("/favicon.ico", handler.Favicon)

	// API proxy routes (used by frontend JS for health/status checks)
	router.GET("/api/health", handler.APIHealth)
	router.GET("/api/status", handler.APIStatus)

	log.Printf("🌐 Frontend pages registered (templates: %s)", templateDir)
}

// findTemplateDir locates the frontend templates directory.
// Checks multiple paths to support both local dev and Docker container layouts.
func findTemplateDir() string {
	candidates := []string{
		"internal/frontend/templates",
		"/app/internal/frontend/templates",
	}

	// Also check relative to executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, "internal", "frontend", "templates"))
	}

	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "layout.html")); err == nil {
			return dir
		}
	}
	return ""
}
