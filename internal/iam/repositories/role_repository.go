package repositories

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
)

// RoleRepository defines CRUD operations for IAM roles and role bindings.
// Implemented by pgstore.Store.
type RoleRepository interface {
	CreateRole(r *models.Role) error
	GetRole(id string) (*models.Role, error)
	GetRoleByName(realmID, name string) (*models.Role, error)
	ListRoles(realmID string) ([]models.Role, error)
	ListClientRoles(clientID string) ([]models.Role, error)
	UpdateRole(r *models.Role) error
	DeleteRole(id string) error

	// Role bindings
	CreateRoleBinding(rb *models.RoleBinding) error
	ListUserRoleBindings(userID string) ([]models.RoleBinding, error)
	ListGroupRoleBindings(groupID string) ([]models.RoleBinding, error)
	DeleteRoleBinding(id string) error
	GetEffectiveRoles(userID, realmID string) ([]string, error)
}
