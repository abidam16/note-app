package postgres

import (
	"context"
	"errors"
	"fmt"

	"note-app/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FolderRepository struct {
	db *pgxpool.Pool
}

func NewFolderRepository(db *pgxpool.Pool) FolderRepository {
	return FolderRepository{db: db}
}

func (r FolderRepository) Create(ctx context.Context, folder domain.Folder) (domain.Folder, error) {
	query := `
		INSERT INTO folders (id, workspace_id, parent_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, workspace_id, parent_id, name, created_at, updated_at
	`

	var created domain.Folder
	var parentID *string
	if err := r.db.QueryRow(ctx, query, folder.ID, folder.WorkspaceID, folder.ParentID, folder.Name, folder.CreatedAt, folder.UpdatedAt).Scan(
		&created.ID,
		&created.WorkspaceID,
		&parentID,
		&created.Name,
		&created.CreatedAt,
		&created.UpdatedAt,
	); err != nil {
		return domain.Folder{}, fmt.Errorf("insert folder: %w", err)
	}
	created.ParentID = parentID

	return created, nil
}

func (r FolderRepository) GetByID(ctx context.Context, folderID string) (domain.Folder, error) {
	query := `SELECT id, workspace_id, parent_id, name, created_at, updated_at FROM folders WHERE id = $1`

	var folder domain.Folder
	var parentID *string
	if err := r.db.QueryRow(ctx, query, folderID).Scan(
		&folder.ID,
		&folder.WorkspaceID,
		&parentID,
		&folder.Name,
		&folder.CreatedAt,
		&folder.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Folder{}, domain.ErrNotFound
		}
		return domain.Folder{}, fmt.Errorf("select folder by id: %w", err)
	}
	folder.ParentID = parentID

	return folder, nil
}

func (r FolderRepository) ListByWorkspaceID(ctx context.Context, workspaceID string) ([]domain.Folder, error) {
	query := `
		SELECT id, workspace_id, parent_id, name, created_at, updated_at
		FROM folders
		WHERE workspace_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.Query(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("query folders: %w", err)
	}
	defer rows.Close()

	folders := make([]domain.Folder, 0)
	for rows.Next() {
		var folder domain.Folder
		var parentID *string
		if err := rows.Scan(
			&folder.ID,
			&folder.WorkspaceID,
			&parentID,
			&folder.Name,
			&folder.CreatedAt,
			&folder.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}
		folder.ParentID = parentID
		folders = append(folders, folder)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate folders: %w", err)
	}

	return folders, nil
}
