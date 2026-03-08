package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"note-app/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RefreshTokenRepository struct {
	db *pgxpool.Pool
}

func NewRefreshTokenRepository(db *pgxpool.Pool) RefreshTokenRepository {
	return RefreshTokenRepository{db: db}
}

func (r RefreshTokenRepository) Create(ctx context.Context, token domain.RefreshToken) (domain.RefreshToken, error) {
	query := `
        INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, user_id, token_hash, expires_at, revoked_at, created_at
    `

	var created domain.RefreshToken
	if err := r.db.QueryRow(ctx, query, token.ID, token.UserID, token.TokenHash, token.ExpiresAt, token.CreatedAt).Scan(
		&created.ID,
		&created.UserID,
		&created.TokenHash,
		&created.ExpiresAt,
		&created.RevokedAt,
		&created.CreatedAt,
	); err != nil {
		return domain.RefreshToken{}, fmt.Errorf("insert refresh token: %w", err)
	}

	return created, nil
}

func (r RefreshTokenRepository) GetByHash(ctx context.Context, hash string) (domain.RefreshToken, error) {
	query := `SELECT id, user_id, token_hash, expires_at, revoked_at, created_at FROM refresh_tokens WHERE token_hash = $1`

	var token domain.RefreshToken
	if err := r.db.QueryRow(ctx, query, hash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.RevokedAt,
		&token.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RefreshToken{}, domain.ErrNotFound
		}
		return domain.RefreshToken{}, fmt.Errorf("select refresh token: %w", err)
	}

	return token, nil
}

func (r RefreshTokenRepository) RevokeByID(ctx context.Context, tokenID string, revokedAt time.Time) error {
	query := `UPDATE refresh_tokens SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL`
	if _, err := r.db.Exec(ctx, query, tokenID, revokedAt); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}
