package frontend

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// HealthResponse represents the health endpoint response
type HealthResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// StatusResponse represents the status endpoint response
type StatusResponse struct {
	Status  string            `json:"status"`
	Message string            `json:"message"`
	Data    map[string]string `json:"data"`
}

// Handler serves all frontend HTML pages.
type Handler struct {
	backendURL string
	httpClient *http.Client
}

// NewHandler creates a frontend handler.
// backendURL is the base URL for API proxy calls (empty = same server).
func NewHandler(backendURL string) *Handler {
	// TLS-aware client — trusts self-signed certs for internal health checks
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed certs in dev
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	return &Handler{
		backendURL: strings.TrimRight(backendURL, "/"),
		httpClient: client,
	}
}

func (h *Handler) normalizeFrontendRole(role string) string {
	value := strings.ToLower(strings.TrimSpace(role))
	switch value {
	case "sysadmin", "system-admin", "system_admin":
		return "system-manager"
	case "superadmin", "super-admin":
		return "admin"
	case "api-manager", "api_manager":
		return "manager"
	case "admin", "manager", "system-manager":
		return value
	default:
		return "user"
	}
}

func (h *Handler) defaultPathForRole(role string) string {
	switch h.normalizeFrontendRole(role) {
	case "system-manager":
		return "/system-manager"
	case "admin":
		return "/admin"
	case "manager":
		return "/manager"
	default:
		return "/"
	}
}

func setNoCacheHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}

func (h *Handler) requireFrontendRoles(allowed ...string) gin.HandlerFunc {
	allowedSet := make(map[string]bool, len(allowed))
	for _, role := range allowed {
		allowedSet[h.normalizeFrontendRole(role)] = true
	}

	return func(c *gin.Context) {
		authToken := c.GetHeader("Authorization")
		if authToken == "" {
			authToken, _ = c.Cookie("authToken")
		}
		if authToken == "" {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		role := c.GetHeader("X-User-Role")
		if role == "" {
			role, _ = c.Cookie("userRole")
		}
		normalized := h.normalizeFrontendRole(role)
		if !allowedSet[normalized] {
			c.Redirect(http.StatusFound, h.defaultPathForRole(normalized))
			c.Abort()
			return
		}

		c.Next()
	}
}

// backendURLFromRequest derives the backend URL from the incoming request
// so client-side JS always uses the correct scheme and host.
func (h *Handler) backendURLFromRequest(c *gin.Context) string {
	if h.backendURL != "" {
		return h.backendURL
	}
	scheme := "https"
	if c.Request.TLS == nil && c.GetHeader("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	return scheme + "://" + c.Request.Host
}

// templateData builds the common template data map.
func (h *Handler) templateData(c *gin.Context, title, pageName string, extra gin.H) gin.H {
	authToken := c.GetHeader("Authorization")
	if authToken == "" {
		authToken, _ = c.Cookie("authToken")
	}
	isAuth := authToken != ""

	userName := "Guest"
	if fromCookie, _ := c.Cookie("userName"); strings.TrimSpace(fromCookie) != "" {
		userName = fromCookie
	}

	data := gin.H{
		"title":      title,
		"pageName":   pageName,
		"page":       pageName,
		"isAuth":     isAuth,
		"userName":   userName,
		"backendURL": h.backendURLFromRequest(c),
	}
	for k, v := range extra {
		data[k] = v
	}
	return data
}

// Dashboard serves the public landing page (no auth required)
func (h *Handler) Dashboard(c *gin.Context) {
	health, _ := h.fetchHealth()
	data := h.templateData(c, "AxiomNizam - Enterprise Data Control Plane", "public-dashboard", gin.H{
		"health": health,
	})
	c.HTML(http.StatusOK, "layout.html", data)
}

// Signup serves the signup page
func (h *Handler) Signup(c *gin.Context) {
	setNoCacheHeaders(c)

	authToken := c.GetHeader("Authorization")
	if authToken == "" {
		authToken, _ = c.Cookie("authToken")
	}
	if strings.TrimSpace(authToken) != "" {
		role := c.GetHeader("X-User-Role")
		if role == "" {
			role, _ = c.Cookie("userRole")
		}
		c.Redirect(http.StatusFound, h.defaultPathForRole(role))
		return
	}

	health, _ := h.fetchHealth()
	c.HTML(http.StatusOK, "layout.html", gin.H{
		"title":      "AxiomNizam - Sign Up",
		"pageName":   "signup",
		"page":       "signup",
		"isAuth":     false,
		"userName":   "Guest",
		"backendURL": h.backendURLFromRequest(c),
		"health":     health,
		"hideChrome": true,
	})
}

// Login serves the login page
func (h *Handler) Login(c *gin.Context) {
	setNoCacheHeaders(c)

	authToken := c.GetHeader("Authorization")
	if authToken == "" {
		authToken, _ = c.Cookie("authToken")
	}
	if strings.TrimSpace(authToken) != "" {
		role := c.GetHeader("X-User-Role")
		if role == "" {
			role, _ = c.Cookie("userRole")
		}
		c.Redirect(http.StatusFound, h.defaultPathForRole(role))
		return
	}

	health, _ := h.fetchHealth()
	c.HTML(http.StatusOK, "layout.html", gin.H{
		"title":      "AxiomNizam - Login",
		"pageName":   "login",
		"page":       "login",
		"isAuth":     false,
		"userName":   "Guest",
		"backendURL": h.backendURLFromRequest(c),
		"health":     health,
		"hideChrome": true,
	})
}

// Admin serves the admin dashboard
func (h *Handler) Admin(c *gin.Context) {
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - Admin", "admin", nil))
}

// Manager serves the manager portal
func (h *Handler) Manager(c *gin.Context) {
	setNoCacheHeaders(c)
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - Manager Portal", "manager", nil))
}

// SystemManager serves the system manager dashboard
func (h *Handler) SystemManager(c *gin.Context) {
	setNoCacheHeaders(c)
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - System Manager", "system-manager", nil))
}

// Analytics serves the analytics dashboard
func (h *Handler) Analytics(c *gin.Context) {
	embedded := c.Query("embed") == "1" || strings.EqualFold(c.Query("embed"), "true")
	if embedded {
		setNoCacheHeaders(c)
	}
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - Analytics Dashboard", "analytics-dashboard", gin.H{
		"embedded": embedded,
	}))
}

// CDCETL serves the CDC/ETL dashboard
func (h *Handler) CDCETL(c *gin.Context) {
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - CDC/ETL Dashboard", "cdc-etl-dashboard", nil))
}

// NetIntel serves the Network Intelligence dashboard
func (h *Handler) NetIntel(c *gin.Context) {
	embedded := c.Query("embed") == "1" || strings.EqualFold(c.Query("embed"), "true")
	if embedded {
		setNoCacheHeaders(c)
	}
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - Network Intelligence", "netintel-dashboard", gin.H{
		"embedded": embedded,
	}))
}

// Governance serves the governance dashboard
func (h *Handler) Governance(c *gin.Context) {
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - Governance Console", "governance-dashboard", nil))
}

// OperationsCenter serves the operations center
func (h *Handler) OperationsCenter(c *gin.Context) {
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - Operations Center", "operations-center", nil))
}

// VersionLineage serves the version and lineage explorer
func (h *Handler) VersionLineage(c *gin.Context) {
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - Version & Lineage Explorer", "version-lineage-dashboard", nil))
}

// GIS serves the GIS dashboard
func (h *Handler) GIS(c *gin.Context) {
	embedded := c.Query("embed") == "1" || strings.EqualFold(c.Query("embed"), "true")
	if embedded {
		setNoCacheHeaders(c)
	}
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - GIS Dashboard", "gis-dashboard", gin.H{
		"embedded": embedded,
	}))
}

// IAMAdmin serves the IAM admin console
func (h *Handler) IAMAdmin(c *gin.Context) {
	setNoCacheHeaders(c)
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - IAM Admin Console", "iam-admin", nil))
}

// ObjectStorage serves the Object Storage console
func (h *Handler) ObjectStorage(c *gin.Context) {
	setNoCacheHeaders(c)
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - Object Storage", "object-storage", nil))
}

// TwoFactor serves the 2FA management page
func (h *Handler) TwoFactor(c *gin.Context) {
	setNoCacheHeaders(c)
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - Two-Factor Authentication", "two-factor", nil))
}

// Conductor serves the conductor dashboard
func (h *Handler) Conductor(c *gin.Context) {
	c.HTML(http.StatusOK, "layout.html", h.templateData(c, "AxiomNizam - Conductor", "conductor-dashboard", nil))
}

// Favicon serves a favicon
func (h *Handler) Favicon(c *gin.Context) {
	c.Header("Content-Type", "image/svg+xml")
	c.String(http.StatusOK, `<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>⚙️</text></svg>`)
}

// APIHealth fetches and returns health status as JSON.
func (h *Handler) APIHealth(c *gin.Context) {
	health, err := h.fetchHealth()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"status":   "error",
			"code":     "backend_unreachable",
			"message":  fmt.Sprintf("Backend unreachable: %v", err),
			"frontend": "ok",
		})
		return
	}
	c.JSON(http.StatusOK, health)
}

// APIStatus fetches and returns status as JSON.
func (h *Handler) APIStatus(c *gin.Context) {
	status, err := h.fetchStatus()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Failed to fetch status: %v", err),
		})
		return
	}
	c.JSON(http.StatusOK, status)
}

// fetchHealth makes a request to the backend health endpoint.
func (h *Handler) fetchHealth() (*HealthResponse, error) {
	// When backendURL is empty, use the local server (HTTPS since TLS is enabled)
	baseURL := h.backendURL
	if baseURL == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8000"
		}
		baseURL = "https://127.0.0.1:" + port
	}

	// Build candidate URLs with fallbacks for Docker networking
	seen := map[string]bool{}
	var candidates []string
	fallbacks := []string{"axiomnizam", "host.docker.internal", "127.0.0.1"}

	candidates = append(candidates, baseURL)
	if strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1") {
		for _, host := range fallbacks {
			fallback := strings.Replace(baseURL, "localhost", host, 1)
			fallback = strings.Replace(fallback, "127.0.0.1", host, 1)
			if !seen[fallback] {
				seen[fallback] = true
				candidates = append(candidates, fallback)
			}
		}
	}

	var lastErr error
	for _, candidate := range candidates {
		resp, err := h.httpClient.Get(fmt.Sprintf("%s/health", candidate))
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("status %d from %s", resp.StatusCode, candidate)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		var health HealthResponse
		if err := json.Unmarshal(body, &health); err != nil {
			return nil, err
		}
		return &health, nil
	}
	return nil, lastErr
}

// fetchStatus makes a request to the backend status endpoint.
func (h *Handler) fetchStatus() (*StatusResponse, error) {
	baseURL := h.backendURL
	if baseURL == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8000"
		}
		baseURL = "https://127.0.0.1:" + port
	}

	resp, err := h.httpClient.Get(fmt.Sprintf("%s/status", baseURL))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var status StatusResponse
	err = json.Unmarshal(body, &status)
	if err != nil {
		return nil, err
	}

	return &status, nil
}
