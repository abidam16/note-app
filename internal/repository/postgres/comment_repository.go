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

type CommentRepository struct {
	db *pgxpool.Pool
}

func NewCommentRepository(db *pgxpool.Pool) CommentRepository {
	return CommentRepository{db: db}
}

func (r CommentRepository) Create(ctx context.Context, comment domain.PageComment) (domain.PageComment, error) {
	query := `
		INSERT INTO page_comments (id, page_id, body, created_by, created_at, resolved_by, resolved_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, page_id, body, created_by, created_at, resolved_by, resolved_at
	`

	var saved domain.PageComment
	if err := r.db.QueryRow(ctx, query, comment.ID, comment.PageID, comment.Body, comment.CreatedBy, comment.CreatedAt, comment.ResolvedBy, comment.ResolvedAt).Scan(
		&saved.ID,
		&saved.PageID,
		&saved.Body,
		&saved.CreatedBy,
		&saved.CreatedAt,
		&saved.ResolvedBy,
		&saved.ResolvedAt,
	); err != nil {
		return domain.PageComment{}, fmt.Errorf("insert page comment: %w", err)
	}

	return saved, nil
}

func (r CommentRepository) GetByID(ctx context.Context, commentID string) (domain.PageComment, error) {
	query := `
		SELECT id, page_id, body, created_by, created_at, resolved_by, resolved_at
		FROM page_comments
		WHERE id = $1
	`

	var comment domain.PageComment
	if err := r.db.QueryRow(ctx, query, commentID).Scan(
		&comment.ID,
		&comment.PageID,
		&comment.Body,
		&comment.CreatedBy,
		&comment.CreatedAt,
		&comment.ResolvedBy,
		&comment.ResolvedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PageComment{}, domain.ErrNotFound
		}
		return domain.PageComment{}, fmt.Errorf("select page comment by id: %w", err)
	}

	return comment, nil
}

func (r CommentRepository) ListByPageID(ctx context.Context, pageID string) ([]domain.PageComment, error) {
	query := `
		SELECT id, page_id, body, created_by, created_at, resolved_by, resolved_at
		FROM page_comments
		WHERE page_id = $1
		ORDER BY created_at ASC, id ASC
	`

	rows, err := r.db.Query(ctx, query, pageID)
	if err != nil {
		return nil, fmt.Errorf("list page comments: %w", err)
	}
	defer rows.Close()

	comments := make([]domain.PageComment, 0)
	for rows.Next() {
		var comment domain.PageComment
		if err := rows.Scan(
			&comment.ID,
			&comment.PageID,
			&comment.Body,
			&comment.CreatedBy,
			&comment.CreatedAt,
			&comment.ResolvedBy,
			&comment.ResolvedAt,
		); err != nil {
			return nil, fmt.Errorf("scan page comment: %w", err)
		}
		comments = append(comments, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate page comments: %w", err)
	}

	return comments, nil
}

func (r CommentRepository) Resolve(ctx context.Context, commentID string, resolvedBy string, resolvedAt time.Time) (domain.PageComment, error) {
	query := `
		UPDATE page_comments
		SET resolved_by = $2, resolved_at = $3
		WHERE id = $1
		RETURNING id, page_id, body, created_by, created_at, resolved_by, resolved_at
	`

	var comment domain.PageComment
	if err := r.db.QueryRow(ctx, query, commentID, resolvedBy, resolvedAt).Scan(
		&comment.ID,
		&comment.PageID,
		&comment.Body,
		&comment.CreatedBy,
		&comment.CreatedAt,
		&comment.ResolvedBy,
		&comment.ResolvedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PageComment{}, domain.ErrNotFound
		}
		return domain.PageComment{}, fmt.Errorf("resolve page comment: %w", err)
	}

	return comment, nil
}
