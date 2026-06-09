package models

// FactorType represents the type of 2FA factor.
type FactorType string

const (
	FactorTypeTOTP       FactorType = "totp"     // Time-based OTP (authenticator apps)
	FactorTypeSMS        FactorType = "sms"      // SMS OTP
	FactorTypeEmail      FactorType = "email"    // Email OTP
	FactorTypeWebAuthn   FactorType = "webauthn" // Security keys / biometric
	FactorTypeBackupCode FactorType = "backup"   // Backup codes
)

// FactorPhase represents the lifecycle state of a Factor.
type FactorPhase string

const (
	FactorPhasePending  FactorPhase = "Pending"  // Enrollment in progress
	FactorPhaseActive   FactorPhase = "Active"   // Ready for authentication
	FactorPhaseDisabled FactorPhase = "Disabled" // Explicitly disabled
	FactorPhaseRevoked  FactorPhase = "Revoked"  // Automatically revoked (e.g., due to compromise)
	FactorPhaseFailed   FactorPhase = "Failed"   // Provisioning error
)

// ChallengePhase represents the lifecycle state of an authentication challenge.
type ChallengePhase string

const (
	ChallengePhaseWaiting  ChallengePhase = "Waiting"  // Awaiting user verification
	ChallengePhaseVerified ChallengePhase = "Verified" // Successfully verified
	ChallengePhaseExpired  ChallengePhase = "Expired"  // TTL exceeded
	ChallengePhaseFailed   ChallengePhase = "Failed"   // Too many failed attempts
	ChallengePhaseRejected ChallengePhase = "Rejected" // Risk check failure
)

// ConditionType is a K8s-style condition type.
type ConditionType string

const (
	ConditionTypeReady         ConditionType = "Ready"
	ConditionTypeProvisioning  ConditionType = "Provisioning"
	ConditionTypeError         ConditionType = "Error"
	ConditionTypeVerified      ConditionType = "Verified"
	ConditionTypeMFARequired   ConditionType = "MFARequired"
)

// ConditionStatus is a K8s-style condition status.
type ConditionStatus string

const (
	ConditionStatusTrue    ConditionStatus = "True"
	ConditionStatusFalse   ConditionStatus = "False"
	ConditionStatusUnknown ConditionStatus = "Unknown"
)

// EnrollmentStage represents the stage of the MFA enrollment workflow.
type EnrollmentStage string

const (
	EnrollmentStageInitiate    EnrollmentStage = "Initiate"    // User starts enrollment
	EnrollmentStageSetup       EnrollmentStage = "Setup"       // Generate secret/QR code
	EnrollmentStageActivate    EnrollmentStage = "Activate"    // User verifies OTP
	EnrollmentStageBackupCodes EnrollmentStage = "BackupCodes" // Generate and acknowledge backup codes
	EnrollmentStageComplete    EnrollmentStage = "Complete"    // Enrollment finished
	EnrollmentStageCancelled   EnrollmentStage = "Cancelled"   // User cancelled
)

// RiskLevel represents the adaptive risk score.
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "Low"
	RiskLevelMedium   RiskLevel = "Medium"
	RiskLevelHigh     RiskLevel = "High"
	RiskLevelCritical RiskLevel = "Critical"
)

// PolicyEnforcement specifies when MFA is required.
type PolicyEnforcement string

const (
	PolicyEnforcementOptional PolicyEnforcement = "Optional" // User can skip MFA
	PolicyEnforcementRequired PolicyEnforcement = "Required" // MFA always required
	PolicyEnforcementAdaptive PolicyEnforcement = "Adaptive" // MFA required based on risk
)

// AuditEventType logs security-relevant MFA actions.
type AuditEventType string

const (
	AuditEventEnrolled             AuditEventType = "Enrolled"
	AuditEventVerified             AuditEventType = "Verified"
	AuditEventVerificationFailed   AuditEventType = "VerificationFailed"
	AuditEventDisabled             AuditEventType = "Disabled"
	AuditEventRevoked              AuditEventType = "Revoked"
	AuditEventBackupCodeUsed       AuditEventType = "BackupCodeUsed"
	AuditEventTrustedDeviceAdded   AuditEventType = "TrustedDeviceAdded"
	AuditEventTrustedDeviceRevoked AuditEventType = "TrustedDeviceRevoked"
	AuditEventHighRiskDetected     AuditEventType = "HighRiskDetected"
	AuditEventChallengeCreated     AuditEventType = "ChallengeCreated"
	AuditEventChallengeExpired     AuditEventType = "ChallengeExpired"
	AuditEventWebAuthnRegistered   AuditEventType = "WebAuthnRegistered"
	AuditEventWebAuthnVerified     AuditEventType = "WebAuthnVerified"
	AuditEventWebAuthnFailed       AuditEventType = "WebAuthnFailed"
	AuditEventWebAuthnCredDeleted  AuditEventType = "WebAuthnCredentialDeleted"
)

// RaftCommandType represents the type of Raft command for MFA state.
type RaftCommandType string

const (
	RaftCmdEnrollFactor         RaftCommandType = "EnrollFactor"
	RaftCmdActivateFactor      RaftCommandType = "ActivateFactor"
	RaftCmdDisableFactor       RaftCommandType = "DisableFactor"
	RaftCmdCreateChallenge     RaftCommandType = "CreateChallenge"
	RaftCmdVerifyChallenge     RaftCommandType = "VerifyChallenge"
	RaftCmdExpireChallenge     RaftCommandType = "ExpireChallenge"
	RaftCmdGenerateBackupCodes RaftCommandType = "GenerateBackupCodes"
	RaftCmdConsumeBackupCode   RaftCommandType = "ConsumeBackupCode"
	RaftCmdTrustDevice         RaftCommandType = "TrustDevice"
	RaftCmdRevokeDevice        RaftCommandType = "RevokeDevice"
)
