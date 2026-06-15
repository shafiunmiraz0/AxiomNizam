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

// ChallengeRepository implements repositories.ChallengeRepository using PostgreSQL.
type ChallengeRepository struct {
	db *sql.DB
}

// NewChallengeRepository creates a new PostgreSQL-backed challenge repository.
func NewChallengeRepository(db *sql.DB) repositories.ChallengeRepository {
	return &ChallengeRepository{db: db}
}

// Create inserts a new challenge.
func (r *ChallengeRepository) Create(ctx context.Context, challenge *models.Challenge) (*models.Challenge, error) {
	id := uuid.New()
	now := time.Now().UTC()

	query := `
		INSERT INTO twofactor_challenges (id, user_id, factor_id, phase, nonce, attempts, expires_at, ip_address, user_agent, created_at, resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 1)
		RETURNING id, user_id, factor_id, phase, nonce, attempts, expires_at, resolved_at, ip_address, user_agent, created_at, resource_version
	`

	var resolvedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query,
		id, challenge.UserID, challenge.FactorID, challenge.Phase, challenge.Nonce,
		challenge.Attempts, challenge.ExpiresAt, challenge.IPAddress, challenge.UserAgent, now,
	).Scan(
		&challenge.ID, &challenge.UserID, &challenge.FactorID, &challenge.Phase, &challenge.Nonce,
		&challenge.Attempts, &challenge.ExpiresAt, &resolvedAt, &challenge.IPAddress, &challenge.UserAgent,
		&challenge.CreatedAt, &challenge.ResourceVersion,
	)
	if err != nil {
		return nil, err
	}

	if resolvedAt.Valid {
		challenge.ResolvedAt = &resolvedAt.Time
	}

	challenge.ID = id
	challenge.CreatedAt = now
	challenge.ResourceVersion = 1

	return challenge, nil
}

// Get retrieves a challenge by ID.
func (r *ChallengeRepository) Get(ctx context.Context, id models.ChallengeID) (*models.Challenge, error) {
	query := `
		SELECT id, user_id, factor_id, phase, nonce, attempts, expires_at, resolved_at, ip_address, user_agent, created_at, resource_version
		FROM twofactor_challenges
		WHERE id = $1
	`

	challenge := &models.Challenge{}
	var resolvedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&challenge.ID, &challenge.UserID, &challenge.FactorID, &challenge.Phase, &challenge.Nonce,
		&challenge.Attempts, &challenge.ExpiresAt, &resolvedAt, &challenge.IPAddress, &challenge.UserAgent,
		&challenge.CreatedAt, &challenge.ResourceVersion,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if resolvedAt.Valid {
		challenge.ResolvedAt = &resolvedAt.Time
	}

	return challenge, nil
}

// GetByUserID retrieves active challenges for a user.
func (r *ChallengeRepository) GetByUserID(ctx context.Context, userID models.UserID) ([]*models.Challenge, error) {
	query := `
		SELECT id, user_id, factor_id, phase, nonce, attempts, expires_at, resolved_at, ip_address, user_agent, created_at, resource_version
		FROM twofactor_challenges
		WHERE user_id = $1 AND phase NOT IN ('Verified', 'Expired', 'Failed', 'Rejected')
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var challenges []*models.Challenge
	for rows.Next() {
		challenge := &models.Challenge{}
		var resolvedAt sql.NullTime

		if err := rows.Scan(
			&challenge.ID, &challenge.UserID, &challenge.FactorID, &challenge.Phase, &challenge.Nonce,
			&challenge.Attempts, &challenge.ExpiresAt, &resolvedAt, &challenge.IPAddress, &challenge.UserAgent,
			&challenge.CreatedAt, &challenge.ResourceVersion,
		); err != nil {
			return nil, err
		}

		if resolvedAt.Valid {
			challenge.ResolvedAt = &resolvedAt.Time
		}

		challenges = append(challenges, challenge)
	}

	return challenges, rows.Err()
}

// Update writes changes to a challenge.
func (r *ChallengeRepository) Update(ctx context.Context, challenge *models.Challenge) (*models.Challenge, error) {
	query := `
		UPDATE twofactor_challenges
		SET phase = $2, nonce = $3, attempts = $4, expires_at = $5, resolved_at = $6, resource_version = resource_version + 1
		WHERE id = $1 AND resource_version = $7
		RETURNING resource_version
	`

	var newVersion int64
	err := r.db.QueryRowContext(ctx, query,
		challenge.ID, challenge.Phase, challenge.Nonce, challenge.Attempts, challenge.ExpiresAt,
		challenge.ResolvedAt, challenge.ResourceVersion,
	).Scan(&newVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("optimistic concurrency violation or challenge not found")
		}
		return nil, err
	}

	challenge.ResourceVersion = newVersion

	return challenge, nil
}

// Delete removes a challenge record.
func (r *ChallengeRepository) Delete(ctx context.Context, id models.ChallengeID) error {
	query := `DELETE FROM twofactor_challenges WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// ListExpired returns challenges past their TTL.
func (r *ChallengeRepository) ListExpired(ctx context.Context) ([]*models.Challenge, error) {
	query := `
		SELECT id, user_id, factor_id, phase, nonce, attempts, expires_at, resolved_at, ip_address, user_agent, created_at, resource_version
		FROM twofactor_challenges
		WHERE expires_at < NOW() AND phase NOT IN ('Verified', 'Expired', 'Failed', 'Rejected')
		ORDER BY expires_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var challenges []*models.Challenge
	for rows.Next() {
		challenge := &models.Challenge{}
		var resolvedAt sql.NullTime

		if err := rows.Scan(
			&challenge.ID, &challenge.UserID, &challenge.FactorID, &challenge.Phase, &challenge.Nonce,
			&challenge.Attempts, &challenge.ExpiresAt, &resolvedAt, &challenge.IPAddress, &challenge.UserAgent,
			&challenge.CreatedAt, &challenge.ResourceVersion,
		); err != nil {
			return nil, err
		}

		if resolvedAt.Valid {
			challenge.ResolvedAt = &resolvedAt.Time
		}

		challenges = append(challenges, challenge)
	}

	return challenges, rows.Err()
}

// CountAttempts returns the number of failed attempts for a challenge.
func (r *ChallengeRepository) CountAttempts(ctx context.Context, challengeID models.ChallengeID) (int, error) {
	query := `SELECT attempts FROM twofactor_challenges WHERE id = $1`

	var attempts int
	err := r.db.QueryRowContext(ctx, query, challengeID).Scan(&attempts)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	return attempts, nil
}
