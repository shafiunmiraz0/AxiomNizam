package integration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apibanks"
	"axiomnizam.bitbd.net/axiomnizam/internal/events"
	"axiomnizam.bitbd.net/axiomnizam/internal/mesh"
	"axiomnizam.bitbd.net/axiomnizam/internal/policies"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ComplianceAuditor tracks compliance for data operations
type ComplianceAuditor struct {
	mu            sync.RWMutex
	auditLog      []AuditRecord
	maxRecords    int
	policyManager *policies.PolicyManager
	eventRecorder events.EventRecorder
	etcd          *clientv3.Client
	stateKey      string
}

type complianceAuditorState struct {
	AuditLog   []AuditRecord `json:"auditLog"`
	MaxRecords int           `json:"maxRecords"`
}

// AuditRecord represents a compliance audit record
type AuditRecord struct {
	Timestamp    time.Time              `json:"timestamp"`
	Operation    string                 `json:"operation"`
	User         string                 `json:"user"`
	Resource     string                 `json:"resource"`
	ResourceType string                 `json:"resourceType"`
	Action       string                 `json:"action"`
	Status       string                 `json:"status"`
	Details      map[string]interface{} `json:"details"`
	Policies     []string               `json:"policies"`
}

// NewComplianceAuditor creates a new compliance auditor
func NewComplianceAuditor(maxRecords int) *ComplianceAuditor {
	ca := &ComplianceAuditor{
		auditLog:      make([]AuditRecord, 0, maxRecords),
		maxRecords:    maxRecords,
		policyManager: policies.GlobalPolicyManager,
		eventRecorder: events.GlobalEventRecorder,
		etcd:          integrationEtcdClient(),
		stateKey:      "integration:compliance:state",
	}
	ca.loadState()
	return ca
}

func (ca *ComplianceAuditor) ConfigurePersistence(etcd *clientv3.Client) {
	ca.mu.Lock()
	ca.etcd = etcd
	if ca.stateKey == "" {
		ca.stateKey = "integration:compliance:state"
	}
	ca.mu.Unlock()
	ca.loadState()
}

func (ca *ComplianceAuditor) loadState() {
	if ca.etcd == nil {
		return
	}

	var state complianceAuditorState
	if !loadStateFromEtcd(ca.etcd, ca.stateKey, &state) {
		return
	}

	ca.mu.Lock()
	defer ca.mu.Unlock()
	if state.AuditLog != nil {
		ca.auditLog = state.AuditLog
	}
	if state.MaxRecords > 0 {
		ca.maxRecords = state.MaxRecords
	}
}

func (ca *ComplianceAuditor) persistStateLocked() {
	if ca.etcd == nil {
		return
	}

	saveStateToEtcd(ca.etcd, ca.stateKey, complianceAuditorState{
		AuditLog:   ca.auditLog,
		MaxRecords: ca.maxRecords,
	})
}

// RecordOperation records an operation in the audit log
func (ca *ComplianceAuditor) RecordOperation(ctx context.Context, op Operation) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	// Evaluate policies
	evaluatedPolicies := make([]string, 0)
	if ca.policyManager != nil {
		results, allowed, err := ca.policyManager.EvaluateAll(ctx, map[string]interface{}{
			"operation": op.Operation,
			"resource":  op.Resource,
			"user":      op.User,
		})

		if err == nil && results != nil {
			for _, result := range results {
				evaluatedPolicies = append(evaluatedPolicies, result.PolicyName)
			}
		}

		if !allowed {
			op.Status = "denied"
		}
	}

	record := AuditRecord{
		Timestamp:    time.Now(),
		Operation:    op.Operation,
		User:         op.User,
		Resource:     op.Resource,
		ResourceType: op.ResourceType,
		Action:       op.Action,
		Status:       op.Status,
		Details:      op.Details,
		Policies:     evaluatedPolicies,
	}

	ca.auditLog = append(ca.auditLog, record)

	// Keep only recent records
	if len(ca.auditLog) > ca.maxRecords {
		ca.auditLog = ca.auditLog[len(ca.auditLog)-ca.maxRecords:]
	}
	ca.persistStateLocked()

	// Record event - skip for now due to interface complexity
	// if ca.eventRecorder != nil {
	//	ca.eventRecorder.Record(ctx, ...)
	// }

	return nil
}

// Operation describes an operation to audit
type Operation struct {
	Operation    string
	User         string
	Resource     string
	ResourceType string
	Action       string
	Status       string
	Details      map[string]interface{}
}

// GetAuditLog returns audit records matching criteria
func (ca *ComplianceAuditor) GetAuditLog(filter AuditFilter) []AuditRecord {
	ca.mu.RLock()
	defer ca.mu.RUnlock()

	result := make([]AuditRecord, 0)

	for _, record := range ca.auditLog {
		if matches(record, filter) {
			result = append(result, record)
		}
	}

	return result
}

// AuditFilter filters audit records
type AuditFilter struct {
	User         string
	ResourceType string
	Operation    string
	Status       string
	Since        time.Time
	Until        time.Time
}

// matches checks if a record matches the filter
func matches(record AuditRecord, filter AuditFilter) bool {
	if filter.User != "" && record.User != filter.User {
		return false
	}
	if filter.ResourceType != "" && record.ResourceType != filter.ResourceType {
		return false
	}
	if filter.Operation != "" && record.Operation != filter.Operation {
		return false
	}
	if filter.Status != "" && record.Status != filter.Status {
		return false
	}
	if !filter.Since.IsZero() && record.Timestamp.Before(filter.Since) {
		return false
	}
	if !filter.Until.IsZero() && record.Timestamp.After(filter.Until) {
		return false
	}

	return true
}

// ComplianceReport generates compliance report
type ComplianceReport struct {
	GeneratedAt      time.Time              `json:"generatedAt"`
	TotalOperations  int                    `json:"totalOperations"`
	SuccessfulOps    int                    `json:"successfulOps"`
	DeniedOps        int                    `json:"deniedOps"`
	FailedOps        int                    `json:"failedOps"`
	OperationsByType map[string]int         `json:"operationsByType"`
	OperationsByUser map[string]int         `json:"operationsByUser"`
	DeniedByPolicy   map[string]int         `json:"deniedByPolicy"`
	RiskAssessment   map[string]interface{} `json:"riskAssessment"`
}

// GenerateReport generates a compliance report
func (ca *ComplianceAuditor) GenerateReport(filter AuditFilter) *ComplianceReport {
	records := ca.GetAuditLog(filter)

	report := &ComplianceReport{
		GeneratedAt:      time.Now(),
		TotalOperations:  len(records),
		OperationsByType: make(map[string]int),
		OperationsByUser: make(map[string]int),
		DeniedByPolicy:   make(map[string]int),
	}

	for _, record := range records {
		switch record.Status {
		case "success":
			report.SuccessfulOps++
		case "denied":
			report.DeniedOps++
		case "failed":
			report.FailedOps++
		}

		report.OperationsByType[record.Operation]++
		report.OperationsByUser[record.User]++

		for _, policy := range record.Policies {
			if record.Status == "denied" {
				report.DeniedByPolicy[policy]++
			}
		}
	}

	// Risk assessment
	report.RiskAssessment = map[string]interface{}{
		"denialRate":  float64(report.DeniedOps) / float64(report.TotalOperations),
		"failureRate": float64(report.FailedOps) / float64(report.TotalOperations),
		"topRiskUser": getTopUser(report.OperationsByUser),
		"riskLevel":   calculateRiskLevel(report),
	}

	return report
}

// getTopUser returns the user with most operations
func getTopUser(ops map[string]int) string {
	max := 0
	top := ""
	for user, count := range ops {
		if count > max {
			max = count
			top = user
		}
	}
	return top
}

// calculateRiskLevel calculates overall risk
func calculateRiskLevel(report *ComplianceReport) string {
	if report.TotalOperations == 0 {
		return "low"
	}

	denialRate := float64(report.DeniedOps) / float64(report.TotalOperations)
	failureRate := float64(report.FailedOps) / float64(report.TotalOperations)

	if denialRate > 0.3 || failureRate > 0.2 {
		return "high"
	}
	if denialRate > 0.1 || failureRate > 0.05 {
		return "medium"
	}

	return "low"
}

// DataAccessControl manages access to data through policies
type DataAccessControl struct {
	mu          sync.RWMutex
	auditor     *ComplianceAuditor
	dataMesh    *mesh.DataMesh
	bankManager *apibanks.APIBankManager
}

// NewDataAccessControl creates access control
func NewDataAccessControl(auditor *ComplianceAuditor) *DataAccessControl {
	return &DataAccessControl{
		auditor:     auditor,
		dataMesh:    mesh.GlobalDataMesh,
		bankManager: apibanks.GlobalAPIBankManager,
	}
}

// CanAccessProduct checks if user can access a data product
func (dac *DataAccessControl) CanAccessProduct(ctx context.Context, user, domainName, productName string) bool {
	product := dac.dataMesh.GetDataProduct(domainName, productName)
	if product == nil {
		return false
	}

	// Check if user is authorized (simplified - would use RBAC in real implementation)
	if product.Owner == user {
		return true
	}

	// Check if user has active subscription
	for _, sub := range product.Subscriptions {
		if sub.SubscriberID == user {
			return true
		}
	}

	return false
}

// CanAccessAPI checks if user can access an API from bank
func (dac *DataAccessControl) CanAccessAPI(ctx context.Context, user, bankName, apiName string) bool {
	bank := dac.bankManager.GetBank(bankName)
	if bank == nil {
		return false
	}

	// Check if user is authorized
	if bank.Owner == user {
		return true
	}

	// In real implementation, check subscription or grant
	return false
}

// RecordDataAccess records access to a data product
func (dac *DataAccessControl) RecordDataAccess(ctx context.Context, user, domainName, productName string) error {
	return dac.auditor.RecordOperation(ctx, Operation{
		Operation:    "DataAccess",
		User:         user,
		Resource:     fmt.Sprintf("%s/%s", domainName, productName),
		ResourceType: "DataProduct",
		Action:       "read",
		Status:       "success",
		Details: map[string]interface{}{
			"domain":  domainName,
			"product": productName,
		},
	})
}

// RecordDataModification records modification to data
func (dac *DataAccessControl) RecordDataModification(ctx context.Context, user, domainName, productName string, changes map[string]interface{}) error {
	return dac.auditor.RecordOperation(ctx, Operation{
		Operation:    "DataModification",
		User:         user,
		Resource:     fmt.Sprintf("%s/%s", domainName, productName),
		ResourceType: "DataProduct",
		Action:       "modify",
		Status:       "success",
		Details:      changes,
	})
}

// Phase 13: Global singletons removed. Use constructors to create instances:
//   - NewComplianceAuditor(maxEvents)
//   - NewDataAccessControl(auditor)
