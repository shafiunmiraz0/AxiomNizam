package apiresource

import (
	"time"

	apiv1 "axiomnizam.bitbd.net/axiomnizam/internal/resources/apiresource/v1"
)

// Migration helpers to convert between old lifecycle.go types and new v1 types

// ConvertOldAPIResourceToV1 converts the old unversioned APIResource to v1
func ConvertOldAPIResourceToV1(old *APIResource) *apiv1.APIResource {
	if old == nil {
		return nil
	}

	now := time.Now()

	// Convert metadata
	metadata := apiv1.ObjectMetadata{
		Name:        old.Metadata.Name,
		Namespace:   old.Metadata.Namespace,
		UID:         old.Metadata.UID,
		Generation:  old.Metadata.Generation,
		CreatedAt:   old.Metadata.CreatedAt,
		UpdatedAt:   old.Metadata.UpdatedAt,
		Labels:      old.Metadata.Labels,
		Annotations: make(map[string]string),
	}

	// Convert spec
	spec := apiv1.APIResourceSpec{
		BasePath:    old.Spec.BasePath,
		Title:       old.Spec.Title,
		Description: old.Spec.Description,
		Version:     old.Spec.Version,
		Tags:        old.Spec.Tags,
		Timeout:     old.Spec.Timeout,
		Data:        make(map[string]interface{}),
	}

	// Convert status
	status := apiv1.APIResourceStatus{
		Phase:              apiv1.Phase(old.Status.Phase),
		Ready:              old.Status.Ready,
		Message:            old.Status.Message,
		LastUpdateTime:     old.Status.LastUpdate,
		Conditions:         convertConditions(old.Status.Conditions),
		ObservedGeneration: old.Metadata.Generation - 1,
		ReconcileCount:     0,
		LastReconcileTime:  now,
	}

	return &apiv1.APIResource{
		APIVersion: apiv1.APIVersion,
		Kind:       apiv1.Kind,
		Metadata:   metadata,
		Spec:       spec,
		Status:     status,
	}
}

// ConvertV1ToOldAPIResource converts v1 APIResource back to the old unversioned type
func ConvertV1ToOldAPIResource(v1 *apiv1.APIResource) *APIResource {
	if v1 == nil {
		return nil
	}

	return &APIResource{
		Metadata: MetadataSpec{
			Name:       v1.Metadata.Name,
			Namespace:  v1.Metadata.Namespace,
			UID:        v1.Metadata.UID,
			Generation: v1.Metadata.Generation,
			CreatedAt:  v1.Metadata.CreatedAt,
			UpdatedAt:  v1.Metadata.UpdatedAt,
			Labels:     v1.Metadata.Labels,
		},
		Spec: SpecSection{
			BasePath:    v1.Spec.BasePath,
			Title:       v1.Spec.Title,
			Description: v1.Spec.Description,
			Version:     v1.Spec.Version,
			Tags:        v1.Spec.Tags,
			Timeout:     v1.Spec.Timeout,
		},
		Status: StatusSection{
			Phase:      string(v1.Status.Phase),
			Ready:      v1.Status.Ready,
			Message:    v1.Status.Message,
			LastUpdate: v1.Status.LastUpdateTime,
			Conditions: convertV1ConditionsToOld(v1.Status.Conditions),
		},
	}
}

// convertConditions converts old Condition to v1 Condition
func convertConditions(oldConds []Condition) []apiv1.Condition {
	if len(oldConds) == 0 {
		return []apiv1.Condition{}
	}

	v1Conds := make([]apiv1.Condition, len(oldConds))
	for i, oldCond := range oldConds {
		v1Conds[i] = apiv1.Condition{
			Type:               oldCond.Type,
			Status:             apiv1.ConditionStatus(oldCond.Status),
			Reason:             oldCond.Type,
			Message:            oldCond.Message,
			FirstObservedTime:  time.Now(),
			LastTransitionTime: time.Now(),
		}
	}

	return v1Conds
}

// convertConditionStatus converts string status to v1 ConditionStatus
func convertConditionStatus(status string) apiv1.ConditionStatus {
	if status == "True" {
		return apiv1.ConditionStatusTrue
	} else if status == "False" {
		return apiv1.ConditionStatusFalse
	}
	return apiv1.ConditionStatusUnknown
}

// convertV1ConditionsToOld converts v1 Condition back to old Condition
func convertV1ConditionsToOld(v1Conds []apiv1.Condition) []Condition {
	if len(v1Conds) == 0 {
		return []Condition{}
	}

	oldConds := make([]Condition, len(v1Conds))
	for i, v1Cond := range v1Conds {
		oldConds[i] = Condition{
			Type:    v1Cond.Type,
			Status:  ConditionStatus(v1Cond.Status),
			Message: v1Cond.Message,
		}
	}

	return oldConds
}

// NOTE: Deprecation path
// The old lifecycle.go types (APIResource, MetadataSpec, SpecSection, StatusSection, Condition)
// are maintained for backward compatibility during migration.
// New code should use internal/resources/apiresource/v1 types directly.
// The ConvertOldAPIResourceToV1 and ConvertV1ToOldAPIResource functions
// provide a migration path for existing code.
