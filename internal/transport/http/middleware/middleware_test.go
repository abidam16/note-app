package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appauth "note-app/internal/infrastructure/auth"
)

func TestAuthenticateMiddleware(t *testing.T) {
	tm := appauth.NewTokenManager("secret", "issuer", 15*time.Minute)
	token, _, err := tm.GenerateAccessToken("user-1", "u@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	okHandler := Authenticate(tm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value(ContextUserIDKey).(string)
		if userID != "user-1" {
			t.Fatalf("expected user id in context, got %q", userID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	okHandler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}

	for _, tc := range []struct {
		name   string
		header string
	}{
		{name: "missing", header: ""},
		{name: "invalid", header: "Bearer bad-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := Authenticate(tm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", res.Code)
			}
		})
	}
}

func TestLoggerMiddleware(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))

	h := Logger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/log", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Code)
	}

	line, err := io.ReadAll(&output)
	if err != nil {
		t.Fatalf("read logger output: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(line, &payload); err != nil {
		t.Fatalf("parse logger output: %v", err)
	}
	if payload["msg"] != "http request" {
		t.Fatalf("unexpected log msg: %v", payload["msg"])
	}
}

func TestStatusRecorderWriteHeader(t *testing.T) {
	r := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	r.WriteHeader(http.StatusAccepted)
	if r.status != http.StatusAccepted {
		t.Fatalf("expected status to be recorded, got %d", r.status)
	}
}
