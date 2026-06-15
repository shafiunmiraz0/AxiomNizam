package repositories

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
)

// IdentityProviderRepository defines CRUD operations for IAM identity providers.
// Implemented by pgstore.Store.
type IdentityProviderRepository interface {
	CreateIdentityProvider(idp *models.IdentityProvider) error
	GetIdentityProvider(id string) (*models.IdentityProvider, error)
	GetIdentityProviderByAlias(realmID, alias string) (*models.IdentityProvider, error)
	ListIdentityProviders(realmID string) ([]models.IdentityProvider, error)
	UpdateIdentityProvider(idp *models.IdentityProvider) error
	DeleteIdentityProvider(id string) error
}
