package models

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

// =====================================================
// WS-7.4 — ML Pipeline Orchestration as declarative resources
// =====================================================

const (
	MLPipelineKind       = "MLPipeline"
	MLPipelineAPIVersion = "ml.axiomnizam.io/v1"

	ModelDeploymentKind       = "ModelDeployment"
	ModelDeploymentAPIVersion = "ml.axiomnizam.io/v1"
)

// --- ML Step ---

type MLStep struct {
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Config    map[string]interface{} `json:"config"`
	DependsOn []string               `json:"dependsOn,omitempty"`
	Timeout   string                 `json:"timeout,omitempty"`
}

// --- MLPipelineSpec ---

type MLPipelineSpec struct {
	DisplayName   string   `json:"displayName"`
	Description   string   `json:"description,omitempty"`
	Steps         []MLStep `json:"steps"`
	Schedule      string   `json:"schedule,omitempty"`
	FeatureGroups []string `json:"featureGroups,omitempty"`
	ModelRegistry string   `json:"modelRegistry,omitempty"`
	Notifications []string `json:"notifications,omitempty"`
	Enabled       bool     `json:"enabled"`
}

// --- Step Status ---

type StepStatus struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	StartedAt *time.Time        `json:"startedAt,omitempty"`
	EndedAt   *time.Time        `json:"endedAt,omitempty"`
	Duration  string            `json:"duration,omitempty"`
	Error     string            `json:"error,omitempty"`
	Output    map[string]string `json:"output,omitempty"`
}

// --- MLPipelineResourceStatus ---

type MLPipelineResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	PipelineStatus  string       `json:"pipelineStatus"`
	CurrentStep     string       `json:"currentStep,omitempty"`
	StepStatuses    []StepStatus `json:"stepStatuses,omitempty"`
	RunCount        int64        `json:"runCount"`
	LastRunAt       *time.Time   `json:"lastRunAt,omitempty"`
	LastRunDuration string       `json:"lastRunDuration,omitempty"`
	LastModelRef    string       `json:"lastModelRef,omitempty"`
}

// --- MLPipelineResource ---

type MLPipelineResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   MLPipelineSpec           `json:"spec"`
	Status MLPipelineResourceStatus `json:"status"`
}

func (r *MLPipelineResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *MLPipelineResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *MLPipelineResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *MLPipelineResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *MLPipelineResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.Steps) > 0 {
		cp.Spec.Steps = make([]MLStep, len(r.Spec.Steps))
		copy(cp.Spec.Steps, r.Spec.Steps)
	}
	return &cp
}
func (r *MLPipelineResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *MLPipelineResource) GetGeneration() int64         { return r.Generation }
func (r *MLPipelineResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// =====================================================
// ModelDeploymentResource
// =====================================================

type AutoScaleConfig struct {
	MinReplicas     int     `json:"minReplicas"`
	MaxReplicas     int     `json:"maxReplicas"`
	TargetCPU       float64 `json:"targetCpu,omitempty"`
	TargetMemory    float64 `json:"targetMemory,omitempty"`
	TargetLatencyMs int64   `json:"targetLatencyMs,omitempty"`
}

type CanaryConfig struct {
	Steps            []int   `json:"steps"`
	StepDuration     string  `json:"stepDuration"`
	SuccessMetric    string  `json:"successMetric"`
	SuccessThreshold float64 `json:"successThreshold"`
	RollbackOnFail   bool    `json:"rollbackOnFail"`
}

type ModelDeploymentSpec struct {
	ModelRef     string           `json:"modelRef"`
	Version      string           `json:"version"`
	Endpoint     string           `json:"endpoint"`
	Replicas     int              `json:"replicas"`
	TrafficSplit map[string]int   `json:"trafficSplit,omitempty"`
	AutoScale    *AutoScaleConfig `json:"autoScale,omitempty"`
	Canary       *CanaryConfig    `json:"canary,omitempty"`
	Enabled      bool             `json:"enabled"`
}

type ModelDeploymentResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	DeploymentStatus string     `json:"deploymentStatus"`
	ActiveVersion    string     `json:"activeVersion"`
	ReadyReplicas    int        `json:"readyReplicas"`
	TotalRequests    int64      `json:"totalRequests"`
	AvgLatencyMs     float64    `json:"avgLatencyMs"`
	ErrorRate        float64    `json:"errorRate"`
	DeployedAt       *time.Time `json:"deployedAt,omitempty"`
	CanaryProgress   int        `json:"canaryProgress,omitempty"`
}

type ModelDeploymentResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   ModelDeploymentSpec           `json:"spec"`
	Status ModelDeploymentResourceStatus `json:"status"`
}

func (r *ModelDeploymentResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *ModelDeploymentResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *ModelDeploymentResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *ModelDeploymentResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *ModelDeploymentResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.TrafficSplit) > 0 {
		cp.Spec.TrafficSplit = make(map[string]int, len(r.Spec.TrafficSplit))
		for k, v := range r.Spec.TrafficSplit {
			cp.Spec.TrafficSplit[k] = v
		}
	}
	return &cp
}
func (r *ModelDeploymentResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *ModelDeploymentResource) GetGeneration() int64         { return r.Generation }
func (r *ModelDeploymentResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
