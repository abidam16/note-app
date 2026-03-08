package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"note-app/internal/application"
	appauth "note-app/internal/infrastructure/auth"
	"note-app/internal/infrastructure/config"
	"note-app/internal/infrastructure/database"
	"note-app/internal/infrastructure/storage"
	postgresrepo "note-app/internal/repository/postgres"
	transporthttp "note-app/internal/transport/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type closable interface {
	Close()
}

type appServer interface {
	ListenAndServe() error
	Shutdown(ctx context.Context) error
	Address() string
}

type httpServerAdapter struct {
	server *http.Server
}

func (a httpServerAdapter) ListenAndServe() error {
	return a.server.ListenAndServe()
}

func (a httpServerAdapter) Shutdown(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}

func (a httpServerAdapter) Address() string {
	return a.server.Addr
}

type runtimeDeps struct {
	newPool      func(ctx context.Context, dsn string) (closable, error)
	buildServer  func(cfg config.Config, logger *slog.Logger, db closable) appServer
	makeStopChan func() chan os.Signal
	notifySignal func(c chan<- os.Signal, sig ...os.Signal)
}

func defaultRuntimeDeps() runtimeDeps {
	return runtimeDeps{
		newPool: func(ctx context.Context, dsn string) (closable, error) {
			return database.NewPool(ctx, dsn)
		},
		buildServer: buildDefaultServer,
		makeStopChan: func() chan os.Signal {
			return make(chan os.Signal, 1)
		},
		notifySignal: signal.Notify,
	}
}

func buildDefaultServer(cfg config.Config, logger *slog.Logger, db closable) appServer {
	pool, ok := db.(*pgxpool.Pool)
	if !ok {
		panic("invalid database pool type")
	}

	tokenManager := appauth.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL)
	passwordManager := appauth.NewPasswordManager()
	fileStorage := storage.NewLocal(cfg.LocalStoragePath)

	userRepo := postgresrepo.NewUserRepository(pool)
	refreshTokenRepo := postgresrepo.NewRefreshTokenRepository(pool)
	workspaceRepo := postgresrepo.NewWorkspaceRepository(pool)
	folderRepo := postgresrepo.NewFolderRepository(pool)
	pageRepo := postgresrepo.NewPageRepository(pool)
	revisionRepo := postgresrepo.NewRevisionRepository(pool)
	commentRepo := postgresrepo.NewCommentRepository(pool)
	notificationRepo := postgresrepo.NewNotificationRepository(pool)

	authService := application.NewAuthService(userRepo, refreshTokenRepo, passwordManager, tokenManager, cfg.RefreshTokenTTL)
	notificationService := application.NewNotificationService(notificationRepo, userRepo, workspaceRepo)
	workspaceService := application.NewWorkspaceService(workspaceRepo, userRepo, notificationService)
	folderService := application.NewFolderService(folderRepo, workspaceRepo)
	pageService := application.NewPageService(pageRepo, workspaceRepo, folderRepo)
	revisionService := application.NewRevisionService(revisionRepo, pageRepo, workspaceRepo)
	commentService := application.NewCommentService(commentRepo, pageRepo, workspaceRepo, notificationService)
	searchService := application.NewSearchService(pageRepo, workspaceRepo)

	server := transporthttp.NewServer(logger, authService, workspaceService, folderService, pageService, revisionService, tokenManager, fileStorage).
		WithCommentService(commentService).
		WithSearchService(searchService).
		WithNotificationService(notificationService)

	httpServer := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	return httpServerAdapter{server: httpServer}
}

func run(cfg config.Config, deps runtimeDeps) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel()}))

	db, err := deps.newPool(context.Background(), cfg.PostgresDSN)
	if err != nil {
		logger.Error("database connection failed", slog.Any("error", err))
		return err
	}
	defer db.Close()

	httpServer := deps.buildServer(cfg, logger, db)
	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("server starting", slog.String("addr", httpServer.Address()))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", slog.Any("error", err))
			serverErrors <- err
		}
	}()

	stop := deps.makeStopChan()
	deps.notifySignal(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		return err
	case <-stop:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", slog.Any("error", err))
		return err
	}

	logger.Info("server stopped")
	return nil
}
