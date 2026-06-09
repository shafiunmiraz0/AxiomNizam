// Package encryption — Phase 8: Auto field encryption.
//
// Provides transparent field-level encryption/decryption via struct tags.
// Fields tagged with `classification:"PII"` or `classification:"Sensitive"`
// are automatically encrypted on write and decrypted on read using AES-256-GCM.

package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"strings"
	"sync"
)

// Classification levels for struct tags.
const (
	ClassificationPII          = "PII"
	ClassificationSensitive    = "Sensitive"
	ClassificationConfidential = "Confidential"
)

// AutoEncryptor provides transparent field-level encryption based on struct tags.
type AutoEncryptor struct {
	keyFunc func() []byte // returns current 32-byte AES key
}

// NewAutoEncryptor creates a new auto-encryptor.
// keyFunc is called on each encrypt/decrypt to get the current key.
func NewAutoEncryptor(keyFunc func() []byte) *AutoEncryptor {
	return &AutoEncryptor{keyFunc: keyFunc}
}

// EncryptStruct encrypts all fields tagged with classification tags.
func (ae *AutoEncryptor) EncryptStruct(obj any) error {
	return ae.walkFields(obj, true)
}

// DecryptStruct decrypts all fields tagged with classification tags.
func (ae *AutoEncryptor) DecryptStruct(obj any) error {
	return ae.walkFields(obj, false)
}

func (ae *AutoEncryptor) walkFields(obj any, encrypt bool) error {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("auto-encrypt: expected pointer to struct, got %T", obj)
	}
	v = v.Elem()
	t := v.Type()

	for i := range t.NumField() {
		field := t.Field(i)
		fieldVal := v.Field(i)

		classification := field.Tag.Get("classification")
		if classification == "" {
			continue
		}
		if fieldVal.Kind() != reflect.String || !fieldVal.CanSet() {
			continue
		}

		val := fieldVal.String()
		if val == "" {
			continue
		}

		if encrypt {
			if strings.HasPrefix(val, "enc:v1:") {
				continue // already encrypted
			}
			encrypted, err := ae.encryptValue(val)
			if err != nil {
				return fmt.Errorf("encrypt field %s: %w", field.Name, err)
			}
			fieldVal.SetString(encrypted)
		} else {
			if !strings.HasPrefix(val, "enc:v1:") {
				continue // not encrypted
			}
			decrypted, err := ae.decryptValue(val)
			if err != nil {
				return fmt.Errorf("decrypt field %s: %w", field.Name, err)
			}
			fieldVal.SetString(decrypted)
		}
	}
	return nil
}

func (ae *AutoEncryptor) encryptValue(plaintext string) (string, error) {
	key := ae.keyFunc()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc:v1:" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (ae *AutoEncryptor) decryptValue(encoded string) (string, error) {
	key := ae.keyFunc()
	b64 := strings.TrimPrefix(encoded, "enc:v1:")
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("invalid base64: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// HasEncryptedFields returns true if any field is tagged with classification.
func HasEncryptedFields(obj any) bool {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	for i := range v.Type().NumField() {
		if v.Type().Field(i).Tag.Get("classification") != "" {
			return true
		}
	}
	return false
}

// ── Default key management ─────────────────────────────────────────────────

var (
	defaultKey     []byte
	defaultKeyOnce sync.Once
)

// GetDefaultKey returns the 32-byte AES key for auto-encryption.
// Priority: ENCRYPTION_AUTO_KEY env var (base64) > ENCRYPTION_MASTER_KEY env var (raw) > generate ephemeral key.
// The key is loaded once and cached for the process lifetime.
func GetDefaultKey() []byte {
	defaultKeyOnce.Do(func() {
		// 1. Try ENCRYPTION_AUTO_KEY (base64-encoded 32 bytes)
		if b64 := strings.TrimSpace(os.Getenv("ENCRYPTION_AUTO_KEY")); b64 != "" {
			if key, err := base64.StdEncoding.DecodeString(b64); err == nil && len(key) == 32 {
				defaultKey = key
				log.Println("✅ Auto-encryption key loaded from ENCRYPTION_AUTO_KEY")
				return
			}
		}

		// 2. Try ENCRYPTION_MASTER_KEY (raw string, hashed to 32 bytes)
		if raw := strings.TrimSpace(os.Getenv("ENCRYPTION_MASTER_KEY")); raw != "" {
			hash := sha256.Sum256([]byte(raw))
			defaultKey = hash[:]
			log.Println("✅ Auto-encryption key derived from ENCRYPTION_MASTER_KEY")
			return
		}

		// 3. Generate ephemeral key (dev only — data encrypted with this key is NOT restart-safe)
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			panic("encryption: failed to generate ephemeral key: " + err.Error())
		}
		defaultKey = key
		log.Println("⚠️  Auto-encryption using ephemeral key (set ENCRYPTION_AUTO_KEY for persistence)")
	})
	return defaultKey
}
