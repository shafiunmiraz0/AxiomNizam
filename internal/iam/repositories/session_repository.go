package repositories

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
)

// SessionRepository defines CRUD operations for IAM SSO sessions.
// Implemented by pgstore.Store.
type SessionRepository interface {
	CreateSSOSession(sess *models.SSOSession) error
	GetSSOSession(id string) (*models.SSOSession, error)
	ListUserSSOSessions(userID string) ([]models.SSOSession, error)
	UpdateSSOSession(sess *models.SSOSession) error
	RevokeSSOSession(id string) error
	RevokeUserSSOSessions(userID string) error
	CleanupExpiredSessions() error
}

// EventRepository defines operations for IAM audit events.
// Implemented by pgstore.Store.
type EventRepository interface {
	RecordEvent(evt *models.Event) error
	ListEvents(realmID string, eventType string, limit int) ([]models.Event, error)
	ListUserEvents(userID string, limit int) ([]models.Event, error)
	CleanupOldEvents(olderThan time.Duration) error
}
