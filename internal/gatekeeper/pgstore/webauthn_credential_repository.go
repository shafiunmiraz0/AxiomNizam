package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"example.com/axiomnizam/internal/gatekeeper/webauthn"
)

// WebAuthnCredentialRepository implements webauthn.CredentialStore using PostgreSQL.
type WebAuthnCredentialRepository struct {
	db *sql.DB
}

// NewWebAuthnCredentialRepository creates a new PostgreSQL-backed credential store.
func NewWebAuthnCredentialRepository(db *sql.DB) *WebAuthnCredentialRepository {
	return &WebAuthnCredentialRepository{db: db}
}

// Create inserts a new WebAuthn credential.
func (r *WebAuthnCredentialRepository) Create(ctx context.Context, cred *webauthn.Credential) error {
	query := `
		INSERT INTO twofactor_webauthn_credentials
			(id, user_id, public_key, attestation_type, aaguid, sign_count, clone_warning, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.ExecContext(ctx, query,
		cred.ID,
		cred.UserID,
		cred.PublicKey,
		cred.AttestationType,
		cred.AAGUID,
		cred.SignCount,
		cred.CloneWarning,
		cred.CreatedAt,
	)
	return err
}

// GetByUserID retrieves all WebAuthn credentials for a user.
func (r *WebAuthnCredentialRepository) GetByUserID(ctx context.Context, userID string) ([]*webauthn.Credential, error) {
	query := `
		SELECT id, user_id, public_key, attestation_type, aaguid, sign_count, clone_warning, created_at
		FROM twofactor_webauthn_credentials
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []*webauthn.Credential
	for rows.Next() {
		c := &webauthn.Credential{}
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.PublicKey, &c.AttestationType,
			&c.AAGUID, &c.SignCount, &c.CloneWarning, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// GetByCredentialID retrieves a single credential by its ID.
func (r *WebAuthnCredentialRepository) GetByCredentialID(ctx context.Context, credentialID []byte) (*webauthn.Credential, error) {
	query := `
		SELECT id, user_id, public_key, attestation_type, aaguid, sign_count, clone_warning, created_at
		FROM twofactor_webauthn_credentials
		WHERE id = $1
	`
	c := &webauthn.Credential{}
	err := r.db.QueryRowContext(ctx, query, credentialID).Scan(
		&c.ID, &c.UserID, &c.PublicKey, &c.AttestationType,
		&c.AAGUID, &c.SignCount, &c.CloneWarning, &c.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

// UpdateSignCount updates the stored sign count for clone detection.
func (r *WebAuthnCredentialRepository) UpdateSignCount(ctx context.Context, credentialID []byte, newCount uint32) error {
	query := `UPDATE twofactor_webauthn_credentials SET sign_count = $1 WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, newCount, credentialID)
	return err
}

// Delete removes a WebAuthn credential.
func (r *WebAuthnCredentialRepository) Delete(ctx context.Context, credentialID []byte) error {
	query := `DELETE FROM twofactor_webauthn_credentials WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, credentialID)
	return err
}

// CountByUserID returns the number of registered WebAuthn credentials for a user.
func (r *WebAuthnCredentialRepository) CountByUserID(ctx context.Context, userID string) (int, error) {
	query := `SELECT COUNT(*) FROM twofactor_webauthn_credentials WHERE user_id = $1`
	var count int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	return count, err
}

// DeleteByUserID removes all WebAuthn credentials for a user.
func (r *WebAuthnCredentialRepository) DeleteByUserID(ctx context.Context, userID string) error {
	query := `DELETE FROM twofactor_webauthn_credentials WHERE user_id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

// ensure interface compliance
var _ webauthn.CredentialStore = (*WebAuthnCredentialRepository)(nil)

// unused import guard
var _ = time.Now
