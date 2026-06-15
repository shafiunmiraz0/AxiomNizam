package testutil

import (
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// TestUserID is a fixed user ID for testing.
var TestUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// TestFactorID is a fixed factor ID for testing.
var TestFactorID = uuid.MustParse("00000000-0000-0000-0000-000000000002")

// TestChallengeID is a fixed challenge ID for testing.
var TestChallengeID = uuid.MustParse("00000000-0000-0000-0000-000000000003")

// NewTestFactor creates a test factor with sensible defaults.
func NewTestFactor() *models.Factor {
	now := time.Now().UTC()
	return &models.Factor{
		ID:     TestFactorID,
		UserID: TestUserID,
		Spec: models.FactorSpec{
			Type:            models.FactorTypeTOTP,
			Issuer:          "AxiomNizam",
			EncryptedSecret: []byte("test-secret"),
		},
		Status: models.FactorStatus{
			Phase: models.FactorPhaseActive,
			Conditions: []models.Condition{
				{
					Type:               models.ConditionTypeReady,
					Status:             models.ConditionStatusTrue,
					Reason:             "Activated",
					Message:            "Factor verified and active.",
					LastTransitionTime: now,
				},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// NewTestChallenge creates a test challenge with sensible defaults.
func NewTestChallenge() *models.Challenge {
	now := time.Now().UTC()
	return &models.Challenge{
		ID:        TestChallengeID,
		UserID:    TestUserID,
		FactorID:  TestFactorID,
		Phase:     models.ChallengePhaseWaiting,
		Nonce:     "123456",
		Attempts:  0,
		ExpiresAt: now.Add(5 * time.Minute),
		IPAddress: "127.0.0.1",
		UserAgent: "test-agent",
		CreatedAt: now,
	}
}
