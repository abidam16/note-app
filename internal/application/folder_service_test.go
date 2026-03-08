package application

import (
	"context"
	"errors"
	"testing"

	"note-app/internal/domain"
)

type fakeFolderRepo struct {
	byID        map[string]domain.Folder
	byWorkspace map[string][]domain.Folder
}

func (r *fakeFolderRepo) Create(_ context.Context, folder domain.Folder) (domain.Folder, error) {
	r.byID[folder.ID] = folder
	r.byWorkspace[folder.WorkspaceID] = append(r.byWorkspace[folder.WorkspaceID], folder)
	return folder, nil
}

func (r *fakeFolderRepo) GetByID(_ context.Context, folderID string) (domain.Folder, error) {
	folder, ok := r.byID[folderID]
	if !ok {
		return domain.Folder{}, domain.ErrNotFound
	}
	return folder, nil
}

func (r *fakeFolderRepo) ListByWorkspaceID(_ context.Context, workspaceID string) ([]domain.Folder, error) {
	return r.byWorkspace[workspaceID], nil
}

func TestFolderServiceCreateFolder(t *testing.T) {
	memberships := &fakeWorkspaceRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
			},
		},
		invitations: map[string]domain.WorkspaceInvitation{},
		owners:      map[string]int{},
	}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	service := NewFolderService(folders, memberships)

	created, err := service.CreateFolder(context.Background(), "user-1", CreateFolderInput{
		WorkspaceID: "workspace-1",
		Name:        "Engineering",
	})
	if err != nil {
		t.Fatalf("CreateFolder() error = %v", err)
	}

	if created.Name != "Engineering" {
		t.Fatalf("expected folder name Engineering, got %s", created.Name)
	}
	if created.WorkspaceID != "workspace-1" {
		t.Fatalf("expected workspace workspace-1, got %s", created.WorkspaceID)
	}
}

func TestFolderServiceRejectsViewer(t *testing.T) {
	memberships := &fakeWorkspaceRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
			},
		},
		invitations: map[string]domain.WorkspaceInvitation{},
		owners:      map[string]int{},
	}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	service := NewFolderService(folders, memberships)

	_, err := service.CreateFolder(context.Background(), "user-1", CreateFolderInput{
		WorkspaceID: "workspace-1",
		Name:        "Engineering",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestFolderServiceRejectsParentFromAnotherWorkspace(t *testing.T) {
	memberships := &fakeWorkspaceRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
			},
		},
		invitations: map[string]domain.WorkspaceInvitation{},
		owners:      map[string]int{},
	}
	folders := &fakeFolderRepo{
		byID: map[string]domain.Folder{
			"folder-1": {ID: "folder-1", WorkspaceID: "workspace-2", Name: "Other"},
		},
		byWorkspace: map[string][]domain.Folder{},
	}
	service := NewFolderService(folders, memberships)
	parentID := "folder-1"

	_, err := service.CreateFolder(context.Background(), "user-1", CreateFolderInput{
		WorkspaceID: "workspace-1",
		Name:        "Engineering",
		ParentID:    &parentID,
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
