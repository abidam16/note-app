package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

type WorkspaceRepository interface {
	CreateWithOwner(ctx context.Context, workspace domain.Workspace, member domain.WorkspaceMember) (domain.Workspace, domain.WorkspaceMember, error)
	HasWorkspaceWithNameForUser(ctx context.Context, userID, workspaceName string) (bool, error)
	GetByID(ctx context.Context, workspaceID string) (domain.Workspace, error)
	UpdateName(ctx context.Context, workspaceID, name string, updatedAt time.Time) (domain.Workspace, error)
	ListByUserID(ctx context.Context, userID string) ([]domain.Workspace, error)
	GetMembershipByUserID(ctx context.Context, workspaceID, userID string) (domain.WorkspaceMember, error)
	CreateInvitation(ctx context.Context, invitation domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error)
	GetActiveInvitationByEmail(ctx context.Context, workspaceID, email string) (domain.WorkspaceInvitation, error)
	GetInvitationByID(ctx context.Context, invitationID string) (domain.WorkspaceInvitation, error)
	AcceptInvitation(ctx context.Context, invitationID, userID string, acceptedAt time.Time) (domain.WorkspaceMember, error)
	ListMembers(ctx context.Context, workspaceID string) ([]domain.WorkspaceMember, error)
	UpdateMemberRole(ctx context.Context, workspaceID, memberID string, role domain.WorkspaceRole) (domain.WorkspaceMember, error)
	CountOwners(ctx context.Context, workspaceID string) (int, error)
}

type CreateWorkspaceInput struct {
	Name string
}

type InviteMemberInput struct {
	WorkspaceID string
	Email       string
	Role        domain.WorkspaceRole
}

type RenameWorkspaceInput struct {
	WorkspaceID string
	Name        string
}

type UpdateMemberRoleInput struct {
	WorkspaceID string
	MemberID    string
	Role        domain.WorkspaceRole
}

type WorkspaceService struct {
	workspaces    WorkspaceRepository
	users         UserRepository
	notifications NotificationEventPublisher
}

func NewWorkspaceService(workspaces WorkspaceRepository, users UserRepository, notifications ...NotificationEventPublisher) WorkspaceService {
	service := WorkspaceService{workspaces: workspaces, users: users}
	if len(notifications) > 0 {
		service.notifications = notifications[0]
	}
	return service
}

func (s WorkspaceService) CreateWorkspace(ctx context.Context, actorID string, input CreateWorkspaceInput) (domain.Workspace, domain.WorkspaceMember, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.Workspace{}, domain.WorkspaceMember{}, fmt.Errorf("%w: workspace name is required", domain.ErrValidation)
	}
	if strings.TrimSpace(actorID) == "" {
		return domain.Workspace{}, domain.WorkspaceMember{}, domain.ErrUnauthorized
	}
	if _, err := s.users.GetByID(ctx, actorID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Workspace{}, domain.WorkspaceMember{}, domain.ErrUnauthorized
		}
		return domain.Workspace{}, domain.WorkspaceMember{}, err
	}

	hasName, err := s.workspaces.HasWorkspaceWithNameForUser(ctx, actorID, name)
	if err != nil {
		return domain.Workspace{}, domain.WorkspaceMember{}, err
	}
	if hasName {
		return domain.Workspace{}, domain.WorkspaceMember{}, fmt.Errorf("%w: workspace name already exists", domain.ErrValidation)
	}

	now := time.Now().UTC()
	workspace := domain.Workspace{
		ID:        uuid.NewString(),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
	member := domain.WorkspaceMember{
		ID:          uuid.NewString(),
		WorkspaceID: workspace.ID,
		UserID:      actorID,
		Role:        domain.RoleOwner,
		CreatedAt:   now,
	}

	return s.workspaces.CreateWithOwner(ctx, workspace, member)
}

func (s WorkspaceService) ListWorkspaces(ctx context.Context, actorID string) ([]domain.Workspace, error) {
	if strings.TrimSpace(actorID) == "" {
		return nil, domain.ErrUnauthorized
	}
	if _, err := s.users.GetByID(ctx, actorID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}
	return s.workspaces.ListByUserID(ctx, actorID)
}

func (s WorkspaceService) RenameWorkspace(ctx context.Context, actorID string, input RenameWorkspaceInput) (domain.Workspace, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.Workspace{}, fmt.Errorf("%w: workspace name is required", domain.ErrValidation)
	}

	membership, err := s.workspaces.GetMembershipByUserID(ctx, input.WorkspaceID, actorID)
	if err != nil {
		return domain.Workspace{}, err
	}
	if membership.Role != domain.RoleOwner {
		return domain.Workspace{}, domain.ErrForbidden
	}

	workspace, err := s.workspaces.GetByID(ctx, input.WorkspaceID)
	if err != nil {
		return domain.Workspace{}, err
	}

	if !equalNormalizedName(workspace.Name, name) {
		hasName, err := s.workspaces.HasWorkspaceWithNameForUser(ctx, actorID, name)
		if err != nil {
			return domain.Workspace{}, err
		}
		if hasName {
			return domain.Workspace{}, fmt.Errorf("%w: workspace name already exists", domain.ErrValidation)
		}
	}

	return s.workspaces.UpdateName(ctx, input.WorkspaceID, name, time.Now().UTC())
}

func (s WorkspaceService) InviteMember(ctx context.Context, actorID string, input InviteMemberInput) (domain.WorkspaceInvitation, error) {
	if !domain.IsValidWorkspaceRole(input.Role) {
		return domain.WorkspaceInvitation{}, fmt.Errorf("%w: invalid role", domain.ErrValidation)
	}

	membership, err := s.workspaces.GetMembershipByUserID(ctx, input.WorkspaceID, actorID)
	if err != nil {
		return domain.WorkspaceInvitation{}, err
	}
	if membership.Role != domain.RoleOwner {
		return domain.WorkspaceInvitation{}, domain.ErrForbidden
	}

	email, err := normalizeEmail(input.Email)
	if err != nil {
		return domain.WorkspaceInvitation{}, fmt.Errorf("%w: invalid email", domain.ErrValidation)
	}
	if _, err := s.users.GetByEmail(ctx, email); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.WorkspaceInvitation{}, fmt.Errorf("%w: invitee must be a registered user", domain.ErrValidation)
		}
		return domain.WorkspaceInvitation{}, err
	}

	_, err = s.workspaces.GetActiveInvitationByEmail(ctx, input.WorkspaceID, email)
	switch {
	case err == nil:
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	case !errors.Is(err, domain.ErrNotFound):
		return domain.WorkspaceInvitation{}, err
	}

	now := time.Now().UTC()
	invitation := domain.WorkspaceInvitation{
		ID:          uuid.NewString(),
		WorkspaceID: input.WorkspaceID,
		Email:       email,
		Role:        input.Role,
		InvitedBy:   actorID,
		CreatedAt:   now,
	}

	saved, err := s.workspaces.CreateInvitation(ctx, invitation)
	if err != nil {
		return domain.WorkspaceInvitation{}, err
	}

	if s.notifications != nil {
		if err := s.notifications.NotifyInvitationCreated(ctx, saved); err != nil {
			return domain.WorkspaceInvitation{}, err
		}
	}

	return saved, nil
}

func (s WorkspaceService) AcceptInvitation(ctx context.Context, actorID, invitationID string) (domain.WorkspaceMember, error) {
	user, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return domain.WorkspaceMember{}, err
	}

	invitation, err := s.workspaces.GetInvitationByID(ctx, invitationID)
	if err != nil {
		return domain.WorkspaceMember{}, err
	}

	if invitation.AcceptedAt != nil {
		return domain.WorkspaceMember{}, domain.ErrConflict
	}
	if !strings.EqualFold(user.Email, invitation.Email) {
		return domain.WorkspaceMember{}, domain.ErrInvitationEmailMismatch
	}

	return s.workspaces.AcceptInvitation(ctx, invitationID, actorID, time.Now().UTC())
}

func (s WorkspaceService) ListMembers(ctx context.Context, actorID, workspaceID string) ([]domain.WorkspaceMember, error) {
	if _, err := s.workspaces.GetMembershipByUserID(ctx, workspaceID, actorID); err != nil {
		return nil, err
	}
	return s.workspaces.ListMembers(ctx, workspaceID)
}

func (s WorkspaceService) UpdateMemberRole(ctx context.Context, actorID string, input UpdateMemberRoleInput) (domain.WorkspaceMember, error) {
	if !domain.IsValidWorkspaceRole(input.Role) {
		return domain.WorkspaceMember{}, fmt.Errorf("%w: invalid role", domain.ErrValidation)
	}

	actorMembership, err := s.workspaces.GetMembershipByUserID(ctx, input.WorkspaceID, actorID)
	if err != nil {
		return domain.WorkspaceMember{}, err
	}
	if actorMembership.Role != domain.RoleOwner {
		return domain.WorkspaceMember{}, domain.ErrForbidden
	}

	targetMembers, err := s.workspaces.ListMembers(ctx, input.WorkspaceID)
	if err != nil {
		return domain.WorkspaceMember{}, err
	}

	for _, member := range targetMembers {
		if member.ID == input.MemberID && member.Role == domain.RoleOwner && input.Role != domain.RoleOwner {
			ownerCount, err := s.workspaces.CountOwners(ctx, input.WorkspaceID)
			if err != nil {
				return domain.WorkspaceMember{}, err
			}
			if ownerCount <= 1 {
				return domain.WorkspaceMember{}, domain.ErrLastOwnerRemoval
			}
			break
		}
	}

	return s.workspaces.UpdateMemberRole(ctx, input.WorkspaceID, input.MemberID, input.Role)
}

func equalNormalizedName(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}
