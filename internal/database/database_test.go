package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"os"
	"strings"
	"testing"
	"time"
)

func baseURL() string {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://plot:plot@postgres:5432/plotting?sslmode=disable"
}

func TestConnectRejectsAnUnparseableURL(t *testing.T) {
	if _, err := Connect(context.Background(), "://nonsense"); err == nil {
		t.Fatal("expected a parse error")
	}
}

// A bad DATABASE_URL must fail at boot, not on the first request that needs it.
func TestConnectFailsFastOnAnUnreachableHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := Connect(ctx, "postgres://nobody:nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=2")
	if err == nil {
		t.Fatal("Connect returned a pool for a host that is not listening")
	}
	if !strings.Contains(err.Error(), "ping") && !strings.Contains(err.Error(), "connect") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()

	admin, err := Connect(ctx, baseURL())
	if err != nil {
		t.Skipf("no Postgres reachable (%v); start it with `make up`", err)
	}
	const name = "plottest_migrate"
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create: %v", err)
	}
	admin.Close()

	url := strings.Replace(baseURL(), "/plotting?", "/"+name+"?", 1)
	db, err := Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("first migrate: %v", err)
	}

	var applied int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied == 0 {
		t.Fatal("no migrations were recorded")
	}

	// Running again must be a no-op rather than an error, because every boot
	// of every container calls this.
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var after int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != applied {
		t.Errorf("a second run applied %d extra migrations", after-applied)
	}

	// The schema the app actually needs must exist afterwards.
	for _, table := range []string{"builders", "societies", "plots", "queries", "fund_entries", "enquiries"} {
		var exists bool
		if err := db.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("table %q was not created", table)
		}
	}
}

func TestMigrateOnAClosedPool(t *testing.T) {
	ctx := context.Background()
	db, err := Connect(ctx, baseURL())
	if err != nil {
		t.Skipf("no Postgres reachable (%v)", err)
	}
	db.Close()

	if err := db.Migrate(ctx); err == nil {
		t.Fatal("Migrate should fail against a closed pool")
	}
}

// Production runs behind a connection pooler in transaction mode, where a
// cached prepared statement fails with SQLSTATE 42P05 as soon as the pool
// reuses a connection. This only shows up against a real pooler, so the
// configuration is asserted here rather than discovered on a deploy.
func TestConnectDisablesPreparedStatementCaching(t *testing.T) {
	ctx := context.Background()
	db, err := Connect(ctx, baseURL())
	if err != nil {
		t.Skipf("no Postgres reachable (%v)", err)
	}
	defer db.Close()

	cfg := db.Config().ConnConfig
	// DescribeExec is NOT acceptable here: its two round trips let a
	// transaction-mode pooler swap the connection in between, which fails only
	// under concurrency.
	if cfg.DefaultQueryExecMode != pgx.QueryExecModeExec {
		t.Errorf("DefaultQueryExecMode = %v, want QueryExecModeExec — the only mode that holds nothing between statements",
			cfg.DefaultQueryExecMode)
	}
	if cfg.StatementCacheCapacity != 0 {
		t.Errorf("StatementCacheCapacity = %d, want 0", cfg.StatementCacheCapacity)
	}
	if cfg.DescriptionCacheCapacity != 0 {
		t.Errorf("DescriptionCacheCapacity = %d, want 0", cfg.DescriptionCacheCapacity)
	}
}

// An operator who sets the mode explicitly should keep it.
func TestConnectHonoursAnExplicitExecMode(t *testing.T) {
	ctx := context.Background()
	url := baseURL()
	if strings.Contains(url, "?") {
		url += "&default_query_exec_mode=cache_statement"
	} else {
		url += "?default_query_exec_mode=cache_statement"
	}

	db, err := Connect(ctx, url)
	if err != nil {
		t.Skipf("no Postgres reachable (%v)", err)
	}
	defer db.Close()

	if got := db.Config().ConnConfig.DefaultQueryExecMode; got != pgx.QueryExecModeCacheStatement {
		t.Errorf("DefaultQueryExecMode = %v, want the explicit cache_statement to win", got)
	}
}
