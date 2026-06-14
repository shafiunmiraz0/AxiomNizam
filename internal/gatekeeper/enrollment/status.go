package enrollment

import "axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"

// FactorStatusSummary provides a user-friendly summary of factor status.
type FactorStatusSummary struct {
	HasActiveFactor bool
	ActiveCount     int
	PendingCount    int
	DisabledCount   int
	NeedsMFA        bool
}

// SummarizeFactors returns a summary of a user's factor statuses.
func SummarizeFactors(factors []*models.Factor) *FactorStatusSummary {
	summary := &FactorStatusSummary{}
	for _, f := range factors {
		switch f.Status.Phase {
		case models.FactorPhaseActive:
			summary.ActiveCount++
			summary.HasActiveFactor = true
		case models.FactorPhasePending:
			summary.PendingCount++
		case models.FactorPhaseDisabled:
			summary.DisabledCount++
		}
	}
	return summary
}

// CanEnroll checks if a user can enroll a new factor of the given type.
func CanEnroll(factors []*models.Factor, factorType models.FactorType) bool {
	for _, f := range factors {
		if f.Spec.Type == factorType && f.Status.Phase == models.FactorPhaseActive {
			return false // Already has an active factor of this type
		}
	}
	return true
}
