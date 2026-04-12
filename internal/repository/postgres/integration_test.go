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
	"note-app/internal/testutil/testenv"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

		_, thisFile, _, _ := runtime.Caller(0)
		projectRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))

		dsn, err := testenv.ResolvePostgresDSN(projectRoot)
		if err != nil {
			testPoolErr = err
			return
		}

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
			outbox_events,
			notifications,
		page_comment_message_mentions,
		thread_notification_preferences,
		page_comment_messages,
			page_comment_threads,
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
