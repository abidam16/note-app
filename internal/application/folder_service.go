package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"note-app/internal/domain"

	"github.com/google/uuid"
)

type FolderRepository interface {
	Create(ctx context.Context, folder domain.Folder) (domain.Folder, error)
	GetByID(ctx context.Context, folderID string) (domain.Folder, error)
	ListByWorkspaceID(ctx context.Context, workspaceID string) ([]domain.Folder, error)
	HasSiblingWithName(ctx context.Context, workspaceID string, parentID *string, name string, excludeFolderID *string) (bool, error)
	UpdateName(ctx context.Context, folderID, name string, updatedAt time.Time) (domain.Folder, error)
}

type WorkspaceMembershipReader interface {
	GetMembershipByUserID(ctx context.Context, workspaceID, userID string) (domain.WorkspaceMember, error)
	ListMembers(ctx context.Context, workspaceID string) ([]domain.WorkspaceMember, error)
}

type CreateFolderInput struct {
	WorkspaceID string
	Name        string
	ParentID    *string
}

type RenameFolderInput struct {
	FolderID string
	Name     string
}

type FolderService struct {
	folders     FolderRepository
	memberships WorkspaceMembershipReader
}

func NewFolderService(folders FolderRepository, memberships WorkspaceMembershipReader) FolderService {
	return FolderService{
		folders:     folders,
		memberships: memberships,
	}
}

func (s FolderService) CreateFolder(ctx context.Context, actorID string, input CreateFolderInput) (domain.Folder, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.Folder{}, fmt.Errorf("%w: folder name is required", domain.ErrValidation)
	}

	membership, err := s.memberships.GetMembershipByUserID(ctx, input.WorkspaceID, actorID)
	if err != nil {
		return domain.Folder{}, err
	}
	if membership.Role == domain.RoleViewer {
		return domain.Folder{}, domain.ErrForbidden
	}

	var parentID *string
	if input.ParentID != nil {
		trimmedParentID := strings.TrimSpace(*input.ParentID)
		if trimmedParentID != "" {
			parent, err := s.folders.GetByID(ctx, trimmedParentID)
			if err != nil {
				return domain.Folder{}, err
			}
			if parent.WorkspaceID != input.WorkspaceID {
				return domain.Folder{}, fmt.Errorf("%w: parent folder must belong to the same workspace", domain.ErrValidation)
			}
			parentID = &trimmedParentID
		}
	}

	exists, err := s.folders.HasSiblingWithName(ctx, input.WorkspaceID, parentID, name, nil)
	if err != nil {
		return domain.Folder{}, err
	}
	if exists {
		return domain.Folder{}, fmt.Errorf("%w: folder name already exists in this location", domain.ErrValidation)
	}

	now := time.Now().UTC()
	folder := domain.Folder{
		ID:          uuid.NewString(),
		WorkspaceID: input.WorkspaceID,
		ParentID:    parentID,
		Name:        name,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return s.folders.Create(ctx, folder)
}

func (s FolderService) ListFolders(ctx context.Context, actorID, workspaceID string) ([]domain.Folder, error) {
	if _, err := s.memberships.GetMembershipByUserID(ctx, workspaceID, actorID); err != nil {
		return nil, err
	}

	return s.folders.ListByWorkspaceID(ctx, workspaceID)
}

func (s FolderService) RenameFolder(ctx context.Context, actorID string, input RenameFolderInput) (domain.Folder, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.Folder{}, fmt.Errorf("%w: folder name is required", domain.ErrValidation)
	}

	folder, err := s.folders.GetByID(ctx, input.FolderID)
	if err != nil {
		return domain.Folder{}, err
	}

	membership, err := s.memberships.GetMembershipByUserID(ctx, folder.WorkspaceID, actorID)
	if err != nil {
		return domain.Folder{}, err
	}
	if membership.Role == domain.RoleViewer {
		return domain.Folder{}, domain.ErrForbidden
	}

	exists, err := s.folders.HasSiblingWithName(ctx, folder.WorkspaceID, folder.ParentID, name, &folder.ID)
	if err != nil {
		return domain.Folder{}, err
	}
	if exists {
		return domain.Folder{}, fmt.Errorf("%w: folder name already exists in this location", domain.ErrValidation)
	}

	return s.folders.UpdateName(ctx, folder.ID, name, time.Now().UTC())
}
