package database

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	applyPoolerSafeQueryMode(cfg, dsn)

	cfg.MaxConns = 10
	cfg.MinConns = 2
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}

func applyPoolerSafeQueryMode(cfg *pgxpool.Config, dsn string) {
	if cfg == nil {
		return
	}

	// PgBouncer/transaction poolers cannot safely reuse prepared statements across backend connections.
	// Switch to simple protocol in those environments to avoid 42P05 "prepared statement already exists".
	if isPoolerDSN(dsn) {
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}
}

func isPoolerDSN(dsn string) bool {
	u, err := url.Parse(dsn)
	if err != nil {
		return false
	}

	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, "pooler.") || strings.Contains(host, "pgbouncer") {
		return true
	}

	q := u.Query()
	if strings.EqualFold(strings.TrimSpace(q.Get("pgbouncer")), "true") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(q.Get("pooler")), "true") {
		return true
	}

	return false
}

func RunMigrations(dsn, migrationsPath, direction string, steps int) error {
	m, err := migrate.New("file://"+migrationsPath, dsn)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	switch direction {
	case "up":
		err = m.Up()
		if err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("apply up migrations: %w", err)
		}
	case "down":
		if steps > 0 {
			err = m.Steps(-steps)
		} else {
			err = m.Down()
		}
		if err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("apply down migrations: %w", err)
		}
	default:
		return fmt.Errorf("unsupported migration direction %q", direction)
	}

	return nil
}
