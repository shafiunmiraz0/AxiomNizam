package repositories

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
)

// ClientScopeRepository defines CRUD operations for IAM client scopes.
// Implemented by pgstore.Store.
type ClientScopeRepository interface {
	CreateClientScope(cs *models.ClientScope) error
	GetClientScope(id string) (*models.ClientScope, error)
	ListClientScopes(realmID string) ([]models.ClientScope, error)
	UpdateClientScope(cs *models.ClientScope) error
	DeleteClientScope(id string) error
}
