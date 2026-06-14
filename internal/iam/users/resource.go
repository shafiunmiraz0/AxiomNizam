// Package users defines the declarative User resource on the platform
// control plane.
//
// Phase 3/4: platform users used to live exclusively inside
// `handlers.PlatformUserHandler`, where a raw `*clientv3.Client`, a
// `sync.RWMutex` and an in-memory map drove both HTTP responses and
// persistence.  `UserResource` moves that into a proper
// Spec/Status/Generation/ObservedGeneration resource that a controller
// can reconcile on the shared workqueue.
package users

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	UserKind       = "User"
	UserAPIVersion = "iam.axiomnizam.io/v1"
)

// UserSpec is the *desired* state of a platform user.  Password is
// intentionally the bcrypt hash — reconcilers never see plaintext.
type UserSpec struct {
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	Suspended    bool   `json:"suspended,omitempty"`
}

// UserResourceStatus extends the canonical ObjectStatus with user
// lifecycle telemetry.
type UserResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	LastLoginAt string `json:"lastLoginAt,omitempty"`
	LoginCount  int64  `json:"loginCount,omitempty"`
}

// UserResource is the declarative resource for a platform user.
type UserResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   UserSpec           `json:"spec"`
	Status UserResourceStatus `json:"status"`
}

// --- resources.Resource ---

func (u *UserResource) GetObjectMeta() *resources.ObjectMeta { return &u.ObjectMeta }
func (u *UserResource) GetTypeMeta() *resources.TypeMeta     { return &u.TypeMeta }
func (u *UserResource) GetStatus() *resources.ObjectStatus   { return &u.Status.ObjectStatus }
func (u *UserResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		u.Status.ObjectStatus = *s
	}
}
func (u *UserResource) DeepCopy() resources.Resource {
	cp := *u
	return &cp
}

// --- reconciler.Resource ---

func (u *UserResource) GetKey() string {
	if u.Namespace == "" {
		return u.Name
	}
	return u.Namespace + "/" + u.Name
}
func (u *UserResource) GetGeneration() int64         { return u.Generation }
func (u *UserResource) GetObservedGeneration() int64 { return u.Status.ObservedGeneration }
