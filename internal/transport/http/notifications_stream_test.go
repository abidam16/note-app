package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"note-app/internal/application"
	appauth "note-app/internal/infrastructure/auth"
	appmiddleware "note-app/internal/transport/http/middleware"
)

type notificationStreamServiceStub struct {
	openFn func(context.Context, string) (application.NotificationStreamSession, error)
}

func (s notificationStreamServiceStub) Open(ctx context.Context, actorID string) (application.NotificationStreamSession, error) {
	if s.openFn != nil {
		return s.openFn(ctx, actorID)
	}
	return nil, errors.New("unexpected open call")
}

type notificationStreamSessionStub struct {
	initialUnreadCount int64
	events             chan application.NotificationStreamEvent
	closeFn            func() error
}

func (s *notificationStreamSessionStub) InitialUnreadCount() int64 {
	return s.initialUnreadCount
}

func (s *notificationStreamSessionStub) Events() <-chan application.NotificationStreamEvent {
	return s.events
}

func (s *notificationStreamSessionStub) Close() error {
	if s.closeFn != nil {
		return s.closeFn()
	}
	return nil
}

type noFlushResponseWriter struct {
	header http.Header
	body   strings.Builder
	code   int
}

func (w *noFlushResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *noFlushResponseWriter) WriteHeader(statusCode int) {
	w.code = statusCode
}

func (w *noFlushResponseWriter) Write(p []byte) (int, error) {
	return w.body.Write(p)
}

func TestNotificationStreamEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	originalHeartbeat := notificationStreamHeartbeatInterval
	notificationStreamHeartbeatInterval = 10 * time.Millisecond
	defer func() {
		notificationStreamHeartbeatInterval = originalHeartbeat
	}()

	t.Run("opens stream and writes snapshot", func(t *testing.T) {
		events := make(chan application.NotificationStreamEvent)
		svc := notificationStreamServiceStub{
			openFn: func(context.Context, string) (application.NotificationStreamSession, error) {
				return &notificationStreamSessionStub{initialUnreadCount: 12, events: events}, nil
			},
		}
		server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, appauth.TokenManager{}, nil).
			WithNotificationStreamService(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/stream", nil)
		req = req.WithContext(context.WithValue(req.Context(), appmiddleware.ContextUserIDKey, "user-1"))
		rec := httptest.NewRecorder()

		close(events)
		server.handleNotificationsStream()(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
			t.Fatalf("unexpected content type: %s", got)
		}
		if !strings.Contains(rec.Body.String(), "event: snapshot") {
			t.Fatalf("expected snapshot event, got %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"unread_count":12`) {
			t.Fatalf("expected snapshot unread count, got %s", rec.Body.String())
		}
	})

	t.Run("writes unread count and invalidation events", func(t *testing.T) {
		events := make(chan application.NotificationStreamEvent, 2)
		svc := notificationStreamServiceStub{
			openFn: func(context.Context, string) (application.NotificationStreamSession, error) {
				return &notificationStreamSessionStub{initialUnreadCount: 3, events: events}, nil
			},
		}
		server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, appauth.TokenManager{}, nil).
			WithNotificationStreamService(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/stream", nil)
		req = req.WithContext(context.WithValue(req.Context(), appmiddleware.ContextUserIDKey, "user-1"))
		rec := httptest.NewRecorder()

		go func() {
			unread := int64(4)
			events <- application.NotificationStreamEvent{Type: "unread_count", UnreadCount: &unread, SentAt: time.Now().UTC()}
			events <- application.NotificationStreamEvent{Type: "inbox_invalidated", Reason: "notifications_changed", SentAt: time.Now().UTC()}
			close(events)
		}()

		server.handleNotificationsStream()(rec, req)

		if !strings.Contains(rec.Body.String(), "event: unread_count") {
			t.Fatalf("expected unread_count event, got %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "event: inbox_invalidated") {
			t.Fatalf("expected inbox_invalidated event, got %s", rec.Body.String())
		}
	})

	t.Run("emits heartbeats while idle", func(t *testing.T) {
		events := make(chan application.NotificationStreamEvent)
		svc := notificationStreamServiceStub{
			openFn: func(context.Context, string) (application.NotificationStreamSession, error) {
				return &notificationStreamSessionStub{initialUnreadCount: 1, events: events}, nil
			},
		}
		server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, appauth.TokenManager{}, nil).
			WithNotificationStreamService(svc)

		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/stream", nil).WithContext(context.WithValue(ctx, appmiddleware.ContextUserIDKey, "user-1"))
		rec := httptest.NewRecorder()

		done := make(chan struct{})
		go func() {
			time.Sleep(35 * time.Millisecond)
			cancel()
			close(done)
		}()

		server.handleNotificationsStream()(rec, req)
		<-done

		if !strings.Contains(rec.Body.String(), ": keep-alive") {
			t.Fatalf("expected heartbeat comment, got %s", rec.Body.String())
		}
	})

	t.Run("requires auth", func(t *testing.T) {
		server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, appauth.TokenManager{}, nil).
			WithNotificationStreamService(notificationStreamServiceStub{})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/stream", nil)
		rec := httptest.NewRecorder()

		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("fails before headers when stream open fails", func(t *testing.T) {
		svc := notificationStreamServiceStub{
			openFn: func(context.Context, string) (application.NotificationStreamSession, error) {
				return nil, errors.New("open failed")
			},
		}
		server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, appauth.TokenManager{}, nil).
			WithNotificationStreamService(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/stream", nil)
		req = req.WithContext(context.WithValue(req.Context(), appmiddleware.ContextUserIDKey, "user-1"))
		rec := httptest.NewRecorder()

		server.handleNotificationsStream()(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("fails before headers when writer cannot flush", func(t *testing.T) {
		called := false
		svc := notificationStreamServiceStub{
			openFn: func(context.Context, string) (application.NotificationStreamSession, error) {
				called = true
				return &notificationStreamSessionStub{initialUnreadCount: 1, events: make(chan application.NotificationStreamEvent)}, nil
			},
		}
		server := NewServer(logger, application.AuthService{}, application.WorkspaceService{}, application.FolderService{}, application.PageService{}, application.RevisionService{}, appauth.TokenManager{}, nil).
			WithNotificationStreamService(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/stream", nil)
		req = req.WithContext(context.WithValue(req.Context(), appmiddleware.ContextUserIDKey, "user-1"))
		rec := &noFlushResponseWriter{}

		server.handleNotificationsStream()(rec, req)
		if called {
			t.Fatal("expected flush check to fail before opening the stream")
		}
		if rec.code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d body=%s", rec.code, rec.body.String())
		}
	})
}
