package repositories

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"

	"gorm.io/gorm"
)

// RBACRepository interface for RBAC operations
type RBACRepository interface {
	CreateRole(role *models.RoleModel) error
	GetRole(id string) (*models.RoleModel, error)
	ListRoles(tenantID string, limit, offset int) ([]*models.RoleModel, error)
	UpdateRole(role *models.RoleModel) error
	DeleteRole(id string) error
	CreateBinding(binding *models.RoleBindingModel) error
	GetBinding(id string) (*models.RoleBindingModel, error)
	ListBindings(tenantID, roleID, subjectID string) ([]*models.RoleBindingModel, error)
	DeleteBinding(id string) error
	CreatePermission(perm *models.PermissionModel) error
	ListPermissions(roleID string) ([]*models.PermissionModel, error)
	DeletePermission(id string) error
	CreateAccessRequest(req *models.AccessRequestModel) error
	GetAccessRequest(id string) (*models.AccessRequestModel, error)
	ListAccessRequests(tenantID, subjectID, status string) ([]*models.AccessRequestModel, error)
	UpdateAccessRequest(req *models.AccessRequestModel) error
}

// RBACRepositoryImpl implements RBACRepository
type RBACRepositoryImpl struct {
	db *gorm.DB
}

// NewRBACRepository creates RBAC repository
func NewRBACRepository(db *gorm.DB) RBACRepository {
	return &RBACRepositoryImpl{db: db}
}

// CreateRole creates role
func (r *RBACRepositoryImpl) CreateRole(role *models.RoleModel) error {
	return r.db.Create(role).Error
}

// GetRole retrieves role
func (r *RBACRepositoryImpl) GetRole(id string) (*models.RoleModel, error) {
	var role models.RoleModel
	err := r.db.Preload("Bindings").Preload("Permissions").Where("id = ?", id).First(&role).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("role not found")
	}
	return &role, err
}

// ListRoles lists roles
func (r *RBACRepositoryImpl) ListRoles(tenantID string, limit, offset int) ([]*models.RoleModel, error) {
	var roles []*models.RoleModel
	query := r.db.Preload("Bindings").Preload("Permissions").Where("tenant_id = ?", tenantID)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Order("level DESC, created_at DESC").Find(&roles).Error
	return roles, err
}

// UpdateRole updates role
func (r *RBACRepositoryImpl) UpdateRole(role *models.RoleModel) error {
	return r.db.Save(role).Error
}

// DeleteRole deletes role
func (r *RBACRepositoryImpl) DeleteRole(id string) error {
	return r.db.Delete(&models.RoleModel{}, "id = ?", id).Error
}

// CreateBinding creates role binding
func (r *RBACRepositoryImpl) CreateBinding(binding *models.RoleBindingModel) error {
	return r.db.Create(binding).Error
}

// GetBinding retrieves binding
func (r *RBACRepositoryImpl) GetBinding(id string) (*models.RoleBindingModel, error) {
	var binding models.RoleBindingModel
	err := r.db.Preload("Role").Where("id = ?", id).First(&binding).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("binding not found")
	}
	return &binding, err
}

// ListBindings lists bindings
func (r *RBACRepositoryImpl) ListBindings(tenantID, roleID, subjectID string) ([]*models.RoleBindingModel, error) {
	var bindings []*models.RoleBindingModel
	query := r.db.Preload("Role").Where("tenant_id = ?", tenantID)
	if roleID != "" {
		query = query.Where("role_id = ?", roleID)
	}
	if subjectID != "" {
		query = query.Where("subject_id = ?", subjectID)
	}
	err := query.Find(&bindings).Error
	return bindings, err
}

// DeleteBinding deletes binding
func (r *RBACRepositoryImpl) DeleteBinding(id string) error {
	return r.db.Delete(&models.RoleBindingModel{}, "id = ?", id).Error
}

// CreatePermission creates permission
func (r *RBACRepositoryImpl) CreatePermission(perm *models.PermissionModel) error {
	return r.db.Create(perm).Error
}

// ListPermissions lists permissions for role
func (r *RBACRepositoryImpl) ListPermissions(roleID string) ([]*models.PermissionModel, error) {
	var perms []*models.PermissionModel
	err := r.db.Where("role_id = ?", roleID).Find(&perms).Error
	return perms, err
}

// DeletePermission deletes permission
func (r *RBACRepositoryImpl) DeletePermission(id string) error {
	return r.db.Delete(&models.PermissionModel{}, "id = ?", id).Error
}

// CreateAccessRequest creates access request
func (r *RBACRepositoryImpl) CreateAccessRequest(req *models.AccessRequestModel) error {
	return r.db.Create(req).Error
}

// GetAccessRequest retrieves access request
func (r *RBACRepositoryImpl) GetAccessRequest(id string) (*models.AccessRequestModel, error) {
	var req models.AccessRequestModel
	err := r.db.Preload("Role").Where("id = ?", id).First(&req).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("request not found")
	}
	return &req, err
}

// ListAccessRequests lists access requests
func (r *RBACRepositoryImpl) ListAccessRequests(tenantID, subjectID, status string) ([]*models.AccessRequestModel, error) {
	var reqs []*models.AccessRequestModel
	query := r.db.Preload("Role").Where("tenant_id = ?", tenantID)
	if subjectID != "" {
		query = query.Where("subject_id = ?", subjectID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Order("created_at DESC").Find(&reqs).Error
	return reqs, err
}

// UpdateAccessRequest updates access request
func (r *RBACRepositoryImpl) UpdateAccessRequest(req *models.AccessRequestModel) error {
	return r.db.Save(req).Error
}
