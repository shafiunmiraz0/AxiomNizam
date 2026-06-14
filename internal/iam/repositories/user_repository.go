package repositories

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
)

// UserRepository defines user-related operations for IAM.
// Implemented by pgstore.Store.
//
// Note: User CRUD (Create/Get/List/Update/Delete) is currently handled
// via direct GORM calls in admin handlers. This interface covers the
// user-adjacent operations that pgstore.Store encapsulates.
type UserRepository interface {
	// Group membership
	AddUserToGroup(userID, groupID string) error
	RemoveUserFromGroup(userID, groupID string) error
	GetUserGroups(userID string) ([]models.Group, error)
	GetGroupMembers(groupID string) ([]models.User, error)

	// Attributes
	SetUserAttribute(userID, key, value string) error
	GetUserAttributes(userID string) ([]models.UserAttribute, error)
	DeleteUserAttribute(userID, key string) error

	// Credentials
	AddCredential(c *models.Credential) error
	GetCredentials(userID, credType string) ([]models.Credential, error)
	DeleteCredential(id string) error

	// Consents
	CreateOrUpdateConsent(consent *models.UserConsent) error
	GetUserConsents(userID string) ([]models.UserConsent, error)
	RevokeConsent(userID, clientID string) error

	// Required actions
	AddRequiredAction(userID, action string) error
	GetRequiredActions(userID string) ([]models.RequiredAction, error)
	RemoveRequiredAction(userID, action string) error
}
