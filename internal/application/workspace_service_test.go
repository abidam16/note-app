package application

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"note-app/internal/domain"
)

type fakeWorkspaceRepo struct {
	memberships map[string][]domain.WorkspaceMember
	invitations map[string]domain.WorkspaceInvitation
	owners      map[string]int
	workspaces  map[string]domain.Workspace
}

func workspaceInvitationIsPending(invitation domain.WorkspaceInvitation) bool {
	if invitation.Status != "" {
		return invitation.Status == domain.WorkspaceInvitationStatusPending
	}
	return invitation.AcceptedAt == nil
}

func (r *fakeWorkspaceRepo) CreateWithOwner(_ context.Context, workspace domain.Workspace, member domain.WorkspaceMember) (domain.Workspace, domain.WorkspaceMember, error) {
	r.memberships[workspace.ID] = []domain.WorkspaceMember{member}
	r.owners[workspace.ID] = 1
	if r.workspaces == nil {
		r.workspaces = map[string]domain.Workspace{}
	}
	r.workspaces[workspace.ID] = workspace
	return workspace, member, nil
}

func (r *fakeWorkspaceRepo) HasWorkspaceWithNameForUser(_ context.Context, userID, workspaceName string) (bool, error) {
	for workspaceID, members := range r.memberships {
		for _, member := range members {
			if member.UserID == userID && equalNormalizedName(r.workspaces[workspaceID].Name, workspaceName) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *fakeWorkspaceRepo) HasWorkspaceWithNameForUserExcludingID(_ context.Context, userID, workspaceName, excludeWorkspaceID string) (bool, error) {
	for workspaceID, members := range r.memberships {
		if workspaceID == excludeWorkspaceID {
			continue
		}
		for _, member := range members {
			if member.UserID == userID && equalNormalizedName(r.workspaces[workspaceID].Name, workspaceName) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *fakeWorkspaceRepo) GetByID(_ context.Context, workspaceID string) (domain.Workspace, error) {
	workspace, ok := r.workspaces[workspaceID]
	if !ok {
		return domain.Workspace{}, domain.ErrNotFound
	}
	return workspace, nil
}

func (r *fakeWorkspaceRepo) UpdateName(_ context.Context, workspaceID, name string, updatedAt time.Time) (domain.Workspace, error) {
	workspace, ok := r.workspaces[workspaceID]
	if !ok {
		return domain.Workspace{}, domain.ErrNotFound
	}
	workspace.Name = name
	workspace.UpdatedAt = updatedAt
	r.workspaces[workspaceID] = workspace
	return workspace, nil
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

func (r *fakeWorkspaceRepo) GetMembershipByID(_ context.Context, workspaceID, memberID string) (domain.WorkspaceMember, error) {
	for _, member := range r.memberships[workspaceID] {
		if member.ID == memberID {
			return member, nil
		}
	}
	return domain.WorkspaceMember{}, domain.ErrNotFound
}

func (r *fakeWorkspaceRepo) CreateInvitation(_ context.Context, invitation domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error) {
	if invitation.Status == "" {
		invitation.Status = domain.WorkspaceInvitationStatusPending
	}
	if invitation.Version == 0 {
		invitation.Version = 1
	}
	if invitation.UpdatedAt.IsZero() {
		invitation.UpdatedAt = invitation.CreatedAt
	}
	r.invitations[invitation.ID] = invitation
	return invitation, nil
}

func (r *fakeWorkspaceRepo) GetActiveInvitationByEmail(_ context.Context, workspaceID, email string) (domain.WorkspaceInvitation, error) {
	for _, invitation := range r.invitations {
		if invitation.WorkspaceID == workspaceID && invitation.Email == email && workspaceInvitationIsPending(invitation) {
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

func (r *fakeWorkspaceRepo) AcceptInvitation(_ context.Context, invitationID, userID string, version int64, acceptedAt time.Time) (domain.AcceptInvitationResult, error) {
	invitation, ok := r.invitations[invitationID]
	if !ok {
		return domain.AcceptInvitationResult{}, domain.ErrNotFound
	}
	if !workspaceInvitationIsPending(invitation) {
		return domain.AcceptInvitationResult{}, domain.ErrConflict
	}
	if invitation.Version != version {
		return domain.AcceptInvitationResult{}, domain.ErrConflict
	}
	invitation.AcceptedAt = &acceptedAt
	invitation.Status = domain.WorkspaceInvitationStatusAccepted
	invitation.Version++
	if invitation.Version == 0 {
		invitation.Version = 2
	}
	invitation.UpdatedAt = acceptedAt
	invitation.RespondedBy = &userID
	invitation.RespondedAt = &acceptedAt
	r.invitations[invitationID] = invitation
	member := domain.WorkspaceMember{ID: "member-2", WorkspaceID: invitation.WorkspaceID, UserID: userID, Role: invitation.Role, CreatedAt: acceptedAt}
	r.memberships[invitation.WorkspaceID] = append(r.memberships[invitation.WorkspaceID], member)
	if invitation.Role == domain.RoleOwner {
		r.owners[invitation.WorkspaceID]++
	}
	return domain.AcceptInvitationResult{Invitation: invitation, Membership: member}, nil
}

func (r *fakeWorkspaceRepo) RejectInvitation(_ context.Context, invitationID, userID string, version int64, rejectedAt time.Time) (domain.WorkspaceInvitation, error) {
	invitation, ok := r.invitations[invitationID]
	if !ok {
		return domain.WorkspaceInvitation{}, domain.ErrNotFound
	}
	if !workspaceInvitationIsPending(invitation) {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Version != version {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	invitation.Status = domain.WorkspaceInvitationStatusRejected
	invitation.Version++
	invitation.UpdatedAt = rejectedAt
	invitation.RespondedBy = &userID
	invitation.RespondedAt = &rejectedAt
	invitation.AcceptedAt = nil
	r.invitations[invitationID] = invitation
	return invitation, nil
}

func (r *fakeWorkspaceRepo) CancelInvitation(_ context.Context, invitationID, userID string, version int64, cancelledAt time.Time) (domain.WorkspaceInvitation, error) {
	invitation, ok := r.invitations[invitationID]
	if !ok {
		return domain.WorkspaceInvitation{}, domain.ErrNotFound
	}
	if !workspaceInvitationIsPending(invitation) {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Version != version {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	invitation.Status = domain.WorkspaceInvitationStatusCancelled
	invitation.Version++
	invitation.UpdatedAt = cancelledAt
	invitation.CancelledBy = &userID
	invitation.CancelledAt = &cancelledAt
	invitation.AcceptedAt = nil
	invitation.RespondedBy = nil
	invitation.RespondedAt = nil
	r.invitations[invitationID] = invitation
	return invitation, nil
}

func (r *fakeWorkspaceRepo) UpdateInvitation(_ context.Context, invitationID string, role domain.WorkspaceRole, version int64, updatedAt time.Time) (domain.WorkspaceInvitation, error) {
	invitation, ok := r.invitations[invitationID]
	if !ok {
		return domain.WorkspaceInvitation{}, domain.ErrNotFound
	}
	if !workspaceInvitationIsPending(invitation) {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Version != version {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Role == role {
		return invitation, nil
	}
	invitation.Role = role
	invitation.Version++
	invitation.UpdatedAt = updatedAt
	r.invitations[invitationID] = invitation
	return invitation, nil
}

func (r *fakeWorkspaceRepo) ListWorkspaceInvitations(_ context.Context, workspaceID string, status domain.WorkspaceInvitationStatusFilter, limit int, cursor string) (domain.WorkspaceInvitationList, error) {
	items := make([]domain.WorkspaceInvitation, 0)
	for _, invitation := range r.invitations {
		if invitation.WorkspaceID != workspaceID {
			continue
		}
		if status != domain.WorkspaceInvitationStatusFilterAll && invitation.Status != domain.WorkspaceInvitationStatus(status) {
			continue
		}
		items = append(items, invitation)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	start := 0
	if cursor != "" {
		for idx := range items {
			if items[idx].ID == cursor {
				start = idx + 1
				break
			}
		}
	}
	if start > len(items) {
		start = len(items)
	}
	items = items[start:]
	result := domain.WorkspaceInvitationList{}
	if len(items) > limit {
		result.Items = append(result.Items, items[:limit]...)
		result.HasMore = true
		next := result.Items[len(result.Items)-1].ID
		result.NextCursor = &next
		return result, nil
	}
	result.Items = append(result.Items, items...)
	return result, nil
}

func (r *fakeWorkspaceRepo) ListMyInvitations(_ context.Context, email string, status domain.WorkspaceInvitationStatusFilter, limit int, cursor string) (domain.WorkspaceInvitationList, error) {
	items := make([]domain.WorkspaceInvitation, 0)
	for _, invitation := range r.invitations {
		if !strings.EqualFold(invitation.Email, email) {
			continue
		}
		if status != domain.WorkspaceInvitationStatusFilterAll && invitation.Status != domain.WorkspaceInvitationStatus(status) {
			continue
		}
		items = append(items, invitation)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	start := 0
	if cursor != "" {
		for idx := range items {
			if items[idx].ID == cursor {
				start = idx + 1
				break
			}
		}
	}
	if start > len(items) {
		start = len(items)
	}
	items = items[start:]
	result := domain.WorkspaceInvitationList{}
	if len(items) > limit {
		result.Items = append(result.Items, items[:limit]...)
		result.HasMore = true
		next := result.Items[len(result.Items)-1].ID
		result.NextCursor = &next
		return result, nil
	}
	result.Items = append(result.Items, items...)
	return result, nil
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
	workspaces := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}, workspaces: map[string]domain.Workspace{}}
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
		workspaces:  map[string]domain.Workspace{"workspace-1": {ID: "workspace-1", Name: "Product"}},
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
