package encryption

import (
	"fmt"
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/audit"
	"axiomnizam.bitbd.net/axiomnizam/internal/lineage"
	"axiomnizam.bitbd.net/axiomnizam/internal/workflows"

	"github.com/gin-gonic/gin"
)

// Phase3Handlers manages all Phase 3 endpoints (encryption, lineage, audit, workflows).
type Phase3Handlers struct {
	encryptionMgr *FieldLevelEncryption
	lineageMgr    *lineage.DataLineageTracker
	auditMgr      *audit.AuditComplianceManager
	workflowMgr   *workflows.MultiVersionWorkflowManager
}

// NewPhase3Handlers creates Phase3Handlers
func NewPhase3Handlers(
	encMgr *FieldLevelEncryption,
	linMgr *lineage.DataLineageTracker,
	audMgr *audit.AuditComplianceManager,
	wfMgr *workflows.MultiVersionWorkflowManager,
) *Phase3Handlers {
	return &Phase3Handlers{
		encryptionMgr: encMgr,
		lineageMgr:    linMgr,
		auditMgr:      audMgr,
		workflowMgr:   wfMgr,
	}
}

// ========== ENCRYPTION ENDPOINTS ==========

// RegisterEncryptionKey registers new encryption key
func (h *Phase3Handlers) RegisterEncryptionKey(c *gin.Context) {
	type Request struct {
		KeyID       string    `json:"key_id" binding:"required"`
		Key         string    `json:"key" binding:"required"`
		ExpiresAt   time.Time `json:"expires_at"`
		EncryptType string    `json:"encrypt_type"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	encKey := &EncryptionKey{
		Name:        req.KeyID,
		KeyType:     KeyTypeDEK,
		Algorithm:   req.EncryptType,
		KeyMaterial: req.Key,
		CreatedAt:   time.Now(),
		ExpiresAt:   req.ExpiresAt,
		Status:      KeyStatusActive,
	}

	if err := h.encryptionMgr.RegisterKey(encKey); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, KeyRegisteredResponse{
		Message: "encryption key registered",
		KeyID:   req.KeyID,
	})
}

// AddEncryptionPolicy adds field encryption policy
func (h *Phase3Handlers) AddEncryptionPolicy(c *gin.Context) {
	type Request struct {
		TableName  string `json:"table_name" binding:"required"`
		ColumnName string `json:"column_name" binding:"required"`
		KeyID      string `json:"key_id" binding:"required"`
		Searchable bool   `json:"searchable"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	policy := &FieldEncryptionPolicy{
		Name:         req.ColumnName,
		ResourceType: req.TableName,
		KeyID:        req.KeyID,
		Enabled:      true,
		FieldRules: []FieldRule{
			{
				FieldName: req.ColumnName,
				Encrypt:   true,
				KeyID:     req.KeyID,
			},
		},
	}

	if err := h.encryptionMgr.AddEncryptionPolicy(policy); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, PolicyAddedResponse{
		Message: "encryption policy added",
		Table:   req.TableName,
		Column:  req.ColumnName,
	})
}

// EncryptFieldValue encrypts a field value
func (h *Phase3Handlers) EncryptFieldValue(c *gin.Context) {
	type Request struct {
		TableName  string `json:"table_name" binding:"required"`
		ColumnName string `json:"column_name" binding:"required"`
		Value      string `json:"value" binding:"required"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	encrypted, err := h.encryptionMgr.EncryptField(req.TableName+"."+req.ColumnName, req.Value, "default")
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, EncryptedFieldResponse{
		EncryptedData: encrypted.EncryptedValue,
		IV:            encrypted.IV,
		KeyID:         encrypted.KeyID,
	})
}

// DecryptFieldValue decrypts a field value
func (h *Phase3Handlers) DecryptFieldValue(c *gin.Context) {
	type Request struct {
		TableName     string `json:"table_name" binding:"required"`
		ColumnName    string `json:"column_name" binding:"required"`
		EncryptedData string `json:"encrypted_data" binding:"required"`
		IV            string `json:"iv" binding:"required"`
		KeyID         string `json:"key_id" binding:"required"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	encField := &EncryptedField{
		FieldName:      req.ColumnName,
		EncryptedValue: req.EncryptedData,
		IV:             req.IV,
		KeyID:          req.KeyID,
	}

	decrypted, err := h.encryptionMgr.DecryptField(encField)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, DecryptedFieldResponse{
		DecryptedValue: decrypted,
	})
}

// RotateEncryptionKey rotates encryption key
func (h *Phase3Handlers) RotateEncryptionKey(c *gin.Context) {
	keyID := c.Param("key_id")

	newKeyData := c.PostForm("new_key")
	if newKeyData == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "new_key required"})
		return
	}

	newKey := &EncryptionKey{
		Name:        "rotated-" + keyID,
		KeyType:     KeyTypeDEK,
		KeyMaterial: newKeyData,
		Status:      KeyStatusActive,
	}

	if _, err := h.encryptionMgr.RotateKey(keyID, newKey); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, KeyRotatedResponse{
		Message: "encryption key rotated",
		KeyID:   keyID,
	})
}

// GetEncryptionMetrics gets encryption metrics
func (h *Phase3Handlers) GetEncryptionMetrics(c *gin.Context) {
	metrics := h.encryptionMgr.GetEncryptionMetrics()

	c.JSON(http.StatusOK, MetricsResponse{Metrics: metrics})
}

// GetEncryptionStatus gets encryption status
func (h *Phase3Handlers) GetEncryptionStatus(c *gin.Context) {
	metrics := h.encryptionMgr.GetEncryptionMetrics()

	c.JSON(http.StatusOK, MetricsResponse{Metrics: metrics})
}

// ========== LINEAGE ENDPOINTS ==========

// RegisterDataNode registers a data node
func (h *Phase3Handlers) RegisterDataNode(c *gin.Context) {
	type Request struct {
		NodeID   string `json:"node_id" binding:"required"`
		NodeName string `json:"node_name" binding:"required"`
		NodeType string `json:"node_type" binding:"required"`
		Schema   string `json:"schema"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	node := &lineage.DataLineageNode{
		ID:     req.NodeID,
		Name:   req.NodeName,
		Type:   req.NodeType,
		Schema: req.Schema,
	}

	if err := h.lineageMgr.RegisterDataNode(node); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, NodeRegisteredResponse{
		Message: "data node registered",
		NodeID:  req.NodeID,
	})
}

// CreateLineageEdge creates data lineage edge
func (h *Phase3Handlers) CreateLineageEdge(c *gin.Context) {
	type Request struct {
		SourceNodeID string `json:"source_node_id" binding:"required"`
		TargetNodeID string `json:"target_node_id" binding:"required"`
		RelationType string `json:"relation_type" binding:"required"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	edge, err := h.lineageMgr.CreateLineageEdge(req.SourceNodeID, req.TargetNodeID, req.RelationType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, LineageEdgeCreatedResponse{
		Message:  "lineage edge created",
		EdgeID:   edge.ID,
		Source:   req.SourceNodeID,
		Target:   req.TargetNodeID,
		Relation: req.RelationType,
	})
}

// GetUpstreamLineage gets upstream lineage
func (h *Phase3Handlers) GetUpstreamLineage(c *gin.Context) {
	nodeID := c.Query("node_id")
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "node_id required"})
		return
	}

	lineageData := h.lineageMgr.GetUpstreamLineage(nodeID)

	c.JSON(http.StatusOK, UpstreamLineageResponse{UpstreamLineage: lineageData})
}

// GetDownstreamLineage gets downstream lineage
func (h *Phase3Handlers) GetDownstreamLineage(c *gin.Context) {
	nodeID := c.Query("node_id")
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "node_id required"})
		return
	}

	lineageData := h.lineageMgr.GetDownstreamLineage(nodeID)

	c.JSON(http.StatusOK, DownstreamLineageResponse{DownstreamLineage: lineageData})
}

// AnalyzeImpact analyzes change impact
func (h *Phase3Handlers) AnalyzeImpact(c *gin.Context) {
	type Request struct {
		SourceNodeID string `json:"source_node_id" binding:"required"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	impact := h.lineageMgr.AnalyzeImpact(req.SourceNodeID)

	c.JSON(http.StatusOK, ImpactAnalysisResponse{ImpactAnalysis: impact})
}

// GetLineageGraph gets lineage graph
func (h *Phase3Handlers) GetLineageGraph(c *gin.Context) {
	graph := h.lineageMgr.GetLineageGraph()

	c.JSON(http.StatusOK, LineageGraphResponse{LineageGraph: graph})
}

// GetLineageStats gets lineage statistics
func (h *Phase3Handlers) GetLineageStats(c *gin.Context) {
	stats := h.lineageMgr.GetLineageStats()

	c.JSON(http.StatusOK, StatisticsResponse{Statistics: stats})
}

// ========== AUDIT & COMPLIANCE ENDPOINTS ==========

// LogAuditEvent logs an audit event
func (h *Phase3Handlers) LogAuditEvent(c *gin.Context) {
	type Request struct {
		UserID       string                 `json:"user_id" binding:"required"`
		Action       string                 `json:"action" binding:"required"`
		ResourceType string                 `json:"resource_type"`
		ResourceID   string                 `json:"resource_id"`
		IPAddress    string                 `json:"ip_address"`
		Changes      map[string]interface{} `json:"changes"`
		Status       string                 `json:"status"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	changes := make([]audit.Change, 0)
	for field, value := range req.Changes {
		changes = append(changes, audit.Change{
			Field:    field,
			NewValue: value,
		})
	}

	auditLog := &audit.AuditLog{
		UserID:       req.UserID,
		Action:       audit.AuditAction(req.Action),
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		SourceIP:     req.IPAddress,
		Timestamp:    time.Now(),
		Changes:      changes,
	}

	if err := h.auditMgr.LogAuditEvent(auditLog); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, AckResponse{Message: "audit event logged"})
}

// RegisterComplianceRule registers compliance rule
func (h *Phase3Handlers) RegisterComplianceRule(c *gin.Context) {
	type Request struct {
		RuleID      string `json:"rule_id" binding:"required"`
		RuleName    string `json:"rule_name" binding:"required"`
		Framework   string `json:"framework" binding:"required"`
		Description string `json:"description"`
		Severity    string `json:"severity"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	rule := &audit.ComplianceRule{
		ID:          req.RuleID,
		Framework:   req.Framework,
		Description: req.Description,
		Requirement: req.RuleName,
		IsActive:    true,
		CreatedAt:   time.Now(),
	}

	if err := h.auditMgr.RegisterComplianceRule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, ComplianceRuleRegisteredResponse{
		Message: "compliance rule registered",
		RuleID:  req.RuleID,
	})
}

// GenerateComplianceReport generates compliance report
func (h *Phase3Handlers) GenerateComplianceReport(c *gin.Context) {
	framework := c.Query("framework")
	if framework == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "framework required"})
		return
	}

	report, err := h.auditMgr.GenerateComplianceReport(framework, time.Now().AddDate(-1, 0, 0), time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, ComplianceReportResponse{ComplianceReport: report})
}

// GetComplianceStatus gets compliance status
func (h *Phase3Handlers) GetComplianceStatus(c *gin.Context) {
	status := h.auditMgr.GetComplianceStatus()

	c.JSON(http.StatusOK, StatusResponse{Status: status})
}

// SearchAuditLogs searches audit logs
func (h *Phase3Handlers) SearchAuditLogs(c *gin.Context) {
	userID := c.Query("user_id")
	resourceType := c.Query("resource_type")

	logs := h.auditMgr.SearchAuditLogs(userID, resourceType)

	c.JSON(http.StatusOK, AuditLogsResponse{AuditLogs: logs})
}

// RecordViolation records compliance violation
func (h *Phase3Handlers) RecordViolation(c *gin.Context) {
	type Request struct {
		RuleID      string `json:"rule_id" binding:"required"`
		Description string `json:"description"`
		Severity    string `json:"severity"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	violation := &audit.ComplianceViolation{
		ID:            fmt.Sprintf("violation-%d", time.Now().UnixNano()),
		RuleID:        req.RuleID,
		Description:   req.Description,
		Severity:      req.Severity,
		Timestamp:     time.Now(),
		ViolationType: "compliance",
		Status:        "open",
	}

	if err := h.auditMgr.RecordViolation(violation); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, AckResponse{Message: "violation recorded"})
}

// ========== WORKFLOW ENDPOINTS ==========

// CreateWorkflow creates new workflow
func (h *Phase3Handlers) CreateWorkflow(c *gin.Context) {
	type Request struct {
		Name        string                   `json:"name" binding:"required"`
		Description string                   `json:"description"`
		Steps       []map[string]interface{} `json:"steps"`
		CreatedBy   string                   `json:"created_by"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	wfDef := &workflows.VersionedWorkflowDefinition{
		Name:        req.Name,
		Description: req.Description,
		CreatedBy:   req.CreatedBy,
		Status:      "draft",
	}

	version, err := h.workflowMgr.CreateWorkflow(wfDef)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, WorkflowCreatedResponse{
		Message:    "workflow created",
		WorkflowID: wfDef.ID,
		Version:    version.Version,
	})
}

// PublishWorkflowVersion publishes workflow version
func (h *Phase3Handlers) PublishWorkflowVersion(c *gin.Context) {
	type Request struct {
		WorkflowID    string                   `json:"workflow_id" binding:"required"`
		ChangeSummary string                   `json:"change_summary"`
		Steps         []map[string]interface{} `json:"steps"`
		CreatedBy     string                   `json:"created_by"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	newDef := &workflows.VersionedWorkflowDefinition{
		CreatedBy: req.CreatedBy,
		Status:    "published",
	}

	version, err := h.workflowMgr.PublishWorkflowVersion(req.WorkflowID, newDef)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, WorkflowVersionPublishedResponse{
		Message: "workflow version published",
		Version: version.Version,
	})
}

// StartWorkflowInstance starts workflow execution
func (h *Phase3Handlers) StartWorkflowInstance(c *gin.Context) {
	type Request struct {
		WorkflowID  string                 `json:"workflow_id" binding:"required"`
		Version     string                 `json:"version"`
		ContextData map[string]interface{} `json:"context_data"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	instance, err := h.workflowMgr.StartWorkflowInstance(req.WorkflowID, req.Version, req.ContextData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, WorkflowInstanceStartedResponse{
		Message:    "workflow instance started",
		InstanceID: instance.ID,
		Status:     instance.Status,
	})
}

// GetWorkflowMetrics gets workflow metrics
func (h *Phase3Handlers) GetWorkflowMetrics(c *gin.Context) {
	workflowID := c.Query("workflow_id")
	if workflowID == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "workflow_id required"})
		return
	}

	metrics := h.workflowMgr.GetWorkflowMetrics(workflowID)

	c.JSON(http.StatusOK, MetricsResponse{Metrics: metrics})
}

// GetWorkflowStatus gets workflow status
func (h *Phase3Handlers) GetWorkflowStatus(c *gin.Context) {
	status := h.workflowMgr.GetWorkflowStatus()

	c.JSON(http.StatusOK, StatusResponse{Status: status})
}

// GetInstanceHistory gets workflow instance history
func (h *Phase3Handlers) GetInstanceHistory(c *gin.Context) {
	workflowID := c.Query("workflow_id")
	if workflowID == "" {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: "workflow_id required"})
		return
	}

	limit := 100
	history := h.workflowMgr.GetInstanceHistory(workflowID, limit)

	c.JSON(http.StatusOK, InstanceHistoryResponse{InstanceHistory: history})
}
