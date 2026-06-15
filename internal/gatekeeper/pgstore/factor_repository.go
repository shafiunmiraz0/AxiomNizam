package pgstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/repositories"
)

// FactorRepository implements repositories.FactorRepository using PostgreSQL.
type FactorRepository struct {
	db *sql.DB
}

// NewFactorRepository creates a new PostgreSQL-backed factor repository.
func NewFactorRepository(db *sql.DB) repositories.FactorRepository {
	return &FactorRepository{db: db}
}

// Create inserts a new factor.
func (r *FactorRepository) Create(ctx context.Context, factor *models.Factor) (*models.Factor, error) {
	id := uuid.New()
	now := time.Now().UTC()

	specJSON, err := json.Marshal(factor.Spec)
	if err != nil {
		return nil, err
	}

	statusJSON, err := json.Marshal(factor.Status)
	if err != nil {
		return nil, err
	}

	query := `
		INSERT INTO twofactor_factors (id, user_id, spec, status, created_at, updated_at, resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, 1)
		RETURNING id, user_id, spec, status, created_at, updated_at, resource_version, deleted_at
	`

	var deletedAt sql.NullTime
	err = r.db.QueryRowContext(ctx, query, id, factor.UserID, specJSON, statusJSON, now, now).Scan(
		&factor.ID, &factor.UserID, &specJSON, &statusJSON, &factor.CreatedAt, &factor.UpdatedAt, &factor.ResourceVersion, &deletedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(specJSON, &factor.Spec); err != nil {
		return nil, fmt.Errorf("unmarshal factor spec: %w", err)
	}
	if err := json.Unmarshal(statusJSON, &factor.Status); err != nil {
		return nil, fmt.Errorf("unmarshal factor status: %w", err)
	}

	if deletedAt.Valid {
		factor.DeletedAt = &deletedAt.Time
	}

	factor.ID = id
	factor.CreatedAt = now
	factor.UpdatedAt = now
	factor.ResourceVersion = 1

	return factor, nil
}

// Get retrieves a factor by ID.
func (r *FactorRepository) Get(ctx context.Context, id models.FactorID) (*models.Factor, error) {
	query := `
		SELECT id, user_id, spec, status, created_at, updated_at, resource_version, deleted_at
		FROM twofactor_factors
		WHERE id = $1 AND deleted_at IS NULL
	`

	factor := &models.Factor{}
	var specJSON, statusJSON []byte
	var deletedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&factor.ID, &factor.UserID, &specJSON, &statusJSON, &factor.CreatedAt, &factor.UpdatedAt, &factor.ResourceVersion, &deletedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(specJSON, &factor.Spec); err != nil {
		return nil, fmt.Errorf("unmarshal factor spec: %w", err)
	}
	if err := json.Unmarshal(statusJSON, &factor.Status); err != nil {
		return nil, fmt.Errorf("unmarshal factor status: %w", err)
	}

	if deletedAt.Valid {
		factor.DeletedAt = &deletedAt.Time
	}

	return factor, nil
}

// GetByUserID retrieves all active factors for a user.
func (r *FactorRepository) GetByUserID(ctx context.Context, userID models.UserID) ([]*models.Factor, error) {
	query := `
		SELECT id, user_id, spec, status, created_at, updated_at, resource_version, deleted_at
		FROM twofactor_factors
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var factors []*models.Factor
	for rows.Next() {
		factor := &models.Factor{}
		var specJSON, statusJSON []byte
		var deletedAt sql.NullTime

		if err := rows.Scan(
			&factor.ID, &factor.UserID, &specJSON, &statusJSON, &factor.CreatedAt, &factor.UpdatedAt, &factor.ResourceVersion, &deletedAt,
		); err != nil {
			return nil, err
		}

		if err := json.Unmarshal(specJSON, &factor.Spec); err != nil {
			return nil, fmt.Errorf("unmarshal factor spec: %w", err)
		}
		if err := json.Unmarshal(statusJSON, &factor.Status); err != nil {
			return nil, fmt.Errorf("unmarshal factor status: %w", err)
		}

		if deletedAt.Valid {
			factor.DeletedAt = &deletedAt.Time
		}

		factors = append(factors, factor)
	}

	return factors, rows.Err()
}

// List retrieves all factors with optional filtering.
func (r *FactorRepository) List(ctx context.Context, filters map[string]interface{}) ([]*models.Factor, error) {
	query := `
		SELECT id, user_id, spec, status, created_at, updated_at, resource_version, deleted_at
		FROM twofactor_factors
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var factors []*models.Factor
	for rows.Next() {
		factor := &models.Factor{}
		var specJSON, statusJSON []byte
		var deletedAt sql.NullTime

		if err := rows.Scan(
			&factor.ID, &factor.UserID, &specJSON, &statusJSON, &factor.CreatedAt, &factor.UpdatedAt, &factor.ResourceVersion, &deletedAt,
		); err != nil {
			return nil, err
		}

		if err := json.Unmarshal(specJSON, &factor.Spec); err != nil {
			return nil, fmt.Errorf("unmarshal factor spec: %w", err)
		}
		if err := json.Unmarshal(statusJSON, &factor.Status); err != nil {
			return nil, fmt.Errorf("unmarshal factor status: %w", err)
		}

		if deletedAt.Valid {
			factor.DeletedAt = &deletedAt.Time
		}

		factors = append(factors, factor)
	}

	return factors, rows.Err()
}

// Update writes changes to an existing factor with optimistic concurrency.
func (r *FactorRepository) Update(ctx context.Context, factor *models.Factor) (*models.Factor, error) {
	specJSON, err := json.Marshal(factor.Spec)
	if err != nil {
		return nil, err
	}

	statusJSON, err := json.Marshal(factor.Status)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	query := `
		UPDATE twofactor_factors
		SET spec = $2, status = $3, updated_at = $4, resource_version = resource_version + 1
		WHERE id = $1 AND resource_version = $5
		RETURNING resource_version, updated_at
	`

	var newVersion int64
	err = r.db.QueryRowContext(ctx, query, factor.ID, specJSON, statusJSON, now, factor.ResourceVersion).Scan(&newVersion, &factor.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("optimistic concurrency violation or factor not found")
		}
		return nil, err
	}

	factor.ResourceVersion = newVersion
	factor.UpdatedAt = now

	return factor, nil
}

// Delete soft-deletes a factor.
func (r *FactorRepository) Delete(ctx context.Context, id models.FactorID) error {
	query := `
		UPDATE twofactor_factors
		SET deleted_at = $2
		WHERE id = $1 AND deleted_at IS NULL
	`

	_, err := r.db.ExecContext(ctx, query, id, time.Now().UTC())
	return err
}

// Exists checks if a factor exists and is active.
func (r *FactorRepository) Exists(ctx context.Context, id models.FactorID) (bool, error) {
	query := `SELECT 1 FROM twofactor_factors WHERE id = $1 AND deleted_at IS NULL`

	var exists int
	err := r.db.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return exists == 1, nil
}

// Count returns the number of active factors for a user.
func (r *FactorRepository) Count(ctx context.Context, userID models.UserID) (int, error) {
	query := `SELECT COUNT(*) FROM twofactor_factors WHERE user_id = $1 AND deleted_at IS NULL`

	var count int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	return count, err
}

// ListExpiredPending returns factors in Pending phase exceeding grace period.
func (r *FactorRepository) ListExpiredPending(ctx context.Context, gracePeriodDays int) ([]*models.Factor, error) {
	query := `
		SELECT id, user_id, spec, status, created_at, updated_at, resource_version, deleted_at
		FROM twofactor_factors
		WHERE deleted_at IS NULL
		  AND status->>'phase' = 'Pending'
		  AND created_at < NOW() - INTERVAL '1 day' * $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, gracePeriodDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var factors []*models.Factor
	for rows.Next() {
		factor := &models.Factor{}
		var specJSON, statusJSON []byte
		var deletedAt sql.NullTime

		if err := rows.Scan(
			&factor.ID, &factor.UserID, &specJSON, &statusJSON, &factor.CreatedAt, &factor.UpdatedAt, &factor.ResourceVersion, &deletedAt,
		); err != nil {
			return nil, err
		}

		if err := json.Unmarshal(specJSON, &factor.Spec); err != nil {
			return nil, fmt.Errorf("unmarshal factor spec: %w", err)
		}
		if err := json.Unmarshal(statusJSON, &factor.Status); err != nil {
			return nil, fmt.Errorf("unmarshal factor status: %w", err)
		}

		if deletedAt.Valid {
			factor.DeletedAt = &deletedAt.Time
		}

		factors = append(factors, factor)
	}

	return factors, rows.Err()
}
