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

type pageRepoExtra struct {
	pages        map[string]domain.Page
	drafts       map[string]domain.PageDraft
	trash        map[string]domain.TrashItem
	trashedPages map[string]domain.Page
	trashedDraft map[string]domain.PageDraft
	listTrashErr error
}

func (r *pageRepoExtra) CreateWithDraft(_ context.Context, page domain.Page, draft domain.PageDraft) (domain.Page, domain.PageDraft, error) {
	if r.pages == nil {
		r.pages = map[string]domain.Page{}
	}
	if r.drafts == nil {
		r.drafts = map[string]domain.PageDraft{}
	}
	r.pages[page.ID] = page
	r.drafts[draft.PageID] = draft
	return page, draft, nil
}
func (r *pageRepoExtra) GetByID(_ context.Context, pageID string) (domain.Page, domain.PageDraft, error) {
	p, ok := r.pages[pageID]
	if !ok {
		return domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	d, ok := r.drafts[pageID]
	if !ok {
		return domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	return p, d, nil
}
func (r *pageRepoExtra) GetTrashedByTrashItemID(_ context.Context, trashItemID string) (domain.TrashItem, domain.Page, domain.PageDraft, error) {
	item, ok := r.trash[trashItemID]
	if !ok {
		return domain.TrashItem{}, domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	page, ok := r.trashedPages[item.PageID]
	if !ok {
		return domain.TrashItem{}, domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	draft, ok := r.trashedDraft[item.PageID]
	if !ok {
		return domain.TrashItem{}, domain.Page{}, domain.PageDraft{}, domain.ErrNotFound
	}
	return item, page, draft, nil
}
func (r *pageRepoExtra) ListByWorkspaceIDAndFolderID(_ context.Context, workspaceID string, folderID *string) ([]domain.PageSummary, error) {
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
func (r *pageRepoExtra) UpdateMetadata(_ context.Context, pageID string, title string, folderID *string, updatedAt time.Time) (domain.Page, error) {
	p, ok := r.pages[pageID]
	if !ok {
		return domain.Page{}, domain.ErrNotFound
	}
	p.Title = title
	p.FolderID = folderID
	p.UpdatedAt = updatedAt
	r.pages[pageID] = p
	return p, nil
}
func (r *pageRepoExtra) UpdateDraft(_ context.Context, pageID string, content json.RawMessage, lastEditedBy string, updatedAt time.Time) (domain.PageDraft, error) {
	d, ok := r.drafts[pageID]
	if !ok {
		return domain.PageDraft{}, domain.ErrNotFound
	}
	d.Content = content
	d.LastEditedBy = lastEditedBy
	d.UpdatedAt = updatedAt
	r.drafts[pageID] = d
	return d, nil
}
func (r *pageRepoExtra) SoftDelete(_ context.Context, trashItem domain.TrashItem) error {
	page, ok := r.pages[trashItem.PageID]
	if !ok {
		return domain.ErrNotFound
	}
	draft, ok := r.drafts[trashItem.PageID]
	if !ok {
		return domain.ErrNotFound
	}
	delete(r.pages, trashItem.PageID)
	delete(r.drafts, trashItem.PageID)
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
	return nil
}
func (r *pageRepoExtra) ListTrashByWorkspaceID(_ context.Context, workspaceID string) ([]domain.TrashItem, error) {
	if r.listTrashErr != nil {
		return nil, r.listTrashErr
	}
	items := make([]domain.TrashItem, 0)
	for _, item := range r.trash {
		if item.WorkspaceID == workspaceID {
			items = append(items, item)
		}
	}
	return items, nil
}
func (r *pageRepoExtra) GetTrashItemByID(_ context.Context, trashItemID string) (domain.TrashItem, error) {
	item, ok := r.trash[trashItemID]
	if !ok {
		return domain.TrashItem{}, domain.ErrNotFound
	}
	return item, nil
}
func (r *pageRepoExtra) RestoreTrashItem(_ context.Context, trashItemID string, restoredBy string, restoredAt time.Time) (domain.Page, error) {
	item, ok := r.trash[trashItemID]
	if !ok {
		return domain.Page{}, domain.ErrNotFound
	}
	page, ok := r.trashedPages[item.PageID]
	if !ok {
		page = domain.Page{ID: item.PageID, WorkspaceID: item.WorkspaceID, Title: item.PageTitle, CreatedBy: restoredBy, CreatedAt: item.DeletedAt, UpdatedAt: restoredAt}
	}
	page.UpdatedAt = restoredAt
	r.pages[page.ID] = page
	draft, ok := r.trashedDraft[item.PageID]
	if !ok {
		draft = domain.PageDraft{PageID: page.ID, Content: json.RawMessage(`[]`), LastEditedBy: restoredBy, CreatedAt: restoredAt, UpdatedAt: restoredAt}
	}
	draft.UpdatedAt = restoredAt
	r.drafts[page.ID] = draft
	delete(r.trash, trashItemID)
	delete(r.trashedPages, item.PageID)
	delete(r.trashedDraft, item.PageID)
	return page, nil
}

func TestPageServiceAdditionalBranches(t *testing.T) {
	folders := &fakeFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	memberships := &fakeWorkspaceRepo{memberships: map[string][]domain.WorkspaceMember{"w1": {{ID: "m1", WorkspaceID: "w1", UserID: "editor", Role: domain.RoleEditor}, {ID: "m2", WorkspaceID: "w1", UserID: "viewer", Role: domain.RoleViewer}}}, invitations: map[string]domain.WorkspaceInvitation{}, owners: map[string]int{}}
	repo := &pageRepoExtra{
		pages:        map[string]domain.Page{"p1": {ID: "p1", WorkspaceID: "w1", Title: "Doc"}},
		drafts:       map[string]domain.PageDraft{"p1": {PageID: "p1", Content: json.RawMessage(`[]`), LastEditedBy: "editor"}},
		trash:        map[string]domain.TrashItem{"t1": {ID: "t1", WorkspaceID: "w1", PageID: "p1", PageTitle: "Doc", DeletedBy: "editor", DeletedAt: time.Now().UTC()}},
		trashedPages: map[string]domain.Page{"p1": {ID: "p1", WorkspaceID: "w1", Title: "Doc"}},
		trashedDraft: map[string]domain.PageDraft{"p1": {PageID: "p1", Content: json.RawMessage(`[]`), LastEditedBy: "editor"}},
	}
	service := NewPageService(repo, memberships, folders)

	if _, _, err := service.CreatePage(context.Background(), "editor", CreatePageInput{WorkspaceID: "w1", Title: "   "}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected create page title validation, got %v", err)
	}

	emptyTitle := "   "
	if _, err := service.UpdatePage(context.Background(), "editor", UpdatePageInput{PageID: "p1", Title: &emptyTitle}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected update page title validation, got %v", err)
	}
	if _, err := service.UpdatePage(context.Background(), "editor", UpdatePageInput{PageID: "missing", Title: &emptyTitle}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected missing page on update, got %v", err)
	}

	if _, err := service.UpdateDraft(context.Background(), "editor", UpdateDraftInput{PageID: "p1", Content: nil}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected empty content validation, got %v", err)
	}

	if err := service.DeletePage(context.Background(), "editor", DeletePageInput{PageID: "missing"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected delete not found, got %v", err)
	}

	repo.listTrashErr = errors.New("list failed")
	if _, err := service.ListTrash(context.Background(), "editor", "w1"); err == nil || err.Error() != "list failed" {
		t.Fatalf("expected list trash error propagation, got %v", err)
	}
	repo.listTrashErr = nil
	if _, err := service.ListTrash(context.Background(), "viewer", "missing"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden for missing workspace membership, got %v", err)
	}

	if _, err := service.RestoreTrashItem(context.Background(), "viewer", RestoreTrashItemInput{TrashItemID: "t1"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected viewer restore forbidden, got %v", err)
	}
	if _, _, _, err := service.GetTrashPage(context.Background(), "viewer", GetTrashPageInput{TrashItemID: "t1"}); err != nil {
		t.Fatalf("expected viewer trash preview success, got %v", err)
	}
	if _, err := service.RestoreTrashItem(context.Background(), "editor", RestoreTrashItemInput{TrashItemID: "missing"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected restore not found, got %v", err)
	}
	if _, _, _, err := service.GetTrashPage(context.Background(), "editor", GetTrashPageInput{TrashItemID: "missing"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected trash preview not found, got %v", err)
	}
	if _, err := service.RestoreTrashItem(context.Background(), "editor", RestoreTrashItemInput{TrashItemID: "t1"}); err != nil {
		t.Fatalf("expected restore success, got %v", err)
	}
}
