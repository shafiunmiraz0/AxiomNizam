package models

// =====================================================
// P2 resource-ification -- Encryption.
//
// EncryptionKeyResource wraps the imperative EncryptionKey so a
// controller can reconcile encryption keys as first-class platform
// resources. EncryptionPolicyResource wraps FieldEncryptionPolicy.
// =====================================================

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	EncryptionKeyKind          = "EncryptionKey"
	EncryptionKeyAPIVersion    = "encryption.axiomnizam.io/v1"
	EncryptionPolicyKind       = "EncryptionPolicy"
	EncryptionPolicyAPIVersion = "encryption.axiomnizam.io/v1"
)

// KeyType represents type of encryption key
type KeyType string

const (
	KeyTypeDEK    KeyType = "DEK"    // Data Encryption Key
	KeyTypeKEK    KeyType = "KEK"    // Key Encryption Key
	KeyTypeMaster KeyType = "MASTER" // Master Key
)

// KeyStatus represents key state
type KeyStatus string

const (
	KeyStatusActive   KeyStatus = "ACTIVE"
	KeyStatusInactive KeyStatus = "INACTIVE"
	KeyStatusRotating KeyStatus = "ROTATING"
	KeyStatusExpired  KeyStatus = "EXPIRED"
	KeyStatusRevoked  KeyStatus = "REVOKED"
)

// RotationPolicy defines key rotation
type RotationPolicy struct {
	Enabled          bool
	RotationInterval int // Days
	AutoRotate       bool
	RotationDay      int    // Day of month
	RotationTime     string // HH:MM
	OnRotation       string // "retire", "archive", "keep"
	MinimumKeyAge    int    // Days before rotation allowed
	MaximumKeyAge    int    // Force rotation after this many days
}

// ACLEntry for key access control
type ACLEntry struct {
	Principal     string                 `json:"principal"`     // User ID or role
	PrincipalType string                 `json:"principalType"` // "user", "role", "service"
	Permissions   []string               `json:"permissions"`   // "encrypt", "decrypt", "rotate", "export"
	Conditions    map[string]interface{} `json:"conditions"`
	GrantedAt     time.Time              `json:"grantedAt"`
	ExpiresAt     time.Time              `json:"expiresAt,omitempty"`
}

// KeyUsageStats tracks key usage
type KeyUsageStats struct {
	EncryptionCount    int64     `json:"encryptionCount"`
	DecryptionCount    int64     `json:"decryptionCount"`
	FailureCount       int64     `json:"failureCount"`
	LastUsedAt         time.Time `json:"lastUsedAt"`
	FirstUsedAt        time.Time `json:"firstUsedAt"`
	AverageLatency     float64   `json:"averageLatency"`     // Milliseconds
	TotalDataEncrypted int64     `json:"totalDataEncrypted"` // Bytes
}

// FieldRule defines encryption for specific fields
type FieldRule struct {
	FieldName      string   `json:"fieldName"`
	Pattern        string   `json:"pattern"`        // Regex pattern
	Classification string   `json:"classification"` // PII, Sensitive
	Encrypt        bool     `json:"encrypt"`
	KeyID          string   `json:"keyId"` // Override default
	Algorithm      string   `json:"algorithm,omitempty"`
	Hashable       bool     `json:"hashable"`
	Searchable     bool     `json:"searchable"`
	MaskPattern    string   `json:"maskPattern,omitempty"` // For display
	Conditions     []string `json:"conditions,omitempty"`  // When to apply
}

// --- EncryptionKeyResource ---

// EncryptionKeySpec is the desired state of an encryption key.
type EncryptionKeySpec struct {
	TenantID       string         `json:"tenantId,omitempty"`
	Description    string         `json:"description,omitempty"`
	KeyType        KeyType        `json:"keyType"`
	Algorithm      string         `json:"algorithm"`
	KeyLength      int            `json:"keyLength"`
	RotationPolicy RotationPolicy `json:"rotationPolicy,omitempty"`
	IsDefault      bool           `json:"isDefault,omitempty"`
	Owner          string         `json:"owner,omitempty"`
	ACL            []ACLEntry     `json:"acl,omitempty"`
	Tags           []string       `json:"tags,omitempty"`

	// RotateNow, when true, asks the controller to rotate the key.
	RotateNow bool `json:"rotateNow,omitempty"`
}

// EncryptionKeyResourceStatus extends the canonical object status.
type EncryptionKeyResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	KeyStatus    KeyStatus     `json:"keyStatus"`
	Version      int           `json:"version"`
	Usage        KeyUsageStats `json:"usage"`
	RotatedAt    *time.Time    `json:"rotatedAt,omitempty"`
	NextRotation *time.Time    `json:"nextRotation,omitempty"`
	ExpiresAt    *time.Time    `json:"expiresAt,omitempty"`
}

// EncryptionKeyResource is the declarative resource for an EncryptionKey.
type EncryptionKeyResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   EncryptionKeySpec           `json:"spec"`
	Status EncryptionKeyResourceStatus `json:"status"`
}

func (r *EncryptionKeyResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *EncryptionKeyResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *EncryptionKeyResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *EncryptionKeyResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *EncryptionKeyResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.ACL) > 0 {
		cp.Spec.ACL = append([]ACLEntry(nil), r.Spec.ACL...)
	}
	if len(r.Spec.Tags) > 0 {
		cp.Spec.Tags = append([]string(nil), r.Spec.Tags...)
	}
	return &cp
}
func (r *EncryptionKeyResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *EncryptionKeyResource) GetGeneration() int64         { return r.Generation }
func (r *EncryptionKeyResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// --- EncryptionPolicyResource ---

// EncryptionPolicySpec is the desired state of a field encryption policy.
type EncryptionPolicySpec struct {
	TenantID        string      `json:"tenantId,omitempty"`
	Description     string      `json:"description,omitempty"`
	ResourceType    string      `json:"resourceType"`
	FieldRules      []FieldRule `json:"fieldRules"`
	KeyID           string      `json:"keyId"`
	Algorithm       string      `json:"algorithm,omitempty"`
	ApplyToExisting bool        `json:"applyToExisting,omitempty"`
	Enabled         bool        `json:"enabled"`
}

// EncryptionPolicyResourceStatus extends the canonical object status.
type EncryptionPolicyResourceStatus struct {
	resources.ObjectStatus `json:",inline"`

	PolicyActive bool `json:"policyActive"`
	FieldCount   int  `json:"fieldCount"`
}

// EncryptionPolicyResource is the declarative resource for a FieldEncryptionPolicy.
type EncryptionPolicyResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`

	Spec   EncryptionPolicySpec           `json:"spec"`
	Status EncryptionPolicyResourceStatus `json:"status"`
}

func (r *EncryptionPolicyResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *EncryptionPolicyResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *EncryptionPolicyResource) GetStatus() *resources.ObjectStatus {
	return &r.Status.ObjectStatus
}
func (r *EncryptionPolicyResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *EncryptionPolicyResource) DeepCopy() resources.Resource {
	cp := *r
	if len(r.Spec.FieldRules) > 0 {
		cp.Spec.FieldRules = append([]FieldRule(nil), r.Spec.FieldRules...)
	}
	return &cp
}
func (r *EncryptionPolicyResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *EncryptionPolicyResource) GetGeneration() int64         { return r.Generation }
func (r *EncryptionPolicyResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
