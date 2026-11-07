package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

// TEST_DATABASE_URL must point at a disposable PostgreSQL database. The test
// creates its own schema, leaving existing application tables untouched.
func TestPostgresSessionsAndCredentials(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	schema := fmt.Sprintf("auth_test_%d", time.Now().UnixNano())
	if _, err = db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	if _, err = db.ExecContext(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	// Minimal users fixture: session migration is independent of vector/PostGIS.
	if _, err = db.ExecContext(ctx, `CREATE TABLE users (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), email text NOT NULL, password_hash text, first_name text, last_name text)`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/002_sessions.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	id := "00000000-0000-0000-0000-000000000001"
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`, id, "user@example.com", string(hash)); err != nil {
		t.Fatal(err)
	}
	credentials := &PasswordCredentials{DB: db}
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX uq_users_email_ci ON users (lower(email))`); err != nil {
		t.Fatal(err)
	}
	registration := Registration{FirstName: "New", LastName: "User", Email: "new@example.com", Password: "long-password"}
	newID, err := credentials.Register(ctx, registration)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := credentials.Authenticate(ctx, registration.Email, registration.Password); err != nil || got != newID {
		t.Fatalf("registered login: %s %v", got, err)
	}
	registration.Email = "NEW@example.com"
	if _, err := credentials.Register(ctx, registration); !errors.Is(err, ErrEmailExists) {
		t.Fatalf("duplicate signup: %v", err)
	}

	got, err := credentials.Authenticate(ctx, " USER@EXAMPLE.COM ", "correct-password")
	if err != nil || got != id {
		t.Fatalf("login: %q %v", got, err)
	}
	for _, tc := range []struct{ email, password string }{{"user@example.com", "wrong"}, {"absent@example.com", "correct-password"}} {
		if _, err := credentials.Authenticate(ctx, tc.email, tc.password); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("invalid login: %v", err)
		}
	}
	store := &PostgresStore{DB: db}
	h := sha256.Sum256([]byte("test-token"))
	if err := store.Create(ctx, h, id, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if session, err := store.Lookup(ctx, h); err != nil || session.UserID != id {
		t.Fatalf("lookup: %+v %v", session, err)
	}
	if err := store.Delete(ctx, h); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lookup(ctx, h); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked session: %v", err)
	}
	if err := store.Create(ctx, h, id, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE sessions SET created_at=now()-interval '2 hours', expires_at=now()-interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lookup(ctx, h); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired session: %v", err)
	}
	if err := store.DeleteExpired(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cleanup: %d %v", count, err)
	}
	for _, token := range []string{"device-one", "device-two"} {
		if err := store.Create(ctx, sha256.Sum256([]byte(token)), id, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DeleteUserSessions(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("revoke all: %d %v", count, err)
	}
}
