package application

import (
	"context"
	"testing"

	"note-app/internal/domain"
)

func TestFolderServiceCreateWithValidParent(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"w1": {{ID: "m1", WorkspaceID: "w1", UserID: "u1", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{"parent": {ID: "parent", WorkspaceID: "w1", Name: "Parent"}}, byWorkspace: map[string][]domain.Folder{"w1": {{ID: "parent", WorkspaceID: "w1", Name: "Parent"}}}}
	svc := NewFolderService(folders, memberships)

	parentID := "parent"
	created, err := svc.CreateFolder(context.Background(), "u1", CreateFolderInput{WorkspaceID: "w1", Name: "Child", ParentID: &parentID})
	if err != nil {
		t.Fatalf("expected create folder with parent success, got %v", err)
	}
	if created.ParentID == nil || *created.ParentID != "parent" {
		t.Fatalf("expected parent id to be preserved, got %+v", created.ParentID)
	}
}

func TestWorkspaceServiceListMembersSuccess(t *testing.T) {
	repo := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"w1": {{ID: "m1", WorkspaceID: "w1", UserID: "u1", Role: domain.RoleOwner}, {ID: "m2", WorkspaceID: "w1", UserID: "u2", Role: domain.RoleViewer}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{"w1": 1}}
	svc := NewWorkspaceService(repo, &fakeUserRepo{byEmail: map[string]domain.User{}, byID: map[string]domain.User{}})

	members, err := svc.ListMembers(context.Background(), "u1", "w1")
	if err != nil {
		t.Fatalf("expected list members success, got %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
}
