package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"
)

type fakePageRepo struct {
	pages  map[string]domain.Page
	drafts map[string]domain.PageDraft
}

func (r *fakePageRepo) CreateWithDraft(_ context.Context, page domain.Page, draft domain.PageDraft) (domain.Page, domain.PageDraft, error) {
	r.pages[page.ID] = page
	r.drafts[draft.PageID] = draft
	return page, draft, nil
}

func (r *fakePageRepo) GetByID(_ context.Context, pageID string) (domain.Page, domain.PageDraft, error) {
	page, ok := r.pages[pageID]
	if !ok {
		return domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	draft, ok := r.drafts[pageID]
	if !ok {
		return domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	return page, draft, nil
}

func (r *fakePageRepo) UpdateMetadata(_ context.Context, pageID string, title string, folderID *string, updatedAt time.Time) (domain.Page, error) {
	page, ok := r.pages[pageID]
	if !ok {
		return domain.Page{}, domain.ErrNotFound
	}
	page.Title = title
	page.FolderID = folderID
	page.UpdatedAt = updatedAt
	r.pages[pageID] = page
	return page, nil
}

func (r *fakePageRepo) UpdateDraft(_ context.Context, pageID string, content json.RawMessage, lastEditedBy string, updatedAt time.Time) (domain.PageDraft, error) {
	draft, ok := r.drafts[pageID]
	if !ok {
		return domain.PageDraft{}, domain.ErrNotFound
	}
	draft.Content = content
	draft.LastEditedBy = lastEditedBy
	draft.UpdatedAt = updatedAt
	r.drafts[pageID] = draft
	page := r.pages[pageID]
	page.UpdatedAt = updatedAt
	r.pages[pageID] = page
	return draft, nil
}

func (r *fakePageRepo) SoftDelete(_ context.Context, trashItem domain.TrashItem) error {
	page, ok := r.pages[trashItem.PageID]
	if !ok {
		return domain.ErrNotFound
	}
	delete(r.pages, trashItem.PageID)
	delete(r.drafts, trashItem.PageID)
	_ = page
	return nil
}

func (r *fakePageRepo) ListTrashByWorkspaceID(_ context.Context, _ string) ([]domain.TrashItem, error) {
	return []domain.TrashItem{}, nil
}

func (r *fakePageRepo) GetTrashItemByID(_ context.Context, _ string) (domain.TrashItem, error) {
	return domain.TrashItem{}, domain.ErrNotFound
}

func (r *fakePageRepo) RestoreTrashItem(_ context.Context, _ string, _ string, _ time.Time) (domain.Page, error) {
	return domain.Page{}, domain.ErrNotFound
}

func TestPageServiceCreatePage(t *testing.T) {
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
	pages := &fakePageRepo{pages: map[string]domain.Page{}, drafts: map[string]domain.PageDraft{}}
	service := NewPageService(pages, memberships, folders)

	page, draft, err := service.CreatePage(context.Background(), "user-1", CreatePageInput{
		WorkspaceID: "workspace-1",
		Title:       "Architecture",
	})
	if err != nil {
		t.Fatalf("CreatePage() error = %v", err)
	}

	if page.Title != "Architecture" {
		t.Fatalf("expected title Architecture, got %s", page.Title)
	}
	if string(draft.Content) != "[]" {
		t.Fatalf("expected empty draft content, got %s", string(draft.Content))
	}
	if draft.PageID != page.ID {
		t.Fatalf("expected draft page id %s, got %s", page.ID, draft.PageID)
	}
}

func TestPageServiceRejectsViewer(t *testing.T) {
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
	pages := &fakePageRepo{pages: map[string]domain.Page{}, drafts: map[string]domain.PageDraft{}}
	service := NewPageService(pages, memberships, folders)

	_, _, err := service.CreatePage(context.Background(), "user-1", CreatePageInput{
		WorkspaceID: "workspace-1",
		Title:       "Architecture",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestPageServiceRejectsFolderFromAnotherWorkspace(t *testing.T) {
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
	pages := &fakePageRepo{pages: map[string]domain.Page{}, drafts: map[string]domain.PageDraft{}}
	service := NewPageService(pages, memberships, folders)
	folderID := "folder-1"

	_, _, err := service.CreatePage(context.Background(), "user-1", CreatePageInput{
		WorkspaceID: "workspace-1",
		FolderID:    &folderID,
		Title:       "Architecture",
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestPageServiceGetPage(t *testing.T) {
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
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Architecture", CreatedBy: "user-1"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage("[]"), LastEditedBy: "user-1"},
		},
	}
	service := NewPageService(pages, memberships, folders)

	page, draft, err := service.GetPage(context.Background(), "user-1", "page-1")
	if err != nil {
		t.Fatalf("GetPage() error = %v", err)
	}
	if page.ID != "page-1" || draft.PageID != "page-1" {
		t.Fatalf("unexpected page payload: %+v %+v", page, draft)
	}
}

func TestPageServiceRename(t *testing.T) {
	memberships := &fakeWorkspaceRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor}},
		},
		invitations: map[string]domain.WorkspaceInvitation{},
		owners:      map[string]int{},
	}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Old Title"}},
		drafts: map[string]domain.PageDraft{"page-1": {PageID: "page-1", Content: json.RawMessage("[]")}},
	}
	service := NewPageService(pages, memberships, folders)
	title := "New Title"

	updated, err := service.UpdatePage(context.Background(), "user-1", UpdatePageInput{PageID: "page-1", Title: &title})
	if err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}
	if updated.Title != "New Title" {
		t.Fatalf("expected title New Title, got %s", updated.Title)
	}
}

func TestPageServiceMoveToFolderAndRoot(t *testing.T) {
	memberships := &fakeWorkspaceRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor}},
		},
		invitations: map[string]domain.WorkspaceInvitation{},
		owners:      map[string]int{},
	}
	folders := &fakeFolderRepo{
		byID: map[string]domain.Folder{
			"folder-1": {ID: "folder-1", WorkspaceID: "workspace-1", Name: "Engineering"},
		},
		byWorkspace: map[string][]domain.Folder{"workspace-1": {{ID: "folder-1", WorkspaceID: "workspace-1", Name: "Engineering"}}},
	}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"}},
		drafts: map[string]domain.PageDraft{"page-1": {PageID: "page-1", Content: json.RawMessage("[]")}},
	}
	service := NewPageService(pages, memberships, folders)
	folderID := "folder-1"

	updated, err := service.UpdatePage(context.Background(), "user-1", UpdatePageInput{PageID: "page-1", FolderID: &folderID, FolderSet: true})
	if err != nil {
		t.Fatalf("UpdatePage() move error = %v", err)
	}
	if updated.FolderID == nil || *updated.FolderID != "folder-1" {
		t.Fatalf("expected folder-1, got %+v", updated.FolderID)
	}

	updated, err = service.UpdatePage(context.Background(), "user-1", UpdatePageInput{PageID: "page-1", FolderID: nil, FolderSet: true})
	if err != nil {
		t.Fatalf("UpdatePage() move to root error = %v", err)
	}
	if updated.FolderID != nil {
		t.Fatalf("expected nil folder_id, got %+v", updated.FolderID)
	}
}

func TestPageServiceUpdateRejectsInvalidFolderAndViewer(t *testing.T) {
	folderID := "folder-x"
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{"folder-x": {ID: "folder-x", WorkspaceID: "workspace-2", Name: "Other"}}, byWorkspace: map[string][]domain.Folder{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"}},
		drafts: map[string]domain.PageDraft{"page-1": {PageID: "page-1", Content: json.RawMessage("[]")}},
	}

	viewerMemberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	service := NewPageService(pages, viewerMemberships, folders)
	_, err := service.UpdatePage(context.Background(), "user-1", UpdatePageInput{PageID: "page-1", Title: &folderID})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}

	editorMemberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	service = NewPageService(pages, editorMemberships, folders)
	_, err = service.UpdatePage(context.Background(), "user-1", UpdatePageInput{PageID: "page-1", FolderID: &folderID, FolderSet: true})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestPageServiceUpdateDraft(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"}},
		drafts: map[string]domain.PageDraft{"page-1": {PageID: "page-1", Content: json.RawMessage("[]"), LastEditedBy: "user-1"}},
	}
	service := NewPageService(pages, memberships, folders)
	content := json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"hello","marks":[{"type":"bold"}]}]}]`)

	draft, err := service.UpdateDraft(context.Background(), "user-1", UpdateDraftInput{PageID: "page-1", Content: content})
	if err != nil {
		t.Fatalf("UpdateDraft() error = %v", err)
	}
	if string(draft.Content) != string(content) {
		t.Fatalf("expected updated content %s, got %s", string(content), string(draft.Content))
	}

	_, fetchedDraft, err := service.GetPage(context.Background(), "user-1", "page-1")
	if err != nil {
		t.Fatalf("GetPage() after draft update error = %v", err)
	}
	if string(fetchedDraft.Content) != string(content) {
		t.Fatalf("expected persisted content %s, got %s", string(content), string(fetchedDraft.Content))
	}
}

func TestPageServiceUpdateDraftRejectsInvalidDocument(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"}},
		drafts: map[string]domain.PageDraft{"page-1": {PageID: "page-1", Content: json.RawMessage("[]")}},
	}
	service := NewPageService(pages, memberships, folders)

	_, err := service.UpdateDraft(context.Background(), "user-1", UpdateDraftInput{PageID: "page-1", Content: json.RawMessage(`[{"type":"unsupported"}]`)})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestPageServiceUpdateDraftRejectsViewerAndMissingPage(t *testing.T) {
	viewerMemberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"}},
		drafts: map[string]domain.PageDraft{"page-1": {PageID: "page-1", Content: json.RawMessage("[]")}},
	}
	service := NewPageService(pages, viewerMemberships, folders)
	_, err := service.UpdateDraft(context.Background(), "user-1", UpdateDraftInput{PageID: "page-1", Content: json.RawMessage(`[]`)})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}

	editorMemberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	service = NewPageService(pages, editorMemberships, folders)
	_, err = service.UpdateDraft(context.Background(), "user-2", UpdateDraftInput{PageID: "missing-page", Content: json.RawMessage(`[]`)})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}
func TestPageServiceDeleteAndListTrashPermissions(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "editor-1", Role: domain.RoleEditor}, {ID: "member-2", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"}},
		drafts: map[string]domain.PageDraft{"page-1": {PageID: "page-1", Content: json.RawMessage("[]")}},
	}
	service := NewPageService(pages, memberships, folders)

	if err := service.DeletePage(context.Background(), "viewer-1", DeletePageInput{PageID: "page-1"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}
	if err := service.DeletePage(context.Background(), "editor-1", DeletePageInput{PageID: "page-1"}); err != nil {
		t.Fatalf("DeletePage() error = %v", err)
	}
}
