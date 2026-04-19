package http

import (
	"context"
	"errors"
	"log/slog"
	nethttp "net/http"
	"strings"
	"time"

	"note-app/internal/application"
	"note-app/internal/domain"
	appauth "note-app/internal/infrastructure/auth"
	"note-app/internal/infrastructure/storage"
	appmiddleware "note-app/internal/transport/http/middleware"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	logger                    *slog.Logger
	authService               application.AuthService
	workspaceService          application.WorkspaceService
	folderService             application.FolderService
	pageService               application.PageService
	revisionService           application.RevisionService
	commentService            application.CommentService
	threadService             application.ThreadService
	searchService             application.SearchService
	notificationService       application.NotificationService
	notificationStreamService interface {
		Open(context.Context, string) (application.NotificationStreamSession, error)
	}
	tokenManager   appauth.TokenManager
	storage        storage.FileStorage
	clientIPConfig appmiddleware.ClientIPConfig
	corsConfig     appmiddleware.CORSConfig
}

const heavyRouteMaxConcurrent = 4

func NewServer(logger *slog.Logger, authService application.AuthService, workspaceService application.WorkspaceService, folderService application.FolderService, pageService application.PageService, revisionService application.RevisionService, tokenManager appauth.TokenManager, fileStorage storage.FileStorage) Server {
	return Server{
		logger:                    logger,
		authService:               authService,
		workspaceService:          workspaceService,
		folderService:             folderService,
		pageService:               pageService,
		revisionService:           revisionService,
		commentService:            application.CommentService{},
		threadService:             application.ThreadService{},
		searchService:             application.SearchService{},
		notificationService:       application.NotificationService{},
		notificationStreamService: nil,
		tokenManager:              tokenManager,
		storage:                   fileStorage,
	}
}

func (s Server) WithCommentService(commentService application.CommentService) Server {
	s.commentService = commentService
	return s
}

func (s Server) WithThreadService(threadService application.ThreadService) Server {
	s.threadService = threadService
	return s
}

func (s Server) WithSearchService(searchService application.SearchService) Server {
	s.searchService = searchService
	return s
}

func (s Server) WithNotificationService(notificationService application.NotificationService) Server {
	s.notificationService = notificationService
	return s
}

func (s Server) WithNotificationStreamService(notificationStreamService interface {
	Open(context.Context, string) (application.NotificationStreamSession, error)
}) Server {
	s.notificationStreamService = notificationStreamService
	return s
}

func (s Server) WithClientIPConfig(clientIPConfig appmiddleware.ClientIPConfig) Server {
	s.clientIPConfig = clientIPConfig
	return s
}

func (s Server) WithCORSConfig(corsConfig appmiddleware.CORSConfig) Server {
	s.corsConfig = corsConfig
	return s
}

func (s Server) Handler() nethttp.Handler {
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(chimiddleware.Recoverer)
	router.Use(appmiddleware.SecurityHeaders())
	router.Use(appmiddleware.ResolveClientIP(s.clientIPConfig))
	router.Use(appmiddleware.Logger(s.logger))

	router.Get("/healthz", s.handleHealth())

	router.Route("/api/v1", func(r chi.Router) {
		heavyRouteLimiter := appmiddleware.ConcurrencyLimit(appmiddleware.ConcurrencyLimitConfig{
			MaxConcurrent: heavyRouteMaxConcurrent,
		})
		r.Use(appmiddleware.RateLimit(appmiddleware.RateLimitConfig{
			Window:      time.Minute,
			MaxRequests: 120,
		}))
		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.Authenticate(s.tokenManager))
			r.Get("/notifications/stream", s.handleNotificationsStream())
		})
		r.Route("/auth", func(r chi.Router) {
			r.Use(appmiddleware.CORS(s.corsConfig))
			r.Use(chimiddleware.Timeout(30 * time.Second))
			r.Use(appmiddleware.NoStore())
			r.With(appmiddleware.RateLimit(appmiddleware.RateLimitConfig{
				Window:      time.Minute,
				MaxRequests: 5,
			})).Post("/login", s.handleLogin())
			r.With(appmiddleware.RateLimit(appmiddleware.RateLimitConfig{
				Window:      time.Minute,
				MaxRequests: 5,
			})).Post("/refresh", s.handleRefresh())
			r.Post("/register", s.handleRegister())
			r.Post("/logout", s.handleLogout())

			r.Group(func(r chi.Router) {
				r.Use(appmiddleware.Authenticate(s.tokenManager))
				r.Get("/me", s.handleCurrentUser())
			})
		})

		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.Authenticate(s.tokenManager))
			r.Use(chimiddleware.Timeout(30 * time.Second))
			r.Get("/workspaces", s.handleListWorkspaces())
			r.Post("/workspaces", s.handleCreateWorkspace())
			r.Patch("/workspaces/{workspaceID}", s.handleRenameWorkspace())
			r.Post("/workspaces/{workspaceID}/invitations", s.handleInviteMember())
			r.Get("/workspaces/{workspaceID}/invitations", s.handleListWorkspaceInvitations())
			r.Get("/my/invitations", s.handleListMyInvitations())
			r.Get("/workspaces/{workspaceID}/members", s.handleListMembers())
			r.Patch("/workspaces/{workspaceID}/members/{memberID}/role", s.handleUpdateMemberRole())
			r.Post("/workspace-invitations/{invitationID}/accept", s.handleAcceptInvitation())
			r.Post("/workspace-invitations/{invitationID}/cancel", s.handleCancelInvitation())
			r.Post("/workspace-invitations/{invitationID}/reject", s.handleRejectInvitation())
			r.Patch("/workspace-invitations/{invitationID}", s.handleUpdateInvitation())
			r.Post("/workspaces/{workspaceID}/folders", s.handleCreateFolder())
			r.Get("/workspaces/{workspaceID}/folders", s.handleListFolders())
			r.Patch("/folders/{folderID}", s.handleRenameFolder())
			r.Get("/workspaces/{workspaceID}/search", s.handleSearchPages())
			r.Get("/workspaces/{workspaceID}/threads", s.handleListWorkspaceThreads())
			r.Get("/workspaces/{workspaceID}/trash", s.handleListTrash())
			r.Get("/workspaces/{workspaceID}/pages", s.handleListPages())
			r.Post("/workspaces/{workspaceID}/pages", s.handleCreatePage())
			r.Get("/pages/{pageID}", s.handleGetPage())
			r.Patch("/pages/{pageID}", s.handleUpdatePage())
			r.Delete("/pages/{pageID}", s.handleDeletePage())
			r.With(heavyRouteLimiter).Put("/pages/{pageID}/draft", s.handleUpdateDraft())
			r.Get("/pages/{pageID}/revisions", s.handleListRevisions())
			r.With(heavyRouteLimiter).Post("/pages/{pageID}/revisions", s.handleCreateRevision())
			r.With(heavyRouteLimiter).Get("/pages/{pageID}/revisions/compare", s.handleCompareRevisions())
			r.With(heavyRouteLimiter).Post("/pages/{pageID}/revisions/{revisionID}/restore", s.handleRestoreRevision())
			r.Post("/pages/{pageID}/comments", s.handleCreateComment())
			r.Get("/pages/{pageID}/comments", s.handleListComments())
			r.Post("/comments/{commentID}/resolve", s.handleResolveComment())
			r.Post("/pages/{pageID}/threads", s.handleCreateThread())
			r.Get("/pages/{pageID}/threads", s.handleListThreads())
			r.Get("/threads/{threadID}", s.handleGetThread())
			r.Get("/threads/{threadID}/notification-preference", s.handleGetThreadNotificationPreference())
			r.Put("/threads/{threadID}/notification-preference", s.handleUpdateThreadNotificationPreference())
			r.Post("/threads/{threadID}/replies", s.handleCreateThreadReply())
			r.Post("/threads/{threadID}/resolve", s.handleResolveThread())
			r.Post("/threads/{threadID}/reopen", s.handleReopenThread())
			r.Get("/notifications", s.handleListNotifications())
			r.Get("/notifications/unread-count", s.handleGetNotificationUnreadCount())
			r.Post("/notifications/read", s.handleBatchMarkNotificationsRead())
			r.Post("/notifications/{notificationID}/read", s.handleMarkNotificationRead())
			r.Get("/trash/{trashItemID}", s.handleGetTrashPage())
			r.Post("/trash/{trashItemID}/restore", s.handleRestoreTrashItem())
		})
	})

	return router
}

func (s Server) handleHealth() nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		WriteJSON(w, nethttp.StatusOK, map[string]any{"status": "ok"})
	}
}

func requestContextUserID(ctx context.Context) string {
	userID, _ := ctx.Value(appmiddleware.ContextUserIDKey).(string)
	return strings.TrimSpace(userID)
}

func mapError(err error) (int, APIError) {
	switch {
	case err == nil:
		return nethttp.StatusOK, APIError{}
	case errors.Is(err, domain.ErrValidation):
		return nethttp.StatusUnprocessableEntity, NewAPIError("validation_failed", normalizeValidationMessage(err))
	case errors.Is(err, domain.ErrInvalidCredentials):
		return nethttp.StatusUnauthorized, NewAPIError("invalid_credentials", "invalid email or password")
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, domain.ErrTokenExpired):
		return nethttp.StatusUnauthorized, NewAPIError("unauthorized", err.Error())
	case errors.Is(err, domain.ErrForbidden):
		return nethttp.StatusForbidden, NewAPIError("forbidden", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return nethttp.StatusNotFound, NewAPIError("not_found", err.Error())
	case errors.Is(err, domain.ErrInvitationSelfEmail):
		return nethttp.StatusConflict, NewAPIError("invitation_self_email", err.Error())
	case errors.Is(err, domain.ErrInvitationUnregistered):
		return nethttp.StatusConflict, NewAPIError("invitation_target_unregistered", err.Error())
	case errors.Is(err, domain.ErrInvitationExistingMember):
		return nethttp.StatusConflict, NewAPIError("invitation_existing_member", err.Error())
	case errors.Is(err, domain.ErrInvitationDuplicatePending):
		return nethttp.StatusConflict, NewAPIError("invitation_duplicate_pending", err.Error())
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrEmailAlreadyUsed), errors.Is(err, domain.ErrLastOwnerRemoval), errors.Is(err, domain.ErrInvitationEmailMismatch):
		return nethttp.StatusConflict, NewAPIError("conflict", err.Error())
	default:
		return nethttp.StatusInternalServerError, NewAPIError("internal_error", "internal server error")
	}
}

func (s Server) writeMappedError(w nethttp.ResponseWriter, r *nethttp.Request, err error) {
	status, apiErr := mapError(err)

	routePattern := ""
	if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil {
		routePattern = routeCtx.RoutePattern()
	}

	attrs := []any{
		slog.String("request_id", chimiddleware.GetReqID(r.Context())),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("route", routePattern),
		slog.String("query", appmiddleware.SanitizeLogQuery(r.URL.RawQuery)),
		slog.String("client_ip", appmiddleware.RequestClientIP(r)),
		slog.String("remote_addr", r.RemoteAddr),
		slog.String("user_id", requestContextUserID(r.Context())),
		slog.Int("status", status),
		slog.String("error_code", apiErr.Code),
		slog.String("failure_class", failureClass(status)),
	}
	if status >= nethttp.StatusInternalServerError {
		s.logger.Error("request failed", attrs...)
	} else {
		s.logger.Warn("request failed", attrs...)
	}

	WriteError(w, r, status, apiErr)
}

func failureClass(status int) string {
	if status >= nethttp.StatusInternalServerError {
		return "server"
	}
	return "client"
}

func normalizeValidationMessage(err error) string {
	if err == nil {
		return "validation failed"
	}
	message := strings.TrimSpace(err.Error())
	prefix := strings.TrimSpace(domain.ErrValidation.Error()) + ":"
	if strings.HasPrefix(strings.ToLower(message), strings.ToLower(prefix)) {
		message = strings.TrimSpace(message[len(prefix):])
	}
	if message == "" {
		return "validation failed"
	}
	return message
}
