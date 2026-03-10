package postgres

import (
	"context"
	"errors"
	"fmt"

	"note-app/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) UserRepository {
	return UserRepository{db: db}
}

func (r UserRepository) Create(ctx context.Context, user domain.User) (domain.User, error) {
	query := `
        INSERT INTO users (id, email, full_name, password_hash, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id, email, full_name, password_hash, created_at, updated_at
    `

	var created domain.User
	if err := r.db.QueryRow(ctx, query, user.ID, user.Email, user.FullName, user.PasswordHash, user.CreatedAt, user.UpdatedAt).Scan(
		&created.ID,
		&created.Email,
		&created.FullName,
		&created.PasswordHash,
		&created.CreatedAt,
		&created.UpdatedAt,
	); err != nil {
		if isUniqueViolation(err) {
			return domain.User{}, domain.ErrEmailAlreadyUsed
		}
		return domain.User{}, fmt.Errorf("insert user: %w", err)
	}

	return created, nil
}

func (r UserRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	query := `SELECT id, email, full_name, password_hash, created_at, updated_at FROM users WHERE email = $1`

	var user domain.User
	if err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.FullName,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}
		return domain.User{}, fmt.Errorf("select user by email: %w", err)
	}

	return user, nil
}

func (r UserRepository) GetByID(ctx context.Context, userID string) (domain.User, error) {
	query := `SELECT id, email, full_name, password_hash, created_at, updated_at FROM users WHERE id = $1`

	var user domain.User
	if err := r.db.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.FullName,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}
		return domain.User{}, fmt.Errorf("select user by id: %w", err)
	}

	return user, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
