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

	if _, err := repo.AcceptInvitation(ctx, createdInvitation.ID, invited.ID, createdInvitation.Version, now.Add(time.Minute)); !errors.Is(err, domain.ErrConflict) {
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

func TestWorkspaceRepositoryUpdateInvitation(t *testing.T) {
	pool := integrationPool(t)
	repo := NewWorkspaceRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	owner := seedUser(t, pool, "owner-update-invitation@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)

	createInvitation := func(email string, role domain.WorkspaceRole, version int64) domain.WorkspaceInvitation {
		invitation, err := repo.CreateInvitation(ctx, domain.WorkspaceInvitation{
			ID:          uuid.NewString(),
			WorkspaceID: workspace.ID,
			Email:       email,
			Role:        role,
			InvitedBy:   owner.ID,
			CreatedAt:   now,
			Status:      domain.WorkspaceInvitationStatusPending,
			Version:     version,
			UpdatedAt:   now,
		})
		if err != nil {
			t.Fatalf("create invitation: %v", err)
		}
		return invitation
	}

	t.Run("updates pending role and bumps version", func(t *testing.T) {
		invitation := createInvitation("pending-update@example.com", domain.RoleViewer, 1)
		updatedAt := now.Add(time.Minute)
		updated, err := repo.UpdateInvitation(ctx, invitation.ID, domain.RoleEditor, invitation.Version, updatedAt)
		if err != nil {
			t.Fatalf("expected update success, got %v", err)
		}
		if updated.Role != domain.RoleEditor || updated.Version != invitation.Version+1 || !updated.UpdatedAt.Equal(updatedAt) {
			t.Fatalf("unexpected updated invitation: %+v", updated)
		}
	})

	t.Run("same role is a no-op", func(t *testing.T) {
		invitation := createInvitation("pending-noop@example.com", domain.RoleViewer, 3)
		updated, err := repo.UpdateInvitation(ctx, invitation.ID, invitation.Role, invitation.Version, now.Add(2*time.Minute))
		if err != nil {
			t.Fatalf("expected same-role success, got %v", err)
		}
		if updated.Version != invitation.Version || !updated.UpdatedAt.Equal(invitation.UpdatedAt) || updated.Role != invitation.Role {
			t.Fatalf("expected unchanged invitation, got %+v", updated)
		}
	})

	t.Run("missing invitation returns not found", func(t *testing.T) {
		if _, err := repo.UpdateInvitation(ctx, uuid.NewString(), domain.RoleEditor, 1, now.Add(3*time.Minute)); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("terminal invitation returns conflict", func(t *testing.T) {
		statuses := []domain.WorkspaceInvitationStatus{
			domain.WorkspaceInvitationStatusAccepted,
			domain.WorkspaceInvitationStatusRejected,
			domain.WorkspaceInvitationStatusCancelled,
		}
		for _, status := range statuses {
			t.Run(string(status), func(t *testing.T) {
				invitation := createInvitation("terminal-"+string(status)+"@example.com", domain.RoleViewer, 4)
				if status == domain.WorkspaceInvitationStatusAccepted {
					acceptedAt := now.Add(4 * time.Minute)
					mustExec(t, pool, `UPDATE workspace_invitations SET status = $2, accepted_at = $3, responded_by = $4, responded_at = $3 WHERE id = $1`, invitation.ID, status, acceptedAt, owner.ID)
				} else if status == domain.WorkspaceInvitationStatusRejected {
					rejectedAt := now.Add(4 * time.Minute)
					mustExec(t, pool, `UPDATE workspace_invitations SET status = $2, responded_by = $3, responded_at = $4 WHERE id = $1`, invitation.ID, status, owner.ID, rejectedAt)
				} else if status == domain.WorkspaceInvitationStatusCancelled {
					cancelledAt := now.Add(4 * time.Minute)
					mustExec(t, pool, `UPDATE workspace_invitations SET status = $2, cancelled_by = $3, cancelled_at = $4 WHERE id = $1`, invitation.ID, status, owner.ID, cancelledAt)
				}
				if _, err := repo.UpdateInvitation(ctx, invitation.ID, domain.RoleEditor, invitation.Version, now.Add(5*time.Minute)); !errors.Is(err, domain.ErrConflict) {
					t.Fatalf("expected conflict for %s invitation, got %v", status, err)
				}
			})
		}
	})

	t.Run("stale version returns conflict", func(t *testing.T) {
		invitation := createInvitation("pending-stale@example.com", domain.RoleViewer, 5)
		if _, err := repo.UpdateInvitation(ctx, invitation.ID, domain.RoleEditor, invitation.Version-1, now.Add(6*time.Minute)); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected stale version conflict, got %v", err)
		}
	})
}

func TestWorkspaceRepositoryRejectInvitation(t *testing.T) {
	pool := integrationPool(t)
	repo := NewWorkspaceRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	owner := seedUser(t, pool, "owner-reject-invitation@example.com")
	target := seedUser(t, pool, "target-reject-invitation@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)

	createInvitation := func(email string, version int64) domain.WorkspaceInvitation {
		invitation, err := repo.CreateInvitation(ctx, domain.WorkspaceInvitation{
			ID:          uuid.NewString(),
			WorkspaceID: workspace.ID,
			Email:       email,
			Role:        domain.RoleEditor,
			InvitedBy:   owner.ID,
			CreatedAt:   now,
			Status:      domain.WorkspaceInvitationStatusPending,
			Version:     version,
			UpdatedAt:   now,
		})
		if err != nil {
			t.Fatalf("create invitation: %v", err)
		}
		return invitation
	}

	t.Run("rejects pending invitation and bumps version", func(t *testing.T) {
		invitation := createInvitation(target.Email, 1)
		rejectedAt := now.Add(time.Minute)
		rejected, err := repo.RejectInvitation(ctx, invitation.ID, target.ID, invitation.Version, rejectedAt)
		if err != nil {
			t.Fatalf("expected reject success, got %v", err)
		}
		if rejected.Status != domain.WorkspaceInvitationStatusRejected || rejected.Version != invitation.Version+1 || !rejected.UpdatedAt.Equal(rejectedAt) {
			t.Fatalf("unexpected rejected invitation: %+v", rejected)
		}
		if rejected.RespondedBy == nil || *rejected.RespondedBy != target.ID || rejected.RespondedAt == nil || !rejected.RespondedAt.Equal(rejectedAt) {
			t.Fatalf("unexpected rejection metadata: %+v", rejected)
		}
		if rejected.AcceptedAt != nil || rejected.CancelledAt != nil || rejected.CancelledBy != nil {
			t.Fatalf("expected accept/cancel fields to remain nil, got %+v", rejected)
		}
	})

	t.Run("missing invitation returns not found", func(t *testing.T) {
		if _, err := repo.RejectInvitation(ctx, uuid.NewString(), target.ID, 1, now.Add(2*time.Minute)); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("terminal invitation returns conflict", func(t *testing.T) {
		statuses := []domain.WorkspaceInvitationStatus{
			domain.WorkspaceInvitationStatusAccepted,
			domain.WorkspaceInvitationStatusRejected,
			domain.WorkspaceInvitationStatusCancelled,
		}
		for _, status := range statuses {
			t.Run(string(status), func(t *testing.T) {
				invitation := createInvitation("terminal-reject-"+string(status)+"@example.com", 4)
				if status == domain.WorkspaceInvitationStatusAccepted {
					acceptedAt := now.Add(3 * time.Minute)
					mustExec(t, pool, `UPDATE workspace_invitations SET status = $2, accepted_at = $3, responded_by = $4, responded_at = $3 WHERE id = $1`, invitation.ID, status, acceptedAt, target.ID)
				} else if status == domain.WorkspaceInvitationStatusRejected {
					rejectedAt := now.Add(3 * time.Minute)
					mustExec(t, pool, `UPDATE workspace_invitations SET status = $2, responded_by = $3, responded_at = $4 WHERE id = $1`, invitation.ID, status, target.ID, rejectedAt)
				} else {
					cancelledAt := now.Add(3 * time.Minute)
					mustExec(t, pool, `UPDATE workspace_invitations SET status = $2, cancelled_by = $3, cancelled_at = $4 WHERE id = $1`, invitation.ID, status, owner.ID, cancelledAt)
				}
				if _, err := repo.RejectInvitation(ctx, invitation.ID, target.ID, invitation.Version, now.Add(4*time.Minute)); !errors.Is(err, domain.ErrConflict) {
					t.Fatalf("expected conflict for %s invitation, got %v", status, err)
				}
			})
		}
	})

	t.Run("stale version returns conflict", func(t *testing.T) {
		invitation := createInvitation("pending-reject-stale@example.com", 5)
		if _, err := repo.RejectInvitation(ctx, invitation.ID, target.ID, invitation.Version-1, now.Add(5*time.Minute)); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected stale version conflict, got %v", err)
		}
	})
}

func TestWorkspaceRepositoryCancelInvitation(t *testing.T) {
	pool := integrationPool(t)
	repo := NewWorkspaceRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	owner := seedUser(t, pool, "owner-cancel-invitation@example.com")
	memberOwner := seedUser(t, pool, "second-owner-cancel-invitation@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)
	mustExec(t, pool, `INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at) VALUES ($1,$2,$3,$4,$5)`, uuid.NewString(), workspace.ID, memberOwner.ID, domain.RoleOwner, now)

	createInvitation := func(email string, version int64) domain.WorkspaceInvitation {
		invitation, err := repo.CreateInvitation(ctx, domain.WorkspaceInvitation{
			ID:          uuid.NewString(),
			WorkspaceID: workspace.ID,
			Email:       email,
			Role:        domain.RoleEditor,
			InvitedBy:   owner.ID,
			CreatedAt:   now,
			Status:      domain.WorkspaceInvitationStatusPending,
			Version:     version,
			UpdatedAt:   now,
		})
		if err != nil {
			t.Fatalf("create invitation: %v", err)
		}
		return invitation
	}

	t.Run("cancels pending invitation and bumps version", func(t *testing.T) {
		invitation := createInvitation("pending-cancel@example.com", 1)
		cancelledAt := now.Add(time.Minute)
		cancelled, err := repo.CancelInvitation(ctx, invitation.ID, memberOwner.ID, invitation.Version, cancelledAt)
		if err != nil {
			t.Fatalf("expected cancel success, got %v", err)
		}
		if cancelled.Status != domain.WorkspaceInvitationStatusCancelled || cancelled.Version != invitation.Version+1 || !cancelled.UpdatedAt.Equal(cancelledAt) {
			t.Fatalf("unexpected cancelled invitation: %+v", cancelled)
		}
		if cancelled.CancelledBy == nil || *cancelled.CancelledBy != memberOwner.ID || cancelled.CancelledAt == nil || !cancelled.CancelledAt.Equal(cancelledAt) {
			t.Fatalf("unexpected cancel metadata: %+v", cancelled)
		}
		if cancelled.AcceptedAt != nil || cancelled.RespondedAt != nil || cancelled.RespondedBy != nil {
			t.Fatalf("expected accept/respond fields to remain nil, got %+v", cancelled)
		}
	})

	t.Run("missing invitation returns not found", func(t *testing.T) {
		if _, err := repo.CancelInvitation(ctx, uuid.NewString(), memberOwner.ID, 1, now.Add(2*time.Minute)); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("terminal invitation returns conflict", func(t *testing.T) {
		statuses := []domain.WorkspaceInvitationStatus{
			domain.WorkspaceInvitationStatusAccepted,
			domain.WorkspaceInvitationStatusRejected,
			domain.WorkspaceInvitationStatusCancelled,
		}
		for _, status := range statuses {
			t.Run(string(status), func(t *testing.T) {
				invitation := createInvitation("terminal-cancel-"+string(status)+"@example.com", 4)
				if status == domain.WorkspaceInvitationStatusAccepted {
					acceptedAt := now.Add(3 * time.Minute)
					mustExec(t, pool, `UPDATE workspace_invitations SET status = $2, accepted_at = $3, responded_by = $4, responded_at = $3 WHERE id = $1`, invitation.ID, status, acceptedAt, memberOwner.ID)
				} else if status == domain.WorkspaceInvitationStatusRejected {
					rejectedAt := now.Add(3 * time.Minute)
					mustExec(t, pool, `UPDATE workspace_invitations SET status = $2, responded_by = $3, responded_at = $4 WHERE id = $1`, invitation.ID, status, memberOwner.ID, rejectedAt)
				} else {
					cancelledAt := now.Add(3 * time.Minute)
					mustExec(t, pool, `UPDATE workspace_invitations SET status = $2, cancelled_by = $3, cancelled_at = $4 WHERE id = $1`, invitation.ID, status, memberOwner.ID, cancelledAt)
				}
				if _, err := repo.CancelInvitation(ctx, invitation.ID, memberOwner.ID, invitation.Version, now.Add(4*time.Minute)); !errors.Is(err, domain.ErrConflict) {
					t.Fatalf("expected conflict for %s invitation, got %v", status, err)
				}
			})
		}
	})

	t.Run("stale version returns conflict", func(t *testing.T) {
		invitation := createInvitation("pending-cancel-stale@example.com", 5)
		if _, err := repo.CancelInvitation(ctx, invitation.ID, memberOwner.ID, invitation.Version-1, now.Add(5*time.Minute)); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected stale version conflict, got %v", err)
		}
	})
}
