package http

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"note-app/internal/application"
	appauth "note-app/internal/infrastructure/auth"
	"note-app/internal/infrastructure/storage"
)

func TestHandlersInvalidJSONBranches(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir()))

	accessToken, _, err := tokenManager.GenerateAccessToken("user-1", "u@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		needAuth   bool
		expectCode int
	}{
		{name: "login invalid json", method: http.MethodPost, path: "/api/v1/auth/login", body: `{"email":`, expectCode: http.StatusBadRequest},
		{name: "refresh invalid json", method: http.MethodPost, path: "/api/v1/auth/refresh", body: `{"refresh_token":`, expectCode: http.StatusBadRequest},
		{name: "logout invalid json", method: http.MethodPost, path: "/api/v1/auth/logout", body: `{"refresh_token":`, expectCode: http.StatusBadRequest},
		{name: "create workspace invalid json", method: http.MethodPost, path: "/api/v1/workspaces", body: `{"name":`, needAuth: true, expectCode: http.StatusBadRequest},
		{name: "rename workspace invalid json", method: http.MethodPatch, path: "/api/v1/workspaces/w1", body: `{"name":`, needAuth: true, expectCode: http.StatusBadRequest},
		{name: "invite member invalid json", method: http.MethodPost, path: "/api/v1/workspaces/w1/invitations", body: `{"email":`, needAuth: true, expectCode: http.StatusBadRequest},
		{name: "update member role invalid json", method: http.MethodPatch, path: "/api/v1/workspaces/w1/members/m1/role", body: `{"role":`, needAuth: true, expectCode: http.StatusBadRequest},
		{name: "create folder invalid json", method: http.MethodPost, path: "/api/v1/workspaces/w1/folders", body: `{"name":`, needAuth: true, expectCode: http.StatusBadRequest},
		{name: "rename folder invalid json", method: http.MethodPatch, path: "/api/v1/folders/f1", body: `{"name":`, needAuth: true, expectCode: http.StatusBadRequest},
		{name: "create page invalid json", method: http.MethodPost, path: "/api/v1/workspaces/w1/pages", body: `{"title":`, needAuth: true, expectCode: http.StatusBadRequest},
		{name: "update page invalid folder id payload", method: http.MethodPatch, path: "/api/v1/pages/p1", body: `{"folder_id":{}}`, needAuth: true, expectCode: http.StatusBadRequest},
		{name: "update draft invalid json", method: http.MethodPut, path: "/api/v1/pages/p1/draft", body: `{"content":`, needAuth: true, expectCode: http.StatusBadRequest},
		{name: "create revision invalid json", method: http.MethodPost, path: "/api/v1/pages/p1/revisions", body: `{"label":`, needAuth: true, expectCode: http.StatusBadRequest},
		{name: "create comment invalid json", method: http.MethodPost, path: "/api/v1/pages/p1/comments", body: `{"body":`, needAuth: true, expectCode: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.needAuth {
				req.Header.Set("Authorization", "Bearer "+accessToken)
			}
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.expectCode {
				t.Fatalf("expected status %d, got %d body=%s", tc.expectCode, rec.Code, rec.Body.String())
			}
		})
	}
}
