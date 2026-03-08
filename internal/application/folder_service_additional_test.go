package application

import (
	"context"
	"errors"
	"testing"

	"note-app/internal/domain"
)

func TestFolderServiceAdditionalBranches(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"w1": {{ID: "m1", WorkspaceID: "w1", UserID: "u1", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{"f1": {ID: "f1", WorkspaceID: "w1", Name: "Root"}}, byWorkspace: map[string][]domain.Folder{"w1": {{ID: "f1", WorkspaceID: "w1", Name: "Root"}}}}
	svc := NewFolderService(folders, memberships)

	if _, err := svc.CreateFolder(context.Background(), "u1", CreateFolderInput{WorkspaceID: "w1", Name: "   "}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected folder name validation, got %v", err)
	}

	emptyParent := "   "
	created, err := svc.CreateFolder(context.Background(), "u1", CreateFolderInput{WorkspaceID: "w1", Name: "Child", ParentID: &emptyParent})
	if err != nil {
		t.Fatalf("expected folder creation with blank parent to succeed, got %v", err)
	}
	if created.ParentID != nil {
		t.Fatalf("expected nil parent for blank parent id, got %+v", created.ParentID)
	}

	list, err := svc.ListFolders(context.Background(), "u1", "w1")
	if err != nil {
		t.Fatalf("expected list folders success, got %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least one folder")
	}
}
