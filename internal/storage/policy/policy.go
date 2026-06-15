package policy

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
)

// Controller maps IAM roles to S3-compatible bucket policies.
// Follows the IAM handler pattern with interface-based dependencies.
type Controller struct {
	mu       sync.RWMutex
	policies map[string]*models.TenantPolicy // key = tenantID/userID/bucket
}

// NewController creates a new policy controller.
func NewController() *Controller {
	return &Controller{
		policies: make(map[string]*models.TenantPolicy),
	}
}

func policyKey(tenantID, userID, bucket string) string {
	return tenantID + "/" + userID + "/" + bucket
}

// GeneratePolicy creates an S3 policy for a given tenant, user, and role.
func (pc *Controller) GeneratePolicy(tenantID, userID, bucketName string, role models.StorageRole, prefix string) (*models.TenantPolicy, error) {
	actions := roleToActions(role)
	if len(actions) == 0 {
		return nil, fmt.Errorf("storage: unknown role %q", role)
	}

	resourceARN := fmt.Sprintf("arn:aws:s3:::%s", bucketName)
	objectARN := fmt.Sprintf("arn:aws:s3:::%s/*", bucketName)
	if prefix != "" {
		objectARN = fmt.Sprintf("arn:aws:s3:::%s/%s/*", bucketName, strings.TrimSuffix(prefix, "/"))
	}

	p := models.S3BucketPolicy{
		Version: "2012-10-17",
		Statement: []models.S3PolicyStatement{
			{
				Sid:       fmt.Sprintf("Tenant%sAccess", tenantID),
				Effect:    "Allow",
				Principal: "*",
				Action:    []models.S3Action{models.S3ActionListBucket, models.S3ActionGetBucketLoc},
				Resource:  []string{resourceARN},
			},
			{
				Sid:       fmt.Sprintf("Tenant%sObjects", tenantID),
				Effect:    "Allow",
				Principal: "*",
				Action:    actions,
				Resource:  []string{objectARN},
			},
		},
	}

	policyJSON, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("storage: failed to marshal policy: %w", err)
	}

	tp := &models.TenantPolicy{
		TenantID:   tenantID,
		UserID:     userID,
		Role:       role,
		BucketName: bucketName,
		Prefix:     prefix,
		PolicyJSON: string(policyJSON),
	}

	pc.mu.Lock()
	pc.policies[policyKey(tenantID, userID, bucketName)] = tp
	pc.mu.Unlock()

	logging.Z().Info(fmt.Sprintf("✅ Storage: policy generated for tenant=%s user=%s bucket=%s role=%s", tenantID, userID, bucketName, role))
	return tp, nil
}

// GetPolicy retrieves a stored policy.
func (pc *Controller) GetPolicy(tenantID, userID, bucket string) (*models.TenantPolicy, bool) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	p, ok := pc.policies[policyKey(tenantID, userID, bucket)]
	return p, ok
}

// ListPolicies returns all policies, optionally filtered by tenant.
func (pc *Controller) ListPolicies(tenantID string) []*models.TenantPolicy {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	var result []*models.TenantPolicy
	for _, p := range pc.policies {
		if tenantID == "" || p.TenantID == tenantID {
			result = append(result, p)
		}
	}
	return result
}

// DeletePolicy removes a policy mapping.
func (pc *Controller) DeletePolicy(tenantID, userID, bucket string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	delete(pc.policies, policyKey(tenantID, userID, bucket))
}

// MapIAMRoleToStorageRole converts a platform IAM role name to a storage role.
func MapIAMRoleToStorageRole(iamRole string) models.StorageRole {
	switch strings.ToLower(strings.TrimSpace(iamRole)) {
	case "sysadmin", "system-manager", "storage-admin":
		return models.StorageRoleAdmin
	case "admin", "manager", "storage-writer":
		return models.StorageRoleWriter
	case "user", "viewer", "storage-reader":
		return models.StorageRoleReader
	case "uploader", "storage-uploader":
		return models.StorageRoleUploader
	default:
		return models.StorageRoleReader
	}
}

func roleToActions(role models.StorageRole) []models.S3Action {
	switch role {
	case models.StorageRoleAdmin:
		return []models.S3Action{models.S3ActionAll}
	case models.StorageRoleWriter:
		return []models.S3Action{models.S3ActionGetObject, models.S3ActionPutObject, models.S3ActionDeleteObject}
	case models.StorageRoleReader:
		return []models.S3Action{models.S3ActionGetObject}
	case models.StorageRoleUploader:
		return []models.S3Action{models.S3ActionPutObject}
	default:
		return nil
	}
}
