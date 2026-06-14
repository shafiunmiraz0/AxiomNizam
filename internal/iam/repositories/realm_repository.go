package repositories

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
)

// RealmRepository defines CRUD operations for IAM realms.
// Implemented by pgstore.Store.
type RealmRepository interface {
	CreateRealm(r *models.Realm) error
	GetRealm(id string) (*models.Realm, error)
	GetRealmByName(name string) (*models.Realm, error)
	ListRealms() ([]models.Realm, error)
	UpdateRealm(r *models.Realm) error
	DeleteRealm(id string) error
	SeedDefaultRealm() (*models.Realm, error)
}
