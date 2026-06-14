package main

import (
	"context"
	ctx "context"
	"fmt"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/client"
	"github.com/spf13/cobra"
)

var (
	username     string
	password     string
	serverURL    string
	insecure     bool
	contextAlias string
	orgID        string
	apiKey       string
	loginMethod  string // "password" or "api-key"
)

var loginCmd = &cobra.Command{
	Use:   "login [server-url]",
	Short: "Authenticate with AxiomNizam server",
	Long: `Login to an AxiomNizam server and save authentication token.

Supports multiple authentication methods:
- Interactive login (username/password)
- API key authentication
- Token-based auth

Examples:
  axiomnizamctl login                                    # Interactive login to default server
  axiomnizamctl login https://api.example.com            # Login to specific server
  axiomnizamctl login --username admin --password secret # Non-interactive login
  axiomnizamctl login --api-key my-key --server https://api.example.com`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			serverURL = args[0]
		}
		return handleLogin()
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from AxiomNizam",
	Long:  "Remove authentication token and clear current context",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleLogout()
	},
}

var currentUserCmd = &cobra.Command{
	Use:   "current-user",
	Short: "Show current logged-in user",
	Long:  "Display information about the currently authenticated user",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleCurrentUser()
	},
}

func handleLogin() error {
	fmt.Println("\n🔐 AxiomNizam Login")
	fmt.Println(strings.Repeat("─", 40))

	// Determine server URL
	if serverURL == "" {
		// Use current context server if available
		if configManager != nil && configManager.GetCurrentContext() != nil {
			serverURL = configManager.GetCurrentContext().Cluster.Server
		} else {
			serverURL = promptInput("Server URL (default: http://localhost:8000)")
			if serverURL == "" {
				serverURL = "http://localhost:8000"
			}
		}
	}

	// Validate server URL
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "http://" + serverURL
	}

	fmt.Printf("\n📍 Server: %s\n", serverURL)

	// Determine authentication method
	if apiKey != "" {
		return loginWithAPIKey(serverURL)
	}

	if loginMethod == "" {
		loginMethod = "password"
	}

	if loginMethod == "password" {
		return loginWithPassword(serverURL)
	}

	return fmt.Errorf("unsupported login method: %s", loginMethod)
}

func loginWithPassword(serverURL string) error {
	// Get credentials
	if username == "" {
		username = promptInput("Username")
	}
	if username == "" {
		return NewCommandError(ErrInvalidInput, "Username is required")
	}

	if password == "" {
		password = promptPassword("Password")
	}
	if password == "" {
		return NewCommandError(ErrInvalidInput, "Password is required")
	}

	// Create temporary client
	tempClient := client.NewClient(serverURL)
	tempClient.SetSkipTLSVerify(insecure)

	// Perform login
	loginReq := map[string]string{
		"username": username,
		"password": password,
	}

	response, err := tempClient.Post(context.Background(), "/api/v1/auth/login", loginReq)
	if err != nil {
		return NewCommandError(ErrNetwork, "Login request failed", err.Error())
	}

	if response.StatusCode == 401 {
		return NewCommandError(ErrUnauthorized, "Invalid username or password")
	}

	if response.StatusCode >= 400 {
		return NewCommandError(ErrServerError, fmt.Sprintf("Login failed (%d)", response.StatusCode), response.Status)
	}

	// Parse response
	var result struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expiresAt,omitempty"`
		User      struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"user"`
	}

	if err := response.JSON(&result); err != nil {
		return NewCommandError(ErrInvalidInput, "Failed to parse login response", err.Error())
	}

	if result.Token == "" {
		return NewCommandError(ErrServerError, "No authentication token in response")
	}

	// Save to config
	return saveLoginContext(serverURL, result.Token, username)
}

func loginWithAPIKey(serverURL string) error {
	if apiKey == "" {
		apiKey = promptPassword("API Key")
	}
	if apiKey == "" {
		return NewCommandError(ErrInvalidInput, "API key is required")
	}

	// Create temporary client
	tempClient := client.NewClient(serverURL)
	tempClient.SetSkipTLSVerify(insecure)
	tempClient.SetToken(apiKey)

	// Verify API key
	response, err := tempClient.Get(context.Background(), "/api/v1/auth/verify", nil)
	if err != nil {
		return NewCommandError(ErrNetwork, "API key verification failed", err.Error())
	}

	if response.StatusCode == 401 {
		return NewCommandError(ErrUnauthorized, "Invalid API key")
	}

	if response.StatusCode >= 400 {
		return NewCommandError(ErrServerError, fmt.Sprintf("API key verification failed (%d)", response.StatusCode), response.Status)
	}

	// Save to config
	var result struct {
		User struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"user"`
	}
	response.JSON(&result)

	return saveLoginContext(serverURL, apiKey, result.User.Name)
}

func saveLoginContext(serverURL, token, user string) error {
	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	// Determine context name
	ctxName := contextAlias
	if ctxName == "" {
		ctxName = "default"
	}

	// Save token
	if err := configManager.SetToken(token); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	// Create or update context
	newContext := &client.Context{
		Name: ctxName,
		Cluster: &client.ClusterInfo{
			Server:                serverURL,
			InsecureSkipTLSVerify: insecure,
		},
		User:      user,
		Namespace: "default",
	}

	if err := configManager.AddOrUpdateContext(newContext); err != nil {
		return fmt.Errorf("failed to save context: %w", err)
	}

	// Set as current context
	if err := configManager.SetCurrentContext(ctxName); err != nil {
		return fmt.Errorf("failed to set current context: %w", err)
	}

	// Success message
	fmt.Println("\n✅ Authentication successful!")
	fmt.Printf("   User: %s\n", user)
	fmt.Printf("   Server: %s\n", serverURL)
	fmt.Printf("   Context: %s\n", ctxName)
	fmt.Println("\n💡 Tip: Use 'axiomnizamctl config view' to see your configuration")

	return nil
}

func handleLogout() error {
	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	if err := configManager.DeleteToken(); err != nil {
		return NewCommandError(ErrConfigError, "Failed to logout", err.Error())
	}

	printSuccessMessage("Successfully logged out")
	printInfoMessage("Run 'axiomnizamctl login' to authenticate again")

	return nil
}

func handleCurrentUser() error {
	if err := ensureConfigManagerLoaded(); err != nil {
		return err
	}

	token := configManager.GetToken()
	if token == "" {
		return NewCommandError(ErrUnauthorized, "Not authenticated", "Run 'axiomnizamctl login' first")
	}

	context := configManager.GetCurrentContext()
	if context == nil {
		return NewCommandError(ErrConfigError, "No context configured")
	}

	fmt.Println("\n👤 Current User")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("   User: %s\n", context.User)
	fmt.Printf("   Context: %s\n", context.Name)
	fmt.Printf("   Server: %s\n", context.Cluster.Server)
	fmt.Printf("   Namespace: %s\n", context.Namespace)

	// Try to fetch user info from server
	if apiClient != nil {
		response, err := apiClient.Get(ctx.Background(), "/api/v1/auth/whoami", nil)
		if err == nil && response.StatusCode == 200 {
			var userInfo struct {
				ID    string `json:"id"`
				Name  string `json:"name"`
				Email string `json:"email"`
				Role  string `json:"role"`
			}
			if response.JSON(&userInfo) == nil {
				if userInfo.Email != "" {
					fmt.Printf("   Email: %s\n", userInfo.Email)
				}
				if userInfo.Role != "" {
					fmt.Printf("   Role: %s\n", userInfo.Role)
				}
			}
		}
	}

	return nil
}

func init() {
	// Login command flags
	loginCmd.Flags().StringVarP(&username, "username", "u", "", "Username for authentication")
	loginCmd.Flags().StringVarP(&password, "password", "p", "", "Password for authentication (interactive if not provided)")
	loginCmd.Flags().StringVar(&apiKey, "api-key", "", "API key for authentication (alternative to password)")
	loginCmd.Flags().StringVar(&loginMethod, "method", "password", "Authentication method: password or api-key")
	loginCmd.Flags().StringVar(&contextAlias, "context", "", "Name to save this context as (default: default)")
	loginCmd.Flags().StringVar(&serverURL, "server", "", "Server URL (can also be provided as argument)")
	loginCmd.Flags().BoolVar(&insecure, "insecure-skip-tls-verify", false, "Skip TLS certificate verification")

	// Mark password flag as sensitive (hides from help if needed)
	loginCmd.Flags().Lookup("password").NoOptDefVal = "true"
	loginCmd.Flags().Lookup("api-key").NoOptDefVal = "true"
}
