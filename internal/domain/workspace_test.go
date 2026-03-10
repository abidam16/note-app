package domain

import "testing"

func TestIsValidWorkspaceRole(t *testing.T) {
	if !IsValidWorkspaceRole(RoleOwner) || !IsValidWorkspaceRole(RoleEditor) || !IsValidWorkspaceRole(RoleViewer) {
		t.Fatal("expected builtin roles to be valid")
	}
	if IsValidWorkspaceRole(WorkspaceRole("admin")) {
		t.Fatal("unexpected role accepted")
	}
}
