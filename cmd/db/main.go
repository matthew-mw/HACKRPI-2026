// Command db checks or initializes the database configured by DATABASE_URL.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/matthew-mw/HACKRPI-2026/internal/database"
)

func fail(err error) {
	// Do not print connection strings or PostgreSQL detail fields containing data.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		log.Fatalf("database setup failed: SQLSTATE %s: %s", pgErr.Code, pgErr.Message)
	}
	log.Fatal("database setup failed; check DATABASE_URL and network access")
}

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "check" && os.Args[1] != "init-db" && os.Args[1] != "migrate") {
		log.Fatal("usage: go run ./cmd/db [check|init-db|migrate]")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fail(err)
	}
	defer db.Close()
	if err = db.PingContext(ctx); err != nil {
		fail(err)
	}
	if os.Args[1] == "check" {
		var users, sessions bool
		err = db.QueryRowContext(ctx, `SELECT to_regclass('public.users') IS NOT NULL, to_regclass('public.sessions') IS NOT NULL`).Scan(&users, &sessions)
		if err != nil {
			fail(err)
		}
		log.Printf("connected; users table: %t; sessions table: %t", users, sessions)
		if !users || !sessions {
			log.Fatal("schema incomplete; initialize a new database with sh scripts/dev.sh init-db")
		}
		return
	}
	if err := database.Apply(ctx, db); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			fail(err)
		}
		log.Fatal(err)
	}
	log.Print("database migrations complete")
}
