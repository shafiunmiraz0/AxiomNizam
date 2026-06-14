package access

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/iam/token"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/audit"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/store"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Controller manages IAM-integrated access control, access keys, and bucket sharing.
type Controller struct {
	mu         sync.RWMutex
	policies   map[string]*models.TenantPolicy // key = tenantID/userID/bucket
	accessKeys map[string]*models.AccessKey    // key = accessKeyID
	shares     map[string]*models.BucketShare  // key = shareID
	auditLog   *audit.AuditLog
	etcd       *clientv3.Client
	kvStore    platformstore.KVStore // Raft-compatible KV persistence (used when etcd is nil).
	etcdTimeout time.Duration
	rateMu     sync.Mutex
	rateMap    map[string]*objectRateState
	rateWindow time.Duration

	defaultReadRateLimit  int
	defaultWriteRateLimit int

	bucketStore *store.BucketStore
}

// ControllerConfig holds configuration for the access controller.
type ControllerConfig struct {
	DefaultReadRateLimit  int
	DefaultWriteRateLimit int
	EtcdTimeout           time.Duration
}

const (
	accessPolicyPrefix = "storage:access:policies/"
	accessKeyPrefix    = "storage:access:keys/"
	accessSharePrefix  = "storage:access:shares/"
)

type objectRateState struct {
	windowStart time.Time
	count       int
}

// NewController creates an access controller.
func NewController(auditLog *audit.AuditLog, cfg ControllerConfig) *Controller {
	etcdTimeout := cfg.EtcdTimeout
	if etcdTimeout <= 0 {
		etcdTimeout = 3 * time.Second
	}
	return &Controller{
		policies:              make(map[string]*models.TenantPolicy),
		accessKeys:            make(map[string]*models.AccessKey),
		shares:                make(map[string]*models.BucketShare),
		auditLog:              auditLog,
		rateMap:               make(map[string]*objectRateState),
		defaultReadRateLimit:  cfg.DefaultReadRateLimit,
		defaultWriteRateLimit: cfg.DefaultWriteRateLimit,
		rateWindow:            time.Minute,
		etcdTimeout:           etcdTimeout,
	}
}

// SetBucketStore attaches the bucket store so the controller can resolve
// bucket-specific settings such as object rate limits.
func (ac *Controller) SetBucketStore(bs *store.BucketStore) {
	ac.mu.Lock()
	ac.bucketStore = bs
	ac.mu.Unlock()
}

// DefaultObjectRateLimits returns global defaults used when bucket-specific
// rate limits are not configured.
func (ac *Controller) DefaultObjectRateLimits() (int, int) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.defaultReadRateLimit, ac.defaultWriteRateLimit
}

// EffectiveBucketObjectRateLimits returns the effective read/write object
// rate limits for a bucket.
func (ac *Controller) EffectiveBucketObjectRateLimits(tenantID, bucket string) (int, int) {
	ac.mu.RLock()
	storeRef := ac.bucketStore
	defaultRead := ac.defaultReadRateLimit
	defaultWrite := ac.defaultWriteRateLimit
	ac.mu.RUnlock()

	if storeRef == nil || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(bucket) == "" {
		return defaultRead, defaultWrite
	}

	b, err := storeRef.Get(strings.TrimSpace(tenantID), strings.TrimSpace(bucket))
	if err != nil || b == nil {
		return defaultRead, defaultWrite
	}

	read := defaultRead
	if b.Spec.ReadOpsPerMinute > 0 {
		read = b.Spec.ReadOpsPerMinute
	}

	write := defaultWrite
	if b.Spec.WriteOpsPerMinute > 0 {
		write = b.Spec.WriteOpsPerMinute
	}

	return read, write
}

// ConfigurePersistence enables etcd-backed persistence for policies, access keys,
// and bucket shares. Existing state is loaded from etcd when configured.
func (ac *Controller) ConfigurePersistence(etcd *clientv3.Client) {
	ac.mu.Lock()
	ac.etcd = etcd
	ac.mu.Unlock()
	ac.loadFromEtcd()
	ac.mu.Lock()
	ac.persistAllUnlocked()
	ac.mu.Unlock()
}

// ConfigureKVPersistence enables KVStore-backed persistence for access controller
// resources. This is used in Raft mode where etcd is not available.
func (ac *Controller) ConfigureKVPersistence(kv platformstore.KVStore) {
	ac.mu.Lock()
	ac.kvStore = kv
	ac.mu.Unlock()
	ac.loadFromKVStore()
	ac.mu.Lock()
	ac.persistAllUnlocked()
	ac.mu.Unlock()
}

// ---------------------------------------------------------------------------
// IAM Middleware — Extract user context from JWT
// ---------------------------------------------------------------------------

// StorageContext holds the authenticated user information extracted from JWT.
type StorageContext struct {
	UserID   string
	Email    string
	TenantID string
	Roles    []string
}

// GetStorageContext extracts the storage context from a Gin context.
// Requires IAM JWTAuth middleware to have run first.
func GetStorageContext(c *gin.Context) *StorageContext {
	v, exists := c.Get("storage_context")
	if !exists {
		return nil
	}
	sc, _ := v.(*StorageContext)
	return sc
}

// RequireStorageAuth is middleware that extracts the IAM user context and
// maps it to a storage-level identity. It also supports access key authentication.
func (ac *Controller) RequireStorageAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Option 1: Check for storage access key in X-Storage-Access-Key header.
		accessKeyID := c.GetHeader("X-Storage-Access-Key")
		secretKey := c.GetHeader("X-Storage-Secret-Key")
		if accessKeyID != "" && secretKey != "" {
			ak := ac.ValidateAccessKey(accessKeyID, secretKey)
			if ak == nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid storage access key"})
				return
			}
			sc := &StorageContext{
				UserID:   ak.UserID,
				TenantID: ak.TenantID,
				Roles:    []string{string(ak.Role)},
			}
			c.Set("storage_context", sc)
			c.Set("storage_access_key", ak)
			c.Next()
			return
		}

		// Option 2: Use IAM JWT claims from IAM middleware.
		claimsRaw, exists := c.Get("iam_claims")
		if !exists {
			// Option 3: allow requests already validated as presigned by the outer storage middleware.
			if c.GetBool("storage_presigned_request") {
				c.Next()
				return
			}

			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "authentication required",
				"details": "provide IAM JWT token or storage access key",
			})
			return
		}

		claims, ok := claimsRaw.(*token.IAMClaims)
		if !ok || claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authentication claims"})
			return
		}

		// Derive tenant ID: use the user's sub (user ID) as default tenant.
		// Applications may override with X-Storage-Tenant header.
		tenantID := claims.Sub
		if override := strings.TrimSpace(c.GetHeader("X-Storage-Tenant")); override != "" {
			// Only sysadmin/admin can impersonate tenants.
			if hasRole(claims.Roles, "sysadmin", "system-manager", "admin") {
				tenantID = override
			}
		}

		sc := &StorageContext{
			UserID:   claims.Sub,
			Email:    claims.Email,
			TenantID: tenantID,
			Roles:    claims.Roles,
		}
		c.Set("storage_context", sc)
		c.Next()
	}
}

// RequireStorageRole checks that the authenticated user holds one of the specified
// storage roles (either via IAM role mapping or direct storage policy).
func (ac *Controller) RequireStorageRole(allowed ...models.StorageRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		sc := GetStorageContext(c)
		if sc == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		// Sysadmin / system-manager bypass all storage role checks.
		if hasRole(sc.Roles, "sysadmin", "system-manager") {
			c.Next()
			return
		}

		// Map IAM roles to storage roles and check.
		for _, iamRole := range sc.Roles {
			storageRole := MapIAMRoleToStorageRole(iamRole)
			for _, a := range allowed {
				if storageRole == a {
					c.Next()
					return
				}
			}
		}

		// Check direct storage access key role.
		for _, r := range sc.Roles {
			for _, a := range allowed {
				if models.StorageRole(r) == a {
					c.Next()
					return
				}
			}
		}

		// Check bucket-specific policies.
		bucket := c.Param("bucket")
		if bucket != "" {
			for _, a := range allowed {
				if ac.HasBucketAccess(sc.UserID, sc.TenantID, bucket, a) {
					c.Next()
					return
				}
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":    "insufficient storage permissions",
			"required": allowed,
		})
	}
}

// RequireBucketAccess checks that the authenticated user can access the specific
// bucket named in the :bucket path parameter. Checks ownership, shares, and policies.
func (ac *Controller) RequireBucketAccess(minRole models.StorageRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		sc := GetStorageContext(c)
		if sc == nil {
			if c.GetBool("storage_presigned_request") {
				if !ac.enforceObjectRateLimit(c, nil) {
					return
				}
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		if !ac.enforceObjectRateLimit(c, sc) {
			return
		}

		// Sysadmin bypass.
		if hasRole(sc.Roles, "sysadmin", "system-manager") {
			c.Next()
			return
		}

		bucket := c.Param("bucket")
		if bucket == "" {
			c.Next()
			return
		}

		// Check if access key has bucket scope.
		if akRaw, exists := c.Get("storage_access_key"); exists {
			ak := akRaw.(*models.AccessKey)
			if len(ak.BucketScope) > 0 {
				found := false
				for _, b := range ak.BucketScope {
					if bucketScopeMatches(b, bucket) {
						found = true
						break
					}
				}
				if !found {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
						"error":  "access key does not have access to this bucket",
						"bucket": bucket,
					})
					return
				}
			}
		}

		// Check direct IAM role mapping.
		for _, iamRole := range sc.Roles {
			sr := MapIAMRoleToStorageRole(iamRole)
			if roleAtLeast(sr, minRole) {
				c.Next()
				return
			}
		}

		// Check bucket-level policy or share.
		if ac.HasBucketAccess(sc.UserID, sc.TenantID, bucket, minRole) {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":    "insufficient permissions for bucket",
			"bucket":   bucket,
			"required": minRole,
		})
	}
}

// ---------------------------------------------------------------------------
// IAM-to-Storage Role Mapping
// ---------------------------------------------------------------------------

// MapIAMRoleToStorageRole maps an IAM role to the equivalent storage role.
func MapIAMRoleToStorageRole(iamRole string) models.StorageRole {
	switch strings.ToLower(strings.TrimSpace(iamRole)) {
	case "sysadmin", "system-manager", "system_admin", "system-admin":
		return models.StorageRoleAdmin
	case "admin":
		return models.StorageRoleAdmin
	case "manager":
		return models.StorageRoleWriter
	case "user":
		return models.StorageRoleReader
	case "uploader":
		return models.StorageRoleUploader
	default:
		return models.StorageRoleReader
	}
}

// roleAtLeast checks if `have` is at least as permissive as `need`.
func roleAtLeast(have, need models.StorageRole) bool {
	order := map[models.StorageRole]int{
		models.StorageRoleAdmin:    4,
		models.StorageRoleWriter:   3,
		models.StorageRoleBrowser:  2,
		models.StorageRoleReader:   1,
		models.StorageRoleUploader: 1,
	}
	return order[have] >= order[need]
}

func hasRole(roles []string, targets ...string) bool {
	for _, r := range roles {
		nr := strings.ToLower(strings.TrimSpace(r))
		for _, t := range targets {
			if nr == t {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Access Keys (Service Accounts for Applications)
// ---------------------------------------------------------------------------

// CreateAccessKey generates a new storage access key bound to a user.
func (ac *Controller) CreateAccessKey(userID, tenantID, name, description string, role models.StorageRole, bucketScope []string, expiresAt *time.Time) (*models.AccessKey, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	accessKeyID := "AXAK" + generateRandomHex(16)
	secretAccessKey := generateRandomHex(32)

	ak := &models.AccessKey{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Name:            name,
		Description:     description,
		UserID:          userID,
		TenantID:        tenantID,
		Role:            role,
		BucketScope:     bucketScope,
		Active:          true,
		ExpiresAt:       expiresAt,
		CreatedAt:       time.Now().UTC(),
	}

	ac.accessKeys[accessKeyID] = ak
	ac.persistAccessKeyUnlocked(ak)

	if ac.auditLog != nil {
		ac.auditLog.Record(models.StorageEvent{
			Type:     "accesskey.created",
			TenantID: tenantID,
			UserID:   userID,
			Details:  fmt.Sprintf("access key %s created for %s (role: %s)", accessKeyID, name, role),
		})
	}

	return ak, nil
}

// ValidateAccessKey verifies an access key ID + secret pair.
func (ac *Controller) ValidateAccessKey(accessKeyID, secretKey string) *models.AccessKey {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	ak, exists := ac.accessKeys[accessKeyID]
	if !exists || !ak.Active {
		return nil
	}

	// Check expiration.
	if ak.ExpiresAt != nil && time.Now().After(*ak.ExpiresAt) {
		return nil
	}

	// Verify secret using HMAC comparison.
	if !hmac.Equal([]byte(ak.SecretAccessKey), []byte(secretKey)) {
		return nil
	}

	// Update last used.
	now := time.Now().UTC()
	ak.LastUsedAt = &now

	return ak
}

// ResolveAccessKey returns an active, non-expired access key by ID.
// The returned key is a copy and includes SecretAccessKey for internal signing.
func (ac *Controller) ResolveAccessKey(accessKeyID string) (*models.AccessKey, error) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	ak, exists := ac.accessKeys[accessKeyID]
	if !exists {
		return nil, fmt.Errorf("access key %q not found", accessKeyID)
	}
	if !ak.Active {
		return nil, fmt.Errorf("access key %q is inactive", accessKeyID)
	}
	if ak.ExpiresAt != nil && time.Now().After(*ak.ExpiresAt) {
		return nil, fmt.Errorf("access key %q is expired", accessKeyID)
	}
	copy := *ak
	return &copy, nil
}

// ResolveUserAccessKeyForPresign returns a user's active key for signing presigned URLs.
// If preferredAccessKeyID is provided, it must belong to that user+tenant.
func (ac *Controller) ResolveUserAccessKeyForPresign(userID, tenantID, preferredAccessKeyID string) (*models.AccessKey, error) {
	if strings.TrimSpace(preferredAccessKeyID) != "" {
		ak, err := ac.ResolveAccessKey(strings.TrimSpace(preferredAccessKeyID))
		if err != nil {
			return nil, err
		}
		if ak.UserID != userID || ak.TenantID != tenantID {
			return nil, fmt.Errorf("access key %q does not belong to user", preferredAccessKeyID)
		}
		return ak, nil
	}

	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var selected *models.AccessKey
	for _, ak := range ac.accessKeys {
		if ak == nil || !ak.Active {
			continue
		}
		if ak.UserID != userID || ak.TenantID != tenantID {
			continue
		}
		if ak.ExpiresAt != nil && time.Now().After(*ak.ExpiresAt) {
			continue
		}
		if selected == nil || ak.CreatedAt.After(selected.CreatedAt) {
			copy := *ak
			selected = &copy
		}
	}

	if selected == nil {
		return nil, fmt.Errorf("no active access key found for user %q in tenant %q", userID, tenantID)
	}
	return selected, nil
}

// ValidateAccessKeyForObjectRequest enforces method and scope constraints for an access key.
func ValidateAccessKeyForObjectRequest(ak *models.AccessKey, method, bucket, key string) error {
	if ak == nil {
		return fmt.Errorf("access key is required")
	}
	if !ak.Active {
		return fmt.Errorf("access key is inactive")
	}
	if ak.ExpiresAt != nil && time.Now().After(*ak.ExpiresAt) {
		return fmt.Errorf("access key is expired")
	}

	bucket = strings.TrimSpace(bucket)
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	method = strings.ToUpper(strings.TrimSpace(method))

	if bucket == "" || key == "" {
		return fmt.Errorf("bucket and object key are required")
	}
	if strings.Contains(bucket, "*") || strings.Contains(key, "*") {
		return fmt.Errorf("wildcard bucket/key is not allowed")
	}

	allowedMethod := false
	switch method {
	case http.MethodGet:
		allowedMethod = roleAtLeast(ak.Role, models.StorageRoleReader) || ak.Role == models.StorageRoleBrowser
	case http.MethodPut:
		allowedMethod = roleAtLeast(ak.Role, models.StorageRoleWriter) || ak.Role == models.StorageRoleUploader
	default:
		return fmt.Errorf("method %s is not allowed for presigned access", method)
	}
	if !allowedMethod {
		return fmt.Errorf("access key role %s does not allow %s", ak.Role, method)
	}

	if len(ak.BucketScope) > 0 {
		ok := false
		for _, b := range ak.BucketScope {
			if bucketScopeMatches(b, bucket) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("access key is not allowed for bucket %s", bucket)
		}
	}

	prefix := strings.TrimSpace(ak.PrefixScope)
	if prefix != "" {
		if strings.Contains(prefix, "*") {
			return fmt.Errorf("wildcard prefix scope is not allowed")
		}
		if key != prefix && !strings.HasPrefix(key, prefix) {
			return fmt.Errorf("object key is outside access key prefix scope")
		}
	}

	return nil
}

func bucketScopeMatches(scopeEntry, bucket string) bool {
	scope := strings.ToLower(strings.TrimSpace(scopeEntry))
	b := strings.ToLower(strings.TrimSpace(bucket))
	if scope == "" || b == "" {
		return false
	}
	if scope == "*" || scope == b {
		return true
	}

	// Backward/forward compatibility:
	// - logical scope (e.g. "m") should match storage bucket "<prefix><tenant>-m"
	// - storage scope should match logical API path bucket when it ends with "-<logical>"
	if strings.HasSuffix(b, "-"+scope) {
		return true
	}
	if strings.HasSuffix(scope, "-"+b) {
		return true
	}
	return false
}

// ListAccessKeys returns all access keys for a user.
func (ac *Controller) ListAccessKeys(userID, tenantID string) []*models.AccessKey {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var result []*models.AccessKey
	for _, ak := range ac.accessKeys {
		if (userID == "" || ak.UserID == userID) && (tenantID == "" || ak.TenantID == tenantID) {
			// Never return the secret key in list operations.
			safe := *ak
			safe.SecretAccessKey = ""
			result = append(result, &safe)
		}
	}
	return result
}

// RevokeAccessKey deactivates an access key.
func (ac *Controller) RevokeAccessKey(accessKeyID, userID string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ak, exists := ac.accessKeys[accessKeyID]
	if !exists {
		return fmt.Errorf("access key %q not found", accessKeyID)
	}
	// Users can only revoke their own keys; admins bypass via the handler.
	if ak.UserID != userID && userID != "" {
		return fmt.Errorf("access key %q does not belong to user %q", accessKeyID, userID)
	}

	ak.Active = false
	ac.persistAccessKeyUnlocked(ak)

	if ac.auditLog != nil {
		ac.auditLog.Record(models.StorageEvent{
			Type:     "accesskey.revoked",
			TenantID: ak.TenantID,
			UserID:   userID,
			Details:  fmt.Sprintf("access key %s revoked", accessKeyID),
		})
	}

	return nil
}

// DeleteAccessKey permanently removes an access key.
func (ac *Controller) DeleteAccessKey(accessKeyID, userID string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ak, exists := ac.accessKeys[accessKeyID]
	if !exists {
		return fmt.Errorf("access key %q not found", accessKeyID)
	}
	if ak.UserID != userID && userID != "" {
		return fmt.Errorf("access key %q does not belong to user %q", accessKeyID, userID)
	}

	delete(ac.accessKeys, accessKeyID)
	ac.deleteEtcdKey(accessKeyPrefix + accessKeyID)
	return nil
}

// ActiveAccessKeyCount returns the number of active access keys.
func (ac *Controller) ActiveAccessKeyCount() int {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	count := 0
	for _, ak := range ac.accessKeys {
		if ak.Active {
			count++
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// Bucket Sharing
// ---------------------------------------------------------------------------

// ShareBucket creates a share granting access to a bucket.
func (ac *Controller) ShareBucket(share models.BucketShare) (*models.BucketShare, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	share.ID = uuid.New().String()
	share.SharedAt = time.Now().UTC()
	share.Active = true

	ac.shares[share.ID] = &share
	ac.persistShareUnlocked(&share)

	if ac.auditLog != nil {
		ac.auditLog.Record(models.StorageEvent{
			Type:     "bucket.shared",
			TenantID: share.TenantID,
			Bucket:   share.BucketName,
			UserID:   share.SharedBy,
			Details:  fmt.Sprintf("shared with %s (%s) as %s", share.GranteeID, share.GranteeType, share.Role),
		})
	}

	return &share, nil
}

// ListBucketShares returns all shares for a bucket.
func (ac *Controller) ListBucketShares(tenantID, bucket string) []*models.BucketShare {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var result []*models.BucketShare
	for _, s := range ac.shares {
		if (tenantID == "" || s.TenantID == tenantID) && (bucket == "" || s.BucketName == bucket) {
			result = append(result, s)
		}
	}
	return result
}

// ListUserShares returns all shares granted to a specific user.
func (ac *Controller) ListUserShares(userID string) []*models.BucketShare {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var result []*models.BucketShare
	for _, s := range ac.shares {
		if s.GranteeID == userID && s.Active {
			result = append(result, s)
		}
	}
	return result
}

// RevokeShare deactivates a bucket share.
func (ac *Controller) RevokeShare(shareID, userID string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	s, exists := ac.shares[shareID]
	if !exists {
		return fmt.Errorf("share %q not found", shareID)
	}
	s.Active = false
	ac.persistShareUnlocked(s)

	if ac.auditLog != nil {
		ac.auditLog.Record(models.StorageEvent{
			Type:     "bucket.share.revoked",
			TenantID: s.TenantID,
			Bucket:   s.BucketName,
			UserID:   userID,
			Details:  fmt.Sprintf("share %s revoked for %s", shareID, s.GranteeID),
		})
	}

	return nil
}

// DeleteShare permanently removes a share.
func (ac *Controller) DeleteShare(shareID string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if _, exists := ac.shares[shareID]; !exists {
		return fmt.Errorf("share %q not found", shareID)
	}
	delete(ac.shares, shareID)
	ac.deleteEtcdKey(accessSharePrefix + shareID)
	return nil
}

// ActiveShareCount returns the number of active shares.
func (ac *Controller) ActiveShareCount() int {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	count := 0
	for _, s := range ac.shares {
		if s.Active {
			count++
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// Bucket Access Policies
// ---------------------------------------------------------------------------

// SetPolicy creates or updates a bucket access policy.
func (ac *Controller) SetPolicy(p models.TenantPolicy) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	key := policyKey(p.TenantID, p.UserID, p.BucketName)
	p.GrantedAt = time.Now().UTC()
	ac.policies[key] = &p
	ac.persistPolicyUnlocked(&p)
	return nil
}

// GetPolicy returns a specific policy.
func (ac *Controller) GetPolicy(tenantID, userID, bucket string) *models.TenantPolicy {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.policies[policyKey(tenantID, userID, bucket)]
}

// ListPolicies returns all policies, optionally filtered.
func (ac *Controller) ListPolicies(tenantID string) []*models.TenantPolicy {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var result []*models.TenantPolicy
	for _, p := range ac.policies {
		if tenantID == "" || p.TenantID == tenantID {
			result = append(result, p)
		}
	}
	return result
}

// DeletePolicy removes a policy.
func (ac *Controller) DeletePolicy(tenantID, userID, bucket string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	key := policyKey(tenantID, userID, bucket)
	if _, exists := ac.policies[key]; !exists {
		return fmt.Errorf("policy not found")
	}
	delete(ac.policies, key)
	ac.deleteEtcdKey(accessPolicyPrefix + key)
	return nil
}

// PolicyCount returns total policies.
func (ac *Controller) PolicyCount() int {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return len(ac.policies)
}

// HasBucketAccess checks if a user has at least the given role on a bucket
// through policies or shares.
func (ac *Controller) HasBucketAccess(userID, tenantID, bucket string, minRole models.StorageRole) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	// Check direct policy.
	if p := ac.policies[policyKey(tenantID, userID, bucket)]; p != nil {
		if roleAtLeast(p.Role, minRole) {
			if p.ExpiresAt == nil || time.Now().Before(*p.ExpiresAt) {
				return true
			}
		}
	}

	// Check wildcard policy (all buckets).
	if p := ac.policies[policyKey(tenantID, userID, "*")]; p != nil {
		if roleAtLeast(p.Role, minRole) {
			if p.ExpiresAt == nil || time.Now().Before(*p.ExpiresAt) {
				return true
			}
		}
	}

	// Check bucket shares.
	for _, s := range ac.shares {
		if s.GranteeID == userID && s.BucketName == bucket && s.Active {
			if roleAtLeast(s.Role, minRole) {
				if s.ExpiresAt == nil || time.Now().Before(*s.ExpiresAt) {
					return true
				}
			}
		}
	}

	return false
}

func policyKey(tenantID, userID, bucket string) string {
	return tenantID + "/" + userID + "/" + bucket
}

func generateRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (ac *Controller) enforceObjectRateLimit(c *gin.Context, sc *StorageContext) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return true
	}

	path := c.Request.URL.Path
	if !strings.Contains(path, "/storage/buckets/") {
		return true
	}
	if !strings.Contains(path, "/objects") && !strings.Contains(path, "/multi-delete") && !strings.Contains(path, "/object-metadata") {
		return true
	}

	bucket := strings.TrimSpace(c.Param("bucket"))
	if bucket == "" {
		bucket = "-"
	}

	tenantID := ""
	if sc != nil {
		tenantID = strings.TrimSpace(sc.TenantID)
	}
	if tenantID == "" {
		tenantID = strings.TrimSpace(c.GetString("storage_presigned_tenant"))
	}

	method := strings.ToUpper(strings.TrimSpace(c.Request.Method))
	opClass := "read"
	if method == http.MethodPut || method == http.MethodPost || method == http.MethodDelete || method == http.MethodPatch {
		opClass = "write"
	}

	readLimit, writeLimit := ac.EffectiveBucketObjectRateLimits(tenantID, bucket)
	limit := readLimit
	if opClass == "write" {
		limit = writeLimit
	}
	if limit <= 0 {
		limit = ac.defaultReadRateLimit
		if limit <= 0 {
			limit = 240
		}
	}

	rateKey := strings.Join([]string{tenantID, bucket, opClass}, "|")

	now := time.Now().UTC()
	ac.rateMu.Lock()
	state := ac.rateMap[rateKey]
	if state == nil || now.Sub(state.windowStart) >= ac.rateWindow {
		state = &objectRateState{windowStart: now, count: 0}
		ac.rateMap[rateKey] = state
	}
	state.count++
	count := state.count
	resetAt := state.windowStart.Add(ac.rateWindow)
	ac.rateMu.Unlock()

	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

	if count > limit {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error":      "bucket object rate limit exceeded",
			"limit":      limit,
			"bucket":     bucket,
			"operation":  opClass,
			"retryAfter": int(time.Until(resetAt).Seconds()),
			"tenantId":   tenantID,
		})
		return false
	}

	return true
}

func (ac *Controller) loadFromEtcd() {
	etcd := ac.etcd
	if etcd == nil {
		return
	}

	load := func(prefix string, fn func([]byte)) {
		ctx, cancel := context.WithTimeout(context.Background(), ac.etcdTimeout)
		defer cancel()
		resp, err := etcd.Get(ctx, prefix, clientv3.WithPrefix())
		if err != nil {
			logging.Z().Info(fmt.Sprintf("storage access: etcd load failed for prefix %s: %v", prefix, err))
			return
		}
		for _, kv := range resp.Kvs {
			fn(kv.Value)
		}
	}

	load(accessPolicyPrefix, func(v []byte) {
		var p models.TenantPolicy
		if err := json.Unmarshal(v, &p); err != nil {
			return
		}
		ac.mu.Lock()
		cp := p
		ac.policies[policyKey(cp.TenantID, cp.UserID, cp.BucketName)] = &cp
		ac.mu.Unlock()
	})

	load(accessKeyPrefix, func(v []byte) {
		var k models.AccessKey
		if err := json.Unmarshal(v, &k); err != nil {
			return
		}
		ac.mu.Lock()
		ck := k
		ac.accessKeys[ck.AccessKeyID] = &ck
		ac.mu.Unlock()
	})

	load(accessSharePrefix, func(v []byte) {
		var s models.BucketShare
		if err := json.Unmarshal(v, &s); err != nil {
			return
		}
		ac.mu.Lock()
		cs := s
		ac.shares[cs.ID] = &cs
		ac.mu.Unlock()
	})
}

func (ac *Controller) putEtcdJSON(key string, value interface{}) {
	data, err := json.Marshal(value)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("storage access: marshal failed for key %s: %v", key, err))
		return
	}

	// Prefer etcd, fall back to KVStore.
	if ac.etcd != nil {
		ctx, cancel := context.WithTimeout(context.Background(), ac.etcdTimeout)
		defer cancel()
		if _, err := ac.etcd.Put(ctx, key, string(data)); err != nil {
			logging.Z().Info(fmt.Sprintf("storage access: etcd put failed for key %s: %v", key, err))
		}
		return
	}
	if ac.kvStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), ac.etcdTimeout)
		defer cancel()
		if err := ac.kvStore.Put(ctx, key, string(data)); err != nil {
			logging.Z().Info(fmt.Sprintf("storage access: kvstore put failed for key %s: %v", key, err))
		}
	}
}

func (ac *Controller) deleteEtcdKey(key string) {
	// Prefer etcd, fall back to KVStore.
	if ac.etcd != nil {
		ctx, cancel := context.WithTimeout(context.Background(), ac.etcdTimeout)
		defer cancel()
		if _, err := ac.etcd.Delete(ctx, key); err != nil {
			logging.Z().Info(fmt.Sprintf("storage access: etcd delete failed for key %s: %v", key, err))
		}
		return
	}
	if ac.kvStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), ac.etcdTimeout)
		defer cancel()
		if err := ac.kvStore.Delete(ctx, key); err != nil {
			logging.Z().Info(fmt.Sprintf("storage access: kvstore delete failed for key %s: %v", key, err))
		}
	}
}

func (ac *Controller) loadFromKVStore() {
	ac.mu.RLock()
	kv := ac.kvStore
	ac.mu.RUnlock()
	if kv == nil {
		return
	}

	loadKV := func(prefix string, fn func(string)) {
		ctx, cancel := context.WithTimeout(context.Background(), ac.etcdTimeout)
		defer cancel()
		entries, err := kv.List(ctx, prefix)
		if err != nil {
			logging.Z().Info(fmt.Sprintf("storage access: kvstore load failed for prefix %s: %v", prefix, err))
			return
		}
		for _, val := range entries {
			fn(val)
		}
	}

	logging.Z().Info(fmt.Sprintf("storage access: starting load from KVStore"))
	loadKV(accessPolicyPrefix, func(v string) {
		var p models.TenantPolicy
		if err := json.Unmarshal([]byte(v), &p); err != nil {
			return
		}
		ac.mu.Lock()
		cp := p
		ac.policies[policyKey(cp.TenantID, cp.UserID, cp.BucketName)] = &cp
		ac.mu.Unlock()
	})

	loadKV(accessKeyPrefix, func(v string) {
		var k models.AccessKey
		if err := json.Unmarshal([]byte(v), &k); err != nil {
			return
		}
		ac.mu.Lock()
		ck := k
		ac.accessKeys[ck.AccessKeyID] = &ck
		ac.mu.Unlock()
	})

	loadKV(accessSharePrefix, func(v string) {
		var s models.BucketShare
		if err := json.Unmarshal([]byte(v), &s); err != nil {
			return
		}
		ac.mu.Lock()
		cs := s
		ac.shares[cs.ID] = &cs
		ac.mu.Unlock()
	})

	logging.Z().Info(fmt.Sprintf("✅ storage access: loaded policies=%d keys=%d shares=%d from KVStore",
		len(ac.policies), len(ac.accessKeys), len(ac.shares)))
}

func (ac *Controller) persistPolicyUnlocked(p *models.TenantPolicy) {
	if p == nil {
		return
	}
	ac.putEtcdJSON(accessPolicyPrefix+policyKey(p.TenantID, p.UserID, p.BucketName), p)
}

func (ac *Controller) persistAccessKeyUnlocked(k *models.AccessKey) {
	if k == nil {
		return
	}
	ac.putEtcdJSON(accessKeyPrefix+k.AccessKeyID, k)
}

func (ac *Controller) persistShareUnlocked(s *models.BucketShare) {
	if s == nil {
		return
	}
	ac.putEtcdJSON(accessSharePrefix+s.ID, s)
}

func (ac *Controller) persistAllUnlocked() {
	for _, p := range ac.policies {
		ac.persistPolicyUnlocked(p)
	}
	for _, k := range ac.accessKeys {
		ac.persistAccessKeyUnlocked(k)
	}
	for _, s := range ac.shares {
		ac.persistShareUnlocked(s)
	}
}
