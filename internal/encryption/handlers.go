package encryption

import (
	"net/http"
	"time"

	"example.com/axiomnizam/internal/audit"

	"github.com/gin-gonic/gin"
)

type EncryptionHandler struct {
	manager            SecretsManager
	keyDualWriteStore  EncryptionKeyDualWriteStore
	auditMgr           *audit.AuditComplianceManager
}

// NewEncryptionHandler creates handler
func NewEncryptionHandler(manager SecretsManager) *EncryptionHandler {
	return &EncryptionHandler{manager: manager}
}

// SetAuditManager wires the central audit system for encryption key audit events (Phase 7).
func (h *EncryptionHandler) SetAuditManager(am *audit.AuditComplianceManager) {
	h.auditMgr = am
}

// logAuditEvent forwards an encryption event to the central audit system.
func (h *EncryptionHandler) logAuditEvent(action audit.AuditAction, resourceID, userID, sourceIP string) {
	if h.auditMgr == nil {
		return
	}
	_ = h.auditMgr.LogAuditEvent(&audit.AuditLog{
		TenantID:     "system",
		UserID:       userID,
		Action:       action,
		Result:       audit.ResultSuccess,
		ResourceType: "EncryptionKey",
		ResourceID:   resourceID,
		SourceIP:     sourceIP,
		Timestamp:    time.Now(),
	})
}

// CreateKey handles POST /api/v1/encryption/keys
func (h *EncryptionHandler) CreateKey(c *gin.Context) {
	var req struct {
		TenantID  string `json:"tenantId" binding:"required"`
		Name      string `json:"name" binding:"required"`
		Algorithm string `json:"algorithm"`
		KeyLength int    `json:"keyLength"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	key := &EncryptionKey{
		TenantID:  req.TenantID,
		Name:      req.Name,
		Algorithm: req.Algorithm,
		KeyLength: req.KeyLength,
		Status:    "Active",
		CreatedAt: time.Now(),
		Version:   1,
	}

	if err := h.manager.CreateKey(key); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	// Phase 7: Forward encryption key creation to central audit system.
	h.logAuditEvent(audit.ActionCreate, key.ID, key.TenantID, c.ClientIP())

	c.JSON(http.StatusCreated, key)
}

// GetKey handles GET /api/v1/encryption/keys/:id
func (h *EncryptionHandler) GetKey(c *gin.Context) {
	id := c.Param("id")
	key, err := h.manager.GetKey(id)
	if err != nil {
		c.JSON(http.StatusNotFound, MessageResponse{Error: "key not found"})
		return
	}

	// Don't expose key material
	key.KeyMaterial = ""
	c.JSON(http.StatusOK, key)
}

// ListKeys handles GET /api/v1/encryption/keys
func (h *EncryptionHandler) ListKeys(c *gin.Context) {
	tenantID := c.Query("tenantId")
	keys, err := h.manager.ListKeys(tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	// Don't expose key material
	for _, key := range keys {
		key.KeyMaterial = ""
	}

	c.JSON(http.StatusOK, KeyListResponse{Keys: keys, Count: len(keys)})
}

// RotateKey handles POST /api/v1/encryption/keys/:id/rotate
func (h *EncryptionHandler) RotateKey(c *gin.Context) {
	id := c.Param("id")
	rotated, err := h.manager.RotateKey(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	// Phase 7: Forward encryption key rotation to central audit system.
	h.logAuditEvent(audit.ActionUpdate, id, "system", c.ClientIP())

	c.JSON(http.StatusOK, rotated)
}

// DeleteKey handles DELETE /api/v1/encryption/keys/:id
func (h *EncryptionHandler) DeleteKey(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.DeleteKey(id); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	// Phase 7: Forward encryption key deletion to central audit system.
	h.logAuditEvent(audit.ActionDelete, id, "system", c.ClientIP())

	c.JSON(http.StatusOK, MessageResponse{Message: "key revoked"})
}

// Encrypt handles POST /api/v1/encryption/encrypt
func (h *EncryptionHandler) Encrypt(c *gin.Context) {
	var req EncryptionRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	field, err := h.manager.Encrypt(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, field)
}

// Decrypt handles POST /api/v1/encryption/decrypt
func (h *EncryptionHandler) Decrypt(c *gin.Context) {
	var req DecryptionRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	resp := &DecryptionResponse{
		Data:      req.Data,
		Decrypted: true,
		Timestamp: time.Now(),
	}

	// Decrypt each field
	for _, field := range req.Fields {
		if val, exists := req.Data[field]; exists {
			if encVal, ok := val.(*EncryptedField); ok {
				decrypted, err := h.manager.Decrypt(encVal)
				if err != nil {
					c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
					return
				}
				resp.Data[field] = decrypted
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

// CreatePolicy handles POST /api/v1/encryption/policies
func (h *EncryptionHandler) CreatePolicy(c *gin.Context) {
	var req struct {
		TenantID     string      `json:"tenantId" binding:"required"`
		Name         string      `json:"name" binding:"required"`
		ResourceType string      `json:"resourceType" binding:"required"`
		FieldRules   []FieldRule `json:"fieldRules" binding:"required"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	policy := &FieldEncryptionPolicy{
		TenantID:     req.TenantID,
		Name:         req.Name,
		ResourceType: req.ResourceType,
		FieldRules:   req.FieldRules,
		Enabled:      true,
		CreatedAt:    time.Now(),
	}

	// Store policy (to be implemented)
	c.JSON(http.StatusCreated, policy)
}

// ListPolicies handles GET /api/v1/encryption/policies
func (h *EncryptionHandler) ListPolicies(c *gin.Context) {
	// Retrieve policies (to be implemented)
	c.JSON(http.StatusOK, PolicyListResponse{Policies: []interface{}{}, Count: 0})
}

// RegisterEncryptionRoutes registers all encryption routes
func RegisterEncryptionRoutes(router *gin.Engine, manager SecretsManager) {
	handler := NewEncryptionHandler(manager)

	group := router.Group("/api/v1/encryption")
	{
		group.POST("/keys", handler.CreateKey)
		group.GET("/keys", handler.ListKeys)
		group.GET("/keys/:id", handler.GetKey)
		group.POST("/keys/:id/rotate", handler.RotateKey)
		group.DELETE("/keys/:id", handler.DeleteKey)
		group.POST("/encrypt", handler.Encrypt)
		group.POST("/decrypt", handler.Decrypt)
		group.POST("/policies", handler.CreatePolicy)
		group.GET("/policies", handler.ListPolicies)
	}
}
