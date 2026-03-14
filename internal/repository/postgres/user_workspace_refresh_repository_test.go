package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

func TestUserRepositoryIntegration(t *testing.T) {
	pool := integrationPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	created, err := repo.Create(ctx, domain.User{
		ID:           uuid.NewString(),
		Email:        "u1@example.com",
		FullName:     "User One",
		PasswordHash: "hash",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if created.Email != "u1@example.com" {
		t.Fatalf("unexpected created email: %s", created.Email)
	}

	if _, err := repo.Create(ctx, domain.User{
		ID:           uuid.NewString(),
		Email:        "u1@example.com",
		FullName:     "Dup",
		PasswordHash: "hash",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); !errors.Is(err, domain.ErrEmailAlreadyUsed) {
		t.Fatalf("expected ErrEmailAlreadyUsed, got %v", err)
	}

	byEmail, err := repo.GetByEmail(ctx, created.Email)
	if err != nil || byEmail.ID != created.ID {
		t.Fatalf("get by email mismatch: err=%v id=%s", err, byEmail.ID)
	}

	byID, err := repo.GetByID(ctx, created.ID)
	if err != nil || byID.Email != created.Email {
		t.Fatalf("get by id mismatch: err=%v email=%s", err, byID.Email)
	}

	if _, err := repo.GetByEmail(ctx, "missing@example.com"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found by email, got %v", err)
	}
	if _, err := repo.GetByID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found by id, got %v", err)
	}
}

func TestRefreshTokenRepositoryIntegration(t *testing.T) {
	pool := integrationPool(t)
	repo := NewRefreshTokenRepository(pool)
	ctx := context.Background()
	user := seedUser(t, pool, "token-user@example.com")
	now := time.Now().UTC().Truncate(time.Microsecond)

	token := domain.RefreshToken{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		TokenHash: "hash-1",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now,
	}

	created, err := repo.Create(ctx, token)
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	if created.TokenHash != token.TokenHash {
		t.Fatalf("unexpected token hash: %s", created.TokenHash)
	}

	fetched, err := repo.GetByHash(ctx, token.TokenHash)
	if err != nil || fetched.ID != token.ID {
		t.Fatalf("get by hash mismatch: err=%v id=%s", err, fetched.ID)
	}

	revokedAt := now.Add(2 * time.Minute)
	if err := repo.RevokeByID(ctx, token.ID, revokedAt); err != nil {
		t.Fatalf("revoke token: %v", err)
	}

	fetched, err = repo.GetByHash(ctx, token.TokenHash)
	if err != nil {
		t.Fatalf("get token after revoke: %v", err)
	}
	if fetched.RevokedAt == nil {
		t.Fatal("expected revoked_at to be set")
	}

	if _, err := repo.GetByHash(ctx, "missing-hash"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found by hash, got %v", err)
	}
}

func TestWorkspaceRepositoryIntegration(t *testing.T) {
	pool := integrationPool(t)
	repo := NewWorkspaceRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	owner := seedUser(t, pool, "owner@example.com")
	invited := seedUser(t, pool, "invited@example.com")

	workspace := domain.Workspace{ID: uuid.NewString(), Name: "WS", CreatedAt: now, UpdatedAt: now}
	ownerMember := domain.WorkspaceMember{ID: uuid.NewString(), WorkspaceID: workspace.ID, UserID: owner.ID, Role: domain.RoleOwner, CreatedAt: now}
	if _, _, err := repo.CreateWithOwner(ctx, workspace, ownerMember); err != nil {
		t.Fatalf("create workspace with owner: %v", err)
	}
	otherWorkspace := domain.Workspace{ID: uuid.NewString(), Name: "Other", CreatedAt: now, UpdatedAt: now}
	otherOwner := seedUser(t, pool, "isolated-owner@example.com")
	otherMember := domain.WorkspaceMember{ID: uuid.NewString(), WorkspaceID: otherWorkspace.ID, UserID: otherOwner.ID, Role: domain.RoleOwner, CreatedAt: now}
	if _, _, err := repo.CreateWithOwner(ctx, otherWorkspace, otherMember); err != nil {
		t.Fatalf("create isolated workspace: %v", err)
	}

	ownerWorkspaces, err := repo.ListByUserID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("list workspaces by owner: %v", err)
	}
	if len(ownerWorkspaces) != 1 || ownerWorkspaces[0].ID != workspace.ID {
		t.Fatalf("owner should only see own workspace, got %+v", ownerWorkspaces)
	}

	otherOwnerWorkspaces, err := repo.ListByUserID(ctx, otherOwner.ID)
	if err != nil {
		t.Fatalf("list workspaces by isolated owner: %v", err)
	}
	if len(otherOwnerWorkspaces) != 1 || otherOwnerWorkspaces[0].ID != otherWorkspace.ID {
		t.Fatalf("isolated owner should only see own workspace, got %+v", otherOwnerWorkspaces)
	}

	fetchedWorkspace, err := repo.GetByID(ctx, workspace.ID)
	if err != nil || fetchedWorkspace.Name != workspace.Name {
		t.Fatalf("get workspace mismatch: err=%v name=%s", err, fetchedWorkspace.Name)
	}

	membership, err := repo.GetMembershipByUserID(ctx, workspace.ID, owner.ID)
	if err != nil || membership.Role != domain.RoleOwner {
		t.Fatalf("get membership mismatch: err=%v role=%s", err, membership.Role)
	}
	if membership.User == nil || membership.User.PasswordHash != "" {
		t.Fatal("expected member user and sanitized password")
	}

	if _, err := repo.GetMembershipByUserID(ctx, workspace.ID, uuid.NewString()); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden on missing member, got %v", err)
	}

	inv := domain.WorkspaceInvitation{
		ID:          uuid.NewString(),
		WorkspaceID: workspace.ID,
		Email:       invited.Email,
		Role:        domain.RoleEditor,
		InvitedBy:   owner.ID,
		CreatedAt:   now,
	}
	createdInv, err := repo.CreateInvitation(ctx, inv)
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}

	if _, err := repo.CreateInvitation(ctx, domain.WorkspaceInvitation{
		ID:          uuid.NewString(),
		WorkspaceID: workspace.ID,
		Email:       invited.Email,
		Role:        domain.RoleEditor,
		InvitedBy:   owner.ID,
		CreatedAt:   now.Add(time.Second),
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected invitation conflict, got %v", err)
	}

	activeInv, err := repo.GetActiveInvitationByEmail(ctx, workspace.ID, invited.Email)
	if err != nil || activeInv.ID != createdInv.ID {
		t.Fatalf("get active invitation mismatch: err=%v id=%s", err, activeInv.ID)
	}

	byID, err := repo.GetInvitationByID(ctx, createdInv.ID)
	if err != nil || byID.Email != invited.Email {
		t.Fatalf("get invitation by id mismatch: err=%v email=%s", err, byID.Email)
	}

	acceptedMember, err := repo.AcceptInvitation(ctx, createdInv.ID, invited.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("accept invitation: %v", err)
	}
	if acceptedMember.UserID != invited.ID || acceptedMember.Role != domain.RoleEditor {
		t.Fatalf("accepted member mismatch: user=%s role=%s", acceptedMember.UserID, acceptedMember.Role)
	}

	if _, err := repo.AcceptInvitation(ctx, createdInv.ID, invited.ID, now.Add(2*time.Minute)); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected accept conflict, got %v", err)
	}
	if _, err := repo.AcceptInvitation(ctx, uuid.NewString(), invited.ID, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected accept not found, got %v", err)
	}

	members, err := repo.ListMembers(ctx, workspace.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("list members mismatch: err=%v len=%d", err, len(members))
	}

	updated, err := repo.UpdateMemberRole(ctx, workspace.ID, acceptedMember.ID, domain.RoleViewer)
	if err != nil {
		t.Fatalf("update member role: %v", err)
	}
	if updated.Role != domain.RoleViewer || updated.User == nil || updated.User.PasswordHash != "" {
		t.Fatalf("updated member mismatch: role=%s user_nil=%t", updated.Role, updated.User == nil)
	}

	if _, err := repo.UpdateMemberRole(ctx, workspace.ID, uuid.NewString(), domain.RoleEditor); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected update role not found, got %v", err)
	}

	owners, err := repo.CountOwners(ctx, workspace.ID)
	if err != nil || owners != 1 {
		t.Fatalf("owner count mismatch: err=%v count=%d", err, owners)
	}

	renamedWorkspace, err := repo.UpdateName(ctx, workspace.ID, "WS Renamed", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("rename workspace: %v", err)
	}
	if renamedWorkspace.Name != "WS Renamed" {
		t.Fatalf("expected renamed workspace, got %s", renamedWorkspace.Name)
	}

	if _, err := repo.GetActiveInvitationByEmail(ctx, workspace.ID, invited.Email); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected no active invitation, got %v", err)
	}
	if _, err := repo.GetByID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected workspace not found, got %v", err)
	}
	if _, err := repo.GetInvitationByID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected invitation not found, got %v", err)
	}
	if _, err := repo.UpdateName(ctx, uuid.NewString(), "Missing", now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected rename workspace not found, got %v", err)
	}
}
