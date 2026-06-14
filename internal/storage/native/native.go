package native

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
)

// Backend is a self-contained, filesystem-based object storage engine.
// It needs no external service (no MinIO, no S3) â€” data lives on the
// local volume mounted into the container.
//
// Layout on disk:
//
//	<root>/
//	  <bucket>/
//	    .axiom/config.json                â€“ versioning, lifecycle, encryption, lock, CORS, notifications
//	    .axiom/tags.json                  â€“ bucket tags
//	    .axiom/policy.json                â€“ S3 bucket policy
//	    .axiom/objects/<key>.meta          â€“ per-object metadata (JSON)
//	    data/<key>                         â€“ the actual object bytes
type Backend struct {
	mu       sync.RWMutex
	root     string // absolute path of the storage root directory
	endpoint string // human-readable label (e.g. "native://data/storage")
	// Secret used for HMAC-signed presign tokens and encryption.
	presignSecret []byte
}

// New creates a native filesystem backend rooted at root.
// The directory is created if it does not exist.
func New(root string, presignSecret string) (*Backend, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("native storage: resolve root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("native storage: create root %q: %w", abs, err)
	}
	secret := []byte(presignSecret)
	if len(secret) == 0 {
		secret = []byte("axiom-native-storage-default-key")
	}
	logging.Z().Info(fmt.Sprintf("âœ… Storage: native filesystem backend at %s", abs))
	return &Backend{
		root:          abs,
		endpoint:      "native://" + abs,
		presignSecret: secret,
	}, nil
}

// ---------------------------------------------------------------------------
// helpers â€” paths
// ---------------------------------------------------------------------------

func (b *Backend) bucketDir(bucket string) string {
	return filepath.Join(b.root, bucket)
}

func (b *Backend) dataDir(bucket string) string {
	return filepath.Join(b.root, bucket, "data")
}

func (b *Backend) metaDir(bucket string) string {
	return filepath.Join(b.root, bucket, ".axiom")
}

func (b *Backend) objectMetaDir(bucket string) string {
	return filepath.Join(b.root, bucket, ".axiom", "objects")
}

func (b *Backend) objectPath(bucket, key string) string {
	return filepath.Join(b.dataDir(bucket), filepath.FromSlash(key))
}

func (b *Backend) objectMetaPath(bucket, key string) string {
	return filepath.Join(b.objectMetaDir(bucket), filepath.FromSlash(key)+".meta")
}

func (b *Backend) configPath(bucket string) string {
	return filepath.Join(b.metaDir(bucket), "config.json")
}

func (b *Backend) tagsPath(bucket string) string {
	return filepath.Join(b.metaDir(bucket), "tags.json")
}

func (b *Backend) policyPath(bucket string) string {
	return filepath.Join(b.metaDir(bucket), "policy.json")
}

// ---------------------------------------------------------------------------
// helpers â€” bucket config (persisted JSON)
// ---------------------------------------------------------------------------

// bucketConfig is persisted per-bucket.
type bucketConfig struct {
	Versioning    bool                             `json:"versioning"`
	Lifecycle     []models.LifecycleRule           `json:"lifecycle,omitempty"`
	Encryption    *models.BucketEncryption         `json:"encryption,omitempty"`
	ObjectLock    *models.ObjectLockConfig         `json:"objectLock,omitempty"`
	CORS          []models.CORSRule                `json:"cors,omitempty"`
	Notifications *models.BucketNotificationConfig `json:"notifications,omitempty"`
	Quota         int64                            `json:"quota,omitempty"` // bytes, 0 = unlimited
}

func (b *Backend) readConfig(bucket string) bucketConfig {
	data, err := os.ReadFile(b.configPath(bucket))
	if err != nil {
		return bucketConfig{}
	}
	var cfg bucketConfig
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func (b *Backend) writeConfig(bucket string, cfg bucketConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(b.configPath(bucket), data, 0o640)
}

// ---------------------------------------------------------------------------
// helpers â€” object metadata (persisted JSON per-object)
// ---------------------------------------------------------------------------

// objectMeta is persisted per-object alongside the data file.
type objectMeta struct {
	ContentType  string            `json:"contentType"`
	Size         int64             `json:"size"`
	ETag         string            `json:"etag"`
	LastModified time.Time         `json:"lastModified"`
	UserMetadata map[string]string `json:"userMetadata,omitempty"`
	Encrypted    bool              `json:"encrypted,omitempty"`
	StorageClass string            `json:"storageClass,omitempty"`
	RetainUntil  *time.Time        `json:"retainUntil,omitempty"`
	RetainMode   string            `json:"retainMode,omitempty"` // "GOVERNANCE" or "COMPLIANCE"
	LegalHold    bool              `json:"legalHold,omitempty"`
}

func (b *Backend) readObjectMeta(bucket, key string) (objectMeta, error) {
	data, err := os.ReadFile(b.objectMetaPath(bucket, key))
	if err != nil {
		return objectMeta{}, err
	}
	var m objectMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return objectMeta{}, err
	}
	return m, nil
}

func (b *Backend) writeObjectMeta(bucket, key string, m objectMeta) error {
	p := b.objectMetaPath(bucket, key)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o640)
}

func (b *Backend) deleteObjectMeta(bucket, key string) {
	_ = os.Remove(b.objectMetaPath(bucket, key))
}

// ---------------------------------------------------------------------------
// helpers â€” AES-256-GCM encryption
// ---------------------------------------------------------------------------

func (b *Backend) encryptionKey() []byte {
	h := sha256.Sum256(b.presignSecret)
	return h[:]
}

func encryptData(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func decryptData(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, ciphertext[:ns], ciphertext[ns:], nil)
}

// ---------------------------------------------------------------------------
// Backend interface â€” System
// ---------------------------------------------------------------------------

// Ping always succeeds â€” the filesystem is always reachable.
func (b *Backend) Ping(_ context.Context) error {
	if _, err := os.Stat(b.root); err != nil {
		return fmt.Errorf("native storage: root directory not accessible: %w", err)
	}
	return nil
}

// Endpoint returns a label for the native backend.
func (b *Backend) Endpoint() string {
	return b.endpoint
}

// ---------------------------------------------------------------------------
// Backend interface â€” Bucket Operations
// ---------------------------------------------------------------------------

func (b *Backend) CreateBucket(_ context.Context, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	dir := b.bucketDir(name)
	if _, err := os.Stat(dir); err == nil {
		return nil // idempotent
	}
	if err := os.MkdirAll(b.dataDir(name), 0o750); err != nil {
		return fmt.Errorf("native storage: create bucket %q: %w", name, err)
	}
	if err := os.MkdirAll(b.objectMetaDir(name), 0o750); err != nil {
		return fmt.Errorf("native storage: create bucket meta %q: %w", name, err)
	}
	if err := b.writeConfig(name, bucketConfig{}); err != nil {
		return fmt.Errorf("native storage: init bucket config %q: %w", name, err)
	}
	logging.Z().Info(fmt.Sprintf("âœ… Storage: bucket %q created (native)", name))
	return nil
}

func (b *Backend) DeleteBucket(_ context.Context, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	dir := b.bucketDir(name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("native storage: bucket %q does not exist", name)
	}

	// Check the bucket is empty (only metadata dirs remain).
	entries, err := os.ReadDir(b.dataDir(name))
	if err == nil && len(entries) > 0 {
		return fmt.Errorf("native storage: bucket %q is not empty (%d objects)", name, len(entries))
	}

	// Check object lock prevents deletion.
	cfg := b.readConfig(name)
	if cfg.ObjectLock != nil && cfg.ObjectLock.Enabled {
		return fmt.Errorf("native storage: bucket %q has object lock enabled; cannot delete", name)
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("native storage: remove bucket %q: %w", name, err)
	}
	logging.Z().Info(fmt.Sprintf("âœ… Storage: bucket %q deleted (native)", name))
	return nil
}

func (b *Backend) BucketExists(_ context.Context, name string) (bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, err := os.Stat(b.bucketDir(name))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (b *Backend) ListBuckets(_ context.Context) ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	entries, err := os.ReadDir(b.root)
	if err != nil {
		return nil, fmt.Errorf("native storage: list buckets: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// ---------------------------------------------------------------------------
// Backend interface â€” Versioning
// ---------------------------------------------------------------------------

func (b *Backend) SetBucketVersioning(_ context.Context, bucket string, enabled bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	cfg := b.readConfig(bucket)
	cfg.Versioning = enabled
	return b.writeConfig(bucket, cfg)
}

func (b *Backend) GetBucketVersioning(_ context.Context, bucket string) (bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cfg := b.readConfig(bucket)
	return cfg.Versioning, nil
}

// ---------------------------------------------------------------------------
// Backend interface â€” Lifecycle
// ---------------------------------------------------------------------------

func (b *Backend) SetBucketLifecycle(_ context.Context, bucket string, rules []models.LifecycleRule) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	cfg := b.readConfig(bucket)
	cfg.Lifecycle = rules
	return b.writeConfig(bucket, cfg)
}

func (b *Backend) GetBucketLifecycle(_ context.Context, bucket string) ([]models.LifecycleRule, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cfg := b.readConfig(bucket)
	return cfg.Lifecycle, nil
}

// ---------------------------------------------------------------------------
// Backend interface â€” Encryption
// ---------------------------------------------------------------------------

func (b *Backend) SetBucketEncryption(_ context.Context, bucket string, enc models.BucketEncryption) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	cfg := b.readConfig(bucket)
	cfg.Encryption = &enc
	return b.writeConfig(bucket, cfg)
}

func (b *Backend) GetBucketEncryption(_ context.Context, bucket string) (*models.BucketEncryption, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cfg := b.readConfig(bucket)
	return cfg.Encryption, nil
}

func (b *Backend) DeleteBucketEncryption(_ context.Context, bucket string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	cfg := b.readConfig(bucket)
	cfg.Encryption = nil
	return b.writeConfig(bucket, cfg)
}

// ---------------------------------------------------------------------------
// Backend interface â€” Object Lock / Retention
// ---------------------------------------------------------------------------

func (b *Backend) SetObjectLockConfig(_ context.Context, bucket string, lockCfg models.ObjectLockConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	cfg := b.readConfig(bucket)
	cfg.ObjectLock = &lockCfg
	return b.writeConfig(bucket, cfg)
}

func (b *Backend) GetObjectLockConfig(_ context.Context, bucket string) (*models.ObjectLockConfig, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cfg := b.readConfig(bucket)
	return cfg.ObjectLock, nil
}

func (b *Backend) PutObjectRetention(_ context.Context, bucket, key string, until time.Time, mode string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	m, err := b.readObjectMeta(bucket, key)
	if err != nil {
		return fmt.Errorf("native storage: retention: object %q/%q not found: %w", bucket, key, err)
	}
	// COMPLIANCE mode cannot be shortened.
	if m.RetainMode == "COMPLIANCE" && m.RetainUntil != nil && until.Before(*m.RetainUntil) {
		return fmt.Errorf("native storage: cannot shorten COMPLIANCE retention for %q/%q", bucket, key)
	}
	m.RetainUntil = &until
	m.RetainMode = mode
	return b.writeObjectMeta(bucket, key, m)
}

func (b *Backend) GetObjectRetention(_ context.Context, bucket, key string) (*time.Time, string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	m, err := b.readObjectMeta(bucket, key)
	if err != nil {
		return nil, "", fmt.Errorf("native storage: retention: object %q/%q not found: %w", bucket, key, err)
	}
	return m.RetainUntil, m.RetainMode, nil
}

func (b *Backend) PutObjectLegalHold(_ context.Context, bucket, key string, hold bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	m, err := b.readObjectMeta(bucket, key)
	if err != nil {
		return fmt.Errorf("native storage: legal hold: object %q/%q not found: %w", bucket, key, err)
	}
	m.LegalHold = hold
	return b.writeObjectMeta(bucket, key, m)
}

func (b *Backend) GetObjectLegalHold(_ context.Context, bucket, key string) (bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	m, err := b.readObjectMeta(bucket, key)
	if err != nil {
		return false, fmt.Errorf("native storage: legal hold: object %q/%q not found: %w", bucket, key, err)
	}
	return m.LegalHold, nil
}

// ---------------------------------------------------------------------------
// Backend interface â€” CORS
// ---------------------------------------------------------------------------

func (b *Backend) SetBucketCORS(_ context.Context, bucket string, rules []models.CORSRule) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	cfg := b.readConfig(bucket)
	cfg.CORS = rules
	return b.writeConfig(bucket, cfg)
}

func (b *Backend) GetBucketCORS(_ context.Context, bucket string) ([]models.CORSRule, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cfg := b.readConfig(bucket)
	return cfg.CORS, nil
}

func (b *Backend) DeleteBucketCORS(_ context.Context, bucket string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	cfg := b.readConfig(bucket)
	cfg.CORS = nil
	return b.writeConfig(bucket, cfg)
}

// ---------------------------------------------------------------------------
// Backend interface â€” Notifications
// ---------------------------------------------------------------------------

func (b *Backend) SetBucketNotification(_ context.Context, bucket string, ncfg models.BucketNotificationConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	cfg := b.readConfig(bucket)
	cfg.Notifications = &ncfg
	return b.writeConfig(bucket, cfg)
}

func (b *Backend) GetBucketNotification(_ context.Context, bucket string) (*models.BucketNotificationConfig, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cfg := b.readConfig(bucket)
	return cfg.Notifications, nil
}

// ---------------------------------------------------------------------------
// Backend interface â€” Bucket Policy
// ---------------------------------------------------------------------------

func (b *Backend) SetBucketPolicy(_ context.Context, bucket string, pol models.S3BucketPolicy) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	data, err := json.MarshalIndent(pol, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(b.policyPath(bucket), data, 0o640)
}

func (b *Backend) GetBucketPolicy(_ context.Context, bucket string) (*models.S3BucketPolicy, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	data, err := os.ReadFile(b.policyPath(bucket))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var pol models.S3BucketPolicy
	if err := json.Unmarshal(data, &pol); err != nil {
		return nil, err
	}
	return &pol, nil
}

func (b *Backend) DeleteBucketPolicy(_ context.Context, bucket string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := os.Remove(b.policyPath(bucket)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Backend interface â€” Object Operations
// ---------------------------------------------------------------------------

func (b *Backend) PutObject(_ context.Context, bucket, key string, data io.Reader, _ int64, contentType string) error {
	return b.putObjectInternal(bucket, key, data, contentType, nil)
}

func (b *Backend) PutObjectWithOptions(_ context.Context, bucket, key string, data io.Reader, _ int64, opts models.PutObjectOptions) error {
	ct := opts.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	return b.putObjectInternal(bucket, key, data, ct, &opts)
}

func (b *Backend) putObjectInternal(bucket, key string, data io.Reader, contentType string, opts *models.PutObjectOptions) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, err := os.Stat(b.bucketDir(bucket)); os.IsNotExist(err) {
		return fmt.Errorf("native storage: bucket %q does not exist", bucket)
	}

	// Enforce retention: if existing object is under retention, deny overwrite.
	if existing, err := b.readObjectMeta(bucket, key); err == nil {
		if existing.RetainUntil != nil && time.Now().Before(*existing.RetainUntil) {
			return fmt.Errorf("native storage: object %q/%q is under retention until %s", bucket, key, existing.RetainUntil.Format(time.RFC3339))
		}
		if existing.LegalHold {
			return fmt.Errorf("native storage: object %q/%q is under legal hold", bucket, key)
		}
	}

	// Enforce quota.
	cfg := b.readConfig(bucket)
	if cfg.Quota > 0 {
		currentSize := b.calcBucketSizeLocked(bucket)
		if currentSize >= cfg.Quota {
			return fmt.Errorf("native storage: bucket %q quota exceeded (%d / %d bytes)", bucket, currentSize, cfg.Quota)
		}
	}

	objPath := b.objectPath(bucket, key)
	if err := os.MkdirAll(filepath.Dir(objPath), 0o750); err != nil {
		return fmt.Errorf("native storage: create parent dirs for %q/%q: %w", bucket, key, err)
	}

	// Read all data into memory for hashing + optional encryption.
	raw, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("native storage: read data %q/%q: %w", bucket, key, err)
	}

	hasher := md5.New()
	hasher.Write(raw)
	etag := hex.EncodeToString(hasher.Sum(nil))

	// Encrypt if bucket encryption is enabled or per-object encryption requested.
	shouldEncrypt := false
	if cfg.Encryption != nil && cfg.Encryption.Enabled {
		shouldEncrypt = true
	}
	if opts != nil && opts.Encryption {
		shouldEncrypt = true
	}

	toWrite := raw
	if shouldEncrypt {
		encrypted, encErr := encryptData(b.encryptionKey(), raw)
		if encErr != nil {
			return fmt.Errorf("native storage: encrypt %q/%q: %w", bucket, key, encErr)
		}
		toWrite = encrypted
	}

	if err := os.WriteFile(objPath, toWrite, 0o640); err != nil {
		return fmt.Errorf("native storage: write %q/%q: %w", bucket, key, err)
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	meta := objectMeta{
		ContentType:  contentType,
		Size:         int64(len(raw)), // original size
		ETag:         etag,
		LastModified: time.Now().UTC(),
		Encrypted:    shouldEncrypt,
	}
	if opts != nil {
		meta.UserMetadata = opts.UserMetadata
		meta.StorageClass = opts.StorageClass
		meta.LegalHold = opts.LegalHold
		if opts.RetainUntil != nil {
			meta.RetainUntil = opts.RetainUntil
		}
	}
	if err := b.writeObjectMeta(bucket, key, meta); err != nil {
		return fmt.Errorf("native storage: write meta %q/%q: %w", bucket, key, err)
	}
	return nil
}

func (b *Backend) GetObject(_ context.Context, bucket, key string) (io.ReadCloser, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	objPath := b.objectPath(bucket, key)
	data, err := os.ReadFile(objPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("native storage: object %q/%q not found", bucket, key)
	}
	if err != nil {
		return nil, fmt.Errorf("native storage: open %q/%q: %w", bucket, key, err)
	}

	// Decrypt if object was encrypted.
	if m, mErr := b.readObjectMeta(bucket, key); mErr == nil && m.Encrypted {
		plain, dErr := decryptData(b.encryptionKey(), data)
		if dErr != nil {
			return nil, fmt.Errorf("native storage: decrypt %q/%q: %w", bucket, key, dErr)
		}
		data = plain
	}

	return io.NopCloser(strings.NewReader(string(data))), nil
}

func (b *Backend) DeleteObject(_ context.Context, bucket, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check retention / legal hold.
	if m, err := b.readObjectMeta(bucket, key); err == nil {
		if m.RetainUntil != nil && time.Now().Before(*m.RetainUntil) {
			return fmt.Errorf("native storage: object %q/%q is under retention until %s", bucket, key, m.RetainUntil.Format(time.RFC3339))
		}
		if m.LegalHold {
			return fmt.Errorf("native storage: object %q/%q is under legal hold", bucket, key)
		}
	}

	if err := os.Remove(b.objectPath(bucket, key)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("native storage: delete %q/%q: %w", bucket, key, err)
	}
	b.deleteObjectMeta(bucket, key)
	return nil
}

func (b *Backend) ListObjects(_ context.Context, bucket, prefix string) ([]models.ObjectInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	dataRoot := b.dataDir(bucket)
	if _, err := os.Stat(dataRoot); os.IsNotExist(err) {
		return nil, nil
	}

	var objects []models.ObjectInfo
	err := filepath.Walk(dataRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dataRoot, path)
		key := filepath.ToSlash(rel)
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}

		obj := models.ObjectInfo{
			Key:          key,
			Size:         info.Size(),
			LastModified: info.ModTime().UTC(),
			StorageClass: "STANDARD",
		}
		// Read cached meta for etag / content-type / user metadata.
		if m, mErr := b.readObjectMeta(bucket, key); mErr == nil {
			obj.ETag = m.ETag
			obj.ContentType = m.ContentType
			obj.Size = m.Size // original size (before encryption)
			obj.UserMetadata = m.UserMetadata
			obj.StorageClass = m.StorageClass
			obj.RetainUntil = m.RetainUntil
			obj.LegalHold = m.LegalHold
			if obj.StorageClass == "" {
				obj.StorageClass = "STANDARD"
			}
		}
		objects = append(objects, obj)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("native storage: walk %q: %w", bucket, err)
	}

	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

func (b *Backend) StatObject(_ context.Context, bucket, key string) (*models.ObjectInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	fi, err := os.Stat(b.objectPath(bucket, key))
	if err != nil {
		return nil, fmt.Errorf("native storage: stat %q/%q: %w", bucket, key, err)
	}

	obj := &models.ObjectInfo{
		Key:          key,
		Size:         fi.Size(),
		LastModified: fi.ModTime().UTC(),
		StorageClass: "STANDARD",
	}
	if m, mErr := b.readObjectMeta(bucket, key); mErr == nil {
		obj.ContentType = m.ContentType
		obj.ETag = m.ETag
		obj.Size = m.Size
		obj.UserMetadata = m.UserMetadata
		obj.StorageClass = m.StorageClass
		obj.RetainUntil = m.RetainUntil
		obj.LegalHold = m.LegalHold
		if obj.StorageClass == "" {
			obj.StorageClass = "STANDARD"
		}
	}
	return obj, nil
}

// ---------------------------------------------------------------------------
// Backend interface â€” Batch / Copy
// ---------------------------------------------------------------------------

func (b *Backend) MultiDeleteObjects(ctx context.Context, bucket string, keys []string) (int, []string, error) {
	var deleted int
	var errs []string
	for _, k := range keys {
		if err := b.DeleteObject(ctx, bucket, k); err != nil {
			errs = append(errs, k+": "+err.Error())
		} else {
			deleted++
		}
	}
	return deleted, errs, nil
}

func (b *Backend) CopyObject(_ context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	srcPath := b.objectPath(srcBucket, srcKey)
	dstPath := b.objectPath(dstBucket, dstKey)

	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("native storage: copy source %q/%q: %w", srcBucket, srcKey, err)
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o750); err != nil {
		return fmt.Errorf("native storage: copy dest dir %q/%q: %w", dstBucket, dstKey, err)
	}

	if err := os.WriteFile(dstPath, srcData, 0o640); err != nil {
		return fmt.Errorf("native storage: copy write %q/%q: %w", dstBucket, dstKey, err)
	}

	// Copy metadata, update timestamps.
	srcMeta, _ := b.readObjectMeta(srcBucket, srcKey)
	dstMeta := srcMeta
	dstMeta.LastModified = time.Now().UTC()

	// Recompute ETag if not encrypted.
	if !srcMeta.Encrypted {
		h := md5.New()
		h.Write(srcData)
		dstMeta.ETag = hex.EncodeToString(h.Sum(nil))
	}

	if err := b.writeObjectMeta(dstBucket, dstKey, dstMeta); err != nil {
		return fmt.Errorf("native storage: copy meta: %w", err)
	}

	logging.Z().Info(fmt.Sprintf("âœ… Storage: copied %s/%s â†’ %s/%s (native)", srcBucket, srcKey, dstBucket, dstKey))
	return nil
}

// ---------------------------------------------------------------------------
// Backend interface â€” Pre-signed URLs
// ---------------------------------------------------------------------------

func (b *Backend) presignToken(method, bucket, key string, expires time.Duration) string {
	exp := time.Now().UTC().Add(expires).Unix()
	payload := fmt.Sprintf("%s\n%s\n%s\n%d", method, bucket, key, exp)
	mac := hmac.New(sha256.New, b.presignSecret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%d:%s", exp, sig)
}

func (b *Backend) PresignGetObject(_ context.Context, bucket, key string, expires time.Duration) (string, error) {
	token := b.presignToken("GET", bucket, key, expires)
	u := url.URL{
		Path: fmt.Sprintf("/api/v1/storage/buckets/%s/objects/%s", bucket, key),
		RawQuery: url.Values{
			"X-Axiom-Expires": {fmt.Sprintf("%d", int(expires.Seconds()))},
			"X-Axiom-Token":   {token},
		}.Encode(),
	}
	return u.String(), nil
}

func (b *Backend) PresignPutObject(_ context.Context, bucket, key string, expires time.Duration) (string, error) {
	token := b.presignToken("PUT", bucket, key, expires)
	u := url.URL{
		Path: fmt.Sprintf("/api/v1/storage/buckets/%s/objects/%s", bucket, key),
		RawQuery: url.Values{
			"X-Axiom-Expires": {fmt.Sprintf("%d", int(expires.Seconds()))},
			"X-Axiom-Token":   {token},
		}.Encode(),
	}
	return u.String(), nil
}

// VerifyPresignToken validates an HMAC presign token. It is exported so the
// HTTP handler layer can call it when a presigned request arrives.
func (b *Backend) VerifyPresignToken(method, bucket, key, token string) bool {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 {
		return false
	}

	expStr := parts[0]
	sig := parts[1]

	var exp int64
	if _, err := fmt.Sscanf(expStr, "%d", &exp); err != nil {
		return false
	}
	if time.Now().UTC().Unix() > exp {
		return false // expired
	}

	payload := fmt.Sprintf("%s\n%s\n%s\n%d", method, bucket, key, exp)
	mac := hmac.New(sha256.New, b.presignSecret)
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

// ---------------------------------------------------------------------------
// Backend interface â€” Tagging
// ---------------------------------------------------------------------------

func (b *Backend) GetBucketTagging(_ context.Context, bucket string) ([]models.BucketTag, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	data, err := os.ReadFile(b.tagsPath(bucket))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("native storage: read tags %q: %w", bucket, err)
	}
	var tags []models.BucketTag
	if err := json.Unmarshal(data, &tags); err != nil {
		return nil, fmt.Errorf("native storage: parse tags %q: %w", bucket, err)
	}
	return tags, nil
}

func (b *Backend) PutBucketTagging(_ context.Context, bucket string, tags []models.BucketTag) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	data, err := json.MarshalIndent(tags, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(b.tagsPath(bucket), data, 0o640)
}

func (b *Backend) DeleteBucketTagging(_ context.Context, bucket string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := os.Remove(b.tagsPath(bucket)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("native storage: delete tags %q: %w", bucket, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Backend interface â€” Object Metadata
// ---------------------------------------------------------------------------

func (b *Backend) GetObjectMetadata(_ context.Context, bucket, key string) (map[string]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	m, err := b.readObjectMeta(bucket, key)
	if err != nil {
		return nil, fmt.Errorf("native storage: metadata %q/%q: %w", bucket, key, err)
	}
	return m.UserMetadata, nil
}

func (b *Backend) PutObjectMetadata(_ context.Context, bucket, key string, metadata map[string]string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	m, err := b.readObjectMeta(bucket, key)
	if err != nil {
		return fmt.Errorf("native storage: metadata %q/%q: %w", bucket, key, err)
	}
	m.UserMetadata = metadata
	return b.writeObjectMeta(bucket, key, m)
}

// ---------------------------------------------------------------------------
// Backend interface â€” Utility
// ---------------------------------------------------------------------------

func (b *Backend) GetBucketSize(ctx context.Context, bucket string) (int64, int64, error) {
	objects, err := b.ListObjects(ctx, bucket, "")
	if err != nil {
		return 0, 0, err
	}
	var totalSize int64
	for _, o := range objects {
		totalSize += o.Size
	}
	return totalSize, int64(len(objects)), nil
}

func (b *Backend) GetBucketQuota(ctx context.Context, bucket string) (*models.QuotaInfo, error) {
	b.mu.RLock()
	cfg := b.readConfig(bucket)
	b.mu.RUnlock()

	totalSize, objectCount, err := b.GetBucketSize(ctx, bucket)
	if err != nil {
		return nil, err
	}

	pct := float64(0)
	if cfg.Quota > 0 {
		pct = float64(totalSize) / float64(cfg.Quota) * 100
	}

	return &models.QuotaInfo{
		Bucket:      bucket,
		QuotaBytes:  cfg.Quota,
		UsedBytes:   totalSize,
		UsedPct:     pct,
		ObjectCount: objectCount,
	}, nil
}

// calcBucketSizeLocked returns total object bytes for a bucket.
// Caller must hold b.mu (at least read).
func (b *Backend) calcBucketSizeLocked(bucket string) int64 {
	dataRoot := b.dataDir(bucket)
	var total int64
	_ = filepath.Walk(dataRoot, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}
