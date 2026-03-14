package http

import (
	"bytes"
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

func TestFolderAndPageHandlersTrimEmptyIDs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	memberships := &testMembershipRepo{memberships: map[string][]domain.WorkspaceMember{
		"workspace-1": {{ID: "member-1", WorkspaceID: "workspace-1", UserID: "user-1", Role: domain.RoleEditor}},
	}}
	folders := &testFolderRepo{byID: map[string]domain.Folder{}, byWorkspace: map[string][]domain.Folder{}}
	pages := &testPageRepo{pages: map[string]domain.Page{}, drafts: map[string]domain.PageDraft{}}
	folderService := application.NewFolderService(folders, memberships)
	pageService := application.NewPageService(pages, memberships, folders)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, folderService, pageService, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	accessToken, _, err := tokenManager.GenerateAccessToken("user-1", "user@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	createFolderReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/folders", bytes.NewBufferString(`{"name":"Parentless","parent_id":"   "}`))
	createFolderReq.Header.Set("Authorization", "Bearer "+accessToken)
	createFolderReq.Header.Set("Content-Type", "application/json")
	createFolderRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createFolderRec, createFolderReq)
	if createFolderRec.Code != http.StatusCreated {
		t.Fatalf("expected folder create 201, got %d body=%s", createFolderRec.Code, createFolderRec.Body.String())
	}
	var createdFolder struct {
		Data domain.Folder `json:"data"`
	}
	if err := json.Unmarshal(createFolderRec.Body.Bytes(), &createdFolder); err != nil {
		t.Fatalf("unmarshal folder payload: %v", err)
	}
	if createdFolder.Data.ParentID != nil {
		t.Fatalf("expected trimmed empty parent_id to become nil, got %+v", createdFolder.Data.ParentID)
	}

	createPageReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/pages", bytes.NewBufferString(`{"title":"Doc","folder_id":"   "}`))
	createPageReq.Header.Set("Authorization", "Bearer "+accessToken)
	createPageReq.Header.Set("Content-Type", "application/json")
	createPageRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createPageRec, createPageReq)
	if createPageRec.Code != http.StatusCreated {
		t.Fatalf("expected page create 201, got %d body=%s", createPageRec.Code, createPageRec.Body.String())
	}
	var createdPage struct {
		Data struct {
			Page domain.Page `json:"page"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createPageRec.Body.Bytes(), &createdPage); err != nil {
		t.Fatalf("unmarshal page payload: %v", err)
	}
	if createdPage.Data.Page.FolderID != nil {
		t.Fatalf("expected trimmed empty folder_id to become nil, got %+v", createdPage.Data.Page.FolderID)
	}
}
