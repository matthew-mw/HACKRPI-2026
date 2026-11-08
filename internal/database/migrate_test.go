package database

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestApplyPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run migration integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	first := "CREATE TABLE widgets (\n    id integer PRIMARY KEY\n);\nCREATE INDEX idx_widgets ON widgets (id);"
	second := "CREATE TABLE sessions (\n    id integer PRIMARY KEY\n);"
	files := func() fstest.MapFS {
		return fstest.MapFS{"001_init.sql": {Data: []byte(first)}, "002_sessions.sql": {Data: []byte(second)}}
	}
	for _, scenario := range []string{"fresh-repeat-checksum", "legacy", "partial", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			schema := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
			if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
				t.Fatal(err)
			}
			defer db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
			if _, err := db.ExecContext(ctx, "SET search_path TO "+schema); err != nil {
				t.Fatal(err)
			}
			f := files()
			switch scenario {
			case "legacy":
				if _, err := db.ExecContext(ctx, first); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, "INSERT INTO widgets VALUES (42)"); err != nil {
					t.Fatal(err)
				}
			case "partial":
				if _, err := db.ExecContext(ctx, "CREATE TABLE widgets (wrong integer)"); err != nil {
					t.Fatal(err)
				}
				if err := apply(ctx, db, f); err == nil || !strings.Contains(err.Error(), "missing column") {
					t.Fatalf("expected partial schema rejection: %v", err)
				}
				return
			case "rollback":
				f["003_bad.sql"] = &fstest.MapFile{Data: []byte("SELECT definitely_missing_column")}
				if err := apply(ctx, db, f); err == nil {
					t.Fatal("expected failure")
				}
				var exists bool
				if err := db.QueryRowContext(ctx, "SELECT to_regclass('widgets') IS NOT NULL").Scan(&exists); err != nil || exists {
					t.Fatalf("migration not rolled back: %v", err)
				}
				return
			}
			if err := apply(ctx, db, f); err != nil {
				t.Fatal(err)
			}
			if err := apply(ctx, db, f); err != nil {
				t.Fatalf("repeat failed: %v", err)
			}
			var count int
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 2 {
				t.Fatalf("ledger count %d: %v", count, err)
			}
			if scenario == "legacy" {
				if err := db.QueryRowContext(ctx, "SELECT count(*) FROM widgets WHERE id=42").Scan(&count); err != nil || count != 1 {
					t.Fatal("legacy data not preserved")
				}
			}
			f["001_init.sql"] = &fstest.MapFile{Data: []byte(first + "\n-- modified")}
			if err := apply(ctx, db, f); err == nil || !strings.Contains(err.Error(), "changed after application") {
				t.Fatalf("expected checksum rejection: %v", err)
			}
		})
	}
}
