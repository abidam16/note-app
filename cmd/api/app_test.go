package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"note-app/internal/infrastructure/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeDB struct {
	closed bool
}

func (d *fakeDB) Close() {
	d.closed = true
}

type fakeServer struct {
	addr       string
	listenFn   func() error
	shutdownFn func(context.Context) error
}

func (s *fakeServer) ListenAndServe() error {
	if s.listenFn != nil {
		return s.listenFn()
	}
	return nil
}

func (s *fakeServer) Shutdown(ctx context.Context) error {
	if s.shutdownFn != nil {
		return s.shutdownFn(ctx)
	}
	return nil
}

func (s *fakeServer) Address() string {
	return s.addr
}

func testConfig() config.Config {
	return config.Config{
		AppEnv:           "test",
		HTTPPort:         "9090",
		PostgresDSN:      "postgres://example",
		JWTIssuer:        "note-app",
		JWTSecret:        "1234567890abcdef",
		AccessTokenTTL:   time.Minute,
		RefreshTokenTTL:  time.Hour,
		LocalStoragePath: "./tmp/storage",
	}
}

func TestRunReturnsPoolError(t *testing.T) {
	poolErr := errors.New("pool failed")
	cfg := testConfig()

	deps := runtimeDeps{
		newPool: func(context.Context, string) (closable, error) {
			return nil, poolErr
		},
		buildServer: func(config.Config, *slog.Logger, closable) appServer {
			t.Fatal("buildServer should not be called when pool creation fails")
			return nil
		},
		newLogger:    func(config.Config) *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) },
		makeStopChan: func() chan os.Signal { return make(chan os.Signal, 1) },
		notifySignal: func(chan<- os.Signal, ...os.Signal) {},
	}

	if err := run(cfg, deps); !errors.Is(err, poolErr) {
		t.Fatalf("expected pool error, got %v", err)
	}
}

func TestRunReturnsListenError(t *testing.T) {
	listenErr := errors.New("listen failed")
	cfg := testConfig()
	db := &fakeDB{}

	deps := runtimeDeps{
		newPool: func(context.Context, string) (closable, error) {
			return db, nil
		},
		buildServer: func(config.Config, *slog.Logger, closable) appServer {
			return &fakeServer{
				addr: ":9090",
				listenFn: func() error {
					return listenErr
				},
			}
		},
		newLogger:    func(config.Config) *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) },
		makeStopChan: func() chan os.Signal { return make(chan os.Signal, 1) },
		notifySignal: func(chan<- os.Signal, ...os.Signal) {},
	}

	if err := run(cfg, deps); !errors.Is(err, listenErr) {
		t.Fatalf("expected listen error, got %v", err)
	}
	if !db.closed {
		t.Fatal("expected db to be closed")
	}
}

func TestRunReturnsShutdownError(t *testing.T) {
	shutdownErr := errors.New("shutdown failed")
	cfg := testConfig()
	db := &fakeDB{}

	deps := runtimeDeps{
		newPool: func(context.Context, string) (closable, error) {
			return db, nil
		},
		buildServer: func(config.Config, *slog.Logger, closable) appServer {
			return &fakeServer{
				addr: ":9090",
				listenFn: func() error {
					return http.ErrServerClosed
				},
				shutdownFn: func(context.Context) error {
					return shutdownErr
				},
			}
		},
		newLogger:    func(config.Config) *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) },
		makeStopChan: func() chan os.Signal { return make(chan os.Signal, 1) },
		notifySignal: func(c chan<- os.Signal, _ ...os.Signal) {
			c <- os.Interrupt
		},
	}

	if err := run(cfg, deps); !errors.Is(err, shutdownErr) {
		t.Fatalf("expected shutdown error, got %v", err)
	}
	if !db.closed {
		t.Fatal("expected db to be closed")
	}
}

func TestRunSuccess(t *testing.T) {
	cfg := testConfig()
	db := &fakeDB{}

	deps := runtimeDeps{
		newPool: func(context.Context, string) (closable, error) {
			return db, nil
		},
		buildServer: func(config.Config, *slog.Logger, closable) appServer {
			return &fakeServer{
				addr: ":9090",
				listenFn: func() error {
					return http.ErrServerClosed
				},
				shutdownFn: func(context.Context) error {
					return nil
				},
			}
		},
		newLogger:    func(config.Config) *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) },
		makeStopChan: func() chan os.Signal { return make(chan os.Signal, 1) },
		notifySignal: func(c chan<- os.Signal, _ ...os.Signal) {
			c <- os.Interrupt
		},
	}

	if err := run(cfg, deps); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !db.closed {
		t.Fatal("expected db to be closed")
	}
}

func TestRunReturnsBuildServerPanicAsError(t *testing.T) {
	cfg := testConfig()
	db := &fakeDB{}

	deps := runtimeDeps{
		newPool: func(context.Context, string) (closable, error) {
			return db, nil
		},
		buildServer: func(config.Config, *slog.Logger, closable) appServer {
			panic("builder exploded")
		},
		newLogger:    func(config.Config) *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) },
		makeStopChan: func() chan os.Signal { return make(chan os.Signal, 1) },
		notifySignal: func(chan<- os.Signal, ...os.Signal) {},
	}

	err := run(cfg, deps)
	if err == nil {
		t.Fatal("expected startup builder panic to become an error")
	}
	if !strings.Contains(err.Error(), "build server") {
		t.Fatalf("expected build server context, got %v", err)
	}
	if !db.closed {
		t.Fatal("expected db to be closed")
	}
}

func TestBuildDefaultServer(t *testing.T) {
	cfg := testConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var db closable = (*pgxpool.Pool)(nil)

	server := buildDefaultServer(cfg, logger, db)
	if got := server.Address(); got != ":"+cfg.HTTPPort {
		t.Fatalf("unexpected server address: %s", got)
	}
}

func TestBuildDefaultServerAppliesHTTPHardening(t *testing.T) {
	cfg := testConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var db closable = (*pgxpool.Pool)(nil)

	server := buildDefaultServer(cfg, logger, db)
	adapter, ok := server.(httpServerAdapter)
	if !ok {
		t.Fatalf("expected httpServerAdapter, got %T", server)
	}

	if got := adapter.server.ReadHeaderTimeout; got != 10*time.Second {
		t.Fatalf("unexpected read header timeout: %s", got)
	}
	if got := adapter.server.ReadTimeout; got != 15*time.Second {
		t.Fatalf("unexpected read timeout: %s", got)
	}
	if got := adapter.server.WriteTimeout; got != 0 {
		t.Fatalf("unexpected write timeout: %s", got)
	}
	if got := adapter.server.IdleTimeout; got != 60*time.Second {
		t.Fatalf("unexpected idle timeout: %s", got)
	}
	if got := adapter.server.MaxHeaderBytes; got != 1<<20 {
		t.Fatalf("unexpected max header bytes: %d", got)
	}
}

func TestBuildDefaultServerPanicsOnInvalidDBType(t *testing.T) {
	cfg := testConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for invalid database type")
		}
	}()

	_ = buildDefaultServer(cfg, logger, &fakeDB{})
}

func TestDefaultRuntimeDeps(t *testing.T) {
	deps := defaultRuntimeDeps()
	if deps.newPool == nil || deps.buildServer == nil || deps.newLogger == nil || deps.makeStopChan == nil || deps.notifySignal == nil {
		t.Fatal("expected default runtime deps to be initialized")
	}

	if _, err := deps.newPool(context.Background(), "::bad-dsn"); err == nil {
		t.Fatal("expected parse error from default newPool with bad dsn")
	}

	if got := cap(deps.makeStopChan()); got != 1 {
		t.Fatalf("unexpected stop channel capacity: %d", got)
	}
}

func TestRunSanitizesPoolErrorLog(t *testing.T) {
	cfg := testConfig()
	secret := "postgres://user:super-secret@db.internal:5432/note"
	poolErr := errors.New("connect failed: " + secret)
	var logs bytes.Buffer

	deps := runtimeDeps{
		newPool: func(context.Context, string) (closable, error) {
			return nil, poolErr
		},
		buildServer: func(config.Config, *slog.Logger, closable) appServer {
			t.Fatal("buildServer should not be called when pool creation fails")
			return nil
		},
		newLogger: func(config.Config) *slog.Logger {
			return slog.New(slog.NewJSONHandler(&logs, nil))
		},
		makeStopChan: func() chan os.Signal { return make(chan os.Signal, 1) },
		notifySignal: func(chan<- os.Signal, ...os.Signal) {},
	}

	err := run(cfg, deps)
	if !errors.Is(err, poolErr) {
		t.Fatalf("expected pool error, got %v", err)
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatalf("expected pool log to redact secret, got %s", logs.String())
	}
	if !strings.Contains(logs.String(), `"msg":"database connection failed"`) {
		t.Fatalf("expected database failure log message, got %s", logs.String())
	}
}

func TestRunSanitizesListenErrorLog(t *testing.T) {
	cfg := testConfig()
	db := &fakeDB{}
	secret := "postgres://user:super-secret@db.internal:5432/note"
	listenErr := errors.New("listen failed after using " + secret)
	var logs bytes.Buffer

	deps := runtimeDeps{
		newPool: func(context.Context, string) (closable, error) {
			return db, nil
		},
		buildServer: func(config.Config, *slog.Logger, closable) appServer {
			return &fakeServer{
				addr: ":9090",
				listenFn: func() error {
					return listenErr
				},
			}
		},
		newLogger: func(config.Config) *slog.Logger {
			return slog.New(slog.NewJSONHandler(&logs, nil))
		},
		makeStopChan: func() chan os.Signal { return make(chan os.Signal, 1) },
		notifySignal: func(chan<- os.Signal, ...os.Signal) {},
	}

	err := run(cfg, deps)
	if !errors.Is(err, listenErr) {
		t.Fatalf("expected listen error, got %v", err)
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatalf("expected listen log to redact secret, got %s", logs.String())
	}
	if !strings.Contains(logs.String(), `"msg":"server failed"`) {
		t.Fatalf("expected server failure log message, got %s", logs.String())
	}
}

func TestHTTPServerAdapterLifecycle(t *testing.T) {
	server := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: http.NewServeMux(),
	}
	adapter := httpServerAdapter{server: server}

	errCh := make(chan error, 1)
	go func() {
		errCh <- adapter.ListenAndServe()
	}()

	time.Sleep(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := adapter.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown error = %v", err)
	}

	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("expected ErrServerClosed, got %v", err)
	}
}
