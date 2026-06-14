package schemaregistry

// SchemaReconciler validates schema compatibility on registration.
//
// Behavior:
//   1. Observe: Read SchemaResource from etcd
//   2. Diff: Check if compatibility validation has been performed
//   3. Act: Fetch previous version, run compatibility check
//   4. Update Status: Mark as compatible/incompatible, assign schemaId

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/storeutil"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"

	"go.uber.org/zap"
)

// Global schema ID counter (in production, this would be persisted in etcd).
var globalSchemaIDCounter int64

// SchemaReconciler reconciles schema resources.
type SchemaReconciler struct {
	schemaStore  store.ResourceStore[*SchemaResource]
	subjectStore store.ResourceStore[*SchemaSubjectResource]
	checker      CompatibilityChecker
}

// CompatibilityChecker validates schema compatibility.
type CompatibilityChecker interface {
	// CheckCompatibility validates a new schema against a previous version.
	// Returns nil if compatible, or a list of incompatibility reasons.
	CheckCompatibility(newSchema, oldSchema string, schemaType SchemaType, mode CompatibilityMode) []string
}

// NewSchemaReconciler creates a new reconciler.
func NewSchemaReconciler(
	schemaStore store.ResourceStore[*SchemaResource],
	subjectStore store.ResourceStore[*SchemaSubjectResource],
	checker CompatibilityChecker,
) *SchemaReconciler {
	return &SchemaReconciler{
		schemaStore:  schemaStore,
		subjectStore: subjectStore,
		checker:      checker,
	}
}

// Reconcile implements reconciler.Reconciler.
func (r *SchemaReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	schema, ok := obj.(*SchemaResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("schema: reconciler received non-SchemaResource")}
	}
	logging.Z().Debug("reconciling resource", zap.String("name", schema.GetKey()), zap.String("kind", schema.GetTypeMeta().Kind))

	now := time.Now()
	status := schema.Status

	// Skip if already processed (observedGeneration matches).
	if status.ObservedGeneration >= schema.Generation && status.SchemaID > 0 {
		return reconciler.ReconcileResult{}
	}

	// Compute fingerprint.
	fingerprint := computeFingerprint(schema.Spec.Schema)
	status.Fingerprint = fingerprint

	// Determine compatibility mode.
	compatMode := schema.Spec.Compatibility
	if compatMode == "" {
		// Look up subject default.
		compatMode = r.getSubjectCompatibility(ctx, schema.Spec.Subject)
	}
	if compatMode == "" {
		compatMode = CompatBackward // Platform default
	}

	// Find previous version for this subject.
	previousSchema := r.findPreviousVersion(ctx, schema.Spec.Subject, schema.Name)

	// Run compatibility check.
	if previousSchema != nil && compatMode != CompatNone && r.checker != nil {
		errors := r.checker.CheckCompatibility(
			schema.Spec.Schema,
			previousSchema.Spec.Schema,
			schema.Spec.SchemaType,
			compatMode,
		)

		if len(errors) > 0 {
			// Incompatible — reject.
			status.IsCompatible = false
			status.CompatibilityErrors = errors
			status.Phase = "Incompatible"
			status.Conditions = upsertCondition(status.Conditions, resources.Condition{
				Type: "Compatible", Status: "False",
				Reason: "IncompatibleSchema",
				Message: fmt.Sprintf("schema is not %s compatible: %d violations", compatMode, len(errors)),
				LastTransitionTime: now,
			})
			status.Conditions = upsertCondition(status.Conditions, resources.Condition{
				Type: "Ready", Status: "False",
				Reason: "CompatibilityCheckFailed",
				Message: errors[0], // First error as summary
				LastTransitionTime: now,
			})
			status.ObservedGeneration = schema.Generation
			status.LastTransitionTime = now
			schema.Status = status
			storeutil.Update(ctx, r.schemaStore, schema) //nolint:errcheck
			return reconciler.ReconcileResult{}
		}
	}

	// Compatible — assign schema ID and version.
	status.IsCompatible = true
	status.CompatibilityErrors = nil

	if status.SchemaID == 0 {
		status.SchemaID = atomic.AddInt64(&globalSchemaIDCounter, 1)
	}

	// Determine version number.
	if status.Version == 0 {
		status.Version = r.getNextVersion(ctx, schema.Spec.Subject)
	}

	status.IsLatest = true
	status.RegisteredAt = &now
	status.Phase = "Registered"
	status.FieldCount = countFields(schema.Spec.Schema, schema.Spec.SchemaType)

	// Mark compatible with previous version.
	if previousSchema != nil {
		status.CompatibleWith = append(status.CompatibleWith, previousSchema.Status.Version)
		// Unmark previous as latest.
		previousSchema.Status.IsLatest = false
		storeutil.Update(ctx, r.schemaStore, previousSchema) //nolint:errcheck
	}

	status.Conditions = upsertCondition(status.Conditions, resources.Condition{
		Type: "Compatible", Status: "True",
		Reason: "CompatibilityCheckPassed",
		Message: fmt.Sprintf("schema is %s compatible (version %d)", compatMode, status.Version),
		LastTransitionTime: now,
	})
	status.Conditions = upsertCondition(status.Conditions, resources.Condition{
		Type: "Ready", Status: "True",
		Reason: "Registered",
		Message: fmt.Sprintf("schema registered as version %d with ID %d", status.Version, status.SchemaID),
		LastTransitionTime: now,
	})

	status.ObservedGeneration = schema.Generation
	status.LastTransitionTime = now
	schema.Status = status

	if r.schemaStore != nil {
		storeutil.Update(ctx, r.schemaStore, schema) //nolint:errcheck
	}

	// Update subject status.
	r.updateSubjectStatus(ctx, schema)

	return reconciler.ReconcileResult{}
}

// getSubjectCompatibility looks up the default compatibility mode for a subject.
func (r *SchemaReconciler) getSubjectCompatibility(ctx context.Context, subject string) CompatibilityMode {
	if r.subjectStore == nil {
		return ""
	}
	subj, err := r.subjectStore.Get(ctx, subject)
	if err != nil {
		return ""
	}
	return subj.Spec.Compatibility
}

// findPreviousVersion finds the most recent schema for a subject (excluding current).
func (r *SchemaReconciler) findPreviousVersion(ctx context.Context, subject, currentName string) *SchemaResource {
	if r.schemaStore == nil {
		return nil
	}
	schemas, err := r.schemaStore.List(ctx, "")
	if err != nil {
		return nil
	}

	var latest *SchemaResource
	var latestVersion int

	for _, s := range schemas {
		if s.Spec.Subject == subject && s.Name != currentName && s.Status.Version > latestVersion {
			latest = s
			latestVersion = s.Status.Version
		}
	}

	return latest
}

// getNextVersion determines the next version number for a subject.
func (r *SchemaReconciler) getNextVersion(ctx context.Context, subject string) int {
	if r.schemaStore == nil {
		return 1
	}
	schemas, err := r.schemaStore.List(ctx, "")
	if err != nil {
		return 1
	}

	maxVersion := 0
	for _, s := range schemas {
		if s.Spec.Subject == subject && s.Status.Version > maxVersion {
			maxVersion = s.Status.Version
		}
	}

	return maxVersion + 1
}

// updateSubjectStatus updates the subject resource with latest schema info.
func (r *SchemaReconciler) updateSubjectStatus(ctx context.Context, schema *SchemaResource) {
	if r.subjectStore == nil {
		return
	}
	subj, err := r.subjectStore.Get(ctx, schema.Spec.Subject)
	if err != nil {
		return
	}

	now := time.Now()
	subj.Status.LatestVersion = schema.Status.Version
	subj.Status.LatestSchemaID = schema.Status.SchemaID
	subj.Status.LastRegisteredAt = &now
	subj.Status.VersionCount++
	subj.Status.LastTransitionTime = now

	storeutil.Update(ctx, r.subjectStore, subj) //nolint:errcheck
}

// computeFingerprint generates a SHA-256 hash of the schema content.
func computeFingerprint(schema string) string {
	hash := sha256.Sum256([]byte(schema))
	return hex.EncodeToString(hash[:])
}

// countFields estimates the number of fields in a schema.
// This is a simplified implementation — production would parse the actual schema.
func countFields(schema string, schemaType SchemaType) int {
	// Simple heuristic: count "name" occurrences for Avro, "properties" keys for JSON.
	// In production, this would use proper schema parsing.
	count := 0
	for i := 0; i < len(schema)-4; i++ {
		if schema[i:i+4] == "name" || schema[i:i+4] == "type" {
			count++
		}
	}
	return count / 2 // Rough estimate (each field has name + type)
}

// upsertCondition adds or updates a condition.
func upsertCondition(conditions []resources.Condition, cond resources.Condition) []resources.Condition {
	for i, existing := range conditions {
		if existing.Type == cond.Type {
			conditions[i] = cond
			return conditions
		}
	}
	return append(conditions, cond)
}
