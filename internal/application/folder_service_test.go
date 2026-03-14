package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"note-app/internal/domain"
)

type fakeFolderRepo struct {
	byID        map[string]domain.Folder
	byWorkspace map[string][]domain.Folder
}

func (r *fakeFolderRepo) Create(_ context.Context, folder domain.Folder) (domain.Folder, error) {
	for _, existing := range r.byWorkspace[folder.WorkspaceID] {
		if sameFolderLocation(existing.ParentID, folder.ParentID) && strings.EqualFold(strings.TrimSpace(existing.Name), strings.TrimSpace(folder.Name)) {
			return domain.Folder{}, domain.ErrValidation
		}
	}
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

func (r *fakeFolderRepo) HasSiblingWithName(_ context.Context, workspaceID string, parentID *string, name string, excludeFolderID *string) (bool, error) {
	for _, folder := range r.byWorkspace[workspaceID] {
		if excludeFolderID != nil && folder.ID == *excludeFolderID {
			continue
		}
		if sameFolderLocation(folder.ParentID, parentID) && strings.EqualFold(strings.TrimSpace(folder.Name), strings.TrimSpace(name)) {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeFolderRepo) UpdateName(_ context.Context, folderID, name string, updatedAt time.Time) (domain.Folder, error) {
	folder, ok := r.byID[folderID]
	if !ok {
		return domain.Folder{}, domain.ErrNotFound
	}
	for idx, existing := range r.byWorkspace[folder.WorkspaceID] {
		if existing.ID == folderID {
			folder.Name = name
			folder.UpdatedAt = updatedAt
			r.byWorkspace[folder.WorkspaceID][idx] = folder
			r.byID[folderID] = folder
			return folder, nil
		}
	}
	return domain.Folder{}, domain.ErrNotFound
}

func sameFolderLocation(left, right *string) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return *left == *right
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

func TestFolderServiceRenameFolder(t *testing.T) {
	parentID := "parent-1"
	memberships := &fakeWorkspaceRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
			},
		},
		invitations: map[string]domain.WorkspaceInvitation{},
		owners:      map[string]int{},
	}
	folders := &fakeFolderRepo{
		byID: map[string]domain.Folder{
			"folder-1": {ID: "folder-1", WorkspaceID: "workspace-1", ParentID: &parentID, Name: "Old"},
			"folder-2": {ID: "folder-2", WorkspaceID: "workspace-1", ParentID: &parentID, Name: "Taken"},
		},
		byWorkspace: map[string][]domain.Folder{
			"workspace-1": {
				{ID: "folder-1", WorkspaceID: "workspace-1", ParentID: &parentID, Name: "Old"},
				{ID: "folder-2", WorkspaceID: "workspace-1", ParentID: &parentID, Name: "Taken"},
			},
		},
	}
	service := NewFolderService(folders, memberships)

	renamed, err := service.RenameFolder(context.Background(), "user-1", RenameFolderInput{FolderID: "folder-1", Name: "Renamed"})
	if err != nil {
		t.Fatalf("RenameFolder() error = %v", err)
	}
	if renamed.Name != "Renamed" {
		t.Fatalf("expected renamed folder, got %s", renamed.Name)
	}

	if _, err := service.RenameFolder(context.Background(), "user-1", RenameFolderInput{FolderID: "folder-1", Name: "  taken  "}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected duplicate sibling validation, got %v", err)
	}

	if _, err := service.RenameFolder(context.Background(), "user-2", RenameFolderInput{FolderID: "folder-1", Name: "Viewer Attempt"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected viewer forbidden, got %v", err)
	}
}
