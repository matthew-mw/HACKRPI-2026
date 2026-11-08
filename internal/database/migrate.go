package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/matthew-mw/HACKRPI-2026/migrations"
)

// Apply upgrades the schema atomically, with a transaction-scoped lock to prevent
// two deployments from applying the same migration concurrently.
func Apply(ctx context.Context, db *sql.DB) error { return apply(ctx, db, migrations.Files) }

func apply(ctx context.Context, db *sql.DB, files fs.FS) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(73462026001)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return err
	}
	for _, name := range names {
		source, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		checksum := fmt.Sprintf("%x", sha256.Sum256(source))
		var recorded string
		err = tx.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE name=$1`, name).Scan(&recorded)
		if err == nil {
			if recorded != checksum {
				return fmt.Errorf("migration %s changed after application; restore it and add a new migration", name)
			}
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}
		// Earlier releases applied these files manually without a migration ledger.
		// Recognize a complete legacy schema, but reject partial installations.
		adopted := false
		if name == "001_init.sql" || name == "002_sessions.sql" {
			adopted, err = legacyComplete(ctx, tx, string(source))
			if err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
		}
		if !adopted {
			if _, err = tx.ExecContext(ctx, string(source)); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (name, checksum) VALUES ($1,$2)`, name, checksum); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var tablePattern = regexp.MustCompile(`(?ms)^CREATE TABLE (\w+) \(\n(.*?)^\);`)
var columnPattern = regexp.MustCompile(`(?m)^    ([a-z][a-z0-9_]*)\s+`)
var indexPattern = regexp.MustCompile(`(?m)^CREATE (?:UNIQUE )?INDEX (\w+) `)

func legacyComplete(ctx context.Context, tx *sql.Tx, source string) (bool, error) {
	tables := tablePattern.FindAllStringSubmatch(source, -1)
	found := 0
	for _, table := range tables {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, table[1]).Scan(&exists); err != nil {
			return false, err
		}
		if exists {
			found++
		}
	}
	if found == 0 {
		return false, nil
	}
	if found != len(tables) {
		return false, fmt.Errorf("partial legacy schema found; repair missing tables before deploying")
	}
	for _, table := range tables {
		for _, column := range columnPattern.FindAllStringSubmatch(table[2], -1) {
			var exists bool
			err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid=to_regclass($1) AND attname=$2 AND attnum>0 AND NOT attisdropped)`, table[1], column[1]).Scan(&exists)
			if err != nil {
				return false, err
			}
			if !exists {
				return false, fmt.Errorf("legacy schema is missing column %s.%s", table[1], column[1])
			}
		}
	}
	for _, index := range indexPattern.FindAllStringSubmatch(source, -1) {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_index WHERE indexrelid=to_regclass($1) AND indisvalid)`, index[1]).Scan(&exists); err != nil {
			return false, err
		}
		if !exists {
			return false, fmt.Errorf("legacy schema is missing index %s", index[1])
		}
	}
	// Extension-backed types must be available in a pre-existing initial schema.
	for _, ext := range []string{"pgcrypto", "vector", "postgis"} {
		if strings.Contains(source, "CREATE EXTENSION IF NOT EXISTS "+ext+";") {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname=$1)`, ext).Scan(&exists); err != nil {
				return false, err
			}
			if !exists {
				return false, fmt.Errorf("legacy schema is missing extension %s", ext)
			}
		}
	}
	return true, nil
}
