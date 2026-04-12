package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"note-app/internal/application"
	"note-app/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PageRepository struct {
	db *pgxpool.Pool
}

func NewPageRepository(db *pgxpool.Pool) PageRepository {
	return PageRepository{db: db}
}

func (r PageRepository) CreateWithDraft(ctx context.Context, page domain.Page, draft domain.PageDraft) (domain.Page, domain.PageDraft, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Page{}, domain.PageDraft{}, fmt.Errorf("begin page transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO pages (id, workspace_id, folder_id, title, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, page.ID, page.WorkspaceID, page.FolderID, page.Title, page.CreatedBy, page.CreatedAt, page.UpdatedAt); err != nil {
		return domain.Page{}, domain.PageDraft{}, fmt.Errorf("insert page: %w", err)
	}

	draft.SearchBody = application.ExtractSearchBody(draft.Content)
	if _, err := tx.Exec(ctx, `
		INSERT INTO page_drafts (page_id, content, search_body, last_edited_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, draft.PageID, draft.Content, draft.SearchBody, draft.LastEditedBy, draft.CreatedAt, draft.UpdatedAt); err != nil {
		return domain.Page{}, domain.PageDraft{}, fmt.Errorf("insert page draft: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Page{}, domain.PageDraft{}, fmt.Errorf("commit page creation: %w", err)
	}

	return page, draft, nil
}

func (r PageRepository) GetByID(ctx context.Context, pageID string) (domain.Page, domain.PageDraft, error) {
	query := `
		SELECT
			p.id,
			p.workspace_id,
			p.folder_id,
			p.title,
			p.created_by,
			p.created_at,
			p.updated_at,
			d.page_id,
			d.content,
			d.search_body,
			d.last_edited_by,
			d.created_at,
			d.updated_at
		FROM pages p
		JOIN page_drafts d ON d.page_id = p.id
		WHERE p.id = $1
		  AND p.deleted_at IS NULL
	`

	var page domain.Page
	var draft domain.PageDraft
	var folderID *string
	if err := r.db.QueryRow(ctx, query, pageID).Scan(
		&page.ID,
		&page.WorkspaceID,
		&folderID,
		&page.Title,
		&page.CreatedBy,
		&page.CreatedAt,
		&page.UpdatedAt,
		&draft.PageID,
		&draft.Content,
		&draft.SearchBody,
		&draft.LastEditedBy,
		&draft.CreatedAt,
		&draft.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
		}
		return domain.Page{}, domain.PageDraft{}, fmt.Errorf("select page with draft: %w", err)
	}
	page.FolderID = folderID

	return page, draft, nil
}

func (r PageRepository) GetVisibleByUserID(ctx context.Context, pageID string, userID string) (domain.Page, domain.PageDraft, error) {
	query := `
		SELECT
			p.id,
			p.workspace_id,
			p.folder_id,
			p.title,
			p.created_by,
			p.created_at,
			p.updated_at,
			d.page_id,
			d.content,
			d.search_body,
			d.last_edited_by,
			d.created_at,
			d.updated_at
		FROM pages p
		JOIN page_drafts d ON d.page_id = p.id
		JOIN workspace_members wm ON wm.workspace_id = p.workspace_id
		WHERE p.id = $1
		  AND wm.user_id = $2
		  AND p.deleted_at IS NULL
	`

	var page domain.Page
	var draft domain.PageDraft
	var folderID *string
	if err := r.db.QueryRow(ctx, query, pageID, userID).Scan(
		&page.ID,
		&page.WorkspaceID,
		&folderID,
		&page.Title,
		&page.CreatedBy,
		&page.CreatedAt,
		&page.UpdatedAt,
		&draft.PageID,
		&draft.Content,
		&draft.SearchBody,
		&draft.LastEditedBy,
		&draft.CreatedAt,
		&draft.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
		}
		return domain.Page{}, domain.PageDraft{}, fmt.Errorf("select visible page with draft: %w", err)
	}
	page.FolderID = folderID

	return page, draft, nil
}

func (r PageRepository) GetTrashedByTrashItemID(ctx context.Context, trashItemID string) (domain.TrashItem, domain.Page, domain.PageDraft, error) {
	query := `
		SELECT
			t.id,
			t.workspace_id,
			t.page_id,
			t.page_title,
			t.deleted_by,
			t.deleted_at,
			p.id,
			p.workspace_id,
			p.folder_id,
			p.title,
			p.created_by,
			p.created_at,
			p.updated_at,
			d.page_id,
			d.content,
			d.search_body,
			d.last_edited_by,
			d.created_at,
			d.updated_at
		FROM trash_items t
		JOIN pages p ON p.id = t.page_id
		JOIN page_drafts d ON d.page_id = p.id
		WHERE t.id = $1
		  AND t.restored_at IS NULL
		  AND p.deleted_at IS NOT NULL
	`

	var trashItem domain.TrashItem
	var page domain.Page
	var draft domain.PageDraft
	var folderID *string
	if err := r.db.QueryRow(ctx, query, trashItemID).Scan(
		&trashItem.ID,
		&trashItem.WorkspaceID,
		&trashItem.PageID,
		&trashItem.PageTitle,
		&trashItem.DeletedBy,
		&trashItem.DeletedAt,
		&page.ID,
		&page.WorkspaceID,
		&folderID,
		&page.Title,
		&page.CreatedBy,
		&page.CreatedAt,
		&page.UpdatedAt,
		&draft.PageID,
		&draft.Content,
		&draft.SearchBody,
		&draft.LastEditedBy,
		&draft.CreatedAt,
		&draft.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TrashItem{}, domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
		}
		return domain.TrashItem{}, domain.Page{}, domain.PageDraft{}, fmt.Errorf("select trashed page with draft: %w", err)
	}
	page.FolderID = folderID

	return trashItem, page, draft, nil
}

func (r PageRepository) ListByWorkspaceIDAndFolderID(ctx context.Context, workspaceID string, folderID *string) ([]domain.PageSummary, error) {
	query := `
		SELECT id, workspace_id, folder_id, title, updated_at
		FROM pages
		WHERE workspace_id = $1
		  AND deleted_at IS NULL
		  AND (($2::uuid IS NULL AND folder_id IS NULL) OR folder_id = $2::uuid)
		ORDER BY updated_at DESC, id ASC
	`

	rows, err := r.db.Query(ctx, query, workspaceID, folderID)
	if err != nil {
		return nil, fmt.Errorf("list pages by workspace and folder: %w", err)
	}
	defer rows.Close()

	pages := make([]domain.PageSummary, 0)
	for rows.Next() {
		var page domain.PageSummary
		if err := rows.Scan(&page.ID, &page.WorkspaceID, &page.FolderID, &page.Title, &page.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan page summary: %w", err)
		}
		pages = append(pages, page)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate page summaries: %w", err)
	}

	return pages, nil
}

func (r PageRepository) UpdateMetadata(ctx context.Context, pageID string, title string, folderID *string, updatedAt time.Time) (domain.Page, error) {
	query := `
		UPDATE pages
		SET title = $2, folder_id = $3, updated_at = $4
		WHERE id = $1
		  AND deleted_at IS NULL
		RETURNING id, workspace_id, folder_id, title, created_by, created_at, updated_at
	`

	var page domain.Page
	var returnedFolderID *string
	if err := r.db.QueryRow(ctx, query, pageID, title, folderID, updatedAt).Scan(
		&page.ID,
		&page.WorkspaceID,
		&returnedFolderID,
		&page.Title,
		&page.CreatedBy,
		&page.CreatedAt,
		&page.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Page{}, domain.ErrNotFound
		}
		return domain.Page{}, fmt.Errorf("update page metadata: %w", err)
	}
	page.FolderID = returnedFolderID

	return page, nil
}

func (r PageRepository) UpdateDraft(ctx context.Context, pageID string, content json.RawMessage, lastEditedBy string, updatedAt time.Time) (domain.PageDraft, error) {
	searchBody := application.ExtractSearchBody(content)
	query := `
		WITH updated_draft AS (
			UPDATE page_drafts AS d
			SET content = $2, search_body = $3, last_edited_by = $5, updated_at = $4
			FROM pages AS p
			WHERE d.page_id = $1
			  AND p.id = d.page_id
			  AND p.deleted_at IS NULL
			RETURNING d.page_id, d.content, d.search_body, d.last_edited_by, d.created_at, d.updated_at
		), touched_page AS (
			UPDATE pages AS p
			SET updated_at = $4
			FROM updated_draft AS d
			WHERE p.id = d.page_id
			RETURNING p.id
		)
		SELECT page_id, content, search_body, last_edited_by, created_at, updated_at
		FROM updated_draft
	`

	var draft domain.PageDraft
	if err := r.db.QueryRow(ctx, query, pageID, content, searchBody, updatedAt, lastEditedBy).Scan(
		&draft.PageID,
		&draft.Content,
		&draft.SearchBody,
		&draft.LastEditedBy,
		&draft.CreatedAt,
		&draft.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PageDraft{}, domain.ErrNotFound
		}
		return domain.PageDraft{}, fmt.Errorf("update page draft: %w", err)
	}

	return draft, nil
}

func (r PageRepository) SearchPages(ctx context.Context, workspaceID string, query string) ([]domain.PageSearchResult, error) {
	searchQuery := `
		WITH search_term AS (
			SELECT plainto_tsquery('simple', $2) AS term
		)
		SELECT p.id, p.workspace_id, p.folder_id, p.title, p.updated_at
		FROM pages p
		JOIN page_drafts d ON d.page_id = p.id
		CROSS JOIN search_term q
		WHERE p.workspace_id = $1
		  AND p.deleted_at IS NULL
		  AND (
			p.title_search @@ q.term
			OR d.search_body_vector @@ q.term
		  )
		ORDER BY p.updated_at DESC, p.id ASC
	`

	rows, err := r.db.Query(ctx, searchQuery, workspaceID, query)
	if err != nil {
		return nil, fmt.Errorf("search pages: %w", err)
	}
	defer rows.Close()

	results := make([]domain.PageSearchResult, 0)
	for rows.Next() {
		var result domain.PageSearchResult
		if err := rows.Scan(&result.ID, &result.WorkspaceID, &result.FolderID, &result.Title, &result.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}

	return results, nil
}

func (r PageRepository) SoftDelete(ctx context.Context, trashItem domain.TrashItem) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin page soft delete transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		UPDATE pages
		SET deleted_at = $2, deleted_by = $3, updated_at = $2
		WHERE id = $1
		  AND deleted_at IS NULL
	`, trashItem.PageID, trashItem.DeletedAt, trashItem.DeletedBy)
	if err != nil {
		return fmt.Errorf("soft delete page: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO trash_items (id, workspace_id, page_id, page_title, deleted_by, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, trashItem.ID, trashItem.WorkspaceID, trashItem.PageID, trashItem.PageTitle, trashItem.DeletedBy, trashItem.DeletedAt); err != nil {
		return fmt.Errorf("insert trash item: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit page soft delete: %w", err)
	}

	return nil
}

func (r PageRepository) ListTrashByWorkspaceID(ctx context.Context, workspaceID string) ([]domain.TrashItem, error) {
	query := `
		SELECT id, workspace_id, page_id, page_title, deleted_by, deleted_at
		FROM trash_items
		WHERE workspace_id = $1
		  AND restored_at IS NULL
		ORDER BY deleted_at DESC, id ASC
	`

	rows, err := r.db.Query(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list trash items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.TrashItem, 0)
	for rows.Next() {
		var item domain.TrashItem
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.PageID, &item.PageTitle, &item.DeletedBy, &item.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan trash item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trash items: %w", err)
	}

	return items, nil
}

func (r PageRepository) GetTrashItemByID(ctx context.Context, trashItemID string) (domain.TrashItem, error) {
	query := `
		SELECT id, workspace_id, page_id, page_title, deleted_by, deleted_at
		FROM trash_items
		WHERE id = $1
		  AND restored_at IS NULL
	`

	var item domain.TrashItem
	if err := r.db.QueryRow(ctx, query, trashItemID).Scan(&item.ID, &item.WorkspaceID, &item.PageID, &item.PageTitle, &item.DeletedBy, &item.DeletedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TrashItem{}, domain.ErrNotFound
		}
		return domain.TrashItem{}, fmt.Errorf("select trash item by id: %w", err)
	}

	return item, nil
}

func (r PageRepository) RestoreTrashItem(ctx context.Context, trashItemID string, restoredBy string, restoredAt time.Time) (domain.Page, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Page{}, fmt.Errorf("begin trash restore transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var item domain.TrashItem
	if err := tx.QueryRow(ctx, `
		SELECT id, workspace_id, page_id, page_title, deleted_by, deleted_at
		FROM trash_items
		WHERE id = $1
		  AND restored_at IS NULL
	`, trashItemID).Scan(&item.ID, &item.WorkspaceID, &item.PageID, &item.PageTitle, &item.DeletedBy, &item.DeletedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Page{}, domain.ErrNotFound
		}
		return domain.Page{}, fmt.Errorf("load trash item for restore: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE pages
		SET deleted_at = NULL, deleted_by = NULL, updated_at = $2
		WHERE id = $1
	`, item.PageID, restoredAt); err != nil {
		return domain.Page{}, fmt.Errorf("restore page from trash: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE trash_items
		SET restored_by = $2, restored_at = $3
		WHERE id = $1
	`, item.ID, restoredBy, restoredAt); err != nil {
		return domain.Page{}, fmt.Errorf("mark trash item restored: %w", err)
	}

	var page domain.Page
	var folderID *string
	if err := tx.QueryRow(ctx, `
		SELECT id, workspace_id, folder_id, title, created_by, created_at, updated_at
		FROM pages
		WHERE id = $1
	`, item.PageID).Scan(&page.ID, &page.WorkspaceID, &folderID, &page.Title, &page.CreatedBy, &page.CreatedAt, &page.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Page{}, domain.ErrNotFound
		}
		return domain.Page{}, fmt.Errorf("select restored page: %w", err)
	}
	page.FolderID = folderID

	if err := tx.Commit(ctx); err != nil {
		return domain.Page{}, fmt.Errorf("commit trash restore: %w", err)
	}

	return page, nil
}
