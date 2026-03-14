package database

import (
	"strings"
	"testing"
)

func TestFormatFolderSiblingUniquenessConflicts(t *testing.T) {
	parentID := "parent-1"
	formatted := formatFolderSiblingUniquenessConflicts([]FolderSiblingUniquenessConflict{
		{WorkspaceID: "workspace-1", ParentID: nil, NormalizedName: "docs", ConflictCount: 2, FolderIDs: []string{"f1", "f2"}},
		{WorkspaceID: "workspace-1", ParentID: &parentID, NormalizedName: "plans", ConflictCount: 3, FolderIDs: []string{"f3", "f4", "f5"}},
	})

	if !strings.Contains(formatted, `workspace_id=workspace-1 parent_id=<root> normalized_name="docs" count=2 folder_ids=f1,f2`) {
		t.Fatalf("expected root conflict summary, got %q", formatted)
	}
	if !strings.Contains(formatted, `workspace_id=workspace-1 parent_id=parent-1 normalized_name="plans" count=3 folder_ids=f3,f4,f5`) {
		t.Fatalf("expected nested conflict summary, got %q", formatted)
	}
}

func TestFormatFolderSiblingUniquenessConflictsLimitsOutput(t *testing.T) {
	conflicts := make([]FolderSiblingUniquenessConflict, 0, 21)
	for idx := 0; idx < 21; idx++ {
		conflicts = append(conflicts, FolderSiblingUniquenessConflict{
			WorkspaceID:    "workspace",
			NormalizedName: "dup",
			ConflictCount:  2,
			FolderIDs:      []string{"f1", "f2"},
		})
	}

	formatted := formatFolderSiblingUniquenessConflicts(conflicts)
	if !strings.Contains(formatted, "... and 1 more conflict group(s)") {
		t.Fatalf("expected overflow summary, got %q", formatted)
	}
}
