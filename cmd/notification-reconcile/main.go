package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"note-app/internal/application"
	"note-app/internal/infrastructure/config"
	appdb "note-app/internal/infrastructure/database"
	postgresrepo "note-app/internal/repository/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
)

type reconcileInput struct {
	EnvFile     string
	WorkspaceID string
	DryRun      bool
	BatchSize   int
}

type runtimeDeps struct {
	newPool      func(ctx context.Context, dsn string) (closable, error)
	newLogger    func(cfg config.Config) *slog.Logger
	buildService func(pool *pgxpool.Pool, logger *slog.Logger) application.NotificationReconciliationRunner
}

type closable interface {
	Close()
}

var (
	loadConfigFn            = config.LoadFromEnvFile
	runFn                   = run
	depsFactoryFn           = defaultRuntimeDeps
	exitFn                  = os.Exit
	stdoutWriter  io.Writer = os.Stdout
	stderrWriter  io.Writer = os.Stderr
)

func parseFlags(args []string) (reconcileInput, error) {
	fs := flag.NewFlagSet("notification-reconcile", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	input := reconcileInput{}
	fs.StringVar(&input.EnvFile, "env-file", ".env", "environment file to load")
	fs.StringVar(&input.WorkspaceID, "workspace-id", "", "workspace id to reconcile")
	fs.BoolVar(&input.DryRun, "dry-run", false, "compute changes without writing")
	fs.IntVar(&input.BatchSize, "batch-size", 500, "batch size to use when scanning source rows")

	if err := fs.Parse(args); err != nil {
		return reconcileInput{}, err
	}
	if input.BatchSize < 1 || input.BatchSize > 2000 {
		return reconcileInput{}, fmt.Errorf("batch-size must be between 1 and 2000")
	}
	return input, nil
}

func defaultRuntimeDeps() runtimeDeps {
	return runtimeDeps{
		newPool: func(ctx context.Context, dsn string) (closable, error) {
			return appdb.NewPool(ctx, dsn)
		},
		newLogger: func(cfg config.Config) *slog.Logger {
			return slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: cfg.LogLevel()}))
		},
		buildService: func(pool *pgxpool.Pool, logger *slog.Logger) application.NotificationReconciliationRunner {
			repo := postgresrepo.NewNotificationReconciliationRepository(pool)
			publisher := appdb.NewNotificationStreamBroker(pool)
			return application.NewNotificationReconciliationService(repo, publisher, timeNow)
		},
	}
}

var timeNow = func() time.Time { return time.Now().UTC() }

func run(cfg config.Config, deps runtimeDeps, input reconcileInput) (application.NotificationReconciliationSummary, error) {
	loggerFactory := deps.newLogger
	if loggerFactory == nil {
		loggerFactory = func(cfg config.Config) *slog.Logger {
			return slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: cfg.LogLevel()}))
		}
	}
	logger := loggerFactory(cfg)

	poolFactory := deps.newPool
	if poolFactory == nil {
		poolFactory = func(ctx context.Context, dsn string) (closable, error) {
			return appdb.NewPool(ctx, dsn)
		}
	}
	db, err := poolFactory(context.Background(), cfg.PostgresDSN)
	if err != nil {
		return application.NotificationReconciliationSummary{}, err
	}
	defer db.Close()

	pool, ok := db.(*pgxpool.Pool)
	if !ok {
		return application.NotificationReconciliationSummary{}, fmt.Errorf("notification reconciliation requires *pgxpool.Pool")
	}

	buildService := deps.buildService
	if buildService == nil {
		buildService = func(pool *pgxpool.Pool, logger *slog.Logger) application.NotificationReconciliationRunner {
			repo := postgresrepo.NewNotificationReconciliationRepository(pool)
			publisher := appdb.NewNotificationStreamBroker(pool)
			return application.NewNotificationReconciliationService(repo, publisher, timeNow)
		}
	}
	service := buildService(pool, logger)
	return service.Run(context.Background(), application.RunNotificationReconciliationInput{
		WorkspaceID: input.WorkspaceID,
		DryRun:      input.DryRun,
		BatchSize:   input.BatchSize,
	})
}

func main() {
	input, err := parseFlags(os.Args[1:])
	if err != nil {
		exitWithError(err)
		return
	}

	cfg, err := loadConfigFn(input.EnvFile)
	if err != nil {
		exitWithError(err)
		return
	}

	summary, err := runFn(cfg, depsFactoryFn(), input)
	if err != nil {
		exitWithError(err)
		return
	}

	if err := json.NewEncoder(stdoutWriter).Encode(summary); err != nil {
		exitWithError(err)
	}
}

func exitWithError(err error) {
	if err != nil {
		_, _ = fmt.Fprintln(stderrWriter, err.Error())
	}
	exitFn(1)
}
