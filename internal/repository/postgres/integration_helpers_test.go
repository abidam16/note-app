package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mustExec(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		t.Fatalf("exec failed: %v", err)
	}
}

func seedUser(t *testing.T, pool *pgxpool.Pool, email string) domain.User {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	user := domain.User{
		ID:           uuid.NewString(),
		Email:        email,
		FullName:     "User " + email,
		PasswordHash: "hash",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	mustExec(t, pool,
		`INSERT INTO users (id, email, full_name, password_hash, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		user.ID, user.Email, user.FullName, user.PasswordHash, user.CreatedAt, user.UpdatedAt,
	)
	return user
}

func seedWorkspaceWithOwner(t *testing.T, pool *pgxpool.Pool, owner domain.User) (domain.Workspace, domain.WorkspaceMember) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	workspace := domain.Workspace{ID: uuid.NewString(), Name: "Acme", CreatedAt: now, UpdatedAt: now}
	member := domain.WorkspaceMember{ID: uuid.NewString(), WorkspaceID: workspace.ID, UserID: owner.ID, Role: domain.RoleOwner, CreatedAt: now}
	mustExec(t, pool, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ($1,$2,$3,$4)`, workspace.ID, workspace.Name, workspace.CreatedAt, workspace.UpdatedAt)
	mustExec(t, pool, `INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at) VALUES ($1,$2,$3,$4,$5)`, member.ID, member.WorkspaceID, member.UserID, member.Role, member.CreatedAt)
	return workspace, member
}

func seedPageWithDraft(t *testing.T, pool *pgxpool.Pool, workspaceID, userID string, folderID *string, title string) (domain.Page, domain.PageDraft) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	content := json.RawMessage(`[{"type":"paragraph","text":"hello world"}]`)
	page := domain.Page{ID: uuid.NewString(), WorkspaceID: workspaceID, FolderID: folderID, Title: title, CreatedBy: userID, CreatedAt: now, UpdatedAt: now}
	draft := domain.PageDraft{PageID: page.ID, Content: content, SearchBody: "hello world", LastEditedBy: userID, CreatedAt: now, UpdatedAt: now}
	mustExec(t, pool, `INSERT INTO pages (id, workspace_id, folder_id, title, created_by, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, page.ID, page.WorkspaceID, page.FolderID, page.Title, page.CreatedBy, page.CreatedAt, page.UpdatedAt)
	mustExec(t, pool, `INSERT INTO page_drafts (page_id, content, search_body, last_edited_by, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)`, draft.PageID, draft.Content, draft.SearchBody, draft.LastEditedBy, draft.CreatedAt, draft.UpdatedAt)
	return page, draft
}
