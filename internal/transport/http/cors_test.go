package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"note-app/internal/application"
	"note-app/internal/domain"
	appauth "note-app/internal/infrastructure/auth"
	"note-app/internal/infrastructure/storage"
	appmiddleware "note-app/internal/transport/http/middleware"
)

func TestAuthPreflightAllowsConfiguredOrigin(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	server := NewServer(logger, authService, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).
		WithCORSConfig(appmiddleware.CORSConfig{
			AllowedOrigins: []string{"http://localhost:5173"},
		})
	handler := server.Handler()

	tests := []struct {
		name                 string
		method               string
		path                 string
		requestMethod        string
		requestHeaders       string
		expectedAllowMethods string
		expectedAllowHeaders string
	}{
		{
			name:                 "register",
			method:               nethttp.MethodOptions,
			path:                 "/api/v1/auth/register",
			requestMethod:        nethttp.MethodPost,
			requestHeaders:       "Content-Type",
			expectedAllowMethods: "POST",
			expectedAllowHeaders: "Content-Type",
		},
		{
			name:                 "login",
			method:               nethttp.MethodOptions,
			path:                 "/api/v1/auth/login",
			requestMethod:        nethttp.MethodPost,
			requestHeaders:       "Content-Type",
			expectedAllowMethods: "POST",
			expectedAllowHeaders: "Content-Type",
		},
		{
			name:                 "refresh",
			method:               nethttp.MethodOptions,
			path:                 "/api/v1/auth/refresh",
			requestMethod:        nethttp.MethodPost,
			requestHeaders:       "Content-Type",
			expectedAllowMethods: "POST",
			expectedAllowHeaders: "Content-Type",
		},
		{
			name:                 "logout",
			method:               nethttp.MethodOptions,
			path:                 "/api/v1/auth/logout",
			requestMethod:        nethttp.MethodPost,
			requestHeaders:       "Content-Type",
			expectedAllowMethods: "POST",
			expectedAllowHeaders: "Content-Type",
		},
		{
			name:                 "me",
			method:               nethttp.MethodOptions,
			path:                 "/api/v1/auth/me",
			requestMethod:        nethttp.MethodGet,
			requestHeaders:       "Authorization",
			expectedAllowMethods: "GET",
			expectedAllowHeaders: "Authorization",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Origin", "http://localhost:5173")
			req.Header.Set("Access-Control-Request-Method", tc.requestMethod)
			req.Header.Set("Access-Control-Request-Headers", tc.requestHeaders)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != nethttp.StatusNoContent {
				t.Fatalf("expected 204, got %d body=%s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
				t.Fatalf("expected allow origin header, got %q", got)
			}
			if got := rec.Header().Get("Access-Control-Allow-Methods"); got != tc.expectedAllowMethods {
				t.Fatalf("expected allow methods %q, got %q", tc.expectedAllowMethods, got)
			}
			if got := rec.Header().Get("Access-Control-Allow-Headers"); got != tc.expectedAllowHeaders {
				t.Fatalf("expected allow headers %q, got %q", tc.expectedAllowHeaders, got)
			}
			if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
				t.Fatalf("expected no credentials header, got %q", got)
			}
			if got := rec.Header().Get("Vary"); got == "" {
				t.Fatal("expected vary header to be present")
			}
		})
	}
}

func TestAuthPreflightRejectsDisallowedOrigin(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	server := NewServer(logger, authService, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).
		WithCORSConfig(appmiddleware.CORSConfig{
			AllowedOrigins: []string{"http://localhost:5173"},
		})

	req := httptest.NewRequest(nethttp.MethodOptions, "/api/v1/auth/register", nil)
	req.Header.Set("Origin", "https://malicious.example.com")
	req.Header.Set("Access-Control-Request-Method", nethttp.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for disallowed origin preflight, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no allow origin header, got %q", got)
	}
}

func TestAuthRouteReturnsCORSHeadersForAllowedOrigin(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokenManager := appauth.NewTokenManager("super-secret-token", "note-app", 15*time.Minute)
	userRepo := &httpUserRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	refreshRepo := &httpRefreshTokenRepo{byHash: map[string]domain.RefreshToken{}}
	authService := application.NewAuthService(userRepo, refreshRepo, appauth.NewPasswordManager(), tokenManager, 24*time.Hour)
	server := NewServer(logger, authService, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, tokenManager, storage.NewLocal(t.TempDir())).
		WithCORSConfig(appmiddleware.CORSConfig{
			AllowedOrigins: []string{"http://localhost:5173"},
		})

	req := httptest.NewRequest(nethttp.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"owner@example.com","password":"Password1","full_name":"Owner"}`))
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected allow origin header, got %q", got)
	}
	assertNoStoreHeaders(t, rec)

	var payload struct {
		Data domain.User `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal register payload: %v", err)
	}
	if payload.Data.Email != "owner@example.com" {
		t.Fatalf("unexpected register payload: %+v", payload.Data)
	}
}
