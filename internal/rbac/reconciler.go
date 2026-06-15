package rbac

// Reconciler for RoleResource and RoleBindingResource.

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// RoleReconciler reconciles RoleResource objects.
type RoleReconciler struct {
	store   store.ResourceStore[*RoleResource]
	manager RBACManager
}

// NewRoleReconciler builds a reconciler.
func NewRoleReconciler(rs store.ResourceStore[*RoleResource], mgr RBACManager) *RoleReconciler {
	return &RoleReconciler{store: rs, manager: mgr}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *RoleReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*RoleResource)
	if !ok {
		return reconciler.ReconcileResult{Error: rbacErr("rbac: role reconciler received wrong type")}
	}

	now := time.Now()
	status := res.Status

	phase := "Inactive"
	if res.Spec.Active {
		phase = "Active"
		// Sync role to manager.
		if r.manager != nil {
			role := &Role{
				ID:             res.Name,
				TenantID:       res.Spec.TenantID,
				Name:           res.Name,
				Description:    res.Spec.Description,
				Type:           res.Spec.Type,
				Permissions:    res.Spec.Permissions,
				InheritedRoles: res.Spec.InheritedRoles,
				IsDefault:      res.Spec.IsDefault,
				IsActive:       true,
				Tags:           res.Spec.Tags,
			}
			_, _ = r.manager.CreateRole(role)
		}
	}

	status.RoleActive = res.Spec.Active
	status.PermissionCount = len(res.Spec.Permissions)
	status.Phase = phase
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertRBACCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: rbacBoolStatus(res.Spec.Active),
		Reason: phase, Message: "role reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

// RoleBindingReconciler reconciles RoleBindingResource objects.
type RoleBindingReconciler struct {
	store   store.ResourceStore[*RoleBindingResource]
	manager RBACManager
}

// NewRoleBindingReconciler builds a reconciler.
func NewRoleBindingReconciler(rs store.ResourceStore[*RoleBindingResource], mgr RBACManager) *RoleBindingReconciler {
	return &RoleBindingReconciler{store: rs, manager: mgr}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *RoleBindingReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*RoleBindingResource)
	if !ok {
		return reconciler.ReconcileResult{Error: rbacErr("rbac: binding reconciler received wrong type")}
	}

	now := time.Now()
	status := res.Status

	phase := "Inactive"
	if res.Spec.Active {
		phase = "Active"
	}

	status.BindingActive = res.Spec.Active
	status.Phase = phase
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now
	status.Conditions = upsertRBACCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: rbacBoolStatus(res.Spec.Active),
		Reason: phase, Message: "role binding reconciled",
		LastTransitionTime: now,
	})
	res.Status = status

	storeutil.Update(ctx, r.store, res) //nolint:errcheck
	return reconciler.ReconcileResult{}
}

func upsertRBACCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}

func rbacBoolStatus(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

type rbacErr string

func (e rbacErr) Error() string { return string(e) }
