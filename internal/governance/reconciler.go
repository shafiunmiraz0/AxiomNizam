package governance

// =====================================================
// WS-6.1 — Compliance Policy Reconciler
//
// Audits catalog assets against compliance policy rules and
// records violations. Supports GDPR, HIPAA, SOC2, PCI-DSS.
//
// Behavior:
//   1. Observe: Read CompliancePolicyResource from etcd
//   2. Diff: Check if audit is due (based on reviewSchedule)
//   3. Act: Scan catalog assets matching scope, evaluate each rule
//   4. Update Status: Record violations, compliance score, audit timestamp
// =====================================================

import (
	"context"
	"fmt"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/governance/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/resilience"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"

	"go.uber.org/zap"
)

// AssetAuditor abstracts looking up catalog assets for compliance auditing.
type AssetAuditor interface {
	// ListAssetsInScope returns catalog assets matching the policy scope.
	ListAssetsInScope(ctx context.Context, scope models.PolicyScope) ([]AuditableAsset, error)
}

// AuditableAsset represents a catalog asset with metadata needed for compliance checks.
type AuditableAsset struct {
	Name            string   `json:"name"`
	Domain          string   `json:"domain"`
	Classification  string   `json:"classification"`
	Tags            []string `json:"tags"`
	DataSourceRef   string   `json:"dataSourceRef"`
	HasPII          bool     `json:"hasPII"`
	EncryptedAtRest bool     `json:"encryptedAtRest"`
	HasAuditLog     bool     `json:"hasAuditLog"`
	RetentionDays   int      `json:"retentionDays"`
	AccessRoles     []string `json:"accessRoles"`
}

// CompliancePolicyReconciler reconciles compliance policies.
type CompliancePolicyReconciler struct {
	store   store.ResourceStore[*models.CompliancePolicyResource]
	auditor AssetAuditor
}

// NewCompliancePolicyReconciler creates a new reconciler.
func NewCompliancePolicyReconciler(
	s store.ResourceStore[*models.CompliancePolicyResource],
	auditor AssetAuditor,
) *CompliancePolicyReconciler {
	return &CompliancePolicyReconciler{store: s, auditor: auditor}
}

// Reconcile implements reconciler.Reconciler.
func (r *CompliancePolicyReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	policy, ok := obj.(*models.CompliancePolicyResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("governance: reconciler received non-CompliancePolicyResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", policy.GetKey()), zap.String("kind", policy.GetTypeMeta().Kind))

	now := time.Now()
	status := policy.Status

	if !policy.Spec.Enabled {
		status.Phase = "Disabled"
		status.ObservedGeneration = policy.Generation
		status.LastTransitionTime = now
		policy.Status = status
		storeutil.Update(ctx, r.store, policy) //nolint:errcheck
		return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 1 * time.Hour}
	}

	// Check if audit is due.
	if status.ObservedGeneration >= policy.Generation && status.LastAuditAt != nil {
		auditInterval := 24 * time.Hour // Default: daily
		if policy.Spec.ReviewSchedule != "" {
			if d, err := time.ParseDuration(policy.Spec.ReviewSchedule); err == nil {
				auditInterval = d
			}
		}
		if now.Sub(*status.LastAuditAt) < auditInterval {
			return reconciler.ReconcileResult{Requeue: true, RequeueAfter: auditInterval - now.Sub(*status.LastAuditAt)}
		}
	}

	// Get assets in scope.
	var assets []AuditableAsset
	if r.auditor != nil {
		var err error
		assets, err = r.auditor.ListAssetsInScope(ctx, policy.Spec.Scope)
		if err != nil {
			status.Phase = "Error"
			status.Conditions = upsertCondition(status.Conditions, resources.Condition{
				Type: "Ready", Status: "False",
				Reason: "AuditFailed", Message: fmt.Sprintf("failed to list assets: %v", err),
				LastTransitionTime: now,
			})
			status.ObservedGeneration = policy.Generation
			status.LastTransitionTime = now
			policy.Status = status
			storeutil.Update(ctx, r.store, policy) //nolint:errcheck
			logging.Z().Warn("reconciliation error", zap.String("name", policy.GetKey()), zap.Error(err))
			return reconciler.ReconcileResult{Requeue: true, RequeueAfter: resilience.ReconcileBackoff(1)}
		}
	}

	// Audit each asset against each rule.
	var violations []models.ComplianceViolation
	compliantCount := 0
	nonCompliantCount := 0

	for _, asset := range assets {
		assetViolations := r.auditAsset(policy, asset, now)
		if len(assetViolations) == 0 {
			compliantCount++
		} else {
			nonCompliantCount++
			violations = append(violations, assetViolations...)
		}
	}

	// Update status.
	status.Violations = violations
	status.Compliant = len(violations) == 0
	status.AssetsAudited = len(assets)
	status.AssetsCompliant = compliantCount
	status.AssetsNonCompliant = nonCompliantCount
	status.LastAuditAt = &now

	if len(assets) > 0 {
		status.ComplianceScore = float64(compliantCount) / float64(len(assets)) * 100.0
	} else {
		status.ComplianceScore = 100.0
	}

	if status.Compliant {
		status.Phase = "Compliant"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Compliant", Status: "True",
			Reason: "AllAssetsCompliant",
			Message: fmt.Sprintf("all %d assets comply with %s policy", len(assets), policy.Spec.Framework),
			LastTransitionTime: now,
		})
	} else {
		status.Phase = "NonCompliant"
		status.Conditions = upsertCondition(status.Conditions, resources.Condition{
			Type: "Compliant", Status: "False",
			Reason: "ViolationsDetected",
			Message: fmt.Sprintf("%d violations across %d non-compliant assets", len(violations), nonCompliantCount),
			LastTransitionTime: now,
		})
	}

	status.Conditions = upsertCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: "True",
		Reason: "AuditComplete", Message: fmt.Sprintf("audited %d assets", len(assets)),
		LastTransitionTime: now,
	})

	status.ObservedGeneration = policy.Generation
	status.LastTransitionTime = now
	policy.Status = status

	storeutil.Update(ctx, r.store, policy) //nolint:errcheck

	return reconciler.ReconcileResult{Requeue: true, RequeueAfter: 24 * time.Hour}
}

// auditAsset checks a single asset against all policy rules.
func (r *CompliancePolicyReconciler) auditAsset(policy *models.CompliancePolicyResource, asset AuditableAsset, now time.Time) []models.ComplianceViolation {
	var violations []models.ComplianceViolation

	for _, rule := range policy.Spec.Rules {
		switch rule.Type {
		case "encryption":
			if rule.Encryption != nil && rule.Encryption.RequireAtRest && !asset.EncryptedAtRest {
				violations = append(violations, models.ComplianceViolation{
					RuleID: rule.ID, RuleName: rule.Name, AssetRef: asset.Name,
					Severity: "critical", DetectedAt: now,
					Message:     fmt.Sprintf("asset '%s' is not encrypted at rest", asset.Name),
					Remediation: "Enable encryption for this datasource",
				})
			}

		case "retention":
			if rule.Retention != nil && rule.Retention.MaxRetentionDays > 0 {
				if asset.RetentionDays > rule.Retention.MaxRetentionDays || asset.RetentionDays == 0 {
					violations = append(violations, models.ComplianceViolation{
						RuleID: rule.ID, RuleName: rule.Name, AssetRef: asset.Name,
						Severity: "warning", DetectedAt: now,
						Message:     fmt.Sprintf("asset '%s' retention (%d days) exceeds maximum (%d days)", asset.Name, asset.RetentionDays, rule.Retention.MaxRetentionDays),
						Remediation: "Configure a retention policy for this asset",
					})
				}
			}

		case "masking":
			if rule.Masking != nil && asset.HasPII {
				// PII data should have masking configured.
				violations = append(violations, models.ComplianceViolation{
					RuleID: rule.ID, RuleName: rule.Name, AssetRef: asset.Name,
					Severity: "warning", DetectedAt: now,
					Message:     fmt.Sprintf("asset '%s' contains PII but no masking policy is applied", asset.Name),
					Remediation: "Apply a data masking policy to PII columns",
				})
			}

		case "audit":
			if !asset.HasAuditLog {
				violations = append(violations, models.ComplianceViolation{
					RuleID: rule.ID, RuleName: rule.Name, AssetRef: asset.Name,
					Severity: "warning", DetectedAt: now,
					Message:     fmt.Sprintf("asset '%s' does not have audit logging enabled", asset.Name),
					Remediation: "Enable audit logging for this datasource",
				})
			}

		case "access":
			if rule.Access != nil && rule.Access.RequireApproval {
				// Check if any forbidden roles have access.
				for _, forbidden := range rule.Access.ForbiddenRoles {
					for _, role := range asset.AccessRoles {
						if strings.EqualFold(role, forbidden) {
							violations = append(violations, models.ComplianceViolation{
								RuleID: rule.ID, RuleName: rule.Name, AssetRef: asset.Name,
								Severity: "critical", DetectedAt: now,
								Message:     fmt.Sprintf("asset '%s' is accessible by forbidden role '%s'", asset.Name, forbidden),
								Remediation: fmt.Sprintf("Remove role '%s' from asset access list", forbidden),
							})
						}
					}
				}
			}
		}
	}

	return violations
}

// upsertCondition adds or updates a condition in the conditions slice.
func upsertCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}
