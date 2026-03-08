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
	logger              *slog.Logger
	authService         application.AuthService
	workspaceService    application.WorkspaceService
	folderService       application.FolderService
	pageService         application.PageService
	revisionService     application.RevisionService
	commentService      application.CommentService
	searchService       application.SearchService
	notificationService application.NotificationService
	tokenManager        appauth.TokenManager
	storage             storage.FileStorage
}

func NewServer(logger *slog.Logger, authService application.AuthService, workspaceService application.WorkspaceService, folderService application.FolderService, pageService application.PageService, revisionService application.RevisionService, tokenManager appauth.TokenManager, fileStorage storage.FileStorage) Server {
	return Server{
		logger:              logger,
		authService:         authService,
		workspaceService:    workspaceService,
		folderService:       folderService,
		pageService:         pageService,
		revisionService:     revisionService,
		commentService:      application.CommentService{},
		searchService:       application.SearchService{},
		notificationService: application.NotificationService{},
		tokenManager:        tokenManager,
		storage:             fileStorage,
	}
}

func (s Server) WithCommentService(commentService application.CommentService) Server {
	s.commentService = commentService
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

func (s Server) Handler() nethttp.Handler {
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(chimiddleware.RealIP)
	router.Use(chimiddleware.Recoverer)
	router.Use(chimiddleware.Timeout(30 * time.Second))
	router.Use(appmiddleware.Logger(s.logger))

	router.Get("/healthz", s.handleHealth())

	router.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", s.handleRegister())
			r.Post("/login", s.handleLogin())
			r.Post("/refresh", s.handleRefresh())
			r.Post("/logout", s.handleLogout())

			r.Group(func(r chi.Router) {
				r.Use(appmiddleware.Authenticate(s.tokenManager))
				r.Get("/me", s.handleCurrentUser())
			})
		})

		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.Authenticate(s.tokenManager))
			r.Post("/workspaces", s.handleCreateWorkspace())
			r.Post("/workspaces/{workspaceID}/invitations", s.handleInviteMember())
			r.Get("/workspaces/{workspaceID}/members", s.handleListMembers())
			r.Patch("/workspaces/{workspaceID}/members/{memberID}/role", s.handleUpdateMemberRole())
			r.Post("/workspace-invitations/{invitationID}/accept", s.handleAcceptInvitation())
			r.Post("/workspaces/{workspaceID}/folders", s.handleCreateFolder())
			r.Get("/workspaces/{workspaceID}/folders", s.handleListFolders())
			r.Get("/workspaces/{workspaceID}/search", s.handleSearchPages())
			r.Get("/workspaces/{workspaceID}/trash", s.handleListTrash())
			r.Post("/workspaces/{workspaceID}/pages", s.handleCreatePage())
			r.Get("/pages/{pageID}", s.handleGetPage())
			r.Patch("/pages/{pageID}", s.handleUpdatePage())
			r.Delete("/pages/{pageID}", s.handleDeletePage())
			r.Put("/pages/{pageID}/draft", s.handleUpdateDraft())
			r.Get("/pages/{pageID}/revisions", s.handleListRevisions())
			r.Post("/pages/{pageID}/revisions", s.handleCreateRevision())
			r.Get("/pages/{pageID}/revisions/compare", s.handleCompareRevisions())
			r.Post("/pages/{pageID}/revisions/{revisionID}/restore", s.handleRestoreRevision())
			r.Post("/pages/{pageID}/comments", s.handleCreateComment())
			r.Get("/pages/{pageID}/comments", s.handleListComments())
			r.Post("/comments/{commentID}/resolve", s.handleResolveComment())
			r.Get("/notifications", s.handleListNotifications())
			r.Post("/notifications/{notificationID}/read", s.handleMarkNotificationRead())
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
		return nethttp.StatusUnprocessableEntity, NewAPIError("validation_failed", err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials):
		return nethttp.StatusUnauthorized, NewAPIError("invalid_credentials", "invalid email or password")
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, domain.ErrTokenExpired):
		return nethttp.StatusUnauthorized, NewAPIError("unauthorized", err.Error())
	case errors.Is(err, domain.ErrForbidden):
		return nethttp.StatusForbidden, NewAPIError("forbidden", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return nethttp.StatusNotFound, NewAPIError("not_found", err.Error())
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrEmailAlreadyUsed), errors.Is(err, domain.ErrLastOwnerRemoval), errors.Is(err, domain.ErrInvitationEmailMismatch):
		return nethttp.StatusConflict, NewAPIError("conflict", err.Error())
	default:
		return nethttp.StatusInternalServerError, NewAPIError("internal_error", "internal server error")
	}
}
