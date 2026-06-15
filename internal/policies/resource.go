package policies

// =====================================================
// P2 resource-ification — Policy.
//
// PolicyResource is a declarative envelope around the existing
// imperative `Policy` struct.  The controller reconciles it onto the
// in-process `PolicyManager`.
// =====================================================

import (
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	PolicyKind       = "Policy"
	PolicyAPIVersion = "policies.axiomnizam.io/v1"
)

// PolicySpec mirrors the non-status fields of `Policy`.
type PolicySpec struct {
	Version     string         `json:"version,omitempty"`
	Language    PolicyLanguage `json:"language"`
	Description string         `json:"description,omitempty"`
	Rule        string         `json:"rule"`
	Condition   string         `json:"condition,omitempty"`
	Effect      string         `json:"effect"`
	Priority    int            `json:"priority,omitempty"`
	Enabled     bool           `json:"enabled"`
}

// PolicyResourceStatus extends canonical object status.
type PolicyResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	CompiledAt *time.Time `json:"compiledAt,omitempty"`
	CompileErr string     `json:"compileError,omitempty"`
}

// PolicyResource is the declarative resource for a Policy.
type PolicyResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   PolicySpec           `json:"spec"`
	Status PolicyResourceStatus `json:"status"`
}

// --- resources.Resource implementation ---

func (r *PolicyResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *PolicyResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *PolicyResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *PolicyResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *PolicyResource) DeepCopy() resources.Resource { cp := *r; return &cp }

// --- reconciler.Resource implementation ---

func (r *PolicyResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *PolicyResource) GetGeneration() int64         { return r.Generation }
func (r *PolicyResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// PolicyReconciler reconciles PolicyResource objects onto PolicyManager.
type PolicyReconciler struct {
	store   store.ResourceStore[*PolicyResource]
	manager *PolicyManager
}

// NewPolicyReconciler builds a reconciler.  `manager` may be nil.
func NewPolicyReconciler(rs store.ResourceStore[*PolicyResource], mgr *PolicyManager) *PolicyReconciler {
	return &PolicyReconciler{store: rs, manager: mgr}
}

// Reconcile implements `reconciler.Reconciler`.
func (r *PolicyReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*PolicyResource)
	if !ok {
		return reconciler.ReconcileResult{Error: policyErr("policies: reconciler received non-PolicyResource")}
	}

	now := time.Now()
	status := res.Status
	status.ObservedGeneration = res.Generation
	status.LastTransitionTime = now

	if r.manager != nil {
		p := &Policy{
			Name:        res.Name,
			Namespace:   res.Namespace,
			Version:     res.Spec.Version,
			Language:    res.Spec.Language,
			Description: res.Spec.Description,
			Rule:        res.Spec.Rule,
			Condition:   res.Spec.Condition,
			Effect:      res.Spec.Effect,
			Priority:    res.Spec.Priority,
			Enabled:     res.Spec.Enabled,
			Labels:      res.Labels,
			Annotations: res.Annotations,
		}
		if err := r.manager.AddPolicy(ctx, p); err != nil {
			status.CompileErr = err.Error()
			status.Phase = "Failed"
		} else {
			status.CompiledAt = &now
			status.CompileErr = ""
			status.Phase = "Ready"
		}
	} else {
		status.Phase = "Ready"
	}
	res.Status = status

	if r.store != nil {
		_ = r.store.Update(ctx, res)
	}
	return reconciler.ReconcileResult{}
}

type policyErr string

func (e policyErr) Error() string { return string(e) }
