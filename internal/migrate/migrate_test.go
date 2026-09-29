package migrate_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/magiconair/beacon/internal/migrate"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://beacon:beacon@127.0.0.1:5433/beacon?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Skip("postgres unavailable:", err)
	}
	return db
}

func TestWipePublicSchemaThenUp(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	if err := migrate.WipePublicSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected migrations applied after wipe")
	}
	// Second wipe clears migration history; Up reapplies.
	if err := migrate.WipePublicSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err == nil {
		t.Fatal("schema_migrations should be gone after wipe")
	}
	if err := migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
}
