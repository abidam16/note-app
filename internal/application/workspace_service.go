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
	HasWorkspaceWithNameForUserExcludingID(ctx context.Context, userID, workspaceName, excludeWorkspaceID string) (bool, error)
	UpdateName(ctx context.Context, workspaceID, name string, updatedAt time.Time) (domain.Workspace, error)
	ListByUserID(ctx context.Context, userID string) ([]domain.Workspace, error)
	GetMembershipByUserID(ctx context.Context, workspaceID, userID string) (domain.WorkspaceMember, error)
	GetMembershipByID(ctx context.Context, workspaceID, memberID string) (domain.WorkspaceMember, error)
	CreateInvitation(ctx context.Context, invitation domain.WorkspaceInvitation) (domain.WorkspaceInvitation, error)
	GetActiveInvitationByEmail(ctx context.Context, workspaceID, email string) (domain.WorkspaceInvitation, error)
	GetInvitationByID(ctx context.Context, invitationID string) (domain.WorkspaceInvitation, error)
	AcceptInvitation(ctx context.Context, invitationID, userID string, version int64, acceptedAt time.Time) (domain.AcceptInvitationResult, error)
	RejectInvitation(ctx context.Context, invitationID, userID string, version int64, rejectedAt time.Time) (domain.WorkspaceInvitation, error)
	CancelInvitation(ctx context.Context, invitationID, userID string, version int64, cancelledAt time.Time) (domain.WorkspaceInvitation, error)
	UpdateInvitation(ctx context.Context, invitationID string, role domain.WorkspaceRole, version int64, updatedAt time.Time) (domain.WorkspaceInvitation, error)
	ListWorkspaceInvitations(ctx context.Context, workspaceID string, status domain.WorkspaceInvitationStatusFilter, limit int, cursor string) (domain.WorkspaceInvitationList, error)
	ListMyInvitations(ctx context.Context, email string, status domain.WorkspaceInvitationStatusFilter, limit int, cursor string) (domain.WorkspaceInvitationList, error)
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

type ListWorkspaceInvitationsInput struct {
	WorkspaceID string
	Status      domain.WorkspaceInvitationStatusFilter
	Limit       int
	Cursor      string
}

type ListMyInvitationsInput struct {
	Status domain.WorkspaceInvitationStatusFilter
	Limit  int
	Cursor string
}

type UpdateInvitationInput struct {
	InvitationID string
	Role         domain.WorkspaceRole
	Version      int64
}

type AcceptInvitationInput struct {
	InvitationID string
	Version      int64
}

type RejectInvitationInput struct {
	InvitationID string
	Version      int64
}

type CancelInvitationInput struct {
	InvitationID string
	Version      int64
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

	hasName, err := s.workspaces.HasWorkspaceWithNameForUserExcludingID(ctx, actorID, name, input.WorkspaceID)
	if err != nil {
		return domain.Workspace{}, err
	}
	if hasName {
		return domain.Workspace{}, fmt.Errorf("%w: workspace name already exists", domain.ErrValidation)
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

	actor, err := s.users.GetByID(ctx, actorID)
	switch {
	case err == nil:
		if strings.EqualFold(actor.Email, email) {
			return domain.WorkspaceInvitation{}, domain.ErrInvitationSelfEmail
		}
	case errors.Is(err, domain.ErrNotFound):
		return domain.WorkspaceInvitation{}, domain.ErrUnauthorized
	case err != nil:
		return domain.WorkspaceInvitation{}, err
	}

	user, err := s.users.GetByEmail(ctx, email)
	switch {
	case err == nil:
		_, membershipErr := s.workspaces.GetMembershipByUserID(ctx, input.WorkspaceID, user.ID)
		switch {
		case membershipErr == nil:
			return domain.WorkspaceInvitation{}, domain.ErrInvitationExistingMember
		case errors.Is(membershipErr, domain.ErrForbidden):
		default:
			return domain.WorkspaceInvitation{}, membershipErr
		}
	case errors.Is(err, domain.ErrNotFound):
		return domain.WorkspaceInvitation{}, domain.ErrInvitationUnregistered
	case err != nil:
		return domain.WorkspaceInvitation{}, err
	}

	_, err = s.workspaces.GetActiveInvitationByEmail(ctx, input.WorkspaceID, email)
	switch {
	case err == nil:
		return domain.WorkspaceInvitation{}, domain.ErrInvitationDuplicatePending
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
		Status:      domain.WorkspaceInvitationStatusPending,
		Version:     1,
		UpdatedAt:   now,
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

func (s WorkspaceService) AcceptInvitation(ctx context.Context, actorID string, input AcceptInvitationInput) (domain.AcceptInvitationResult, error) {
	if input.Version <= 0 {
		return domain.AcceptInvitationResult{}, fmt.Errorf("%w: version must be greater than zero", domain.ErrValidation)
	}

	user, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.AcceptInvitationResult{}, domain.ErrUnauthorized
		}
		return domain.AcceptInvitationResult{}, err
	}

	invitation, err := s.workspaces.GetInvitationByID(ctx, input.InvitationID)
	if err != nil {
		return domain.AcceptInvitationResult{}, err
	}

	if invitation.Status != domain.WorkspaceInvitationStatusPending {
		return domain.AcceptInvitationResult{}, domain.ErrConflict
	}
	if !strings.EqualFold(user.Email, invitation.Email) {
		return domain.AcceptInvitationResult{}, domain.ErrNotFound
	}

	return s.workspaces.AcceptInvitation(ctx, input.InvitationID, actorID, input.Version, time.Now().UTC())
}

func (s WorkspaceService) RejectInvitation(ctx context.Context, actorID string, input RejectInvitationInput) (domain.WorkspaceInvitation, error) {
	if input.Version <= 0 {
		return domain.WorkspaceInvitation{}, fmt.Errorf("%w: version must be greater than zero", domain.ErrValidation)
	}

	user, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.WorkspaceInvitation{}, domain.ErrUnauthorized
		}
		return domain.WorkspaceInvitation{}, err
	}

	invitation, err := s.workspaces.GetInvitationByID(ctx, input.InvitationID)
	if err != nil {
		return domain.WorkspaceInvitation{}, err
	}
	if invitation.Status != domain.WorkspaceInvitationStatusPending {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if !strings.EqualFold(user.Email, invitation.Email) {
		return domain.WorkspaceInvitation{}, domain.ErrNotFound
	}

	return s.workspaces.RejectInvitation(ctx, input.InvitationID, actorID, input.Version, time.Now().UTC())
}

func (s WorkspaceService) CancelInvitation(ctx context.Context, actorID string, input CancelInvitationInput) (domain.WorkspaceInvitation, error) {
	if input.Version <= 0 {
		return domain.WorkspaceInvitation{}, fmt.Errorf("%w: version must be greater than zero", domain.ErrValidation)
	}

	if _, err := s.users.GetByID(ctx, actorID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.WorkspaceInvitation{}, domain.ErrUnauthorized
		}
		return domain.WorkspaceInvitation{}, err
	}

	invitation, err := s.workspaces.GetInvitationByID(ctx, input.InvitationID)
	if err != nil {
		return domain.WorkspaceInvitation{}, err
	}

	membership, err := s.workspaces.GetMembershipByUserID(ctx, invitation.WorkspaceID, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrForbidden) {
			return domain.WorkspaceInvitation{}, domain.ErrNotFound
		}
		return domain.WorkspaceInvitation{}, err
	}
	if membership.Role != domain.RoleOwner {
		return domain.WorkspaceInvitation{}, domain.ErrForbidden
	}
	if invitation.Status != domain.WorkspaceInvitationStatusPending {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}

	return s.workspaces.CancelInvitation(ctx, input.InvitationID, actorID, input.Version, time.Now().UTC())
}

func (s WorkspaceService) UpdateInvitation(ctx context.Context, actorID string, input UpdateInvitationInput) (domain.WorkspaceInvitation, error) {
	if !domain.IsValidWorkspaceRole(input.Role) {
		return domain.WorkspaceInvitation{}, fmt.Errorf("%w: invalid role", domain.ErrValidation)
	}
	if input.Version <= 0 {
		return domain.WorkspaceInvitation{}, fmt.Errorf("%w: version must be greater than zero", domain.ErrValidation)
	}

	invitation, err := s.workspaces.GetInvitationByID(ctx, input.InvitationID)
	if err != nil {
		return domain.WorkspaceInvitation{}, err
	}

	membership, err := s.workspaces.GetMembershipByUserID(ctx, invitation.WorkspaceID, actorID)
	if err != nil {
		return domain.WorkspaceInvitation{}, err
	}
	if membership.Role != domain.RoleOwner {
		return domain.WorkspaceInvitation{}, domain.ErrForbidden
	}

	if invitation.Status != domain.WorkspaceInvitationStatusPending {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Version != input.Version {
		return domain.WorkspaceInvitation{}, domain.ErrConflict
	}
	if invitation.Role == input.Role {
		return invitation, nil
	}

	return s.workspaces.UpdateInvitation(ctx, input.InvitationID, input.Role, input.Version, time.Now().UTC())
}

func (s WorkspaceService) ListWorkspaceInvitations(ctx context.Context, actorID string, input ListWorkspaceInvitationsInput) (domain.WorkspaceInvitationList, error) {
	status := input.Status
	if status == "" {
		status = domain.WorkspaceInvitationStatusFilterAll
	}
	switch status {
	case domain.WorkspaceInvitationStatusFilterAll,
		domain.WorkspaceInvitationStatusFilterPending,
		domain.WorkspaceInvitationStatusFilterAccepted,
		domain.WorkspaceInvitationStatusFilterRejected,
		domain.WorkspaceInvitationStatusFilterCancelled:
	default:
		return domain.WorkspaceInvitationList{}, fmt.Errorf("%w: invalid status", domain.ErrValidation)
	}

	limit := input.Limit
	switch {
	case limit < 0:
		limit = 50
	case limit == 0 || limit > 100:
		return domain.WorkspaceInvitationList{}, fmt.Errorf("%w: invalid limit", domain.ErrValidation)
	}

	membership, err := s.workspaces.GetMembershipByUserID(ctx, input.WorkspaceID, actorID)
	if err != nil {
		return domain.WorkspaceInvitationList{}, err
	}
	if membership.Role != domain.RoleOwner {
		return domain.WorkspaceInvitationList{}, domain.ErrForbidden
	}

	return s.workspaces.ListWorkspaceInvitations(ctx, input.WorkspaceID, status, limit, strings.TrimSpace(input.Cursor))
}

func (s WorkspaceService) ListMyInvitations(ctx context.Context, actorID string, input ListMyInvitationsInput) (domain.WorkspaceInvitationList, error) {
	user, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.WorkspaceInvitationList{}, domain.ErrUnauthorized
		}
		return domain.WorkspaceInvitationList{}, err
	}

	status := input.Status
	if status == "" {
		status = domain.WorkspaceInvitationStatusFilterAll
	}
	switch status {
	case domain.WorkspaceInvitationStatusFilterAll,
		domain.WorkspaceInvitationStatusFilterPending,
		domain.WorkspaceInvitationStatusFilterAccepted,
		domain.WorkspaceInvitationStatusFilterRejected,
		domain.WorkspaceInvitationStatusFilterCancelled:
	default:
		return domain.WorkspaceInvitationList{}, fmt.Errorf("%w: invalid status", domain.ErrValidation)
	}

	limit := input.Limit
	switch {
	case limit < 0:
		limit = 50
	case limit == 0 || limit > 100:
		return domain.WorkspaceInvitationList{}, fmt.Errorf("%w: invalid limit", domain.ErrValidation)
	}

	return s.workspaces.ListMyInvitations(ctx, user.Email, status, limit, strings.TrimSpace(input.Cursor))
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

	targetMember, err := s.workspaces.GetMembershipByID(ctx, input.WorkspaceID, input.MemberID)
	if err != nil {
		return domain.WorkspaceMember{}, err
	}

	if targetMember.Role == domain.RoleOwner && input.Role != domain.RoleOwner {
		ownerCount, err := s.workspaces.CountOwners(ctx, input.WorkspaceID)
		if err != nil {
			return domain.WorkspaceMember{}, err
		}
		if ownerCount <= 1 {
			return domain.WorkspaceMember{}, domain.ErrLastOwnerRemoval
		}
	}

	return s.workspaces.UpdateMemberRole(ctx, input.WorkspaceID, input.MemberID, input.Role)
}

func equalNormalizedName(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}
