package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type fakeWorkspaceRepo struct {
	memberships map[string][]domain.WorkspaceMember
	invitations map[string]domain.WorkspaceInvitation
	owners      map[string]int
}

func (r *fakeWorkspaceRepo) CreateWithOwner(_ context.Context, workspace domain.Workspace, member domain.WorkspaceMember) (domain.Workspace, domain.WorkspaceMember, error) {
	r.memberships[workspace.ID] = []domain.WorkspaceMember{member}
	r.owners[workspace.ID] = 1
	return workspace, member, nil
}

func (r *fakeWorkspaceRepo) HasWorkspaceWithNameForUser(_ context.Context, userID, workspaceName string) (bool, error) {
	return false, nil
}

func (r *fakeWorkspaceRepo) ListByUserID(_ context.Context, userID string) ([]domain.Workspace, error) {
	workspaces := make([]domain.Workspace, 0)
	for workspaceID, members := range r.memberships {
		for _, member := range members {
			if member.UserID == userID {
				workspaces = append(workspaces, domain.Workspace{ID: workspaceID})
				break
			}
		}
	}
	return workspaces, nil
}

func (r *fakeWorkspaceRepo) GetMembershipByUserID(_ context.Context, workspaceID, userID string) (domain.WorkspaceMember, error) {
	for _, member := range r.memberships[workspaceID] {
		if member.UserID == userID {
			return member, nil
		}
	}
	return domain.WorkspaceMember{}, domain.ErrForbidden
}

func (r *fakeWorkspaceRepo) CreateInvitation(_ context.Context, invitation domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error) {
	r.invitations[invitation.ID] = invitation
	return invitation, nil
}

func (r *fakeWorkspaceRepo) GetActiveInvitationByEmail(_ context.Context, workspaceID, email string) (domain.WorkspaceInvitation, error) {
	for _, invitation := range r.invitations {
		if invitation.WorkspaceID == workspaceID && invitation.Email == email && invitation.AcceptedAt == nil {
			return invitation, nil
		}
	}
	return domain.WorkspaceInvitation{}, domain.ErrNotFound
}

func (r *fakeWorkspaceRepo) GetInvitationByID(_ context.Context, invitationID string) (domain.WorkspaceInvitation, error) {
	invitation, ok := r.invitations[invitationID]
	if !ok {
		return domain.WorkspaceInvitation{}, domain.ErrNotFound
	}
	return invitation, nil
}

func (r *fakeWorkspaceRepo) AcceptInvitation(_ context.Context, invitationID, userID string, acceptedAt time.Time) (domain.WorkspaceMember, error) {
	invitation, ok := r.invitations[invitationID]
	if !ok {
		return domain.WorkspaceMember{}, domain.ErrNotFound
	}
	invitation.AcceptedAt = &acceptedAt
	r.invitations[invitationID] = invitation
	member := domain.WorkspaceMember{ID: "member-2", WorkspaceID: invitation.WorkspaceID, UserID: userID, Role: invitation.Role, CreatedAt: acceptedAt}
	r.memberships[invitation.WorkspaceID] = append(r.memberships[invitation.WorkspaceID], member)
	if invitation.Role == domain.RoleOwner {
		r.owners[invitation.WorkspaceID]++
	}
	return member, nil
}

func (r *fakeWorkspaceRepo) ListMembers(_ context.Context, workspaceID string) ([]domain.WorkspaceMember, error) {
	return r.memberships[workspaceID], nil
}

func (r *fakeWorkspaceRepo) UpdateMemberRole(_ context.Context, workspaceID, memberID string, role domain.WorkspaceRole) (domain.WorkspaceMember, error) {
	members := r.memberships[workspaceID]
	for idx, member := range members {
		if member.ID == memberID {
			if member.Role == domain.RoleOwner && role != domain.RoleOwner {
				r.owners[workspaceID]--
			}
			if member.Role != domain.RoleOwner && role == domain.RoleOwner {
				r.owners[workspaceID]++
			}
			member.Role = role
			members[idx] = member
			r.memberships[workspaceID] = members
			return member, nil
		}
	}
	return domain.WorkspaceMember{}, domain.ErrNotFound
}

func (r *fakeWorkspaceRepo) CountOwners(_ context.Context, workspaceID string) (int, error) {
	return r.owners[workspaceID], nil
}

func TestWorkspaceServiceCreateWorkspace(t *testing.T) {
	workspaces := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	users := &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{"user-1": {ID: "user-1", Email: "owner@example.com", FullName: "Owner"}}}
	service := NewWorkspaceService(workspaces, users)

	workspace, member, err := service.CreateWorkspace(context.Background(), "user-1", CreateWorkspaceInput{Name: "Product"})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}

	if workspace.Name != "Product" {
		t.Fatalf("expected workspace name Product, got %s", workspace.Name)
	}
	if member.Role != domain.RoleOwner {
		t.Fatalf("expected owner role, got %s", member.Role)
	}
}

func TestWorkspaceServiceProtectsLastOwner(t *testing.T) {
	workspaces := &fakeWorkspaceRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleOwner},
			},
		},
		invitations: map[string]domain.WorkspaceInvitation{},
		owners:      map[string]int{"workspace-1": 1},
	}
	users := &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{"user-1": {ID: "user-1", Email: "owner@example.com", FullName: "Owner"}}}
	service := NewWorkspaceService(workspaces, users)

	_, err := service.UpdateMemberRole(context.Background(), "user-1", UpdateMemberRoleInput{
		WorkspaceID: "workspace-1",
		MemberID:    "member-1",
		Role:        domain.RoleViewer,
	})
	if !errors.Is(err, domain.ErrLastOwnerRemoval) {
		t.Fatalf("expected last owner protection error, got %v", err)
	}
}
