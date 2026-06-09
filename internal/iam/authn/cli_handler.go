package authn

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// CLIAuthHandler handles authentication requests from the CLI.
type CLIAuthHandler struct {
	iamBaseURL string
	httpClient *http.Client
}

// NewCLIAuthHandler creates a CLI auth adapter backed by IAM endpoints.
func NewCLIAuthHandler() *CLIAuthHandler {
	baseURL := strings.TrimSpace(getEnv("IAM_INTERNAL_BASE_URL", ""))
	if baseURL == "" {
		baseURL = strings.TrimSpace(getEnv("IAM_ISSUER_URL", ""))
	}
	if baseURL == "" {
		baseURL = defaultIAMInternalBaseURL()
	}
	baseURL = normalizeIAMBaseURL(baseURL)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TLS_ENABLED")), "true") {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return &CLIAuthHandler{
		iamBaseURL: baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second, Transport: transport},
	}
}

func (h *CLIAuthHandler) pickPrimaryRole(roles []string) string {
	resolved := "user"
	for _, role := range roles {
		r := strings.ToLower(strings.TrimSpace(role))
		switch {
		case r == "system-manager" || r == "system_manager" || r == "system-admin" || r == "sysadmin":
			return "system-manager"
		case strings.Contains(r, "admin") && !strings.Contains(r, "account"):
			resolved = "admin"
		case (r == "manager" || r == "api-manager" || r == "api_manager") && resolved == "user":
			resolved = "manager"
		}
	}
	return resolved
}

func (h *CLIAuthHandler) callWhoAmI(token string) (map[string]interface{}, int, error) {
	req, err := http.NewRequest(http.MethodGet, h.iamBaseURL+"/iam/auth/whoami", nil)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, http.StatusBadGateway, err
	}
	return body, resp.StatusCode, nil
}

// Login handles CLI login requests.
func (h *CLIAuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	identifier := resolveIAMLoginIdentifier(req.Username)
	if identifier == "" || strings.TrimSpace(req.Password) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"email":    identifier,
		"password": req.Password,
	})

	resp, err := h.httpClient.Post(h.iamBaseURL+"/iam/auth/login", "application/json", bytes.NewReader(payload))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to reach IAM login endpoint"})
		return
	}
	defer resp.Body.Close()

	var loginResp struct {
		AccessToken string `json:"access_token"`
		ExpiresAt   string `json:"expires_at"`
		ExpiresIn   int    `json:"expires_in"`
		User        struct {
			DisplayName string `json:"display_name"`
			Email       string `json:"email"`
		} `json:"user"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to parse IAM login response"})
		return
	}

	if resp.StatusCode != http.StatusOK || strings.TrimSpace(loginResp.AccessToken) == "" {
		errMsg := strings.TrimSpace(loginResp.Error)
		if errMsg == "" {
			errMsg = "invalid username or password"
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": errMsg})
		return
	}

	expiresAt := strings.TrimSpace(loginResp.ExpiresAt)
	if expiresAt == "" && loginResp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(loginResp.ExpiresIn) * time.Second).Format(time.RFC3339)
	}

	name := strings.TrimSpace(loginResp.User.DisplayName)
	if name == "" {
		name = strings.TrimSpace(req.Username)
	}

	c.JSON(http.StatusOK, gin.H{
		"token":     loginResp.AccessToken,
		"expiresAt": expiresAt,
		"user": gin.H{
			"name":  name,
			"email": loginResp.User.Email,
		},
	})
}

// Verify verifies a bearer token by calling IAM whoami.
func (h *CLIAuthHandler) Verify(c *gin.Context) {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	whoami, status, err := h.callWhoAmI(token)
	if err != nil || status != http.StatusOK {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	name, _ := whoami["display_name"].(string)
	email, _ := whoami["email"].(string)
	if strings.TrimSpace(name) == "" {
		name = email
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"name":  name,
			"email": email,
		},
	})
}

// WhoAmI returns current user info.
func (h *CLIAuthHandler) WhoAmI(c *gin.Context) {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	whoami, status, err := h.callWhoAmI(token)
	if err != nil || status != http.StatusOK {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	userID, _ := whoami["user_id"].(string)
	name, _ := whoami["display_name"].(string)
	email, _ := whoami["email"].(string)
	if strings.TrimSpace(name) == "" {
		name = email
	}

	roles := make([]string, 0)
	if rawRoles, ok := whoami["roles"].([]interface{}); ok {
		for _, r := range rawRoles {
			if rs, ok := r.(string); ok {
				roles = append(roles, rs)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":    userID,
		"name":  name,
		"email": email,
		"role":  h.pickPrimaryRole(roles),
	})
}
