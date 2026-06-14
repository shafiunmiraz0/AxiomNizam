package repositories

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
)

// ClientRepository defines CRUD operations for IAM clients (OAuth2/OIDC).
// Implemented by pgstore.Store.
type ClientRepository interface {
	CreateClient(c *models.Client) error
	GetClient(id string) (*models.Client, error)
	ListClients(realmID string) ([]models.Client, error)
	UpdateClient(c *models.Client) error
	DeleteClient(id string) error
}
