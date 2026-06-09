package handlers

import (
	"net/http"

	"example.com/axiomnizam/internal/gatekeeper/contracts"
	"example.com/axiomnizam/internal/gatekeeper/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HTTPHandler provides REST endpoints for 2FA operations.
type HTTPHandler struct {
	enrollmentSvc contracts.EnrollmentService
	challengeSvc  contracts.ChallengeService
	factorSvc     contracts.FactorService
	policyService contracts.PolicyService
	riskSvc       contracts.RiskService
	deviceSvc     contracts.TrustedDeviceService
	backupSvc     contracts.BackupCodeService
	webauthnSvc   contracts.WebAuthnService
}

// NewHTTPHandler creates a new HTTP handler.
func NewHTTPHandler(
	es contracts.EnrollmentService,
	cs contracts.ChallengeService,
	fs contracts.FactorService,
	ps contracts.PolicyService,
	rs contracts.RiskService,
	ds contracts.TrustedDeviceService,
	bs contracts.BackupCodeService,
	ws contracts.WebAuthnService,
) *HTTPHandler {
	return &HTTPHandler{
		enrollmentSvc: es,
		challengeSvc:  cs,
		factorSvc:     fs,
		policyService: ps,
		riskSvc:       rs,
		deviceSvc:     ds,
		backupSvc:     bs,
		webauthnSvc:   ws,
	}
}

// RegisterRoutes registers all HTTP endpoints under the provided router group.
// The caller is responsible for applying auth middleware to the group.
func (h *HTTPHandler) RegisterRoutes(api *gin.RouterGroup) {

	// Enrollment endpoints
	api.POST("/enroll", h.EnrollFactor)
	api.POST("/activate", h.ActivateFactor)
	api.POST("/disable/:factorID", h.DisableFactor)

	// Factor endpoints
	api.GET("/factors/:userID", h.ListFactors)
	api.GET("/factor/:factorID", h.GetFactor)
	api.DELETE("/factor/:factorID", h.DeleteFactor)

	// Backup code endpoints
	api.POST("/backup-codes/regenerate", h.RegenerateBackupCodes)

	// Challenge endpoints
	api.POST("/challenge/begin", h.BeginChallenge)
	api.POST("/challenge/verify", h.VerifyChallenge)

	// Policy endpoints
	api.GET("/policy/:userID", h.EvaluatePolicy)

	// Risk endpoints
	api.POST("/risk/score", h.ScoreRisk)

	// Trusted device endpoints
	api.POST("/trust-device", h.TrustDevice)
	api.GET("/trust-device/list/:userID", h.ListTrustedDevices)
	api.DELETE("/trust-device/:deviceID", h.RevokeTrustedDevice)

	// WebAuthn / FIDO2 endpoints (Phase 10)
	api.POST("/webauthn/register/begin", h.BeginWebAuthnRegistration)
	api.POST("/webauthn/register/finish", h.FinishWebAuthnRegistration)
	api.POST("/webauthn/authenticate/begin", h.BeginWebAuthnAuthentication)
	api.POST("/webauthn/authenticate/finish", h.FinishWebAuthnAuthentication)
	api.GET("/webauthn/credentials/:userID", h.ListWebAuthnCredentials)
	api.DELETE("/webauthn/credentials/:credentialID", h.DeleteWebAuthnCredential)
}

// EnrollFactor enrolls a new factor for a user.
func (h *HTTPHandler) EnrollFactor(c *gin.Context) {
	var req EnrollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.enrollmentSvc.SetupFactor(c.Request.Context(), req.UserID, models.FactorType(req.FactorType), req.Label)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, EnrollResponse{
		FactorID: result.FactorID,
		Secret:   result.Secret,
	})
}

// ActivateFactor completes factor activation.
func (h *HTTPHandler) ActivateFactor(c *gin.Context) {
	var req ActivateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	codes, err := h.enrollmentSvc.ActivateFactor(c.Request.Context(), req.FactorID, req.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ActivateResponse{
		FactorID:    req.FactorID,
		BackupCodes: codes,
	})
}

// DisableFactor disables a factor.
func (h *HTTPHandler) DisableFactor(c *gin.Context) {
	factorID := uuid.MustParse(c.Param("factorID"))

	if err := h.enrollmentSvc.DisableFactor(c.Request.Context(), factorID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Factor disabled"})
}

// DeleteFactor permanently deletes (soft-delete) a factor.
func (h *HTTPHandler) DeleteFactor(c *gin.Context) {
	factorID := uuid.MustParse(c.Param("factorID"))

	if err := h.factorSvc.DeleteFactor(c.Request.Context(), factorID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Factor deleted"})
}

// RegenerateBackupCodes generates new backup codes for a factor.
func (h *HTTPHandler) RegenerateBackupCodes(c *gin.Context) {
	var req struct {
		FactorID uuid.UUID `json:"factor_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	codes, err := h.backupSvc.RegenerateBackupCodes(c.Request.Context(), req.FactorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"backup_codes": codes})
}

// ListFactors lists all factors for a user.
func (h *HTTPHandler) ListFactors(c *gin.Context) {
	userID := uuid.MustParse(c.Param("userID"))

	factors, err := h.factorSvc.ListFactors(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"factors": FactorsToResponse(factors)})
}

// GetFactor retrieves a single factor.
func (h *HTTPHandler) GetFactor(c *gin.Context) {
	factorID := uuid.MustParse(c.Param("factorID"))

	factor, err := h.factorSvc.GetFactor(c.Request.Context(), factorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if factor == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "factor not found"})
		return
	}

	c.JSON(http.StatusOK, FactorToResponse(factor))
}

// BeginChallenge starts a new MFA challenge.
func (h *HTTPHandler) BeginChallenge(c *gin.Context) {
	var req BeginChallengeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	challengeID, err := h.challengeSvc.BeginChallenge(c.Request.Context(), req.UserID, req.FactorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"challenge_id": challengeID})
}

// VerifyChallenge verifies a challenge response.
func (h *HTTPHandler) VerifyChallenge(c *gin.Context) {
	var req VerifyChallengeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	verified, err := h.challengeSvc.VerifyChallenge(c.Request.Context(), req.ChallengeID, req.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, VerifyChallengeResponse{
		Verified: verified,
	})
}

// EvaluatePolicy evaluates if MFA is required for a user.
func (h *HTTPHandler) EvaluatePolicy(c *gin.Context) {
	userID := uuid.MustParse(c.Param("userID"))

	requiresMFA, factors, err := h.policyService.EvaluatePolicy(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"requires_mfa": requiresMFA,
		"factors":      factors,
	})
}

// ScoreRisk scores the risk of an authentication request.
func (h *HTTPHandler) ScoreRisk(c *gin.Context) {
	var req ScoreRiskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	score, err := h.riskSvc.ScoreAuthentication(c.Request.Context(), req.UserID, req.IPAddress)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ScoreRiskResponse{
		Score:     score,
		Level:     RiskLevelForScore(score),
		IsHigh:    score >= 61,
		IPAddress: req.IPAddress,
	})
}

// TrustDevice registers a trusted device.
func (h *HTTPHandler) TrustDevice(c *gin.Context) {
	var req TrustDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := h.deviceSvc.TrustDevice(c.Request.Context(), req.UserID, req.Fingerprint, req.UserAgent, req.IPAddress)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

// ListTrustedDevices lists trusted devices for a user.
func (h *HTTPHandler) ListTrustedDevices(c *gin.Context) {
	userID := uuid.MustParse(c.Param("userID"))

	devices, err := h.deviceSvc.ListTrustedDevices(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responses := make([]*TrustDeviceResponse, len(devices))
	for i, d := range devices {
		responses[i] = DeviceToResponse(d)
	}

	c.JSON(http.StatusOK, gin.H{"devices": responses})
}

// RevokeTrustedDevice revokes a trusted device.
func (h *HTTPHandler) RevokeTrustedDevice(c *gin.Context) {
	deviceID := uuid.MustParse(c.Param("deviceID"))

	if err := h.deviceSvc.RevokeTrustedDevice(c.Request.Context(), deviceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Device revoked"})
}

// ─── WebAuthn / FIDO2 Handlers (Phase 10) ─────────────────────────────────

// BeginWebAuthnRegistration starts a WebAuthn registration ceremony.
func (h *HTTPHandler) BeginWebAuthnRegistration(c *gin.Context) {
	if h.webauthnSvc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "webauthn not available"})
		return
	}
	var req WebAuthnBeginRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sessionID, options, err := h.webauthnSvc.BeginRegistration(c.Request.Context(), req.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"session_id": sessionID,
		"options":    options,
	})
}

// FinishWebAuthnRegistration completes a WebAuthn registration ceremony.
func (h *HTTPHandler) FinishWebAuthnRegistration(c *gin.Context) {
	if h.webauthnSvc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "webauthn not available"})
		return
	}
	var req WebAuthnFinishRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.webauthnSvc.FinishRegistration(c.Request.Context(), req.UserID, req.SessionID, req.Response); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "WebAuthn credential registered successfully"})
}

// BeginWebAuthnAuthentication starts a WebAuthn authentication ceremony.
func (h *HTTPHandler) BeginWebAuthnAuthentication(c *gin.Context) {
	if h.webauthnSvc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "webauthn not available"})
		return
	}
	var req WebAuthnBeginAuthenticationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sessionID, options, err := h.webauthnSvc.BeginAuthentication(c.Request.Context(), req.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"session_id": sessionID,
		"options":    options,
	})
}

// FinishWebAuthnAuthentication completes a WebAuthn authentication ceremony.
func (h *HTTPHandler) FinishWebAuthnAuthentication(c *gin.Context) {
	if h.webauthnSvc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "webauthn not available"})
		return
	}
	var req WebAuthnFinishAuthenticationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	verified, err := h.webauthnSvc.FinishAuthentication(c.Request.Context(), req.UserID, req.SessionID, req.Response)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "verified": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"verified": verified})
}

// ListWebAuthnCredentials lists all WebAuthn credentials for a user.
func (h *HTTPHandler) ListWebAuthnCredentials(c *gin.Context) {
	if h.webauthnSvc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "webauthn not available"})
		return
	}
	userID := uuid.MustParse(c.Param("userID"))
	creds, err := h.webauthnSvc.ListCredentials(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"credentials": creds})
}

// DeleteWebAuthnCredential deletes a WebAuthn credential.
func (h *HTTPHandler) DeleteWebAuthnCredential(c *gin.Context) {
	if h.webauthnSvc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "webauthn not available"})
		return
	}
	credIDParam := c.Param("credentialID")
	credID, err := uuid.Parse(credIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential ID"})
		return
	}
	userID := uuid.UUID{} // delete by credential ID alone
	if err := h.webauthnSvc.DeleteCredential(c.Request.Context(), userID, credID[:]); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "WebAuthn credential deleted"})
}
