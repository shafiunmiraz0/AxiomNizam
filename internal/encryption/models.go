package encryption

import (
	"time"

	"example.com/axiomnizam/internal/encryption/models"
)

// EncryptedField represents an encrypted data field
type EncryptedField struct {
	FieldName      string            `json:"fieldName"`
	EncryptedValue string            `json:"encryptedValue" classification:"Confidential"` // Base64 encoded
	KeyID          string            `json:"keyId"`
	KeyVersion     int               `json:"keyVersion"`
	Algorithm      string            `json:"algorithm"`         // AES-256-GCM, ChaCha20-Poly1305
	IV             string            `json:"iv,omitempty" classification:"Confidential"`      // Base64 encoded initialization vector
	Salt           string            `json:"salt,omitempty" classification:"Confidential"`    // Base64 encoded salt
	Nonce          string            `json:"nonce,omitempty" classification:"Confidential"`   // Base64 encoded nonce
	AuthTag        string            `json:"authTag,omitempty" classification:"Confidential"` // Authentication tag for AEAD
	IsEncrypted    bool              `json:"isEncrypted"`
	EncryptedAt    time.Time         `json:"encryptedAt"`
	EncryptedBy    string            `json:"encryptedBy"`    // User ID
	CanDecrypt     bool              `json:"canDecrypt"`     // Current user can decrypt
	Classification string            `json:"classification"` // PII, Sensitive, etc
	Hashable       bool              `json:"hashable"`       // Can field be hashed for lookup
	Searchable     bool              `json:"searchable"`     // Can be searched (requires special indexing)
	Metadata       map[string]string `json:"metadata"`
}

// EncryptionKey represents encryption key
type EncryptionKey struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenantId"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	KeyType        KeyType           `json:"keyType"`               // DEK, KEK, Master
	Algorithm      string            `json:"algorithm"`             // AES-256, ChaCha20
	KeyMaterial    string            `json:"keyMaterial,omitempty" classification:"Confidential"` // Base64, not exposed
	PublicKey      string            `json:"publicKey,omitempty" classification:"Sensitive"`   // For asymmetric
	KeyLength      int               `json:"keyLength"`             // Bits: 128, 256, 512
	Status         KeyStatus         `json:"status"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
	ExpiresAt      time.Time         `json:"expiresAt,omitempty"`
	RotatedAt      time.Time         `json:"rotatedAt,omitempty"`
	NextRotation   time.Time         `json:"nextRotation,omitempty"`
	RotationPolicy RotationPolicy    `json:"rotationPolicy"`
	Version        int               `json:"version"`
	IsDefault      bool              `json:"isDefault"`
	CreatedBy      string            `json:"createdBy"`
	Owner          string            `json:"owner"`
	ACL            []ACLEntry        `json:"acl"` // Access control
	Usage          KeyUsageStats     `json:"usage"`
	Metadata       map[string]string `json:"metadata"`
	Tags           []string          `json:"tags"`
}

// Type aliases (canonical definitions in models/)
type KeyType = models.KeyType
type KeyStatus = models.KeyStatus
type RotationPolicy = models.RotationPolicy
type ACLEntry = models.ACLEntry
type KeyUsageStats = models.KeyUsageStats

const (
	KeyTypeDEK    = models.KeyTypeDEK
	KeyTypeKEK    = models.KeyTypeKEK
	KeyTypeMaster = models.KeyTypeMaster

	KeyStatusActive   = models.KeyStatusActive
	KeyStatusInactive = models.KeyStatusInactive
	KeyStatusRotating = models.KeyStatusRotating
	KeyStatusExpired  = models.KeyStatusExpired
	KeyStatusRevoked  = models.KeyStatusRevoked
)

// KeyProvider represents external key management
type KeyProvider struct {
	ID              string                 `json:"id"`
	Type            string                 `json:"type"` // "aws-kms", "azure-keyvault", "gcp-cloud-kms", "vault"
	Name            string                 `json:"name"`
	Endpoint        string                 `json:"endpoint"`
	Region          string                 `json:"region,omitempty"`
	Config          map[string]interface{} `json:"config"`
	Credentials     ProviderCredentials    `json:"credentials"`
	IsHealthy       bool                   `json:"isHealthy"`
	LastHealthCheck time.Time              `json:"lastHealthCheck"`
	ConnectedAt     time.Time              `json:"connectedAt"`
	DisconnectedAt  time.Time              `json:"disconnectedAt,omitempty"`
	Metadata        map[string]string      `json:"metadata"`
}

// ProviderCredentials for key provider
type ProviderCredentials struct {
	Type        string            `json:"type"` // "api-key", "oauth2", "certificate"
	ApiKey      string            `json:"apiKey,omitempty" classification:"Confidential"`
	ApiSecret   string            `json:"apiSecret,omitempty" classification:"Confidential"`
	Certificate string            `json:"certificate,omitempty" classification:"Confidential"`
	OAuth2      OAuth2Credentials `json:"oauth2,omitempty"`
}

// OAuth2Credentials for OAuth2 auth
type OAuth2Credentials struct {
	ClientID     string   `json:"clientId" classification:"Sensitive"`
	ClientSecret string   `json:"clientSecret" classification:"Confidential"`
	TokenURL     string   `json:"tokenUrl"`
	Scopes       []string `json:"scopes"`
}

// FieldEncryptionPolicy defines which fields get encrypted
type FieldEncryptionPolicy struct {
	ID              string      `json:"id"`
	TenantID        string      `json:"tenantId"`
	Name            string      `json:"name"`
	Description     string      `json:"description"`
	ResourceType    string      `json:"resourceType"`
	FieldRules      []FieldRule `json:"fieldRules"` // Which fields to encrypt
	KeyID           string      `json:"keyId"`      // Default key
	Algorithm       string      `json:"algorithm"`  // Default algorithm
	Enabled         bool        `json:"enabled"`
	ApplyToExisting bool        `json:"applyToExisting"` // Encrypt existing data
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
	CreatedBy       string      `json:"createdBy"`
}

// Type alias (canonical definition in models/)
type FieldRule = models.FieldRule

// EncryptionRequest encrypts data
type EncryptionRequest struct {
	TenantID  string                 `json:"tenantId"`
	Data      map[string]interface{} `json:"data"`
	Fields    []string               `json:"fields"` // Which fields to encrypt
	KeyID     string                 `json:"keyId,omitempty"`
	Algorithm string                 `json:"algorithm,omitempty"`
	Metadata  map[string]string      `json:"metadata,omitempty"`
}

// EncryptionResponse returns encrypted data
type EncryptionResponse struct {
	Data      map[string]interface{} `json:"data"`
	Encrypted bool                   `json:"encrypted"`
	KeyID     string                 `json:"keyId"`
	Algorithm string                 `json:"algorithm"`
	Timestamp time.Time              `json:"timestamp"`
}

// DecryptionRequest decrypts data
type DecryptionRequest struct {
	TenantID string                 `json:"tenantId"`
	Data     map[string]interface{} `json:"data"`
	Fields   []string               `json:"fields,omitempty"` // Which fields to decrypt (all if empty)
	KeyID    string                 `json:"keyId,omitempty"`
}

// DecryptionResponse returns decrypted data
type DecryptionResponse struct {
	Data      map[string]interface{} `json:"data"`
	Decrypted bool                   `json:"decrypted"`
	Timestamp time.Time              `json:"timestamp"`
}

// TokenizationPolicy replaces sensitive data with tokens
type TokenizationPolicy struct {
	ID              string   `json:"id"`
	TenantID        string   `json:"tenantId"`
	Name            string   `json:"name"`
	Fields          []string `json:"fields"`      // Which fields
	TokenFormat     string   `json:"tokenFormat"` // "uuid", "hash", "masked"
	Reversible      bool     `json:"reversible"`  // Can be reversed
	KeyID           string   `json:"keyId"`
	StorageLocation string   `json:"storageLocation"` // Where to store token mapping
	Enabled         bool     `json:"enabled"`
}

// EncryptionAuditLog tracks encryption operations
type EncryptionAuditLog struct {
	ID           string            `json:"id"`
	TenantID     string            `json:"tenantId"`
	KeyID        string            `json:"keyId"`
	Operation    string            `json:"operation"` // "encrypt", "decrypt", "rotate", "export"
	ResourceType string            `json:"resourceType"`
	ResourceID   string            `json:"resourceId"`
	User         string            `json:"user"`
	Timestamp    time.Time         `json:"timestamp"`
	Status       string            `json:"status"` // "success", "failure"
	ErrorMessage string            `json:"errorMessage,omitempty"`
	DataSize     int64             `json:"dataSize"`
	Duration     int64             `json:"duration"` // Milliseconds
	SourceIP     string            `json:"sourceIp"`
	Metadata     map[string]string `json:"metadata"`
}

// KeyRotationEvent tracks key rotations
type KeyRotationEvent struct {
	ID                string    `json:"id"`
	KeyID             string    `json:"keyId"`
	TenantID          string    `json:"tenantId"`
	OldKeyVersion     int       `json:"oldKeyVersion"`
	NewKeyVersion     int       `json:"newKeyVersion"`
	Reason            string    `json:"reason"` // "scheduled", "manual", "revoked"
	StartedAt         time.Time `json:"startedAt"`
	CompletedAt       time.Time `json:"completedAt,omitempty"`
	Status            string    `json:"status"` // "in_progress", "completed", "failed"
	DataMigratedCount int64     `json:"dataMigratedCount"`
	ErrorCount        int64     `json:"errorCount"`
	Errors            []string  `json:"errors"`
	RotatedBy         string    `json:"rotatedBy"`
}

// SecretsManager interface for managing secrets
type SecretsManager interface {
	CreateKey(key *EncryptionKey) error
	GetKey(keyID string) (*EncryptionKey, error)
	UpdateKey(key *EncryptionKey) error
	DeleteKey(keyID string) error
	RotateKey(keyID string) (*EncryptionKey, error)
	ListKeys(tenantID string) ([]*EncryptionKey, error)
	Encrypt(req *EncryptionRequest) (*EncryptedField, error)
	Decrypt(field *EncryptedField) (interface{}, error)
	GenerateDEK(keyID string) ([]byte, error)
}

// EncryptionStats tracks encryption metrics
type EncryptionStats struct {
	TenantID              string
	TotalKeysActive       int
	TotalKeysExpired      int
	TotalKeysRevoked      int
	TotalEncryptions      int64
	TotalDecryptions      int64
	FailureCount          int64
	AverageLatency        float64 // Milliseconds
	BytesEncrypted        int64
	KeyRotationsCount     int
	LastKeyRotation       time.Time
	PolicyComplianceScore float64 // 0-100
}
