package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/keyring"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"

	"go.uber.org/zap"
)

// FieldLevelEncryption manages field-level encryption
type FieldLevelEncryption struct {
	mu                sync.RWMutex
	keys              map[string]*EncryptionKey
	policies          map[string][]*FieldEncryptionPolicy
	encryptedData     map[string]*EncryptedField
	keyRotationLog    []*KeyRotationEvent
	encryptionMetrics *EncryptionStats
	maxKeyVersions    int
	maxLogSize        int
	ring              *keyring.Keyring // AES-GCM key rotation manager
}

// NewFieldLevelEncryption creates encryption manager
func NewFieldLevelEncryption() *FieldLevelEncryption {
	ring := keyring.New()
	// Install an initial active key so Encrypt works immediately.
	if _, err := ring.Rotate(); err != nil {
		// Log but don't fail — callers can retry via RotateKeyring().
		logging.Z().Warn("initial keyring rotation failed", zap.Error(err))
	}

	return &FieldLevelEncryption{
		keys:              make(map[string]*EncryptionKey),
		policies:          make(map[string][]*FieldEncryptionPolicy),
		encryptedData:     make(map[string]*EncryptedField),
		keyRotationLog:    make([]*KeyRotationEvent, 0),
		encryptionMetrics: &EncryptionStats{},
		maxKeyVersions:    10,
		maxLogSize:        10000,
		ring:              ring,
	}
}

// RegisterKey registers an encryption key
func (fle *FieldLevelEncryption) RegisterKey(key *EncryptionKey) error {
	fle.mu.Lock()
	defer fle.mu.Unlock()

	if key.ID == "" {
		key.ID = fmt.Sprintf("key-%d", time.Now().UnixNano())
	}

	key.CreatedAt = time.Now()
	fle.keys[key.ID] = key
	return nil
}

// AddEncryptionPolicy adds an encryption policy for a field
func (fle *FieldLevelEncryption) AddEncryptionPolicy(policy *FieldEncryptionPolicy) error {
	fle.mu.Lock()
	defer fle.mu.Unlock()

	if policy.ID == "" {
		policy.ID = fmt.Sprintf("pol-%d", time.Now().UnixNano())
	}

	policy.CreatedAt = time.Now()
	policy.UpdatedAt = time.Now()

	if policy.ID == "" {
		policy.ID = fmt.Sprintf("policy-%d", time.Now().UnixNano())
	}
	return nil
}

// EncryptField encrypts a field value
func (fle *FieldLevelEncryption) EncryptField(tableField string, value interface{}, keyID string) (*EncryptedField, error) {
	fle.mu.RLock()
	key, exists := fle.keys[keyID]
	if !exists {
		fle.mu.RUnlock()
		fle.mu.Lock()
		fle.encryptionMetrics.FailureCount++
		fle.mu.Unlock()
		return nil, fmt.Errorf("key not found: %s", keyID)
	}

	if key.Status != KeyStatusActive {
		fle.mu.RUnlock()
		return nil, fmt.Errorf("key not active: %s", keyID)
	}
	fle.mu.RUnlock()

	// Decode key material
	keyBytes, err := base64.StdEncoding.DecodeString(key.KeyMaterial)
	if err != nil {
		fle.mu.Lock()
		fle.encryptionMetrics.FailureCount++
		fle.mu.Unlock()
		return nil, err
	}

	// Convert value to string
	plaintext := []byte(fmt.Sprintf("%v", value))

	// Create AES cipher
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		fle.mu.Lock()
		fle.encryptionMetrics.FailureCount++
		fle.mu.Unlock()
		return nil, err
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		fle.mu.Lock()
		fle.encryptionMetrics.FailureCount++
		fle.mu.Unlock()
		return nil, err
	}

	// Create nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		fle.mu.Lock()
		fle.encryptionMetrics.FailureCount++
		fle.mu.Unlock()
		return nil, err
	}

	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	encryptedValue := base64.StdEncoding.EncodeToString(ciphertext)
	ivValue := base64.StdEncoding.EncodeToString(nonce)

	encryptedField := &EncryptedField{
		FieldName:      tableField,
		EncryptedValue: encryptedValue,
		IV:             ivValue,
		KeyID:          keyID,
		EncryptedAt:    time.Now(),
	}

	fle.mu.Lock()
	fle.encryptedData[tableField] = encryptedField
	fle.encryptionMetrics.TotalEncryptions++
	fle.encryptionMetrics.BytesEncrypted += int64(len(plaintext))
	fle.mu.Unlock()

	return encryptedField, nil
}

// DecryptField decrypts a field value
func (fle *FieldLevelEncryption) DecryptField(encryptedField *EncryptedField) (interface{}, error) {
	fle.mu.RLock()
	key, exists := fle.keys[encryptedField.KeyID]
	if !exists {
		fle.mu.RUnlock()
		fle.mu.Lock()
		fle.encryptionMetrics.FailureCount++
		fle.mu.Unlock()
		return nil, fmt.Errorf("key not found: %s", encryptedField.KeyID)
	}
	fle.mu.RUnlock()

	// Decode ciphertext
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedField.EncryptedValue)
	if err != nil {
		fle.mu.Lock()
		fle.encryptionMetrics.FailureCount++
		fle.mu.Unlock()
		return nil, err
	}

	// Decode IV
	iv, err := base64.StdEncoding.DecodeString(encryptedField.IV)
	if err != nil {
		fle.mu.Lock()
		fle.encryptionMetrics.FailureCount++
		fle.mu.Unlock()
		return nil, err
	}

	// Decode key material
	keyBytes, err := base64.StdEncoding.DecodeString(key.KeyMaterial)
	if err != nil {
		fle.mu.Lock()
		fle.encryptionMetrics.FailureCount++
		fle.mu.Unlock()
		return nil, err
	}

	// Create cipher
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		fle.mu.Lock()
		fle.encryptionMetrics.FailureCount++
		fle.mu.Unlock()
		return nil, err
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		fle.mu.Lock()
		fle.encryptionMetrics.FailureCount++
		fle.mu.Unlock()
		return nil, err
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		fle.mu.Lock()
		fle.encryptionMetrics.FailureCount++
		fle.mu.Unlock()
		return nil, err
	}

	fle.mu.Lock()
	fle.encryptionMetrics.TotalDecryptions++
	fle.mu.Unlock()

	return string(plaintext), nil
}

// RotateKey rotates encryption key
func (fle *FieldLevelEncryption) RotateKey(oldKeyID string, newKey *EncryptionKey) (*KeyRotationEvent, error) {
	fle.mu.Lock()
	defer fle.mu.Unlock()

	// Get old key
	oldKey, exists := fle.keys[oldKeyID]
	if !exists {
		return nil, fmt.Errorf("old key not found: %s", oldKeyID)
	}

	event := &KeyRotationEvent{
		ID:            fmt.Sprintf("rot-%d", time.Now().UnixNano()),
		StartedAt:     time.Now(),
		OldKeyVersion: oldKey.Version,
		Status:        "in_progress",
	}

	// Register new key
	if newKey.ID == "" {
		newKey.ID = fmt.Sprintf("key-%d", time.Now().UnixNano())
	}
	newKey.CreatedAt = time.Now()
	fle.keys[newKey.ID] = newKey
	event.KeyID = newKey.ID

	// Mark old key inactive
	if oldKey, exists := fle.keys[oldKeyID]; exists {
		oldKey.Status = KeyStatusInactive
	}

	event.Status = "completed"
	fle.keyRotationLog = append(fle.keyRotationLog, event)
	fle.encryptionMetrics.KeyRotationsCount++
	fle.encryptionMetrics.LastKeyRotation = time.Now()

	if len(fle.keyRotationLog) > fle.maxLogSize {
		fle.keyRotationLog = fle.keyRotationLog[1:]
	}

	return event, nil
}

// GetPoliciesForField gets encryption policies for a field
func (fle *FieldLevelEncryption) GetPoliciesForField(table, field string) []*FieldEncryptionPolicy {
	fle.mu.RLock()
	defer fle.mu.RUnlock()

	tableField := fmt.Sprintf("%s.%s", table, field)
	if policies, exists := fle.policies[tableField]; exists {
		return policies
	}
	return make([]*FieldEncryptionPolicy, 0)
}

// GetEncryptionMetrics returns encryption metrics
func (fle *FieldLevelEncryption) GetEncryptionMetrics() *EncryptionStats {
	return fle.encryptionMetrics
}

// GetKeyRotationLog gets key rotation history
func (fle *FieldLevelEncryption) GetKeyRotationLog(limit int) []*KeyRotationEvent {
	fle.mu.RLock()
	defer fle.mu.RUnlock()

	if limit > len(fle.keyRotationLog) {
		limit = len(fle.keyRotationLog)
	}
	if limit == 0 {
		return make([]*KeyRotationEvent, 0)
	}

	return fle.keyRotationLog[len(fle.keyRotationLog)-limit:]
}

// RotateKeyring generates a new active AES-GCM key in the keyring.
// The previous active key is automatically retired and kept for
// decryption of older ciphertexts. Returns the new key ID.
func (fle *FieldLevelEncryption) RotateKeyring() (string, error) {
	fle.mu.Lock()
	defer fle.mu.Unlock()

	newID, err := fle.ring.Rotate()
	if err != nil {
		return "", err
	}

	fle.encryptionMetrics.KeyRotationsCount++
	fle.encryptionMetrics.LastKeyRotation = time.Now()

	event := &KeyRotationEvent{
		ID:        fmt.Sprintf("rot-%d", time.Now().UnixNano()),
		KeyID:     newID.String(),
		StartedAt: time.Now(),
		Status:    "completed",
	}
	fle.keyRotationLog = append(fle.keyRotationLog, event)
	if len(fle.keyRotationLog) > fle.maxLogSize {
		fle.keyRotationLog = fle.keyRotationLog[1:]
	}

	return newID.String(), nil
}

// EncryptWithKeyring encrypts using the keyring's active key.
// The ciphertext includes the key ID prefix, enabling automatic
// key selection during decryption.
func (fle *FieldLevelEncryption) EncryptWithKeyring(plaintext []byte) ([]byte, error) {
	fle.mu.RLock()
	defer fle.mu.RUnlock()

	ciphertext, err := fle.ring.Encrypt(plaintext, nil)
	if err != nil {
		fle.encryptionMetrics.FailureCount++
		return nil, err
	}

	fle.encryptionMetrics.TotalEncryptions++
	fle.encryptionMetrics.BytesEncrypted += int64(len(plaintext))
	return ciphertext, nil
}

// DecryptWithKeyring decrypts using the keyring, automatically
// selecting the correct key (active or retired) from the ciphertext prefix.
func (fle *FieldLevelEncryption) DecryptWithKeyring(ciphertext []byte) ([]byte, error) {
	fle.mu.RLock()
	defer fle.mu.RUnlock()

	plaintext, err := fle.ring.Decrypt(ciphertext, nil)
	if err != nil {
		fle.encryptionMetrics.FailureCount++
		return nil, err
	}

	fle.encryptionMetrics.TotalDecryptions++
	return plaintext, nil
}
