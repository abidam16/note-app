package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testDSN() string {
	if dsn := os.Getenv("POSTGRES_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://noteapp:noteapp@localhost:5432/noteapp?sslmode=disable"
}

func projectRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	return filepath.Clean(filepath.Join(cwd, "..", "..", ".."))
}

func TestNewPool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := NewPool(ctx, "::bad-dsn"); err == nil {
		t.Fatal("expected parse config error")
	}

	pool, err := NewPool(ctx, testDSN())
	if err != nil {
		t.Fatalf("new pool should connect with local dsn: %v", err)
	}
	pool.Close()

	canceledCtx, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if _, err := NewPool(canceledCtx, testDSN()); err == nil {
		t.Fatal("expected ping failure with canceled context")
	}
}

func TestRunMigrations(t *testing.T) {
	root := projectRoot(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir project root: %v", err)
	}

	dsn := testDSN()
	if err := RunMigrations(dsn, "migrations", "up", 0); err != nil {
		t.Fatalf("up migrations should succeed: %v", err)
	}

	if err := RunMigrations(dsn, "migrations", "up", 0); err != nil {
		t.Fatalf("up no-change should still succeed: %v", err)
	}

	if err := RunMigrations(dsn, "migrations", "sideways", 0); err == nil || !strings.Contains(err.Error(), "unsupported migration direction") {
		t.Fatalf("expected unsupported direction error, got %v", err)
	}

	if err := RunMigrations(dsn, "missing-migrations", "up", 0); err == nil {
		t.Fatal("expected create migrator error for missing path")
	}
}

func TestRunMigrationsDownPaths(t *testing.T) {
	root := projectRoot(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir project root: %v", err)
	}

	dsn := testDSN()

	if err := RunMigrations(dsn, "migrations", "up", 0); err != nil {
		t.Fatalf("setup up migrations should succeed: %v", err)
	}

	if err := RunMigrations(dsn, "migrations", "down", 1); err != nil {
		t.Fatalf("down migrations with steps should succeed: %v", err)
	}

	if err := RunMigrations(dsn, "migrations", "up", 0); err != nil {
		t.Fatalf("up migrations after stepped down should succeed: %v", err)
	}

	if err := RunMigrations(dsn, "migrations", "down", 0); err != nil {
		t.Fatalf("down migrations all should succeed: %v", err)
	}

	if err := RunMigrations(dsn, "migrations", "down", 0); err != nil {
		t.Fatalf("down no-change should still succeed: %v", err)
	}

	if err := RunMigrations(dsn, "migrations", "up", 0); err != nil {
		t.Fatalf("restore up migrations should succeed: %v", err)
	}
}

func TestRunMigrationsUpDirtyDatabaseError(t *testing.T) {
	root := projectRoot(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir project root: %v", err)
	}

	dsn := testDSN()
	if err := RunMigrations(dsn, "migrations", "up", 0); err != nil {
		t.Fatalf("setup up migrations should succeed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `UPDATE schema_migrations SET dirty = TRUE`); err != nil {
		t.Fatalf("mark schema migrations dirty: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE schema_migrations SET dirty = FALSE`)
	}()

	if err := RunMigrations(dsn, "migrations", "up", 0); err == nil || !strings.Contains(err.Error(), "apply up migrations") {
		t.Fatalf("expected up apply error on dirty database, got %v", err)
	}
}
