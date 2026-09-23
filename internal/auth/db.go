// Package auth stores dashboard accounts and sessions in a local SQLite file.
package auth

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const DefaultBcryptCost = 12

type DB struct {
	sql        *sql.DB
	Now        func() time.Time
	BcryptCost int
}

// Open creates (0600, in a 0700 directory) or opens the database at path and
// applies any pending migrations.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	dsn := "file:" + path +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_txlock=immediate"
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection serialises writers; traffic is a single operator.
	sdb.SetMaxOpenConns(1)
	db := &DB{sql: sdb, Now: time.Now, BcryptCost: DefaultBcryptCost}
	if err := db.migrate(context.Background()); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return db, nil
}

func (db *DB) Close() error { return db.sql.Close() }

func (db *DB) now() time.Time {
	if db.Now != nil {
		return db.Now()
	}
	return time.Now()
}

func (db *DB) migrate(ctx context.Context) error {
	if _, err := db.sql.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		prefix, _, _ := strings.Cut(name, "_")
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return fmt.Errorf("migration %s: bad version prefix", name)
		}
		var n int
		if err := db.sql.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
