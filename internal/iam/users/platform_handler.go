package users

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/iam/authn"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"

	"github.com/gin-gonic/gin"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// PlatformUser represents a platform user with role-based access.
type PlatformUser struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreatePlatformUserRequest is the request body for creating a platform user.
type CreatePlatformUserRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role" binding:"required"`
}

// UpdatePlatformUserRequest is the request body for updating a platform user.
type UpdatePlatformUserRequest struct {
	Email  string `json:"email"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

// PlatformUserHandler manages platform user CRUD operations.
type PlatformUserHandler struct {
	mu       sync.RWMutex
	users    map[string]*PlatformUser
	etcd     *clientv3.Client
	stateKey string
}

var validPlatformRoles = map[string]bool{"admin": true, "manager": true, "user": true}

// NewPlatformUserHandler creates a new platform user handler.
func NewPlatformUserHandler(etcd *clientv3.Client) *PlatformUserHandler {
	h := &PlatformUserHandler{
		users:    make(map[string]*PlatformUser),
		etcd:     etcd,
		stateKey: "handlers:users:state",
	}
	h.loadState()
	return h
}

func (h *PlatformUserHandler) loadState() {
	if h.etcd == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := h.etcd.Get(ctx, h.stateKey)
	if err != nil || len(resp.Kvs) == 0 {
		return
	}
	var state map[string]*PlatformUser
	if err := json.Unmarshal(resp.Kvs[0].Value, &state); err != nil {
		logging.Z().Warn("failed to unmarshal platform user state", zap.Error(err))
		return
	}
	h.users = state
}

func (h *PlatformUserHandler) saveState() {
	if h.etcd == nil {
		return
	}
	data, err := json.Marshal(h.users)
	if err != nil {
		logging.Z().Error("failed to marshal platform user state", zap.Error(err))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := h.etcd.Put(ctx, h.stateKey, string(data)); err != nil {
		logging.Z().Error("failed to persist platform user state", zap.Error(err))
	}
}

func generateUserID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// ValidateCredentials checks username/password against the in-memory store.
// Returns a minimal authn.PlatformUser to satisfy the PlatformUserStore interface.
func (h *PlatformUserHandler) ValidateCredentials(username, password string) (*authn.PlatformUser, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	key := strings.ToLower(strings.TrimSpace(username))
	if key == "" {
		return nil, false
	}

	user, exists := h.users[key]
	if !exists || strings.ToLower(user.Status) != "active" {
		return nil, false
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, false
	}

	return &authn.PlatformUser{
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
		Status:   user.Status,
	}, true
}

// EnsureFederatedUser creates or returns an existing user from OAuth login.
func (h *PlatformUserHandler) EnsureFederatedUser(username, email, defaultRole string) (*authn.PlatformUser, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := strings.ToLower(strings.TrimSpace(username))
	if key == "" {
		return nil, fmt.Errorf("username is required")
	}

	if existing, ok := h.users[key]; ok {
		return &authn.PlatformUser{
			Username: existing.Username,
			Email:    existing.Email,
			Role:     existing.Role,
			Status:   existing.Status,
		}, nil
	}

	now := time.Now()
	user := &PlatformUser{
		ID:        generateUserID(),
		Username:  username,
		Email:     strings.TrimSpace(email),
		Role:      defaultRole,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}
	h.users[key] = user
	h.saveState()

	return &authn.PlatformUser{
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
		Status:   user.Status,
	}, nil
}

// Create handles POST /api/v1/platform/users
func (h *PlatformUserHandler) Create(c *gin.Context) {
	var req CreatePlatformUserRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	if !validPlatformRoles[req.Role] {
		c.JSON(400, gin.H{"error": "Invalid role. Must be one of: admin, manager, user"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to hash password"})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	key := strings.ToLower(strings.TrimSpace(req.Username))
	if _, exists := h.users[key]; exists {
		c.JSON(409, gin.H{"error": "Username already exists"})
		return
	}

	now := time.Now()
	user := &PlatformUser{
		ID:        generateUserID(),
		Username:  req.Username,
		Email:     req.Email,
		Password:  string(hashedPassword),
		Role:      req.Role,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}

	h.users[key] = user
	h.saveState()

	c.JSON(201, gin.H{
		"status":  "ok",
		"message": "User created successfully",
		"user": gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"status":     user.Status,
			"created_at": user.CreatedAt,
		},
	})
}

// List handles GET /api/v1/platform/users
func (h *PlatformUserHandler) List(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	users := make([]gin.H, 0, len(h.users))
	for _, user := range h.users {
		users = append(users, gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"status":     user.Status,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		})
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"users":  users,
		"total":  len(users),
	})
}

// Get handles GET /api/v1/platform/users/:username
func (h *PlatformUserHandler) Get(c *gin.Context) {
	username := strings.ToLower(strings.TrimSpace(c.Param("username")))

	h.mu.RLock()
	defer h.mu.RUnlock()

	user, exists := h.users[username]
	if !exists {
		c.JSON(404, gin.H{"error": "User not found"})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"user": gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"status":     user.Status,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		},
	})
}

// Update handles PUT /api/v1/platform/users/:username
func (h *PlatformUserHandler) Update(c *gin.Context) {
	username := strings.ToLower(strings.TrimSpace(c.Param("username")))

	var req UpdatePlatformUserRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	user, exists := h.users[username]
	if !exists {
		c.JSON(404, gin.H{"error": "User not found"})
		return
	}

	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Role != "" {
		if !validPlatformRoles[req.Role] {
			c.JSON(400, gin.H{"error": "Invalid role. Must be one of: admin, manager, user"})
			return
		}
		user.Role = req.Role
	}
	if req.Status != "" {
		user.Status = req.Status
	}
	user.UpdatedAt = time.Now()

	h.saveState()

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "User updated successfully",
		"user": gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"status":     user.Status,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		},
	})
}

// Delete handles DELETE /api/v1/platform/users/:username
func (h *PlatformUserHandler) Delete(c *gin.Context) {
	username := strings.ToLower(strings.TrimSpace(c.Param("username")))

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.users[username]; !exists {
		c.JSON(404, gin.H{"error": "User not found"})
		return
	}

	delete(h.users, username)
	h.saveState()

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "User deleted successfully",
	})
}

// ListPlatformUsers is an alias for List (backward compat with route registrations).
func (h *PlatformUserHandler) ListPlatformUsers(c *gin.Context) { h.List(c) }

// GetPlatformUser handles GET /api/v1/users/:id using the :id param as username key.
func (h *PlatformUserHandler) GetPlatformUser(c *gin.Context) {
	id := strings.ToLower(strings.TrimSpace(c.Param("id")))

	h.mu.RLock()
	defer h.mu.RUnlock()

	user, exists := h.users[id]
	if !exists {
		c.JSON(404, gin.H{"error": "User not found"})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"user": gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"status":     user.Status,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		},
	})
}

// CreatePlatformUser handles POST /api/v1/users (backward compat).
func (h *PlatformUserHandler) CreatePlatformUser(c *gin.Context) { h.Create(c) }

// UpdatePlatformUser handles PUT /api/v1/users/:id (backward compat).
func (h *PlatformUserHandler) UpdatePlatformUser(c *gin.Context) {
	// Rewrite :id to :username for the underlying Update handler
	c.Params = append(c.Params, gin.Param{Key: "username", Value: c.Param("id")})
	h.Update(c)
}

// DeletePlatformUser handles DELETE /api/v1/users/:id (backward compat).
func (h *PlatformUserHandler) DeletePlatformUser(c *gin.Context) {
	c.Params = append(c.Params, gin.Param{Key: "username", Value: c.Param("id")})
	h.Delete(c)
}

// ResetPassword handles POST /api/v1/platform/users/:username/reset-password
func (h *PlatformUserHandler) ResetPassword(c *gin.Context) {
	username := strings.ToLower(strings.TrimSpace(c.Param("username")))

	var req struct {
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	user, exists := h.users[username]
	if !exists {
		c.JSON(404, gin.H{"error": "User not found"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to hash password"})
		return
	}

	user.Password = string(hashedPassword)
	user.UpdatedAt = time.Now()
	h.saveState()

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "Password reset successfully",
	})
}
