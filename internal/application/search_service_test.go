package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type fakeSearchRepo struct {
	resultsByWorkspace map[string][]domain.PageSearchResult
	lastWorkspaceID    string
	lastQuery          string
}

func (r *fakeSearchRepo) SearchPages(_ context.Context, workspaceID string, query string) ([]domain.PageSearchResult, error) {
	r.lastWorkspaceID = workspaceID
	r.lastQuery = query
	return r.resultsByWorkspace[workspaceID], nil
}

func TestSearchServiceSearchPages(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer}},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	searches := &fakeSearchRepo{resultsByWorkspace: map[string][]domain.PageSearchResult{
		"workspace-1": {
			{ID: "page-1", WorkspaceID: "workspace-1", Title: "Architecture", UpdatedAt: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)},
		},
	}}
	service := NewSearchService(searches, memberships)

	results, err := service.SearchPages(context.Background(), "user-1", SearchInput{WorkspaceID: "workspace-1", Query: "  architecture  "})
	if err != nil {
		t.Fatalf("SearchPages() error = %v", err)
	}
	if len(results) != 1 || results[0].ID != "page-1" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if searches.lastWorkspaceID != "workspace-1" || searches.lastQuery != "architecture" {
		t.Fatalf("unexpected repo args: workspace=%s query=%s", searches.lastWorkspaceID, searches.lastQuery)
	}
}

func TestSearchServiceRejectsInvalidQueryAndUnauthorizedAccess(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	searches := &fakeSearchRepo{resultsByWorkspace: map[string][]domain.PageSearchResult{}}
	service := NewSearchService(searches, memberships)

	_, err := service.SearchPages(context.Background(), "user-1", SearchInput{WorkspaceID: "workspace-1", Query: "   "})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}

	_, err = service.SearchPages(context.Background(), "user-1", SearchInput{WorkspaceID: "workspace-1", Query: "docs"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}
