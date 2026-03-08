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
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel()}))

	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error("database connection failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer db.Close()

	tokenManager := appauth.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL)
	passwordManager := appauth.NewPasswordManager()
	fileStorage := storage.NewLocal(cfg.LocalStoragePath)

	userRepo := postgresrepo.NewUserRepository(db)
	refreshTokenRepo := postgresrepo.NewRefreshTokenRepository(db)
	workspaceRepo := postgresrepo.NewWorkspaceRepository(db)
	folderRepo := postgresrepo.NewFolderRepository(db)
	pageRepo := postgresrepo.NewPageRepository(db)
	revisionRepo := postgresrepo.NewRevisionRepository(db)
	commentRepo := postgresrepo.NewCommentRepository(db)

	authService := application.NewAuthService(userRepo, refreshTokenRepo, passwordManager, tokenManager, cfg.RefreshTokenTTL)
	workspaceService := application.NewWorkspaceService(workspaceRepo, userRepo)
	folderService := application.NewFolderService(folderRepo, workspaceRepo)
	pageService := application.NewPageService(pageRepo, workspaceRepo, folderRepo)
	revisionService := application.NewRevisionService(revisionRepo, pageRepo, workspaceRepo)
	commentService := application.NewCommentService(commentRepo, pageRepo, workspaceRepo)
	searchService := application.NewSearchService(pageRepo, workspaceRepo)

	server := transporthttp.NewServer(logger, authService, workspaceService, folderService, pageService, revisionService, tokenManager, fileStorage).
		WithCommentService(commentService).
		WithSearchService(searchService)

	httpServer := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("server starting", slog.String("addr", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("server stopped")
}
