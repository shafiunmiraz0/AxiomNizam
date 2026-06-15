package models

// =====================================================
// P2 resource-ification -- APIBank.
//
// This file adds a declarative envelope around the existing imperative
// `APIBank` config object so the platform can reconcile API banks like
// any other resource (ETLPipeline, CDCStream, Job, Workflow).
//
// The in-memory `APIBank` type stays untouched; `APIBankResource` is the
// Spec/Status shape that flows through the API server, store, informer,
// workqueue and reconciler.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// APIReference is a reference to an API in the bank
type APIReference struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"` // e.g., "GraphQL", "REST", "gRPC"
	Endpoint    string   `json:"endpoint"`
	Description string   `json:"description,omitempty"`
	SLA         string   `json:"sla,omitempty"`         // e.g., "99.9%"
	DataClasses []string `json:"dataClasses,omitempty"` // What data this API exposes
}

// Kind / APIVersion constants.
const (
	APIBankKind       = "APIBank"
	APIBankAPIVersion = "apibanks.axiomnizam.io/v1"
)

// APIBankSpec is the desired state of an APIBank.  It mirrors the
// existing `APIBank` struct without timestamps (those live on
// ObjectMeta / Status).
type APIBankSpec struct {
	Description string            `json:"description,omitempty"`
	Owner       string            `json:"owner,omitempty"`
	Version     string            `json:"version,omitempty"`
	APIs        []APIReference    `json:"apis,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// APIBankResourceStatus extends the canonical object status with
// bank-specific telemetry.  Controller-owned.
type APIBankResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	// APICount is the number of APIs currently registered in the bank.
	APICount int `json:"apiCount"`

	// LastSyncedAt is when the controller last reconciled the bank.
	LastSyncedAt *time.Time `json:"lastSyncedAt,omitempty"`
}

// APIBankResource is the declarative resource for an APIBank.
type APIBankResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   APIBankSpec           `json:"spec"`
	Status APIBankResourceStatus `json:"status"`
}

// --- resources.Resource implementation ---

func (r *APIBankResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *APIBankResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *APIBankResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *APIBankResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *APIBankResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.APIs) > 0 {
		cp.Spec.APIs = append([]APIReference(nil), r.Spec.APIs...)
	}
	if len(r.Spec.Tags) > 0 {
		cp.Spec.Tags = append([]string(nil), r.Spec.Tags...)
	}
	return &cp
}

// --- reconciler.Resource implementation ---

func (r *APIBankResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *APIBankResource) GetGeneration() int64         { return r.Generation }
func (r *APIBankResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
