package repositories

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
)

// GroupRepository defines CRUD operations for IAM groups.
// Implemented by pgstore.Store.
type GroupRepository interface {
	CreateGroup(g *models.Group) error
	GetGroup(id string) (*models.Group, error)
	ListGroups(realmID string) ([]models.Group, error)
	ListSubGroups(parentID string) ([]models.Group, error)
	UpdateGroup(g *models.Group) error
	DeleteGroup(id string) error
}
