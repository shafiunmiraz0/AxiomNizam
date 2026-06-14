package handlers

import (
	"context"

	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/contracts"
)

// GRPCHandler provides gRPC endpoints for MFA operations.
// This is a placeholder — full gRPC implementation requires
// protobuf definitions and generated code.
type GRPCHandler struct {
	enrollmentSvc contracts.EnrollmentService
	challengeSvc  contracts.ChallengeService
	factorSvc     contracts.FactorService
}

// NewGRPCHandler creates a new gRPC handler.
func NewGRPCHandler(
	es contracts.EnrollmentService,
	cs contracts.ChallengeService,
	fs contracts.FactorService,
) *GRPCHandler {
	return &GRPCHandler{
		enrollmentSvc: es,
		challengeSvc:  cs,
		factorSvc:     fs,
	}
}

// HealthCheck returns the service health status.
func (h *GRPCHandler) HealthCheck(ctx context.Context) (bool, error) {
	return true, nil
}
