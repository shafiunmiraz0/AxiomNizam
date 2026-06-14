package controller

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/repositories"
	"github.com/google/uuid"
)

type reconcileRequest struct {
	userID   string
	factorID uuid.UUID
	reason   string
}

func (r reconcileRequest) key() string {
	if r.factorID != uuid.Nil {
		return r.userID + "/" + r.factorID.String()
	}
	return r.userID
}

// FactorReconciler implements the K8s-style reconcile loop for MFA factors.
// Pattern: Observe → Diff → Act → Update Status
type FactorReconciler struct {
	factorRepo     repositories.FactorRepository
	challengeRepo  repositories.ChallengeRepository
	debugEnabled   bool
	resyncInterval time.Duration
}

// NewFactorReconciler creates a new reconciler for MFA factors.
func NewFactorReconciler(factorRepo repositories.FactorRepository, challengeRepo repositories.ChallengeRepository) *FactorReconciler {
	return &FactorReconciler{
		factorRepo:     factorRepo,
		challengeRepo:  challengeRepo,
		debugEnabled:   false,
		resyncInterval: 5 * time.Minute,
	}
}

// Reconcile performs the reconcile operation for a factor.
// Returns error if reconciliation fails; caller should handle retry.
func (r *FactorReconciler) Reconcile(ctx context.Context, req reconcileRequest) error {
	r.debugf("reconcile factor for user=%s factor=%s reason=%s", req.userID, req.factorID, req.reason)

	// If factorID provided, reconcile that specific factor
	if req.factorID != uuid.Nil {
		return r.reconcileFactor(ctx, req.factorID)
	}

	// Otherwise reconcile all factors for the user
	return r.reconcileUserFactors(ctx, req.userID)
}

func (r *FactorReconciler) reconcileFactor(ctx context.Context, factorID uuid.UUID) error {
	factor, err := r.factorRepo.Get(ctx, factorID)
	if err != nil {
		if isNotFound(err) {
			r.debugf("factor already deleted: %s", factorID)
			return nil
		}
		return err
	}

	// Observe: read current state
	// Diff: compare Spec → Status
	// Act: drive status toward desired state

	switch factor.Status.Phase {
	case models.FactorPhasePending:
		// Factor awaiting activation - nothing to reconcile
		r.debugf("factor pending activation: %s", factorID)
		return nil

	case models.FactorPhaseActive:
		// Check if verified recently
		if factor.Status.LastVerifiedAt != nil {
			// Check verification TTL - re-verify if expired
			verificationTTL := 30 * 24 * time.Hour // 30 days
			if time.Since(*factor.Status.LastVerifiedAt) > verificationTTL {
				factor.Status.SetCondition(models.Condition{
					Type:    models.ConditionTypeVerified,
					Status:  models.ConditionStatusFalse,
					Reason:  "VerificationExpired",
					Message: "Factor requires re-verification",
				})
			}
		}
		_, err := r.factorRepo.Update(ctx, factor)
		return err

	case models.FactorPhaseDisabled:
		// Factor is disabled - ensure it's properly cleaned up
		r.debugf("factor disabled: %s", factorID)
		return nil

	default:
		r.debugf("unknown phase for factor %s: %s", factorID, factor.Status.Phase)
		return nil
	}
}

func (r *FactorReconciler) reconcileUserFactors(ctx context.Context, userID string) error {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return err
	}

	factors, err := r.factorRepo.GetByUserID(ctx, userUUID)
	if err != nil {
		return err
	}

	r.debugf("reconciling %d factors for user %s", len(factors), userID)

	// Reconcile each factor
	for _, factor := range factors {
		if err := r.reconcileFactor(ctx, factor.ID); err != nil {
			logging.Z().Info(fmt.Sprintf("⚠️  Gatekeeper: factor reconcile error for %s: %v", factor.ID, err))
			// Continue with other factors
		}
	}

	return nil
}

// ReconcileExpiredChallenges cleans up expired challenge records.
func (r *FactorReconciler) ReconcileExpiredChallenges(ctx context.Context) error {
	challenges, err := r.challengeRepo.ListExpired(ctx)
	if err != nil {
		return err
	}

	for _, challenge := range challenges {
		if err := r.challengeRepo.Delete(ctx, challenge.ID); err != nil {
			r.debugf("failed to delete expired challenge %s: %v", challenge.ID, err)
			continue
		}
		r.debugf("deleted expired challenge: %s", challenge.ID)
	}

	return nil
}

func (r *FactorReconciler) debugf(format string, args ...interface{}) {
	if r.debugEnabled {
		logging.Z().Info(fmt.Sprintf("🔒 Gatekeeper: "+format, args...))
	}
}

func isNotFound(err error) bool {
	return err != nil && (err == repositories.ErrFactorNotFound ||
		err.Error() == "not found" ||
		err.Error() == "sql: no rows in result set")
}