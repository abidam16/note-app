package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type fakeRevisionRepo struct {
	revisions map[string]domain.Revision
	ordered   []domain.Revision
}

func (r *fakeRevisionRepo) Create(_ context.Context, revision domain.Revision) (domain.Revision, error) {
	r.revisions[revision.ID] = revision
	r.ordered = append(r.ordered, revision)
	return revision, nil
}

func (r *fakeRevisionRepo) GetByID(_ context.Context, revisionID string) (domain.Revision, error) {
	revision, ok := r.revisions[revisionID]
	if !ok {
		return domain.Revision{}, domain.ErrNotFound
	}
	return revision, nil
}

func (r *fakeRevisionRepo) ListByPageID(_ context.Context, pageID string) ([]domain.Revision, error) {
	result := make([]domain.Revision, 0)
	for _, revision := range r.ordered {
		if revision.PageID == pageID {
			revision.Content = nil
			result = append(result, revision)
		}
	}
	return result, nil
}

func TestRevisionServiceCreateRevision(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"Saved draft"}]}]`), LastEditedBy: "user-1"},
		},
	}
	revisions := &fakeRevisionRepo{revisions: map[string]domain.Revision{}, ordered: []domain.Revision{}}
	service := NewRevisionService(revisions, pages, memberships)
	label := "Milestone 1"
	note := "Before rewrite"

	revision, err := service.CreateRevision(context.Background(), "user-1", CreateRevisionInput{PageID: "page-1", Label: &label, Note: &note})
	if err != nil {
		t.Fatalf("CreateRevision() error = %v", err)
	}
	if revision.PageID != "page-1" {
		t.Fatalf("expected page-1, got %s", revision.PageID)
	}
	if revision.Label == nil || *revision.Label != "Milestone 1" {
		t.Fatalf("unexpected label: %+v", revision.Label)
	}
	if revision.Note == nil || *revision.Note != "Before rewrite" {
		t.Fatalf("unexpected note: %+v", revision.Note)
	}
	if string(revision.Content) != string(pages.drafts["page-1"].Content) {
		t.Fatalf("expected revision content to match draft")
	}
	if string(pages.drafts["page-1"].Content) != `[{"type":"paragraph","children":[{"type":"text","text":"Saved draft"}]}]` {
		t.Fatalf("expected draft to remain unchanged, got %s", string(pages.drafts["page-1"].Content))
	}
	if len(revisions.revisions) != 1 {
		t.Fatalf("expected one revision, got %d", len(revisions.revisions))
	}
}

func TestRevisionServiceListRevisions(t *testing.T) {
	viewerMemberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	revisions := &fakeRevisionRepo{revisions: map[string]domain.Revision{}, ordered: []domain.Revision{
		{ID: "rev-1", PageID: "page-1", Label: stringPtr("First"), CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph"}]`)},
		{ID: "rev-2", PageID: "page-1", Label: stringPtr("Second"), CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph"}]`)},
		{ID: "rev-x", PageID: "page-2", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph"}]`)},
	}}
	service := NewRevisionService(revisions, pages, viewerMemberships)

	listed, err := service.ListRevisions(context.Background(), "user-1", "page-1")
	if err != nil {
		t.Fatalf("ListRevisions() error = %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected two revisions, got %d", len(listed))
	}
	if listed[0].ID != "rev-1" || listed[1].ID != "rev-2" {
		t.Fatalf("unexpected order: %+v", listed)
	}
	if listed[0].Content != nil || listed[1].Content != nil {
		t.Fatalf("expected list payload to exclude content")
	}
}

func TestRevisionServiceCompareRevisions(t *testing.T) {
	viewerMemberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	revisions := &fakeRevisionRepo{revisions: map[string]domain.Revision{
		"rev-1": {ID: "rev-1", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"hello world"}]},{"type":"quote","text":"unchanged"}]`)},
		"rev-2": {ID: "rev-2", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"hello brave world"}]},{"type":"quote","text":"unchanged"},{"type":"image","src":"/uploads/a.png"}]`)},
	}, ordered: []domain.Revision{}}
	service := NewRevisionService(revisions, pages, viewerMemberships)

	diff, err := service.CompareRevisions(context.Background(), "user-1", CompareRevisionsInput{PageID: "page-1", FromRevisionID: "rev-1", ToRevisionID: "rev-2"})
	if err != nil {
		t.Fatalf("CompareRevisions() error = %v", err)
	}
	if len(diff.Blocks) != 3 {
		t.Fatalf("expected three diff blocks, got %d", len(diff.Blocks))
	}
	if diff.Blocks[0].Status != "modified" {
		t.Fatalf("expected first block modified, got %s", diff.Blocks[0].Status)
	}
	if len(diff.Blocks[0].InlineDiff) == 0 {
		t.Fatalf("expected inline diff for modified block")
	}
	if len(diff.Blocks[0].Lines) != 2 {
		t.Fatalf("expected line-aware diff for modified block, got %+v", diff.Blocks[0].Lines)
	}
	if diff.Blocks[1].Status != "unchanged" {
		t.Fatalf("expected second block unchanged, got %s", diff.Blocks[1].Status)
	}
	if len(diff.Blocks[1].Lines) != 1 || diff.Blocks[1].Lines[0].Operation != "context" {
		t.Fatalf("expected unchanged block to expose context line, got %+v", diff.Blocks[1].Lines)
	}
	if diff.Blocks[2].Status != "added" {
		t.Fatalf("expected third block added, got %s", diff.Blocks[2].Status)
	}
	if len(diff.Blocks[2].Lines) != 1 || diff.Blocks[2].Lines[0].Operation != "added" {
		t.Fatalf("expected added block to expose added line, got %+v", diff.Blocks[2].Lines)
	}
}

func TestRevisionServiceRestoreRevision(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"current"}]}]`), LastEditedBy: "user-1"},
		},
	}
	revisions := &fakeRevisionRepo{revisions: map[string]domain.Revision{
		"rev-1": {ID: "rev-1", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"old value"}]}]`)},
		"rev-2": {ID: "rev-2", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"current"}]}]`)},
	}, ordered: []domain.Revision{
		{ID: "rev-1", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)},
		{ID: "rev-2", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC)},
	}}
	service := NewRevisionService(revisions, pages, memberships)

	result, err := service.RestoreRevision(context.Background(), "user-1", RestoreRevisionInput{PageID: "page-1", RevisionID: "rev-1"})
	if err != nil {
		t.Fatalf("RestoreRevision() error = %v", err)
	}
	if string(result.Draft.Content) != string(revisions.revisions["rev-1"].Content) {
		t.Fatalf("expected restored draft content %s, got %s", string(revisions.revisions["rev-1"].Content), string(result.Draft.Content))
	}
	if string(pages.drafts["page-1"].Content) != string(revisions.revisions["rev-1"].Content) {
		t.Fatalf("expected page draft to be updated")
	}
	if len(revisions.ordered) != 3 {
		t.Fatalf("expected history to gain a new revision, got %d entries", len(revisions.ordered))
	}
	if string(result.Revision.Content) != string(revisions.revisions["rev-1"].Content) {
		t.Fatalf("expected restore revision content to match restored revision")
	}
}

func TestRevisionServiceRejectsViewerAndMissingPage(t *testing.T) {
	viewerMemberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"Saved draft"}]}]`)},
		},
	}
	revisions := &fakeRevisionRepo{revisions: map[string]domain.Revision{}, ordered: []domain.Revision{}}
	service := NewRevisionService(revisions, pages, viewerMemberships)
	_, err := service.CreateRevision(context.Background(), "user-1", CreateRevisionInput{PageID: "page-1"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}
	_, err = service.RestoreRevision(context.Background(), "user-1", RestoreRevisionInput{PageID: "page-1", RevisionID: "rev-1"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}

	editorMemberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	service = NewRevisionService(revisions, pages, editorMemberships)
	_, err = service.CreateRevision(context.Background(), "user-2", CreateRevisionInput{PageID: "missing-page"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}

	_, err = service.ListRevisions(context.Background(), "user-2", "missing-page")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestRevisionServiceHidesForeignWorkspaceResources(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "editor-1", Role: domain.RoleEditor},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"Saved draft"}]}]`)},
		},
	}
	revisions := &fakeRevisionRepo{revisions: map[string]domain.Revision{
		"rev-1": {ID: "rev-1", PageID: "page-1", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"Saved draft"}]}]`)},
	}, ordered: []domain.Revision{
		{ID: "rev-1", PageID: "page-1", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"Saved draft"}]}]`)},
	}}
	service := NewRevisionService(revisions, pages, memberships)

	if _, err := service.CreateRevision(context.Background(), "outsider-1", CreateRevisionInput{PageID: "page-1"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for foreign page revision create, got %v", err)
	}
	if _, err := service.ListRevisions(context.Background(), "outsider-1", "page-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for foreign page revision list, got %v", err)
	}
	if _, err := service.CompareRevisions(context.Background(), "outsider-1", CompareRevisionsInput{PageID: "page-1", FromRevisionID: "rev-1", ToRevisionID: "rev-1"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for foreign page revision compare, got %v", err)
	}
	if _, err := service.RestoreRevision(context.Background(), "outsider-1", RestoreRevisionInput{PageID: "page-1", RevisionID: "rev-1"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for foreign page revision restore, got %v", err)
	}
}

func TestRevisionServiceRejectsInvalidDraftAndInvalidComparison(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"type":"unsupported"}]`)},
		},
	}
	revisions := &fakeRevisionRepo{revisions: map[string]domain.Revision{
		"rev-1": {ID: "rev-1", PageID: "page-1", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"a"}]}]`)},
		"rev-x": {ID: "rev-x", PageID: "page-x", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"b"}]}]`)},
	}, ordered: []domain.Revision{}}
	service := NewRevisionService(revisions, pages, memberships)

	_, err := service.CreateRevision(context.Background(), "user-1", CreateRevisionInput{PageID: "page-1"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}

	_, err = service.CompareRevisions(context.Background(), "user-1", CompareRevisionsInput{PageID: "page-1", FromRevisionID: "rev-1", ToRevisionID: "rev-x"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}

	_, err = service.CompareRevisions(context.Background(), "user-1", CompareRevisionsInput{PageID: "page-1", FromRevisionID: "rev-1", ToRevisionID: "missing"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}

	_, err = service.RestoreRevision(context.Background(), "user-1", RestoreRevisionInput{PageID: "page-1", RevisionID: "rev-x"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func stringPtr(value string) *string {
	return &value
}
