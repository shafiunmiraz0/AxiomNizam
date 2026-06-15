package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/repositories"
)

// TrustedDeviceRepository implements repositories.TrustedDeviceRepository using PostgreSQL.
type TrustedDeviceRepository struct {
	db *sql.DB
}

// NewTrustedDeviceRepository creates a new PostgreSQL-backed trusted device repository.
func NewTrustedDeviceRepository(db *sql.DB) repositories.TrustedDeviceRepository {
	return &TrustedDeviceRepository{db: db}
}

// Create registers a new trusted device.
func (r *TrustedDeviceRepository) Create(ctx context.Context, device *models.TrustedDevice) (*models.TrustedDevice, error) {
	id := uuid.New()
	now := time.Now().UTC()

	query := `
		INSERT INTO twofactor_trusted_devices (id, user_id, token_hash, fingerprint, user_agent, ip_address, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, user_id, token_hash, fingerprint, user_agent, ip_address, expires_at, revoked_at, created_at
	`

	var revokedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query,
		id, device.UserID, device.TokenHash, device.Fingerprint, device.UserAgent, device.IPAddress, device.ExpiresAt, now,
	).Scan(
		&device.ID, &device.UserID, &device.TokenHash, &device.Fingerprint, &device.UserAgent, &device.IPAddress, &device.ExpiresAt, &revokedAt, &device.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if revokedAt.Valid {
		device.RevokedAt = &revokedAt.Time
	}

	device.ID = id
	device.CreatedAt = now

	return device, nil
}

// Get retrieves a trusted device by ID.
func (r *TrustedDeviceRepository) Get(ctx context.Context, id uuid.UUID) (*models.TrustedDevice, error) {
	query := `
		SELECT id, user_id, token_hash, fingerprint, user_agent, ip_address, expires_at, revoked_at, created_at
		FROM twofactor_trusted_devices
		WHERE id = $1
	`

	device := &models.TrustedDevice{}
	var revokedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&device.ID, &device.UserID, &device.TokenHash, &device.Fingerprint, &device.UserAgent, &device.IPAddress, &device.ExpiresAt, &revokedAt, &device.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if revokedAt.Valid {
		device.RevokedAt = &revokedAt.Time
	}

	return device, nil
}

// GetByUserID retrieves active trusted devices for a user.
func (r *TrustedDeviceRepository) GetByUserID(ctx context.Context, userID models.UserID) ([]*models.TrustedDevice, error) {
	query := `
		SELECT id, user_id, token_hash, fingerprint, user_agent, ip_address, expires_at, revoked_at, created_at
		FROM twofactor_trusted_devices
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []*models.TrustedDevice
	for rows.Next() {
		device := &models.TrustedDevice{}
		var revokedAt sql.NullTime

		if err := rows.Scan(
			&device.ID, &device.UserID, &device.TokenHash, &device.Fingerprint, &device.UserAgent, &device.IPAddress, &device.ExpiresAt, &revokedAt, &device.CreatedAt,
		); err != nil {
			return nil, err
		}

		if revokedAt.Valid {
			device.RevokedAt = &revokedAt.Time
		}

		devices = append(devices, device)
	}

	return devices, rows.Err()
}

// GetByFingerprint retrieves a trusted device by fingerprint.
func (r *TrustedDeviceRepository) GetByFingerprint(ctx context.Context, userID models.UserID, fingerprint string) (*models.TrustedDevice, error) {
	query := `
		SELECT id, user_id, token_hash, fingerprint, user_agent, ip_address, expires_at, revoked_at, created_at
		FROM twofactor_trusted_devices
		WHERE user_id = $1 AND fingerprint = $2 AND revoked_at IS NULL AND expires_at > NOW()
	`

	device := &models.TrustedDevice{}
	var revokedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, userID, fingerprint).Scan(
		&device.ID, &device.UserID, &device.TokenHash, &device.Fingerprint, &device.UserAgent, &device.IPAddress, &device.ExpiresAt, &revokedAt, &device.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if revokedAt.Valid {
		device.RevokedAt = &revokedAt.Time
	}

	return device, nil
}

// Update updates device metadata.
func (r *TrustedDeviceRepository) Update(ctx context.Context, device *models.TrustedDevice) (*models.TrustedDevice, error) {
	query := `
		UPDATE twofactor_trusted_devices
		SET user_agent = $2, ip_address = $3, fingerprint = $4
		WHERE id = $1
		RETURNING id, user_id, token_hash, fingerprint, user_agent, ip_address, expires_at, revoked_at, created_at
	`

	var revokedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, device.ID, device.UserAgent, device.IPAddress, device.Fingerprint).Scan(
		&device.ID, &device.UserID, &device.TokenHash, &device.Fingerprint, &device.UserAgent, &device.IPAddress, &device.ExpiresAt, &revokedAt, &device.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if revokedAt.Valid {
		device.RevokedAt = &revokedAt.Time
	}

	return device, nil
}

// Revoke marks a device as revoked.
func (r *TrustedDeviceRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE twofactor_trusted_devices
		SET revoked_at = $2
		WHERE id = $1 AND revoked_at IS NULL
	`

	_, err := r.db.ExecContext(ctx, query, id, time.Now().UTC())
	return err
}

// RevokeByUserID revokes all devices for a user.
func (r *TrustedDeviceRepository) RevokeByUserID(ctx context.Context, userID models.UserID) error {
	query := `
		UPDATE twofactor_trusted_devices
		SET revoked_at = $2
		WHERE user_id = $1 AND revoked_at IS NULL
	`

	_, err := r.db.ExecContext(ctx, query, userID, time.Now().UTC())
	return err
}

// DeleteExpired removes all expired device records.
func (r *TrustedDeviceRepository) DeleteExpired(ctx context.Context) error {
	query := `DELETE FROM twofactor_trusted_devices WHERE expires_at < NOW()`

	_, err := r.db.ExecContext(ctx, query)
	return err
}

// Count returns the number of active devices for a user.
func (r *TrustedDeviceRepository) Count(ctx context.Context, userID models.UserID) (int, error) {
	query := `SELECT COUNT(*) FROM twofactor_trusted_devices WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()`

	var count int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	return count, err
}
