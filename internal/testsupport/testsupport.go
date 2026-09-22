// Package testsupport gives the integration tests a real Postgres.
//
// The store and handler layers are mostly SQL, and SQL is exactly what a mock
// cannot check: a mock will happily agree that a query filters by builder_id
// when it does not. So these tests run against the same Postgres the app runs
// against, via the docker-compose stack.
//
// Each package gets its own database so `go test ./...` can run packages in
// parallel without them truncating each other's fixtures.
package testsupport

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jason-bourne-gg/plotting-society/internal/database"
)

// DefaultURL points at the docker-compose Postgres as seen from inside the
// toolchain container started by ./go.sh.
const DefaultURL = "postgres://plot:plot@postgres:5432/plotting?sslmode=disable"

func baseURL() string {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	return DefaultURL
}

// DB returns a migrated, empty database dedicated to the named package.
//
// The test is skipped, not failed, when no Postgres is reachable, so a
// checkout without the stack running still gets the pure-logic suite.
func DB(t *testing.T, pkg string) *database.DB {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbName := "plottest_" + sanitise(pkg)

	admin, err := pgx.Connect(ctx, baseURL())
	if err != nil {
		t.Skipf("no Postgres reachable (%v); start it with `make up`", err)
	}
	// CREATE DATABASE cannot run inside a transaction, and "already exists" is
	// the normal case on every run after the first.
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+dbName); err != nil &&
		!strings.Contains(err.Error(), "already exists") {
		_ = admin.Close(ctx)
		t.Fatalf("create test database: %v", err)
	}
	_ = admin.Close(ctx)

	db, err := database.Connect(ctx, replaceDBName(baseURL(), dbName))
	if err != nil {
		t.Fatalf("connect to %s: %v", dbName, err)
	}
	if err := db.Migrate(ctx); err != nil {
		db.Close()
		t.Fatalf("migrate %s: %v", dbName, err)
	}

	Reset(t, db)
	t.Cleanup(db.Close)
	return db
}

// Reset empties every table. builders cascades to societies, plots, queries,
// enquiries and the rest; users and audit rows are not reachable that way.
func Reset(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Exec(ctx,
		`TRUNCATE builders, users, audit_log RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("reset database: %v", err)
	}
}

func sanitise(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// replaceDBName swaps the path segment of a Postgres URL.
func replaceDBName(url, name string) string {
	slash := strings.LastIndex(url, "/")
	if slash < 0 {
		return url
	}
	rest := ""
	if q := strings.Index(url[slash:], "?"); q >= 0 {
		rest = url[slash+q:]
	}
	return fmt.Sprintf("%s/%s%s", url[:slash], name, rest)
}
