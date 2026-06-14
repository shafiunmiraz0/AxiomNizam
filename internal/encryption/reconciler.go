package encryption

// Reconciler for EncryptionKeyResource and EncryptionPolicyResource.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// EncryptionKeyReconciler reconciles EncryptionKeyResource objects.
type EncryptionKeyReconciler struct {
	store   store.ResourceStore[*EncryptionKeyResource]
	manager SecretsManager
}

// NewEncryptionKeyReconciler builds a reconciler.
func NewEncryptionKeyReconciler(rs store.ResourceStore[*EncryptionKeyResource], mgr SecretsManager) *EncryptionKeyReconciler {
	return &EncryptionKeyReconciler{store: rs, manager: mgr}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *EncryptionKeyReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*EncryptionKeyResource)
	if !ok {
		return reconciler.ReconcileResult{Error: encryptionErr("encryption: key reconciler received wrong type")}
	}

	now := time.Now()
	status := res.Status

	// Handle rotation request.
	if res.Spec.RotateNow && r.manager != nil {
		rotated, err := r.manager.RotateKey(res.Name)
		if err != nil {
			status.Conditions = upsertEncryptionCondition(status.Conditions, resources.Condition{
				Type: "Rotated", Status: "False",
				Reason: "RotationFailed", Message: err.Error(),
				LastTransitionTime: now,
			})
		} else if rotated != nil {
			status.Version = rotated.Version
			status.KeyStatus = KeyStatusActive
			rotatedAt := now
			status.RotatedAt = &rotatedAt
			status.Conditions = upsertEncryptionCondition(status.Conditions, resources.Condition{
				Type: "Rotated", Status: "True",
				Reason: "RotationComplete", Message: "key rotated successfully",
				LastTransitionTime: now,
			})
		}
	}

	// Ensure key exists in manager.
	if status.KeyStatus == "" && r.manager != nil {
		key := &EncryptionKey{
			ID:             res.Name,
			TenantID:       res.Spec.TenantID,
			Name:           res.Name,
			Description:    res.Spec.Description,
			KeyType:        res.Spec.KeyType,
			Algorithm:      res.Spec.Algorithm,
			KeyLength:      res.Spec.KeyLength,
			RotationPolicy: res.Spec.RotationPolicy,
			IsDefault:      res.Spec.IsDefault,
			Owner:          res.Spec.Owner,
			ACL:            res.Spec.ACL,
			Tags:           res.Spec.Tags,
			Status:         KeyStatusActive,
		}
		if err := r.manager.CreateKey(key); err != nil {
			status.KeyStatus = KeyStatusInactive
			status.Phase = string(KeyStatusInactive)
		} else {
			status.KeyStatus = KeyStatusActive
			status.Version = 1
		}
	}

	phase := string(status.KeyStatus)
	if phase == "" {
		phase = "Pending"
	}
	status.Phase = phase
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertEncryptionCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: encryptionBoolStatus(status.KeyStatus == KeyStatusActive),
		Reason: phase, Message: "encryption key reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

// EncryptionPolicyReconciler reconciles EncryptionPolicyResource objects.
type EncryptionPolicyReconciler struct {
	store store.ResourceStore[*EncryptionPolicyResource]
}

// NewEncryptionPolicyReconciler builds a reconciler.
func NewEncryptionPolicyReconciler(rs store.ResourceStore[*EncryptionPolicyResource]) *EncryptionPolicyReconciler {
	return &EncryptionPolicyReconciler{store: rs}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *EncryptionPolicyReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*EncryptionPolicyResource)
	if !ok {
		return reconciler.ReconcileResult{Error: encryptionErr("encryption: policy reconciler received wrong type")}
	}

	now := time.Now()
	status := res.Status

	phase := "Disabled"
	if res.Spec.Enabled {
		phase = "Active"
	}

	status.PolicyActive = res.Spec.Enabled
	status.FieldCount = len(res.Spec.FieldRules)
	status.Phase = phase
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertEncryptionCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: encryptionBoolStatus(res.Spec.Enabled),
		Reason: phase, Message: "encryption policy reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

func upsertEncryptionCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}

func encryptionBoolStatus(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

type encryptionErr string

func (e encryptionErr) Error() string { return string(e) }
