package postgres

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"note-app/internal/infrastructure/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultTestDSN = "postgres://noteapp:noteapp@localhost:5432/noteapp?sslmode=disable"

var (
	testPoolOnce sync.Once
	testPool     *pgxpool.Pool
	testPoolErr  error
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	testPoolOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		dsn := os.Getenv("POSTGRES_DSN")
		if dsn == "" {
			dsn = defaultTestDSN
		}

		_, thisFile, _, _ := runtime.Caller(0)
		projectRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
		testPoolErr = os.Chdir(projectRoot)
		if testPoolErr != nil {
			return
		}

		testPoolErr = database.RunMigrations(dsn, "migrations", "up", 0)
		if testPoolErr != nil {
			return
		}

		testPool, testPoolErr = database.NewPool(ctx, dsn)
	})

	if testPoolErr != nil {
		t.Fatalf("integration db unavailable: %v", testPoolErr)
	}

	t.Cleanup(func() {
		resetTestDatabase(t, testPool)
	})

	return testPool
}

func resetTestDatabase(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		TRUNCATE TABLE
			notifications,
			page_comments,
			revisions,
			trash_items,
			page_drafts,
			pages,
			folders,
			workspace_invitations,
			workspace_members,
			workspaces,
			refresh_tokens,
			users
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("reset test database: %v", err)
	}
}
