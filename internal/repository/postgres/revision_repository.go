package postgres

import (
	"context"
	"errors"
	"fmt"

	"note-app/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RevisionRepository struct {
	db *pgxpool.Pool
}

func NewRevisionRepository(db *pgxpool.Pool) RevisionRepository {
	return RevisionRepository{db: db}
}

func (r RevisionRepository) Create(ctx context.Context, revision domain.Revision) (domain.Revision, error) {
	query := `
		INSERT INTO revisions (id, page_id, label, note, content, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, page_id, label, note, content, created_by, created_at
	`

	var saved domain.Revision
	if err := r.db.QueryRow(ctx, query, revision.ID, revision.PageID, revision.Label, revision.Note, revision.Content, revision.CreatedBy, revision.CreatedAt).Scan(
		&saved.ID,
		&saved.PageID,
		&saved.Label,
		&saved.Note,
		&saved.Content,
		&saved.CreatedBy,
		&saved.CreatedAt,
	); err != nil {
		return domain.Revision{}, fmt.Errorf("insert revision: %w", err)
	}

	return saved, nil
}

func (r RevisionRepository) GetByID(ctx context.Context, revisionID string) (domain.Revision, error) {
	query := `
		SELECT id, page_id, label, note, content, created_by, created_at
		FROM revisions
		WHERE id = $1
	`

	var revision domain.Revision
	if err := r.db.QueryRow(ctx, query, revisionID).Scan(
		&revision.ID,
		&revision.PageID,
		&revision.Label,
		&revision.Note,
		&revision.Content,
		&revision.CreatedBy,
		&revision.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Revision{}, domain.ErrNotFound
		}
		return domain.Revision{}, fmt.Errorf("select revision by id: %w", err)
	}

	return revision, nil
}

func (r RevisionRepository) ListByPageID(ctx context.Context, pageID string) ([]domain.Revision, error) {
	query := `
		SELECT id, page_id, label, note, created_by, created_at
		FROM revisions
		WHERE page_id = $1
		ORDER BY created_at ASC, id ASC
	`

	rows, err := r.db.Query(ctx, query, pageID)
	if err != nil {
		return nil, fmt.Errorf("list revisions by page id: %w", err)
	}
	defer rows.Close()

	revisions := make([]domain.Revision, 0)
	for rows.Next() {
		var revision domain.Revision
		if err := rows.Scan(
			&revision.ID,
			&revision.PageID,
			&revision.Label,
			&revision.Note,
			&revision.CreatedBy,
			&revision.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan revision: %w", err)
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate revisions: %w", err)
	}

	return revisions, nil
}
