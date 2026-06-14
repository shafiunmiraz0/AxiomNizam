package testutil

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/iam/models"
)

// TestRealmID is a fixed realm ID for testing.
const TestRealmID = "master"

// TestUserID is a fixed user ID for testing.
const TestUserID = "test-user-001"

// TestClientID is a fixed client ID for testing.
const TestClientID = "test-client-001"

// TestRoleID is a fixed role ID for testing.
const TestRoleID = "test-role-001"

// NewTestRealm creates a test Realm with sensible defaults.
func NewTestRealm() *models.Realm {
	now := time.Now().UTC()
	return &models.Realm{
		ID:          TestRealmID,
		Name:        "master",
		DisplayName: "Master Realm",
		Enabled:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// NewTestUser creates a test User with sensible defaults.
func NewTestUser() *models.User {
	now := time.Now().UTC()
	return &models.User{
		ID:           TestUserID,
		RealmID:      TestRealmID,
		Username:     "testuser",
		Email:        "test@example.com",
		Active:       true,
		DisplayName:  "Test User",
		PasswordHash: "hashed-password",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// NewTestClient creates a test Client with sensible defaults.
func NewTestClient() *models.Client {
	now := time.Now().UTC()
	return &models.Client{
		ID:        TestClientID,
		RealmID:   TestRealmID,
		Name:      "Test Client",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// NewTestRole creates a test Role with sensible defaults.
func NewTestRole() *models.Role {
	now := time.Now().UTC()
	return &models.Role{
		ID:          TestRoleID,
		RealmID:     TestRealmID,
		Name:        "test-role",
		Description: "Test role for unit tests",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
