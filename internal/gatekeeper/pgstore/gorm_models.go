package pgstore

import (
	"time"

	"gorm.io/gorm"
)

// ────────────────────────────────────────────────────────────────
// GORM models for Gatekeeper 2FA tables.
// These mirror the domain models but carry GORM struct tags
// so that AutoMigrate can create / alter the schema.
// ────────────────────────────────────────────────────────────────

// FactorRow is the GORM representation of the twofactor_factors table.
type FactorRow struct {
	ID               string         `gorm:"primaryKey;type:varchar(36);not null"`
	UserID           string         `gorm:"type:varchar(36);not null;index"`
	Spec             []byte         `gorm:"type:jsonb;not null"`
	Status           []byte         `gorm:"type:jsonb;not null"`
	ResourceVersion  int64          `gorm:"not null;default:1"`
	DeletedAt        gorm.DeletedAt `gorm:"index"`
	CreatedAt        time.Time      `gorm:"not null;autoCreateTime"`
	UpdatedAt        time.Time      `gorm:"not null;autoUpdateTime"`
}

func (FactorRow) TableName() string { return "twofactor_factors" }

// ChallengeRow is the GORM representation of the twofactor_challenges table.
type ChallengeRow struct {
	ID              string    `gorm:"primaryKey;type:varchar(36);not null"`
	UserID          string    `gorm:"type:varchar(36);not null;index"`
	FactorID        string    `gorm:"type:varchar(36);not null;index"`
	Phase           string    `gorm:"type:varchar(20);not null"`
	Nonce           string    `gorm:"type:text"`
	Attempts        int       `gorm:"not null;default:0"`
	ExpiresAt       time.Time `gorm:"not null;index"`
	ResolvedAt      *time.Time
	IPAddress       string    `gorm:"type:varchar(45);not null"`
	UserAgent       string    `gorm:"type:text;not null"`
	ResourceVersion int64     `gorm:"not null;default:1"`
	CreatedAt       time.Time `gorm:"not null;autoCreateTime"`
}

func (ChallengeRow) TableName() string { return "twofactor_challenges" }

// BackupCodeRow is the GORM representation of the twofactor_backup_codes table.
type BackupCodeRow struct {
	ID        string     `gorm:"primaryKey;type:varchar(36);not null"`
	UserID    string     `gorm:"type:varchar(36);not null;index"`
	FactorID  string     `gorm:"type:varchar(36);not null;index"`
	CodeHash  []byte     `gorm:"type:bytea;not null"`
	UsedAt    *time.Time `gorm:"index"`
	CreatedAt time.Time  `gorm:"not null;autoCreateTime"`
}

func (BackupCodeRow) TableName() string { return "twofactor_backup_codes" }

// TrustedDeviceRow is the GORM representation of the twofactor_trusted_devices table.
type TrustedDeviceRow struct {
	ID          string     `gorm:"primaryKey;type:varchar(36);not null"`
	UserID      string     `gorm:"type:varchar(36);not null;index"`
	TokenHash   []byte     `gorm:"type:bytea;not null"`
	Fingerprint string     `gorm:"type:varchar(255);not null"`
	UserAgent   string     `gorm:"type:text;not null"`
	IPAddress   string     `gorm:"type:varchar(45);not null"`
	ExpiresAt   time.Time  `gorm:"not null;index"`
	RevokedAt   *time.Time `gorm:"index"`
	CreatedAt   time.Time  `gorm:"not null;autoCreateTime"`
}

func (TrustedDeviceRow) TableName() string { return "twofactor_trusted_devices" }

// AuditLogRow is the GORM representation of the twofactor_audit_log table.
type AuditLogRow struct {
	ID          string     `gorm:"primaryKey;type:varchar(36);not null"`
	EventType   string     `gorm:"type:varchar(50);not null;index"`
	UserID      string     `gorm:"type:varchar(36);not null;index"`
	FactorID    *string    `gorm:"type:varchar(36)"`
	ChallengeID *string    `gorm:"type:varchar(36)"`
	Severity    string     `gorm:"type:varchar(20);not null;index"`
	Message     string     `gorm:"type:text;not null"`
	SourceIP    string     `gorm:"type:varchar(45)"`
	UserAgent   string     `gorm:"type:text"`
	Metadata    []byte     `gorm:"type:jsonb"`
	CreatedAt   time.Time  `gorm:"not null;autoCreateTime;index"`
}

func (AuditLogRow) TableName() string { return "twofactor_audit_log" }

// WebAuthnCredentialRow is the GORM representation of the twofactor_webauthn_credentials table.
type WebAuthnCredentialRow struct {
	ID              []byte    `gorm:"primaryKey;type:bytea;not null"`
	UserID          string    `gorm:"type:varchar(36);not null;index"`
	PublicKey        []byte    `gorm:"type:bytea;not null"`
	AttestationType string    `gorm:"type:varchar(50);not null;default:none"`
	AAGUID          []byte    `gorm:"type:bytea;not null;default:''"`
	SignCount       uint32    `gorm:"not null;default:0"`
	CloneWarning    bool      `gorm:"not null;default:false"`
	CreatedAt       time.Time `gorm:"not null;autoCreateTime"`
}

func (WebAuthnCredentialRow) TableName() string { return "twofactor_webauthn_credentials" }

// MigrateGatekeeperTables runs GORM AutoMigrate for all Gatekeeper 2FA tables.
func MigrateGatekeeperTables(db *gorm.DB) error {
	return db.AutoMigrate(
		&FactorRow{},
		&ChallengeRow{},
		&BackupCodeRow{},
		&TrustedDeviceRow{},
		&AuditLogRow{},
		&WebAuthnCredentialRow{},
	)
}
