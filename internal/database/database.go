// Package database opens the Postgres pool and applies embedded migrations.
//
// Migrations are plain .sql files applied in filename order inside a
// transaction, each recorded in schema_migrations. No external tool to
// install, and nothing to run by hand on a deploy.
package database

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed all:migrations
var migrationFS embed.FS

type DB struct {
	*pgxpool.Pool
}

// Connect opens the pool and verifies it before returning, so a bad
// DATABASE_URL surfaces at boot rather than on the first request.
func Connect(ctx context.Context, url string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	// Sized for a serverless Postgres free tier, which caps connections hard.
	cfg.MaxConns = 8
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = 2 * time.Minute
	cfg.MaxConnLifetime = 30 * time.Minute

	// Never cache prepared statements.
	//
	// Production runs through a connection pooler in transaction mode — that is
	// the whole point of the pooler on a free tier, and both Supabase and Neon
	// hand you that URL by default. In transaction mode a client does not keep
	// the same backend between statements, so a cached prepared statement
	// resolves against a backend that never parsed it, and the pool fails with
	// "prepared statement ... already exists" (SQLSTATE 42P05) the moment a
	// connection is reused.
	//
	// QueryExecModeExec: one round trip, nothing held between statements.
	//
	// This is the only mode that survives a connection pooler in transaction
	// mode, which is what production runs on — both Supabase and Neon hand you
	// that URL by default on a free tier.
	//
	// The two modes that look reasonable are not:
	//
	//   CacheStatement caches named prepared statements, so the pool fails with
	//   "prepared statement already exists" (42P05) the moment a connection is
	//   reused.
	//
	//   DescribeExec sends Parse+Describe, then Bind+Execute — two round trips,
	//   which the pooler sees as two transactions. Between them it can hand the
	//   connection to another client, and the unnamed statement is gone. That
	//   fails only under concurrency, so it passes every sequential test and
	//   then 500s as soon as a real page loads three requests at once.
	//
	// Exec sends everything in one round trip. The cost is that pgx must infer
	// parameter types itself rather than asking the server, so any parameter
	// whose type is not obvious from context carries an explicit ::cast in the
	// SQL. That is why those casts exist; do not remove them.
	//
	// An explicit default_query_exec_mode in the connection string wins, since
	// ParseConfig has already applied it by this point.
	if !strings.Contains(url, "default_query_exec_mode") {
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	}
	cfg.ConnConfig.StatementCacheCapacity = 0
	cfg.ConnConfig.DescriptionCacheCapacity = 0

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{Pool: pool}, nil
}

// Migrate applies every migration not yet recorded, in filename order.
func (db *DB) Migrate(ctx context.Context) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[string]bool{}
	rows, err := db.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		if applied[name] {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}

		tx, err := db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}
		slog.Info("migration applied", "version", name)
	}
	return nil
}
