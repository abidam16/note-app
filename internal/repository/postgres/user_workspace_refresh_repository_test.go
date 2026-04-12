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
	ownerSecondWorkspace := domain.Workspace{ID: uuid.NewString(), Name: "Product", CreatedAt: now, UpdatedAt: now}
	ownerSecondMember := domain.WorkspaceMember{ID: uuid.NewString(), WorkspaceID: ownerSecondWorkspace.ID, UserID: owner.ID, Role: domain.RoleOwner, CreatedAt: now}
	if _, _, err := repo.CreateWithOwner(ctx, ownerSecondWorkspace, ownerSecondMember); err != nil {
		t.Fatalf("create owner second workspace: %v", err)
	}

	ownerWorkspaces, err := repo.ListByUserID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("list workspaces by owner: %v", err)
	}
	if len(ownerWorkspaces) != 2 {
		t.Fatalf("owner should only see own workspace, got %+v", ownerWorkspaces)
	}
	ownerWorkspaceIDs := map[string]bool{}
	for _, item := range ownerWorkspaces {
		ownerWorkspaceIDs[item.ID] = true
	}
	if !ownerWorkspaceIDs[workspace.ID] || !ownerWorkspaceIDs[ownerSecondWorkspace.ID] {
		t.Fatalf("owner workspace list missing expected workspaces: %+v", ownerWorkspaces)
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

	hasDuplicate, err := repo.HasWorkspaceWithNameForUserExcludingID(ctx, owner.ID, ownerSecondWorkspace.Name, workspace.ID)
	if err != nil || !hasDuplicate {
		t.Fatalf("expected duplicate workspace name with exclusion lookup, err=%v exists=%t", err, hasDuplicate)
	}
	hasDuplicate, err = repo.HasWorkspaceWithNameForUserExcludingID(ctx, owner.ID, workspace.Name, workspace.ID)
	if err != nil || hasDuplicate {
		t.Fatalf("expected excluded workspace name not to conflict, err=%v exists=%t", err, hasDuplicate)
	}

	if _, err := repo.GetMembershipByUserID(ctx, workspace.ID, uuid.NewString()); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden on missing member, got %v", err)
	}
	if _, err := repo.GetMembershipByID(ctx, workspace.ID, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected membership by id not found, got %v", err)
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
	if createdInv.Status != domain.WorkspaceInvitationStatusPending {
		t.Fatalf("expected created invitation status pending, got %q", createdInv.Status)
	}
	if createdInv.Version != 1 {
		t.Fatalf("expected created invitation version 1, got %d", createdInv.Version)
	}
	if !createdInv.UpdatedAt.Equal(createdInv.CreatedAt) {
		t.Fatalf("expected created invitation updated_at to equal created_at, got updated_at=%s created_at=%s", createdInv.UpdatedAt, createdInv.CreatedAt)
	}
	if createdInv.RespondedBy != nil || createdInv.RespondedAt != nil {
		t.Fatalf("expected created invitation response fields to be nil, got responded_by=%v responded_at=%v", createdInv.RespondedBy, createdInv.RespondedAt)
	}
	if createdInv.CancelledBy != nil || createdInv.CancelledAt != nil {
		t.Fatalf("expected created invitation cancel fields to be nil, got cancelled_by=%v cancelled_at=%v", createdInv.CancelledBy, createdInv.CancelledAt)
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
	if activeInv.Status != domain.WorkspaceInvitationStatusPending {
		t.Fatalf("expected active invitation status pending, got %q", activeInv.Status)
	}

	byID, err := repo.GetInvitationByID(ctx, createdInv.ID)
	if err != nil || byID.Email != invited.Email {
		t.Fatalf("get invitation by id mismatch: err=%v email=%s", err, byID.Email)
	}
	if byID.Status != domain.WorkspaceInvitationStatusPending {
		t.Fatalf("expected invitation by id status pending before accept, got %q", byID.Status)
	}

	acceptedAt := now.Add(time.Minute)
	accepted, err := repo.AcceptInvitation(ctx, createdInv.ID, invited.ID, createdInv.Version, acceptedAt)
	if err != nil {
		t.Fatalf("accept invitation: %v", err)
	}
	acceptedMember := accepted.Membership
	if acceptedMember.UserID != invited.ID || acceptedMember.Role != domain.RoleEditor {
		t.Fatalf("accepted member mismatch: user=%s role=%s", acceptedMember.UserID, acceptedMember.Role)
	}
	if accepted.Invitation.Status != domain.WorkspaceInvitationStatusAccepted || accepted.Invitation.Version != 2 {
		t.Fatalf("unexpected accepted invitation result: %+v", accepted.Invitation)
	}

	memberByID, err := repo.GetMembershipByID(ctx, workspace.ID, acceptedMember.ID)
	if err != nil || memberByID.UserID != invited.ID || memberByID.Role != domain.RoleEditor {
		t.Fatalf("get membership by id mismatch: err=%v member=%+v", err, memberByID)
	}

	acceptedInvitation, err := repo.GetInvitationByID(ctx, createdInv.ID)
	if err != nil {
		t.Fatalf("get accepted invitation by id: %v", err)
	}
	if acceptedInvitation.Status != domain.WorkspaceInvitationStatusAccepted {
		t.Fatalf("expected accepted invitation status accepted, got %q", acceptedInvitation.Status)
	}
	if acceptedInvitation.Version != 2 {
		t.Fatalf("expected accepted invitation version 2, got %d", acceptedInvitation.Version)
	}
	if acceptedInvitation.AcceptedAt == nil || !acceptedInvitation.AcceptedAt.Equal(acceptedAt) {
		t.Fatalf("expected accepted_at %s, got %v", acceptedAt, acceptedInvitation.AcceptedAt)
	}
	if !acceptedInvitation.UpdatedAt.Equal(acceptedAt) {
		t.Fatalf("expected updated_at %s after accept, got %s", acceptedAt, acceptedInvitation.UpdatedAt)
	}
	if acceptedInvitation.RespondedBy == nil || *acceptedInvitation.RespondedBy != invited.ID {
		t.Fatalf("expected responded_by %s, got %v", invited.ID, acceptedInvitation.RespondedBy)
	}
	if acceptedInvitation.RespondedAt == nil || !acceptedInvitation.RespondedAt.Equal(acceptedAt) {
		t.Fatalf("expected responded_at %s, got %v", acceptedAt, acceptedInvitation.RespondedAt)
	}
	if acceptedInvitation.CancelledBy != nil || acceptedInvitation.CancelledAt != nil {
		t.Fatalf("expected cancelled fields to remain nil after accept, got cancelled_by=%v cancelled_at=%v", acceptedInvitation.CancelledBy, acceptedInvitation.CancelledAt)
	}

	if _, err := repo.AcceptInvitation(ctx, createdInv.ID, invited.ID, 1, now.Add(2*time.Minute)); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected accept conflict, got %v", err)
	}
	if _, err := repo.AcceptInvitation(ctx, uuid.NewString(), invited.ID, 1, now); !errors.Is(err, domain.ErrNotFound) {
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

func TestWorkspaceRepositoryListInvitationsIntegration(t *testing.T) {
	pool := integrationPool(t)
	repo := NewWorkspaceRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	owner := seedUser(t, pool, "list-owner@example.com")
	invitedA := seedUser(t, pool, "list-a@example.com")
	invitedB := seedUser(t, pool, "list-b@example.com")
	invitedC := seedUser(t, pool, "list-c@example.com")
	workspace, _ := seedWorkspaceWithOwner(t, pool, owner)

	pendingOld, err := repo.CreateInvitation(ctx, domain.WorkspaceInvitation{
		ID:          uuid.NewString(),
		WorkspaceID: workspace.ID,
		Email:       invitedA.Email,
		Role:        domain.RoleViewer,
		InvitedBy:   owner.ID,
		CreatedAt:   now.Add(-3 * time.Minute),
		Status:      domain.WorkspaceInvitationStatusPending,
		Version:     1,
		UpdatedAt:   now.Add(-3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create pending old invitation: %v", err)
	}

	acceptedInvitation, err := repo.CreateInvitation(ctx, domain.WorkspaceInvitation{
		ID:          uuid.NewString(),
		WorkspaceID: workspace.ID,
		Email:       invitedB.Email,
		Role:        domain.RoleEditor,
		InvitedBy:   owner.ID,
		CreatedAt:   now.Add(-2 * time.Minute),
		Status:      domain.WorkspaceInvitationStatusPending,
		Version:     1,
		UpdatedAt:   now.Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create accepted invitation seed: %v", err)
	}
	if _, err := repo.AcceptInvitation(ctx, acceptedInvitation.ID, invitedB.ID, acceptedInvitation.Version, now.Add(-90*time.Second)); err != nil {
		t.Fatalf("accept invitation seed: %v", err)
	}

	cancelledID := uuid.NewString()
	cancelledAt := now.Add(-time.Minute)
	mustExec(t, pool, `
		INSERT INTO workspace_invitations (
			id, workspace_id, email, role, invited_by, accepted_at, created_at, status, version, updated_at, cancelled_by, cancelled_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, cancelledID, workspace.ID, invitedC.Email, domain.RoleViewer, owner.ID, nil, now.Add(-time.Minute), domain.WorkspaceInvitationStatusCancelled, 2, cancelledAt, owner.ID, cancelledAt)

	allPage, err := repo.ListWorkspaceInvitations(ctx, workspace.ID, domain.WorkspaceInvitationStatusFilterAll, 2, "")
	if err != nil {
		t.Fatalf("list all invitations first page: %v", err)
	}
	if len(allPage.Items) != 2 {
		t.Fatalf("expected first page size 2, got %+v", allPage)
	}
	if allPage.Items[0].ID != cancelledID || allPage.Items[1].ID != acceptedInvitation.ID {
		t.Fatalf("unexpected all invitation ordering: %+v", allPage.Items)
	}
	if !allPage.HasMore || allPage.NextCursor == nil {
		t.Fatalf("expected next cursor on first invitation page, got %+v", allPage)
	}

	nextPage, err := repo.ListWorkspaceInvitations(ctx, workspace.ID, domain.WorkspaceInvitationStatusFilterAll, 2, *allPage.NextCursor)
	if err != nil {
		t.Fatalf("list all invitations second page: %v", err)
	}
	if len(nextPage.Items) != 1 || nextPage.Items[0].ID != pendingOld.ID {
		t.Fatalf("unexpected second invitation page: %+v", nextPage)
	}
	if nextPage.HasMore || nextPage.NextCursor != nil {
		t.Fatalf("expected final invitation page without next cursor, got %+v", nextPage)
	}

	pendingOnly, err := repo.ListWorkspaceInvitations(ctx, workspace.ID, domain.WorkspaceInvitationStatusFilterPending, 10, "")
	if err != nil {
		t.Fatalf("list pending invitations: %v", err)
	}
	if len(pendingOnly.Items) != 1 || pendingOnly.Items[0].ID != pendingOld.ID || pendingOnly.Items[0].Status != domain.WorkspaceInvitationStatusPending {
		t.Fatalf("unexpected pending invitation list: %+v", pendingOnly)
	}

	acceptedOnly, err := repo.ListWorkspaceInvitations(ctx, workspace.ID, domain.WorkspaceInvitationStatusFilterAccepted, 10, "")
	if err != nil {
		t.Fatalf("list accepted invitations: %v", err)
	}
	if len(acceptedOnly.Items) != 1 || acceptedOnly.Items[0].ID != acceptedInvitation.ID || acceptedOnly.Items[0].Status != domain.WorkspaceInvitationStatusAccepted {
		t.Fatalf("unexpected accepted invitation list: %+v", acceptedOnly)
	}

	if _, err := repo.ListWorkspaceInvitations(ctx, workspace.ID, domain.WorkspaceInvitationStatusFilterAll, 10, "broken"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid cursor validation error, got %v", err)
	}
}

func TestWorkspaceRepositoryListMyInvitationsIntegration(t *testing.T) {
	pool := integrationPool(t)
	repo := NewWorkspaceRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	ownerA := seedUser(t, pool, "my-owner-a@example.com")
	ownerB := seedUser(t, pool, "my-owner-b@example.com")
	member := seedUser(t, pool, "my-member@example.com")
	other := seedUser(t, pool, "my-other@example.com")
	workspaceA, _ := seedWorkspaceWithOwner(t, pool, ownerA)
	workspaceB, _ := seedWorkspaceWithOwner(t, pool, ownerB)

	pendingOld, err := repo.CreateInvitation(ctx, domain.WorkspaceInvitation{
		ID:          uuid.NewString(),
		WorkspaceID: workspaceA.ID,
		Email:       member.Email,
		Role:        domain.RoleViewer,
		InvitedBy:   ownerA.ID,
		CreatedAt:   now.Add(-3 * time.Minute),
		Status:      domain.WorkspaceInvitationStatusPending,
		Version:     1,
		UpdatedAt:   now.Add(-3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create my pending old invitation: %v", err)
	}

	acceptedInvitation, err := repo.CreateInvitation(ctx, domain.WorkspaceInvitation{
		ID:          uuid.NewString(),
		WorkspaceID: workspaceB.ID,
		Email:       member.Email,
		Role:        domain.RoleEditor,
		InvitedBy:   ownerB.ID,
		CreatedAt:   now.Add(-2 * time.Minute),
		Status:      domain.WorkspaceInvitationStatusPending,
		Version:     1,
		UpdatedAt:   now.Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create my accepted invitation seed: %v", err)
	}
	if _, err := repo.AcceptInvitation(ctx, acceptedInvitation.ID, member.ID, acceptedInvitation.Version, now.Add(-90*time.Second)); err != nil {
		t.Fatalf("accept my invitation seed: %v", err)
	}

	cancelledID := uuid.NewString()
	cancelledAt := now.Add(-time.Minute)
	mustExec(t, pool, `
		INSERT INTO workspace_invitations (
			id, workspace_id, email, role, invited_by, accepted_at, created_at, status, version, updated_at, cancelled_by, cancelled_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, cancelledID, workspaceA.ID, member.Email, domain.RoleViewer, ownerA.ID, nil, now.Add(-time.Minute), domain.WorkspaceInvitationStatusCancelled, 2, cancelledAt, ownerA.ID, cancelledAt)

	if _, err := repo.CreateInvitation(ctx, domain.WorkspaceInvitation{
		ID:          uuid.NewString(),
		WorkspaceID: workspaceA.ID,
		Email:       other.Email,
		Role:        domain.RoleViewer,
		InvitedBy:   ownerA.ID,
		CreatedAt:   now.Add(-30 * time.Second),
		Status:      domain.WorkspaceInvitationStatusPending,
		Version:     1,
		UpdatedAt:   now.Add(-30 * time.Second),
	}); err != nil {
		t.Fatalf("create other-email invitation seed: %v", err)
	}

	allPage, err := repo.ListMyInvitations(ctx, member.Email, domain.WorkspaceInvitationStatusFilterAll, 2, "")
	if err != nil {
		t.Fatalf("list my invitations first page: %v", err)
	}
	if len(allPage.Items) != 2 {
		t.Fatalf("expected first my-invitation page size 2, got %+v", allPage)
	}
	if allPage.Items[0].ID != cancelledID || allPage.Items[1].ID != acceptedInvitation.ID {
		t.Fatalf("unexpected my invitation ordering: %+v", allPage.Items)
	}
	if !allPage.HasMore || allPage.NextCursor == nil {
		t.Fatalf("expected next cursor on first my invitation page, got %+v", allPage)
	}

	nextPage, err := repo.ListMyInvitations(ctx, member.Email, domain.WorkspaceInvitationStatusFilterAll, 2, *allPage.NextCursor)
	if err != nil {
		t.Fatalf("list my invitations second page: %v", err)
	}
	if len(nextPage.Items) != 1 || nextPage.Items[0].ID != pendingOld.ID {
		t.Fatalf("unexpected second my invitation page: %+v", nextPage)
	}
	if nextPage.HasMore || nextPage.NextCursor != nil {
		t.Fatalf("expected final my invitation page without next cursor, got %+v", nextPage)
	}

	pendingOnly, err := repo.ListMyInvitations(ctx, member.Email, domain.WorkspaceInvitationStatusFilterPending, 10, "")
	if err != nil {
		t.Fatalf("list my pending invitations: %v", err)
	}
	if len(pendingOnly.Items) != 1 || pendingOnly.Items[0].ID != pendingOld.ID || pendingOnly.Items[0].Status != domain.WorkspaceInvitationStatusPending {
		t.Fatalf("unexpected my pending invitation list: %+v", pendingOnly)
	}

	acceptedOnly, err := repo.ListMyInvitations(ctx, member.Email, domain.WorkspaceInvitationStatusFilterAccepted, 10, "")
	if err != nil {
		t.Fatalf("list my accepted invitations: %v", err)
	}
	if len(acceptedOnly.Items) != 1 || acceptedOnly.Items[0].ID != acceptedInvitation.ID || acceptedOnly.Items[0].Status != domain.WorkspaceInvitationStatusAccepted {
		t.Fatalf("unexpected my accepted invitation list: %+v", acceptedOnly)
	}

	if _, err := repo.ListMyInvitations(ctx, member.Email, domain.WorkspaceInvitationStatusFilterAll, 10, "broken"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected invalid my invitation cursor validation error, got %v", err)
	}
}
