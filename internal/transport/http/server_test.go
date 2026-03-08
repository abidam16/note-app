package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"note-app/internal/application"
	"note-app/internal/domain"
	appauth "note-app/internal/infrastructure/auth"
	"note-app/internal/infrastructure/storage"
)

type testMembershipRepo struct {
	memberships map[string][]domain.WorkspaceMember
}

func (r *testMembershipRepo) GetMembershipByUserID(_ context.Context, workspaceID, userID string) (domain.WorkspaceMember, error) {
	for _, member := range r.memberships[workspaceID] {
		if member.UserID == userID {
			return member, nil
		}
	}
	return domain.WorkspaceMember{}, domain.ErrForbidden
}

type testFolderRepo struct {
	byID        map[string]domain.Folder
	byWorkspace map[string][]domain.Folder
}

func (r *testFolderRepo) Create(_ context.Context, folder domain.Folder) (domain.Folder, error) {
	r.byID[folder.ID] = folder
	r.byWorkspace[folder.WorkspaceID] = append(r.byWorkspace[folder.WorkspaceID], folder)
	return folder, nil
}

func (r *testFolderRepo) GetByID(_ context.Context, folderID string) (domain.Folder, error) {
	folder, ok := r.byID[folderID]
	if !ok {
		return domain.Folder{}, domain.ErrNotFound
	}
	return folder, nil
}

func (r *testFolderRepo) ListByWorkspaceID(_ context.Context, workspaceID string) ([]domain.Folder, error) {
	return r.byWorkspace[workspaceID], nil
}

type testPageRepo struct {
	pages  map[string]domain.Page
	drafts map[string]domain.PageDraft
	trash  map[string]domain.TrashItem
}

func (r *testPageRepo) CreateWithDraft(_ context.Context, page domain.Page, draft domain.PageDraft) (domain.Page, domain.PageDraft, error) {
	r.pages[page.ID] = page
	r.drafts[draft.PageID] = draft
	return page, draft, nil
}

func (r *testPageRepo) GetByID(_ context.Context, pageID string) (domain.Page, domain.PageDraft, error) {
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

func (r *testPageRepo) UpdateMetadata(_ context.Context, pageID string, title string, folderID *string, updatedAt time.Time) (domain.Page, error) {
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

func (r *testPageRepo) UpdateDraft(_ context.Context, pageID string, content json.RawMessage, lastEditedBy string, updatedAt time.Time) (domain.PageDraft, error) {
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

func (r *testPageRepo) SoftDelete(_ context.Context, trashItem domain.TrashItem) error {
	if _, ok := r.pages[trashItem.PageID]; !ok {
		return domain.ErrNotFound
	}
	delete(r.pages, trashItem.PageID)
	if r.trash == nil {
		r.trash = map[string]domain.TrashItem{}
	}
	r.trash[trashItem.ID] = trashItem
	return nil
}

func (r *testPageRepo) ListTrashByWorkspaceID(_ context.Context, workspaceID string) ([]domain.TrashItem, error) {
	items := make([]domain.TrashItem, 0)
	for _, item := range r.trash {
		if item.WorkspaceID == workspaceID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *testPageRepo) GetTrashItemByID(_ context.Context, trashItemID string) (domain.TrashItem, error) {
	item, ok := r.trash[trashItemID]
	if !ok {
		return domain.TrashItem{}, domain.ErrNotFound
	}
	return item, nil
}

func (r *testPageRepo) RestoreTrashItem(_ context.Context, trashItemID string, _ string, restoredAt time.Time) (domain.Page, error) {
	item, ok := r.trash[trashItemID]
	if !ok {
		return domain.Page{}, domain.ErrNotFound
	}
	page := domain.Page{
		ID:          item.PageID,
		WorkspaceID: item.WorkspaceID,
		Title:       item.PageTitle,
		CreatedBy:   item.DeletedBy,
		CreatedAt:   item.DeletedAt,
		UpdatedAt:   restoredAt,
	}
	r.pages[page.ID] = page
	delete(r.trash, trashItemID)
	return page, nil
}

type testRevisionRepo struct {
	revisions map[string]domain.Revision
	ordered   []domain.Revision
}

func (r *testRevisionRepo) Create(_ context.Context, revision domain.Revision) (domain.Revision, error) {
	r.revisions[revision.ID] = revision
	r.ordered = append(r.ordered, revision)
	return revision, nil
}

func (r *testRevisionRepo) GetByID(_ context.Context, revisionID string) (domain.Revision, error) {
	revision, ok := r.revisions[revisionID]
	if !ok {
		return domain.Revision{}, domain.ErrNotFound
	}
	return revision, nil
}

func (r *testRevisionRepo) ListByPageID(_ context.Context, pageID string) ([]domain.Revision, error) {
	result := make([]domain.Revision, 0)
	for _, revision := range r.ordered {
		if revision.PageID == pageID {
			revision.Content = nil
			result = append(result, revision)
		}
	}
	return result, nil
}

type testCommentRepo struct {
	comments map[string]domain.PageComment
	ordered  []domain.PageComment
}

func (r *testCommentRepo) Create(_ context.Context, comment domain.PageComment) (domain.PageComment, error) {
	r.comments[comment.ID] = comment
	r.ordered = append(r.ordered, comment)
	return comment, nil
}

func (r *testCommentRepo) GetByID(_ context.Context, commentID string) (domain.PageComment, error) {
	comment, ok := r.comments[commentID]
	if !ok {
		return domain.PageComment{}, domain.ErrNotFound
	}
	return comment, nil
}

func (r *testCommentRepo) ListByPageID(_ context.Context, pageID string) ([]domain.PageComment, error) {
	result := make([]domain.PageComment, 0)
	for _, comment := range r.ordered {
		if comment.PageID == pageID {
			result = append(result, r.comments[comment.ID])
		}
	}
	return result, nil
}

func (r *testCommentRepo) Resolve(_ context.Context, commentID string, resolvedBy string, resolvedAt time.Time) (domain.PageComment, error) {
	comment, ok := r.comments[commentID]
	if !ok {
		return domain.PageComment{}, domain.ErrNotFound
	}
	comment.ResolvedBy = &resolvedBy
	comment.ResolvedAt = &resolvedAt
	r.comments[commentID] = comment
	for idx := range r.ordered {
		if r.ordered[idx].ID == commentID {
			r.ordered[idx] = comment
		}
	}
	return comment, nil
}

type testSearchRepo struct {
	resultsByQuery map[string][]domain.PageSearchResult
}

func (r *testSearchRepo) SearchPages(_ context.Context, workspaceID string, query string) ([]domain.PageSearchResult, error) {
	results := r.resultsByQuery[workspaceID+":"+query]
	return results, nil
}

func TestHealthEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute), storage.NewLocal(t.TempDir()))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Data.Status != "ok" {
		t.Fatalf("expected status ok, got %s", payload.Data.Status)
	}
}

func TestFolderEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
			},
		},
	}
	folders := &testFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	folderService := application.NewFolderService(folders, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, folderService, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	accessToken, _, err := tokenManager.GenerateAccessToken("user-1", "user@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/folders", bytes.NewBufferString(`{"name":"Engineering"}`))
	createReq.Header.Set("Authorization", "Bearer "+accessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/folders", nil)
	listReq.Header.Set("Authorization", "Bearer "+accessToken)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	var payload struct {
		Data []domain.Folder `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal folders response: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].Name != "Engineering" {
		t.Fatalf("unexpected folders payload: %+v", payload.Data)
	}
}

func TestPageEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
			},
		},
	}
	folders := &testFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &testPageRepo{pages: map[string]domain.Page{}, drafts: map[string]domain.PageDraft{}}
	pageService := application.NewPageService(pages, memberships, folders)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, pageService, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	accessToken, _, err := tokenManager.GenerateAccessToken("user-1", "user@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/pages", bytes.NewBufferString(`{"title":"Architecture"}`))
	createReq.Header.Set("Authorization", "Bearer "+accessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		Data struct {
			Page domain.Page `json:"page"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create page response: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/"+created.Data.Page.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+accessToken)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var payload struct {
		Data struct {
			Page  domain.Page      `json:"page"`
			Draft domain.PageDraft `json:"draft"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal get page response: %v", err)
	}
	if payload.Data.Page.Title != "Architecture" || string(payload.Data.Draft.Content) != "[]" {
		t.Fatalf("unexpected page payload: %+v", payload.Data)
	}
}

func TestPageUpdateEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
			},
		},
	}
	folders := &testFolderRepo{
		byID: map[string]domain.Folder{
			"folder-1": {ID: "folder-1", WorkspaceID: "workspace-1", Name: "Engineering"},
		},
		byWorkspace: map[string][]domain.Folder{"workspace-1": {{ID: "folder-1", WorkspaceID: "workspace-1", Name: "Engineering"}}},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Old Title"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage("[]")},
		},
	}
	pageService := application.NewPageService(pages, memberships, folders)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, pageService, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	editorToken, _, err := tokenManager.GenerateAccessToken("user-1", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("user-2", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	renameReq := httptest.NewRequest(http.MethodPatch, "/api/v1/pages/page-1", bytes.NewBufferString(`{"title":"New Title"}`))
	renameReq.Header.Set("Authorization", "Bearer "+editorToken)
	renameReq.Header.Set("Content-Type", "application/json")
	renameRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(renameRec, renameReq)
	if renameRec.Code != http.StatusOK {
		t.Fatalf("expected rename status 200, got %d body=%s", renameRec.Code, renameRec.Body.String())
	}

	moveReq := httptest.NewRequest(http.MethodPatch, "/api/v1/pages/page-1", bytes.NewBufferString(`{"folder_id":"folder-1"}`))
	moveReq.Header.Set("Authorization", "Bearer "+editorToken)
	moveReq.Header.Set("Content-Type", "application/json")
	moveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(moveRec, moveReq)
	if moveRec.Code != http.StatusOK {
		t.Fatalf("expected move status 200, got %d body=%s", moveRec.Code, moveRec.Body.String())
	}

	moveRootReq := httptest.NewRequest(http.MethodPatch, "/api/v1/pages/page-1", bytes.NewBufferString(`{"folder_id":null}`))
	moveRootReq.Header.Set("Authorization", "Bearer "+editorToken)
	moveRootReq.Header.Set("Content-Type", "application/json")
	moveRootRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(moveRootRec, moveRootReq)
	if moveRootRec.Code != http.StatusOK {
		t.Fatalf("expected move root status 200, got %d body=%s", moveRootRec.Code, moveRootRec.Body.String())
	}

	invalidReq := httptest.NewRequest(http.MethodPatch, "/api/v1/pages/page-1", bytes.NewBufferString(`{"folder_id":"folder-x"}`))
	invalidReq.Header.Set("Authorization", "Bearer "+editorToken)
	invalidReq.Header.Set("Content-Type", "application/json")
	invalidRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusNotFound {
		t.Fatalf("expected invalid folder status 404, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}

	viewerReq := httptest.NewRequest(http.MethodPatch, "/api/v1/pages/page-1", bytes.NewBufferString(`{"title":"Viewer Attempt"}`))
	viewerReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerReq.Header.Set("Content-Type", "application/json")
	viewerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer status 403, got %d body=%s", viewerRec.Code, viewerRec.Body.String())
	}
}

func TestPageDraftEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
			},
		},
	}
	folders := &testFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage("[]"), LastEditedBy: "user-1"},
		},
	}
	pageService := application.NewPageService(pages, memberships, folders)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, pageService, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	editorToken, _, err := tokenManager.GenerateAccessToken("user-1", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("user-2", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	validContent := `{"content":[{"type":"paragraph","children":[{"type":"text","text":"hello","marks":[{"type":"bold"},{"type":"link","href":"https://example.com/docs"}]}]}]}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(validContent))
	updateReq.Header.Set("Authorization", "Bearer "+editorToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected draft update status 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1", nil)
	getReq.Header.Set("Authorization", "Bearer "+editorToken)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected get page status 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var payload struct {
		Data struct {
			Draft domain.PageDraft `json:"draft"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal updated page response: %v", err)
	}
	if string(payload.Data.Draft.Content) != `[{"type":"paragraph","children":[{"type":"text","text":"hello","marks":[{"type":"bold"},{"type":"link","href":"https://example.com/docs"}]}]}]` {
		t.Fatalf("unexpected draft content: %s", string(payload.Data.Draft.Content))
	}

	invalidBlockReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(`{"content":[{"type":"unsupported"}]}`))
	invalidBlockReq.Header.Set("Authorization", "Bearer "+editorToken)
	invalidBlockReq.Header.Set("Content-Type", "application/json")
	invalidBlockRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidBlockRec, invalidBlockReq)
	if invalidBlockRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid block status 422, got %d body=%s", invalidBlockRec.Code, invalidBlockRec.Body.String())
	}

	invalidLinkReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(`{"content":[{"type":"paragraph","children":[{"type":"text","text":"bad","marks":[{"type":"link","href":"notaurl"}]}]}]}`))
	invalidLinkReq.Header.Set("Authorization", "Bearer "+editorToken)
	invalidLinkReq.Header.Set("Content-Type", "application/json")
	invalidLinkRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidLinkRec, invalidLinkReq)
	if invalidLinkRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid link status 422, got %d body=%s", invalidLinkRec.Code, invalidLinkRec.Body.String())
	}

	invalidImageReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(`{"content":[{"type":"image","alt":"missing src"}]}`))
	invalidImageReq.Header.Set("Authorization", "Bearer "+editorToken)
	invalidImageReq.Header.Set("Content-Type", "application/json")
	invalidImageRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidImageRec, invalidImageReq)
	if invalidImageRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid image status 422, got %d body=%s", invalidImageRec.Code, invalidImageRec.Body.String())
	}

	viewerReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/page-1/draft", bytes.NewBufferString(`{"content":[]}`))
	viewerReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerReq.Header.Set("Content-Type", "application/json")
	viewerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer draft status 403, got %d body=%s", viewerRec.Code, viewerRec.Body.String())
	}

	missingReq := httptest.NewRequest(http.MethodPut, "/api/v1/pages/missing-page/draft", bytes.NewBufferString(`{"content":[]}`))
	missingReq.Header.Set("Authorization", "Bearer "+editorToken)
	missingReq.Header.Set("Content-Type", "application/json")
	missingRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing page status 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}
}

func TestPageRevisionEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"checkpoint"}]}]`), LastEditedBy: "user-1"},
		},
	}
	revisions := &testRevisionRepo{revisions: map[string]domain.Revision{}}
	revisionService := application.NewRevisionService(revisions, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, revisionService, tokenManager, storage.NewLocal(t.TempDir()))

	editorToken, _, err := tokenManager.GenerateAccessToken("user-1", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("user-2", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions", bytes.NewBufferString(`{"label":"Milestone 1","note":"Before rewrite"}`))
	createReq.Header.Set("Authorization", "Bearer "+editorToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected revision create status 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	if len(revisions.revisions) != 1 {
		t.Fatalf("expected one revision, got %d", len(revisions.revisions))
	}
	for _, revision := range revisions.revisions {
		if revision.Label == nil || *revision.Label != "Milestone 1" {
			t.Fatalf("unexpected label: %+v", revision.Label)
		}
		if revision.Note == nil || *revision.Note != "Before rewrite" {
			t.Fatalf("unexpected note: %+v", revision.Note)
		}
		if string(revision.Content) != string(pages.drafts["page-1"].Content) {
			t.Fatalf("expected revision content to match draft")
		}
	}

	viewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions", bytes.NewBufferString(`{}`))
	viewerReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerReq.Header.Set("Content-Type", "application/json")
	viewerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer revision status 403, got %d body=%s", viewerRec.Code, viewerRec.Body.String())
	}

	invalidReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions", bytes.NewBufferString(`{"label":true}`))
	invalidReq.Header.Set("Authorization", "Bearer "+editorToken)
	invalidReq.Header.Set("Content-Type", "application/json")
	invalidRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid json status 400, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
}

func TestPageRevisionHistoryEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	revisions := &testRevisionRepo{revisions: map[string]domain.Revision{}, ordered: []domain.Revision{
		{ID: "rev-1", PageID: "page-1", Label: stringPtrHTTP("First"), CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph"}]`)},
		{ID: "rev-2", PageID: "page-1", Label: stringPtrHTTP("Second"), CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph"}]`)},
	}}
	revisionService := application.NewRevisionService(revisions, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, revisionService, tokenManager, storage.NewLocal(t.TempDir()))

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-2", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/revisions", nil)
	listReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	var payload struct {
		Data []domain.Revision `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal revision list response: %v", err)
	}
	if len(payload.Data) != 2 {
		t.Fatalf("expected two revisions, got %d", len(payload.Data))
	}
	if payload.Data[0].ID != "rev-1" || payload.Data[1].ID != "rev-2" {
		t.Fatalf("unexpected order: %+v", payload.Data)
	}
	if payload.Data[0].Content != nil || payload.Data[1].Content != nil {
		t.Fatalf("expected history payload to omit content")
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/missing-page/revisions", nil)
	missingReq.Header.Set("Authorization", "Bearer "+viewerToken)
	missingRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing page status 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}
}

func stringPtrHTTP(value string) *string {
	return &value
}

func TestPageRevisionCompareEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	revisions := &testRevisionRepo{revisions: map[string]domain.Revision{
		"rev-1": {ID: "rev-1", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"hello world"}]}]`)},
		"rev-2": {ID: "rev-2", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"hello brave world"}]},{"type":"image","src":"/uploads/a.png"}]`)},
		"rev-x": {ID: "rev-x", PageID: "page-x", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"other"}]}]`)},
	}, ordered: []domain.Revision{}}
	revisionService := application.NewRevisionService(revisions, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, revisionService, tokenManager, storage.NewLocal(t.TempDir()))

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	compareReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/revisions/compare?from=rev-1&to=rev-2", nil)
	compareReq.Header.Set("Authorization", "Bearer "+viewerToken)
	compareRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(compareRec, compareReq)
	if compareRec.Code != http.StatusOK {
		t.Fatalf("expected compare status 200, got %d body=%s", compareRec.Code, compareRec.Body.String())
	}

	var payload struct {
		Data domain.RevisionDiff `json:"data"`
	}
	if err := json.Unmarshal(compareRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal compare response: %v", err)
	}
	if payload.Data.FromRevisionID != "rev-1" || payload.Data.ToRevisionID != "rev-2" {
		t.Fatalf("unexpected compare payload: %+v", payload.Data)
	}
	if len(payload.Data.Blocks) != 2 {
		t.Fatalf("expected two diff blocks, got %d", len(payload.Data.Blocks))
	}
	if payload.Data.Blocks[0].Status != "modified" || payload.Data.Blocks[1].Status != "added" {
		t.Fatalf("unexpected diff blocks: %+v", payload.Data.Blocks)
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/revisions/compare?from=rev-1&to=rev-x", nil)
	invalidReq.Header.Set("Authorization", "Bearer "+viewerToken)
	invalidRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid comparison status 422, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/revisions/compare?from=rev-1&to=missing", nil)
	missingReq.Header.Set("Authorization", "Bearer "+viewerToken)
	missingRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing revision status 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}
}

func TestPageRevisionRestoreEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleViewer},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"current"}]}]`), LastEditedBy: "user-1"},
		},
	}
	revisions := &testRevisionRepo{revisions: map[string]domain.Revision{
		"rev-1": {ID: "rev-1", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"old value"}]}]`)},
		"rev-2": {ID: "rev-2", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"current"}]}]`)},
		"rev-x": {ID: "rev-x", PageID: "page-x", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC), Content: json.RawMessage(`[{"type":"paragraph","children":[{"type":"text","text":"other"}]}]`)},
	}, ordered: []domain.Revision{
		{ID: "rev-1", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)},
		{ID: "rev-2", PageID: "page-1", CreatedBy: "user-1", CreatedAt: time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC)},
	}}
	revisionService := application.NewRevisionService(revisions, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, revisionService, tokenManager, storage.NewLocal(t.TempDir()))

	editorToken, _, err := tokenManager.GenerateAccessToken("user-1", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("user-2", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions/rev-1/restore", nil)
	restoreReq.Header.Set("Authorization", "Bearer "+editorToken)
	restoreRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected restore status 200, got %d body=%s", restoreRec.Code, restoreRec.Body.String())
	}
	if string(pages.drafts["page-1"].Content) != string(revisions.revisions["rev-1"].Content) {
		t.Fatalf("expected draft content to be restored")
	}
	if len(revisions.ordered) != 3 {
		t.Fatalf("expected new revision event, got %d history entries", len(revisions.ordered))
	}

	viewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions/rev-1/restore", nil)
	viewerReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer restore status 403, got %d body=%s", viewerRec.Code, viewerRec.Body.String())
	}

	invalidReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/revisions/rev-x/restore", nil)
	invalidReq.Header.Set("Authorization", "Bearer "+editorToken)
	invalidRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected mismatched revision status 422, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
}
func TestCommentEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
				{ID: "member-2", WorkspaceID: "workspace-1", UserID: "user-2", Role: domain.RoleEditor},
			},
		},
	}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc"},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
	}
	comments := &testCommentRepo{comments: map[string]domain.PageComment{}, ordered: []domain.PageComment{}}
	commentService := application.NewCommentService(comments, pages, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithCommentService(commentService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	editorToken, _, err := tokenManager.GenerateAccessToken("user-2", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/comments", bytes.NewBufferString(`{"body":"  Please verify this section  "}`))
	createReq.Header.Set("Authorization", "Bearer "+viewerToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create comment status 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		Data domain.PageComment `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create comment response: %v", err)
	}
	if created.Data.Body != "Please verify this section" {
		t.Fatalf("unexpected comment body: %q", created.Data.Body)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/comments", nil)
	listReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list comments status 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	var listed struct {
		Data []domain.PageComment `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list comments response: %v", err)
	}
	if len(listed.Data) != 1 {
		t.Fatalf("expected one comment, got %d", len(listed.Data))
	}
	if listed.Data[0].ResolvedAt != nil {
		t.Fatalf("expected unresolved comment before resolve")
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/comments/"+created.Data.ID+"/resolve", nil)
	resolveReq.Header.Set("Authorization", "Bearer "+editorToken)
	resolveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("expected resolve status 200, got %d body=%s", resolveRec.Code, resolveRec.Body.String())
	}

	viewerResolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/comments/"+created.Data.ID+"/resolve", nil)
	viewerResolveReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerResolveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerResolveRec, viewerResolveReq)
	if viewerResolveRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer resolve status 403, got %d body=%s", viewerResolveRec.Code, viewerResolveRec.Body.String())
	}

	listAfterResolveReq := httptest.NewRequest(http.MethodGet, "/api/v1/pages/page-1/comments", nil)
	listAfterResolveReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listAfterResolveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listAfterResolveRec, listAfterResolveReq)
	if listAfterResolveRec.Code != http.StatusOK {
		t.Fatalf("expected post-resolve list status 200, got %d body=%s", listAfterResolveRec.Code, listAfterResolveRec.Body.String())
	}

	var listedAfterResolve struct {
		Data []domain.PageComment `json:"data"`
	}
	if err := json.Unmarshal(listAfterResolveRec.Body.Bytes(), &listedAfterResolve); err != nil {
		t.Fatalf("unmarshal post-resolve list response: %v", err)
	}
	if len(listedAfterResolve.Data) != 1 || listedAfterResolve.Data[0].ResolvedAt == nil {
		t.Fatalf("expected resolved comment to remain visible: %+v", listedAfterResolve.Data)
	}

	emptyBodyReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/page-1/comments", bytes.NewBufferString(`{"body":"   "}`))
	emptyBodyReq.Header.Set("Authorization", "Bearer "+viewerToken)
	emptyBodyReq.Header.Set("Content-Type", "application/json")
	emptyBodyRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(emptyBodyRec, emptyBodyReq)
	if emptyBodyRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected empty body status 422, got %d body=%s", emptyBodyRec.Code, emptyBodyRec.Body.String())
	}

	missingPageReq := httptest.NewRequest(http.MethodPost, "/api/v1/pages/missing-page/comments", bytes.NewBufferString(`{"body":"hello"}`))
	missingPageReq.Header.Set("Authorization", "Bearer "+viewerToken)
	missingPageReq.Header.Set("Content-Type", "application/json")
	missingPageRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingPageRec, missingPageReq)
	if missingPageRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing page status 404, got %d body=%s", missingPageRec.Code, missingPageRec.Body.String())
	}
}
func TestSearchEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{
		memberships: map[string][]domain.WorkspaceMember{
			"workspace-1": {
				{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleViewer},
			},
		},
	}
	searches := &testSearchRepo{resultsByQuery: map[string][]domain.PageSearchResult{
		"workspace-1:architecture": {
			{ID: "page-title", WorkspaceID: "workspace-1", Title: "Architecture Spec", UpdatedAt: time.Date(2026, 3, 7, 13, 0, 0, 0, time.UTC)},
		},
		"workspace-1:postgres": {
			{ID: "page-body", WorkspaceID: "workspace-1", Title: "Storage Notes", UpdatedAt: time.Date(2026, 3, 7, 14, 0, 0, 0, time.UTC)},
		},
	}}
	searchService := application.NewSearchService(searches, memberships)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).WithSearchService(searchService)

	viewerToken, _, err := tokenManager.GenerateAccessToken("user-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	titleReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/search?q=architecture", nil)
	titleReq.Header.Set("Authorization", "Bearer "+viewerToken)
	titleRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(titleRec, titleReq)
	if titleRec.Code != http.StatusOK {
		t.Fatalf("expected title search status 200, got %d body=%s", titleRec.Code, titleRec.Body.String())
	}

	var titlePayload struct {
		Data []domain.PageSearchResult `json:"data"`
	}
	if err := json.Unmarshal(titleRec.Body.Bytes(), &titlePayload); err != nil {
		t.Fatalf("unmarshal title search response: %v", err)
	}
	if len(titlePayload.Data) != 1 || titlePayload.Data[0].ID != "page-title" {
		t.Fatalf("unexpected title search results: %+v", titlePayload.Data)
	}

	bodyReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/search?q=postgres", nil)
	bodyReq.Header.Set("Authorization", "Bearer "+viewerToken)
	bodyRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(bodyRec, bodyReq)
	if bodyRec.Code != http.StatusOK {
		t.Fatalf("expected body search status 200, got %d body=%s", bodyRec.Code, bodyRec.Body.String())
	}

	var bodyPayload struct {
		Data []domain.PageSearchResult `json:"data"`
	}
	if err := json.Unmarshal(bodyRec.Body.Bytes(), &bodyPayload); err != nil {
		t.Fatalf("unmarshal body search response: %v", err)
	}
	if len(bodyPayload.Data) != 1 || bodyPayload.Data[0].ID != "page-body" {
		t.Fatalf("unexpected body search results: %+v", bodyPayload.Data)
	}

	missingQueryReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/search?q=%20%20%20", nil)
	missingQueryReq.Header.Set("Authorization", "Bearer "+viewerToken)
	missingQueryRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingQueryRec, missingQueryReq)
	if missingQueryRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected missing query status 422, got %d body=%s", missingQueryRec.Code, missingQueryRec.Body.String())
	}
}
func TestTrashEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {
			{ID: "member-1", WorkspaceID: "workspace-1", UserID: "editor-1", Role: domain.RoleEditor},
			{ID: "member-2", WorkspaceID: "workspace-1", UserID: "viewer-1", Role: domain.RoleViewer},
		},
	}}
	folders := &testFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &testPageRepo{
		pages: map[string]domain.Page{
			"page-1": {ID: "page-1", WorkspaceID: "workspace-1", Title: "Doc to Trash", CreatedBy: "editor-1", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		},
		drafts: map[string]domain.PageDraft{
			"page-1": {PageID: "page-1", Content: json.RawMessage(`[]`)},
		},
		trash: map[string]domain.TrashItem{},
	}
	pageService := application.NewPageService(pages, memberships, folders)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, pageService, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	editorToken, _, err := tokenManager.GenerateAccessToken("editor-1", "editor@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	viewerToken, _, err := tokenManager.GenerateAccessToken("viewer-1", "viewer@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	viewerDeleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/pages/page-1", nil)
	viewerDeleteReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerDeleteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(viewerDeleteRec, viewerDeleteReq)
	if viewerDeleteRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer delete status 403, got %d body=%s", viewerDeleteRec.Code, viewerDeleteRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/pages/page-1", nil)
	deleteReq.Header.Set("Authorization", "Bearer "+editorToken)
	deleteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected delete status 204, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	trashListReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/trash", nil)
	trashListReq.Header.Set("Authorization", "Bearer "+viewerToken)
	trashListRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(trashListRec, trashListReq)
	if trashListRec.Code != http.StatusOK {
		t.Fatalf("expected trash list status 200, got %d body=%s", trashListRec.Code, trashListRec.Body.String())
	}

	var trashPayload struct {
		Data []domain.TrashItem `json:"data"`
	}
	if err := json.Unmarshal(trashListRec.Body.Bytes(), &trashPayload); err != nil {
		t.Fatalf("unmarshal trash list response: %v", err)
	}
	if len(trashPayload.Data) != 1 {
		t.Fatalf("expected one trash item, got %d", len(trashPayload.Data))
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/trash/"+trashPayload.Data[0].ID+"/restore", nil)
	restoreReq.Header.Set("Authorization", "Bearer "+editorToken)
	restoreRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected restore status 200, got %d body=%s", restoreRec.Code, restoreRec.Body.String())
	}
}
