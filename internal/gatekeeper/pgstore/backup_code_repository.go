package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/repositories"
)

// BackupCodeRepository implements repositories.BackupCodeRepository using PostgreSQL.
type BackupCodeRepository struct {
	db *sql.DB
}

// NewBackupCodeRepository creates a new PostgreSQL-backed backup code repository.
func NewBackupCodeRepository(db *sql.DB) repositories.BackupCodeRepository {
	return &BackupCodeRepository{db: db}
}

// Create inserts new backup codes (bulk operation).
func (r *BackupCodeRepository) Create(ctx context.Context, codes []*models.BackupCode) error {
	if len(codes) == 0 {
		return nil
	}

	query := `
		INSERT INTO twofactor_backup_codes (id, user_id, factor_id, code_hash, created_at)
		VALUES
	`

	args := make([]interface{}, 0, len(codes)*5)
	for i, code := range codes {
		if i > 0 {
			query += ", "
		}
		query += `($` + strconv.Itoa(i*5+1) + `, $` + strconv.Itoa(i*5+2) + `, $` + strconv.Itoa(i*5+3) + `, $` + strconv.Itoa(i*5+4) + `, $` + strconv.Itoa(i*5+5) + `)`

		args = append(args, code.ID, code.UserID, code.FactorID, code.CodeHash, time.Now().UTC())
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// GetByCodeHash retrieves an unused backup code by its hash.
func (r *BackupCodeRepository) GetByCodeHash(ctx context.Context, codeHash []byte) (*models.BackupCode, error) {
	query := `
		SELECT id, user_id, factor_id, code_hash, used_at, created_at
		FROM twofactor_backup_codes
		WHERE code_hash = $1 AND used_at IS NULL
		LIMIT 1
	`

	code := &models.BackupCode{}
	var usedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, codeHash).Scan(
		&code.ID, &code.UserID, &code.FactorID, &code.CodeHash, &usedAt, &code.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if usedAt.Valid {
		code.UsedAt = &usedAt.Time
	}

	return code, nil
}

// Get retrieves a single backup code by ID.
func (r *BackupCodeRepository) Get(ctx context.Context, id uuid.UUID) (*models.BackupCode, error) {
	query := `
		SELECT id, user_id, factor_id, code_hash, used_at, created_at
		FROM twofactor_backup_codes
		WHERE id = $1
	`

	code := &models.BackupCode{}
	var usedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&code.ID, &code.UserID, &code.FactorID, &code.CodeHash, &usedAt, &code.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if usedAt.Valid {
		code.UsedAt = &usedAt.Time
	}

	return code, nil
}

// GetByUserID retrieves all backup codes for a user.
func (r *BackupCodeRepository) GetByUserID(ctx context.Context, userID models.UserID) ([]*models.BackupCode, error) {
	query := `
		SELECT id, user_id, factor_id, code_hash, used_at, created_at
		FROM twofactor_backup_codes
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codes []*models.BackupCode
	for rows.Next() {
		code := &models.BackupCode{}
		var usedAt sql.NullTime

		if err := rows.Scan(
			&code.ID, &code.UserID, &code.FactorID, &code.CodeHash, &usedAt, &code.CreatedAt,
		); err != nil {
			return nil, err
		}

		if usedAt.Valid {
			code.UsedAt = &usedAt.Time
		}

		codes = append(codes, code)
	}

	return codes, rows.Err()
}

// GetByFactorID retrieves all backup codes for a factor.
func (r *BackupCodeRepository) GetByFactorID(ctx context.Context, factorID models.FactorID) ([]*models.BackupCode, error) {
	query := `
		SELECT id, user_id, factor_id, code_hash, used_at, created_at
		FROM twofactor_backup_codes
		WHERE factor_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, factorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codes []*models.BackupCode
	for rows.Next() {
		code := &models.BackupCode{}
		var usedAt sql.NullTime

		if err := rows.Scan(
			&code.ID, &code.UserID, &code.FactorID, &code.CodeHash, &usedAt, &code.CreatedAt,
		); err != nil {
			return nil, err
		}

		if usedAt.Valid {
			code.UsedAt = &usedAt.Time
		}

		codes = append(codes, code)
	}

	return codes, rows.Err()
}

// MarkUsed marks a code as consumed.
func (r *BackupCodeRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE twofactor_backup_codes
		SET used_at = $2
		WHERE id = $1 AND used_at IS NULL
	`

	_, err := r.db.ExecContext(ctx, query, id, time.Now().UTC())
	return err
}

// CountUnused returns the number of unused backup codes for a user.
func (r *BackupCodeRepository) CountUnused(ctx context.Context, userID models.UserID) (int, error) {
	query := `SELECT COUNT(*) FROM twofactor_backup_codes WHERE user_id = $1 AND used_at IS NULL`

	var count int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	return count, err
}

// DeleteByFactorID removes all backup codes for a factor.
func (r *BackupCodeRepository) DeleteByFactorID(ctx context.Context, factorID models.FactorID) error {
	query := `DELETE FROM twofactor_backup_codes WHERE factor_id = $1`

	_, err := r.db.ExecContext(ctx, query, factorID)
	return err
}
