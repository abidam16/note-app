package application

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"note-app/internal/domain"
)

type fakePageRepo struct {
	pages        map[string]domain.Page
	drafts       map[string]domain.PageDraft
	trash        map[string]domain.TrashItem
	trashedPages map[string]domain.Page
	trashedDraft map[string]domain.PageDraft
}

type fakeThreadAnchorReevaluator struct {
	pageID  string
	content json.RawMessage
	context domain.ThreadAnchorReevaluationContext
	err     error
	called  bool
}

func (r *fakeThreadAnchorReevaluator) ReevaluatePageAnchors(_ context.Context, pageID string, content json.RawMessage, reevaluation domain.ThreadAnchorReevaluationContext) error {
	r.called = true
	r.pageID = pageID
	r.content = append(json.RawMessage(nil), content...)
	r.context = reevaluation
	return r.err
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

func (r *fakePageRepo) GetTrashedByTrashItemID(_ context.Context, trashItemID string) (domain.TrashItem, domain.Page, domain.PageDraft, error) {
	trashItem, ok := r.trash[trashItemID]
	if !ok {
		return domain.TrashItem{}, domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	page, ok := r.trashedPages[trashItem.PageID]
	if !ok {
		return domain.TrashItem{}, domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	draft, ok := r.trashedDraft[trashItem.PageID]
	if !ok {
		return domain.TrashItem{}, domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	return trashItem, page, draft, nil
}

func (r *fakePageRepo) ListByWorkspaceIDAndFolderID(_ context.Context, workspaceID string, folderID *string) ([]domain.PageSummary, error) {
	items := make([]domain.PageSummary, 0)
	for _, page := range r.pages {
		if page.WorkspaceID != workspaceID {
			continue
		}
		if folderID == nil && page.FolderID != nil {
			continue
		}
		if folderID != nil && (page.FolderID == nil || *page.FolderID != *folderID) {
			continue
		}
		items = append(items, domain.PageSummary{
			ID:          page.ID,
			WorkspaceID: page.WorkspaceID,
			FolderID:    page.FolderID,
			Title:       page.Title,
			UpdatedAt:   page.UpdatedAt,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
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
	draft, ok := r.drafts[trashItem.PageID]
	if !ok {
		return domain.ErrNotFound
	}
	if r.trash == nil {
		r.trash = map[string]domain.TrashItem{}
	}
	if r.trashedPages == nil {
		r.trashedPages = map[string]domain.Page{}
	}
	if r.trashedDraft == nil {
		r.trashedDraft = map[string]domain.PageDraft{}
	}
	r.trash[trashItem.ID] = trashItem
	r.trashedPages[trashItem.PageID] = page
	r.trashedDraft[trashItem.PageID] = draft
	delete(r.pages, trashItem.PageID)
	delete(r.drafts, trashItem.PageID)
	_ = page
	return nil
}

func (r *fakePageRepo) ListTrashByWorkspaceID(_ context.Context, _ string) ([]domain.TrashItem, error) {
	return []domain.TrashItem{}, nil
}

func (r *fakePageRepo) GetTrashItemByID(_ context.Context, trashItemID string) (domain.TrashItem, error) {
	item, ok := r.trash[trashItemID]
	if !ok {
		return domain.TrashItem{}, domain.ErrNotFound
	}
	return item, nil
}

func (r *fakePageRepo) RestoreTrashItem(_ context.Context, trashItemID string, _ string, restoredAt time.Time) (domain.Page, error) {
	trashItem, ok := r.trash[trashItemID]
	if !ok {
		return domain.Page{}, domain.ErrNotFound
	}
	page, ok := r.trashedPages[trashItem.PageID]
	if !ok {
		return domain.Page{}, domain.ErrNotFound
	}
	page.UpdatedAt = restoredAt
	if r.pages == nil {
		r.pages = map[string]domain.Page{}
	}
	r.pages[page.ID] = page
	if draft, ok := r.trashedDraft[trashItem.PageID]; ok {
		if r.drafts == nil {
			r.drafts = map[string]domain.PageDraft{}
		}
		r.drafts[draft.PageID] = draft
	}
	delete(r.trash, trashItemID)
	delete(r.trashedPages, trashItem.PageID)
	delete(r.trashedDraft, trashItem.PageID)
	return page, nil
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

func TestPageServiceResourceByIDHidesForeignWorkspaceExistence(t *testing.T) {
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
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Architecture", CreatedBy: "user-1"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage("[]"), LastEditedBy: "user-1"},
		},
	}
	service := NewPageService(pages, memberships, folders)

	if _, _, err := service.GetPage(context.Background(), "outsider-1", "page-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for foreign page get, got %v", err)
	}
	if _, err := service.UpdateDraft(context.Background(), "outsider-1", UpdateDraftInput{PageID: "page-1", Content: json.RawMessage(`[]`)}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for foreign page draft update, got %v", err)
	}
	if err := service.DeletePage(context.Background(), "outsider-1", DeletePageInput{PageID: "page-1"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for foreign page delete, got %v", err)
	}
}

func TestPageServiceListPages(t *testing.T) {
	memberships := &fakeWorkspaceRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			},
		},
		invitations: map[string]domain.WorkspaceInvitation{},
		owners:      map[string]int{},
	}
	folderID := "folder-1"
	otherFolderID := "folder-2"
	now := time.Now().UTC()
	pages := &fakePageRepo{
		pages: map[string]domain.Page{
			"root-new":     {ID: "root-new", WorkspaceID: "workspace-1", Title: "Root New", UpdatedAt: now},
			"root-old":     {ID: "root-old", WorkspaceID: "workspace-1", Title: "Root Old", UpdatedAt: now.Add(-time.Minute)},
			"folder-page":  {ID: "folder-page", WorkspaceID: "workspace-1", FolderID: &folderID, Title: "Folder Page", UpdatedAt: now.Add(-2 * time.Minute)},
			"other-folder": {ID: "other-folder", WorkspaceID: "workspace-1", FolderID: &otherFolderID, Title: "Other Folder Page", UpdatedAt: now.Add(-3 * time.Minute)},
		},
		drafts: map[string]domain.PageDraft{},
	}
	folders := &fakeFolderRepo{
		byID: map[string]domain.Folder{
			"folder-1": {ID: "folder-1", WorkspaceID: "workspace-1", Name: "Docs"},
			"folder-2": {ID: "folder-2", WorkspaceID: "workspace-1", Name: "Notes"},
		},
		byWorkspace: map[string][]domain.Folder{
			"workspace-1": {
				{ID: "folder-1", WorkspaceID: "workspace-1", Name: "Docs"},
				{ID: "folder-2", WorkspaceID: "workspace-1", Name: "Notes"},
			},
		},
	}
	service := NewPageService(pages, memberships, folders)

	rootPages, err := service.ListPages(context.Background(), "viewer-1", "workspace-1", nil)
	if err != nil {
		t.Fatalf("ListPages() root error = %v", err)
	}
	if len(rootPages) != 2 || rootPages[0].ID != "root-new" || rootPages[1].ID != "root-old" {
		t.Fatalf("unexpected root pages: %+v", rootPages)
	}

	folderPages, err := service.ListPages(context.Background(), "viewer-1", "workspace-1", &folderID)
	if err != nil {
		t.Fatalf("ListPages() folder error = %v", err)
	}
	if len(folderPages) != 1 || folderPages[0].ID != "folder-page" {
		t.Fatalf("unexpected folder pages: %+v", folderPages)
	}
}

func TestPageServiceListPagesRejectsInvalidFolderAndNonMember(t *testing.T) {
	crossWorkspaceFolderID := "folder-x"
	missingFolderID := "missing"
	memberships := &fakeWorkspaceRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
			},
		},
		invitations: map[string]domain.WorkspaceInvitation{},
		owners:      map[string]int{},
	}
	pages := &fakePageRepo{pages: map[string]domain.Page{}, drafts: map[string]domain.PageDraft{}}
	folders := &fakeFolderRepo{
		byID: map[string]domain.Folder{
			"folder-x": {ID: "folder-x", WorkspaceID: "workspace-2", Name: "Other"},
		},
		byWorkspace: map[string][]domain.Folder{},
	}
	service := NewPageService(pages, memberships, folders)

	if _, err := service.ListPages(context.Background(), "viewer-1", "workspace-1", &missingFolderID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected missing folder not found, got %v", err)
	}
	if _, err := service.ListPages(context.Background(), "viewer-1", "workspace-1", &crossWorkspaceFolderID); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected cross-workspace folder validation, got %v", err)
	}
	if _, err := service.ListPages(context.Background(), "missing-user", "workspace-1", nil); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected non-member forbidden, got %v", err)
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
	content := json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello","marks":[{"type":"bold"}]}]}]`)

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

func TestPageServiceUpdateDraftTriggersThreadAnchorReevaluation(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"}},
		drafts: map[string]domain.PageDraft{"page-1": {PageID: "page-1", Content: json.RawMessage("[]"), LastEditedBy: "user-1"}},
	}
	reevaluator := &fakeThreadAnchorReevaluator{}
	service := NewPageService(pages, memberships, folders, reevaluator)
	content := json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello"}]}]`)

	if _, err := service.UpdateDraft(context.Background(), "user-1", UpdateDraftInput{PageID: "page-1", Content: content}); err != nil {
		t.Fatalf("UpdateDraft() error = %v", err)
	}
	if !reevaluator.called || reevaluator.pageID != "page-1" || string(reevaluator.content) != string(content) {
		t.Fatalf("expected reevaluator to be called with saved content, got called=%t page=%s content=%s", reevaluator.called, reevaluator.pageID, string(reevaluator.content))
	}
	if reevaluator.context.Reason != domain.PageCommentThreadEventReasonDraftUpdated {
		t.Fatalf("expected draft_updated reason, got %s", reevaluator.context.Reason)
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

	_, err = service.UpdateDraft(context.Background(), "user-1", UpdateDraftInput{PageID: "page-1", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"missing id"}]}]`)})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected block id validation error, got %v", err)
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

func TestPageServiceDeletePageTriggersThreadAnchorReevaluationToMissing(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "editor-1", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"}},
		drafts: map[string]domain.PageDraft{"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello"}]}]`)}},
	}
	reevaluator := &fakeThreadAnchorReevaluator{}
	service := NewPageService(pages, memberships, folders, reevaluator)

	if err := service.DeletePage(context.Background(), "editor-1", DeletePageInput{PageID: "page-1"}); err != nil {
		t.Fatalf("DeletePage() error = %v", err)
	}
	if !reevaluator.called || reevaluator.pageID != "page-1" || string(reevaluator.content) != "[]" {
		t.Fatalf("expected reevaluator to be called with empty content, got called=%t page=%s content=%s", reevaluator.called, reevaluator.pageID, string(reevaluator.content))
	}
	if reevaluator.context.Reason != domain.PageCommentThreadEventReasonPageDeleted {
		t.Fatalf("expected page_deleted reason, got %s", reevaluator.context.Reason)
	}
}

func TestPageServiceDeletePagePropagatesThreadAnchorReevaluationError(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "editor-1", Role: domain.RoleEditor}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"}},
		drafts: map[string]domain.PageDraft{"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)}},
	}
	reevaluator := &fakeThreadAnchorReevaluator{err: errors.New("reevaluate failed")}
	service := NewPageService(pages, memberships, folders, reevaluator)

	if err := service.DeletePage(context.Background(), "editor-1", DeletePageInput{PageID: "page-1"}); err == nil || err.Error() != "reevaluate failed" {
		t.Fatalf("expected reevaluator error propagation, got %v", err)
	}
}

func TestPageServiceRestoreTrashItemTriggersThreadAnchorReevaluation(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "editor-1", Role: domain.RoleEditor},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	content := json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello"}]}]`)
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{},
		drafts: map[string]domain.PageDraft{},
		trash: map[string]domain.TrashItem{
			"trash-1": {ID: "trash-1", WorkspaceID: "workspace-1", PageID: "page-1", PageTitle: "Doc", DeletedBy: "editor-1", DeletedAt: time.Now().UTC()},
		},
		trashedPages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		trashedDraft: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: content, LastEditedBy: "editor-1"},
		},
	}
	reevaluator := &fakeThreadAnchorReevaluator{}
	service := NewPageService(pages, memberships, folders, reevaluator)

	restored, err := service.RestoreTrashItem(context.Background(), "editor-1", RestoreTrashItemInput{TrashItemID: "trash-1"})
	if err != nil {
		t.Fatalf("RestoreTrashItem() error = %v", err)
	}
	if restored.ID != "page-1" {
		t.Fatalf("unexpected restored page: %+v", restored)
	}
	if !reevaluator.called || reevaluator.pageID != "page-1" || string(reevaluator.content) != string(content) {
		t.Fatalf("expected reevaluator to be called with trashed draft content, got called=%t page=%s content=%s", reevaluator.called, reevaluator.pageID, string(reevaluator.content))
	}
	if reevaluator.context.Reason != domain.PageCommentThreadEventReasonPageRestored {
		t.Fatalf("expected page_restored reason, got %s", reevaluator.context.Reason)
	}
}

func TestPageServiceRestoreTrashItemPropagatesThreadAnchorReevaluationError(t *testing.T) {
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "editor-1", Role: domain.RoleEditor},
		},
	}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	content := json.RawMessage(`[{"id":"block-1","type":"paragraph","children":[{"type":"text","text":"hello"}]}]`)
	pages := &fakePageRepo{
		pages:  map[string]domain.Page{},
		drafts: map[string]domain.PageDraft{},
		trash: map[string]domain.TrashItem{
			"trash-1": {ID: "trash-1", WorkspaceID: "workspace-1", PageID: "page-1", PageTitle: "Doc", DeletedBy: "editor-1", DeletedAt: time.Now().UTC()},
		},
		trashedPages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		trashedDraft: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: content, LastEditedBy: "editor-1"},
		},
	}
	reevaluator := &fakeThreadAnchorReevaluator{err: errors.New("reevaluate failed")}
	service := NewPageService(pages, memberships, folders, reevaluator)

	if _, err := service.RestoreTrashItem(context.Background(), "editor-1", RestoreTrashItemInput{TrashItemID: "trash-1"}); err == nil || err.Error() != "reevaluate failed" {
		t.Fatalf("expected reevaluator error propagation, got %v", err)
	}
}
