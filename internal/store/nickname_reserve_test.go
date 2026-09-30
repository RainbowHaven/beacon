package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/RainbowHaven/beacon/internal/migrate"
	"github.com/RainbowHaven/beacon/internal/store"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func testStore(t *testing.T) *store.Store {
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
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	return store.New(db)
}

func TestNicknameKeysNeverReuse(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	_, house, err := st.EnsureDemoTenancy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC()

	tom, err := st.CreateOccupant(ctx, store.CreateOccupantInput{
		SafeHouseID: house.ID, Nickname: "tom", ArrivedAt: today,
	})
	if err != nil {
		t.Fatal(err)
	}
	tom2, err := st.CreateOccupant(ctx, store.CreateOccupantInput{
		SafeHouseID: house.ID, Nickname: "tom-2", ArrivedAt: today,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.RenameOccupant(ctx, tom2.ID, "tom-3"); err != nil {
		t.Fatal(err)
	}
	// Former tom-2 key must stay reserved.
	if _, err := st.RenameOccupant(ctx, tom.ID, "tom-2"); !errors.Is(err, store.ErrNicknameTaken) {
		t.Fatalf("rename tom→tom-2 want ErrNicknameTaken, got %v", err)
	}
	if _, err := st.RenameOccupant(ctx, tom2.ID, "tom-2"); !errors.Is(err, store.ErrNicknameTaken) {
		t.Fatalf("rename tom-3→tom-2 want ErrNicknameTaken, got %v", err)
	}
	// Folded look-alike of a reserved key is also blocked.
	if _, err := st.CreateOccupant(ctx, store.CreateOccupantInput{
		SafeHouseID: house.ID, Nickname: "TOM!", ArrivedAt: today,
	}); !errors.Is(err, store.ErrNicknameTaken) {
		t.Fatalf("create TOM! want ErrNicknameTaken, got %v", err)
	}

	sug, err := st.SuggestNickname(ctx, house.ID, "tom", 0)
	if err != nil {
		t.Fatal(err)
	}
	if sug != "tom-4" {
		t.Fatalf("suggestion=%q want tom-4 (tom,tom-2,tom-3 reserved)", sug)
	}
}
