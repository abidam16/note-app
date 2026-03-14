package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

func TestPageRepositoryAdditionalIntegrationBranches(t *testing.T) {
	pool := integrationPool(t)
	repo := NewPageRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	owner := seedUser(t, pool, "page-extra-owner@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)

	pageIDWithoutDraft := uuid.NewString()
	mustExec(t, pool, `INSERT INTO pages (id, workspace_id, folder_id, title, created_by, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, pageIDWithoutDraft, workspace.ID, nil, "No Draft", owner.ID, now, now)

	if _, _, err := repo.GetByID(ctx, pageIDWithoutDraft); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found when draft missing, got %v", err)
	}

	if _, err := repo.UpdateDraft(ctx, pageIDWithoutDraft, json.RawMessage(`[]`), owner.ID, now.Add(time.Minute)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected update draft not found when draft row missing, got %v", err)
	}

	if _, _, err := repo.CreateWithDraft(ctx,
		domain.Page{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), Title: "Bad Workspace", CreatedBy: owner.ID, CreatedAt: now, UpdatedAt: now},
		domain.PageDraft{PageID: uuid.NewString(), Content: json.RawMessage(`[]`), LastEditedBy: owner.ID, CreatedAt: now, UpdatedAt: now},
	); err == nil {
		t.Fatal("expected create with draft to fail for invalid workspace")
	}

	badDraftPage := domain.Page{ID: uuid.NewString(), WorkspaceID: workspace.ID, Title: "Bad Draft", CreatedBy: owner.ID, CreatedAt: now, UpdatedAt: now}
	if _, _, err := repo.CreateWithDraft(ctx,
		badDraftPage,
		domain.PageDraft{PageID: badDraftPage.ID, Content: json.RawMessage(`[]`), LastEditedBy: uuid.NewString(), CreatedAt: now, UpdatedAt: now},
	); err == nil {
		t.Fatal("expected create with draft to fail for invalid draft editor")
	}
}

func TestWorkspaceRepositoryAcceptInvitationUniqueConflict(t *testing.T) {
	pool := integrationPool(t)
	repo := NewWorkspaceRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	owner := seedUser(t, pool, "owner-extra@example.com")
	invited := seedUser(t, pool, "invited-extra@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)

	invitation := domain.WorkspaceInvitation{ID: uuid.NewString(), WorkspaceID: workspace.ID, Email: invited.Email, Role: domain.RoleEditor, InvitedBy: owner.ID, CreatedAt: now}
	createdInvitation, err := repo.CreateInvitation(ctx, invitation)
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}

	mustExec(t, pool, `INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at) VALUES ($1,$2,$3,$4,$5)`, uuid.NewString(), workspace.ID, invited.ID, domain.RoleEditor, now)

	if _, err := repo.AcceptInvitation(ctx, createdInvitation.ID, invited.ID, now.Add(time.Minute)); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict when member already exists, got %v", err)
	}
}

func TestWorkspaceRepositoryInternalNotFoundBranches(t *testing.T) {
	pool := integrationPool(t)
	repo := NewWorkspaceRepository(pool)
	ctx := context.Background()

	if _, err := repo.getUser(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected getUser not found, got %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := repo.getInvitationForUpdate(ctx, tx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected getInvitationForUpdate not found, got %v", err)
	}
}
